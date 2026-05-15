// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package session

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/voice/gateway"
	"github.com/LinByte/VoiceServer/pkg/voice/recorder"
)

// fakeStore captures the latest written object so tests can verify the
// recorder integration without an external storage backend. It satisfies
// pkg/stores.Store.
type fakeStore struct {
	mu     sync.Mutex
	bucket string
	key    string
	bytes  []byte
}

func (f *fakeStore) Write(bucket, key string, r io.Reader) error {
	buf, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bucket = bucket
	f.key = key
	f.bytes = buf
	return nil
}

func (f *fakeStore) Read(bucket, key string) (io.ReadCloser, int64, error) {
	return nil, 0, nil
}

func (f *fakeStore) Delete(string, string) error         { return nil }
func (f *fakeStore) Exists(string, string) (bool, error) { return true, nil }
func (f *fakeStore) PublicURL(bucket, key string) string {
	return "https://test.example/" + bucket + "/" + key
}

// TestCallSession_RecorderHelpersNilSafe verifies that the recorder API
// on CallSession is nil-safe (the helpers are no-ops when the recorder
// has not been enabled), matching the documented contract.
func TestCallSession_RecorderHelpersNilSafe(t *testing.T) {
	cs := &CallSession{CallID: "ut-call", pcmSampleRate: 16000}

	if cs.HasRecorder() {
		t.Fatal("expected HasRecorder=false before EnableRecorder")
	}

	// All write paths must tolerate a missing recorder without panic.
	cs.WriteCallerPCM([]byte{0, 0, 0, 0})
	cs.WriteAIPCM([]byte{0, 0, 0, 0})

	info, ok := cs.FlushRecorder(context.Background())
	if ok {
		t.Fatalf("expected FlushRecorder to return ok=false on inactive recorder, got %+v", info)
	}
}

// TestCallSession_RecorderHandlesOddLengthPCM verifies that odd-byte
// PCM inputs do not corrupt the stereo WAV. A naive recorder would
// glue the stray byte onto the next sample's high byte, shifting the
// rest of the channel by 1 byte ("电流音"-style static). The fix in
// pkg/voice/recorder/recorder.go (append) drops the trailing odd byte.
func TestCallSession_RecorderHandlesOddLengthPCM(t *testing.T) {
	cs := &CallSession{CallID: "ut-odd-pcm", pcmSampleRate: 16000}
	store := &fakeStore{}
	if !cs.EnableRecorder(recorder.Config{Store: store}) {
		t.Fatal("EnableRecorder failed")
	}

	// Mix even and odd-length PCM frames as if a buggy resampler had
	// returned 321 / 159 byte tails. The recorder must absorb these
	// without breaking byte alignment in the resulting WAV.
	cs.WriteCallerPCM(make([]byte, 320))
	cs.WriteCallerPCM(make([]byte, 321)) // odd
	cs.WriteAIPCM(make([]byte, 320))
	cs.WriteAIPCM(make([]byte, 159)) // odd
	cs.WriteAIPCM(make([]byte, 320))

	info, ok := cs.FlushRecorder(context.Background())
	if !ok {
		t.Fatalf("FlushRecorder failed: %+v", info)
	}
	// Stereo PCM16 LE: total bytes must be a multiple of 4 (2 channels × 2 bytes).
	if info.Bytes <= 0 || info.Bytes%4 != 0 {
		t.Fatalf("expected stereo WAV bytes %% 4 == 0, got %d", info.Bytes)
	}
	if !strings.HasPrefix(info.Hash, "sha256:") {
		t.Fatalf("expected sha256 hash, got %q", info.Hash)
	}
}

// TestCallSession_EnableRecorderForcesSessionFields verifies that
// EnableRecorder overrides cfg.CallID and cfg.SampleRate from the call
// session — callers should not have to pre-populate them.
func TestCallSession_EnableRecorderForcesSessionFields(t *testing.T) {
	cs := &CallSession{CallID: "ut-call-2", pcmSampleRate: 8000}

	store := &fakeStore{}
	cfg := recorder.Config{
		CallID:     "wrong-id",
		SampleRate: 0, // invalid; EnableRecorder must replace with cs.pcmSampleRate
		Store:      store,
	}
	if !cs.EnableRecorder(cfg) {
		t.Fatal("EnableRecorder should succeed when pcmSampleRate>0")
	}
	if !cs.HasRecorder() {
		t.Fatal("HasRecorder should be true after EnableRecorder")
	}

	// Re-enabling is a no-op (idempotent contract).
	if !cs.EnableRecorder(cfg) {
		t.Fatal("second EnableRecorder should still report ok=true (idempotent)")
	}

	// Feed a tiny amount of PCM and flush; the result must reference the
	// session's own CallID, not the (wrong) one we passed in.
	cs.WriteCallerPCM(make([]byte, 320))
	cs.WriteAIPCM(make([]byte, 320))

	info, ok := cs.FlushRecorder(context.Background())
	if !ok {
		t.Fatalf("FlushRecorder failed: %+v", info)
	}
	if !strings.Contains(info.Key, "ut-call-2") {
		t.Fatalf("recording key %q should embed the session CallID", info.Key)
	}
	if info.SampleRate != 8000 {
		t.Fatalf("expected SampleRate=8000 (forced from session), got %d", info.SampleRate)
	}
	if info.Bytes <= 0 || len(store.bytes) == 0 {
		t.Fatalf("expected non-empty WAV uploaded; got bytes=%d store=%d", info.Bytes, len(store.bytes))
	}
	if info.Layout != "stereo-l-r" {
		t.Fatalf("expected stereo layout, got %q", info.Layout)
	}
	// LingEchoX-side improvement over VoiceServer: Hash must be filled.
	if !strings.HasPrefix(info.Hash, "sha256:") || len(info.Hash) != len("sha256:")+64 {
		t.Fatalf("expected sha256:<64 hex> hash, got %q", info.Hash)
	}

	// Second Flush is a no-op.
	if _, ok := cs.FlushRecorder(context.Background()); ok {
		t.Fatal("second FlushRecorder should return ok=false")
	}
	// Recorder is detached.
	if cs.HasRecorder() {
		t.Fatal("HasRecorder should be false after Flush")
	}

	// Sanity: returned info uses the gateway type so it can flow into
	// ByePersistParams.WAVRecording without conversion.
	var _ gateway.RecordingInfo = info
}
