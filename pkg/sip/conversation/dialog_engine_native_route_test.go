// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package conversation

import (
	"testing"
)

// makeFakeEnv builds a getenv stub that returns the given map values.
// Anything not in the map maps to "" (matches os.Getenv semantics for
// unset vars).
func makeFakeEnv(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

// --- isTruthyEnv -----------------------------------------------------

func TestIsTruthyEnv(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"FALSE", false},
		{"no", false},
		{"off", false},
		{"  off  ", false},
		{"1", true},
		{"true", true},
		{"True", true},
		{"yes", true},
		{"on", true},
		// any other non-empty value → truthy (operator-friendly)
		{"enabled", true},
		{"please", true},
	}
	for _, c := range cases {
		if got := isTruthyEnv(c.in); got != c.want {
			t.Errorf("isTruthyEnv(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// --- parseTenantList -------------------------------------------------

func TestParseTenantList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b ,c", []string{"a", "b", "c"}},
		{"a;b;c", []string{"a", "b", "c"}},
		{"a b c", []string{"a", "b", "c"}},
		{"a\tb\nc", []string{"a", "b", "c"}},
		{",,a,,,b,,", []string{"a", "b"}},
		{"tenant-1,tenant-2", []string{"tenant-1", "tenant-2"}},
	}
	for _, c := range cases {
		got := parseTenantList(c.in)
		if !slicesEqual(got, c.want) {
			t.Errorf("parseTenantList(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// --- useNativeCascaded ----------------------------------------------
//
// Post-rollout semantics: native is the default. The kill-switch
// envNativeCascadedDisable is the only knob; legacy ALL/TENANTS env
// vars are no longer load-bearing.

func TestUseNativeCascaded_DefaultOn(t *testing.T) {
	cleanup := withNativeCascadedRouter(makeFakeEnv(nil))
	defer cleanup()
	if !useNativeCascaded("any-tenant") {
		t.Error("default behaviour must be native (no env set)")
	}
	if !useNativeCascaded("") {
		t.Error("empty tenant id should still route native (no defensive bail-out)")
	}
}

func TestUseNativeCascaded_KillSwitchAll(t *testing.T) {
	cleanup := withNativeCascadedRouter(makeFakeEnv(map[string]string{
		envNativeCascadedDisable: "ALL",
	}))
	defer cleanup()
	if useNativeCascaded("any-tenant") {
		t.Error("DISABLE=ALL should kill native for every tenant")
	}
}

func TestUseNativeCascaded_KillSwitchTruthyValueDisablesAll(t *testing.T) {
	cleanup := withNativeCascadedRouter(makeFakeEnv(map[string]string{
		envNativeCascadedDisable: "true",
	}))
	defer cleanup()
	if useNativeCascaded("any-tenant") {
		t.Error("DISABLE=true should also kill native globally")
	}
}

func TestUseNativeCascaded_KillSwitchPerTenant(t *testing.T) {
	cleanup := withNativeCascadedRouter(makeFakeEnv(map[string]string{
		envNativeCascadedDisable: "tenant-a, tenant-b",
	}))
	defer cleanup()
	if useNativeCascaded("tenant-a") {
		t.Error("tenant-a in disable list should fall back to legacy")
	}
	if useNativeCascaded("tenant-b") {
		t.Error("tenant-b in disable list should fall back to legacy")
	}
	if !useNativeCascaded("tenant-z") {
		t.Error("tenant-z NOT in disable list should still route native")
	}
}

func TestUseNativeCascaded_LegacyAllEnvIsNoOp(t *testing.T) {
	// Pre-rollout playbooks set ALL=1; today that's a no-op (the
	// flag is no longer read). Ensure native still fires regardless.
	cleanup := withNativeCascadedRouter(makeFakeEnv(map[string]string{
		envNativeCascadedAll: "0", // simulate legacy disable
	}))
	defer cleanup()
	if !useNativeCascaded("any-tenant") {
		t.Error("legacy ALL=0 should be ignored; native is the default")
	}
}

// --- helpers ---------------------------------------------------------

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
