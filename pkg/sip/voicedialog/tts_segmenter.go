package voicedialog

// 段间无握手 TTS 流水线。
//
// 背景：腾讯云 TTS Speech Synthesizer SDK 不支持「同一会话内追加文本」——每次
// `Synthesize(text)` 都会建一条独立的 WebSocket，首字节延迟 200~400ms。当 LLM
// 流式按句切分后我们会下发 N 个 `tts.speak`，串行调用 SDK 会让段间出现大量空隙。
//
// 因此我们**不**真的去复用同一个 SDK 会话，而是：
//   1. 收到一段 tts.speak 立即起 prefetch goroutine 调用 SDK，PCM 写入 segment 自带的 channel；
//   2. 单一 player goroutine 串行 drain 每段的 channel 并按 20ms 帧送 RTP；
//   3. 段 N 在段 N-1 还在播放时已并行完成网络握手 / 首字节，N-1 一播完段 N 的 PCM 已就绪。
//
// 用户体感等价于「段间无握手」，同时不破坏腾讯云 SDK 的请求-响应语义。

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/media"
	"github.com/LingByte/SoulNexus/pkg/sip/conversation"
	"go.uber.org/zap"
)

const (
	// ttsSegmentQueueDepth 上限太大没意义（一轮回复通常 2~5 段），太小遇到突发会丢段。
	ttsSegmentQueueDepth = 32
	// ttsSegmentPCMChanCap 每段的 PCM 缓冲块数量；腾讯云 SDK 一般每 ~80ms 推一块，
	// 32 块覆盖 ~2.5s，足以让播放线程不饿死也不至于占用过多内存。
	ttsSegmentPCMChanCap = 32
	// ttsFrameDuration 推 RTP 的帧粒度，与现有上行/下行一致。
	ttsFrameDuration = 20 * time.Millisecond
	// ttsJitterPrebufferMs 首帧出门前需先在 buffer 里攒够多少毫秒的 PCM。
	// 腾讯云 WS 推 PCM 节奏不均（典型 80ms 一块，偶尔 150~200ms 一块），如果
	// 拿到第一块就发，紧接着碰到长间隔会让 RTP 节拍停顿 → 对端音质卡顿。
	// 攒 60ms 相当于 3 帧 jitter buffer，绝大多数情况下不会再饿死播放线程；
	// 代价是首字节延迟 +60ms，对总体感（>1s）几乎不可察觉。
	ttsJitterPrebufferMs = 60
	// ttsUnderrunSilenceMaxMs drain 中游 pcmCh 暂时空了，最多用静音帧填充多久
	// 维持 RTP 节拍。超过该上限说明上游真的卡死了，让 select 阻塞回退到正常等待。
	ttsUnderrunSilenceMaxMs = 200
)

// ttsSegmentJob 是上层 (loopback) 通过 tts.speak 入队的一段语音。
type ttsSegmentJob struct {
	text        string
	utteranceID string
	gen         uint64

	// 入队时间，用于度量「tts.speak 收到 → 首 PCM 出 RTP」端到端延迟。
	enqueueT0 time.Time

	// prefetch 产出的原始 PCM 块（cloudSR / 16bit / mono，未分帧、未重采样）。
	pcmCh chan []byte

	// 关闭 prefetch 的 ctx；在被 player 跳过或 barge-in 时调用。
	prefetchCancel context.CancelFunc

	// prefetchErr 由 prefetch 在 close(pcmCh) 之前以 sync.Once 风格写入，
	// player 在 drain 完 pcmCh 之后读取，无需额外加锁。
	prefetchErr error
	prefetchMu  sync.Mutex
}

func (j *ttsSegmentJob) setPrefetchErr(err error) {
	if err == nil {
		return
	}
	j.prefetchMu.Lock()
	if j.prefetchErr == nil {
		j.prefetchErr = err
	}
	j.prefetchMu.Unlock()
}

func (j *ttsSegmentJob) getPrefetchErr() error {
	j.prefetchMu.Lock()
	defer j.prefetchMu.Unlock()
	return j.prefetchErr
}

// startTTSPlayer is called once per session at attachGatewayMedia time.
// It owns: ttsSegmentCh consumer, in-flight cancel handle, framing/pacing/SendToOutput.
func (sess *dialogSession) startTTSPlayer() {
	sess.ttsPlayerOnce.Do(func() {
		sess.ttsSegmentCh = make(chan *ttsSegmentJob, ttsSegmentQueueDepth)
		sess.ttsPlayerWg.Add(1)
		go sess.ttsPlayerLoop()
	})
}

