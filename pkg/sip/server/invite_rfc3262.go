package server

import (
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/sip/protocol"
	"github.com/LingByte/SoulNexus/pkg/sip/stack"
	"github.com/LingByte/SoulNexus/pkg/sip/transaction"
	"go.uber.org/zap"
)

type inviteEnvConfig struct {
	RingbackMs    int
	Send180       bool
	Force100rel   bool
	EarlyMediaSDP bool
}

func parseInviteEnvConfig() inviteEnvConfig {
	c := inviteEnvConfig{Send180: true}
	if v := strings.TrimSpace(os.Getenv("SIP_INVITE_RINGBACK_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			c.RingbackMs = n
		}
	}
	if v := strings.TrimSpace(strings.ToLower(os.Getenv("SIP_INVITE_SEND_180"))); v == "0" || v == "false" || v == "off" || v == "no" {
		c.Send180 = false
	}
	if v := strings.TrimSpace(os.Getenv("SIP_INVITE_FORCE_100REL")); v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") {
		c.Force100rel = true
	}
	if v := strings.TrimSpace(os.Getenv("SIP_INVITE_EARLY_MEDIA_SDP")); v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") {
		c.EarlyMediaSDP = true
	}
	return c
}

func optionListRequires(list, opt string) bool {
	opt = strings.TrimSpace(strings.ToLower(opt))
	for _, p := range strings.Split(list, ",") {
		tok := strings.TrimSpace(strings.ToLower(p))
		if tok == "" {
			continue
		}
		if semi := strings.IndexByte(tok, ';'); semi >= 0 {
			tok = strings.TrimSpace(tok[:semi])
		}
		if tok == opt {
			return true
		}
	}
	return false
}

func require100rel(req *protocol.Message) bool {
	if req == nil {
		return false
	}
	for _, name := range []string{"Require", "Supported"} {
		if optionListRequires(strings.ToLower(req.GetHeader(name)), "100rel") {
			return true
		}
	}
	return false
}

func inviteFlightKey(req *protocol.Message) string {
	if req == nil {
		return ""
	}
	return transaction.TopBranch(req) + "\x00" + strings.TrimSpace(req.GetHeader("Call-ID"))
}

type inviteFlightState struct {
	mu sync.Mutex

	flightKey string
	callID    string

	lastProvRaw  string
	lastOK200Raw string
	completed    bool

	inviteCSeq int
	awaitRSeq  uint32
	prackDone  chan struct{}
}

func (s *SIPServer) inviteNeedsAsync(req *protocol.Message) bool {
	if s == nil || req == nil {
		return false
	}
	inv := s.inviteEnv
	reliable := inviteReliable(inv, req)
	return reliable || inv.RingbackMs > 0
}

func inviteReliable(inv inviteEnvConfig, req *protocol.Message) bool {
	return inv.Force100rel || require100rel(req) || inv.EarlyMediaSDP
}

func (s *SIPServer) resendInviteProgress(fl *inviteFlightState, addr *net.UDPAddr) {
	if s == nil || fl == nil || s.proto == nil || addr == nil {
		return
	}
	fl.mu.Lock()
	okRaw := fl.lastOK200Raw
	provRaw := fl.lastProvRaw
	done := fl.completed
	fl.mu.Unlock()
	if done && strings.TrimSpace(okRaw) != "" {
		if m, err := protocol.Parse(okRaw); err == nil && m != nil {
			_ = s.proto.Send(m, addr)
		}
		return
	}
	if strings.TrimSpace(provRaw) != "" {
		if m, err := protocol.Parse(provRaw); err == nil && m != nil {
			_ = s.proto.Send(m, addr)
		}
	}
}

func (s *SIPServer) inviteAsyncEnd(callID string) {
	if s == nil || callID == "" {
		return
	}
	v, ok := s.inviteByCall.LoadAndDelete(callID)
	if !ok {
		return
	}
	fl := v.(*inviteFlightState)
	if fl.flightKey != "" {
		s.inviteFlights.Delete(fl.flightKey)
	}
}

func (s *SIPServer) inviteFinalRetransmitCleanup(callID string) {
	if s == nil || callID == "" {
		return
	}
	if v, ok := s.inviteFlightKeyByCall.LoadAndDelete(callID); ok {
		if fk, _ := v.(string); fk != "" {
			s.inviteFinal200Raw.Delete(fk)
		}
	}
}

func (s *SIPServer) abortInviteFlight(flight *inviteFlightState, req *protocol.Message, addr *net.UDPAddr, toTag string) {
	if s == nil || flight == nil {
		return
	}
	cid := flight.callID
	if s.proto != nil && addr != nil && req != nil {
		t504 := s.makeResponse(req, 504, "Server Time-out", "", toTag)
		_ = s.proto.Send(t504, addr)
	}
	s.stopCallSessionLocked(cid)
	s.inviteAsyncEnd(cid)
}

func (s *SIPServer) stopCallSessionLocked(callID string) {
	if s == nil || callID == "" {
		return
	}
	s.mu.Lock()
	cs := s.callStore[callID]
	delete(s.callStore, callID)
	s.mu.Unlock()
	if cs != nil {
		cs.Stop()
	}
}

