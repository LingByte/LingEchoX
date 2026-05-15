package outbound

import (
	"fmt"
	"net"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/sip/stack"
)

func buildBYE(inv inviteParams, toHeader200, requestURI string, cseq int, branch string) *stack.Message {
	reqURI := strings.TrimSpace(requestURI)
	if reqURI == "" {
		reqURI = inv.RequestURI
	}
	msg := &stack.Message{
		IsRequest:  true,
		Method:     stack.MethodBye,
		RequestURI: reqURI,
		Version:    "SIP/2.0",
	}
	via := fmt.Sprintf("SIP/2.0/UDP %s:%d;branch=z9hG4bK%s;rport",
		nonEmpty(inv.SIPHost, "127.0.0.1"), nonZero(inv.SIPPort, 6050), branch)
	msg.SetHeader("Via", via)
	msg.SetHeader("Max-Forwards", "70")
	msg.SetHeader("From", formatOutboundFromHeader(inv.FromDisplayName, inv.FromUser, inv.SIPHost, inv.SIPPort, inv.FromTag))
	if strings.TrimSpace(toHeader200) != "" {
		msg.SetHeader("To", toHeader200)
	} else {
		msg.SetHeader("To", formatToHeader(inv.RequestURI))
	}
	msg.SetHeader("Call-ID", inv.CallID)
	msg.SetHeader("CSeq", fmt.Sprintf("%d BYE", cseq))
	msg.SetHeader("Content-Length", "0")
	return msg
}

// SendBYE sends an in-dialog BYE for an established outbound leg (after 200 OK to INVITE).
func (m *Manager) SendBYE(callID string) error {
	if m == nil || m.send == nil {
		return fmt.Errorf("sip/outbound: manager not ready")
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return fmt.Errorf("sip/outbound: empty call-id")
	}
	leg := m.legByCallIDOrHostRewrite(callID)
	if leg == nil {
		return fmt.Errorf("sip/outbound: unknown call-id %s", callID)
	}
	leg.sigMu.Lock()
	defer leg.sigMu.Unlock()
	if strings.TrimSpace(leg.byeToHeader) == "" {
		return fmt.Errorf("sip/outbound: dialog not ready for BYE")
	}
	dst := leg.byeRemote
	if dst == nil {
		dst = leg.dst
	}
	if dst == nil {
		return fmt.Errorf("sip/outbound: no signaling address for BYE")
	}
	cseq := leg.byeCSeqNext
	if cseq <= 0 {
		cseq = leg.params.CSeq + 1
	}
	leg.byeCSeqNext = cseq + 1
	branch := randomHex(10)
	msg := buildBYE(leg.params, leg.byeToHeader, leg.byeRequestURI, cseq, branch)
	return m.send(msg, dst)
}

// InboundCallIDForEstablishedTransferBridge returns the DialRequest.CorrelationID (inbound PSTN Call-ID)
// for an established blind-transfer agent leg using MediaProfileTransferBridge.
func (m *Manager) InboundCallIDForEstablishedTransferBridge(outboundCallID string) (string, bool) {
	outboundCallID = strings.TrimSpace(outboundCallID)
	if m == nil || outboundCallID == "" {
		return "", false
	}
	leg := m.legByCallIDOrHostRewrite(outboundCallID)
	if leg == nil {
		return "", false
	}
	leg.mu.Lock()
	est := leg.established
	leg.mu.Unlock()
	if !est {
		return "", false
	}
	if leg.req.Scenario != ScenarioTransferAgent || leg.req.MediaProfile != MediaProfileTransferBridge {
		return "", false
	}
	in := strings.TrimSpace(leg.req.CorrelationID)
	if in == "" {
		return "", false
	}
	return in, true
}

// CleanupLegIfPresent removes outbound leg state (RTP + maps) when no longer needed, e.g. after remote BYE.
func (m *Manager) CleanupLegIfPresent(callID string) {
	if m == nil {
		return
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return
	}
	leg := m.legByCallIDOrHostRewrite(callID)
	if leg == nil {
		return
	}
	leg.cleanupLeg()
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	if a == nil {
		return nil
	}
	b := *a
	return &b
}
