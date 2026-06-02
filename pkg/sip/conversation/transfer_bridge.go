package conversation

import (
	"strings"
	"sync"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/LinByte/VoiceServer/pkg/media/encoder"
	"github.com/LinByte/VoiceServer/pkg/sip/bridge"
	"github.com/LinByte/VoiceServer/pkg/sip/sipagentpoll"
	siprtp "github.com/LinByte/VoiceServer/pkg/sip/rtp"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

// rawRelayDecoderFor returns a payload→PCM16 decoder for narrowband G.711
// inbound legs. Raw relay only kicks in when both bridge legs share the
// same G.711 family (CanRawDatagramRelay), so feeding the inbound payload
// through one of these is enough to populate the stereo recorder. Other
// codecs are not expected on the raw relay path; nil disables recorder
// taps without breaking the bridge itself.
func rawRelayDecoderFor(c media.CodecConfig) func([]byte) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(c.Codec)) {
	case "pcmu":
		return encoder.DecodePCMU
	case "pcma":
		return encoder.DecodePCMA
	default:
		return nil
	}
}

// CallStore removes sessions from the SIP server map without invoking Stop (RTP already handled).
type CallStore interface {
	RemoveCallSession(callID string)
}

var (
	lookupInbound func(callID string) *sipSession.CallSession
	callStore     CallStore

	bridgeSendOutboundBYE func(callID string) error
	bridgeHangupInbound   func(callID string) error

	bridgeMu sync.Mutex
	bridges  map[string]*transferBridgeState // keyed by inbound or outbound Call-ID
)

// legBridge is PCM transcode or raw G.711 RTP relay.
type legBridge interface {
	Start()
	Stop()
}

type transferBridgeState struct {
	br         legBridge
	inboundID  string
	outboundID string
	inboundCS  *sipSession.CallSession
	outboundCS *sipSession.CallSession
}

// TransferBridgeByePersist carries inbound-leg recording + media metadata for sippersist.OnBye after a transfer bridge ends.
type TransferBridgeByePersist struct {
	InboundCallID      string
	OutboundCallID     string // agent/trunk leg (optional; used to drop outbound.Manager state after BYE)
	RawPayload         []byte
	CodecName          string
	Initiator          string
	RecordSampleRate   int
	RecordOpusChannels int
}

func normCallID(s string) string {
	return strings.TrimSpace(s)
}

// ResolveInboundCallIDForTransfer maps an inbound or outbound bridge Call-ID to the PSTN inbound leg.
func ResolveInboundCallIDForTransfer(callID string) string {
	callID = normCallID(callID)
	if callID == "" {
		return ""
	}
	if id, ok := PeekInboundTransferACDTargetID(callID); ok && id > 0 {
		return callID
	}
	bridgeMu.Lock()
	bs := findBridgeStateUnlocked(callID)
	bridgeMu.Unlock()
	if bs != nil {
		return bs.inboundID
	}
	return ""
}

// bridgeCallLocalPart returns the substring before '@' so we can match dialog Call-IDs when only the host differs (SBC).
func bridgeCallLocalPart(cid string) (string, bool) {
	cid = normCallID(cid)
	if cid == "" {
		return "", false
	}
	i := strings.LastIndex(cid, "@")
	if i <= 0 || i >= len(cid)-1 {
		return "", false
	}
	return cid[:i], true
}

// findBridgeStateUnlocked finds bs by exact map key or by outbound/inbound Call-ID local-part match (bridgeMu held).
func findBridgeStateUnlocked(callID string) *transferBridgeState {
	if bridges == nil {
		return nil
	}
	callID = normCallID(callID)
	if callID == "" {
		return nil
	}
	if bs := bridges[callID]; bs != nil {
		return bs
	}
	loc, ok := bridgeCallLocalPart(callID)
	if !ok {
		return nil
	}
	for _, bs := range bridges {
		if bs == nil {
			continue
		}
		if lo, oko := bridgeCallLocalPart(bs.outboundID); oko && lo == loc {
			return bs
		}
		if li, oki := bridgeCallLocalPart(bs.inboundID); oki && li == loc {
			return bs
		}
	}
	return nil
}

