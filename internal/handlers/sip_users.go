package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/sip/persist"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/gin-gonic/gin"
)

// acdPoolTargetDisplayName 给通话记录里的"转接"列生成展示名称：
// 优先用管理员配置的 Name（人名/工号）→ 退而求其次用 TargetValue
// （SIP 用户名 / 拨号串）→ 最后兜底"坐席#<id>"。明确不使用通用的
// "WebSeat" / "SIP" 字样，因为运营要看到具体接听人是谁。
func acdPoolTargetDisplayName(r models.ACDPoolTarget) string {
	if v := strings.TrimSpace(r.Name); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.TargetValue); v != "" {
		return v
	}
	if r.ID > 0 {
		return fmt.Sprintf("坐席#%d", r.ID)
	}
	return ""
}

func (h *Handlers) listSIPUsers(c *gin.Context) {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	s, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	page, size := utils.NormalizePage(p, s, 100)
	list, total, err := persist.ListSIPUsersPage(h.db, page, size)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"list": list, "total": total, "page": page, "size": size})
}

func (h *Handlers) getSIPUser(c *gin.Context) {
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	row, err := persist.GetActiveSIPUserByID(h.db, id)
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) deleteSIPUser(c *gin.Context) {
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	rows, err := persist.SoftDeleteSIPUserByID(h.db, id)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if rows == 0 {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

// listSIPCalls 通话记录分页查询：
//   - 平台管理员：跨租户查看全部；可用 ?tenantId=N 过滤指定租户（0 / 缺省 = 全部，包括测试通话 tenant_id=0）。
//   - 租户用户：仅返回自身租户的通话记录。
//   - 入局话单的 tenant_id / inbound_trunk_number_id 由被叫 DID 在 sip_trunk_numbers 的解析结果写入（与限并发、号码池作用域一致）。
//     未匹配 DID 且未开启 SIP_INBOUND_ALLOW_UNKNOWN_DID 的呼叫不会落库为租户数据。
func (h *Handlers) listSIPCalls(c *gin.Context) {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	s, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	page, size := utils.NormalizePage(p, s, 100)

	var (
		list  []persist.SIPCall
		total int64
		err   error
	)

	if middleware.AuthPlatformAdminID(c) > 0 {
		var tenantFilter uint
		if s := strings.TrimSpace(c.Query("tenantId")); s != "" {
			if v, perr := strconv.ParseUint(s, 10, 32); perr == nil {
				tenantFilter = uint(v)
			}
		}
		list, total, err = persist.ListAllSIPCallsPage(h.db, tenantFilter, page, size, c.Query("callId"), c.Query("state"))
	} else {
		tid := middleware.CurrentTenantID(c)
		list, total, err = persist.ListSIPCallsPage(h.db, tid, page, size, c.Query("callId"), c.Query("state"))
	}
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	for i := range list {
		persist.EnrichSIPCallResponse(&list[i])
		persist.RedactSIPCallForAPI(&list[i])
	}
	// Attach transferTo label in batch (if transferred).
	{
		ids := make([]uint, 0, len(list))
		seen := map[uint]struct{}{}
		for i := range list {
			if list[i].TransferACDTargetID > 0 {
				if _, ok := seen[list[i].TransferACDTargetID]; !ok {
					seen[list[i].TransferACDTargetID] = struct{}{}
					ids = append(ids, list[i].TransferACDTargetID)
				}
			}
		}
		if len(ids) > 0 {
			var rows []models.ACDPoolTarget
			_ = h.db.Model(&models.ACDPoolTarget{}).Where("id IN ?", ids).Find(&rows).Error
			m := map[uint]string{}
			for _, r := range rows {
				m[r.ID] = acdPoolTargetDisplayName(r)
			}
			for i := range list {
				if list[i].TransferACDTargetID > 0 {
					if v := strings.TrimSpace(m[list[i].TransferACDTargetID]); v != "" {
						list[i].TransferTo = v
					}
				}
			}
		}
	}
	response.Success(c, "success", gin.H{"list": list, "total": total, "page": page, "size": size})
}

func (h *Handlers) getSIPCall(c *gin.Context) {
	id, idErr := utils.ParseID(c.Param("id"))
	if idErr != nil {
		response.Fail(c, "invalid id", nil)
		return
	}

	var (
		row persist.SIPCall
		err error
	)
	if middleware.AuthPlatformAdminID(c) > 0 {
		row, err = persist.GetActiveSIPCallByID(h.db, id)
	} else {
		tid := middleware.CurrentTenantID(c)
		row, err = persist.GetActiveSIPCallForTenant(h.db, id, tid)
	}
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	persist.EnrichSIPCallResponse(&row)
	persist.RedactSIPCallForAPI(&row)
	if row.TransferACDTargetID > 0 {
		var tgt models.ACDPoolTarget
		if err := h.db.Model(&models.ACDPoolTarget{}).Where("id = ?", row.TransferACDTargetID).First(&tgt).Error; err == nil {
			row.TransferTo = acdPoolTargetDisplayName(tgt)
		}
	}
	response.Success(c, "success", row)
}
