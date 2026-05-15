package outbound

import (
	"strings"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/sip/rtp"
	"github.com/LinByte/VoiceServer/pkg/sip/sdp"
)

func TestApplyOutboundAnswerSRTP_downgradesToPlainRTP(t *testing.T) {
	sess, err := rtp.NewSession(0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()

	offerK := make([]byte, 16)
	offerS := make([]byte, 14)
	for i := range offerK {
		offerK[i] = byte(i + 1)
	}
	for i := range offerS {
		offerS[i] = byte(i + 3)
	}

	ans := &sdp.Info{Proto: "RTP/AVP"}
	if err := applyOutboundAnswerSRTP(sess, offerK, offerS, ans); err != nil {
		t.Fatal(err)
	}
}

func TestApplyOutboundAnswerSRTP_negotiatesWithAnswerCrypto(t *testing.T) {
	sess, err := rtp.NewSession(0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()

	offerK := bytesRepeat(16, 0x11)
	offerS := bytesRepeat(14, 0x22)
	answerK := bytesRepeat(16, 0x33)
	answerS := bytesRepeat(14, 0x44)

	line, err := sdp.FormatCryptoLine(1, sdp.SuiteAESCM128HMACSHA180, answerK, answerS)
	if err != nil {
		t.Fatal(err)
	}
	idx := strings.Index(line, "inline:")
	if idx < 0 {
		t.Fatalf("line: %s", line)
	}
	kp := strings.TrimSpace(line[idx:])

	ans := &sdp.Info{
		Proto: "RTP/SAVPF",
		CryptoOffers: []sdp.CryptoOffer{
			{Tag: 1, Suite: sdp.SuiteAESCM128HMACSHA180, KeyParams: kp},
		},
	}
	if err := applyOutboundAnswerSRTP(sess, offerK, offerS, ans); err != nil {
		t.Fatal(err)
	}
}

func TestApplyOutboundAnswerSRTP_missingCryptoErrors(t *testing.T) {
	sess, err := rtp.NewSession(0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()

	offerK := bytesRepeat(16, 1)
	offerS := bytesRepeat(14, 2)
	ans := &sdp.Info{Proto: "RTP/SAVPF", CryptoOffers: nil}
	if err := applyOutboundAnswerSRTP(sess, offerK, offerS, ans); err == nil {
		t.Fatal("expected error")
	}
}

func bytesRepeat(n int, v byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = v
	}
	return b
}
