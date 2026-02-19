---
name: clawtivity
description: Log OpenClaw turn activity to a local Clawtivity server (http://localhost:18730) with retry, fallback queueing, and replay. Use for automatic activity telemetry after each agent turn.
---

# Clawtivity Skill

This skill sends activity payloads to local Clawtivity API:

- `POST http://localhost:18730/api/activity`
- `Content-Type: application/json`

It is designed to complement:
- the CLAW-16/18 plugin (`plugins/clawtivity-activity/`) for reliable outbound assistant telemetry

## Usage

Pipe turn JSON payload to:

```bash
python3 clawtivity/scripts/log_activity.py
```

The script reads JSON from stdin, normalizes fields, retries failed posts, and queues failures.

## Reliability Behavior

- Retry up to 3 attempts with exponential backoff: `1s`, `2s`, `4s`
- If all attempts fail, write markdown queue entry to:
  - `~/.clawtivity/queue/YYYY-MM-DD.md`
- On next successful POST, queued entries are replayed automatically

## Payload Mapping

Primary outgoing payload fields:

- `session_key`
- `model`
- `tokens_in`
- `tokens_out`
- `cost_estimate`
- `duration_ms`
- `project_tag` (auto-detected from workspace path if missing)
- `channel`
- `user_id`
- `status`
- `tools_used` (JSON array string)
- plus supported Clawtivity fields (`external_ref`, `category`, `thinking`, `reasoning`, `created_at`)

## Optional Environment Variables

- `CLAWTIVITY_API_URL` (default: `http://localhost:18730/api/activity`)
- `OPENCLAW_CHANNEL` (default: `webchat`)
- `OPENCLAW_USER_ID` (default: `unknown-user`)

## Manual Commands

```bash
# Send one payload from stdin
echo '{"session_key":"abc","model":"gpt-5"}' | python3 clawtivity/scripts/log_activity.py

# Replay queued entries only
python3 clawtivity/scripts/log_activity.py --flush-only
```
