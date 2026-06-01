package models

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"gorm.io/gorm"
)

// NormalizeACDDispatchMode normalizes sip_trunk_numbers.acd_dispatch_mode.
func NormalizeACDDispatchMode(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case constants.ACDDispatchModeRoundRobin, "rr":
		return constants.ACDDispatchModeRoundRobin
	case "", constants.ACDDispatchModeWeight:
		return constants.ACDDispatchModeWeight
	default:
		return constants.ACDDispatchModeWeight
	}
}

// ACDPoolTarget is one row in the transfer routing table (acd_pool_targets) when cmd/sip uses a database.
// Selection: highest Weight first, then lowest id; only Weight>0 and WorkState==available.
// SIP rows: internal TargetValue = sip_users.username; trunk = dial string + trunk host fields.
// Web rows: TargetValue usually empty; WebSeat handoff when this row wins over SIP rows by Weight.
type ACDPoolTarget struct {
	BaseModel
	TenantID              uint       `json:"tenantId" gorm:"index;not null;default:0"`                             // SaaS isolation
	TrunkNumberID         uint       `json:"trunkNumberId" gorm:"column:trunk_number_id;not null;default:0;index"` // per-number routing
	Name                  string     `json:"name" gorm:"size:128"`                                                 // optional admin label
	RouteType             string     `json:"routeType" gorm:"size:16;not null;index"`                              // RouteType is constants.ACDPoolRouteTypeSIP or constants.ACDPoolRouteTypeWeb.
	TargetValue           string     `json:"targetValue" gorm:"size:256"`                                          // TargetValue: sip internal → sip_users.username; sip trunk → dial digits / URI; web → usually empty.
	SipSource             string     `json:"sipSource" gorm:"size:16;not null;default:'';index"`                   // SipSource: internal | trunk when RouteType is SIP; empty when web.
	SipTrunkHost          string     `json:"sipTrunkHost" gorm:"size:128"`                                         // Sip trunk only: next SIP hop for INVITE (Request-URI host:port + optional signaling override).
	SipTrunkPort          int        `json:"sipTrunkPort" gorm:"not null;default:0"`
	SipTrunkSignalingAddr string     `json:"sipTrunkSignalingAddr" gorm:"size:160"` // host:port, optional; default host:port from above
	SipCallerID           string     `json:"sipCallerId" gorm:"size:64"`            // SIP only: optional outbound From user / display (like SIP_CALLER_ID); empty → cmd/sip uses global .env default.
	SipCallerDisplayName  string     `json:"sipCallerDisplayName" gorm:"size:128"`
	Weight                int        `json:"weight" gorm:"not null;default:0;index"`                  // Weight: higher = higher priority when selecting; 0 = disabled (not eligible).
	WorkState             string     `json:"workState" gorm:"size:24;not null;default:offline;index"` // WorkState: see ACDWorkState*; default offline until sign-in or integration sets available.
	WorkStateAt           *time.Time `json:"workStateAt"`                                             // WorkStateAt: optional last transition (ring timeouts, metrics).
	WebSeatLastSeenAt     *time.Time `json:"webSeatLastSeenAt" gorm:"column:web_seat_last_seen_at"`   // WebSeatLastSeenAt: route_type=web only; last heartbeat from browser (keepalive). Used for pick + admin "链路在线".
	// ShiftScheduleJSON optional weekly windows, e.g. [{"weekdays":[1,2,3,4,5],"start":"09:00","end":"18:00"}] (weekdays: 0=Sun .. 6=Sat). Empty = no restriction.
	ShiftScheduleJSON string `json:"shiftSchedule" gorm:"column:shift_schedule_json;type:text"`
	// MetaData optional JSON object for template vars (e.g. transfer_agent_brief {{MetaData.FactoryNumber}}).
	// Use this for structured fields; BaseModel.Remark remains a short plain-text admin note.
	MetaData string `json:"metaData" gorm:"column:meta_data;type:text"`
}

