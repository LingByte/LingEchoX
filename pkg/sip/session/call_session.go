package session

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/LinByte/VoiceServer/pkg/media/encoder"
	"github.com/LinByte/VoiceServer/pkg/sip/rtp"
	"github.com/LinByte/VoiceServer/pkg/sip/sdp"
	"github.com/LinByte/VoiceServer/pkg/voice/gateway"
	"github.com/LinByte/VoiceServer/pkg/voice/recorder"
	"go.uber.org/zap"
)

// SIP recording blob format v3 (sippersist): magic "SN3" then repeated
// [dir u8][seq u16LE][rtpTs u32LE][wallNs u64LE][len u16LE][payload].
// wallNs is nanoseconds since the first captured frame (time.Since anchor): restores real gaps between
// TTS phrases when RTP timestamps stay continuous across silence (unlike SN2 RTP-only placement).
// Legacy "SN2" blobs remain readable in pkg/utils/sip_recording_wav.go.
const recBlobMagic = "SN3"

const (
	recDirUser = 0
	recDirAI   = 1
)

// RecordingDirUser / RecordingDirAI match SN3 dir bytes for AppendRecordingSample (e.g. SIP transfer raw relay).
const (
	RecordingDirUser = recDirUser
	RecordingDirAI   = recDirAI
)

// CallSession binds an RTP session to a MediaSession for SIP calls.
//
// Uplink: RTP -> decode -> PCM for ASR processors.
// Downlink: only synthesized (TTS) PCM is encoded and sent as RTP; uplink is not echoed
// (see media.KeySIPSuppressUplinkEcho).
type CallSession struct {
	CallID        string
	rtpSess       *rtp.Session
	media         *media.MediaSession
	neg           sdp.Codec
	rxTransport   *rtp.SIPRTPTransport // RTP transports and codec (same as used for MediaSession) for handoff to in-process PCM bridge.
	txTransport   *rtp.SIPRTPTransport
	srcCodec      media.CodecConfig
	pcmSampleRate int // internal PCM bridge rate (matches InternalPCMSampleRate(src))
	dtmfPT        uint8
	ctx           context.Context
	cancel        context.CancelFunc
	startOnce     sync.Once
	ackOnce       sync.Once // For SIP: media starts on ACK, not on INVITE.
	voiceMu       sync.Mutex
	voiceAttached bool
	recMu         sync.Mutex
	recBuf        []byte
	recTimeOrigin time.Time // first appendRecordingFrame sets anchor for wallNs (monotonic via time.Since)

	// New stereo PCM recorder, ported from VoiceServer pkg/voice/recorder.
	// Optional: if nil, recording falls back to the legacy SN3 blob path
	// above. When configured (via EnableRecorder), the recorder captures
	// already-decoded PCM frames with wall-clock alignment and produces
	// a stereo WAV directly at flush time — bypassing the SN3 → WAV
	// post-processing step that used to live in pkg/utils.
	rec *recorder.Recorder
}

