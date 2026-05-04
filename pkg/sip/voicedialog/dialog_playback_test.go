package voicedialog

import (
	"context"
	"testing"
	"time"

	"github.com/LingByte/SoulNexus/pkg/media"
)

func TestWaitFirstRTP_NoOp(t *testing.T) {
	ctx := context.Background()
	waitFirstRTP(ctx, nil, 100)
	waitFirstRTP(ctx, nil, 0)
}

func TestPlayPCMFrames_NoOp(t *testing.T) {
	ctx := context.Background()
	var ms *media.MediaSession
	if err := playPCMFrames(ctx, ms, nil, 16000, "t"); err != nil {
		t.Fatal(err)
	}
	if err := playPCMFrames(ctx, nil, []byte{1, 2}, 16000, "t"); err != nil {
		t.Fatal(err)
	}
	if err := playPCMFrames(ctx, ms, []byte{1, 2}, 0, "t"); err != nil {
		t.Fatal(err)
	}
}

func TestPlayPCMFrames_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ms := media.NewDefaultSession().Context(ctx)
	pcm := make([]byte, 640)
	if err := playPCMFrames(ctx, ms, pcm, 16000, "t"); err == nil || err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}

func TestEmitGatewayNilSafe(t *testing.T) {
	var sess *dialogSession
	sess.emitGateway(event(EvPong, "x", nil))
}

func TestDeliverConversationTransferPhase_NoHub(t *testing.T) {
	defaultHub = nil
	deliverConversationTransferPhase("any", PhaseTransferRequested, nil)
}

func TestTransferLoadingStopNil(t *testing.T) {
	var sess *dialogSession
	sess.stopTransferLoadingPlayback()
}

func TestBeginTransferLoadingNilSession(t *testing.T) {
	var sess *dialogSession
	sess.beginTransferLoadingPlayback()
}

func TestRunDialogWelcomeNil(t *testing.T) {
	var sess *dialogSession
	sess.runDialogWelcome()
}

func TestLoadVoicedialogWAVPCM_BadURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := loadVoicedialogWAVPCM(ctx, SourceKindURL, "http://127.0.0.1:9/no-listener", 16000)
	if err == nil {
		t.Fatal("expected error")
	}
}