// stopTTSPlayer signals the player to drain & exit. Idempotent.
func (sess *dialogSession) stopTTSPlayer() {
	sess.ttsPlayerStopOnce.Do(func() {
		// Mark all queued segments stale so the player drops them quickly.
		sess.invalidateQueuedTTS()
		// Cancel any in-flight segment.
		sess.cancelCurrentTTSSegment()
		// Close the input channel; player will exit after draining.
		close(sess.ttsSegmentCh)
	})
	sess.ttsPlayerWg.Wait()
}

func (sess *dialogSession) cancelCurrentTTSSegment() {
	sess.ttsCurrentMu.Lock()
	cancel := sess.ttsCurrentCancel
	sess.ttsCurrentCancel = nil
	sess.ttsCurrentMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// enqueueTTSSegment is the entry point invoked from handleTTSSpeak.
// Returns false if the session is already shutting down or queue full.
func (sess *dialogSession) enqueueTTSSegment(job *ttsSegmentJob) bool {
	if sess == nil || job == nil {
		return false
	}
	defer func() {
		// channel close panics on send → recover and treat as failure to enqueue.
		_ = recover()
	}()
	select {
	case sess.ttsSegmentCh <- job:
		return true
	default:
		logger.Warn("voicedialog tts segment queue full, dropping",
			zap.String(KeyCallID, sess.meta.CallID),
			zap.String(KeyUtteranceID, job.utteranceID),
		)
		return false
	}
}

// startSegmentPrefetch kicks off the SDK call for one segment. PCM bytes are written
// into job.pcmCh; channel is closed on completion or cancellation.
func (sess *dialogSession) startSegmentPrefetch(job *ttsSegmentJob) {
	if sess == nil || job == nil {
		return
	}
	parent := context.Background()
	if ms := sess.cs.MediaSession(); ms != nil {
		parent = ms.GetContext()
	}
	ctx, cancel := context.WithCancel(parent)
	job.prefetchCancel = cancel
	job.pcmCh = make(chan []byte, ttsSegmentPCMChanCap)

	go func() {
		defer close(job.pcmCh)
		svc := sess.ttsService
		if svc == nil {
			job.setPrefetchErr(errors.New("voicedialog: nil tts service"))
			return
		}
		err := svc.SynthesizeStream(ctx, job.text, func(pcm []byte) error {
			if len(pcm) == 0 {
				return nil
			}
			cp := make([]byte, len(pcm))
			copy(cp, pcm)
			select {
			case job.pcmCh <- cp:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			job.setPrefetchErr(err)
		}
	}()
}

func (sess *dialogSession) ttsPlayerLoop() {
	defer sess.ttsPlayerWg.Done()
	ms := sess.cs.MediaSession()
	if ms == nil {
		return
	}
	bridgeSR := sess.ttsBridgeSR
	cloudSR := sess.ttsCloudSR
	if bridgeSR <= 0 {
		bridgeSR = 16000
	}
	if cloudSR <= 0 {
		cloudSR = bridgeSR
	}
	bytesPerFrame := bridgeSR * 2 * int(ttsFrameDuration/time.Millisecond) / 1000
	if bytesPerFrame <= 0 {
		bytesPerFrame = 640 // 16k/16bit/mono/20ms fallback
	}

	for job := range sess.ttsSegmentCh {
		sess.playSegment(ms, job, cloudSR, bridgeSR, bytesPerFrame)
		sess.endPendingTTS()
	}
}

// playSegment drains one job's pcmCh and emits framed PCM to the call's RTP output.
// On generation invalidation / barge-in / context cancel it short-circuits and emits
// tts.ended with ok=false.
func (sess *dialogSession) playSegment(
	ms *media.MediaSession,
	job *ttsSegmentJob,
	cloudSR, bridgeSR int,
	bytesPerFrame int,
) {
	if job == nil {
		return
	}
	callID := sess.meta.CallID

	// Generation gate: drop if invalidated before we even started.
	if job.gen != 0 && job.gen <= sess.ttsGenInvalidBefore.Load() {
		logger.Info("voicedialog gateway tts segment dropped (stale generation, pre-play)",
			zap.String(KeyCallID, callID),
			zap.String(KeyUtteranceID, job.utteranceID),
			zap.Uint64("gen", job.gen),
		)
		if job.prefetchCancel != nil {
			job.prefetchCancel()
		}
		// drain channel to unblock prefetch
		for range job.pcmCh {
		}
		sess.emitGateway(event(EvTTSEnded, callID, map[string]any{
			KeyUtteranceID: job.utteranceID,
			KeyOK:          false,
		}))
		return
	}
	// Hard cutoff: transfer in progress.
	if conversation.IsTransferInProgress(callID) {
		logger.Info("voicedialog gateway tts segment dropped (transfer in progress)",
			zap.String(KeyCallID, callID),
			zap.String(KeyUtteranceID, job.utteranceID),
		)
		if job.prefetchCancel != nil {
			job.prefetchCancel()
		}
		for range job.pcmCh {
		}
		sess.emitGateway(event(EvTTSEnded, callID, map[string]any{
			KeyUtteranceID: job.utteranceID,
			KeyOK:          false,
		}))
		return
	}

	// Per-segment cancel context: invalidateQueuedTTS / handleTTSCancel reach in via cancelCurrentTTSSegment.
	parent := ms.GetContext()
	segCtx, segCancel := context.WithCancel(parent)
	sess.ttsCurrentMu.Lock()
	sess.ttsCurrentCancel = segCancel
	sess.ttsCurrentMu.Unlock()
	defer func() {
		sess.ttsCurrentMu.Lock()
		if sess.ttsCurrentCancel != nil {
			// Best effort: only clear if still ours.
			sess.ttsCurrentCancel = nil
		}
		sess.ttsCurrentMu.Unlock()
		segCancel()
		if job.prefetchCancel != nil {
			job.prefetchCancel()
		}
	}()

	logger.Info("voicedialog gateway tts speak start",
		zap.String(KeyCallID, callID),
		zap.String(KeyUtteranceID, job.utteranceID),
		zap.Int("text_len", len([]rune(job.text))),
		zap.String("text_preview", truncateRunes(job.text, 120)),
	)
	sess.emitGateway(event(EvTTSStarted, callID, map[string]any{
		KeyUtteranceID: job.utteranceID,
		KeyTextPreview: truncateRunes(job.text, 160),
	}))

	// Arm one-shot first-audio hook (logged on first frame actually leaving the gateway).
	speakCmdT0 := job.enqueueT0
	utterIDLocal := job.utteranceID
	sess.armFirstAudioHook(func() {
		logger.Info("voicedialog gateway tts first audio out",
			zap.String(KeyCallID, callID),
			zap.String(KeyUtteranceID, utterIDLocal),
			zap.Int("first_audio_ms", int(time.Since(speakCmdT0).Milliseconds())),
		)
	})
	defer sess.armFirstAudioHook(nil)

	if sess.ttsPlayingPtr != nil {
		sess.ttsPlayingPtr.Store(true)
	}
	if sess.ttsStartedNS != nil {
		sess.ttsStartedNS.Store(time.Now().UnixNano())
	}
	defer func() {
		if sess.ttsPlayingPtr != nil {
			sess.ttsPlayingPtr.Store(false)
		}
		if sess.ttsStartedNS != nil {
			sess.ttsStartedNS.Store(0)
		}
	}()

	pipelineT0 := time.Now()
	ttsT0 := time.Now()

	// jitter buffer 预蓄阈值（字节）。bridgeSR=16k 时 60ms = 1920B = 3 帧。
	prebufferBytes := bridgeSR * 2 * ttsJitterPrebufferMs / 1000
	frameMs := int(ttsFrameDuration / time.Millisecond)
	maxSilenceFrames := ttsUnderrunSilenceMaxMs / frameMs

	var (
		buffer          []byte
		drainedClean    bool
		aborted         bool
		pcmChClosed     bool // 上游 prefetch 已经 close(pcmCh)，buffer 里剩余字节就是全部
		firstFrameSent  bool // 用于 jitter prebuffer：首帧未出门前累积到 prebufferBytes 再发
		silenceFrames   int  // 连续静音兜底帧数，达到 maxSilenceFrames 即停止补静音回退到阻塞等待
		emittedAnyAudio bool // 仅 firstAudioHook 触发用
	)

	emitFrame := func(payload []byte) bool {
		if job.gen != 0 && job.gen <= sess.ttsGenInvalidBefore.Load() {
			aborted = true
			return false
		}
		if segCtx.Err() != nil {
			aborted = true
			return false
		}
		if !emittedAnyAudio {
			sess.fireAndClearFirstAudioHook()
			emittedAnyAudio = true
		}
		ms.SendToOutput("voice-gateway-tts", &media.AudioPacket{
			Payload:       payload,
			IsSynthesized: true,
		})
		select {
		case <-segCtx.Done():
			aborted = true
			return false
		case <-time.After(ttsFrameDuration):
		}
		return true
	}

	// 把一块 PCM（已经过重采样）追加到 buffer。
	appendPCM := func(pcm []byte) {
		if len(pcm) == 0 {
			return
		}
		out := pcm
		if cloudSR != bridgeSR && len(pcm) >= 2 {
			if rs, err := media.ResamplePCM(pcm, cloudSR, bridgeSR); err == nil && len(rs) > 0 {
				out = rs
			}
		}
		buffer = append(buffer, out...)
	}

drainLoop:
	for {
		// Pre-frame gen check so barge-in mid-segment exits promptly.
		if job.gen != 0 && job.gen <= sess.ttsGenInvalidBefore.Load() {
			aborted = true
			break drainLoop
		}

		// ---- 阶段 A：决定是否要从 pcmCh 读 ----
		// 必须读：buffer 不够发一帧；或首帧未出门、还没攒够 jitter 预蓄。
		needRead := !pcmChClosed && (len(buffer) < bytesPerFrame ||
			(!firstFrameSent && len(buffer) < prebufferBytes))

		if needRead {
			// 先尝试非阻塞读：如果 pcmCh 暂时没数据但已经在播放（firstFrameSent=true），
			// 给一帧静音兜底，避免 RTP 节拍停顿造成对端音质卡顿。
			select {
			case <-segCtx.Done():
				aborted = true
				break drainLoop
			case pcm, ok := <-job.pcmCh:
				if !ok {
					pcmChClosed = true
				} else {
					appendPCM(pcm)
					silenceFrames = 0
				}
			default:
				if firstFrameSent && silenceFrames < maxSilenceFrames {
					if !emitFrame(make([]byte, bytesPerFrame)) {
						break drainLoop
					}
					silenceFrames++
					continue drainLoop
				}
				// 阻塞等：要么真有数据，要么上游 close（短句已经全收完）。
				select {
				case <-segCtx.Done():
					aborted = true
					break drainLoop
				case pcm, ok := <-job.pcmCh:
					if !ok {
						pcmChClosed = true
					} else {
						appendPCM(pcm)
						silenceFrames = 0
					}
				}
			}
		}

		// ---- 阶段 B：判断是否可以发帧 ----
		// 首帧出门门槛：
		//   - 已经攒够 prebufferBytes，或
		//   - pcmCh 已 close（短句不足 prebufferBytes，全部 flush 出去）
		canEmitFirst := firstFrameSent ||
			len(buffer) >= prebufferBytes ||
			pcmChClosed

		for len(buffer) >= bytesPerFrame && canEmitFirst {
			frame := make([]byte, bytesPerFrame)
			copy(frame, buffer[:bytesPerFrame])
			buffer = buffer[bytesPerFrame:]
			if !emitFrame(frame) {
				break drainLoop
			}
			firstFrameSent = true
			silenceFrames = 0
		}

		// 上游 close + buffer 不足一帧 = 干净结束。
		if pcmChClosed && len(buffer) < bytesPerFrame {
			drainedClean = true
			break drainLoop
		}
	}

	// Flush trailing partial frame on clean drain (zero-pad to bytesPerFrame).
	if drainedClean && !aborted && len(buffer) > 0 {
		padded := make([]byte, bytesPerFrame)
		copy(padded, buffer)
		_ = emitFrame(padded)
	}

	ttsMs := int(time.Since(ttsT0).Milliseconds())
	prefetchErr := job.getPrefetchErr()
	ok := drainedClean && prefetchErr == nil && !aborted

	sess.emitGateway(event(EvTTSEnded, callID, map[string]any{
		KeyUtteranceID: job.utteranceID,
		KeyOK:          ok,
	}))
	if !ok {
		if prefetchErr != nil {
			logger.Warn("voicedialog gateway tts speak error",
				zap.String(KeyCallID, callID),
				zap.String(KeyUtteranceID, job.utteranceID),
				zap.Error(prefetchErr),
			)
		}
		return
	}
	logger.Info("voicedialog gateway tts speak done",
		zap.String(KeyCallID, callID),
		zap.String(KeyUtteranceID, job.utteranceID),
		zap.Int("tts_ms", ttsMs),
	)

	asrSnap := sess.lastASRFinalSnapshot()
	asrProv := strings.TrimSpace(sess.voicedialogASRProv)
	if asrProv == "" {
		asrProv = "qcloud_asr"
	}
	llmModel, llmWall := sess.takePendingLLMMeta()
	go conversation.RecordDialogTurn(context.Background(), callID, conversation.DialogTurn{
		ASRText:     asrSnap,
		LLMText:     job.text,
		ASRProvider: asrProv,
		TTSProvider: "qcloud_tts",
		LLMModel:    llmModel,
		Trigger:     "final",
		LLMWallMs:   llmWall,
		TTSMs:       ttsMs,
		PipelineMs:  int(time.Since(pipelineT0).Milliseconds()),
	})
}