func transferBridgeLegHung(bs *transferBridgeState, hungCallID string) (inboundHung, outboundHung bool) {
	if bs == nil {
		return false, false
	}
	h := normCallID(hungCallID)
	if h == "" {
		return false, false
	}
	if h == bs.inboundID {
		return true, false
	}
	if h == bs.outboundID {
		return false, true
	}
	loc, ok := bridgeCallLocalPart(h)
	if !ok {
		return false, false
	}
	if li, ok2 := bridgeCallLocalPart(bs.inboundID); ok2 && li == loc {
		return true, false
	}
	if lo, ok3 := bridgeCallLocalPart(bs.outboundID); ok3 && lo == loc {
		return false, true
	}
	return false, false
}

func transferBridgePersistSnapshot(bs *transferBridgeState, initiator string) *TransferBridgeByePersist {
	if bs == nil || bs.inboundID == "" {
		return nil
	}
	p := &TransferBridgeByePersist{
		InboundCallID:  bs.inboundID,
		OutboundCallID: bs.outboundID,
		Initiator:      initiator,
	}
	if initiator == "" {
		p.Initiator = "remote"
	}
	if bs.inboundCS != nil {
		p.RawPayload = bs.inboundCS.TakeRecording()
		p.CodecName = bs.inboundCS.NegotiatedCodec().Name
		src := bs.inboundCS.SourceCodec()
		p.RecordSampleRate = src.SampleRate
		p.RecordOpusChannels = src.OpusDecodeChannels
		if p.RecordOpusChannels < 1 {
			p.RecordOpusChannels = src.Channels
		}
	}
	return p
}

// SetInboundSessionLookup resolves the inbound UAS CallSession by Call-ID (set from cmd/sip).
func SetInboundSessionLookup(fn func(string) *sipSession.CallSession) {
	lookupInbound = fn
}

// SetCallStore removes call entries when a transfer bridge ends (set from cmd/sip).
func SetCallStore(cs CallStore) {
	callStore = cs
}

// SetTransferPeerCallbacks wires BYE to the peer leg when one side hangs up (outbound Manager.SendBYE, server SendUASBye).
func SetTransferPeerCallbacks(sendOutboundBYE func(callID string) error, hangupInboundRemote func(callID string) error) {
	bridgeSendOutboundBYE = sendOutboundBYE
	bridgeHangupInbound = hangupInboundRemote
}

// ActiveTransferBridgeForCallID is true when this Call-ID (inbound or outbound) is in an active media bridge.
func ActiveTransferBridgeForCallID(callID string) bool {
	callID = normCallID(callID)
	if callID == "" {
		return false
	}
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	if bridges == nil {
		return false
	}
	return findBridgeStateUnlocked(callID) != nil
}

// MigrateTransferBridgeOutboundCallID rekeys the active transfer bridge when the outbound dialog Call-ID
// changes after 200 OK (SBC rewrite). Safe no-op if no bridge or IDs do not match.
func MigrateTransferBridgeOutboundCallID(inbound, oldID, newID string) {
	inbound, oldID, newID = normCallID(inbound), normCallID(oldID), normCallID(newID)
	if inbound == "" || oldID == "" || newID == "" || oldID == newID {
		return
	}
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	if bridges == nil {
		return
	}
	bs, ok := bridges[oldID]
	if !ok || bs == nil || bs.inboundID != inbound || bs.outboundID != oldID {
		return
	}
	delete(bridges, oldID)
	bs.outboundID = newID
	bridges[newID] = bs
	bridges[inbound] = bs
}

