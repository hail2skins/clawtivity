# Clawtivity

A self-hosted, local-first activity feed and memory tracking service for OpenClaw agents.

## Overview

Clawtivity provides:
- structured activity logging from hooks or agents
- turn-level memory storage
- query and summary APIs for reporting
- Swagger/OpenAPI docs for API consumers

## Current Tech Stack

- **Language:** Go
- **HTTP Framework:** Gin
- **ORM:** GORM
- **Database:** SQLite (local-first)
- **Templating/UI:** Templ + Tailwind
- **API Docs:** swaggo + gin-swagger

## Quick Start

### Prerequisites

- Go 1.25+
- Air (optional, for live reload)

### Install

```bash
git clone https://github.com/hail2skins/clawtivity.git
cd clawtivity
go mod download
```

### Run

```bash
make run
```

By default the API runs on port `18730` (override with `PORT`).

### Test

```bash
make test
```

## API Endpoints

### Activity

- `POST /api/activity`
  - Create an activity entry.
- `GET /api/activity`
  - List activity entries.
  - Supported query params:
    - `project` (maps to `project_tag`)
    - `model`
    - `date` (`YYYY-MM-DD`, filters by `created_at` day)
- `GET /api/activity/summary`
  - Aggregated stats (`count`, token totals, cost total, duration total, grouped status counts).
  - Supports the same filters as `GET /api/activity`.

### Health

- `GET /health`
  - Service/database health information.

### Swagger UI

- `GET /swagger/index.html`
  - Interactive OpenAPI UI.
- Generated spec artifacts:
  - `docs/swagger.json`
  - `docs/swagger.yaml`

## OpenClaw Integration

### Skill (CLAW-7)

Repository source:
- `skills/clawtivity/SKILL.md`
- `skills/clawtivity/scripts/log_activity.py`

Local install path:
- `~/.openclaw/skills/clawtivity/`

Install/update locally:

```bash
mkdir -p ~/.openclaw/skills/clawtivity
cp -R skills/clawtivity/. ~/.openclaw/skills/clawtivity/
```

This skill script is the ingestion client for the hook path (legacy/optional).
It handles retry, fallback queueing, and replay.

### Plugin (CLAW-16, primary/reliable)

Repository source:
- `plugins/clawtivity-activity/openclaw.plugin.json`
- `plugins/clawtivity-activity/index.js`

Install and enable:

```bash
openclaw plugins install ./plugins/clawtivity-activity
openclaw plugins enable clawtivity-activity
openclaw plugins list --json
```

Behavior:
- listens to `llm_output`, `message_received`, `message_sending`, and `agent_end`
- uses `agent_end` as the primary write trigger for reliable turn logging
- captures assistant turn outcomes (`success` / `failed`) and best-effort model/token usage
- posts normalized JSON directly to `POST /api/activity` via in-plugin JS

Optional plugin config fields (in OpenClaw plugin config):
- `apiUrl` (default `http://localhost:18730/api/activity`)
- `projectTag`
- `userId`

### Hook (CLAW-15, legacy/optional)

Repository source:
- `skills/clawtivity/hook/HOOK.md`
- `skills/clawtivity/hook/handler.ts`

Local install path:
- `~/.openclaw/hooks/clawtivity/`

Install/update locally:

```bash
mkdir -p ~/.openclaw/hooks/clawtivity
cp skills/clawtivity/hook/HOOK.md ~/.openclaw/hooks/clawtivity/HOOK.md
cp skills/clawtivity/hook/handler.ts ~/.openclaw/hooks/clawtivity/handler.ts
```

The hook is configured for `message:sent` and pipes normalized turn JSON into:

```bash
echo "$JSON" | python3 ~/.openclaw/skills/clawtivity/scripts/log_activity.py
```

### Retry/Fallback Behavior

- POST target: `http://localhost:18730/api/activity`
- Retries: `1s`, `2s`, `4s` exponential backoff (3 attempts total)
- Fallback queue on failure: `~/.clawtivity/queue/YYYY-MM-DD.md`
- Queue replay on next successful POST

Retry/fallback behavior:
- plugin path: JS-native retry in `plugins/clawtivity-activity/index.js`
- skill path: retry + fallback queue + replay in `skills/clawtivity/scripts/log_activity.py` (legacy/optional)

### Verify Wiring

```bash
openclaw plugins list --json
openclaw hooks list --json
openclaw plugins enable clawtivity-activity
openclaw hooks disable clawtivity
```

Then send a message/turn through OpenClaw and verify ingestion:

```bash
curl "http://localhost:18730/api/activity"
curl "http://localhost:18730/api/activity/summary"
```

Expected behavior:
- new rows should be written on bot turn completion
- legacy `pending` rows may exist from older hook-based runs, but new plugin writes should be `success`/`failed`

If plugin install state gets stuck:

```bash
printf 'y\n' | openclaw plugins uninstall clawtivity-activity
rm -rf ~/.openclaw/extensions/clawtivity-activity
openclaw plugins install ./plugins/clawtivity-activity
openclaw plugins enable clawtivity-activity
```

## Data Model Snapshot

### `activity_feed`

Fields:
- `id` (UUID, primary key)
- `session_key` (indexed)
- `model`
- `tokens_in`
- `tokens_out`
- `cost_estimate`
- `duration_ms`
- `project_tag` (indexed)
- `external_ref`
- `category` (indexed)
- `thinking`
- `reasoning`
- `channel`
- `status` (indexed)
- `user_id` (indexed)
- `created_at`

### `turn_memories`

Fields:
- `id` (UUID, primary key)
- `session_key` (indexed)
- `summary`
- `tools_used` (JSON)
- `files_touched` (JSON)
- `key_decisions` (JSON)
- `context_snippet`
- `tags` (JSON)
- `created_at`

## Development Notes

- SQLite schema is managed through GORM `AutoMigrate` on startup.
- API handlers are test-driven in `internal/server`.
- Database schema and adapter behavior are test-driven in `internal/database`.

## Contributing Rules

1. Work in `dev` or feature branches first.
2. Every commit must include a Jira ticket key (example: `[CLAW-123]`).
3. Follow TDD: tests first, implementation second.

## License

MIT
