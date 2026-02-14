package constants

import (
	"time"
)

const (
	DefaultICETimeout = 10 * time.Second
	DefaultStreamID   = "lingechox"
	DefaultCodec      = "pcmu"
	WebRTCOffer       = "offer"
	WebRTCAnswer      = "answer"
	WebRTCCandidate   = "candidate"
)

const (
	CodecPCMU = "pcmu"
	CodecPCMA = "pcma"
	CodecG722 = "g722"
	CodecOPUS = "opus"
	CodecG711 = "g711"
)

const (
	MESSAGE_INIT      = "init"
	MESSAGE_OFFER     = "offer"
	MESSAGE_ANSWER    = "answer"
	MESSAGE_CONNECTED = "connected"
)

// ClientAudioFile Audio file configuration
const ClientAudioFile = "ringring.wav"

const (
	ConnectionStateLogInterval = 10
	MaxConnectionRetries       = 100
	ConnectionRetryDelay       = 50 * time.Millisecond
)
