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

func TestUseNativeCascaded_GlobalOverride(t *testing.T) {
	cleanup := withNativeCascadedRouter(makeFakeEnv(map[string]string{
		envNativeCascadedAll: "1",
	}))
	defer cleanup()
	if !useNativeCascaded("any-tenant") {
		t.Error("ALL=1 should route every tenant through native")
	}
	if !useNativeCascaded("") {
		t.Error("ALL=1 should still take effect for empty tenant id (caller decides)")
	}
}

func TestUseNativeCascaded_GlobalDisabled(t *testing.T) {
	cleanup := withNativeCascadedRouter(makeFakeEnv(map[string]string{
		envNativeCascadedAll: "0",
	}))
	defer cleanup()
	if useNativeCascaded("any-tenant") {
		t.Error("ALL=0 should NOT route through native")
	}
}

func TestUseNativeCascaded_TenantAllowList(t *testing.T) {
	cleanup := withNativeCascadedRouter(makeFakeEnv(map[string]string{
		envNativeCascadedTenants: "tenant-a, tenant-b",
	}))
	defer cleanup()
	if !useNativeCascaded("tenant-a") {
		t.Error("tenant-a is in the allow-list; should route native")
	}
	if !useNativeCascaded("tenant-b") {
		t.Error("tenant-b is in the allow-list; should route native")
	}
	if useNativeCascaded("tenant-c") {
		t.Error("tenant-c is NOT in the allow-list; should NOT route native")
	}
}

func TestUseNativeCascaded_EmptyTenantNeverMatchesAllowList(t *testing.T) {
	cleanup := withNativeCascadedRouter(makeFakeEnv(map[string]string{
		envNativeCascadedTenants: "tenant-a",
	}))
	defer cleanup()
	if useNativeCascaded("") {
		t.Error("empty tenant id must not match the allow-list (defensive)")
	}
}

func TestUseNativeCascaded_NoEnvDefaultOff(t *testing.T) {
	cleanup := withNativeCascadedRouter(makeFakeEnv(nil))
	defer cleanup()
	if useNativeCascaded("any-tenant") {
		t.Error("no env vars set; default behaviour must be legacy bridge")
	}
}

func TestUseNativeCascaded_AllOverridesAllowList(t *testing.T) {
	cleanup := withNativeCascadedRouter(makeFakeEnv(map[string]string{
		envNativeCascadedAll:     "true",
		envNativeCascadedTenants: "only-tenant-a",
	}))
	defer cleanup()
	if !useNativeCascaded("tenant-z") {
		t.Error("ALL=true takes precedence over the allow-list; tenant-z should route native")
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
