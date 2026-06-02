package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/internal/constants"
	"gorm.io/gorm"
)

// ACDPoolTransferAuditParams scopes transfer candidate audit for one inbound pick.
type ACDPoolTransferAuditParams struct {
	TenantID             uint
	InboundTrunkNumberID uint
	ExcludeIDs           []uint
}

// ACDPoolTransferCandidateAudit is one pool row with DB/shift eligibility reasons.
type ACDPoolTransferCandidateAudit struct {
	Row      ACDPoolTarget
	Eligible bool
	Reasons  []string
}

// AuditACDPoolTransferCandidates lists tenant pool rows and why each is or is not eligible
// before transfer target selection (weight/work_state/route/shift/trunk scope/web heartbeat).
func AuditACDPoolTransferCandidates(ctx context.Context, db *gorm.DB, p ACDPoolTransferAuditParams) ([]ACDPoolTransferCandidateAudit, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	q := ActiveACDPoolTargets(db.WithContext(ctx))
	if p.TenantID > 0 {
		q = q.Where("tenant_id = ?", p.TenantID)
	}
	q = q.Order("weight DESC").Order("id ASC")
	var rows []ACDPoolTarget
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}

	exclude := make(map[uint]struct{}, len(p.ExcludeIDs))
	for _, id := range p.ExcludeIDs {
		if id > 0 {
			exclude[id] = struct{}{}
		}
	}
	now := time.Now()
	loc := ACDShiftTimeLocation()
	freshWebSince := now.Add(-constants.WebSeatStaleAfter)

	out := make([]ACDPoolTransferCandidateAudit, 0, len(rows))
	for _, row := range rows {
		reasons := ACDPoolTransferRejectReasons(row, p, exclude, freshWebSince, now, loc)
		out = append(out, ACDPoolTransferCandidateAudit{
			Row:      row,
			Eligible: len(reasons) == 0,
			Reasons:  reasons,
		})
	}
	return out, nil
}

// ACDPoolTransferRejectReasons returns non-empty reasons when the row fails DB/shift eligibility.
func ACDPoolTransferRejectReasons(row ACDPoolTarget, p ACDPoolTransferAuditParams, exclude map[uint]struct{}, freshWebSince, now time.Time, loc *time.Location) []string {
	var reasons []string
	if _, ok := exclude[row.ID]; ok {
		reasons = append(reasons, "already_attempted_this_call")
	}
	if p.InboundTrunkNumberID > 0 {
		if row.TrunkNumberID != 0 && row.TrunkNumberID != p.InboundTrunkNumberID {
			reasons = append(reasons, fmt.Sprintf("trunk_number_mismatch(row=%d inbound=%d)", row.TrunkNumberID, p.InboundTrunkNumberID))
		}
	} else if row.TrunkNumberID != 0 {
		reasons = append(reasons, fmt.Sprintf("inbound_did_unknown_only_tenant_wide_pool(row_trunk=%d)", row.TrunkNumberID))
	}
	if row.Weight <= 0 {
		reasons = append(reasons, "weight<=0")
	}
	ws := strings.ToLower(strings.TrimSpace(row.WorkState))
	if ws != constants.ACDWorkStateAvailable {
		reasons = append(reasons, fmt.Sprintf("work_state=%s", strings.TrimSpace(row.WorkState)))
	}
	rt := strings.ToLower(strings.TrimSpace(row.RouteType))
	switch rt {
	case constants.ACDPoolRouteTypeSIP, constants.ACDPoolRouteTypeWeb:
	default:
		reasons = append(reasons, fmt.Sprintf("route_type=%s", strings.TrimSpace(row.RouteType)))
	}
	if rt == constants.ACDPoolRouteTypeWeb {
		if row.WebSeatLastSeenAt == nil {
			reasons = append(reasons, "web_seat_never_seen")
		} else if row.WebSeatLastSeenAt.Before(freshWebSince) {
			reasons = append(reasons, fmt.Sprintf("web_seat_stale(last_seen=%s)", row.WebSeatLastSeenAt.UTC().Format(time.RFC3339)))
		}
	}
	if !ACDFitsShiftSchedule(row.ShiftScheduleJSON, now, loc) {
		reasons = append(reasons, "outside_shift_schedule")
	}
	return reasons
}
