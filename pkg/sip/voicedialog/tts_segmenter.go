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

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/LinByte/VoiceServer/pkg/sip/conversation"
	siptts "github.com/LinByte/VoiceServer/pkg/voice/tts"
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
	// ttsUnderrunSilenceMaxMs 已废弃：此前在 underrun 时插入「DC hold 帧」试图
	// 维持 RTP 节拍，但 hold-frame → 真 PCM 重连那一刻会产生波形阶跃（hold 住
	// 的最后样本值和下一块 PCM 首样本之间是任意 step），PCMA 编码后接收端听到
	// 一个 click。一段回复里 SDK 推 PCM 节奏不均（典型 8-20 次 underrun），
	// click 串叠起来就是用户反馈的"滋滋滋"电流音。VS 的 voice/tts/Pipeline 在
	// underrun 时只是阻塞等下一块 PCM，从不合成兜底帧；实测对端 SIP UA 的
	// jitter buffer (60-200ms) 完全能吸收这种短暂停顿。改为零兜底帧。
	ttsUnderrunSilenceMaxMs = 0
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

	// 改造：丢掉自己写的 buffer/emitFrame/兜底帧那套，**完全复用 VS 的 voice/tts.Pipeline**。
	// 原因：welcome.wav (playPCMFrames) 和上行用户录音都没有"电流音"，唯独 TTS 这条路径有；
	// 唯一可定位的差异是 player loop 自己重新发明了一遍「分帧 + 节拍 + underrun 兜底」，并且
	// emitFrame 用 time.After 而非 ticker，长话累计漂移 + chunk 边界不连续 + 之前的 DC-hold
	// 兜底叠加产生周期 click → 听起来像电流音。VS 的 Pipeline:
	//   - 帧从一个**连续 buffer** 切（chunk 边界天然安全）
	//   - PaceRealtime + 单点 sleep，无漂移
	//   - underrun 直接阻塞等下一块（不合成假帧）
	//   - 残余尾字节用 Finalize() 单次 zero-pad，不会每段 cliff
	// prefetch goroutine 这层不去掉：tts.Pipeline 的 Service 注入 chanReplayService，把 prefetch
	// 已经写好的 pcmCh 直接 replay 给 Pipeline；段间 SDK 握手并行的优化继续保留。
	chanSvc := &chanReplayService{
		ch:           job.pcmCh,
		ctxBlockChan: segCtx.Done(),
	}
	pipe, perr := siptts.New(siptts.Config{
		Service:       chanSvc,
		SampleRate:    bridgeSR,
		Channels:      1,
		FrameDuration: ttsFrameDuration,
		PaceRealtime:  true,
		SendPCMFrame: func(frame []byte) error {
			if job.gen != 0 && job.gen <= sess.ttsGenInvalidBefore.Load() {
				return context.Canceled
			}
			if segCtx.Err() != nil {
				return segCtx.Err()
			}
			sess.fireAndClearFirstAudioHook()
			ms.SendToOutput("voice-gateway-tts", &media.AudioPacket{
				Payload:       frame,
				IsSynthesized: true,
			})
			return nil
		},
	})
	if perr != nil {
		logger.Warn("voicedialog gateway tts pipeline init failed",
			zap.String(KeyCallID, callID), zap.Error(perr))
		// drain channel to unblock prefetch
		for range job.pcmCh {
		}
		sess.emitGateway(event(EvTTSEnded, callID, map[string]any{
			KeyUtteranceID: job.utteranceID,
			KeyOK:          false,
		}))
		return
	}
	if cloudSR != bridgeSR {
		// chanReplayService will resample CONCATENATED bytes (not per-chunk) to avoid
		// the InterpolatingConverter chunk-boundary phase reset. See its impl below.
		chanSvc.cloudSR = cloudSR
		chanSvc.bridgeSR = bridgeSR
	}
	pipe.Start(segCtx)
	speakErr := pipe.Speak(job.text)
	// Drain the residual sub-frame tail with a single zero-pad — same as VS pipeline's
	// Finalize. Avoids losing the last <20ms of audio at segment end.
	_ = pipe.Finalize()
	pipe.Stop()

	aborted := segCtx.Err() != nil ||
		(job.gen != 0 && job.gen <= sess.ttsGenInvalidBefore.Load())
	prefetchErr := job.getPrefetchErr()
	drainedClean := !aborted && speakErr == nil && prefetchErr == nil
	ttsMs := int(time.Since(ttsT0).Milliseconds())
	ok := drainedClean

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
	// 同一轮次的所有 TTS 段共用 parent utterance ID（loopback.go 生成的 "loopback-<nanos>-sN"
	// 格式，去掉 "-sN" 后缀得到父级 ID），传给持久化层让它折叠成一条对话记录。
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
		TurnGroupID: parentUtteranceID(job.utteranceID),
	})
}