// TeardownTransferBridgeOnOutboundRemoteByeFallback ends the PSTN inbound leg when the agent/trunk side
// sent BYE but the bridge map had no entry for outboundCallID (e.g. Call-ID drift). Idempotent with callStore.RemoveCallSession.
func TeardownTransferBridgeOnOutboundRemoteByeFallback(inboundCallID, outboundCallID string) *TransferBridgeByePersist {
	inboundCallID = normCallID(inboundCallID)
	outboundCallID = normCallID(outboundCallID)
	if inboundCallID == "" || outboundCallID == "" {
		return nil
	}
	var inboundCS *sipSession.CallSession
	if lookupInbound != nil {
		inboundCS = lookupInbound(inboundCallID)
	}
	persist := transferBridgePersistSnapshot(&transferBridgeState{
		inboundID:  inboundCallID,
		outboundID: outboundCallID,
		inboundCS:  inboundCS,
	}, "remote")
	if bridgeHangupInbound != nil {
		if err := bridgeHangupInbound(inboundCallID); err != nil && logger.Lg != nil {
			logger.Lg.Warn("sip transfer bridge: fallback inbound BYE failed",
				zap.String("inbound_call_id", inboundCallID),
				zap.String("outbound_call_id", outboundCallID),
				zap.Error(err),
			)
		}
	} else if logger.Lg != nil {
		logger.Lg.Warn("sip transfer bridge: bridgeHangupInbound not wired; inbound leg may leak (fallback)",
			zap.String("inbound_call_id", inboundCallID),
			zap.String("outbound_call_id", outboundCallID),
		)
	}
	if inboundCS != nil {
		inboundCS.CloseRTPOnly()
	}
	if callStore != nil {
		callStore.RemoveCallSession(inboundCallID)
		callStore.RemoveCallSession(outboundCallID)
	}
	if logger.Lg != nil {
		logger.Lg.Info("sip transfer bridge ended (outbound bye map miss fallback)",
			zap.String("inbound_call_id", inboundCallID),
			zap.String("outbound_call_id", outboundCallID),
		)
	}
	return persist
}

// StartTransferBridge stops AI media on both legs and bridges audio.
// When TrunkNumber transfer brief templates are set, caller and agent hear TTS
// (hold music stops first; same text syncs via one pipeline, different text in parallel).
func StartTransferBridge(inboundCallID string, outboundCS *sipSession.CallSession, outboundCallID string, lg *zap.Logger) {
	inboundCallID = normCallID(inboundCallID)
	outboundCallID = normCallID(outboundCallID)
	if lg == nil && logger.Lg != nil {
		lg = logger.Lg
	}
	if lg == nil {
		lg = zap.NewNop()
	}
	if hasTransferBriefConfigured(inboundCallID) {
		if _, loaded := transferAgentBriefRunning.LoadOrStore(inboundCallID, struct{}{}); loaded {
			return
		}
		go func() {
			defer transferAgentBriefRunning.Delete(inboundCallID)
			playTransferAgentBriefThenBridge(inboundCallID, outboundCS, outboundCallID, lg)
		}()
		return
	}
	startTransferBridgeNow(inboundCallID, outboundCS, outboundCallID, lg)
}

