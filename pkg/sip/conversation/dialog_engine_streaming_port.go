package conversation

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// StreamingCallSessionPort — Phase 3 real MediaPort.
//
// Unlike CallSessionPort (the PR-6 legacy-bridge adapter whose
// streaming methods return ErrLegacyBridgeOnly), this port owns real
// PCM channels:
//
//   - InputPCM(): a buffered receive channel fed by a media processor
//     registered on the underlying *media.MediaSession. One AudioPacket
//     ↦ one engine.PCMFrame.
//   - SendOutputPCM(): wraps a PCMFrame in &media.AudioPacket{
//     IsSynthesized:true} and pumps it through MediaSession.SendToOutput,
//     resampling when the caller-supplied SampleRate differs from the
//     bridge rate.
//   - OnBargeIn(): the registration is stored but NOT fired by the port
//     itself in this phase. The legacy attachers own VAD (sipvad) and
//     fire barge-in by draining outputs directly. A future PR moves
//     VAD into a pipeline.Stage so the engine can fire OnBargeIn
//     through here; until then this is a pass-through stub.
//
// Lifecycle:
//
//   - NewStreamingCallSessionPort registers the input processor and
//     starts the goroutine that pumps PCM into the input channel.
//   - InputPCM is closed (and SendOutputPCM starts returning ErrPortClosed)
//     when EITHER MediaSession.GetContext is cancelled (call ended) OR
//     Close is called explicitly (engine.Detach teardown).
//   - Close is idempotent; second call is a no-op.
//
// NOT registered in production yet. The OnACK seam still wires
// CallSessionPort via the legacy bridge. To exercise this port, an
// engine implementation must explicitly construct it
// (see pkg/dialog/cascaded for the consumer-side shape).

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/media"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
)

// ErrPortClosed is returned by SendOutputPCM after the port has been
// closed (either via Close or via the media session ctx ending).
var ErrPortClosed = errors.New("dialog engine streaming port: closed")

// defaultStreamingPortBuffer is the buffered depth of InputPCM. With
// 20 ms audio frames this is ~640 ms of slack before back-pressure
// kicks in — generous but bounded so a stalled engine reader can't
// blow memory on a long call.
const defaultStreamingPortBuffer = 32

// streamingPortSender is the sender name used for SendToOutput. Logged
// inside MediaSession traces so we can attribute output frames to the
// native engine path rather than legacy attachers.
const streamingPortSender = "dialog-engine-streaming"

// StreamingCallSessionPort adapts *sipSession.CallSession to
// engine.MediaPort with real bi-directional PCM streaming. Construct
// via NewStreamingCallSessionPort; the zero value is invalid.
type StreamingCallSessionPort struct {
	cs         *sipSession.CallSession
	ms         *media.MediaSession
	bridgeRate int // mono PCM16LE rate the underlying RTP decode produces

	in chan engine.PCMFrame

	bargeMu sync.RWMutex
	barge   func()

	closeOnce sync.Once
	closed    atomic.Bool
}

// Compile-time assertion: keep the streaming port honest against the
// MediaPort contract. We do NOT implement legacy.LegacyHandle here
// because that's a transitional escape hatch tied to the legacy
// bridge — native engines must speak MediaPort only.
var _ engine.MediaPort = (*StreamingCallSessionPort)(nil)

// NewStreamingCallSessionPort wraps cs with a streaming media port.
// Returns nil when cs / MediaSession is nil so callers can guard with
// a single check instead of deferred nil dereferences inside Attach.
//
// On construction the port:
//
//   - Registers a media processor on cs.MediaSession() that copies
//     each inbound (non-synthesized) AudioPacket into the InputPCM
//     channel as an engine.PCMFrame.
//   - Starts a goroutine that closes the InputPCM channel when the
//     media session's context is cancelled (call ended).
func NewStreamingCallSessionPort(cs *sipSession.CallSession) *StreamingCallSessionPort {
	if cs == nil {
		return nil
	}
	return newStreamingPort(cs, cs.MediaSession(), cs.PCMSampleRate())
}

