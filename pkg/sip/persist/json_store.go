package persist

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type callsDisk struct {
	Version int                   `json:"version"`
	Calls   map[string]*SIPCall `json:"calls"`
}

type usersDisk struct {
	Version int                   `json:"version"`
	Users   map[string]*SIPUser `json:"users"`
}

type jsonStores struct {
	dir      string
	callPath string
	userPath string

	callMu sync.Mutex
	userMu sync.Mutex
}

// NewJSONStores persists SIP calls and registrants as JSON files under dir/sip/.
func NewJSONStores(dir string) (Stores, error) {
	dir = filepath.Clean(dir)
	sipDir := filepath.Join(dir, "sip")
	if err := os.MkdirAll(sipDir, 0o755); err != nil {
		return Stores{}, err
	}
	j := &jsonStores{
		dir:      dir,
		callPath: filepath.Join(sipDir, "calls.json"),
		userPath: filepath.Join(sipDir, "users.json"),
	}
	return Stores{Calls: &jsonCallStore{j: j}, Users: &jsonUserStore{j: j}}, nil
}

func readCalls(path string) (callsDisk, error) {
	var d callsDisk
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			d.Version = 1
			d.Calls = make(map[string]*SIPCall)
			return d, nil
		}
		return d, err
	}
	if len(b) == 0 {
		d.Version = 1
		d.Calls = make(map[string]*SIPCall)
		return d, nil
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return d, err
	}
	if d.Calls == nil {
		d.Calls = make(map[string]*SIPCall)
	}
	if d.Version == 0 {
		d.Version = 1
	}
	return d, nil
}

func writeCalls(path string, d callsDisk) error {
	if d.Calls == nil {
		d.Calls = make(map[string]*SIPCall)
	}
	d.Version = 1
	b, err := json.MarshalIndent(&d, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type jsonCallStore struct{ j *jsonStores }

func (s *jsonCallStore) CreateSIPCall(ctx context.Context, c *SIPCall) error {
	_ = ctx
	if c == nil || c.CallID == "" {
		return errors.New("persist: empty call")
	}
	s.j.callMu.Lock()
	defer s.j.callMu.Unlock()

	d, err := readCalls(s.j.callPath)
	if err != nil {
		return err
	}
	now := time.Now()
	if dst, dup := d.Calls[c.CallID]; dup && dst != nil {
		MergeSIPCall(dst, c)
		dst.UpdatedAt = now
		return writeCalls(s.j.callPath, d)
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	cp := *c
	d.Calls[c.CallID] = &cp
	return writeCalls(s.j.callPath, d)
}

func (s *jsonCallStore) UpdateSIPCall(ctx context.Context, patch *SIPCall) error {
	_ = ctx
	if patch == nil || patch.CallID == "" {
		return errors.New("persist: empty call_id")
	}
	s.j.callMu.Lock()
	defer s.j.callMu.Unlock()

	d, err := readCalls(s.j.callPath)
	if err != nil {
		return err
	}
	dst, ok := d.Calls[patch.CallID]
	if !ok {
		now := time.Now()
		dst = &SIPCall{CallID: patch.CallID, CreatedAt: now}
		d.Calls[patch.CallID] = dst
	}
	MergeSIPCall(dst, patch)
	return writeCalls(s.j.callPath, d)
}

func readUsers(path string) (usersDisk, error) {
	var d usersDisk
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			d.Version = 1
			d.Users = make(map[string]*SIPUser)
			return d, nil
		}
		return d, err
	}
	if len(b) == 0 {
		d.Version = 1
		d.Users = make(map[string]*SIPUser)
		return d, nil
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return d, err
	}
	if d.Users == nil {
		d.Users = make(map[string]*SIPUser)
	}
	if d.Version == 0 {
		d.Version = 1
	}
	return d, nil
}

func writeUsers(path string, d usersDisk) error {
	if d.Users == nil {
		d.Users = make(map[string]*SIPUser)
	}
	d.Version = 1
	b, err := json.MarshalIndent(&d, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type jsonUserStore struct{ j *jsonStores }

func sipUserKey(username, domain string) string {
	return username + "@" + domain
}

func (s *jsonUserStore) GetUser(ctx context.Context, username, domain string) (*SIPUser, error) {
	_ = ctx
	s.j.userMu.Lock()
	defer s.j.userMu.Unlock()

	d, err := readUsers(s.j.userPath)
	if err != nil {
		return nil, err
	}
	u := d.Users[sipUserKey(username, domain)]
	if u == nil {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (s *jsonUserStore) UpsertUser(ctx context.Context, u *SIPUser) error {
	_ = ctx
	if u == nil || u.Username == "" {
		return nil
	}
	s.j.userMu.Lock()
	defer s.j.userMu.Unlock()

	d, err := readUsers(s.j.userPath)
	if err != nil {
		return err
	}
	key := sipUserKey(u.Username, u.Domain)
	now := time.Now()
	if existing := d.Users[key]; existing != nil {
		u.ID = existing.ID
		if u.CreatedAt.IsZero() {
			u.CreatedAt = existing.CreatedAt
		}
	} else if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	cp := *u
	d.Users[key] = &cp
	return writeUsers(s.j.userPath, d)
}