func (s *SIPServer) runInviteAsync(
	req *protocol.Message,
	addr *net.UDPAddr,
	flight *inviteFlightState,
	resp200 *protocol.Message,
	reliable bool,
	sdp183Body string,
	callID string,
) {
	if s == nil || req == nil || addr == nil || flight == nil || resp200 == nil {
		return
	}
	inv := s.inviteEnv
	toTag := resp200.GetHeader("To")

	if inv.Send180 && s.proto != nil {
		ring180 := s.makeResponse(req, 180, "Ringing", "", toTag)
		ring180.SetHeader("To", toTag)
		ring180.SetHeader("Contact", resp200.GetHeader("Contact"))
		ring180.SetHeader("Content-Length", "0")
		_ = s.proto.Send(ring180, addr)
	}

	if reliable {
		body := strings.TrimSpace(sdp183Body)
		ct := ""
		if body != "" {
			ct = "application/sdp"
		}
		p183 := s.makeResponse(req, 183, "Session Progress", body, toTag)
		p183.SetHeader("To", toTag)
		p183.SetHeader("Contact", resp200.GetHeader("Contact"))
		if ct != "" {
			p183.SetHeader("Content-Type", ct)
		}
		p183.SetHeader("RSeq", "1")
		p183.SetHeader("Require", "100rel")
		p183.SetHeader("Allow", resp200.GetHeader("Allow"))
		p183.SetHeader("Content-Length", strconv.Itoa(protocol.BodyBytesLen(body)))

		raw183 := p183.String()
		flight.mu.Lock()
		flight.awaitRSeq = 1
		flight.lastProvRaw = raw183
		flight.mu.Unlock()

		if s.proto != nil {
			_ = s.proto.Send(p183, addr)
		}

		timer := time.NewTimer(32 * time.Second)
		var badExit bool
		if s.sigCtx != nil {
			select {
			case <-flight.prackDone:
			case <-timer.C:
				badExit = true
				if logger.Lg != nil {
					logger.Lg.Warn("sip invite PRACK timeout", zap.String("call_id", callID))
				}
			case <-s.sigCtx.Done():
				badExit = true
			}
		} else {
			select {
			case <-flight.prackDone:
			case <-timer.C:
				badExit = true
				if logger.Lg != nil {
					logger.Lg.Warn("sip invite PRACK timeout", zap.String("call_id", callID))
				}
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		if badExit {
			s.abortInviteFlight(flight, req, addr, toTag)
			return
		}
		flight.mu.Lock()
		flight.awaitRSeq = 0
		flight.lastProvRaw = ""
		flight.mu.Unlock()
	}

	if inv.RingbackMs > 0 {
		t := time.NewTimer(time.Duration(inv.RingbackMs) * time.Millisecond)
		if s.sigCtx != nil {
			select {
			case <-t.C:
			case <-s.sigCtx.Done():
				if !t.Stop() {
					<-t.C
				}
				s.abortInviteFlight(flight, req, addr, toTag)
				return
			}
		} else {
			<-t.C
		}
	}

	okRaw := resp200.String()
	flight.mu.Lock()
	flight.lastOK200Raw = okRaw
	flight.completed = true
	flight.mu.Unlock()
	if flight.flightKey != "" {
		s.inviteFinal200Raw.Store(flight.flightKey, okRaw)
		s.inviteFlightKeyByCall.Store(callID, flight.flightKey)
	}

	if s.proto != nil {
		if err := s.proto.Send(resp200, addr); err != nil && logger.Lg != nil {
			logger.Lg.Warn("sip invite send 200", zap.String("call_id", callID), zap.Error(err))
		}
	}
}

func (s *SIPServer) handlePrack(msg *protocol.Message, addr *net.UDPAddr) *protocol.Message {
	if msg == nil || !msg.IsRequest || strings.ToUpper(msg.Method) != protocol.MethodPrack {
		return nil
	}
	callID := strings.TrimSpace(msg.GetHeader("Call-ID"))
	if callID == "" {
		return s.makeResponse(msg, 400, "Bad Request", "", "")
	}
	rseq, cseqNum, method, err := stack.ParseRAck(msg.GetHeader("RAck"))
	if err != nil || method != "INVITE" {
		return s.makeResponse(msg, 400, "Bad Request", "", "")
	}
	v, ok := s.inviteByCall.Load(callID)
	if !ok {
		return s.makeResponse(msg, 481, "Call/Transaction Does Not Exist", "", "")
	}
	fl := v.(*inviteFlightState)
	fl.mu.Lock()
	okMatch := fl.awaitRSeq != 0 && rseq == fl.awaitRSeq && cseqNum == fl.inviteCSeq
	fl.mu.Unlock()
	if !okMatch {
		return s.makeResponse(msg, 481, "Call/Transaction Does Not Exist", "", "")
	}
	select {
	case fl.prackDone <- struct{}{}:
	default:
	}
	return s.makeResponse(msg, 200, "OK", "", "")
}