// WebSeatLastSeenFresh reports whether a web seat heartbeat is recent enough to treat the row as reachable.
func WebSeatLastSeenFresh(t *time.Time) bool {
	if t == nil {
		return false
	}
	return time.Since(*t) <= constants.WebSeatStaleAfter
}

func (ACDPoolTarget) TableName() string {
	return constants.ACDPoolTargetTableName
}

// --- Normalization (admin API + transfer pick) ---

// ParseACDRouteType returns canonical route type or false.
func ParseACDRouteType(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case constants.ACDPoolRouteTypeSIP, constants.ACDPoolRouteTypeWeb:
		return s, true
	default:
		return "", false
	}
}

// NormalizeACDSipSource returns internal or trunk.
func NormalizeACDSipSource(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == constants.ACDSipSourceTrunk || s == "external" {
		return constants.ACDSipSourceTrunk
	}
	return constants.ACDSipSourceInternal
}

// NormalizeACDWorkState returns a known work_state or offline.
func NormalizeACDWorkState(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case constants.ACDWorkStateOffline, constants.ACDWorkStateAvailable, constants.ACDWorkStateRinging,
		constants.ACDWorkStateBusy, constants.ACDWorkStateACW, constants.ACDWorkStateBreak:
		return s
	default:
		return constants.ACDWorkStateOffline
	}
}

// NormalizeACDTrunkPort returns a valid SIP port or 6050.
func NormalizeACDTrunkPort(p int) int {
	if p <= 0 || p >= 65536 {
		return 6050
	}
	return p
}

// ACDSipInternalLiveLineEligible is true when list UI may show SIP registration hint.
func ACDSipInternalLiveLineEligible(row ACDPoolTarget) bool {
	if row.RouteType != constants.ACDPoolRouteTypeSIP || strings.TrimSpace(row.TargetValue) == "" {
		return false
	}
	src := strings.ToLower(strings.TrimSpace(row.SipSource))
	if src == constants.ACDSipSourceTrunk || src == "external" {
		return false
	}
	return true
}

// ACDTrunkStorageFields returns DB trunk columns; empty when not sip+trunk.
func ACDTrunkStorageFields(routeType, sipSource, trunkHost string, trunkPort int, trunkSig string) (host string, port int, sig string) {
	if routeType != constants.ACDPoolRouteTypeSIP || sipSource != constants.ACDSipSourceTrunk {
		return "", 0, ""
	}
	return strings.TrimSpace(trunkHost), NormalizeACDTrunkPort(trunkPort), strings.TrimSpace(trunkSig)
}

// ACDCallerStorageFields returns outbound CLI fields for SIP rows only.
func ACDCallerStorageFields(routeType, sipCallerID, sipCallerDisplayName string) (id, disp string) {
	if routeType != constants.ACDPoolRouteTypeSIP {
		return "", ""
	}
	return strings.TrimSpace(sipCallerID), strings.TrimSpace(sipCallerDisplayName)
}

// ValidateACDTrunkCreateUpdate requires dial target + gateway for trunk SIP rows.
func ValidateACDTrunkCreateUpdate(routeType, sipSource, targetValue, trunkHost string) bool {
	if routeType != constants.ACDPoolRouteTypeSIP || sipSource != constants.ACDSipSourceTrunk {
		return true
	}
	return strings.TrimSpace(targetValue) != "" && strings.TrimSpace(trunkHost) != ""
}

// --- Queries ---

// ActiveACDPoolTargets is the non-deleted scope.
func ActiveACDPoolTargets(db *gorm.DB) *gorm.DB {
	return db.Model(&ACDPoolTarget{})
}

