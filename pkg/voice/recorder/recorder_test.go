// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package recorder

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// memStore is a minimal stores.Store impl backed by an in-process map so
// the test can inspect the WAV bytes without touching disk. We only
// implement what the recorder actually calls (Write); the rest of the
// interface is satisfied with no-op stubs.
type memStore struct{ m map[string][]byte }

func newMemStore() *memStore { return &memStore{m: map[string][]byte{}} }

func (s *memStore) Write(bucket, key string, body io.Reader) error {
	b, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.m[bucket+"/"+key] = b
	return nil
}
func (s *memStore) Read(_, _ string) (io.ReadCloser, int64, error) { return nil, 0, nil }
func (s *memStore) Exists(_, _ string) (bool, error)               { return false, nil }
func (s *memStore) Delete(bucket, key string) error {
	delete(s.m, bucket+"/"+key)
	return nil
}
func (s *memStore) PublicURL(_, _ string) string { return "" }

func TestRecorder_FlushProducesValidStereoWAV(t *testing.T) {
	store := newMemStore()
	r := New(Config{
		CallID:     "call-test-1",
		Bucket:     "test-recordings",
		SampleRate: 16000,
		Transport:  "test",
		Store:      store,
	})
	if r == nil {
		t.Fatal("New returned nil")
	}

	// Write 100 ms of caller PCM (3200 bytes = 1600 samples @ 16 kHz).
	caller := make([]byte, 3200)
	r.WriteCaller(caller)
	// Stagger AI write by 30 ms so wall-clock alignment puts a small
	// silence at the start of the right channel.
	time.Sleep(30 * time.Millisecond)
	ai := make([]byte, 3200)
	for i := range ai {
		ai[i] = byte(i % 256) // non-zero so we can detect it in the WAV
	}
	r.WriteAI(ai)

	info, ok := r.Flush(context.Background())
	if !ok {
		t.Fatal("Flush returned ok=false")
	}
	if info.Format != "wav" || info.Channels != 2 || info.SampleRate != 16000 {
		t.Fatalf("info: %+v", info)
	}
	if info.Bytes < 44 {
		t.Fatalf("wav too small: %d", info.Bytes)
	}
	stored := store.m[info.URL]
	if len(stored) != info.Bytes {
		t.Fatalf("stored %d bytes, info says %d", len(stored), info.Bytes)
	}
	// Validate RIFF/WAVE header.
	if string(stored[0:4]) != "RIFF" || string(stored[8:12]) != "WAVE" {
		t.Fatalf("not a WAVE file: %q ... %q", stored[0:4], stored[8:12])
	}
	// fmt chunk: PCM=1, channels=2, sample rate=16000, bits=16.
	channels := binary.LittleEndian.Uint16(stored[22:24])
	sr := binary.LittleEndian.Uint32(stored[24:28])
	bits := binary.LittleEndian.Uint16(stored[34:36])
	if channels != 2 || sr != 16000 || bits != 16 {
		t.Fatalf("fmt: ch=%d sr=%d bits=%d", channels, sr, bits)
	}
	// Find the data chunk and confirm it contains the AI bytes interleaved
	// in the right channel. Header is 44 bytes for canonical PCM WAV.
	data := stored[44:]
	if !bytes.Contains(data, []byte{0x01, 0x02, 0x03, 0x04}) {
		// Right-channel bytes are interleaved every 4 bytes; samples 1..N
		// of the AI track produce the pattern 1,0,2,0,3,0,4,0... in the
		// right pair after the alignment offset, but a coarse contains
		// check on a recognisable AI byte (0x10) in the right slot is
		// enough: at least one byte from the AI buffer must appear.
		// Use a loose substring check.
		var found bool
		for i := 2; i+1 < len(data); i += 4 {
			if data[i] != 0 || data[i+1] != 0 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("right channel appears to be all zero (AI not interleaved)")
		}
	}
}

