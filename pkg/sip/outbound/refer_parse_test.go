package outbound

import "testing"

func TestDialTargetFromReferTo(t *testing.T) {
	dt, err := DialTargetFromReferTo(`<sip:8000@10.0.0.5:5060;user=phone>`)
	if err != nil {
		t.Fatal(err)
	}
	if dt.RequestURI != "sip:8000@10.0.0.5:5060" {
		t.Fatalf("RequestURI: %q", dt.RequestURI)
	}
	if dt.SignalingAddr != "10.0.0.5:5060" {
		t.Fatalf("SignalingAddr: %q", dt.SignalingAddr)
	}
}

func TestDialTargetFromReferToDefaultPort(t *testing.T) {
	dt, err := DialTargetFromReferTo("sip:bob@pbx.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if dt.SignalingAddr != "pbx.example.test:5060" {
		t.Fatalf("SignalingAddr: %q", dt.SignalingAddr)
	}
}