// ListACDPoolTargetsPage lists active targets; routeType empty skips filter.
// trunkNumberID = 0 lists all numbers; >0 returns rows bound to that specific TrunkNumber.
func ListACDPoolTargetsPage(db *gorm.DB, tenantID uint, page, size int, routeType string, trunkNumberID uint) ([]ACDPoolTarget, int64, error) {
	q := ActiveACDPoolTargets(db).Where("tenant_id = ?", tenantID)
	if rt := strings.TrimSpace(routeType); rt != "" {
		if t, ok := ParseACDRouteType(rt); ok {
			q = q.Where("route_type = ?", t)
		}
	}
	if trunkNumberID > 0 {
		q = q.Where("trunk_number_id = ?", trunkNumberID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	var list []ACDPoolTarget
	if err := q.Order("weight DESC, id DESC").Offset(offset).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetActiveACDPoolTargetByID returns one active row.
func GetActiveACDPoolTargetByID(db *gorm.DB, id uint) (ACDPoolTarget, error) {
	var row ACDPoolTarget
	err := ActiveACDPoolTargets(db).Where("id = ?", id).First(&row).Error
	return row, err
}

// FindSIPACDPoolTargetForIncomingPoll resolves a SIP pool row for the agent incoming poll API.
// Exactly one of id, name, or targetValue should be set (id wins when > 0).
func FindSIPACDPoolTargetForIncomingPoll(db *gorm.DB, tenantID, id uint, name, targetValue string) (ACDPoolTarget, bool, error) {
	if db == nil || tenantID == 0 {
		return ACDPoolTarget{}, false, nil
	}
	if id > 0 {
		row, err := GetActiveACDPoolTargetByID(db, id)
		if err != nil {
			return ACDPoolTarget{}, false, err
		}
		if row.TenantID != tenantID || row.RouteType != constants.ACDPoolRouteTypeSIP {
			return ACDPoolTarget{}, false, nil
		}
		return row, true, nil
	}
	q := ActiveACDPoolTargets(db).
		Where("tenant_id = ?", tenantID).
		Where("route_type = ?", constants.ACDPoolRouteTypeSIP)
	name = strings.TrimSpace(name)
	targetValue = strings.TrimSpace(targetValue)
	switch {
	case name != "":
		q = q.Where("name = ?", name)
	case targetValue != "":
		q = q.Where("target_value = ?", targetValue)
	default:
		return ACDPoolTarget{}, false, nil
	}
	var row ACDPoolTarget
	if err := q.First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ACDPoolTarget{}, false, nil
		}
		return ACDPoolTarget{}, false, err
	}
	return row, true, nil
}

// ReloadACDPoolTargetByID refetches by primary key (any delete state).
func ReloadACDPoolTargetByID(db *gorm.DB, id uint) (ACDPoolTarget, error) {
	var row ACDPoolTarget
	err := db.Where("id = ?", id).First(&row).Error
	return row, err
}

// SoftDeleteACDPoolTargetByID soft-deletes an active row.
func SoftDeleteACDPoolTargetByID(db *gorm.DB, id uint, updateBy string) (int64, error) {
	meta := BaseModel{}
	meta.SoftDelete(updateBy)
	u := map[string]any{
		"updated_at": meta.UpdatedAt,
		"deleted_at": meta.DeletedAt,
	}
	if meta.UpdateBy != "" {
		u["update_by"] = meta.UpdateBy
	}
	res := db.Model(&ACDPoolTarget{}).Where("id = ?", id).Updates(u)
	return res.RowsAffected, res.Error
}

// SoftDeleteACDPoolTargetByIDForTenant deletes within tenant scope.
func SoftDeleteACDPoolTargetByIDForTenant(db *gorm.DB, id uint, tenantID uint, updateBy string) (int64, error) {
	meta := BaseModel{}
	meta.SoftDelete(updateBy)
	u := map[string]any{
		"updated_at": meta.UpdatedAt,
		"deleted_at": meta.DeletedAt,
	}
	if meta.UpdateBy != "" {
		u["update_by"] = meta.UpdateBy
	}
	res := db.Model(&ACDPoolTarget{}).Where("id = ? AND tenant_id = ?", id, tenantID).Updates(u)
	return res.RowsAffected, res.Error
}

// NewACDPoolTargetForCreate builds a row after request normalization.
func NewACDPoolTargetForCreate(
	name, routeType, sipSource, targetValue string,
	trunkHost string, trunkPort int, trunkSig string,
	sipCallerID, sipCallerDisplayName string,
	weight int, workState string,
	now time.Time,
	webSeatLastSeen *time.Time,
	shiftScheduleJSON string,
	trunkNumberID uint,
	remark string,
	metaDataJSON string,
) ACDPoolTarget {
	th, tp, ts := ACDTrunkStorageFields(routeType, sipSource, trunkHost, trunkPort, trunkSig)
	cid, cdn := ACDCallerStorageFields(routeType, sipCallerID, sipCallerDisplayName)
	row := ACDPoolTarget{
		TrunkNumberID:         trunkNumberID,
		Name:                  strings.TrimSpace(name),
		RouteType:             routeType,
		SipSource:             sipSource,
		TargetValue:           strings.TrimSpace(targetValue),
		SipTrunkHost:          th,
		SipTrunkPort:          tp,
		SipTrunkSignalingAddr: ts,
		SipCallerID:           cid,
		SipCallerDisplayName:  cdn,
		Weight:                weight,
		WorkState:             workState,
		WorkStateAt:           &now,
		WebSeatLastSeenAt:     webSeatLastSeen,
		ShiftScheduleJSON:     strings.TrimSpace(shiftScheduleJSON),
		MetaData:              strings.TrimSpace(metaDataJSON),
	}
	row.Remark = strings.TrimSpace(remark)
	return row
}

// BuildACDPoolTargetUpdateMap builds GORM Updates for admin PUT.
func BuildACDPoolTargetUpdateMap(
	existing ACDPoolTarget,
	name, routeType, sipSource, targetValue string,
	trunkHost string, trunkPort int, trunkSig string,
	sipCallerID, sipCallerDisplayName string,
	weight int, workState string,
	now time.Time,
	updateBy string,
	shiftScheduleJSON string,
	trunkNumberID uint,
	remark string,
	metaDataJSON string,
) map[string]any {
	th, tp, ts := ACDTrunkStorageFields(routeType, sipSource, trunkHost, trunkPort, trunkSig)
	cid, cdn := ACDCallerStorageFields(routeType, sipCallerID, sipCallerDisplayName)
	u := map[string]any{
		"trunk_number_id":          trunkNumberID,
		"name":                     strings.TrimSpace(name),
		"route_type":               routeType,
		"sip_source":               sipSource,
		"target_value":             strings.TrimSpace(targetValue),
		"sip_trunk_host":           th,
		"sip_trunk_port":           tp,
		"sip_trunk_signaling_addr": ts,
		"sip_caller_id":            cid,
		"sip_caller_display_name":  cdn,
		"weight":                   weight,
		"work_state":               workState,
		"updated_at":               now,
		"shift_schedule_json":      strings.TrimSpace(shiftScheduleJSON),
		"meta_data":                strings.TrimSpace(metaDataJSON),
		"remark":                   strings.TrimSpace(remark),
	}
	if existing.WorkState != workState {
		u["work_state_at"] = now
	}
	if routeType == constants.ACDPoolRouteTypeWeb && workState == constants.ACDWorkStateAvailable {
		u["web_seat_last_seen_at"] = now
	}
	meta := BaseModel{}
	meta.SetUpdateInfo(updateBy)
	if meta.UpdateBy != "" {
		u["update_by"] = meta.UpdateBy
	}
	return u
}

// ClearACDPoolTargetWebSeatLastSeen sets web_seat_last_seen_at NULL (web row offline).
func ClearACDPoolTargetWebSeatLastSeen(db *gorm.DB, id uint) error {
	return db.Model(&ACDPoolTarget{}).Where("id = ?", id).Update("web_seat_last_seen_at", nil).Error
}

// UpdateACDPoolTargetWebSeatHeartbeat refreshes web seat presence only.
// It must not overwrite work_state (ringing/busy/acw etc.) — those are driven by transfer + webseat lifecycle.
func UpdateACDPoolTargetWebSeatHeartbeat(db *gorm.DB, id uint, operator string, now time.Time) error {
	u := map[string]any{
		"web_seat_last_seen_at": now,
		"updated_at":            now,
	}
	meta := BaseModel{}
	meta.SetUpdateInfo(operator)
	if meta.UpdateBy != "" {
		u["update_by"] = meta.UpdateBy
	}
	return db.Model(&ACDPoolTarget{}).Where("id = ?", id).Updates(u).Error
}

// UpdateACDPoolTargetWorkState updates work_state for an active row (admin, SIP transfer, or webseat).
func UpdateACDPoolTargetWorkState(ctx context.Context, db *gorm.DB, id uint, workState string, updateBy string) error {
	if db == nil || id == 0 {
		return nil
	}
	ws := NormalizeACDWorkState(workState)
	now := time.Now()
	u := map[string]any{
		"work_state":    ws,
		"work_state_at": now,
		"updated_at":    now,
	}
	meta := BaseModel{}
	meta.SetUpdateInfo(updateBy)
	if meta.UpdateBy != "" {
		u["update_by"] = meta.UpdateBy
	}
	return ActiveACDPoolTargets(db.WithContext(ctx)).Where("id = ?", id).Updates(u).Error
}

// WebSeatActorMayTouchRow allows heartbeat when CreateBy is empty or matches operator.
func WebSeatActorMayTouchRow(row ACDPoolTarget, operator string) bool {
	operator = strings.TrimSpace(operator)
	cb := strings.TrimSpace(row.CreateBy)
	if cb == "" {
		return true
	}
	return cb == operator
}

func filterACDPoolTargetsByShift(rows []ACDPoolTarget) []ACDPoolTarget {
	loc := ACDShiftTimeLocation()
	now := time.Now()
	out := make([]ACDPoolTarget, 0, len(rows))
	for _, row := range rows {
		if ACDFitsShiftSchedule(row.ShiftScheduleJSON, now, loc) {
			out = append(out, row)
		}
	}
	return out
}

// listEligibleACDPoolTargetsForTransferScoped returns eligible rows for one trunk_number_id scope (specific DID row id, or 0 = tenant-wide fallback).
func listEligibleACDPoolTargetsForTransferScoped(ctx context.Context, db *gorm.DB, excludeIDs []uint, limit int, tenantID uint, trunkNumberScope uint, freshWebSince time.Time) ([]ACDPoolTarget, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		limit = 64
	}
	q := ActiveACDPoolTargets(db.WithContext(ctx))
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	q = q.Where("trunk_number_id = ?", trunkNumberScope).
		Where("weight > ? AND work_state = ? AND route_type IN ?",
			0, constants.ACDWorkStateAvailable,
			[]string{constants.ACDPoolRouteTypeSIP, constants.ACDPoolRouteTypeWeb}).
		Where("(route_type != ? OR (web_seat_last_seen_at IS NOT NULL AND web_seat_last_seen_at > ?))",
			constants.ACDPoolRouteTypeWeb, freshWebSince).
		Order("weight DESC").Order("id ASC").
		Limit(limit)
	if len(excludeIDs) > 0 {
		q = q.Where("id NOT IN ?", excludeIDs)
	}
	var rows []ACDPoolTarget
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return filterACDPoolTargetsByShift(rows), nil
}

