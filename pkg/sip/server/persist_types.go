package server

import "context"

// InvitePersistParams describes one inbound/outbound INVITE persistence snapshot.
type InvitePersistParams struct {
	CallID      string
	From        string
	To          string
	RemoteSig   string
	RemoteRTP   string
	LocalRTP    string
	Codec       string
	PayloadType uint8
	ClockRate   int
	CSeqInvite  string
	Direction   string
}

// ByePersistParams describes BYE-time persistence and recording metadata.
type ByePersistParams struct {
	CallID             string
	RawPayload         []byte
	CodecName          string
	Initiator          string
	RecordSampleRate   int
	RecordOpusChannels int
}

// SIPCallPersistStore defines persistence hooks used by SIP server.
type SIPCallPersistStore interface {
	OnInvite(ctx context.Context, p InvitePersistParams)
	OnEstablished(ctx context.Context, callID string)
	OnBye(ctx context.Context, p ByePersistParams)
}

