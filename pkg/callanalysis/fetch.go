package callanalysis

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DownloadAudioURLToTemp downloads http(s) audio to a temp file and returns Input.LocalPath (caller must Remove).
func DownloadAudioURLToTemp(ctx context.Context, rawURL string) (tmpPath string, in Input, err error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return "", in, fmt.Errorf("audio_url must be http(s) with a host")
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", in, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", in, fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", in, fmt.Errorf("download: http %d", resp.StatusCode)
	}
	body, err := ReadAllLimited(resp.Body, MaxAudioBytes)
	if err != nil {
		return "", in, err
	}
	ext := filepath.Ext(u.Path)
	if ext == "" || len(ext) > 8 {
		ext = ".audio"
	}
	f, err := os.CreateTemp("", "call-analysis-url-*"+ext)
	if err != nil {
		return "", in, err
	}
	tmpPath = f.Name()
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", in, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", in, err
	}
	in = Input{
		LocalPath: tmpPath,
		Source:    "url",
		Filename:  filepath.Base(u.Path),
		AudioURL:  u.String(),
	}
	return tmpPath, in, nil
}
