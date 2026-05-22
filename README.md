# LingEchoX

LingEchoX is an AI-powered voice contact center platform.  
It combines SIP telephony, outbound campaigns, script-driven calling, ASR/TTS orchestration, and a React-based operations console.

## Highlights

- SIP-focused contact center backend built with Go and Gin.
- Frontend console built with React + Vite + TypeScript.
- Supports outbound campaign lifecycle: create, start, pause, resume, stop.
- Script template management for call flow orchestration.
- Multi-provider speech ecosystem (ASR/TTS/LLM) through configurable integrations.
- SQLite by default, with PostgreSQL/MySQL options.

## Tech Stack

- **Backend:** Go, Gin, GORM
- **Frontend:** React 18, Vite, TypeScript, Tailwind CSS
- **Telephony & Media:** SIP, RTP, DTMF, embedded SIP stack
- **Storage/Infra (optional):** Redis, object storage providers, Neo4j

## Repository Structure

```text
.
├── cmd/
│   ├── bootstrap/        # config/db/router bootstrap shared by server entry
│   └── server/           # HTTP API + embedded SIP entry point
├── internal/             # handlers, models, SIP app glue logic
├── pkg/                  # reusable modules (sip/media/asr/tts/llm/config/logger)
├── web/                  # frontend app (React + Vite + TypeScript)
├── scripts/              # reserved scripts directory
├── env.example           # environment configuration template
└── docs/                 # project docs and roadmap
```

## Quick Start

### 1) Prerequisites

- Go 1.24+
- Node.js 18+ (recommended)
- npm / pnpm / yarn

### 2) Backend setup

```bash
cp env.example .env
go mod download
go run ./cmd/server
```

Tenant integrations can call HTTP APIs with **access key + secret key** only (no user password). Sign each request with your AK/SK and include the AK/SK headers expected by the platform.

Default API server address:

- `http://localhost:7072`

### 3) Frontend setup

```bash
cd web
npm install
npm run dev
```

By default Vite will expose a local dev URL (typically `http://localhost:5173`).

## Core Functional Areas

- **SIP Users**: manage SIP user accounts.
- **Call Records**: query and inspect call sessions.
- **Number Pool**: manage number pool resources.
- **Outbound Tasks**: launch and manage outbound campaigns.
- **Script Manager**: maintain script templates for outbound workflows.
- **Web Agents**: web-side agent capabilities in UI.

## Environment Configuration

Key configuration is provided through `.env` (see `env.example`), including:

- Server mode/address and API prefixes
- Database driver/DSN
- SIP/voice provider credentials
- LLM provider settings
- Search/backup/monitoring toggles
- Storage/cache/SSL options

## Documentation

- Project status and module map: `docs/project-overview.md`
- Future features and development roadmap: `docs/roadmap.md`

## License

This project includes an AGPL-3.0 license file (`LICENSE`).


// Package sipenv documents environment variables consulted under pkg/sip (and tightly coupled outbound/script paths).
//
// Unless noted, variables are optional—omit them and the code uses the documented default.
//
// # Persistence (pkg/sip/persist)
//
//   - SIP calls / registrar rows use GORM via app DB (see cmd/bootstrap migrations); no separate SIP_PERSIST file mode.
//
// # SIP server / RTP (pkg/sip/server, pkg/sip/session)
//
//   - SIP_RTP_PORT: fixed RTP listen port (>0).
//   - SIP_RTP_PORT_START / SIP_RTP_PORT_END: allocate RTP from this UDP range (rotation).
//   - SIP_MEDIA_TX_QUEUE_SIZE: outbound media queue depth (default 512, clamped 64–2048).
//   - SIP_MEDIA_MAX_SECONDS: max AI media session duration per call in seconds (default 3600; 0 = unlimited).
//
// # Voice pipeline ASR / LLM / TTS (pkg/sip/conversation — credentials required for AI audio)
//
//   - ASR_APPID, ASR_SECRET_ID, ASR_SECRET_KEY, ASR_MODEL_TYPE
//   - LLM_PROVIDER, LLM_BASEURL, LLM_APIKEY, LLM_MODEL, LLM_APP_ID (or ALIBABA_AI_APP_ID + ALIBABA_AI_API_KEY for alibaba)
//   - TTS_APPID, TTS_SECRET_ID, TTS_SECRET_KEY, TTS_VOICE_TYPE, TTS_SPEED, TTS_SAMPLE_RATE (default 16000 when unset)
//
// # SIP conversation tuning (defaults apply when unset)
//
//   - SIP_AI_HANGUP_PHRASES: comma-separated phrases (default Chinese goodbye list).
//   - SIP_VAD_BARGE_IN: 0/false/off/no disables RMS barge-in during TTS (default enabled).
//   - SIP_VAD_THRESHOLD: RMS threshold (default 3200).
//   - SIP_VAD_CONSEC_FRAMES: frames for barge-in (default 3).
//   - SIP_WELCOME_WAIT_FIRST_RTP_MS: delay before welcome WAV (default 2000; 0 disables wait).
//   - SIP_WELCOME_WAV_PATH: optional welcome clip path.
//
// # Transfer / bridge (pkg/sip/conversation, pkg/sip/bridge)
//
//   - SIP_TRANSFER_RINGING_WAV_PATH: clip during transfer ringback.
//   - SIP_TRANSFER_GOODBYE_TAIL_MS: tail timing for transfer goodbye.
//
// # WebSeat WebRTC (pkg/sip/webseat)
//
//   - SIP_WEBSEAT_WS_TOKEN: shared secret for browser WS (empty = accept any; dev only).
//   - SIP_WEBSEAT_JOIN_TIMEOUT: browser join deadline.
//   - SIP_WEBSEAT_TRACK_WAIT: wait for first audio track after SDP answer.
//   - SIP_WEBSEAT_ICE_SERVERS: JSON ICE server list for peer connection.
//
// # Registrar / presence
//
//   - CONVERSATION_WEBSEAT_SIP_DOMAIN: SIP domain hint for WebSeat-related registrar rows (pkg/sip/persist/sip_user.go).
//
// # Hotword ASR corrections (pkg/sip/conversation/hotword_rms_compat.go)
//
//   - SIP_HOTWORD_CORRECTIONS_JSON, SIP_HOTWORD_CORRECTIONS
//
// # Outbound / ACD / hybrid script (pkg/sip/outbound, pkg/constants)
//
//   - Outbound gateway: configure Trunk LocalAddr + TrunkNumber direction in DB (no SIP_OUTBOUND_* / SIP_TARGET_NUMBER).
//   - SIP_CALLER_ID, SIP_CALLER_DISPLAY_NAME,
//     SIP_DEFAULT_DOMAIN, SIP_DEFAULT_URI_PORT, SIP_PASSWORD (REGISTER auth),
//     SIP_TRANSFER_* (host/port/sig/request-uri/number),
//     SIP_SCRIPT_LISTEN_AFTER_TTS_TAIL, SIP_SCRIPT_LISTEN_TAIL_MS_MAX/MIN, SIP_SCRIPT_LISTEN_POLL_MS,
//     SIP_SCRIPT_LLM_FAIL_PROMPT,
//     CHECK_LLM_PROVIDER, CHECK_LLM_BASEURL, CHECK_LLM_APIKEY, CHECK_LLM_MODEL,
//     CHECK_LLM_ROUTE_TIMEOUT_MS, CHECK_LLM_ROUTE_DISABLED, CHECK_LLM_ROUTE_LEGACY_JSON, CHECK_LLM_ROUTE_MAX_TOKENS
//
// Credentials and routing keys above have no safe default; tuning keys use defaults in code when unset.
package sipenv
