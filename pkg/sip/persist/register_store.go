package persist

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/sip/outbound"
	"github.com/LinByte/VoiceServer/pkg/utils"

	"gorm.io/gorm"
)

// GormStore implements server.SIPRegisterStore using sip_users.
type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) SaveRegister(ctx context.Context, user, domain, contactURI string, sig *net.UDPAddr, expiresAt time.Time, userAgent string) error {
	if s == nil || s.db == nil || sig == nil {
		return nil
	}
	user = strings.TrimSpace(user)
	domain = strings.TrimSpace(domain)
	if user == "" || domain == "" {
		return nil
	}
	now := time.Now()
	exp := expiresAt
	return UpsertSIPUserRegister(ctx, s.db, SIPUser{
		Username:   user,
		Domain:     domain,
		ContactURI: contactURI,
		RemoteIP:   sig.IP.String(),
		RemotePort: sig.Port,
		UserAgent:  userAgent,
		Online:     true,
		ExpiresAt:  &exp,
		LastSeenAt: &now,
	})
}

func (s *GormStore) DeleteRegister(ctx context.Context, user, domain string) error {
	if s == nil || s.db == nil {
		return nil
	}
	user = strings.TrimSpace(user)
	domain = strings.TrimSpace(domain)
	if user == "" || domain == "" {
		return nil
	}
	return MarkSIPUserOffline(ctx, s.db, user, domain)
}

func (s *GormStore) LookupRegister(ctx context.Context, user, domain string) (*net.UDPAddr, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, nil
	}
	user = strings.TrimSpace(user)
	domain = strings.TrimSpace(domain)
	if user == "" || domain == "" {
		return nil, false, nil
	}
	row, err := FindOnlineSIPUserByAOR(ctx, s.db, user, domain)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if row.RemoteIP == "" || row.RemotePort <= 0 {
		return nil, false, nil
	}
	ip := net.ParseIP(row.RemoteIP)
	if ip == nil {
		return nil, false, nil
	}
	return &net.UDPAddr{IP: ip, Port: row.RemotePort}, true, nil
}

// DialTargetFromSIPUser builds Request-URI + signaling UDP target from an online sip_users row.
// Matches GormStore.DialTargetForUsername URI rules (SIP_DEFAULT_URI_PORT vs embedded listen port).
// Caller must ensure RemoteIP/RemotePort/freshness are valid.
func DialTargetFromSIPUser(row SIPUser) outbound.DialTarget {
	d := EffectiveDialDomain(row.Domain, row.RemoteIP)
	port := 6050
	if ps := utils.GetEnv(constants.EnvSIPDefaultURIPort); ps != "" {
		if p, err := strconv.Atoi(ps); err == nil && p > 0 && p < 65536 {
			port = p
		}
	} else {
		port = EffectiveRegisterDialRequestURIPort(port)
	}
	reqURI := fmt.Sprintf("sip:%s@%s:%d", row.Username, d, port)
	sig := net.JoinHostPort(row.RemoteIP, strconv.Itoa(row.RemotePort))
	return outbound.DialTarget{RequestURI: reqURI, SignalingAddr: sig}
}

// DialTargetForUsername returns outbound.DialTarget for a registered extension (username).
// SIP_DEFAULT_DOMAIN optionally restricts to one AOR when multiple domains exist.
func (s *GormStore) DialTargetForUsername(ctx context.Context, username string) (outbound.DialTarget, bool) {
	var zero outbound.DialTarget
	if s == nil || s.db == nil {
		return zero, false
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return zero, false
	}
	domain := utils.GetEnv(constants.EnvSIPDefaultDomain)
	row, err := FindOnlineSIPUserByAOR(ctx, s.db, username, domain)
	if err != nil {
		return zero, false
	}
	if row.RemoteIP == "" || row.RemotePort <= 0 {
		return zero, false
	}
	if !IsRegisterFresh(row.LastSeenAt) {
		return zero, false
	}
	return DialTargetFromSIPUser(row), true
}
