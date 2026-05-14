package webseat

import (
	"testing"
)

func TestPersistSnapshotInboundNil(t *testing.T) {
	raw, name, sr, ch := persistSnapshotInbound(nil)
	if raw != nil || name != "" || sr != 0 || ch != 0 {
		t.Fatalf("%v %q %d %d", raw, name, sr, ch)
	}
}

func TestRegisterAwaitingErrors(t *testing.T) {
	old := defaultHub
	defer func() { defaultHub = old }()

	defaultHub = nil
	if err := RegisterAwaiting("x", nil, nil); err == nil {
		t.Fatal("nil hub")
	}

	InitDefault(Config{})
	if err := RegisterAwaiting("", nil, nil); err == nil {
		t.Fatal("invalid args")
	}
}

func TestIsPendingOrActiveNilHub(t *testing.T) {
	old := defaultHub
	defaultHub = nil
	defer func() { defaultHub = old }()
	if IsPendingOrActive("any") || IsActive("any") {
		t.Fatal()
	}
}

func TestBindReleaseWebACDNilHub(t *testing.T) {
	old := defaultHub
	defaultHub = nil
	defer func() { defaultHub = old }()
	BindInboundCallToWebACD("cid", 1)
	ReleaseInboundWebACDOffer("cid")
}

func TestTeardownNilHub(t *testing.T) {
	old := defaultHub
	defaultHub = nil
	defer func() { defaultHub = old }()
	if teardownWebSeat("x", false) {
		t.Fatal()
	}
}

func TestEnvConstant(t *testing.T) {
	if EnvWSToken != "SIP_WEBSEAT_WS_TOKEN" {
		t.Fatal(EnvWSToken)
	}
}