// newStreamingPort is the package-private assembler used by both the
// production constructor and tests. Tests pass a nil CallSession with
// an explicit *media.MediaSession built via media.NewDefaultSession();
// metadata accessors are nil-cs safe so they degrade to empty strings
// rather than panicking.
func newStreamingPort(cs *sipSession.CallSession, ms *media.MediaSession, bridgeRate int) *StreamingCallSessionPort {
	if ms == nil {
		return nil
	}
	if bridgeRate <= 0 {
		bridgeRate = 16000
	}
	p := &StreamingCallSessionPort{
		cs:         cs,
		ms:         ms,
		bridgeRate: bridgeRate,
		in:         make(chan engine.PCMFrame, defaultStreamingPortBuffer),
	}

	// Register the inbound processor synchronously so any caller
	// reading InputPCM right after construction sees a wired pipe.
	// PriorityHigh matches the legacy ASR feed processor — both want
	// to see caller audio as early as possible in the chain.
	proc := media.NewPacketProcessor(streamingPortSender, media.PriorityHigh,
		func(_ context.Context, _ *media.MediaSession, packet media.MediaPacket) error {
			if p.closed.Load() {
				return nil
			}
			ap, ok := packet.(*media.AudioPacket)
			if !ok || ap == nil || len(ap.Payload) == 0 || ap.IsSynthesized {
				return nil
			}
			// Copy because the AudioPacket buffer is owned by the
			// decode pipeline and may be reused after this callback
			// returns. The engine consumer must NOT see mutated bytes.
			data := make([]byte, len(ap.Payload))
			copy(data, ap.Payload)
			frame := engine.PCMFrame{
				Data:       data,
				SampleRate: p.bridgeRate,
			}
			// Non-blocking send: if the engine reader is behind by
			// more than defaultStreamingPortBuffer frames we drop the
			// oldest frame and enqueue the new one. Choice: keep the
			// freshest audio rather than block the media pipeline,
			// which would cascade into RTP jitter on the caller leg.
			select {
			case p.in <- frame:
			default:
				select {
				case <-p.in: // discard oldest
				default:
				}
				select {
				case p.in <- frame:
				default:
					// extremely contended; drop the new frame too.
				}
			}
			return nil
		})
	ms.RegisterProcessor(proc)

	// Close the input channel when the media session ends. This
	// matches the engine.MediaPort contract: "Closed by the
	// transport on hangup".
	go func() {
		<-ms.GetContext().Done()
		p.markClosed()
	}()

	return p
}

// markClosed transitions the port to closed once. Idempotent.
func (p *StreamingCallSessionPort) markClosed() {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		close(p.in)
	})
}

// Close transitions the port to the closed state. Idempotent; safe
// from any goroutine. The engine should call this from its Detach
// handle so the input goroutine releases and SendOutputPCM stops
// accepting frames.
//
// Returns nil so it can be wired to engine.Detach without an adapter.
func (p *StreamingCallSessionPort) Close() error {
	if p == nil {
		return nil
	}
	p.markClosed()
	return nil
}

// InputPCM returns the channel of decoded caller PCM. Closed by the
// port when the call ends (media session ctx cancel) or when Close
// is invoked.
func (p *StreamingCallSessionPort) InputPCM() <-chan engine.PCMFrame {
	if p == nil {
		// Return a permanently-closed channel so a caller that
		// somehow has a nil port still terminates its read loop
		// instead of blocking forever.
		ch := make(chan engine.PCMFrame)
		close(ch)
		return ch
	}
	return p.in
}