// startTransferBridgeNow performs hold-music stop and bidirectional media bridge.
func startTransferBridgeNow(inboundCallID string, outboundCS *sipSession.CallSession, outboundCallID string, lg *zap.Logger) {
	inboundCallID = normCallID(inboundCallID)
	outboundCallID = normCallID(outboundCallID)
	stopTransferRinging(inboundCallID)
	cancelTransferInviteWatch(outboundCallID)
	transferExcludeReset(inboundCallID)
	if lg == nil && logger.Lg != nil {
		lg = logger.Lg
	}
	if lg == nil {
		lg = zap.NewNop()
	}
	if lookupInbound == nil {
		lg.Warn("sip transfer bridge: SetInboundSessionLookup not configured")
		if outboundCS != nil {
			outboundCS.Stop()
		}
		if callStore != nil && outboundCallID != "" {
			callStore.RemoveCallSession(outboundCallID)
		}
		return
	}
	inbound := lookupInbound(inboundCallID)
	if inbound == nil {
		// 200 OK / CANCEL race: PSTN 用户挂断、我们已发 CANCEL，但坐席
		// 那条 leg 同时 200 OK 进来 —— 入局 session 已被清，桥接做不成。
		// 此时 outbound leg 已经 established（200 OK + ACK），必须发
		// 标准 in-dialog BYE 把坐席手机挂掉；只 outboundCS.Stop() 仅停本地
		// 媒体，坐席端会一直在通话状态等到自己挂或 RTP idle timeout。
		lg.Warn("sip transfer bridge: inbound session gone after 200 OK (race vs CANCEL/hangup)",
			zap.String("inbound_call_id", inboundCallID),
			zap.String("outbound_call_id", outboundCallID))
		if bridgeSendOutboundBYE != nil && outboundCallID != "" {
			if err := bridgeSendOutboundBYE(outboundCallID); err != nil {
				lg.Warn("sip transfer bridge: BYE on race fallback failed",
					zap.String("outbound_call_id", outboundCallID),
					zap.Error(err))
			}
		}
		if outboundCS != nil {
			outboundCS.Stop()
		}
		if callStore != nil && outboundCallID != "" {
			callStore.RemoveCallSession(outboundCallID)
		}
		return
	}
	if outboundCS == nil {
		lg.Warn("sip transfer bridge: nil outbound session")
		return
	}

	inbound.StopMediaPreserveRTP()
	outboundCS.StopMediaPreserveRTP()

	ccIn := inbound.SourceCodec()
	ccOut := outboundCS.SourceCodec()
	var br legBridge
	var err error
	var mode string
	var pcmReason string
	rawOK := bridge.CanRawDatagramRelay(ccIn, ccOut)
	if rawOK {
		br, err = bridge.NewTwoLegPayloadRelay(
			inbound.RTPSession(), outboundCS.RTPSession(),
			ccIn, ccOut,
			inbound.DTMFPayloadType(), outboundCS.DTMFPayloadType(),
		)
		if err != nil {
			pcmReason = "raw_relay_error: " + err.Error()
			lg.Warn("sip transfer bridge: raw relay failed, falling back to pcm", zap.Error(err))
			br = nil
		} else {
			mode = "raw_rtp_forward"
			if relay, ok := br.(*bridge.TwoLegPayloadRelay); ok {
				// 双通道喂数据：
				//  1) 旧版 SN3 blob —— 兼容历史 SN3→WAV 解码回退路径。
				//  2) 新版立体声 recorder —— 把 G.711 RTP payload 解码成 PCM16
				//     喂进 WriteCallerPCM / WriteAIPCM，否则 OnBye 时 WAV 里
				//     桥接后整段都是静音（recorder 不消化 SN3 raw bytes）。
				//     raw relay 只在双方都是 narrowband G.711 时启用，所以
				//     用 inbound 侧码方解码即可，采样率与 recorder 配置一致。
				dec := rawRelayDecoderFor(ccIn)
				relay.SetInboundRecording(
					func(seq uint16, ts uint32, p []byte) {
						inbound.AppendRecordingSample(sipSession.RecordingDirUser, seq, ts, p)
						if dec != nil {
							if pcm, derr := dec(p); derr == nil {
								inbound.WriteCallerPCM(pcm)
							}
						}
					},
					func(seq uint16, ts uint32, p []byte) {
						inbound.AppendRecordingSample(sipSession.RecordingDirAI, seq, ts, p)
						if dec != nil {
							if pcm, derr := dec(p); derr == nil {
								inbound.WriteAIPCM(pcm)
							}
						}
					},
				)
			}
		}
	} else {
		pcmReason = "codecs_not_eligible_for_raw_relay"
	}
	if br == nil {
		callerRx := siprtp.NewSIPRTPTransport(inbound.RTPSession(), ccIn, media.DirectionInput, inbound.DTMFPayloadType())
		callerTx := siprtp.NewSIPRTPTransport(inbound.RTPSession(), ccIn, media.DirectionOutput, 0)
		inbound.WireTransferBridgeRecording(callerRx, callerTx)
		agentRx := siprtp.NewSIPRTPTransport(outboundCS.RTPSession(), ccOut, media.DirectionInput, outboundCS.DTMFPayloadType())
		agentTx := siprtp.NewSIPRTPTransport(outboundCS.RTPSession(), ccOut, media.DirectionOutput, 0)
		br, err = bridge.NewTwoLegPCMBridge(callerRx, callerTx, agentRx, agentTx)
		if err != nil {
			lg.Warn("sip transfer bridge: build failed", zap.Error(err))
			inbound.CloseRTPOnly()
			outboundCS.CloseRTPOnly()
			if callStore != nil {
				callStore.RemoveCallSession(inboundCallID)
				callStore.RemoveCallSession(outboundCallID)
			}
			return
		}
		mode = "pcm_transcode"
		// 把转接桥接后的双向 PCM 同步喂进新版立体声录音器：
		// 否则 OnBye 时优先采用 voice/recorder 产出的 WAV，桥接后的人工对话
		// 只会在 SN3 raw 字节里，最终 WAV 在转接之后就静音了。
		// PCM transcode mid 采样率与 inbound.PCMSampleRate() 一致：
		//  - 双方 G.711  → 8kHz；
		//  - 否则（典型 Opus↔PCMU）→ 16kHz。
		// recorder 在 EnableRecorder 时同样按 cs.pcmSampleRate 配置，无需重采样。
		if pcmBr, ok := br.(*bridge.TwoLegPCMBridge); ok {
			pcmBr.SetDirectionalPCMTap(func(dir bridge.BridgeDirection, pcm []byte) {
				switch dir {
				case bridge.DirectionCallerToAgent:
					inbound.WriteCallerPCM(pcm)
				case bridge.DirectionAgentToCaller:
					inbound.WriteAIPCM(pcm)
				}
			})
		}
	}

	bs := &transferBridgeState{
		br:         br,
		inboundID:  inboundCallID,
		outboundID: outboundCallID,
		inboundCS:  inbound,
		outboundCS: outboundCS,
	}
	bridgeMu.Lock()
	if bridges == nil {
		bridges = make(map[string]*transferBridgeState)
	}
	bridges[inboundCallID] = bs
	bridges[outboundCallID] = bs
	bridgeMu.Unlock()

	br.Start()

	logFields := []zap.Field{
		zap.String("inbound_call_id", inboundCallID),
		zap.String("outbound_call_id", outboundCallID),
		zap.String("mode", mode),
		zap.String("in_codec", ccIn.Codec),
		zap.String("out_codec", ccOut.Codec),
	}
	if strings.EqualFold(strings.TrimSpace(ccIn.Codec), "opus") {
		logFields = append(logFields, zap.Int("in_opus_decode_ch", ccIn.OpusDecodeChannels))
	}
	if mode == "pcm_transcode" && pcmReason != "" {
		logFields = append(logFields, zap.String("pcm_reason", pcmReason))
	}
	lg.Info("sip transfer bridge started", logFields...)
	if id, ok := PeekInboundTransferACDTargetID(inboundCallID); ok && id > 0 {
		RecordTransferAnswered(inboundCallID, id)
	}
	markTransferACDWorkStateForCall(inboundCallID, "busy")
	sipagentpoll.MarkInboundConnected(inboundCallID)
	MarkInboundHadSIPAgentTransfer(inboundCallID)
	notifyTransferPhase(inboundCallID, TransferPhaseConnected, map[string]any{
		"outbound_call_id": outboundCallID,
		"bridge_mode":      mode,
	})
}

