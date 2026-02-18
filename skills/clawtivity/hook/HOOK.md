---
name: clawtivity
description: "Legacy outbound message telemetry bridge to local Clawtivity API"
homepage: https://github.com/hail2skins/clawtivity
metadata:
  {
    "openclaw":
      {
        "emoji": "📊",
        "events": ["message:sent"],
        "requires": { "bins": ["python3"] },
        "install": [{ "id": "local", "kind": "manual", "label": "Local hook install" }],
      },
  }
---

# Clawtivity Hook (Legacy Message Bridge)

This hook forwards outbound message telemetry to the local Clawtivity skill script:

`echo "$JSON" | python3 ~/.openclaw/skills/clawtivity/scripts/log_activity.py`

For reliable agent activity capture, use the shipped plugin at:

- `plugins/clawtivity-activity/`

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
- `message:sent` -> `success` when `context.success=true`, else `failed`

## Destination

- `POST http://localhost:18730/api/activity` (handled by the skill script)
