#!/usr/bin/env python3
"""OpenClaw -> Clawtivity activity logger.

Reads JSON payload from stdin, normalizes required fields, posts to local
Clawtivity API with retry/backoff, and falls back to markdown queue files.
"""

import argparse
import datetime as dt
import json
import os
import re
import sys
import time
from pathlib import Path
from typing import Dict, List, Tuple
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

API_URL = "http://localhost:18730/api/activity"
BACKOFF_SECONDS = (1, 2, 4)
QUEUE_ROOT = Path.home() / ".clawtivity" / "queue"
PROJECT_OVERRIDE_PATTERN = re.compile(r"\bproject\b\s*:?\s*([a-zA-Z0-9][a-zA-Z0-9._-]*)", re.IGNORECASE)
PROJECT_PATH_MENTION_PATTERN = re.compile(r"/projects?/([a-zA-Z0-9][a-zA-Z0-9._-]*)", re.IGNORECASE)
PROJECT_OVERRIDE_STOPWORDS = {"as", "is", "was", "the", "a", "an", "to", "for"}


def _http_post_json(url: str, body: bytes, timeout: int = 5):
    req = Request(url, data=body, method="POST")
    req.add_header("Content-Type", "application/json")
    with urlopen(req, timeout=timeout) as response:
        raw = response.read().decode("utf-8")
        return json.loads(raw) if raw else {"ok": True}


def normalize_payload(raw: Dict) -> Dict:
    now = dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z")

    cwd = raw.get("workspace") or raw.get("cwd") or os.getcwd()
    project = resolve_project_context(
        prompt_text=raw.get("prompt_text", ""),
        workspace_dir=cwd,
        configured_project_tag=raw.get("project_tag", ""),
    )

    tools_used = raw.get("tools_used") or raw.get("tools") or []
    if isinstance(tools_used, str):
        try:
            tools_used = json.loads(tools_used)
        except json.JSONDecodeError:
            tools_used = [tools_used]

    return {
        "session_key": raw.get("session_key") or raw.get("session_id") or "unknown-session",
        "model": raw.get("model") or "unknown-model",
        "tokens_in": int(raw.get("tokens_in") or 0),
        "tokens_out": int(raw.get("tokens_out") or 0),
        "cost_estimate": float(raw.get("cost_estimate") or 0),
        "duration_ms": int(raw.get("duration_ms") or 0),
        "project_tag": project["project_tag"],
        "project_reason": project["project_reason"],
        "external_ref": raw.get("external_ref") or "",
        "category": raw.get("category") or "general",
        "thinking": raw.get("thinking") or "medium",
        "reasoning": bool(raw.get("reasoning", False)),
        "channel": raw.get("channel") or os.environ.get("OPENCLAW_CHANNEL", "webchat"),
        "status": raw.get("status") or "success",
        "user_id": raw.get("user_id") or os.environ.get("OPENCLAW_USER_ID", "unknown-user"),
        "created_at": raw.get("created_at") or now,
        "tools_used": json.dumps(tools_used),
    }


def normalize_project_tag(value: str) -> str:
    raw = str(value or "").strip().lower()
    if not raw:
        return ""
    raw = re.sub(r"\s+", "-", raw)
    raw = re.sub(r"[^a-z0-9._-]", "", raw)
    raw = re.sub(r"-+", "-", raw)
    return raw.strip("-")


def project_from_prompt(prompt_text: str) -> str:
    text = str(prompt_text or "").strip()
    if not text:
        return ""
    match = PROJECT_OVERRIDE_PATTERN.search(text)
    if not match:
        return ""
    candidate = normalize_project_tag(match.group(1).rstrip(".,;:!?)]}\"'"))
    if not candidate or candidate in PROJECT_OVERRIDE_STOPWORDS:
        return ""
    return candidate


def project_from_path_mention(prompt_text: str) -> str:
    text = str(prompt_text or "").strip()
    if not text:
        return ""
    match = PROJECT_PATH_MENTION_PATTERN.search(text)
    if not match:
        return ""
    candidate = normalize_project_tag(match.group(1).rstrip(".,;:!?)]}\"'"))
    return candidate


def project_from_workspace_dir(workspace_dir: str) -> str:
    directory = str(workspace_dir or "").strip()
    if not directory:
        return ""
    normalized = directory.replace("\\", "/")
    match = re.search(r"/projects?/([^/]+)", normalized, flags=re.IGNORECASE)
    if not match:
        return ""
    return normalize_project_tag(match.group(1))


def discover_project_roots(workspace_dir: str) -> List[Path]:
    roots: List[Path] = []
    seen = set()
    candidates = [value for value in [workspace_dir, os.getcwd()] if value]
    for candidate in candidates:
        normalized = str(candidate).replace("\\", "/")
        match = re.search(r"^(.*?/projects?)(?:/.*)?$", normalized, flags=re.IGNORECASE)
        if match:
            root = Path(match.group(1))
            if root not in seen:
                seen.add(root)
                roots.append(root)
        for suffix in ("projects", "project"):
            root = Path(candidate) / suffix
            if root not in seen:
                seen.add(root)
                roots.append(root)
    return roots


