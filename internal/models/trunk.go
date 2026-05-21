package models

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Trunk struct {
	ID           uint           `json:"id" gorm:"primarykey"`
	TenantID     uint           `json:"tenantId,string" gorm:"index;not null;default:0"` // SaaS isolation
	CreatedAt    time.Time      `json:"createdAt" label:"创建时间"`
	UpdatedAt    time.Time      `json:"updatedAt" label:"更新时间"`
	DeletedAt    gorm.DeletedAt `json:"deletedAt" gorm:"index"`
	Name         string         `json:"name" gorm:"size:200" label:"线路名称"`
	Description  string         `json:"description,omitempty" label:"备注"`
	Prefix       string         `json:"prefix"`
	LocalAddr    string         `json:"local_addr" label:"网关地址"`
	Numbers      []TrunkNumber  `json:"numbers" gorm:"foreignKey:TrunkID" label:"号码"`
	NumberNames  []string       `json:"numberNames" gorm:"-" label:"号码名称"`
	ProviderCode string         `json:"providerCode" gorm:"column:provider_code;size:64;uniqueIndex:idx_trunk_provider_code" label:"供应商编码"`
	Provider     string         `json:"-" label:"供应商"`
}

type TrunkNumber struct {
	ID                    uint           `json:"id" gorm:"primarykey" label:"编号"`
	CreatedAt             time.Time      `json:"createdAt" label:"创建时间"`
	UpdatedAt             time.Time      `json:"updatedAt" label:"更新时间"`
	DeletedAt             gorm.DeletedAt `json:"deletedAt" gorm:"index"`
	TrunkID               uint           `json:"trunkId" label:"所属线路"`
	Trunk                 Trunk          `json:"-" label:"所属线路"`
	TenantID              uint           `json:"tenantId,string" gorm:"column:tenant_id;index;not null;default:0" label:"分配租户"`
	Number                string         `json:"number" gorm:"size:200" label:"号码"`
	CallerDisplayName     string         `json:"callerDisplayName" gorm:"column:caller_display_name;size:200" label:"主叫显示名"`
	Prefix                string         `json:"prefix" gorm:"size:200" label:"前缀"`
	Description           string         `json:"description,omitempty" label:"备注"`
	Direction             string         `json:"direction" label:"呼叫用途"`
	Status                string         `json:"status" gorm:"size:64;index" label:"状态"`
	Concurrent            uint           `json:"concurrent" label:"呼出并发数"`
	CallInConcurrent      uint           `json:"callInConcurrent" gorm:"column:call_in_concurrent;default:0" label:"呼入并发数"`
	IsTransferRelay       bool           `json:"isTransferRelay" gorm:"column:is_transfer_relay;default:0;comment:是否为转人工中继号码" label:"转人工中继号码"`
	EffectiveTime         *time.Time     `json:"effectiveTime" gorm:"column:effective_time;comment:生效时间" label:"生效时间"`
	ExpirationTime        *time.Time     `json:"expirationTime" gorm:"column:expiration_time;comment:失效时间" label:"失效时间"`
	ProviderCode          string         `json:"providerCode" gorm:"column:provider_code;size:64;uniqueIndex:idx_trunk_number_provider_code" label:"供应商编码"`
	Provider              string         `json:"-" label:"供应商"`
	VoiceDialogWSURL      string         `json:"voiceDialogWsUrl,omitempty" gorm:"column:voice_dialog_ws_url;size:512" label:"呼入语音对话WS"`
	WelcomeAudioURL       string         `json:"welcomeAudioUrl,omitempty" gorm:"column:welcome_audio_url;size:1024" label:"欢迎语音频URL"`
	TransferRingingURL    string         `json:"transferRingingUrl,omitempty" gorm:"column:transfer_ringing_url;size:1024" label:"转接回铃音频URL"`
	ACDDispatchMode       string         `json:"acdDispatchMode,omitempty" gorm:"column:acd_dispatch_mode;size:24;index;default:weight" label:"ACD 分配模式"`
	OutboundTrunkNumberID uint           `json:"outboundTrunkNumberId" gorm:"column:outbound_trunk_number_id;not null;default:0;index" label:"外呼号码"`
}

// BeforeCreate 后端自动分配供应商编码，前端无法覆盖（即便传入也会被丢弃）。
func (t *Trunk) BeforeCreate(_ *gorm.DB) error {
	t.ProviderCode = constants.ProviderCodePrefixTrunk + "_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	return nil
}

func (n *TrunkNumber) BeforeCreate(_ *gorm.DB) error {
	n.ProviderCode = constants.ProviderCodePrefixTrunkNumber + "_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	return nil
}

func (Trunk) TableName() string {
	return constants.SIPTrunkTableName
}

func (TrunkNumber) TableName() string {
	return constants.SIPTrunkNumberTableName
}

