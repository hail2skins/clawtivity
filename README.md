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
