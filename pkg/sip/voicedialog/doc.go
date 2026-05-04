// Package voicedialog bridges inbound SIP legs to HTTP WebSocket clients for LLM-centric dialogue.
//
// SIP gateway responsibilities:
//   - RTP ↔ PCM, ASR streaming (Tencent/QCloud by env), TTS playback from HTTP commands
//   - DTMF and RMS VAD barge-in while TTS is playing
//
// HTTP / WebSocket client responsibilities:
//   - Receive ASR and signaling events, call your LLM, send tts.speak with assistant text
//
// Voicedialog-specific env:
//   - VOICE_DIALOG_WS_TOKEN — shared secret for GET .../ws?token=... (empty = dev only; set in production).
//
// Gateway media (read in attachGatewayMedia): ASR_APPID, ASR_SECRET_ID, ASR_SECRET_KEY, ASR_MODEL_TYPE;
// TTS_APPID, TTS_SECRET_ID, TTS_SECRET_KEY; optional TTS_VOICE_TYPE, TTS_SPEED, TTS_SAMPLE_RATE.
//
// SIP_WELCOME_WAV_PATH — welcome clip (URL, absolute path, or scripts/ name); default scripts/welcome.wav.
//
// SIP_TRANSFER_RINGING_WAV_PATH — transfer ringback WAV (see pkg/sip/conversation/transfer.go); when set,
// the same path is used for voicedialog transfer “loading” loop audio before ringing.
//
// Inbound UAS attaches this bridge on ACK. Outbound AI legs use conversation.AttachVoicePipeline from sipapp.
// In-process loopback to HTTP /ws?call_id= is always enabled from sipapp (InboundLoopbackWS: true).
//
// HTTP GET upgrade (method, token, status responses) is implemented in internal/handlers/voice_dialog.go;
// this package exposes UpgradeVoiceDialogWebSocket + ServeVoiceDialogWebSocket after handshake.
//
// WebSocket URLs (under HTTP API prefix, e.g. /api/):
//   - GET .../lingecho/voice-dialog/v1/ws?token=T — subscribe; receives call.pending for new inbound legs.
//   - GET .../lingecho/voice-dialog/v1/ws?token=T&call_id=C — session for Call-ID C.
//
// Protocol v2 (JSON):
//
// Gateway → client (upstream): hello, call.pending, asr.partial, asr.final, asr.error, dtmf,
// interrupt { origin, cause, reason? }, tts.started, tts.ended, tts.cancelled, call.ended,
// dialog.welcome { phase: started|playing|ended|skipped|error, source_kind?, source?, detail? },
// dialog.transfer { phase: requested|loading|ringing|connected|failed|no_agent, source_kind?, source?, … }.
//
// Client → gateway (downstream): tts.speak { text, utterance_id? }, tts.cancel, interrupt { reason? }, hangup, ping.
//
// Per-call WebSocket disconnect triggers SIP hangup. Loopback client runs asr.final → LLM → tts.speak when LLM env is configured.
//
// Logging: inbound WebSocket commands and gateway ASR/TTS events use pkg/logger (global Lg).
package voicedialog
