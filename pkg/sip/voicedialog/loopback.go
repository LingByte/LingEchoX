package voicedialog

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/llm"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/sip/conversation"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// loopbackSoftCutMinRunes returns the minimum rune count required before allowing a soft
// punctuation (comma / colon / 顿号 / ：) to act as a sentence boundary inside the streaming
// LLM → TTS segmenter. Tunable via VOICEDIALOG_SOFT_CUT_MIN_RUNES (default 6). Values <2
// are clamped to 2 to avoid speaking "您，" as its own utterance.
func loopbackSoftCutMinRunes() int {
	const defaultVal = 6
	const minVal = 2
	s := strings.TrimSpace(os.Getenv("VOICEDIALOG_SOFT_CUT_MIN_RUNES"))
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < minVal {
		return defaultVal
	}
	return n
}

// findSentenceCut returns the byte index AFTER the first sentence-end punctuation in s,
// or 0 if none found. Threshold runeMin avoids splitting on ultra-short fragments.
//
// Strong terminators (always cut): 。 ！ ？ ； . ! ? ; \n
// Soft terminators (cut only when buffer >= runeMin runes): ， , 、 ：
func findSentenceCut(s string, runeMin int) int {
	if s == "" {
		return 0
	}
	runeCount := 0
	for i, r := range s {
		runeCount++
		switch r {
		case '。', '！', '？', '；', '!', '?', ';', '\n':
			return i + utf8.RuneLen(r)
		case '.':
			// avoid splitting on numerics like "3.14"
			if runeCount >= runeMin {
				return i + utf8.RuneLen(r)
			}
		case '，', ',', '、', '：', ':':
			if runeCount >= runeMin {
				return i + utf8.RuneLen(r)
			}
		}
	}
	return 0
}

func (h *Hub) startInboundLoopbackWS(sess *dialogSession) {
	if h == nil || sess == nil {
		return
	}
	if custom := strings.TrimSpace(sess.meta.CustomVoiceWSURL); custom != "" {
		go h.runInboundAssistantWS(sess.meta.CallID, custom)
		return
	}
	if !h.cfg.InboundLoopbackWS {
		return
	}
	host := strings.TrimSpace(h.cfg.LoopbackHTTPHostPort)
	if host == "" {
		logger.Warn("voicedialog loopback ws skipped (empty LoopbackHTTPHostPort)")
		return
	}
	go h.runInboundLoopbackWS(sess.meta.CallID)
}

func (h *Hub) runInboundAssistantWS(callID string, target string) {
	if h == nil {
		return
	}
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		logger.Warn("voicedialog inbound ws skipped (invalid custom URL)",
			zap.String(KeyCallID, callID),
			zap.String("target", target),
			zap.Error(err),
		)
		return
	}
	if u == nil || (u.Scheme != "ws" && u.Scheme != "wss") {
		logger.Warn("voicedialog inbound ws skipped (invalid custom URL)",
			zap.String(KeyCallID, callID),
			zap.String("target", target),
		)
		return
	}
	q := u.Query()
	tok := wsTokenExpected()
	if tok != "" {
		q.Set("token", tok)
	}
	q.Set("call_id", callID)
	u.RawQuery = q.Encode()
	full := u.String()

	d := websocket.Dialer{
		HandshakeTimeout: 12 * time.Second,
	}
	if u.Scheme == "wss" {
		d.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: h.cfg.LoopbackTLSInsecureSkipVerify,
			MinVersion:         tls.VersionTLS12,
		}
	}

	var conn *websocket.Conn
	var dialErr error
	const retries = 60
	const delay = 100 * time.Millisecond
	for i := 0; i < retries; i++ {
		conn, _, dialErr = d.Dial(full, nil)
		if dialErr == nil {
			break
		}
		time.Sleep(delay)
	}
	if dialErr != nil {
		logHostPath := u.Scheme + "://" + u.Host + u.Path
		logger.Warn("voicedialog custom ws dial failed",
			zap.String(KeyCallID, callID),
			zap.String("dial_target", logHostPath),
			zap.Error(dialErr),
		)
		return
	}
	logger.Info("voicedialog custom ws connected (tenant voice dialog URL)",
		zap.String(KeyCallID, callID),
	)
	go runLoopbackAssistant(callID, conn)
}

