package callanalysis

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"sync"
	"time"
)

// Store keeps recent export documents in memory (TTL).
type Store struct {
	mu   sync.RWMutex
	data map[string]entry
}

type entry struct {
	doc       *ExportDoc
	expiresAt time.Time
}

// NewStore creates an empty store.
func NewStore() *Store {
	return &Store{data: make(map[string]entry)}
}

// Put stores a copy keyed by doc.ID with default TTL 24h.
func (s *Store) Put(doc *ExportDoc) {
	if s == nil || doc == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[doc.ID] = entry{
		doc:       doc,
		expiresAt: time.Now().Add(24 * time.Hour),
	}
}

// Get returns the document if present and not expired.
func (s *Store) Get(id string) (*ExportDoc, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[id]
	if !ok || time.Now().After(e.expiresAt) {
		if ok {
			delete(s.data, id)
		}
		return nil, false
	}
	return e.doc, true
}