// ListEligibleACDPoolTargetsForTransfer returns pool rows eligible for transfer, honoring exclude IDs and shift_schedule_json.
// inboundTrunkNumberID is sip_trunk_numbers.id resolved from the inbound call's called party (To / Request-URI user).
// When >0, only rows with trunk_number_id = inboundTrunkNumberID are considered first; if none match, falls back to trunk_number_id = 0 (tenant-wide pool).
// When 0 (unknown DID), only trunk_number_id = 0 rows are used so per-number Web seats never steal unrelated DIDs.
func ListEligibleACDPoolTargetsForTransfer(ctx context.Context, db *gorm.DB, excludeIDs []uint, limit int, tenantID uint, inboundTrunkNumberID uint) ([]ACDPoolTarget, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	freshWebSince := time.Now().Add(-constants.WebSeatStaleAfter)
	if inboundTrunkNumberID > 0 {
		specific, err := listEligibleACDPoolTargetsForTransferScoped(ctx, db, excludeIDs, limit, tenantID, inboundTrunkNumberID, freshWebSince)
		if err != nil {
			return nil, err
		}
		if len(specific) > 0 {
			return specific, nil
		}
	}
	return listEligibleACDPoolTargetsForTransferScoped(ctx, db, excludeIDs, limit, tenantID, 0, freshWebSince)
}

