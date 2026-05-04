package voicedialog

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/LingByte/SoulNexus/pkg/llm"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/sip/conversation"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

func (h *Hub) startInboundLoopbackWS(sess *dialogSession) {
	if h == nil || sess == nil || !h.cfg.InboundLoopbackWS {
		return
	}
	host := strings.TrimSpace(h.cfg.LoopbackHTTPHostPort)
	if host == "" {
		logger.Warn("voicedialog loopback ws skipped (empty LoopbackHTTPHostPort)")
		return
	}
	go h.runInboundLoopbackWS(sess.meta.CallID)
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

	env := conversation.VoiceEnvForVoicedialogLoopback()
	if !env.ReadyForVoicedialogLoopbackLLM() {
		logger.Warn("voicedialog loopback: LLM env incomplete — drain WS only (configure LLM_PROVIDER + LLM_APIKEY + LLM_BASEURL or Alibaba APP_ID, same as outbound SIP)",
			zap.String(KeyCallID, callID),
		)
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}

	ctx := context.Background()
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
				logger.Info("voicedialog loopback asr.final → LLM",
					zap.String(KeyCallID, callID),
					zap.Int("user_text_len", len([]rune(user))),
				)
				reply, err := conversation.VoicedialogLoopbackLLMQuery(ctx, prov, model, user)
				if err != nil {
					logger.Warn("voicedialog loopback LLM failed", zap.String(KeyCallID, callID), zap.Error(err))
					return
				}
				reply = strings.TrimSpace(reply)
				if reply == "" {
					return
				}
				out := map[string]any{
					KeyType:        CmdTTSSpeak,
					KeyCallID:      callID,
					KeyText:        reply,
					KeyUtteranceID: fmt.Sprintf("loopback-%d", time.Now().UnixNano()),
				}
				writeMu.Lock()
				werr := writeJSONDeadline(c, out)
				writeMu.Unlock()
				if werr != nil {
					logger.Warn("voicedialog loopback tts.speak write failed", zap.String(KeyCallID, callID), zap.Error(werr))
					return
				}
				logger.Info("voicedialog loopback sent tts.speak",
					zap.String(KeyCallID, callID),
					zap.Int("reply_len", len([]rune(reply))),
				)
				if conversation.VoicedialogShouldTriggerTransfer(callID, user, prov) {
					conversation.TriggerTransferToAgent(context.Background(), callID, logger.Lg)
				}
			}(text)

		case EvCallEnded:
			return
		default:
		}
	}
}
