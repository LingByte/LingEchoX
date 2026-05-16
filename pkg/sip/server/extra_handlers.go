package server

import (
	"net"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/sip/stack"
)

func (s *SIPServer) handleNotify(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	if s == nil || msg == nil {
		return nil
	}
	if s.absorbNonInviteRetransmit(msg, addr) {
		return nil
	}
	ev := strings.ToLower(strings.TrimSpace(msg.GetHeader("Event")))
	if strings.HasPrefix(ev, "refer") {
		resp := s.makeResponse(msg, 200, "OK", "", "")
		resp.SetHeader("Content-Length", "0")
		return resp
	}
	return s.handleNotifyPresence(msg, addr)
}

func (s *SIPServer) handleUpdate(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	if s == nil || msg == nil {
		return nil
	}
	if s.absorbNonInviteRetransmit(msg, addr) {
		return nil
	}
	// RFC 4028 §6: UPDATE is a valid session-timer refresh method. If
	// we know this Call-ID's CallSession, touch its watchdog so the
	// peer's keep-alive doesn't get us BYE'd in `ChosenSE` seconds.
	if callID := strings.TrimSpace(msg.GetHeader("Call-ID")); callID != "" {
		if cs := s.GetCallSession(callID); cs != nil {
			cs.TouchSessionTimer()
		}
	}
	resp := s.makeResponse(msg, 200, "OK", "", "")
	resp.SetHeader("Content-Length", "0")
	return resp
}

func (s *SIPServer) handleMessage(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	if s == nil || msg == nil {
		return nil
	}
	if s.absorbNonInviteRetransmit(msg, addr) {
		return nil
	}
	if strings.TrimSpace(msg.Body) == "" {
		resp := s.makeResponse(msg, 200, "OK", "", "")
		resp.SetHeader("Content-Length", "0")
		return resp
	}
	resp := s.makeResponse(msg, 415, "Unsupported Media Type", "", "")
	resp.SetHeader("Accept", "text/plain")
	resp.SetHeader("Content-Length", "0")
	return resp
}
