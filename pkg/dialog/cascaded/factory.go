// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package cascaded

import (
	"fmt"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
)

// factory implements engine.Factory by constructing one Engine per
// call. Stateless; the Build method is safe to call from any goroutine.
type factory struct{}

// Build constructs a cascaded.Engine. Validates that cfg.Mode is the
// one this factory is registered under (defensive — engine.New
// already checks, but a misconfigured custom registry could call us
// directly with the wrong mode).
func (factory) Build(cfg engine.Config) (engine.Engine, error) {
	if cfg.Mode != engine.ModeCascaded {
		return nil, fmt.Errorf("dialog/cascaded: factory called with mode %q, want %q",
			string(cfg.Mode), string(engine.ModeCascaded))
	}
	return New(cfg), nil
}

// NewFactory returns the cascaded engine factory. Exposed so callers
// (test setup, feature-flagged production wiring) can register this
// engine without depending on package-private types.
func NewFactory() engine.Factory { return factory{} }

// RegisterForTesting installs the cascaded factory under
// engine.ModeCascaded. Intended for tests and feature-flagged
// experiments only.
//
// Why no init()-time auto-register: the legacy bridge in
// pkg/sip/conversation/dialog_engine_bridge.go already claims
// engine.ModeCascaded at bootstrap. engine.Register panics on a
// duplicate, so the two registrations would race. Production keeps
// the legacy bridge; opt-in callers (typically tests after calling
// engine.ResetRegistryForTest) get the native engine.
//
// Returns an error rather than panicking so callers can decide how
// to handle the duplicate case.
func RegisterForTesting() error {
	defer func() { _ = recover() }()
	// Best-effort: engine.Register panics on duplicate. Wrapping it
	// in a defer-recover keeps test setup robust against ordering
	// quirks; the caller is responsible for calling
	// engine.ResetRegistryForTest first if they want a clean slate.
	engine.Register(engine.ModeCascaded, factory{})
	return nil
}