func (h *Hub) runInboundLoopbackWS(callID string) {
	scheme := "ws"
	if h.cfg.LoopbackUseTLS {
		scheme = "wss"
	}
	api := strings.Trim(h.cfg.APIPrefix, "/")
	path := fmt.Sprintf("/%s/%s/ws", api, constants.LingechoVoiceDialogPathPrefix)

	q := url.Values{}
	tok := wsTokenExpected()
	if tok != "" {
		q.Set("token", tok)
	}
	q.Set("call_id", callID)
	full := fmt.Sprintf("%s://%s%s?%s", scheme, h.cfg.LoopbackHTTPHostPort, path, q.Encode())

	d := websocket.Dialer{
		HandshakeTimeout: 12 * time.Second,
	}
	if h.cfg.LoopbackUseTLS {
		d.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: h.cfg.LoopbackTLSInsecureSkipVerify,
			MinVersion:         tls.VersionTLS12,
		}
	}

	var conn *websocket.Conn
	var err error
	const retries = 60
	const delay = 100 * time.Millisecond
	for i := 0; i < retries; i++ {
		conn, _, err = d.Dial(full, nil)
		if err == nil {
			break
		}
		time.Sleep(delay)
	}
	if err != nil {
		logger.Warn("voicedialog loopback ws dial failed",
			zap.String(KeyCallID, callID),
			zap.String("dial_target", scheme+"://"+h.cfg.LoopbackHTTPHostPort+path),
			zap.Error(err),
		)
		return
	}

	logger.Info("voicedialog loopback ws connected (in-process client → HTTP upgrade)",
		zap.String(KeyCallID, callID),
	)
	go runLoopbackAssistant(callID, conn)
}