func hangPeerIfNeeded(bs *transferBridgeState, hungCallID string) {
	if bs == nil {
		return
	}
	inboundHung, outboundHung := transferBridgeLegHung(bs, hungCallID)
	if inboundHung {
		if bridgeSendOutboundBYE != nil {
			if err := bridgeSendOutboundBYE(bs.outboundID); err != nil && logger.Lg != nil {
				logger.Lg.Warn("sip transfer bridge: send BYE to outbound peer failed",
					zap.String("inbound_call_id", bs.inboundID),
					zap.String("outbound_call_id", bs.outboundID),
					zap.Error(err),
				)
			}
		} else if logger.Lg != nil {
			logger.Lg.Warn("sip transfer bridge: bridgeSendOutboundBYE not wired; outbound leg will leak",
				zap.String("outbound_call_id", bs.outboundID),
			)
		}
		return
	}
	if outboundHung {
		if bridgeHangupInbound != nil {
			if err := bridgeHangupInbound(bs.inboundID); err != nil && logger.Lg != nil {
				logger.Lg.Warn("sip transfer bridge: send BYE to inbound peer failed",
					zap.String("inbound_call_id", bs.inboundID),
					zap.String("outbound_call_id", bs.outboundID),
					zap.Error(err),
				)
			}
		} else if logger.Lg != nil {
			logger.Lg.Warn("sip transfer bridge: bridgeHangupInbound not wired; inbound leg will leak",
				zap.String("inbound_call_id", bs.inboundID),
			)
		}
	}
}