// parentUtteranceID 把 voicedialog loopback 生成的 "loopback-<nanos>-s<N>" 段级 ID
// 还原成轮次级父 ID："loopback-<nanos>"。无 "-s<N>" 后缀时原样返回（兼容外部 utterance）。
func parentUtteranceID(uid string) string {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return ""
	}
	idx := strings.LastIndex(uid, "-s")
	if idx <= 0 {
		return uid
	}
	tail := uid[idx+2:]
	if tail == "" {
		return uid
	}
	for _, r := range tail {
		if r < '0' || r > '9' {
			return uid
		}
	}
	return uid[:idx]
}

// chanReplayService adapts the prefetch goroutine's PCM channel to siptts.Service so the
// VS-style Pipeline can frame/pace it. The Pipeline calls SynthesizeStream once per Speak;
// we drain `ch` and forward bytes to its onPCMChunk. The TEXT argument is unused here (the
// real synthesis already happened upstream in startSegmentPrefetch).
//
// Resample policy:
//
//   - cloudSR == bridgeSR: pass-through, Pipeline handles per-chunk concat into its own
//     buffer (chunk-boundary alignment automatically safe).
//
//   - cloudSR == 2 * bridgeSR (the common 16k → 8k case for G.711 calls): use a streaming
//     2-tap box-average decimation INLINE per chunk. Statelessly maps src[2k], src[2k+1]
//     to one output sample (s2k + s2k+1) / 2. This is mild anti-alias filtering AND it
//     produces NO chunk-boundary phase discontinuity (because every input sample-pair
//     maps to exactly one output sample, no fractional carry). Far better than:
//     a) `media.ResamplePCM` per-chunk linear interp (resets sourcePos=0 each call →
//     chunk-boundary phase jump → broadband click train = "电流音")
//     b) raw decimation (no LP filter → 4-8kHz content folded into [0,4kHz] band)
//     This is the path that fixes the user-reported "TTS 电流音"； welcome.wav and uplink
//     audio bypassed it because they were already at the bridge rate.
//
//   - cloudSR is otherwise unequal to bridgeSR (e.g. 24k → 8k): fall back to whole-segment
//     buffer + single ResamplePCM call. Trades a little first-audio latency for clean
//     boundaries. Rare path; QCloud rates are commonly 8/16/24/48 kHz.
//
// In all cases we defensively drop trailing odd byte from each chunk to guard against PCM16
// sample mis-alignment (one odd byte would swap high/low bytes on every subsequent sample).
type chanReplayService struct {
	ch           <-chan []byte
	ctxBlockChan <-chan struct{}
	cloudSR      int // 0 means same as bridge — no resample
	bridgeSR     int
	// halfRem holds the trailing odd-sample-pair byte from the previous chunk in the
	// 16k→8k streaming path (when a chunk ends mid-pair). Carries to next chunk so we
	// never lose samples or split a pair across chunks. nil when no pending sample.
	halfRem []byte
}