// SendOutputPCM enqueues one frame for playback to the caller. The
// frame is wrapped in &media.AudioPacket{IsSynthesized: true} and
// sent via MediaSession.SendToOutput. The transport handles RTP
// pacing.
//
// Resampling: when frame.SampleRate is non-zero and differs from the
// bridge rate, the frame is resampled in-line using media.ResamplePCM.
// SampleRate == 0 is treated as "already at bridge rate" — most native
// stages will tag their frames so resample is opt-in.
//
// Errors:
//
//   - port == nil → ErrPortClosed.
//   - port closed (Close called / call ended) → ErrPortClosed.
//   - len(frame.Data) == 0 → nil (silently dropped; matches the
//     "drain in" stage idiom that may emit empty frames as control
//     signals).
//   - resample failure → wrapped error with frame sample-rate context.
func (p *StreamingCallSessionPort) SendOutputPCM(frame engine.PCMFrame) error {
	if p == nil || p.closed.Load() {
		return ErrPortClosed
	}
	if len(frame.Data) == 0 {
		return nil
	}
	payload := frame.Data
	if frame.SampleRate > 0 && frame.SampleRate != p.bridgeRate {
		out, err := media.ResamplePCM(frame.Data, frame.SampleRate, p.bridgeRate)
		if err != nil {
			return fmt.Errorf("dialog engine streaming port: resample %d→%d: %w",
				frame.SampleRate, p.bridgeRate, err)
		}
		if len(out) == 0 {
			return nil
		}
		payload = out
	}
	// Stereo recorder: keep the recording symmetric with the legacy
	// attach path, which writes every synthesized frame via
	// cs.WriteAIPCM. Skipping this on the native path would silently
	// produce a one-sided recording — and stakeholders depend on
	// stereo audit. Use a defensive nil-guard because
	// cs.WriteAIPCM is safe on nil receiver but explicit is
	// clearer.
	if p.cs != nil {
		p.cs.WriteAIPCM(payload)
	}
	p.ms.SendToOutput(streamingPortSender, &media.AudioPacket{
		Payload:       payload,
		IsSynthesized: true,
	})
	return nil
}

// OnBargeIn stores the callback. Phase-3 stub: the port itself does
// not fire it (VAD lives in legacy attachers + sipvad today). Future
// PR moves VAD into a pipeline.Stage that will invoke this through
// (*StreamingCallSessionPort).fireBargeIn — see the package doc.
//
// Replacing a previous registration is allowed; nil clears it.
func (p *StreamingCallSessionPort) OnBargeIn(fn func()) {
	if p == nil {
		return
	}
	p.bargeMu.Lock()
	p.barge = fn
	p.bargeMu.Unlock()
}

// fireBargeIn is the package-internal hook that invokes the
// registered barge-in callback (if any). Reserved for the future
// VAD-pipeline stage; not yet wired but exposed at package scope so
// the upcoming PR can call it without exporting OnBargeIn's storage.
func (p *StreamingCallSessionPort) fireBargeIn() {
	if p == nil {
		return
	}
	p.bargeMu.RLock()
	fn := p.barge
	p.bargeMu.RUnlock()
	if fn != nil {
		fn()
	}
	// Also drain queued AI output so the caller hears their voice
	// without the AI finishing the previous sentence first — matches
	// the realtime attach path's barge-in behaviour.
	if p.ms != nil {
		_ = p.ms.DrainOutputs()
	}
}

// Codec returns the SIP-negotiated codec for this call. Engines that
// don't care can ignore it; recorders / passthrough optimisations do.
func (p *StreamingCallSessionPort) Codec() engine.CodecSpec {
	if p == nil || p.cs == nil {
		return engine.CodecSpec{}
	}
	c := p.cs.NegotiatedCodec()
	channels := c.Channels
	if channels <= 0 {
		channels = 1
	}
	return engine.CodecSpec{
		Name:       strings.ToUpper(strings.TrimSpace(c.Name)),
		SampleRate: c.ClockRate,
		Channels:   channels,
	}
}

// SampleRate returns the bridge-side mono PCM rate.
func (p *StreamingCallSessionPort) SampleRate() int {
	if p == nil {
		return 16000
	}
	return p.bridgeRate
}

// CallID returns the unique call identifier.
func (p *StreamingCallSessionPort) CallID() string {
	if p == nil || p.cs == nil {
		return ""
	}
	return p.cs.CallID
}

// TenantID returns the owning tenant as a decimal string. Matches
// CallSessionPort.TenantID's encoding so dashboards keep working
// across the two port types.
func (p *StreamingCallSessionPort) TenantID() string {
	if p == nil || p.cs == nil {
		return ""
	}
	return strconv.FormatUint(uint64(p.cs.TenantID()), 10)
}
