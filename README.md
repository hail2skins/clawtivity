# Clawtivity

A self-hosted, local-first activity feed and memory tracking service for OpenClaw agents.

## Overview

Clawtivity provides:
- structured activity logging from OpenClaw agents
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

## Database Support (Current)

- Supported database engine: **SQLite only** (at this stage).
- Default DB file: `./test.db` (unless overridden by `BLUEPRINT_DB_URL`).
- GORM `AutoMigrate` runs automatically on startup for the configured SQLite database.
- You do **not** need the `sqlite3` CLI tool installed; the app uses the Go SQLite driver.

Planned support for PostgreSQL/MySQL will be added in a separate ticket.

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

### Skill Script (CLAW-7, optional utility)

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

This script can be used manually for payload posting/replay workflows.

### Plugin (CLAW-16, primary/reliable)

Repository source:
- `plugins/clawtivity-activity/openclaw.plugin.json`
- `plugins/clawtivity-activity/index.js`

Install and enable (default OpenClaw security):

```bash
openclaw plugins install ./plugins/clawtivity-activity
openclaw plugins enable clawtivity-activity
openclaw gateway restart
openclaw plugins list --json
```

Install and enable (hardened allowlist security):

```bash
cp ~/.openclaw/openclaw.json ~/.openclaw/openclaw.json.bak.codex
openclaw plugins install ./plugins/clawtivity-activity
openclaw plugins enable clawtivity-activity

tmp="$HOME/.openclaw/openclaw.json.tmp.codex"
jq '
  .plugins = (.plugins // {}) |
  .plugins.allow = ((.plugins.allow // []) + ["clawtivity-activity","memory-core","discord","telegram"] | unique)
' "$HOME/.openclaw/openclaw.json" > "$tmp" && mv "$tmp" "$HOME/.openclaw/openclaw.json"

openclaw gateway restart
openclaw plugins list --json | jq '.plugins[] | {id,enabled,status,error} | select(.id=="clawtivity-activity" or .id=="memory-core" or .id=="discord" or .id=="telegram")'
```

If `openclaw gateway restart` says `Gateway service not loaded`, initialize it once:

```bash
openclaw gateway install
openclaw gateway start
```

Behavior:
- listens to `llm_output`, `message_received`, `message_sending`, and `agent_end`
- uses `agent_end` as the primary write trigger for reliable turn logging
- captures assistant turn outcomes (`success` / `failed`) and best-effort model/token usage
- posts normalized JSON directly to `POST /api/activity` via in-plugin JS
- on API outage, writes fallback payloads to local queue markdown files

Optional plugin config fields (in OpenClaw plugin config):
- `apiUrl` (default `http://localhost:18730/api/activity`)
- `queueRoot` (default `~/.clawtivity/queue`)
- `projectTag`
- `userId`

### Retry/Fallback Behavior

- POST target: `http://localhost:18730/api/activity`
- Retries: `1s`, `2s`, `4s` exponential backoff (3 attempts total)
- Fallback queue on failure: `~/.clawtivity/queue/YYYY-MM-DD.md` (home directory)
- Queue replay occurs on API startup flush

Retry/fallback behavior:
- plugin path: JS-native retry + write-only queue fallback in `plugins/clawtivity-activity/index.js`
- skill script path: retry + fallback queue + replay in `skills/clawtivity/scripts/log_activity.py` (optional utility)

Queue replay behavior:
- API startup automatically drains queue files from `~/.clawtivity/queue` (or `CLAWTIVITY_QUEUE_DIR`)
- successfully imported entries are removed from queue files
- empty queue files are deleted

### Categorization (CLAW-22)

- Activity categorization is rule-based and deterministic.
- Seed rule file: `internal/classifier/category_rules.json`
- Default category remains `general`.
- API classifies from available signals (`prompt_text`, `assistant_text`, `tools_used`) and writes `category_reason` for auditability.

### Verify Wiring

```bash
openclaw plugins list --json
openclaw plugins enable clawtivity-activity
```

Then send a message/turn through OpenClaw and verify ingestion:

```bash
curl "http://localhost:18730/api/activity"
curl "http://localhost:18730/api/activity/summary"
```

Expected behavior:
- new rows should be written on bot turn completion
- plugin writes should be `success`/`failed`

If plugin install state gets stuck:

```bash
printf 'y\n' | openclaw plugins uninstall clawtivity-activity
rm -rf ~/.openclaw/extensions/clawtivity-activity
openclaw plugins install ./plugins/clawtivity-activity
openclaw plugins enable clawtivity-activity
```

If plugin is installed/enabled but shows `not in allowlist`:

```bash
cp ~/.openclaw/openclaw.json ~/.openclaw/openclaw.json.bak.codex
tmp="$HOME/.openclaw/openclaw.json.tmp.codex"
jq '
  .plugins = (.plugins // {}) |
  .plugins.allow = ((.plugins.allow // []) + ["clawtivity-activity","memory-core","discord","telegram"] | unique)
' \
  "$HOME/.openclaw/openclaw.json" > "$tmp" && mv "$tmp" "$HOME/.openclaw/openclaw.json"
openclaw gateway restart
```

If OpenClaw fails with:
`Invalid config ... plugins.allow: plugin not found: clawtivity-activity`

This means the plugin is currently not installed but still listed in `plugins.allow`.
Recover with:

```bash
cp ~/.openclaw/openclaw.json ~/.openclaw/openclaw.json.bak.codex
tmp="$HOME/.openclaw/openclaw.json.tmp.codex"
jq '.plugins = (.plugins // {}) | .plugins.allow = ((.plugins.allow // []) | map(select(. != "clawtivity-activity")))' \
  "$HOME/.openclaw/openclaw.json" > "$tmp" && mv "$tmp" "$HOME/.openclaw/openclaw.json"

openclaw plugins install ./plugins/clawtivity-activity
openclaw plugins enable clawtivity-activity

tmp="$HOME/.openclaw/openclaw.json.tmp.codex"
jq '
  .plugins = (.plugins // {}) |
  .plugins.allow = ((.plugins.allow // []) + ["clawtivity-activity","memory-core","discord","telegram"] | unique)
' "$HOME/.openclaw/openclaw.json" > "$tmp" && mv "$tmp" "$HOME/.openclaw/openclaw.json"

openclaw gateway restart
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
- `category_reason`
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

## License

MIT
