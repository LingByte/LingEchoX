package persist

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/LinByte/VoiceServer/pkg/constants"
	"github.com/LinByte/VoiceServer/pkg/sip/stack"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SIPUser is registrar-facing registration / online state (sip_users).
type SIPUser struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime;comment:Creation time"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" gorm:"autoUpdateTime;comment:Update time"`
	IsDeleted int8      `json:"isDeleted,omitempty" gorm:"default:0;index;comment:Soft delete flag (0:not deleted, 1:deleted)"`
	CreateBy  string    `json:"createBy,omitempty" gorm:"size:128;index;comment:Creator"`
	UpdateBy  string    `json:"updateBy,omitempty" gorm:"size:128;index;comment:Updater"`

	Username   string     `json:"username" gorm:"size:128;not null;uniqueIndex:idx_sip_user_aor"`
	Domain     string     `json:"domain" gorm:"size:128;not null;uniqueIndex:idx_sip_user_aor"`
	ContactURI string     `json:"contactUri" gorm:"size:512"`
	RemoteIP   string     `json:"remoteIp" gorm:"size:64;index"`
	RemotePort int        `json:"remotePort" gorm:"index"`
	Online     bool       `json:"online" gorm:"default:false;index"`
	ExpiresAt  *time.Time `json:"expiresAt" gorm:"index"`
	LastSeenAt *time.Time `json:"lastSeenAt" gorm:"index"`
	UserAgent  string     `json:"userAgent" gorm:"size:256"`
	Via        string     `json:"via" gorm:"type:text"`
}

func (SIPUser) TableName() string { return constants.SIP_USER_TABLE_NAME }

func (s *SIPUser) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	if s.IsDeleted == 0 {
		s.IsDeleted = SoftDeleteStatusActive
	}
	return nil
}

func (s *SIPUser) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = time.Now()
	return nil
}

func ActiveSIPUsers(db *gorm.DB) *gorm.DB {
	return db.Model(&SIPUser{}).Where("is_deleted = ?", SoftDeleteStatusActive)
}

func OnlineSIPUsers(db *gorm.DB, now time.Time) *gorm.DB {
	return ActiveSIPUsers(db).
		Where("online = ?", true).
		Where("expires_at IS NULL OR expires_at > ?", now)
}

func ListSIPUsersPage(db *gorm.DB, page, size int) ([]SIPUser, int64, error) {
	var total int64
	q := ActiveSIPUsers(db)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	var list []SIPUser
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func GetActiveSIPUserByID(db *gorm.DB, id uint) (SIPUser, error) {
	var row SIPUser
	err := ActiveSIPUsers(db).Where("id = ?", id).First(&row).Error
	return row, err
}

func SoftDeleteSIPUserByID(db *gorm.DB, id uint) (int64, error) {
	res := db.Model(&SIPUser{}).Where("id = ?", id).Updates(map[string]any{
		"is_deleted": SoftDeleteStatusDeleted,
	})
	return res.RowsAffected, res.Error
}

func CountOnlineSIPUsersByUsername(db *gorm.DB, username string) (int64, error) {
	var n int64
	err := OnlineSIPUsers(db, time.Now()).
		Where("username = ?", strings.TrimSpace(username)).
		Count(&n).Error
	return n, err
}

func FindOnlineSIPUserByAOR(ctx context.Context, db *gorm.DB, username, domain string) (SIPUser, error) {
	q := OnlineSIPUsers(db.WithContext(ctx), time.Now()).
		Where("username = ?", strings.TrimSpace(username))
	if d := strings.TrimSpace(domain); d != "" {
		q = q.Where("domain = ?", d)
	}
	var row SIPUser
	err := q.First(&row).Error
	return row, err
}

func FindLatestOnlineSIPUserByUsername(ctx context.Context, db *gorm.DB, username string) (SIPUser, error) {
	var row SIPUser
	err := OnlineSIPUsers(db.WithContext(ctx), time.Now()).
		Where("username = ?", strings.TrimSpace(username)).
		Order("last_seen_at DESC").
		First(&row).Error
	return row, err
}

func UpsertSIPUserRegister(ctx context.Context, db *gorm.DB, user SIPUser) error {
	now := time.Now()
	user.Username = strings.TrimSpace(user.Username)
	user.Domain = strings.TrimSpace(user.Domain)
	if user.Username == "" || user.Domain == "" {
		return nil
	}
	if user.LastSeenAt == nil {
		user.LastSeenAt = &now
	}
	user.IsDeleted = SoftDeleteStatusActive
	updates := map[string]any{
		"contact_uri":  user.ContactURI,
		"remote_ip":    user.RemoteIP,
		"remote_port":  user.RemotePort,
		"user_agent":   user.UserAgent,
		"via":          user.Via,
		"online":       user.Online,
		"expires_at":   user.ExpiresAt,
		"last_seen_at": user.LastSeenAt,
		"is_deleted":   SoftDeleteStatusActive,
		"updated_at":   now,
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "username"},
			{Name: "domain"},
		},
		DoUpdates: clause.Assignments(updates),
	}).Create(&user).Error
}

func MarkSIPUserOffline(ctx context.Context, db *gorm.DB, username, domain string) error {
	return ActiveSIPUsers(db.WithContext(ctx)).
		Where("username = ? AND domain = ?", strings.TrimSpace(username), strings.TrimSpace(domain)).
		Updates(map[string]any{
			"online":       false,
			"expires_at":   nil,
			"last_seen_at": time.Now(),
		}).Error
}

func MarkExpiredSIPUsersOffline(ctx context.Context, db *gorm.DB, now time.Time) (int64, error) {
	res := ActiveSIPUsers(db.WithContext(ctx)).
		Where("online = ?", true).
		Where("expires_at IS NOT NULL AND expires_at <= ?", now).
		Updates(map[string]any{
			"online":       false,
			"last_seen_at": now,
		})
	return res.RowsAffected, res.Error
}

// SIPUserFromRegister fills SIPUser from REGISTER and peer.
func SIPUserFromRegister(req *stack.Message, peer *net.UDPAddr) *SIPUser {
	if req == nil {
		return nil
	}
	user, domain := sipUserDomain(req.RequestURI)
	now := time.Now()
	sec := parseRegisterExpires(req)
	var exp *time.Time
	online := false
	if sec > 0 {
		t := now.Add(time.Duration(sec) * time.Second)
		exp = &t
		online = true
	}
	u := &SIPUser{
		Username:   user,
		Domain:     domain,
		ContactURI: firstContactValue(req),
		UserAgent:  req.GetHeader("user-agent"),
		Via:        joinHeaderValues(req.GetHeaders("via")),
		LastSeenAt: &now,
		Online:     online,
		ExpiresAt:  exp,
	}
	if peer != nil {
		if ip := peer.IP.String(); ip != "" {
			u.RemoteIP = ip
		}
		u.RemotePort = peer.Port
	}
	return u
}

// ParseRegistrationAOR returns username and host from a sip:/sips: Request-URI.
func ParseRegistrationAOR(requestURI string) (username, domain string) {
	return sipUserDomain(requestURI)
}

func sipUserDomain(requestURI string) (user, dom string) {
	requestURI = strings.TrimSpace(requestURI)
	low := strings.ToLower(requestURI)
	if !strings.HasPrefix(low, "sip:") && !strings.HasPrefix(low, "sips:") {
		return "", ""
	}
	colon := strings.Index(requestURI, ":")
	if colon < 0 {
		return "", ""
	}
	rest := requestURI[colon+1:]
	if i := strings.Index(rest, ";"); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.Index(rest, "@"); i >= 0 {
		return rest[:i], rest[i+1:]
	}
	return rest, ""
}

func firstContactValue(req *stack.Message) string {
	vals := req.GetHeaders("contact")
	if len(vals) == 0 {
		return ""
	}
	return strings.TrimSpace(vals[0])
}

func joinHeaderValues(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return strings.Join(vals, " | ")
}

// RegisterExpiresSeconds returns REGISTER lifetime from Expires header or Contact ;expires=.
func RegisterExpiresSeconds(req *stack.Message) int {
	return parseRegisterExpires(req)
}

func parseRegisterExpires(req *stack.Message) int {
	if v := strings.TrimSpace(req.GetHeader("expires")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	c := firstContactValue(req)
	low := strings.ToLower(c)
	idx := strings.Index(low, ";expires=")
	if idx < 0 {
		return 0
	}
	rest := c[idx+len(";expires="):]
	rest = strings.TrimSpace(rest)
	for i, r := range rest {
		if r == ';' || r == ' ' || r == '\t' {
			rest = rest[:i]
			break
		}
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func sanitizeWebSeatUsername(operatorKey string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(operatorKey)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		case r == '@':
			b.WriteRune('_')
		}
	}
	s := strings.Trim(b.String(), "._-")
	if s == "" {
		return "webseat"
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return "ws-" + s
}

// UpsertWebSeatSIPPresence ensures a sip_users row for a WebSeat operator (not SIP REGISTER).
func UpsertWebSeatSIPPresence(db *gorm.DB, operatorKey, seatDisplayName string) error {
	if db == nil {
		return nil
	}
	op := strings.TrimSpace(operatorKey)
	if op == "" {
		return nil
	}
	user := sanitizeWebSeatUsername(op)
	dom := utils.GetEnv("CONVERSATION_WEBSEAT_SIP_DOMAIN")
	if dom == "" {
		dom = "webseat"
	}
	now := time.Now()
	exp := now.Add(365 * 24 * time.Hour)
	ua := "LingEchoX-WebSeat"
	if t := strings.TrimSpace(seatDisplayName); t != "" {
		if len(t) > 480 {
			t = t[:480]
		}
		ua = "WebSeat/" + t
	}
	contact := fmt.Sprintf("<sip:%s@%s>", user, dom)

	var row SIPUser
	err := db.Where("username = ? AND domain = ?", user, dom).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = SIPUser{
			Username:   user,
			Domain:     dom,
			ContactURI: contact,
			Online:     true,
			ExpiresAt:  &exp,
			LastSeenAt: &now,
			UserAgent:  ua,
		}
		return db.Create(&row).Error
	}
	if err != nil {
		return err
	}
	return db.Model(&row).Updates(map[string]any{
		"online":       true,
		"expires_at":   exp,
		"last_seen_at": now,
		"contact_uri":  contact,
		"user_agent":   ua,
		"updated_at":   now,
	}).Error
}
