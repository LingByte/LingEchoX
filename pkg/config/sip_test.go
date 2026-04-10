package config

import (
	"os"
	"testing"

	"github.com/LingByte/SoulNexus/pkg/constants"
)

func TestDialTargetFromEnv_BuildsFromTargetAndHost(t *testing.T) {
	_ = os.Setenv(constants.EnvSIPTargetNumber, "1001")
	_ = os.Setenv(constants.EnvSIPOutboundHost, "pbx.example.com")
	_ = os.Setenv(constants.EnvSIPOutboundPort, "5060")
	_ = os.Setenv(constants.EnvSIPSignalingAddr, "")
	defer func() {
		_ = os.Unsetenv(constants.EnvSIPTargetNumber)
		_ = os.Unsetenv(constants.EnvSIPOutboundHost)
		_ = os.Unsetenv(constants.EnvSIPOutboundPort)
		_ = os.Unsetenv(constants.EnvSIPSignalingAddr)
		_ = os.Unsetenv(constants.EnvSIPOutboundReqURI)
	}()

	dt, ok := DialTargetFromEnv()
	if !ok {
		t.Fatal("expected ok")
	}
	if dt.RequestURI != "sip:1001@pbx.example.com:5060" {
		t.Fatalf("RequestURI: %q", dt.RequestURI)
	}
	if dt.SignalingAddr != "pbx.example.com:5060" {
		t.Fatalf("SignalingAddr: %q", dt.SignalingAddr)
	}
}

func TestTransferDialTargetFromEnv_WebSeat(t *testing.T) {
	_ = os.Setenv(constants.EnvSIPTransferReqURI, "")
	_ = os.Setenv(constants.EnvSIPTransferSigAddr, "")
	_ = os.Setenv(constants.EnvSIPTransferNumber, "web")
	_ = os.Setenv(constants.EnvSIPTransferHost, "")
	defer func() {
		_ = os.Unsetenv(constants.EnvSIPTransferReqURI)
		_ = os.Unsetenv(constants.EnvSIPTransferSigAddr)
		_ = os.Unsetenv(constants.EnvSIPTransferNumber)
		_ = os.Unsetenv(constants.EnvSIPTransferHost)
	}()

	dt, ok := TransferDialTargetFromEnv()
	if !ok || !dt.WebSeat {
		t.Fatalf("expected WebSeat ok, got ok=%v dt=%+v", ok, dt)
	}
	if dt.RequestURI != "" || dt.SignalingAddr != "" {
		t.Fatalf("expected empty SIP fields, got %+v", dt)
	}
}