// streamingHalveDecimate16to8 averages every two consecutive int16 LE samples in `pcm` to
// one output sample, with state from `c.halfRem` so chunk boundaries never split sample
// pairs. Returns a freshly allocated output buffer at half the input sample count.
func (c *chanReplayService) streamingHalveDecimate16to8(pcm []byte) []byte {
	// Prepend any carry-over partial pair from previous chunk.
	src := pcm
	if len(c.halfRem) > 0 {
		merged := make([]byte, 0, len(c.halfRem)+len(pcm))
		merged = append(merged, c.halfRem...)
		merged = append(merged, pcm...)
		src = merged
		c.halfRem = nil
	}
	// Each output sample = avg of 2 input samples = 4 input bytes → 2 output bytes.
	pairs := len(src) / 4
	tail := len(src) - pairs*4
	if tail > 0 {
		// Save trailing 1-3 bytes for next call (incomplete sample-pair).
		c.halfRem = append(c.halfRem[:0], src[pairs*4:]...)
	}
	if pairs == 0 {
		return nil
	}
	out := make([]byte, pairs*2)
	for i := 0; i < pairs; i++ {
		s1 := int32(int16(src[i*4]) | int16(src[i*4+1])<<8)
		s2 := int32(int16(src[i*4+2]) | int16(src[i*4+3])<<8)
		avg := int16((s1 + s2) >> 1)
		out[i*2] = byte(avg)
		out[i*2+1] = byte(avg >> 8)
	}
	return out
}

func (c *chanReplayService) SynthesizeStream(
	ctx context.Context,
	_ string,
	onPCMChunk func([]byte) error,
) error {
	if c == nil || c.ch == nil || onPCMChunk == nil {
		return errors.New("chanReplayService: nil deps")
	}
	if c.cloudSR <= 0 || c.bridgeSR <= 0 || c.cloudSR == c.bridgeSR {
		return c.streamPassthrough(ctx, onPCMChunk)
	}
	if c.cloudSR == 2*c.bridgeSR {
		return c.stream2to1Decimate(ctx, onPCMChunk)
	}
	return c.bufferAndResampleOnce(ctx, onPCMChunk)
}

func (c *chanReplayService) streamPassthrough(
	ctx context.Context,
	onPCMChunk func([]byte) error,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctxBlockChan:
			return context.Canceled
		case pcm, ok := <-c.ch:
			if !ok {
				return nil
			}
			if len(pcm) == 0 {
				continue
			}
			if len(pcm)&1 != 0 {
				pcm = pcm[:len(pcm)-1]
				if len(pcm) == 0 {
					continue
				}
			}
			if err := onPCMChunk(pcm); err != nil {
				return err
			}
		}
	}
}

func (c *chanReplayService) stream2to1Decimate(
	ctx context.Context,
	onPCMChunk func([]byte) error,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctxBlockChan:
			return context.Canceled
		case pcm, ok := <-c.ch:
			if !ok {
				// Drop any final unpaired sample (<2ms at 16k). Loss is sub-perceptual.
				c.halfRem = nil
				return nil
			}
			if len(pcm) == 0 {
				continue
			}
			out := c.streamingHalveDecimate16to8(pcm)
			if len(out) == 0 {
				continue
			}
			if err := onPCMChunk(out); err != nil {
				return err
			}
		}
	}
}

func (c *chanReplayService) bufferAndResampleOnce(
	ctx context.Context,
	onPCMChunk func([]byte) error,
) error {
	var all []byte
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctxBlockChan:
			return context.Canceled
		case pcm, ok := <-c.ch:
			if !ok {
				if len(all) == 0 {
					return nil
				}
				if len(all)&1 != 0 {
					all = all[:len(all)-1]
				}
				out, err := media.ResamplePCM(all, c.cloudSR, c.bridgeSR)
				if err != nil || len(out) == 0 {
					return err
				}
				return onPCMChunk(out)
			}
			if len(pcm) > 0 {
				all = append(all, pcm...)
			}
		}
	}
}
