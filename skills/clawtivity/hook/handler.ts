import { spawn } from "node:child_process";
import path from "node:path";

type AnyRecord = Record<string, unknown>;

function get(obj: unknown, ...paths: string[]): unknown {
  for (const p of paths) {
    const parts = p.split(".");
    let cur: unknown = obj;
    let ok = true;
    for (const part of parts) {
      if (!cur || typeof cur !== "object" || !(part in (cur as AnyRecord))) {
        ok = false;
        break;
      }
      cur = (cur as AnyRecord)[part];
    }
    if (ok) {
      return cur;
    }
  }
  return undefined;
}

function asString(v: unknown, fallback = ""): string {
  if (typeof v === "string") {
    const s = v.trim();
    return s === "" ? fallback : s;
  }
  if (typeof v === "number" || typeof v === "boolean") {
    return String(v);
  }
  return fallback;
}

function asInt(v: unknown, fallback = 0): number {
  if (typeof v === "number" && Number.isFinite(v)) {
    return Math.round(v);
  }
  if (typeof v === "string") {
    const n = Number(v);
    if (Number.isFinite(n)) {
      return Math.round(n);
    }
  }
  return fallback;
}

function pickTools(v: unknown): string[] {
  if (Array.isArray(v)) {
    return v.map((item) => asString(item)).filter(Boolean);
  }
  if (typeof v === "string" && v.trim() !== "") {
    try {
      const parsed = JSON.parse(v);
      if (Array.isArray(parsed)) {
        return parsed.map((item) => asString(item)).filter(Boolean);
      }
      return [v.trim()];
    } catch {
      return [v.trim()];
    }
  }
  return [];
}

function detectProjectTag(event: unknown): string {
  const workspace = asString(
    get(event, "workspace", "workspaceDir", "context.workspaceDir", "context.cwd", "cwd"),
    process.cwd(),
  );

  const tag = path.basename(workspace);
  if (tag) {
    return tag;
  }
  return path.basename(process.cwd()) || "unknown";
}

function toPayload(event: unknown): AnyRecord {
  const action = asString(get(event, "action"), "sent");
  const eventType = asString(get(event, "type"), "message");

  const tools = pickTools(
    get(
      event,
      "tools_used",
      "toolsUsed",
      "tools",
      "turn.tools",
      "result.tools",
      "metrics.tools",
      "context.tools",
      "context.tools_used",
    ),
  );

  const sentSuccess = get(event, "context.success");
  const status =
    action === "received"
      ? "pending"
      : typeof sentSuccess === "boolean"
        ? sentSuccess
          ? "success"
          : "failed"
        : asString(get(event, "status", "result.status"), "success");

  return {
    session_key: asString(get(event, "session_key", "sessionKey", "context.sessionKey", "sessionKey"), "unknown-session"),
    model: asString(get(event, "model", "result.model", "context.model", "context.metadata.model"), "unknown-model"),
    tokens_in: asInt(
      get(
        event,
        "tokens_in",
        "tokensIn",
        "usage.input_tokens",
        "usage.prompt_tokens",
        "usage.tokens_in",
        "metrics.tokens_in",
      ),
      0,
    ),
    tokens_out: asInt(
      get(
        event,
        "tokens_out",
        "tokensOut",
        "usage.output_tokens",
        "usage.completion_tokens",
        "usage.tokens_out",
        "metrics.tokens_out",
      ),
      0,
    ),
    duration_ms: asInt(get(event, "duration_ms", "durationMs", "metrics.duration_ms", "context.duration_ms"), 0),
    channel: asString(get(event, "channel", "context.channel", "context.commandSource", "context.channelId"), "webchat"),
    user_id: asString(
      get(event, "user_id", "userId", "context.senderId", "senderId", "context.from", "context.to"),
      "unknown-user",
    ),
    tools_used: tools,
    project_tag: asString(get(event, "project_tag", "projectTag"), detectProjectTag(event)),
    status,
    category: eventType === "message" ? "general" : "other",
  };
}

function sendToSkill(payload: AnyRecord): Promise<void> {
  return new Promise((resolve, reject) => {
    const command = 'echo "$CLAWTIVITY_HOOK_JSON" | python3 ~/.openclaw/skills/clawtivity/scripts/log_activity.py';
    const child = spawn("bash", ["-lc", command], {
      env: {
        ...process.env,
        CLAWTIVITY_HOOK_JSON: JSON.stringify(payload),
      },
      stdio: ["ignore", "pipe", "pipe"],
    });

    let stderr = "";
    child.stderr.on("data", (chunk) => {
      stderr += String(chunk);
    });

    child.on("error", (err) => reject(err));
    child.on("close", (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`clawtivity hook exited ${code}: ${stderr.trim()}`));
    });
  });
}

export default async function clawtivityMessageHook(event: unknown): Promise<void> {
  const eventType = asString(get(event, "type"));
  const action = asString(get(event, "action"));
  if (eventType !== "message") {
    return;
  }
  if (action !== "received" && action !== "sent") {
    return;
  }

  const payload = toPayload(event);
  await sendToSkill(payload);
}