// runLoopbackAssistant reads gateway events on the loopback socket; on asr.final runs LLM and sends tts.speak.
func runLoopbackAssistant(callID string, c *websocket.Conn) {
	defer func() { _ = c.Close() }()

	cs := conversation.LookupInboundCallSession(callID)
	ctx := context.Background()
	env, loaded, err := conversation.ResolveTenantVoiceEnv(ctx, cs)
	if cs == nil || err != nil || !loaded || !env.ReadyForVoicedialogLoopbackLLM() {
		if cs == nil {
			logger.Warn("voicedialog loopback: no inbound CallSession — drain WS only",
				zap.String(KeyCallID, callID),
			)
		} else if err != nil {
			logger.Warn("voicedialog loopback: tenant voice env error — drain WS only",
				zap.String(KeyCallID, callID),
				zap.Error(err),
			)
		} else {
			logger.Warn("voicedialog loopback: tenant llmConfig incomplete — drain WS only",
				zap.String(KeyCallID, callID),
			)
		}
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}

	var prov llm.LLMProvider
	var model string
	var cleanup func()
	var writeMu sync.Mutex
	var turnMu sync.Mutex
	inFlight := false

	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		rawTyp, _ := msg[KeyType].(string)
		typ := strings.ToLower(strings.TrimSpace(rawTyp))

		switch typ {
		case EvHello:
			if prov != nil {
				continue
			}
			var ierr error
			prov, model, cleanup, ierr = conversation.NewVoicedialogLoopbackLLMProvider(ctx, callID, logger.Lg)
			if ierr != nil {
				logger.Warn("voicedialog loopback LLM init failed", zap.String(KeyCallID, callID), zap.Error(ierr))
				continue
			}
			logger.Info("voicedialog loopback LLM session started",
				zap.String(KeyCallID, callID),
				zap.String("model", model),
				zap.String("llm_provider", strings.TrimSpace(env.LLMProvider)),
			)

		case EvASRFinal:
			text, _ := msg[KeyText].(string)
			text = strings.TrimSpace(text)
			if text == "" || prov == nil {
				continue
			}
			// If transfer is already in progress, do not start a new LLM turn — the user
			// is no longer talking to the AI. Drop the final transcript silently; ASR feed
			// itself is also gated upstream so this branch is mostly defensive.
			if conversation.IsTransferInProgress(callID) {
				logger.Info("voicedialog loopback asr.final dropped (transfer in progress)",
					zap.String(KeyCallID, callID),
					zap.Int("user_text_len", len([]rune(text))),
				)
				continue
			}
			turnMu.Lock()
			if inFlight {
				turnMu.Unlock()
				continue
			}
			inFlight = true
			turnMu.Unlock()

			go func(user string) {
				defer func() {
					turnMu.Lock()
					inFlight = false
					turnMu.Unlock()
				}()
				logger.Info("voicedialog loopback asr.final → LLM (stream)",
					zap.String(KeyCallID, callID),
					zap.Int("user_text_len", len([]rune(user))),
				)
				llmT0 := time.Now()
				parentUtteranceID := fmt.Sprintf("loopback-%d", time.Now().UnixNano())

				var (
					segBuf             strings.Builder
					segIdx             int
					firstDeltaLogged   bool
					firstSegSentAt     time.Time
					lastSegSentAt      time.Time
					lastSegUtteranceID string
				)

				flushSegment := func(text string, isLast bool) {
					text = strings.TrimSpace(text)
					if text == "" {
						return
					}
					// Mid-stream transfer guard: if transfer started while the LLM was still
					// streaming, stop emitting further tts.speak segments. Already-queued
					// segments are killed via invalidateQueuedTTS in deliverConversationTransferPhase.
					if conversation.IsTransferInProgress(callID) {
						logger.Info("voicedialog loopback tts segment skipped (transfer in progress)",
							zap.String(KeyCallID, callID),
							zap.Int("seg_len", len([]rune(text))),
						)
						return
					}
					segIdx++
					utterID := fmt.Sprintf("%s-s%d", parentUtteranceID, segIdx)
					if firstSegSentAt.IsZero() {
						firstSegSentAt = time.Now()
						logger.Info("voicedialog loopback first tts segment emit",
							zap.String(KeyCallID, callID),
							zap.Duration("llm_to_first_segment", time.Since(llmT0)),
							zap.Int("seg_len", len([]rune(text))),
						)
					}
					lastSegSentAt = time.Now()
					lastSegUtteranceID = utterID
					if defaultHub != nil {
						defaultHub.mu.Lock()
						ds := defaultHub.sessions[strings.TrimSpace(callID)]
						defaultHub.mu.Unlock()
						if ds != nil {
							ds.beginPendingTTS()
						}
					}
					out := map[string]any{
						KeyType:        CmdTTSSpeak,
						KeyCallID:      callID,
						KeyText:        text,
						KeyUtteranceID: utterID,
					}
					writeMu.Lock()
					werr := writeJSONDeadline(c, out)
					writeMu.Unlock()
					if werr != nil {
						// roll back the pending counter we just bumped
						if defaultHub != nil {
							defaultHub.mu.Lock()
							ds := defaultHub.sessions[strings.TrimSpace(callID)]
							defaultHub.mu.Unlock()
							if ds != nil {
								ds.endPendingTTSWithoutTransfer()
							}
						}
						logger.Warn("voicedialog loopback tts.speak segment write failed",
							zap.String(KeyCallID, callID), zap.Error(werr))
						return
					}
					logger.Info("voicedialog loopback sent tts.speak segment",
						zap.String(KeyCallID, callID),
						zap.String(KeyUtteranceID, utterID),
						zap.Int("seg_len", len([]rune(text))),
						zap.Bool("last", isLast),
					)
				}

				softCutMinRunes := loopbackSoftCutMinRunes()
				fullReply, err := conversation.VoicedialogLoopbackLLMStream(ctx, prov, model, user,
					func(delta string, isFinal bool) error {
						if delta != "" {
							if !firstDeltaLogged {
								firstDeltaLogged = true
								logger.Info("voicedialog loopback LLM first delta",
									zap.String(KeyCallID, callID),
									zap.Duration("ttfb", time.Since(llmT0)),
								)
							}
							segBuf.WriteString(delta)
							for {
								cur := segBuf.String()
								cut := findSentenceCut(cur, softCutMinRunes)
								if cut <= 0 {
									break
								}
								head := cur[:cut]
								rest := cur[cut:]
								segBuf.Reset()
								segBuf.WriteString(rest)
								flushSegment(head, false)
							}
						}
						if isFinal {
							rem := strings.TrimSpace(segBuf.String())
							segBuf.Reset()
							if rem != "" {
								flushSegment(rem, true)
							}
						}
						return nil
					},
				)
				llmWall := int(time.Since(llmT0).Milliseconds())
				if err != nil {
					logger.Warn("voicedialog loopback LLM stream failed",
						zap.String(KeyCallID, callID), zap.Error(err))
					// flush any buffered tail before bail-out
					if rem := strings.TrimSpace(segBuf.String()); rem != "" {
						segBuf.Reset()
						flushSegment(rem, true)
					}
					return
				}
				fullReply = strings.TrimSpace(fullReply)
				if defaultHub != nil {
					defaultHub.mu.Lock()
					ds := defaultHub.sessions[strings.TrimSpace(callID)]
					defaultHub.mu.Unlock()
					if ds != nil {
						ds.setPendingLoopbackLLM(model, llmWall)
					}
				}
				logger.Info("voicedialog loopback LLM stream done",
					zap.String(KeyCallID, callID),
					zap.Int("reply_len", len([]rune(fullReply))),
					zap.Int("segments", segIdx),
					zap.Int("llm_wall_ms", llmWall),
					zap.Time("last_seg_sent_at", lastSegSentAt),
					zap.String("last_utterance_id", lastSegUtteranceID),
				)
				if segIdx == 0 {
					return
				}
				if conversation.VoicedialogShouldTriggerTransfer(callID, prov) {
					if defaultHub != nil {
						defaultHub.mu.Lock()
						ds := defaultHub.sessions[strings.TrimSpace(callID)]
						defaultHub.mu.Unlock()
						if ds != nil {
							ds.markTransferAfterNextTTS()
							// If queued segments already drained (fast playback), fire now.
							if ds.pendingTTSEmpty() && ds.consumeTransferAfterNextTTS() {
								logger.Info("voicedialog loopback: transfer fires immediately (no pending TTS)",
									zap.String(KeyCallID, callID),
								)
								go conversation.TriggerTransferToAgent(context.Background(), callID, logger.Lg)
							} else {
								logger.Info("voicedialog loopback: transfer deferred until last TTS segment ends",
									zap.String(KeyCallID, callID),
								)
							}
						}
					}
				}
			}(text)

		case EvCallEnded:
			return
		default:
		}
	}
}
