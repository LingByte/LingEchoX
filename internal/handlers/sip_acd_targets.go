package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/LingByte/SoulNexus/pkg/sip/persist"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"github.com/gin-gonic/gin"
)

type acdPoolTargetWriteReq struct {
	Name string `json:"name"`
	// TrunkNumberID 把这条坐席绑定到「某个具体的中继号码」（即被叫号码）。
	// 0 = 任意号码（全租户兜底）；>0 = 仅对该 TrunkNumber 生效。
	// 租户写入时后端会校验：该 TrunkNumber 必须 tenant_id = 当前租户。
	TrunkNumberID        uint   `json:"trunkNumberId"`
	RouteType            string `json:"routeType"`
	SipSource            string `json:"sipSource"` // internal | trunk (SIP only)
	TargetValue          string `json:"targetValue"`
	SipCallerID          string `json:"sipCallerId"`
	SipCallerDisplayName string `json:"sipCallerDisplayName"`
	Weight               int    `json:"weight"`
	WorkState            string `json:"workState"`
	// ShiftSchedule JSON: e.g. [{"weekdays":[1,2,3,4,5],"start":"09:00","end":"18:00"}] (weekdays 0=Sun .. 6=Sat). Empty = 24/7.
	ShiftSchedule string `json:"shiftSchedule"`
}

// validateTrunkNumberOwnedByTenant: 0 表示「不绑定具体号码」，跳过校验；
// >0 必须存在且 tenant_id = tid。
func (h *Handlers) validateTrunkNumberOwnedByTenant(c *gin.Context, trunkNumberID, tid uint) bool {
	if trunkNumberID == 0 {
		return true
	}
	num, err := models.GetTrunkNumberByIDForTenant(h.db, trunkNumberID, tid)
	if err != nil || num.ID == 0 {
		response.Fail(c, "trunkNumberId 不属于当前租户", nil)
		return false
	}
	return true
}

func acdOperator(c *gin.Context) string {
	if s := strings.TrimSpace(middleware.AuthEmail(c)); s != "" {
		return s
	}
	if uid := middleware.AuthUserID(c); uid > 0 {
		return strconv.FormatUint(uint64(uid), 10)
	}
	return "system"
}

// acdPoolTargetListItem adds live SIP registration hint for admin list (not stored in acd_pool_targets).
type acdPoolTargetListItem struct {
	models.ACDPoolTarget
	LiveLineOnline bool `json:"liveLineOnline"`
}

func (h *Handlers) listACDPoolTargets(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	s, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	page, size := utils.NormalizePage(p, s, 100)
	var trunkNumID uint
	if s := strings.TrimSpace(c.Query("trunkNumberId")); s != "" {
		if v, perr := strconv.ParseUint(s, 10, 32); perr == nil {
			trunkNumID = uint(v)
		}
	}
	list, total, err := models.ListACDPoolTargetsPage(h.db, tid, page, size, c.Query("routeType"), trunkNumID)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	out := make([]acdPoolTargetListItem, 0, len(list))
	for _, row := range list {
		item := acdPoolTargetListItem{ACDPoolTarget: row}
		if models.ACDSipInternalLiveLineEligible(row) {
			n, _ := persist.CountOnlineSIPUsersByUsername(h.db, row.TargetValue)
			item.LiveLineOnline = n > 0
		} else if row.RouteType == models.ACDPoolRouteTypeWeb {
			item.LiveLineOnline = models.WebSeatLastSeenFresh(row.WebSeatLastSeenAt)
		}
		out = append(out, item)
	}
	response.Success(c, "success", gin.H{"list": out, "total": total, "page": page, "size": size})
}

// getACDDispatchMode returns current dispatch mode for one trunkNumberId (tenant-owned).
func (h *Handlers) getACDDispatchMode(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	var trunkNumID uint
	if s := strings.TrimSpace(c.Query("trunkNumberId")); s != "" {
		if v, err := strconv.ParseUint(s, 10, 32); err == nil {
			trunkNumID = uint(v)
		}
	}
	if trunkNumID == 0 {
		response.Fail(c, "invalid trunkNumberId", nil)
		return
	}
	if !h.validateTrunkNumberOwnedByTenant(c, trunkNumID, tid) {
		return
	}
	num, err := models.GetTrunkNumberByIDForTenant(h.db, trunkNumID, tid)
	if err != nil || num.ID == 0 {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"trunkNumberId": trunkNumID, "acdDispatchMode": models.NormalizeACDDispatchMode(num.ACDDispatchMode)})
}

