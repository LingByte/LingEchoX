package models

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/LinByte/VoiceServer/internal/constants"
	"gorm.io/gorm"
)

// TrunkTransferConfig 描述了从 Trunk + TrunkNumber 推导出来的转呼/呼出网关与呼叫者身份。
type TrunkTransferConfig struct {
	Host          string // 网关 host，如 "183.213.19.195"
	Port          int    // 网关端口，如 50400
	CallerUser    string // 主叫号码，从 TrunkNumber.Number 取
	CallerDisplay string // 主叫显示名，从 TrunkNumber.CallerDisplayName 取
	TrunkID       uint
	TrunkNumberID uint
}

// SignalingAddr 返回 host:port，便于直接喂给 outbound.DialTarget.SignalingAddr。
func (c TrunkTransferConfig) SignalingAddr() string {
	if c.Host == "" || c.Port <= 0 {
		return ""
	}
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// TrunkPickRole 控制 PickTrunkConfig 选号时的偏好。
type TrunkPickRole int

const (
	// TrunkRoleTransfer：转人工。优先选择 is_transfer_relay = true 的号码，
	// 其次接受 direction 在 {空, outbound, both, all} 的号码。
	TrunkRoleTransfer TrunkPickRole = iota
	// TrunkRoleOutbound：外呼。仅接受 direction 在 {outbound, both, all} 的号码
	// （direction 为 inbound 的号码不允许用于外呼，与 UI 配置语义保持一致：
	// "外呼是否开启取决于号码 direction"）。
	TrunkRoleOutbound
)

// PickTrunkConfig 从数据库挑选一条用于「转人工 / 外呼」的 Trunk + TrunkNumber。
//
// 选择规则（按优先级）：
//  1. tenantID > 0 时：优先选 TrunkNumber.tenant_id = tenantID 的号码，并跟随它的 Trunk；
//     找不到时回退到 tenant_id = 0（平台号池待分配）的号码。
//  2. tenantID == 0 时：选 tenant_id = 0 的号码。
//  3. 号码必须满足 role 对 direction 的限制；其 Trunk 必须有合法的 local_addr。
//
// 解析 Trunk.LocalAddr 时，支持以下形式：
//
//	"183.213.19.195:50400"
//	"sip:183.213.19.195:50400"
//	"183.213.19.195"           （没有端口时，默认 50400）
//
// 任意一步失败都会返回 ok=false，调用方应直接报错。
func PickTrunkConfig(db *gorm.DB, tenantID uint, role TrunkPickRole) (TrunkTransferConfig, bool) {
	if db == nil {
		return TrunkTransferConfig{}, false
	}

	tenantsToTry := []uint{}
	if tenantID > 0 {
		tenantsToTry = append(tenantsToTry, tenantID, 0)
	} else {
		tenantsToTry = append(tenantsToTry, 0)
	}

	for _, tid := range tenantsToTry {
		number, ok := pickTrunkNumberForTenant(db, tid, role)
		if !ok {
			continue
		}
		var trunk Trunk
		if err := db.First(&trunk, number.TrunkID).Error; err != nil {
			continue
		}
		host, port, ok := parseTrunkLocalAddr(trunk.LocalAddr)
		if !ok {
			continue
		}
		return TrunkTransferConfig{
			Host:          host,
			Port:          port,
			CallerUser:    strings.TrimSpace(number.Number),
			CallerDisplay: strings.TrimSpace(number.CallerDisplayName),
			TrunkID:       trunk.ID,
			TrunkNumberID: number.ID,
		}, true
	}
	return TrunkTransferConfig{}, false
}

// PickTrunkTransferConfig 是 PickTrunkConfig(_, _, TrunkRoleTransfer) 的快捷方式。
func PickTrunkTransferConfig(db *gorm.DB, tenantID uint) (TrunkTransferConfig, bool) {
	return PickTrunkConfig(db, tenantID, TrunkRoleTransfer)
}

// PickTrunkOutboundConfig 是 PickTrunkConfig(_, _, TrunkRoleOutbound) 的快捷方式。
// 用于外呼场景（campaign worker / 启动日志）；网关来自中继 LocalAddr。
func PickTrunkOutboundConfig(db *gorm.DB, tenantID uint) (TrunkTransferConfig, bool) {
	return PickTrunkConfig(db, tenantID, TrunkRoleOutbound)
}

// ResolveACDOutboundFromTrunkNumber 把一条已分配给租户的 TrunkNumber 展开成 ACD 行需要的
// 出局网关 + 主叫字段。号码必须显式可外呼（direction ∈ {outbound, both, all}）。
// 用于「转接/外呼时用另一个号码」：呼入号码只支持 inbound 时仍可独立绑定可外呼号码。
func ResolveACDOutboundFromTrunkNumber(db *gorm.DB, tenantID, trunkNumberID uint) (TrunkTransferConfig, bool) {
	if db == nil || tenantID == 0 || trunkNumberID == 0 {
		return TrunkTransferConfig{}, false
	}
	var n TrunkNumber
	if err := db.Where("id = ? AND tenant_id = ?", trunkNumberID, tenantID).First(&n).Error; err != nil {
		return TrunkTransferConfig{}, false
	}
	dir := strings.ToLower(strings.TrimSpace(n.Direction))
	if dir != "outbound" && dir != "both" && dir != "all" {
		return TrunkTransferConfig{}, false
	}
	var trunk Trunk
	if err := db.First(&trunk, n.TrunkID).Error; err != nil {
		return TrunkTransferConfig{}, false
	}
	host, port, ok := parseTrunkLocalAddr(trunk.LocalAddr)
	if !ok {
		return TrunkTransferConfig{}, false
	}
	return TrunkTransferConfig{
		Host:          host,
		Port:          port,
		CallerUser:    strings.TrimSpace(n.Number),
		CallerDisplay: strings.TrimSpace(n.CallerDisplayName),
		TrunkID:       trunk.ID,
		TrunkNumberID: n.ID,
	}, true
}

// PickTrunkOutboundConfigByCaller selects trunk gateway by an explicitly specified caller number.
// The matched number must belong to tenantID and be outbound-enabled (direction in outbound/both/all).
func PickTrunkOutboundConfigByCaller(db *gorm.DB, tenantID uint, callerRaw string) (TrunkTransferConfig, bool) {
	if db == nil || tenantID == 0 {
		return TrunkTransferConfig{}, false
	}
	row, ok := FindTrunkNumberForOutboundCaller(db, tenantID, callerRaw)
	if !ok || row.ID == 0 {
		return TrunkTransferConfig{}, false
	}
	dir := strings.ToLower(strings.TrimSpace(row.Direction))
	if dir != "outbound" && dir != "both" && dir != "all" {
		return TrunkTransferConfig{}, false
	}
	var trunk Trunk
	if err := db.First(&trunk, row.TrunkID).Error; err != nil {
		return TrunkTransferConfig{}, false
	}
	host, port, ok := parseTrunkLocalAddr(trunk.LocalAddr)
	if !ok {
		return TrunkTransferConfig{}, false
	}
	return TrunkTransferConfig{
		Host:          host,
		Port:          port,
		CallerUser:    strings.TrimSpace(row.Number),
		CallerDisplay: strings.TrimSpace(row.CallerDisplayName),
		TrunkID:       trunk.ID,
		TrunkNumberID: row.ID,
	}, true
}

// pickTrunkNumberForTenant 在「号码所属租户 = tenantID」的范围内，按 role 选一条号码。
// 同时通过 INNER JOIN trunks 排除 trunk 已软删 / local_addr 为空的行。
func pickTrunkNumberForTenant(db *gorm.DB, tenantID uint, role TrunkPickRole) (TrunkNumber, bool) {
	if db == nil {
		return TrunkNumber{}, false
	}
	tn := TrunkNumber{}.TableName()
	tr := Trunk{}.TableName()
	base := db.Model(&TrunkNumber{}).
		Joins("INNER JOIN "+tr+" AS tr ON tr.id = "+tn+".trunk_id AND tr.deleted_at IS NULL AND tr.local_addr <> ''").
		Where(tn+".tenant_id = ?", tenantID).
		Where(tn + ".`number` <> ''").
		Select(tn + ".*")

	outboundDirections := []string{"outbound", "both", "all"}

	switch role {
	case TrunkRoleOutbound:
		// 严格：必须显式声明可外呼（direction ∈ {outbound, both, all}）。
		var n TrunkNumber
		if err := base.Where(tn+".direction IN ?", outboundDirections).
			Order(tn + ".id ASC").First(&n).Error; err == nil {
			return n, true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return TrunkNumber{}, false
		}
		return TrunkNumber{}, false

	default: // TrunkRoleTransfer
		// 优先转人工中继号
		var n TrunkNumber
		if err := base.Where(tn+".is_transfer_relay = ?", true).
			Order(tn + ".id ASC").First(&n).Error; err == nil {
			return n, true
		}
		// 其次 direction 为空或外呼/both
		if err := base.Where(tn+".direction IN ?", append([]string{""}, outboundDirections...)).
			Order(tn + ".id ASC").First(&n).Error; err == nil {
			return n, true
		}
		// 再次：任意非空号码
		if err := base.Order(tn + ".id ASC").First(&n).Error; err == nil {
			return n, true
		}
		return TrunkNumber{}, false
	}
}

// parseTrunkLocalAddr 把 Trunk.LocalAddr 解析成 (host, port)。
// 端口缺省时返回 50400（与历史 SIP_TRANSFER_PORT 默认值一致）。
func parseTrunkLocalAddr(raw string) (host string, port int, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", 0, false
	}
	// 去掉 sip: / sips: 前缀（trunk 配置允许使用 URI 形式）。
	for _, prefix := range []string{"sip:", "sips:", "SIP:", "SIPS:"} {
		if strings.HasPrefix(s, prefix) {
			s = s[len(prefix):]
			break
		}
	}
	// 去掉路径 / 查询串（比如 "host:port;param=foo"）。
	if i := strings.IndexAny(s, ";?/"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "", 0, false
	}
	if h, p, err := net.SplitHostPort(s); err == nil {
		if pi, _ := strconv.Atoi(p); pi > 0 && pi < 65536 {
			return strings.TrimSpace(h), pi, true
		}
	}
	// 没有端口则按 50400 默认。
	return strings.TrimSpace(s), constants.TrunkDefaultSIPPort, true
}
