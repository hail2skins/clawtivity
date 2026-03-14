#!/usr/bin/env python3
"""OpenClaw -> Clawtivity activity logger.

Reads JSON payload from stdin, normalizes required fields, posts to local
Clawtivity API with retry/backoff, and falls back to markdown queue files.
"""

import argparse
import datetime as dt
import json
import logging
import math
import os
import re
import sys
import time
from pathlib import Path
from typing import Dict, List, Optional, Tuple
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

API_URL = "http://localhost:18730/api/activity"
DEFAULT_BACKOFF_SECONDS = (1, 2, 4)
DEFAULT_QUEUE_ROOT = Path.home() / ".clawtivity" / "queue"
PROJECT_OVERRIDE_PATTERN = re.compile(r"\bproject\b\s*:?\s*([a-zA-Z0-9][a-zA-Z0-9._-]*)", re.IGNORECASE)
PROJECT_PATH_MENTION_PATTERN = re.compile(r"/projects?/([a-zA-Z0-9][a-zA-Z0-9._-]*)", re.IGNORECASE)
PROJECT_OVERRIDE_STOPWORDS = {"as", "is", "was", "the", "a", "an", "to", "for"}
LOG_LEVEL_ENV = "CLAWTIVITY_LOG_LEVEL"
DEFAULT_LOG_LEVEL = "info"
LOG_LEVEL_PRIORITY = {
    "debug": logging.DEBUG,
    "info": logging.INFO,
    "warn": logging.WARNING,
    "error": logging.ERROR,
}
LOGGER = logging.getLogger("clawtivity.fallback")
if not LOGGER.handlers:
    _handler = logging.StreamHandler()
    _handler.setFormatter(logging.Formatter("%(message)s"))
    LOGGER.addHandler(_handler)
LOGGER.setLevel(logging.DEBUG)
LOGGER.propagate = False

METRICS_COUNTERS = {
    'activities_created': 0,
    'queue_flush_attempted': 0,
    'queue_flush_succeeded': 0,
    'queue_flush_failed': 0,
    'plugin_post_failed': 0,
    'queue_fallback_enqueued': 0,
    'replay_succeeded': 0,
    'replay_failed': 0,
}


def reset_metrics_counters() -> None:
    for key in METRICS_COUNTERS:
        METRICS_COUNTERS[key] = 0


def _inc_metric(name: str) -> None:
    if name in METRICS_COUNTERS:
        METRICS_COUNTERS[name] += 1


def _metrics_payload(queue_depth: int) -> Dict[str, int]:
    metrics = {key: METRICS_COUNTERS[key] for key in METRICS_COUNTERS}
    metrics['queue_depth'] = int(queue_depth)
    return metrics



def resolve_log_level() -> str:
    value = str(os.environ.get(LOG_LEVEL_ENV, DEFAULT_LOG_LEVEL) or DEFAULT_LOG_LEVEL).strip().lower()
    return value if value in LOG_LEVEL_PRIORITY else DEFAULT_LOG_LEVEL


def should_log(level: str) -> bool:
    target = LOG_LEVEL_PRIORITY.get(str(level).strip().lower(), logging.INFO)
    current = LOG_LEVEL_PRIORITY.get(resolve_log_level(), logging.INFO)
    return target >= current


def log_event(level: str, event: str, details: Optional[Dict] = None, queue_depth: int = 0):
    normalized = str(level or DEFAULT_LOG_LEVEL).strip().lower()
    if not should_log(normalized):
        return

    payload = {
        "timestamp": dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z"),
        "level": normalized,
        "event": event,
        "metrics": _metrics_payload(queue_depth),
        "details": {
            **(details or {}),
            "queue_depth": int(queue_depth),
        },
    }
    LOGGER.log(LOG_LEVEL_PRIORITY.get(normalized, logging.INFO), json.dumps(payload, ensure_ascii=True, separators=(",", ":")))


def count_queued_entries(queue_root: Path) -> int:
    root = Path(queue_root)
    if not root.exists():
        return 0

    count = 0
    for path in sorted(root.glob("*.md")):
        count += len(_extract_payloads(path.read_text(encoding="utf-8")))
    return count