def project_exists_under_known_roots(project_tag: str, workspace_dir: str) -> bool:
    roots = discover_project_roots(workspace_dir)
    if not roots:
        return True
    for root in roots:
        target = root / project_tag
        if target.is_dir():
            return True
    return False


def resolve_project_context(prompt_text: str = "", workspace_dir: str = "", configured_project_tag: str = "") -> Dict[str, str]:
    from_prompt = project_from_prompt(prompt_text)
    if from_prompt and project_exists_under_known_roots(from_prompt, workspace_dir):
        return {
            "project_tag": from_prompt,
            "project_reason": "prompt_override",
        }

    from_path_mention = project_from_path_mention(prompt_text)
    if from_path_mention:
        return {
            "project_tag": from_path_mention,
            "project_reason": "prompt_path_mention",
        }

    from_workspace = project_from_workspace_dir(workspace_dir)
    if from_workspace:
        return {
            "project_tag": from_workspace,
            "project_reason": "workspace_path",
        }

    from_config = normalize_project_tag(configured_project_tag)
    if from_config:
        return {
            "project_tag": from_config,
            "project_reason": "plugin_config",
        }

    return {
        "project_tag": "workspace",
        "project_reason": "fallback:workspace",
    }


def _queue_file(queue_root: Path, when: dt.datetime = None) -> Path:
    when = when or dt.datetime.now()
    queue_root.mkdir(parents=True, exist_ok=True)
    return queue_root / f"{when.strftime('%Y-%m-%d')}.md"


def enqueue_payload(queue_root: Path, payload: Dict):
    path = _queue_file(queue_root)
    timestamp = dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z")
    if not path.exists():
        path.write_text(f"# Clawtivity Fallback Queue ({path.stem})\n\n", encoding="utf-8")

    block = (
        f"## queued_at: {timestamp}\n"
        "```json\n"
        f"{json.dumps(payload, ensure_ascii=True, separators=(',', ':'))}\n"
        "```\n\n"
    )
    with path.open("a", encoding="utf-8") as f:
        f.write(block)


def _extract_payloads(markdown: str) -> List[Dict]:
    matches = re.findall(r"```json\n(.*?)\n```", markdown, flags=re.DOTALL)
    out: List[Dict] = []
    for entry in matches:
        try:
            out.append(json.loads(entry))
        except json.JSONDecodeError:
            continue
    return out


def _write_payloads(path: Path, payloads: List[Dict]):
    if not payloads:
        path.unlink(missing_ok=True)
        return

    lines = [f"# Clawtivity Fallback Queue ({path.stem})\n\n"]
    for payload in payloads:
        lines.append(
            "## queued_at: replay_pending\n"
            "```json\n"
            f"{json.dumps(payload, ensure_ascii=True, separators=(',', ':'))}\n"
            "```\n\n"
        )
    path.write_text("".join(lines), encoding="utf-8")


def flush_queue(url: str, queue_root: Path = QUEUE_ROOT):
    if not queue_root.exists():
        return

    for path in sorted(queue_root.glob("*.md")):
        body = path.read_text(encoding="utf-8")
        payloads = _extract_payloads(body)
        remaining = []

        for payload in payloads:
            ok = post_with_retry(payload, url, queue_root=queue_root, flush_on_success=False)
            if not ok:
                remaining.append(payload)

        _write_payloads(path, remaining)


def post_with_retry(payload: Dict, url: str, queue_root: Path = QUEUE_ROOT, flush_on_success: bool = True) -> bool:
    body = json.dumps(payload, ensure_ascii=True).encode("utf-8")

    attempts = len(BACKOFF_SECONDS)
    for idx in range(attempts):
        try:
            _http_post_json(url, body)
            if flush_on_success:
                flush_queue(url, queue_root=queue_root)
            return True
        except (HTTPError, URLError, RuntimeError, ValueError):
            if idx < attempts - 1:
                time.sleep(BACKOFF_SECONDS[idx])

    enqueue_payload(queue_root, payload)
    return False


def _read_stdin_payload() -> Dict:
    raw = sys.stdin.read().strip()
    if not raw:
        return {}
    try:
        value = json.loads(raw)
        if isinstance(value, dict):
            return value
        return {}
    except json.JSONDecodeError:
        return {}


def main() -> int:
    parser = argparse.ArgumentParser(description="Send OpenClaw activity to Clawtivity API.")
    parser.add_argument("--api-url", default=os.environ.get("CLAWTIVITY_API_URL", API_URL))
    parser.add_argument("--flush-only", action="store_true", help="Only flush queued entries.")
    parser.add_argument("--queue-root", default=str(QUEUE_ROOT))
    args = parser.parse_args()

    queue_root = Path(args.queue_root).expanduser()

    if args.flush_only:
        flush_queue(args.api_url, queue_root=queue_root)
        return 0

    raw = _read_stdin_payload()
    payload = normalize_payload(raw)
    ok = post_with_retry(payload, args.api_url, queue_root=queue_root, flush_on_success=True)

    if ok:
        print(json.dumps({"status": "sent"}))
        return 0

    print(json.dumps({"status": "queued"}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
