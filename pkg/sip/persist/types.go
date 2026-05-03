package persist

import (
	"context"
	"path/filepath"
	"strings"
)

// Soft-delete flags (legacy BaseModel convention).
const (
	SoftDeleteStatusActive  int8 = 0
	SoftDeleteStatusDeleted int8 = 1
)

// SIPCallStore persists SIP call rows (GORM or JSON backend).
type SIPCallStore interface {
	CreateSIPCall(ctx context.Context, c *SIPCall) error
	UpdateSIPCall(ctx context.Context, c *SIPCall) error
}

// SIPUserStore persists registrar / presence rows.
type SIPUserStore interface {
	GetUser(ctx context.Context, username, domain string) (*SIPUser, error)
	UpsertUser(ctx context.Context, u *SIPUser) error
}

// Stores groups optional backends for standalone SIP tooling.
type Stores struct {
	Calls SIPCallStore
	Users SIPUserStore
}

func DefaultNopStores() Stores {
	n := Nop{}
	return Stores{Calls: n, Users: n}
}

type Nop struct{}

func (Nop) CreateSIPCall(context.Context, *SIPCall) error { return nil }
func (Nop) UpdateSIPCall(context.Context, *SIPCall) error { return nil }
func (Nop) GetUser(context.Context, string, string) (*SIPUser, error) {
	return nil, nil
}
func (Nop) UpsertUser(context.Context, *SIPUser) error { return nil }

// RecordingRelPathForCall returns a stable relative WAV path under data/recordings/.
func RecordingRelPathForCall(callID string) string {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return ""
	}
	name := sanitizeFilePart(callID) + ".wav"
	return filepath.ToSlash(filepath.Join("data", "recordings", name))
}

// RecordingPathForCall joins baseDir with the same filename as RecordingRelPathForCall.
func RecordingPathForCall(baseDir, callID string) string {
	baseDir = strings.TrimSpace(baseDir)
	callID = strings.TrimSpace(callID)
	if baseDir == "" || callID == "" {
		return ""
	}
	rel := RecordingRelPathForCall(callID)
	if rel == "" {
		return ""
	}
	_, file := filepath.Split(rel)
	return filepath.Join(baseDir, file)
}

func sanitizeFilePart(s string) string {
	const repl = '_'
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '.':
		default:
			b[i] = repl
		}
	}
	out := strings.Trim(string(b), "._")
	if out == "" {
		return "unknown"
	}
	return out
}
