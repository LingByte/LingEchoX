package persist

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/sip/stack"
)

func NewSIPCallRinging(callID, from, to, cseqInvite, remoteAddr, direction, remoteRTP, localRTP string, payloadType uint8, codec string, clockRate int, inviteAt time.Time) SIPCall {
	dir := strings.TrimSpace(direction)
	if dir == "" {
		dir = DirectionInbound
	}
	return SIPCall{
		CallID:        callID,
		FromHeader:    from,
		ToHeader:      to,
		CSeqInvite:    cseqInvite,
		RemoteAddr:    remoteAddr,
		Direction:     dir,
		RemoteRTPAddr: remoteRTP,
		LocalRTPAddr:  localRTP,
		PayloadType:   payloadType,
		Codec:         codec,
		ClockRate:     clockRate,
		State:         SIPCallStateRinging,
		InviteAt:      &inviteAt,
	}
}

func SIPCallInviteRefreshUpdateMap(from, to, remoteAddr, remoteRTP, localRTP, codec string, payloadType uint8, clockRate int, now time.Time) map[string]interface{} {
	return map[string]interface{}{
		"from_header":     from,
		"to_header":       to,
		"remote_addr":     remoteAddr,
		"remote_rtp_addr": remoteRTP,
		"local_rtp_addr":  localRTP,
		"codec":           codec,
		"payload_type":    payloadType,
		"clock_rate":      clockRate,
		"state":           SIPCallStateRinging,
		"updated_at":      now,
	}
}

func SIPCallEstablishedUpdateMap(now time.Time) map[string]interface{} {
	return map[string]interface{}{
		"state":      SIPCallStateEstablished,
		"ack_at":     now,
		"updated_at": now,
	}
}

func SIPCallEndStatusForBye(initiator string, hadSIPAgentTransfer, hadWebSeat bool) string {
	hadXfer := hadSIPAgentTransfer || hadWebSeat
	local := strings.EqualFold(strings.TrimSpace(initiator), "local")
	if hadXfer {
		if local {
			return SIPCallEndAfterTransferLocal
		}
		return SIPCallEndAfterTransferRemote
	}
	if local {
		return SIPCallEndCompletedLocal
	}
	return SIPCallEndCompletedRemote
}

func SIPCallDurationSince(ackAt, inviteAt *time.Time, end time.Time) int {
	var start time.Time
	if ackAt != nil && !ackAt.IsZero() {
		start = *ackAt
	} else if inviteAt != nil && !inviteAt.IsZero() {
		start = *inviteAt
	}
	if start.IsZero() {
		return 0
	}
	sec := int(end.Sub(start).Seconds())
	if sec < 0 {
		return 0
	}
	return sec
}

func SIPCallByeFinalizeUpdateMap(now time.Time, endStatus string, hadSIPTransfer, hadWebSeat bool, durationSec int) map[string]interface{} {
	return map[string]interface{}{
		"state":            SIPCallStateEnded,
		"bye_at":           now,
		"ended_at":         now,
		"updated_at":       now,
		"end_status":       endStatus,
		"had_sip_transfer": hadSIPTransfer,
		"had_web_seat":     hadWebSeat,
		"duration_sec":     durationSec,
	}
}

func ApplyRTPMediaToSIPCall(c *SIPCall, remoteIP string, remotePort int, localIP string, localPort int, codec string, pt uint8, clock int) {
	if c == nil || remoteIP == "" || remotePort <= 0 {
		return
	}
	c.RemoteRTPAddr = net.JoinHostPort(remoteIP, strconv.Itoa(remotePort))
	if localIP != "" && localPort > 0 {
		c.LocalRTPAddr = net.JoinHostPort(localIP, strconv.Itoa(localPort))
	}
	c.Codec = strings.ToLower(strings.TrimSpace(codec))
	c.PayloadType = pt
	c.ClockRate = clock
}

func SIPCallFromInboundInvite(req *stack.Message, peer *net.UDPAddr) *SIPCall {
	if req == nil {
		return nil
	}
	now := time.Now()
	nowPtr := &now
	callID := strings.TrimSpace(req.GetHeader("call-id"))
	c := &SIPCall{
		CallID:       callID,
		FromHeader:   req.GetHeader("from"),
		ToHeader:     req.GetHeader("to"),
		CSeqInvite:   req.GetHeader("cseq"),
		Direction:    DirectionInbound,
		State:        SIPCallStateInit,
		InviteAt:     nowPtr,
		RecordingURL: RecordingRelPathForCall(callID),
		EndStatus:    SIPCallEndUnknown,
	}
	if peer != nil {
		c.RemoteAddr = peer.String()
	}
	return c
}

func ApplyInboundInviteFailure(c *SIPCall, sipStatus int, reason string) {
	if c == nil {
		return
	}
	now := time.Now()
	c.State = SIPCallStateFailed
	c.EndedAt = &now
	c.FailureReason = reason
	switch sipStatus {
	case 486:
		c.EndStatus = SIPCallEndBusy
	case 487:
		c.EndStatus = SIPCallEndCancelled
	case 603:
		c.EndStatus = SIPCallEndDeclined
	default:
		if sipStatus >= 500 {
			c.EndStatus = SIPCallEndServerError
		} else {
			c.EndStatus = SIPCallEndDeclined
		}
	}
}
