package server

import (
	"context"
	"net"
	"time"

	"github.com/LinByte/VoiceServer/pkg/voice/gateway"
)

// SIPRegisterStore persists REGISTER bindings for INVITE proxy and outbound dial lookup.
// Implementations must be safe for concurrent use (e.g. GORM).
type SIPRegisterStore interface {
	// SaveRegister stores the resolved Contact signaling target (UDP), same as INVITE proxy destination.
	SaveRegister(ctx context.Context, user, domain, contactURI string, sig *net.UDPAddr, expiresAt time.Time, userAgent string) error

	DeleteRegister(ctx context.Context, user, domain string) error
	// LookupRegister returns the UDP signaling target for a registered AOR (Contact / Via path).
	LookupRegister(ctx context.Context, user, domain string) (*net.UDPAddr, bool, error)
}

// InboundDIDBinding ties an inbound INVITE's called-party (DID) to tenant + trunk_number row.
type InboundDIDBinding struct {
	TenantID      uint
	TrunkNumberID uint // sip_trunk_numbers.id; 0 if unresolved
}

// InvitePersistParams describes one inbound/outbound INVITE persistence snapshot.
type InvitePersistParams struct {
	TenantID             uint
	InboundTrunkNumberID uint // sip_trunk_numbers.id when matched via DID resolver; 0 otherwise
	CallID               string
	From                 string
	To                   string
	RemoteSig            string
	RemoteRTP            string
	LocalRTP             string
	Codec                string
	PayloadType          uint8
	ClockRate            int
	CSeqInvite           string
	Direction            string
}

// ByePersistParams describes BYE-time persistence and recording metadata.
//
// RawPayload carries the legacy SN3 blob (RTP-payload-level recording);
// downstream persisters decode this into a WAV by calling into
// pkg/utils/sip_recording_wav.go. When the new stereo PCM recorder
// (pkg/voice/recorder) was active for the call, the server also fills in
// WAVRecording with the already-uploaded RecordingInfo and the persister
// should prefer that path (zero re-encoding, single canonical artefact).
type ByePersistParams struct {
	CallID             string
	RawPayload         []byte
	CodecName          string
	Initiator          string
	RecordSampleRate   int
	RecordOpusChannels int

	// WAVRecording is non-nil when pkg/voice/recorder produced a stereo
	// WAV for this call. Persister implementations should write its
	// URL/Bytes/DurationMs onto the call_recording row instead of
	// re-decoding RawPayload. Zero-valued (non-pointer? — Bucket=="" check) means SN3 fallback.
	WAVRecording gateway.RecordingInfo
}

// SIPCallPersistStore defines persistence hooks used by SIP server.
type SIPCallPersistStore interface {
	OnInvite(ctx context.Context, p InvitePersistParams)
	OnEstablished(ctx context.Context, callID string)
	OnBye(ctx context.Context, p ByePersistParams)
}
