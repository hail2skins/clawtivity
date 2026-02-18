---
name: clawtivity
description: "Send after-turn activity telemetry to local Clawtivity API"
homepage: https://github.com/hail2skins/clawtivity
metadata:
  {
    "openclaw":
      {
        "emoji": "📊",
        "events": ["after_agent_turn"],
        "requires": { "bins": ["python3"] },
        "install": [{ "id": "local", "kind": "manual", "label": "Local hook install" }],
      },
  }
---

# Clawtivity Hook

Runs after each agent turn and forwards selected telemetry fields to the local Clawtivity skill script:

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

## Destination

- `POST http://localhost:18730/api/activity` (handled by the skill script)