func TestRecorder_FlushIdempotent(t *testing.T) {
	store := newMemStore()
	r := New(Config{CallID: "c1", SampleRate: 16000, Store: store})
	r.WriteCaller(make([]byte, 320))
	if _, ok := r.Flush(context.Background()); !ok {
		t.Fatal("first flush failed")
	}
	if _, ok := r.Flush(context.Background()); ok {
		t.Fatal("second flush should be a no-op")
	}
}

func TestRecorder_NilSafeMethods(t *testing.T) {
	var r *Recorder
	r.WriteCaller([]byte{1, 2})
	r.WriteAI([]byte{3, 4})
	if _, ok := r.Flush(context.Background()); ok {
		t.Fatal("nil receiver Flush should return ok=false")
	}
}

func TestRecorder_RollingChunkUpload(t *testing.T) {
	store := newMemStore()
	r := New(Config{
		CallID:        "chunked-call",
		Bucket:        "test",
		SampleRate:    16000,
		Store:         store,
		ChunkInterval: 50 * time.Millisecond,
	})
	if r == nil {
		t.Fatal("New returned nil")
	}
	// Push a frame, wait for a tick, push another, wait again, then flush.
	r.WriteCaller(make([]byte, 320))
	time.Sleep(80 * time.Millisecond)
	r.WriteAI(make([]byte, 320))
	time.Sleep(80 * time.Millisecond)
	if _, ok := r.Flush(context.Background()); !ok {
		t.Fatal("flush failed")
	}
	// Expect at least one chunk + one final WAV.
	var parts, finals int
	for k := range store.m {
		if bytes.Contains([]byte(k), []byte("-part-")) {
			parts++
		} else if bytes.HasSuffix([]byte(k), []byte(".wav")) {
			finals++
		}
	}
	if parts != 0 {
		t.Fatalf("expected 0 part-* after Flush (chunks should be reclaimed), got %d. keys: %v", parts, keysOf(store.m))
	}
	if finals < 1 {
		t.Fatalf("expected ≥1 final wav, got %d. keys: %v", finals, keysOf(store.m))
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestNew_RejectsBadConfig(t *testing.T) {
	if r := New(Config{SampleRate: 16000}); r != nil {
		t.Fatal("expected nil for empty CallID")
	}
	if r := New(Config{CallID: "x", SampleRate: 0}); r != nil {
		t.Fatal("expected nil for zero SampleRate")
	}
}

// TestPlacePCMTrackBytes_JitterSnap asserts the LingEchoX-side fix that
// continuous, paced TTS frames (~20ms apart but with ±1-3ms scheduler
// jitter) are concatenated back-to-back rather than zero-padded
// between every frame. The upstream VS recorder placed every frame on
// a raw wall-clock grid, producing periodic clicks/electric noise
// during continuous TTS playback.
func TestPlacePCMTrackBytes_JitterSnap(t *testing.T) {
	const rate = 8000
	const frameSamples = 160 // 20ms @ 8kHz
	frameBytes := frameSamples * 2

	// 10 frames, each carrying a non-zero marker byte so we can tell
	// apart "audio sample" vs "zero pad" in the output.
	mkFrame := func(marker byte) []byte {
		b := make([]byte, frameBytes)
		for i := 0; i < frameBytes; i++ {
			b[i] = marker
		}
		return b
	}

	base := int64(1_000_000_000) // arbitrary baseNs (1s into epoch)
	frameDurNs := int64(20 * 1_000_000)
	// Simulate ±2ms scheduler jitter on every frame (always positive
	// — the worst case for the old algorithm, which would insert
	// 16 samples of silence between every frame).
	segs := make([]frame, 0, 10)
	for i := 0; i < 10; i++ {
		jitterNs := int64(2_000_000) // +2ms drift per frame
		segs = append(segs, frame{
			wallNs: base + int64(i)*frameDurNs + jitterNs,
			pcm:    mkFrame(byte(0xA0 + i)),
		})
	}
	out := placePCMTrackBytes(segs, base, rate)

	// With jitter snap, all 10 frames concatenate back-to-back
	// without ANY initial pad (the 2ms initial offset also falls
	// inside the 80ms snap window). 10 × 320 bytes = 3200 bytes.
	wantBytes := 10 * frameBytes
	if len(out) != wantBytes {
		t.Fatalf("expected %d bytes (10 frames contiguous, jitter snapped), got %d",
			wantBytes, len(out))
	}

	// Verify there are NO zero bytes at all (every frame is filled
	// with a non-zero marker; any zero would indicate a silence pad).
	for i, b := range out {
		if b == 0 {
			t.Fatalf("found zero byte at offset %d — jitter snap not applied", i)
		}
	}
}

// TestPlacePCMTrackBytes_RealSilencePreserved ensures the jitter snap
// does NOT swallow legitimate inter-turn silence (>80ms gap).
func TestPlacePCMTrackBytes_RealSilencePreserved(t *testing.T) {
	const rate = 8000
	frameBytes := 160 * 2
	mk := func(marker byte) []byte {
		b := make([]byte, frameBytes)
		for i := range b {
			b[i] = marker
		}
		return b
	}
	base := int64(0)
	segs := []frame{
		{wallNs: 0, pcm: mk(0xAA)},
		// 500ms real silence gap before the next utterance.
		{wallNs: 500 * 1_000_000, pcm: mk(0xBB)},
	}
	out := placePCMTrackBytes(segs, base, rate)
	// Expect: 160 samples of frame 0, ~4000 samples of zero pad,
	// 160 samples of frame 1 → ~4320 samples total = 8640 bytes.
	if len(out) < 8000 || len(out) > 9000 {
		t.Fatalf("expected ~8640 bytes (silence preserved), got %d", len(out))
	}
	// Verify there IS a long zero run between the two frames.
	zeros := 0
	maxZeros := 0
	for _, b := range out {
		if b == 0 {
			zeros++
			if zeros > maxZeros {
				maxZeros = zeros
			}
		} else {
			zeros = 0
		}
	}
	if maxZeros < 1000 {
		t.Fatalf("expected ≥1000 contiguous zero bytes (real silence), got %d", maxZeros)
	}
}

// TestPlacePCMTrackBytes_EnvJitterSnapOverride 验证 SIP_RECORDING_JITTER_SNAP_MS
// 环境变量能调整 snap 窗口：把窗口改成 30ms 后，原本被 80ms 默认窗口吃掉的
// 60ms 间隔就应当被识别为"真静音"而保留零填充。这是排查现场抖动问题的旋钮。
func TestPlacePCMTrackBytes_EnvJitterSnapOverride(t *testing.T) {
	t.Setenv("SIP_RECORDING_JITTER_SNAP_MS", "30")
	const rate = 8000
	frameBytes := 160 * 2
	mk := func(marker byte) []byte {
		b := make([]byte, frameBytes)
		for i := range b {
			b[i] = marker
		}
		return b
	}
	// 60ms gap：默认 80ms 阈值会 snap 拼接（无零填充），改成 30ms 阈值后
	// 60ms > 30ms 应当被当作真静音保留。
	segs := []frame{
		{wallNs: 0, pcm: mk(0xAA)},
		{wallNs: 60 * 1_000_000, pcm: mk(0xBB)},
	}
	out := placePCMTrackBytes(segs, 0, rate)
	// 60ms @ 8kHz = 480 samples = 960 bytes 之间应该有非可忽略的零填充。
	zeros := 0
	maxZeros := 0
	for _, b := range out {
		if b == 0 {
			zeros++
			if zeros > maxZeros {
				maxZeros = zeros
			}
		} else {
			zeros = 0
		}
	}
	if maxZeros < 200 {
		t.Fatalf("expected ≥200 contiguous zero bytes once snap window is shrunk to 30ms, got %d", maxZeros)
	}
}

// TestRecorder_SampleRateMismatchWarns 验证当 caller 喂入的 PCM 字节流在
// 5s 观察窗口内推算出的 implied rate 与 cfg.SampleRate 偏差超过 30% 时，
// recorder 会立即 WARN 一次（且只一次）。
//
// 模拟方式：cfg 配置 8kHz，但实际以 16kHz 节奏喂入字节（每 wall-clock
// 秒喂 32000 字节而非 16000），observer 捕获 zap 日志。
func TestRecorder_SampleRateMismatchWarns(t *testing.T) {
	core, obs := observer.New(zap.WarnLevel)
	r := New(Config{
		CallID:     "rate-mismatch-test",
		Bucket:     "test",
		SampleRate: 8000,
		Logger:     zap.New(core),
		Store:      newMemStore(),
	})
	if r == nil {
		t.Fatal("New returned nil")
	}
	// 模拟真实 RTP 节奏：每 20ms 喂一块。
	// 16 kHz 实际：每 20ms = 320 samples = 640 bytes。
	// 喂 300 个 20ms chunk 覆盖 ~6 秒 wall，跨过 5s 阈值。
	// 注：N 太小时 implied = bytes/(N-1 个间隔) 会高估，需要 N 大一些才收敛。
	const (
		wallStart = int64(1_000_000_000)
		stepNs    = int64(20_000_000) // 20ms
		bytesPer  = int64(640)        // 16 kHz × 20ms × 2 byte/sample
		nChunks   = 300
	)
	for i := int64(0); i < nChunks; i++ {
		r.mu.Lock()
		r.updateRateStats(true, wallStart+i*stepNs, bytesPer)
		r.mu.Unlock()
	}
	logs := obs.FilterMessage("recorder: sample-rate mismatch detected").All()
	if len(logs) != 1 {
		t.Fatalf("expected exactly 1 warn log, got %d", len(logs))
	}
	got := logs[0].ContextMap()
	if got["leg"] != "caller" {
		t.Errorf("leg: got %v want caller", got["leg"])
	}
	if got["configured_hz"] != int64(8000) {
		t.Errorf("configured_hz: got %v want 8000", got["configured_hz"])
	}
	implied, _ := got["implied_hz"].(float64)
	// implied 在 ±5% 内应当贴近 16 kHz。
	if implied < 15200 || implied > 16800 {
		t.Errorf("implied_hz: got %v want ≈16000", implied)
	}

	// 再喂几次，warn 仍只应有 1 条（per-leg 只 warn 一次）。
	for i := int64(nChunks); i < nChunks+50; i++ {
		r.mu.Lock()
		r.updateRateStats(true, wallStart+i*stepNs, bytesPer)
		r.mu.Unlock()
	}
	logs = obs.FilterMessage("recorder: sample-rate mismatch detected").All()
	if len(logs) != 1 {
		t.Fatalf("expected per-leg-once warn, got %d", len(logs))
	}
}

// TestRecorder_SampleRateWithinTolerance_NoWarn 验证 implied rate 落在 ±30%
// 容忍度内时不报警。短期网络抖动 / 突发包不均会让短窗口 implied 飘 ±10-15%，
// 这些是正常的不应当被噪音化。
func TestRecorder_SampleRateWithinTolerance_NoWarn(t *testing.T) {
	core, obs := observer.New(zap.WarnLevel)
	r := New(Config{
		CallID:     "rate-tolerant-test",
		Bucket:     "test",
		SampleRate: 16000,
		Logger:     zap.New(core),
		Store:      newMemStore(),
	})
	if r == nil {
		t.Fatal("New returned nil")
	}
	// 16 kHz cfg，喂 18 kHz 速率（+12.5% 偏差，仍在 30% 内）。
	// 每 20ms 喂一块：18 kHz × 20ms × 2 = 720 字节。
	const (
		wallStart = int64(1_000_000_000)
		stepNs    = int64(20_000_000) // 20ms
		bytesPer  = int64(720)        // 18 kHz × 20ms × 2 byte/sample
		nChunks   = 300
	)
	for i := int64(0); i < nChunks; i++ {
		r.mu.Lock()
		r.updateRateStats(false, wallStart+i*stepNs, bytesPer)
		r.mu.Unlock()
	}
	if n := obs.FilterMessage("recorder: sample-rate mismatch detected").Len(); n != 0 {
		t.Fatalf("expected no warn within tolerance, got %d", n)
	}
}
