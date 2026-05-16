// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package conversation

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveWelcomeWavPath_EnvOverride 验证 SIP_WELCOME_WAV_PATH 优先于
// 默认 scripts/welcome.wav；空 / 未设置时回退到默认路径。
func TestResolveWelcomeWavPath_EnvOverride(t *testing.T) {
	t.Setenv("SIP_WELCOME_WAV_PATH", "")
	if got := resolveWelcomeWavPath(); got != "scripts/welcome.wav" {
		t.Errorf("default: got %q want scripts/welcome.wav", got)
	}

	t.Setenv("SIP_WELCOME_WAV_PATH", "/abs/path/hi.wav")
	if got := resolveWelcomeWavPath(); got != "/abs/path/hi.wav" {
		t.Errorf("abs override: got %q", got)
	}

	// Relative path should be filepath.Clean'd (collapse `./`, `..`, etc.)
	// but NOT prefixed — playResolvedWelcomeWav reads relative to PWD.
	t.Setenv("SIP_WELCOME_WAV_PATH", "./assets//hi.wav")
	if got, want := resolveWelcomeWavPath(), filepath.Clean("./assets//hi.wav"); got != want {
		t.Errorf("relative clean: got %q want %q", got, want)
	}
}

// TestWelcomeWavExists_MissingAndPresent 覆盖三种生产路径：文件存在、
// 文件不存在（"skip this phase" 的核心场景）、路径指向目录。
func TestWelcomeWavExists_MissingAndPresent(t *testing.T) {
	dir := t.TempDir()
	wavPath := filepath.Join(dir, "hi.wav")
	if err := os.WriteFile(wavPath, []byte("RIFF"), 0644); err != nil {
		t.Fatal(err)
	}

	if ok, err := welcomeWavExists(wavPath); !ok || err != nil {
		t.Errorf("present file: ok=%v err=%v want true,nil", ok, err)
	}
	if ok, err := welcomeWavExists(filepath.Join(dir, "missing.wav")); ok || err != nil {
		t.Errorf("missing file: ok=%v err=%v want false,nil", ok, err)
	}
	// 路径指向目录时应当返回 false（避免试图把目录当 WAV 加载）。
	if ok, err := welcomeWavExists(dir); ok || err != nil {
		t.Errorf("directory path: ok=%v err=%v want false,nil", ok, err)
	}
	if ok, err := welcomeWavExists(""); ok || err != nil {
		t.Errorf("empty path: ok=%v err=%v want false,nil", ok, err)
	}
	if ok, err := welcomeWavExists("   "); ok || err != nil {
		t.Errorf("whitespace path: ok=%v err=%v want false,nil", ok, err)
	}
}
