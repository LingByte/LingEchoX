package protocol

// WebRTC-specific message types
const (
	MessageTypeICECandidate  MessageType = 10
	MessageTypeSDPOffer      MessageType = 11
	MessageTypeSDPAnswer     MessageType = 12
	MessageTypeDTLSHandshake MessageType = 13
)

// ICECandidateMessage represents an ICE candidate message
type ICECandidateMessage struct {
	Candidate     string `json:"candidate"`
	SDPMLineIndex *int   `json:"sdpMLineIndex"`
	SDPMid        string `json:"sdpMid"`
}

// SDPMessage represents an SDP offer/answer message
type SDPMessage struct {
	Type string `json:"type"` // "offer" or "answer"
	SDP  string `json:"sdp"`
}

// DTLSHandshakeMessage represents a DTLS handshake message
type DTLSHandshakeMessage struct {
	Fingerprint string `json:"fingerprint"`
	Hash        string `json:"hash"`
}

// Extend ControlMessage to support WebRTC messages
func NewICECandidateMessage(nodeID, roomID string, candidate ICECandidateMessage) *ControlMessage {
	return &ControlMessage{
		Type:    MessageTypeICECandidate,
		NodeID:  nodeID,
		RoomID:  roomID,
		Payload: candidate,
	}
}

func NewSDPMessage(nodeID, roomID string, sdp SDPMessage) *ControlMessage {
	var msgType MessageType
	if sdp.Type == "offer" {
		msgType = MessageTypeSDPOffer
	} else {
		msgType = MessageTypeSDPAnswer
	}

	return &ControlMessage{
		Type:    msgType,
		NodeID:  nodeID,
		RoomID:  roomID,
		Payload: sdp,
	}
}