// PickEligibleACDPoolTargetForTransfer loads highest-weight available target (same rule as sipserver.PickTransferDialTarget query).
// tenantID>0 restricts to rows for that tenant; 0 preserves legacy behaviour (pool shared until inbound calls carry tenant scope).
func PickEligibleACDPoolTargetForTransfer(ctx context.Context, db *gorm.DB, excludeIDs []uint, tenantID uint, inboundTrunkNumberID uint) (ACDPoolTarget, error) {
	rows, err := ListEligibleACDPoolTargetsForTransfer(ctx, db, excludeIDs, 64, tenantID, inboundTrunkNumberID)
	if err != nil {
		return ACDPoolTarget{}, err
	}
	if len(rows) == 0 {
		return ACDPoolTarget{}, gorm.ErrRecordNotFound
	}
	return rows[0], nil
}

var acdRRLastPicked sync.Map // key=tenantID:trunkNumberScope:topWeight -> uint(lastPickedTargetID)

// PickEligibleACDPoolTargetForTransferWithMode picks one eligible target using the given dispatch mode.
// Modes:
//   - weight: deterministic (existing behavior): highest weight, then lowest id.
//   - round_robin: hybrid strategy (available-only + top-weight first + round-robin among same top weight).
//     This gives "idle priority + weight + round-robin" in one normalized flow.
func PickEligibleACDPoolTargetForTransferWithMode(ctx context.Context, db *gorm.DB, excludeIDs []uint, tenantID uint, inboundTrunkNumberID uint, mode string) (ACDPoolTarget, error) {
	mode = NormalizeACDDispatchMode(mode)
	if mode != constants.ACDDispatchModeRoundRobin {
		return PickEligibleACDPoolTargetForTransfer(ctx, db, excludeIDs, tenantID, inboundTrunkNumberID)
	}
	rows, err := ListEligibleACDPoolTargetsForTransfer(ctx, db, excludeIDs, 64, tenantID, inboundTrunkNumberID)
	if err != nil {
		return ACDPoolTarget{}, err
	}
	if len(rows) == 0 {
		return ACDPoolTarget{}, gorm.ErrRecordNotFound
	}
	// Always sort by id for stable RR candidate order.
	slices.SortFunc(rows, func(a, b ACDPoolTarget) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	// Keep only the top-weight group: weight priority remains effective in RR mode.
	topWeight := rows[0].Weight
	group := make([]ACDPoolTarget, 0, len(rows))
	for _, r := range rows {
		if r.Weight == topWeight {
			group = append(group, r)
		}
	}
	if len(group) == 0 {
		return ACDPoolTarget{}, gorm.ErrRecordNotFound
	}

	key := fmt.Sprintf("%d:%d:%d", tenantID, inboundTrunkNumberID, topWeight)
	var last uint
	if v, ok := acdRRLastPicked.Load(key); ok {
		if n, ok := v.(uint); ok {
			last = n
		}
	}
	// find next after last id (wrap)
	pick := group[0]
	if last != 0 {
		for i := 0; i < len(group); i++ {
			if group[i].ID == last {
				pick = group[(i+1)%len(group)]
				break
			}
			// if last falls between ids, pick the first higher id
			if group[i].ID > last {
				pick = group[i]
				break
			}
		}
	}
	acdRRLastPicked.Store(key, pick.ID)
	return pick, nil
}

// ApplyACDPoolTargetShiftWorkState reconciles one row's work_state with its shift schedule.
// Empty shift schedule means 24/7 (always in shift). Break/busy/ringing/acw are not promoted
// to available; outside-shift available/ringing/break/acw become offline.
func ApplyACDPoolTargetShiftWorkState(ctx context.Context, db *gorm.DB, row *ACDPoolTarget, now time.Time, updateBy string) (bool, error) {
	if db == nil || row == nil || row.ID == 0 {
		return false, nil
	}
	if updateBy == "" {
		updateBy = "acd-shift"
	}
	loc := ACDShiftTimeLocation()
	inShift := ACDFitsShiftSchedule(row.ShiftScheduleJSON, now, loc)
	ws := NormalizeACDWorkState(row.WorkState)

	if !inShift {
		switch ws {
		case constants.ACDWorkStateAvailable, constants.ACDWorkStateRinging, constants.ACDWorkStateBreak, constants.ACDWorkStateACW:
			if err := UpdateACDPoolTargetWorkState(ctx, db, row.ID, constants.ACDWorkStateOffline, updateBy); err != nil {
				return false, err
			}
			row.WorkState = constants.ACDWorkStateOffline
			return true, nil
		}
		return false, nil
	}

	if ws != constants.ACDWorkStateOffline {
		return false, nil
	}
	if row.RouteType == constants.ACDPoolRouteTypeWeb && !WebSeatLastSeenFresh(row.WebSeatLastSeenAt) {
		return false, nil
	}
	if err := UpdateACDPoolTargetWorkState(ctx, db, row.ID, constants.ACDWorkStateAvailable, updateBy); err != nil {
		return false, err
	}
	row.WorkState = constants.ACDWorkStateAvailable
	return true, nil
}

// MarkACDPoolTargetsOfflineOutsideSchedule forces offline when current time is outside configured shift windows.
func MarkACDPoolTargetsOfflineOutsideSchedule(ctx context.Context, db *gorm.DB, now time.Time) (int64, error) {
	if db == nil {
		return 0, nil
	}
	loc := ACDShiftTimeLocation()
	var rows []ACDPoolTarget
	err := ActiveACDPoolTargets(db.WithContext(ctx)).
		Where("shift_schedule_json IS NOT NULL AND shift_schedule_json <> ?", "").
		Where("work_state IN ?", []string{
			constants.ACDWorkStateAvailable, constants.ACDWorkStateRinging, constants.ACDWorkStateBreak, constants.ACDWorkStateACW,
		}).
		Find(&rows).Error
	if err != nil {
		return 0, err
	}
	var n int64
	for i := range rows {
		if ACDFitsShiftSchedule(rows[i].ShiftScheduleJSON, now, loc) {
			continue
		}
		changed, err := ApplyACDPoolTargetShiftWorkState(ctx, db, &rows[i], now, "acd-shift")
		if err != nil || !changed {
			continue
		}
		n++
	}
	return n, nil
}

// MarkACDPoolTargetsAvailableInsideSchedule sets offline seats to available when current time
// falls inside configured shift windows (empty schedule = 24/7). Intentional break is left
// unchanged. Web seats require a fresh heartbeat so disconnected browsers are not marked reachable.
func MarkACDPoolTargetsAvailableInsideSchedule(ctx context.Context, db *gorm.DB, now time.Time) (int64, error) {
	if db == nil {
		return 0, nil
	}
	var rows []ACDPoolTarget
	err := ActiveACDPoolTargets(db.WithContext(ctx)).
		Where("work_state = ?", constants.ACDWorkStateOffline).
		Find(&rows).Error
	if err != nil {
		return 0, err
	}
	var n int64
	for i := range rows {
		changed, err := ApplyACDPoolTargetShiftWorkState(ctx, db, &rows[i], now, "acd-shift")
		if err != nil || !changed {
			continue
		}
		n++
	}
	return n, nil
}

// ListActiveWebACDPoolTargetsByCreateBy lists active web rows owned by the same creator within tenant.
func ListActiveWebACDPoolTargetsByCreateBy(ctx context.Context, db *gorm.DB, createBy string, tenantID uint) ([]ACDPoolTarget, error) {
	createBy = strings.TrimSpace(createBy)
	if createBy == "" {
		return nil, nil
	}
	var rows []ACDPoolTarget
	q := ActiveACDPoolTargets(db.WithContext(ctx)).
		Where("route_type = ? AND create_by = ?", constants.ACDPoolRouteTypeWeb, createBy).
		Where("tenant_id = ?", tenantID)
	err := q.Order("updated_at DESC").Order("id DESC").Find(&rows).Error
	return rows, err
}

// SoftDeleteACDPoolTargetsByIDs soft-deletes rows by ids.
func SoftDeleteACDPoolTargetsByIDs(ctx context.Context, db *gorm.DB, ids []uint, updateBy string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	meta := BaseModel{}
	meta.SoftDelete(updateBy)
	u := map[string]any{
		"updated_at": meta.UpdatedAt,
		"deleted_at": meta.DeletedAt,
	}
	if meta.UpdateBy != "" {
		u["update_by"] = meta.UpdateBy
	}
	res := db.WithContext(ctx).Model(&ACDPoolTarget{}).Where("id IN ?", ids).Updates(u)
	return res.RowsAffected, res.Error
}

// MarkStaleWebACDPoolTargetsOffline sets stale web seats to offline for abnormal-disconnect fallback.
func MarkStaleWebACDPoolTargetsOffline(ctx context.Context, db *gorm.DB, now time.Time) (int64, error) {
	freshSince := now.Add(-constants.WebSeatStaleAfter)
	res := ActiveACDPoolTargets(db.WithContext(ctx)).
		Where("route_type = ?", constants.ACDPoolRouteTypeWeb).
		Where("work_state <> ?", constants.ACDWorkStateOffline).
		Where("web_seat_last_seen_at IS NULL OR web_seat_last_seen_at <= ?", freshSince).
		Updates(map[string]any{
			"work_state":    constants.ACDWorkStateOffline,
			"work_state_at": now,
			"updated_at":    now,
		})
	return res.RowsAffected, res.Error
}

// ──────────────────────────────────────────────────────────────────────────────
// ACD Shift Schedule helpers
// ──────────────────────────────────────────────────────────────────────────────

type acdShiftWindow struct {
	Weekdays []int  `json:"weekdays"`
	Start    string `json:"start"`
	End      string `json:"end"`
}

// ACDFitsShiftSchedule reports whether t falls into any configured window. Empty or invalid JSON means 24/7 (eligible).
func ACDFitsShiftSchedule(scheduleJSON string, t time.Time, loc *time.Location) bool {
	scheduleJSON = strings.TrimSpace(scheduleJSON)
	if scheduleJSON == "" {
		return true
	}
	var windows []acdShiftWindow
	if err := json.Unmarshal([]byte(scheduleJSON), &windows); err != nil || len(windows) == 0 {
		return true
	}
	if loc == nil {
		loc = time.Local
	}
	tt := t.In(loc)
	wd := int(tt.Weekday())
	minutes := tt.Hour()*60 + tt.Minute()

	for _, w := range windows {
		if !acdWeekdayListed(w.Weekdays, wd) {
			continue
		}
		startM, endM, ok := acdParseHHMMRange(w.Start, w.End)
		if !ok {
			continue
		}
		if startM <= endM {
			if minutes >= startM && minutes < endM {
				return true
			}
		} else {
			if minutes >= startM || minutes < endM {
				return true
			}
		}
	}
	return false
}

func acdWeekdayListed(days []int, wd int) bool {
	if len(days) == 0 {
		return true
	}
	for _, d := range days {
		if d == wd {
			return true
		}
	}
	return false
}

func acdParseHHMMRange(start, end string) (startMin, endMin int, ok bool) {
	a, ok1 := acdParseHHMM(strings.TrimSpace(start))
	b, ok2 := acdParseHHMM(strings.TrimSpace(end))
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return a, b, true
}

func acdParseHHMM(s string) (minutes int, ok bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// ACDShiftTimeLocation loads IANA zone from ACD_SHIFT_TIMEZONE or falls back to Local.
func ACDShiftTimeLocation() *time.Location {
	name := strings.TrimSpace(utils.GetEnv("ACD_SHIFT_TIMEZONE"))
	if name == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Local
	}
	return loc
}