// ListTrunksPage lists non-deleted trunks with optional name filter.
func ListTrunksPage(db *gorm.DB, tenantID uint, page, size int, nameContains string) ([]Trunk, int64, error) {
	q := db.Model(&Trunk{})
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if name := strings.TrimSpace(nameContains); name != "" {
		q = q.Where("name LIKE ?", "%"+name+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	offset := (page - 1) * size
	var list []Trunk
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetTrunkByID loads one trunk (with numbers); ErrRecordNotFound if missing or soft-deleted.
func GetTrunkByID(db *gorm.DB, id uint) (Trunk, error) {
	var row Trunk
	err := db.Preload("Numbers", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("id ASC")
	}).First(&row, id).Error
	return row, err
}

// GetTrunkByIDBare loads trunk row without numbers.
func GetTrunkByIDBare(db *gorm.DB, id uint) (Trunk, error) {
	var row Trunk
	err := db.First(&row, id).Error
	return row, err
}

// GetTrunkByIDBareForTenant loads trunk row belonging to tenant.
func GetTrunkByIDBareForTenant(db *gorm.DB, id uint, tenantID uint) (Trunk, error) {
	var row Trunk
	err := db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&row).Error
	return row, err
}

// GetTrunkByIDForTenant loads trunk + numbers scoped to tenant.
func GetTrunkByIDForTenant(db *gorm.DB, id uint, tenantID uint) (Trunk, error) {
	var row Trunk
	err := db.Preload("Numbers", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("id ASC")
	}).Where("id = ? AND tenant_id = ?", id, tenantID).First(&row).Error
	return row, err
}

// SoftDeleteTrunkCascade soft-deletes a trunk and its numbers.
func SoftDeleteTrunkCascade(db *gorm.DB, id uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("trunk_id = ?", id).Delete(&TrunkNumber{}).Error; err != nil {
			return err
		}
		return tx.Delete(&Trunk{}, id).Error
	})
}