func teardownBridge(bs *transferBridgeState) {
	if bs == nil {
		return
	}
	if bs.br != nil {
		bs.br.Stop()
	}
	if bs.inboundCS != nil {
		bs.inboundCS.CloseRTPOnly()
	}
	if bs.outboundCS != nil {
		bs.outboundCS.CloseRTPOnly()
	}
	if callStore != nil {
		callStore.RemoveCallSession(bs.inboundID)
		callStore.RemoveCallSession(bs.outboundID)
	}
}

// HangupTransferBridgeIfAny stops bridging and RTP for both legs when either Call-ID hangs up.
// Notifies the peer leg with BYE before local teardown when callbacks are wired.
// When non-nil, the caller must run sippersist.OnBye once for TransferBridgeByePersist.InboundCallID (BYE arrived on this stack).
func HangupTransferBridgeIfAny(callID string) *TransferBridgeByePersist {
	callID = normCallID(callID)
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	if bridges == nil {
		return nil
	}
	bs := findBridgeStateUnlocked(callID)
	if bs == nil {
		return nil
	}
	persist := transferBridgePersistSnapshot(bs, "remote")
	hangPeerIfNeeded(bs, callID)
	delete(bridges, bs.inboundID)
	delete(bridges, bs.outboundID)
	teardownBridge(bs)
	if logger.Lg != nil {
		logger.Lg.Info("sip transfer bridge ended", zap.String("hangup_call_id", callID),
			zap.String("inbound_call_id", bs.inboundID), zap.String("outbound_call_id", bs.outboundID))
	}
	return persist
}

// HangupTransferBridgeFull tears down an active transfer bridge and BYE both SIP legs (e.g. keyword hangup).
// When non-nil, the caller must run sippersist.OnBye for TransferBridgeByePersist.InboundCallID with initiator "local".
func HangupTransferBridgeFull(callID string) *TransferBridgeByePersist {
	callID = normCallID(callID)
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	if bridges == nil {
		return nil
	}
	bs := findBridgeStateUnlocked(callID)
	if bs == nil {
		return nil
	}
	persist := transferBridgePersistSnapshot(bs, "local")
	if bridgeSendOutboundBYE != nil {
		_ = bridgeSendOutboundBYE(bs.outboundID)
	}
	if bridgeHangupInbound != nil {
		_ = bridgeHangupInbound(bs.inboundID)
	}
	delete(bridges, bs.inboundID)
	delete(bridges, bs.outboundID)
	teardownBridge(bs)
	if logger.Lg != nil {
		logger.Lg.Info("sip transfer bridge full hangup", zap.String("trigger_call_id", callID),
			zap.String("inbound_call_id", bs.inboundID), zap.String("outbound_call_id", bs.outboundID))
	}
	return persist
}
