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
├── cmd/                 # server startup and bootstrap
├── internal/            # handlers, models, SIP app glue logic
├── pkg/                 # reusable modules (sip/media/asr/tts/llm/config/logger)
├── qiniu/               # frontend app (React + Vite)
├── scripts/             # reserved scripts directory
├── env.example          # environment configuration template
└── docs/                # project docs and roadmap
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

Default API server address:

- `http://localhost:7072`

### 3) Frontend setup

```bash
cd qiniu
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