type acdDispatchModeReq struct {
	TrunkNumberID   uint   `json:"trunkNumberId"`
	ACDDispatchMode string `json:"acdDispatchMode"`
}

// updateACDDispatchMode updates sip_trunk_numbers.acd_dispatch_mode for the current tenant.
func (h *Handlers) updateACDDispatchMode(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	var req acdDispatchModeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TrunkNumberID == 0 {
		response.Fail(c, "invalid body: need trunkNumberId", nil)
		return
	}
	if !h.validateTrunkNumberOwnedByTenant(c, req.TrunkNumberID, tid) {
		return
	}
	mode := models.NormalizeACDDispatchMode(req.ACDDispatchMode)
	if err := h.db.Model(&models.TrunkNumber{}).
		Where("id = ? AND tenant_id = ?", req.TrunkNumberID, tid).
		Update("acd_dispatch_mode", mode).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"trunkNumberId": req.TrunkNumberID, "acdDispatchMode": mode})
}

func (h *Handlers) getACDPoolTarget(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	row, err := models.GetActiveACDPoolTargetByID(h.db, id)
	if err != nil || !tenantOwns(row.TenantID, tid) {
		response.Fail(c, "not found", nil)
		return
	}
	item := acdPoolTargetListItem{ACDPoolTarget: row}
	if models.ACDSipInternalLiveLineEligible(row) {
		n, _ := persist.CountOnlineSIPUsersByUsername(h.db, row.TargetValue)
		item.LiveLineOnline = n > 0
	} else if row.RouteType == models.ACDPoolRouteTypeWeb {
		item.LiveLineOnline = models.WebSeatLastSeenFresh(row.WebSeatLastSeenAt)
	}
	response.Success(c, "success", item)
}

