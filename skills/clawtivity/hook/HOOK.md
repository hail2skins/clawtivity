---
name: clawtivity
description: "Send message event telemetry to local Clawtivity API"
homepage: https://github.com/hail2skins/clawtivity
metadata:
  {
    "openclaw":
      {
        "emoji": "📊",
        "events": ["message:received", "message:sent"],
        "requires": { "bins": ["python3"] },
        "install": [{ "id": "local", "kind": "manual", "label": "Local hook install" }],
      },
  }
---

# Clawtivity Hook (Message Events)

Runs on message lifecycle events and forwards telemetry fields to the local Clawtivity skill script:

`echo "$JSON" | python3 ~/.openclaw/skills/clawtivity/scripts/log_activity.py`

## Extracted fields

- `session_key`
- `model`
- `tokens_in`
- `tokens_out`
- `duration_ms`
- `channel`
- `user_id`
- `tools_used`
- `project_tag`
- `status`

Status behavior:
- `message:received` -> `pending`
- `message:sent` -> `success` when `context.success=true`, else `failed`

## Destination

- `POST http://localhost:18730/api/activity` (handled by the skill script)