// ListTrunkNumbersPage lists numbers; trunkID 0 means all trunks; tenantID 0 lists across all tenants
// (platform-admin view). Use ListTrunkNumbersForTenant for the tenant-scoped variant.
func ListTrunkNumbersPage(db *gorm.DB, tenantID uint, trunkID uint, page, size int, numberContains string) ([]TrunkNumber, int64, error) {
	tn := TrunkNumber{}.TableName()
	tr := Trunk{}.TableName()
	q := db.Model(&TrunkNumber{}).
		Joins("INNER JOIN " + tr + " AS tr ON tr.id = " + tn + ".trunk_id AND tr.deleted_at IS NULL").
		Where("1 = 1")
	if tenantID > 0 {
		// 注意：这里筛选的是 TrunkNumber.tenant_id（号码已分配给的租户），不是 Trunk.tenant_id。
		q = q.Where(tn+".tenant_id = ?", tenantID)
	}
	if trunkID > 0 {
		q = q.Where(tn+".trunk_id = ?", trunkID)
	}
	if num := strings.TrimSpace(numberContains); num != "" {
		q = q.Where("`number` LIKE ?", "%"+num+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	offset := (page - 1) * size
	var list []TrunkNumber
	if err := q.Order(tn + ".id DESC").Offset(offset).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ListTrunkNumbersForTenant lists every号码 already 分配给 tenantID（含搜索 / 分页参数）。
func ListTrunkNumbersForTenant(db *gorm.DB, tenantID uint, page, size int, numberContains string) ([]TrunkNumber, int64, error) {
	if tenantID == 0 {
		return nil, 0, nil
	}
	return ListTrunkNumbersPage(db, tenantID, 0, page, size, numberContains)
}

// GetTrunkNumberByID loads one trunk number row.
func GetTrunkNumberByID(db *gorm.DB, id uint) (TrunkNumber, error) {
	var row TrunkNumber
	err := db.First(&row, id).Error
	return row, err
}

// GetTrunkNumberByIDForTenant loads a trunk number row only if it is allocated to tenantID (sip_trunk_numbers.tenant_id).
func GetTrunkNumberByIDForTenant(db *gorm.DB, id uint, tenantID uint) (TrunkNumber, error) {
	var row TrunkNumber
	err := db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&row).Error
	return row, err
}

// SoftDeleteTrunkNumberByID soft-deletes one number row.
func SoftDeleteTrunkNumberByID(db *gorm.DB, id uint) error {
	return db.Delete(&TrunkNumber{}, id).Error
}

// NormalizeDialDigits keeps decimal digits only and strips country code "86" for DID matching.
func NormalizeDialDigits(s string) string {
	out := dialDigitsOnly(s)
	// Strip country code "86" for external DID matching.
	// Use len>10 to also support +86 prefixed service numbers like 400-xxxx-xxx (10 digits local, 12 with 86).
	if strings.HasPrefix(out, "86") && len(out) > 10 {
		out = out[2:]
	}
	return out
}

func dialDigitsOnly(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isLikelyDialUser reports whether the called user is treated as an external DID candidate.
// Rule (strict):
// 1) "+<digits>" (international style), OR
// 2) "<digits>" only.
// Anything else (extensions/usernames like "alice", "1001a", "ext-100") is treated as internal and
// will NOT enter digit-normalized DID matching.
func isLikelyDialUser(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "+") {
		if len(raw) == 1 {
			return false
		}
		for _, r := range raw[1:] {
			if !unicode.IsDigit(r) {
				return false
			}
		}
		return true
	}
	for _, r := range raw {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(raw) > 0
}

// FindTrunkNumberByInboundDID finds an allocated tenant trunk number whose DID matches the SIP Request-URI / To user.
// Used on inbound INVITE to attach TenantID to call records and downstream routing.
func FindTrunkNumberByInboundDID(db *gorm.DB, calledRaw string) (TrunkNumber, bool) {
	if db == nil {
		return TrunkNumber{}, false
	}
	raw := strings.TrimSpace(calledRaw)
	if raw == "" {
		return TrunkNumber{}, false
	}
	rawDigits := dialDigitsOnly(raw)
	calledNo86 := ""
	if strings.HasPrefix(raw, "+86") && len(rawDigits) > 10 && strings.HasPrefix(rawDigits, "86") {
		calledNo86 = rawDigits[2:]
	}
	rawHasOnlyDigits := isLikelyDialUser(raw)
	var rows []TrunkNumber
	if err := db.Where("tenant_id > 0").Order("id ASC").Find(&rows).Error; err != nil {
		return TrunkNumber{}, false
	}
	for _, row := range rows {
		num := strings.TrimSpace(row.Number)
		if num == "" {
			continue
		}
		if num == raw {
			return row, true
		}
		rn := dialDigitsOnly(num)
		// Strict inbound DID matching:
		// 1) no suffix/substring fuzzy matching;
		// 2) only compare normalized digits when called user itself is a pure digit string.
		// This prevents extension-like users such as "test1" from being normalized to "1"
		// and accidentally matching trunk DID numbers ending with "1".
		if !rawHasOnlyDigits || rn == "" || rawDigits == "" {
			continue
		}
		if rn == rawDigits {
			return row, true
		}
		if calledNo86 != "" && rn == calledNo86 {
			return row, true
		}
	}
	return TrunkNumber{}, false
}

// DialMatchKeys builds candidate sip_calls.from_number / to_number keys for capacity checks
// (raw SIP user part vs trunk_number.number vs digit-normalized forms).
func DialMatchKeys(raw string, rowStoredNumber string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(raw)
	add(rowStoredNumber)
	add(NormalizeDialDigits(raw))
	add(NormalizeDialDigits(rowStoredNumber))
	return out
}

// FindTrunkNumberForOutboundCaller resolves a tenant's trunk number row used as outbound CLI (Caller-ID user).
func FindTrunkNumberForOutboundCaller(db *gorm.DB, tenantID uint, callerRaw string) (TrunkNumber, bool) {
	if db == nil || tenantID == 0 {
		return TrunkNumber{}, false
	}
	raw := strings.TrimSpace(callerRaw)
	if raw == "" {
		return TrunkNumber{}, false
	}
	norm := NormalizeDialDigits(raw)
	var rows []TrunkNumber
	if err := db.Where("tenant_id = ?", tenantID).Order("id ASC").Find(&rows).Error; err != nil {
		return TrunkNumber{}, false
	}
	for _, row := range rows {
		num := strings.TrimSpace(row.Number)
		if num == "" {
			continue
		}
		if num == raw {
			return row, true
		}
		rn := NormalizeDialDigits(num)
		if rn == "" || norm == "" {
			continue
		}
		if rn == norm || strings.HasSuffix(rn, norm) || strings.HasSuffix(norm, rn) {
			return row, true
		}
	}
	return TrunkNumber{}, false
}

// ValidateOutboundTrunkNumberBinding checks outbound trunk number binding for create/update.
func ValidateOutboundTrunkNumberBinding(db *gorm.DB, selfID, outboundID, tenantID uint) error {
	if outboundID == 0 {
		return nil
	}
	if tenantID == 0 {
		return fmt.Errorf("outboundTrunkNumberId 需先把本号码分配给某个租户")
	}
	if outboundID == selfID {
		return fmt.Errorf("outboundTrunkNumberId 不能指向号码自身")
	}
	var n TrunkNumber
	if err := db.Where("id = ? AND tenant_id = ?", outboundID, tenantID).First(&n).Error; err != nil {
		return fmt.Errorf("outboundTrunkNumberId 不属于同一租户或不存在")
	}
	dir := strings.ToLower(strings.TrimSpace(n.Direction))
	if dir != "outbound" && dir != "both" && dir != "all" {
		return fmt.Errorf("outboundTrunkNumberId 对应号码 direction 必须是 outbound/both/all")
	}
	return nil
}