def _parse_seconds(value: str) -> Tuple[int, ...]:
    if not value:
        return ()
    entries = re.split(r"[\s,]+", value.strip())
    out = []
    for entry in entries:
        if not entry:
            continue
        try:
            number = float(entry)
        except ValueError:
            continue
        if not math.isfinite(number):
            continue
        out.append(int(number))
    return tuple(out)


def resolve_backoff_seconds() -> Tuple[int, ...]:
    env_value = os.environ.get("CLAWTIVITY_BACKOFF_SECONDS", "").strip()
    parsed = _parse_seconds(env_value)
    if parsed:
        return parsed
    return DEFAULT_BACKOFF_SECONDS


def resolve_queue_root(cli_value: Optional[str] = None) -> Path:
    if cli_value:
        return Path(cli_value).expanduser()
    env_value = os.environ.get("CLAWTIVITY_QUEUE_ROOT", "").strip()
    if env_value:
        return Path(env_value).expanduser()
    return DEFAULT_QUEUE_ROOT


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


def enqueue_payload(queue_root: Path, payload: Dict, *, emit_log: bool = True):
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

    queue_depth = count_queued_entries(queue_root)
    if emit_log:
        _inc_metric("queue_fallback_enqueued")
        log_event("info", "queue_fallback_enqueued", {
            "file": str(path),
            "queue_root": str(queue_root),
            "session_key": payload.get("session_key", ""),
        }, queue_depth=queue_depth)
    return queue_depth


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


def flush_queue(url: str, queue_root: Optional[Path] = None):
    queue_root = queue_root or resolve_queue_root()
    if not queue_root.exists():
        return

    _inc_metric("queue_flush_attempted")
    log_event("info", "queue_flush_attempted", {
        "queue_root": str(queue_root),
    }, queue_depth=count_queued_entries(queue_root))

    for path in sorted(queue_root.glob("*.md")):
        body = path.read_text(encoding="utf-8")
        payloads = _extract_payloads(body)
        remaining = []

        for payload in payloads:
            ok = post_with_retry(
                payload,
                url,
                queue_root=queue_root,
                flush_on_success=False,
                enqueue_on_failure=False,
            )
            if ok:
                _inc_metric("queue_flush_succeeded")
                _inc_metric("replay_succeeded")
                log_event("info", "replay_succeeded", {
                    "file": str(path),
                    "queue_root": str(queue_root),
                    "session_key": payload.get("session_key", ""),
                }, queue_depth=max(count_queued_entries(queue_root) - 1, 0))
            else:
                remaining.append(payload)
                _inc_metric("queue_flush_failed")
                _inc_metric("replay_failed")
                log_event("warn", "replay_failed", {
                    "file": str(path),
                    "queue_root": str(queue_root),
                    "session_key": payload.get("session_key", ""),
                }, queue_depth=count_queued_entries(queue_root))

        _write_payloads(path, remaining)


def post_with_retry(
    payload: Dict,
    url: str,
    queue_root: Optional[Path] = None,
    backoff_seconds: Optional[Tuple[int, ...]] = None,
    flush_on_success: bool = True,
    enqueue_on_failure: bool = True,
) -> bool:
    queue_root = queue_root or resolve_queue_root()
    backoff_seconds = backoff_seconds or resolve_backoff_seconds()
    body = json.dumps(payload, ensure_ascii=True).encode("utf-8")

    attempts = len(backoff_seconds)
    last_error = None
    for idx in range(attempts):
        try:
            _http_post_json(url, body)
            _inc_metric("activities_created")
            if flush_on_success:
                flush_queue(url, queue_root=queue_root)
            return True
        except (HTTPError, URLError, RuntimeError, ValueError) as err:
            last_error = err
            if idx < attempts - 1:
                time.sleep(backoff_seconds[idx])

    _inc_metric("plugin_post_failed")
    log_event("warn", "plugin_post_failed", {
        "api_url": url,
        "error": str(last_error) if last_error else "unknown error",
        "session_key": payload.get("session_key", ""),
    }, queue_depth=count_queued_entries(queue_root))
    if enqueue_on_failure:
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
    parser.add_argument("--queue-root", default=None)
    args = parser.parse_args()

    queue_root = resolve_queue_root(args.queue_root)

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