func (h *Handlers) createACDPoolTarget(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	var req acdPoolTargetWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	rt, ok := models.ParseACDRouteType(req.RouteType)
	if !ok {
		response.Fail(c, "routeType must be sip or web", nil)
		return
	}
	if req.TrunkNumberID == 0 {
		response.Fail(c, "请选择中继号码后再绑定坐席", nil)
		return
	}
	if !h.validateTrunkNumberOwnedByTenant(c, req.TrunkNumberID, tid) {
		return
	}
	ws := models.NormalizeACDWorkState(req.WorkState)
	if ws != models.ACDWorkStateAvailable && ws != models.ACDWorkStateOffline && ws != models.ACDWorkStateBreak {
		response.Fail(c, "workState 仅允许 available/offline/break", nil)
		return
	}
	now := time.Now()
	sipSrc := ""
	if rt == models.ACDPoolRouteTypeSIP {
		// SIP ACD rows are now unified as outbound trunk-style targets.
		sipSrc = models.ACDSipSourceTrunk
	}
	var webSeen *time.Time
	if rt == models.ACDPoolRouteTypeWeb && ws == models.ACDWorkStateAvailable {
		webSeen = &now
	}
	row := models.NewACDPoolTargetForCreate(
		req.Name, rt, sipSrc, req.TargetValue,
		"", 0, "",
		req.SipCallerID, req.SipCallerDisplayName,
		req.Weight, ws, now, webSeen,
		req.ShiftSchedule,
		req.TrunkNumberID,
	)
	row.TenantID = tid
	op := acdOperator(c)
	if op != "" {
		row.SetCreateInfo(op)
	}
	// Web seat rows should not be duplicated for one operator.
	// Reuse one row and clean up older duplicates.
	if rt == models.ACDPoolRouteTypeWeb && op != "" {
		ctx := c.Request.Context()
		existing, err := models.ListActiveWebACDPoolTargetsByCreateBy(ctx, h.db, op, tid)
		if err != nil {
			response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
			return
		}
		if len(existing) > 0 {
			keep := existing[0]
			updates := models.BuildACDPoolTargetUpdateMap(
				keep, req.Name, rt, "", req.TargetValue,
				"", 0, "",
				"", "",
				req.Weight, ws, now, op,
				req.ShiftSchedule,
				req.TrunkNumberID,
			)
			if err := h.db.WithContext(ctx).Model(&models.ACDPoolTarget{}).Where("id = ?", keep.ID).Updates(updates).Error; err != nil {
				response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
				return
			}
			if ws == models.ACDWorkStateOffline {
				_ = models.ClearACDPoolTargetWebSeatLastSeen(h.db, keep.ID)
			}
			if len(existing) > 1 {
				dupIDs := make([]uint, 0, len(existing)-1)
				for i := 1; i < len(existing); i++ {
					dupIDs = append(dupIDs, existing[i].ID)
				}
				_, _ = models.SoftDeleteACDPoolTargetsByIDs(ctx, h.db, dupIDs, op)
			}
			updated, _ := models.ReloadACDPoolTargetByID(h.db, keep.ID)
			response.Success(c, "success", updated)
			return
		}
	}
	if err := h.db.Create(&row).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) updateACDPoolTarget(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	var req acdPoolTargetWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	rt, ok := models.ParseACDRouteType(req.RouteType)
	if !ok {
		response.Fail(c, "routeType must be sip or web", nil)
		return
	}
	if req.TrunkNumberID == 0 {
		response.Fail(c, "请选择中继号码后再绑定坐席", nil)
		return
	}
	if !h.validateTrunkNumberOwnedByTenant(c, req.TrunkNumberID, tid) {
		return
	}
	row, err := models.GetActiveACDPoolTargetByID(h.db, id)
	if err != nil || !tenantOwns(row.TenantID, tid) {
		response.Fail(c, "not found", nil)
		return
	}
	ws := models.NormalizeACDWorkState(req.WorkState)
	if ws != models.ACDWorkStateAvailable && ws != models.ACDWorkStateOffline && ws != models.ACDWorkStateBreak {
		response.Fail(c, "workState 仅允许 available/offline/break", nil)
		return
	}
	now := time.Now()
	sipSrc := ""
	if rt == models.ACDPoolRouteTypeSIP {
		// SIP ACD rows are now unified as outbound trunk-style targets.
		sipSrc = models.ACDSipSourceTrunk
	}
	op := acdOperator(c)
	updates := models.BuildACDPoolTargetUpdateMap(
		row, req.Name, rt, sipSrc, req.TargetValue,
		"", 0, "",
		req.SipCallerID, req.SipCallerDisplayName,
		req.Weight, ws, now, op,
		req.ShiftSchedule,
		req.TrunkNumberID,
	)
	if err := h.db.Model(&row).Updates(updates).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if rt == models.ACDPoolRouteTypeWeb && ws == models.ACDWorkStateOffline {
		_ = models.ClearACDPoolTargetWebSeatLastSeen(h.db, id)
	}
	row, _ = models.ReloadACDPoolTargetByID(h.db, id)
	response.Success(c, "success", row)
}

func (h *Handlers) deleteACDPoolTarget(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	n, err := models.SoftDeleteACDPoolTargetByIDForTenant(h.db, id, tid, acdOperator(c))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if n == 0 {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

type webSeatACDHeartbeatReq struct {
	TargetID uint `json:"targetId"`
}

// webSeatACDHeartbeat refreshes web_seat_last_seen_at for the anchored browser row (keepalive).
func (h *Handlers) webSeatACDHeartbeat(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	var req webSeatACDHeartbeatReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TargetID == 0 {
		response.Fail(c, "invalid body: need targetId", nil)
		return
	}
	op := acdOperator(c)
	if op == "" {
		response.Fail(c, "unauthorized", nil)
		return
	}
	row, err := models.GetActiveACDPoolTargetByID(h.db, req.TargetID)
	if err != nil || !tenantOwns(row.TenantID, tid) {
		response.Fail(c, "not found", nil)
		return
	}
	if row.RouteType != models.ACDPoolRouteTypeWeb {
		response.Fail(c, "not a web target", nil)
		return
	}
	if !models.WebSeatActorMayTouchRow(row, op) {
		response.Fail(c, "forbidden", nil)
		return
	}
	if err := models.UpdateACDPoolTargetWebSeatHeartbeat(h.db, req.TargetID, op, time.Now()); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"ok": true})
}