// NewCallSession creates a call session with codec negotiation from SDP.
func NewCallSession(callID string, rtpSess *rtp.Session, sdpCodecs []sdp.Codec) (*CallSession, error) {
	if callID == "" {
		return nil, fmt.Errorf("sip: empty callID")
	}
	if rtpSess == nil {
		return nil, fmt.Errorf("sip: nil rtp session")
	}
	if len(sdpCodecs) == 0 {
		return nil, fmt.Errorf("sip: empty sdp codecs")
	}
	preferredCodecs := map[string]int{
		// Prefer narrowband G.711 A-law first for best PSTN/carrier interoperability.
		// Order matters when multiple codecs are offered; we pick the first supported.
		"pcma": 0,
		"pcmu": 1,
		"g722": 2,
		"opus": 3,
	}
	codecs := make([]sdp.Codec, len(sdpCodecs))
	copy(codecs, sdpCodecs)
	sort.SliceStable(codecs, func(i, j int) bool {
		ci := strings.ToLower(strings.TrimSpace(codecs[i].Name))
		cj := strings.ToLower(strings.TrimSpace(codecs[j].Name))
		ri, okI := preferredCodecs[ci]
		rj, okJ := preferredCodecs[cj]
		if !okI {
			ri = 100
		}
		if !okJ {
			rj = 100
		}
		return ri < rj
	})

	// Choose the first supported codec by preference.
	var src media.CodecConfig
	negotiatedPayloadType := uint8(0)
	var negotiatedSDP sdp.Codec
	found := false
	for _, c := range codecs {
		switch c.Name {
		case "pcmu", "pcma":
			found = true
			negotiatedPayloadType = c.PayloadType
			negotiatedSDP = c
			negotiatedSDP.Channels = 1
			src = media.CodecConfig{
				Codec:         c.Name, // "pcmu" or "pcma"
				SampleRate:    c.ClockRate,
				Channels:      1,
				BitDepth:      8, // PCMU/PCMA payload is 8-bit
				PayloadType:   negotiatedPayloadType,
				FrameDuration: "20ms",
			}
			break
		case "g722":
			found = true
			negotiatedPayloadType = c.PayloadType
			negotiatedSDP = c
			negotiatedSDP.Channels = 1
			src = media.CodecConfig{
				Codec:         "g722",
				SampleRate:    16000,
				Channels:      1,
				BitDepth:      16,
				PayloadType:   negotiatedPayloadType,
				FrameDuration: "20ms",
			}
			break
		case "opus":
			found = true
			negotiatedPayloadType = c.PayloadType
			decodeCh := c.Channels
			if decodeCh < 1 {
				decodeCh = 1
			}
			if decodeCh > 2 {
				decodeCh = 2
			}
			negotiatedSDP = c
			// 200 OK SDP must match offered channel count (e.g. OPUS/48000/2). Answering /1 while
			// the peer sends stereo RTP breaks several stacks; we still encode TTS mono (Channels:1).
			negotiatedSDP.Channels = decodeCh
			src = media.CodecConfig{
				Codec:              "opus",
				SampleRate:         c.ClockRate, // typically 48000
				Channels:           1,
				OpusDecodeChannels: decodeCh,
				BitDepth:           16,
				PayloadType:        negotiatedPayloadType,
				FrameDuration:      "20ms",
			}
			break
		}
		if found {
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("sip: unsupported codec (need one of: opus/g722/pcmu/pcma)")
	}

	pcmSR := InternalPCMSampleRate(src)
	pcm := media.CodecConfig{
		Codec:         "pcm",
		SampleRate:    pcmSR,
		Channels:      1,
		BitDepth:      16,
		FrameDuration: "",
	}

	dec, err := encoder.CreateDecode(src, pcm)
	if err != nil {
		return nil, fmt.Errorf("sip: CreateDecode failed: %w", err)
	}
	dec = passthroughDTMFDecode(dec)
	enc, err := encoder.CreateEncode(src, pcm)
	if err != nil {
		return nil, fmt.Errorf("sip: CreateEncode failed: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	dtmfPT := telephoneEventPayloadType(sdpCodecs)
	cs := &CallSession{
		CallID:        callID,
		rtpSess:       rtpSess,
		neg:           negotiatedSDP,
		srcCodec:      src,
		pcmSampleRate: pcmSR,
		dtmfPT:        dtmfPT,
		ctx:           ctx,
		cancel:        cancel,
	}
	rxTransport := rtp.NewSIPRTPTransport(rtpSess, src, media.DirectionInput, dtmfPT)
	rxTransport.JitterPlaybackDelay = rtp.DefaultJitterPlaybackDelay
	rxTransport.OnInputRTP = func(seq uint16, ts uint32, p []byte) { cs.appendRecordingFrame(recDirUser, seq, ts, p) }
	txTransport := rtp.NewSIPRTPTransport(rtpSess, src, media.DirectionOutput, 0)
	txTransport.OnOutputRTP = func(seq uint16, ts uint32, p []byte) { cs.appendRecordingFrame(recDirAI, seq, ts, p) }
	cs.rxTransport = rxTransport
	cs.txTransport = txTransport

	ms := media.NewDefaultSession().Context(ctx).SetSessionID("sip-call-" + callID)
	ms.QueueSize = config.MediaTxQueueSizeFromEnv()
	ms.Decode(dec).
		Encode(enc).
		Input(rxTransport).
		Output(txTransport)
	ms.Set(media.KeySIPSuppressUplinkEcho, true)
	ms.MaxSessionDuration = config.MediaMaxSecondsFromEnv()
	cs.media = ms

	return cs, nil
}

// MediaSession exposes the underlying media pipeline for voice processors (ASR/TTS hooks).
func (cs *CallSession) MediaSession() *media.MediaSession {
	if cs == nil {
		return nil
	}
	return cs.media
}

// AttachVoiceConversation runs fn once before media Serve() (typically from ACK) to register
// processors or other hooks. If fn fails, a later call may retry.
func (cs *CallSession) AttachVoiceConversation(fn func() error) error {
	if cs == nil || fn == nil {
		return nil
	}
	cs.voiceMu.Lock()
	defer cs.voiceMu.Unlock()
	if cs.voiceAttached {
		logger.Debug("sip session: voice attach skipped (already attached; often duplicate ACK)",
			zap.String("call_id", cs.CallID),
		)
		return nil
	}
	if err := fn(); err != nil {
		return err
	}
	cs.voiceAttached = true
	return nil
}

func passthroughDTMFDecode(dec media.EncoderFunc) media.EncoderFunc {
	return func(p media.MediaPacket) ([]media.MediaPacket, error) {
		if _, ok := p.(*media.DTMFPacket); ok {
			return []media.MediaPacket{p}, nil
		}
		return dec(p)
	}
}

func telephoneEventPayloadType(codecs []sdp.Codec) uint8 {
	for _, c := range codecs {
		if strings.EqualFold(strings.TrimSpace(c.Name), "telephone-event") {
			return c.PayloadType
		}
	}
	return 0
}

func (cs *CallSession) NegotiatedCodec() sdp.Codec {
	if cs == nil {
		return sdp.Codec{}
	}
	return cs.neg
}

// RTPSession returns the underlying RTP/UDP session (for building a transfer bridge).
func (cs *CallSession) RTPSession() *rtp.Session {
	if cs == nil {
		return nil
	}
	return cs.rtpSess
}

// SourceCodec is the negotiated RTP codec (PCMU/PCMA/G722/OPUS) for this leg.
func (cs *CallSession) SourceCodec() media.CodecConfig {
	if cs == nil {
		return media.CodecConfig{}
	}
	return cs.srcCodec
}

// PCMSampleRate is the internal mono PCM rate produced by RTP decode (and fed to ASR processors).
func (cs *CallSession) PCMSampleRate() int {
	if cs == nil || cs.pcmSampleRate <= 0 {
		return 16000
	}
	return cs.pcmSampleRate
}

// DTMFPayloadType is the negotiated telephone-event PT, or 0 if none.
func (cs *CallSession) DTMFPayloadType() uint8 {
	if cs == nil {
		return 0
	}
	return cs.dtmfPT
}

// StopMediaPreserveRTP stops the MediaSession (AI pipeline, RTP read/write loops) but keeps the UDP
// socket open so new SIPRTPTransport instances can attach for bridging.
func (cs *CallSession) StopMediaPreserveRTP() {
	if cs == nil {
		return
	}
	if cs.rxTransport != nil {
		cs.rxTransport.PreserveSessionOnClose = true
	}
	if cs.txTransport != nil {
		cs.txTransport.PreserveSessionOnClose = true
	}
	// With PreserveSessionOnClose, Transport.Close() does not close the UDP socket, so a goroutine
	// blocked in ReceiveRTP would otherwise keep running. The transfer bridge then reads the same
	// socket and two readers split packets → noise. Wake the blocked read before tearing down media.
	if cs.rtpSess != nil && cs.rtpSess.Conn != nil {
		_ = cs.rtpSess.Conn.SetReadDeadline(time.Now())
	}
	if cs.cancel != nil {
		cs.cancel()
	}
	if cs.media != nil {
		_ = cs.media.Close()
		// Do not hand the RTP socket to the transfer bridge until MediaSession transport goroutines
		// have stopped calling ReadFromUDP — two readers on one UDP socket steal packets.
		drainCtx, drainCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = cs.media.WaitServeShutdown(drainCtx)
		drainCancel()
	}
	// The wakeup above leaves a past deadline on the conn; the next Read (transfer bridge) would
	// otherwise return i/o timeout immediately and silence audio. Clear the deadline for new readers.
	if cs.rtpSess != nil && cs.rtpSess.Conn != nil {
		_ = cs.rtpSess.Conn.SetReadDeadline(time.Time{})
	}
}

// CloseRTPOnly closes the RTP UDP socket after a bridge or full teardown path.
func (cs *CallSession) CloseRTPOnly() {
	if cs == nil || cs.rtpSess == nil {
		return
	}
	_ = cs.rtpSess.Close()
	cs.rtpSess = nil
}

// Start starts MediaSession serving in background.
func (cs *CallSession) Start() {
	if cs == nil || cs.media == nil {
		return
	}
	cs.startOnce.Do(func() {
		cs.media.NotifyServeStarting()
		go func() {
			_ = cs.media.Serve()
		}()
	})
}

// StartOnACK starts media pipeline once (idempotent) when ACK is received.
func (cs *CallSession) StartOnACK() {
	if cs == nil {
		return
	}
	cs.ackOnce.Do(func() {
		cs.Start()
	})
}

// Stop stops the session and closes underlying RTP resources.
func (cs *CallSession) Stop() {
	if cs == nil {
		return
	}
	if cs.cancel != nil {
		cs.cancel()
	}
	if cs.media != nil {
		_ = cs.media.Close()
	}
	if cs.rtpSess != nil {
		_ = cs.rtpSess.Close()
		cs.rtpSess = nil
	}
}

// EnableRecorder configures the new stereo PCM recorder for this call.
//
// cfg.SampleRate is overridden with the call's negotiated PCM bridge rate
// so callers can pass a half-filled Config; cfg.CallID is forced to the
// session's CallID for the same reason. Pass cfg.ChunkInterval > 0 to
// enable rolling partial uploads as a crash-safety net.
//
// Returns true when the recorder was successfully created. Idempotent
// across repeated calls with the same configuration: if a recorder is
// already attached, this is a no-op and returns true.
func (cs *CallSession) EnableRecorder(cfg recorder.Config) bool {
	if cs == nil {
		return false
	}
	cs.recMu.Lock()
	defer cs.recMu.Unlock()
	if cs.rec != nil {
		return true
	}
	cfg.CallID = cs.CallID
	cfg.SampleRate = cs.pcmSampleRate
	if cfg.SampleRate <= 0 {
		return false
	}
	cfg.Transport = "sip"
	if cfg.Codec == "" {
		cfg.Codec = cs.neg.Name
	}
	r := recorder.New(cfg)
	if r == nil {
		return false
	}
	cs.rec = r
	return true
}

// HasRecorder reports whether the new stereo PCM recorder is attached.
func (cs *CallSession) HasRecorder() bool {
	if cs == nil {
		return false
	}
	cs.recMu.Lock()
	defer cs.recMu.Unlock()
	return cs.rec != nil
}

// WriteCallerPCM records one mono PCM16 LE frame from the caller side.
// Caller must own the slice (recorder copies internally).
// Sample rate must match the bridge rate set by EnableRecorder.
// No-op when the recorder is not attached.
func (cs *CallSession) WriteCallerPCM(pcm []byte) {
	if cs == nil || len(pcm) == 0 {
		return
	}
	cs.recMu.Lock()
	r := cs.rec
	cs.recMu.Unlock()
	if r == nil {
		return
	}
	r.WriteCaller(pcm)
}

// WriteAIPCM records one mono PCM16 LE frame from the AI / TTS side.
// Caller must own the slice (recorder copies internally).
// No-op when the recorder is not attached.
func (cs *CallSession) WriteAIPCM(pcm []byte) {
	if cs == nil || len(pcm) == 0 {
		return
	}
	cs.recMu.Lock()
	r := cs.rec
	cs.recMu.Unlock()
	if r == nil {
		return
	}
	r.WriteAI(pcm)
}

// FlushRecorder finalises the recording and uploads the canonical stereo
// WAV. Returns (RecordingInfo, true) on success, (zero, false) when the
// recorder is not attached or upload failed (consult logs). Idempotent —
// repeated calls return false after the first success.
func (cs *CallSession) FlushRecorder(ctx context.Context) (gateway.RecordingInfo, bool) {
	if cs == nil {
		return gateway.RecordingInfo{}, false
	}
	cs.recMu.Lock()
	r := cs.rec
	cs.rec = nil
	cs.recMu.Unlock()
	if r == nil {
		return gateway.RecordingInfo{}, false
	}
	return r.Flush(ctx)
}

// AppendRecordingSample appends one RTP payload to the SN3 blob (used during transfer bridge when
// media bypasses the original rx/tx transports).
func (cs *CallSession) AppendRecordingSample(dir byte, seq uint16, ts uint32, payload []byte) {
	cs.appendRecordingFrame(dir, seq, ts, payload)
}

// WireTransferBridgeRecording attaches SN3 callbacks to PCM-bridge transports sharing inbound RTP session.
func (cs *CallSession) WireTransferBridgeRecording(callerRx, callerTx *rtp.SIPRTPTransport) {
	if cs == nil {
		return
	}
	if callerRx != nil {
		callerRx.OnInputRTP = func(seq uint16, ts uint32, p []byte) {
			cs.appendRecordingFrame(recDirUser, seq, ts, p)
		}
	}
	if callerTx != nil {
		callerTx.OnOutputRTP = func(seq uint16, ts uint32, p []byte) {
			cs.appendRecordingFrame(recDirAI, seq, ts, p)
		}
	}
}

func (cs *CallSession) appendRecordingFrame(dir byte, seq uint16, ts uint32, p []byte) {
	if cs == nil || len(p) == 0 {
		return
	}
	if dir != recDirUser && dir != recDirAI {
		return
	}
	maxB := 50 * 1024 * 1024
	cs.recMu.Lock()
	defer cs.recMu.Unlock()
	if len(cs.recBuf) >= maxB {
		return
	}
	rem := maxB - len(cs.recBuf)
	if rem <= 0 {
		return
	}
	frameOverhead := 1 + 2 + 4 + 8 + 2 // dir + seq + rtpTs + wallNs + uint16 len
	if len(cs.recBuf) == 0 {
		if len(recBlobMagic) > rem {
			return
		}
		cs.recTimeOrigin = time.Now()
		cs.recBuf = append(cs.recBuf, recBlobMagic...)
		rem = maxB - len(cs.recBuf)
	}
	if frameOverhead+len(p) > rem {
		return
	}
	wallNs := uint64(time.Since(cs.recTimeOrigin))
	cs.recBuf = append(cs.recBuf, dir)
	var hdr [16]byte
	binary.LittleEndian.PutUint16(hdr[0:2], seq)
	binary.LittleEndian.PutUint32(hdr[2:6], ts)
	binary.LittleEndian.PutUint64(hdr[6:14], wallNs)
	binary.LittleEndian.PutUint16(hdr[14:16], uint16(len(p)))
	cs.recBuf = append(cs.recBuf, hdr[:]...)
	cs.recBuf = append(cs.recBuf, p...)
}

// TakeRecording returns buffered RTP recording (SN3 …) and clears the buffer.
func (cs *CallSession) TakeRecording() []byte {
	if cs == nil {
		return nil
	}
	cs.recMu.Lock()
	defer cs.recMu.Unlock()
	if len(cs.recBuf) == 0 {
		return nil
	}
	out := make([]byte, len(cs.recBuf))
	copy(out, cs.recBuf)
	cs.recBuf = cs.recBuf[:0]
	cs.recTimeOrigin = time.Time{}
	return out
}
