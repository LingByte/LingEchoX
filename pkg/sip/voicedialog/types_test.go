package voicedialog

import "testing"

func TestWireConstants(t *testing.T) {
	if ProtocolVersion != 2 {
		t.Fatalf("ProtocolVersion %d", ProtocolVersion)
	}
	if ProtocolLingechoVoiceDialog != "lingecho-voice-dialog" {
		t.Fatal(ProtocolLingechoVoiceDialog)
	}
	if EvASRFinal != "asr.final" || CmdTTSSpeak != "tts.speak" {
		t.Fatal("event/cmd mismatch")
	}
	if WSReadBufferSize <= 0 || WSWriteBufferSize <= 0 {
		t.Fatal("ws buffer sizes")
	}
}

func TestConfigZero(t *testing.T) {
	var c Config
	if c.InboundLoopbackWS {
		t.Fatal("zero value")
	}
}
