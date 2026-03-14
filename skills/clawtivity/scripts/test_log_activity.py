import json
import os
import shutil
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import log_activity


class LogActivityTests(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.mkdtemp(prefix="clawtivity-log-activity-test-")
        self.queue_dir = Path(self._tmp) / "queue"
        log_activity.reset_metrics_counters()

    def tearDown(self):
        shutil.rmtree(self._tmp, ignore_errors=True)

    def test_shared_prompt_spec_cases_stay_aligned(self):
        spec_path = Path(__file__).resolve().parents[3] / "spec" / "project_tag_prompt_cases.json"
        cases = json.loads(spec_path.read_text(encoding="utf-8"))

        for case in cases:
            with self.subTest(case=case["name"]):
                self.assertEqual(
                    log_activity.project_from_prompt(case["prompt_text"]),
                    case["expected_override"],
                )
                self.assertEqual(
                    log_activity.project_from_path_mention(case["prompt_text"]),
                    case["expected_path_mention"],
                )

    def test_resolve_queue_root_prefers_environment(self):
        env_name = "CLAWTIVITY_QUEUE_ROOT"
        original = os.environ.get(env_name)
        os.environ[env_name] = str(self.queue_dir / "env")
        try:
            result = log_activity.resolve_queue_root()
            self.assertEqual(str(result), str(self.queue_dir / "env"))
        finally:
            if original is None:
                os.environ.pop(env_name, None)
            else:
                os.environ[env_name] = original

    def test_resolve_queue_root_falls_back_to_default(self):
        env_name = "CLAWTIVITY_QUEUE_ROOT"
        original = os.environ.get(env_name)
        os.environ.pop(env_name, None)
        try:
            result = log_activity.resolve_queue_root()
            self.assertEqual(str(result), str(log_activity.DEFAULT_QUEUE_ROOT))
        finally:
            if original is not None:
                os.environ[env_name] = original

    def test_resolve_backoff_seconds_reads_environment(self):
        env_name = "CLAWTIVITY_BACKOFF_SECONDS"
        original = os.environ.get(env_name)
        os.environ[env_name] = "2,5"
        try:
            result = log_activity.resolve_backoff_seconds()
            self.assertEqual(result, (2, 5))
        finally:
            if original is None:
                os.environ.pop(env_name, None)
            else:
                os.environ[env_name] = original

    def test_resolve_backoff_seconds_returns_default_when_missing(self):
        env_name = "CLAWTIVITY_BACKOFF_SECONDS"
        original = os.environ.get(env_name)
        os.environ.pop(env_name, None)
        try:
            result = log_activity.resolve_backoff_seconds()
            self.assertEqual(result, log_activity.DEFAULT_BACKOFF_SECONDS)
        finally:
            if original is not None:
                os.environ[env_name] = original

    def test_normalize_payload_resolves_project_from_workspace_context(self):
        workspace_dir = Path(self._tmp) / "projects" / "clawtivity"
        workspace_dir.mkdir(parents=True, exist_ok=True)

        payload = log_activity.normalize_payload({
            "prompt_text": "Please proceed.",
            "workspace": str(workspace_dir / "internal" / "server"),
        })

        self.assertEqual(payload["project_tag"], "clawtivity")
        self.assertEqual(payload["project_reason"], "workspace_path")

    def test_post_with_retry_success_after_failures(self):
        payload = {"session_key": "s-1"}
        calls = []

        def flaky_post(url, body, timeout=5):
            calls.append((url, body))
            if len(calls) < 3:
                raise RuntimeError("temporary failure")
            return {"ok": True}

        with mock.patch.object(log_activity, "_http_post_json", side_effect=flaky_post):
            with mock.patch("time.sleep") as sleep:
                ok = log_activity.post_with_retry(payload, "http://localhost:18730/api/activity", queue_root=self.queue_dir)

        self.assertTrue(ok)
        self.assertEqual(len(calls), 3)
        self.assertEqual(sleep.call_count, 2)
        self.assertEqual([args[0][0] for args in sleep.call_args_list], [1, 2])

    def test_post_with_retry_queues_after_3_failures(self):
        payload = {"session_key": "s-2", "model": "gpt-5"}

        with mock.patch.object(log_activity, "_http_post_json", side_effect=RuntimeError("down")):
            with mock.patch("time.sleep"):
                ok = log_activity.post_with_retry(payload, "http://localhost:18730/api/activity", queue_root=self.queue_dir)

        self.assertFalse(ok)
        files = sorted(self.queue_dir.glob("*.md"))
        self.assertEqual(len(files), 1)
        body = files[0].read_text(encoding="utf-8")
        self.assertIn("```json", body)
        payloads = log_activity._extract_payloads(body)
        self.assertEqual(payloads[0]["session_key"], "s-2")


    def test_post_with_retry_logs_structured_failure_and_queue_depth(self):
        payload = {"session_key": "s-3", "model": "gpt-5"}

        with self.assertLogs("clawtivity.fallback", level="INFO") as captured:
            with mock.patch.object(log_activity, "_http_post_json", side_effect=RuntimeError("down")):
                with mock.patch("time.sleep"):
                    ok = log_activity.post_with_retry(payload, "http://localhost:18730/api/activity", queue_root=self.queue_dir)

        self.assertFalse(ok)
        entries = [json.loads(record.split(":", 2)[-1]) for record in captured.output]
        self.assertEqual(entries[0]["event"], "plugin_post_failed")
        self.assertEqual(entries[-1]["event"], "queue_fallback_enqueued")
        self.assertEqual(entries[-1]["metrics"]["queue_depth"], 1)
        fallback_metrics = entries[-1]["metrics"]
        self.assertEqual(fallback_metrics["queue_fallback_enqueued"], 1)
        self.assertEqual(fallback_metrics["plugin_post_failed"], 1)
        metrics = entries[0]["metrics"]
        self.assertEqual(metrics["activities_created"], 0)
        self.assertEqual(metrics["queue_flush_attempted"], 0)
        self.assertEqual(metrics["queue_flush_succeeded"], 0)
        self.assertEqual(metrics["queue_flush_failed"], 0)
        self.assertEqual(metrics["plugin_post_failed"], 1)
        self.assertEqual(metrics["queue_fallback_enqueued"], 0)
        self.assertEqual(metrics["replay_succeeded"], 0)
        self.assertEqual(metrics["replay_failed"], 0)

    def test_flush_queue_logs_replay_success_and_failure(self):
        log_activity.reset_metrics_counters()
        first = {"session_key": "queued-1", "model": "gpt-5"}
        second = {"session_key": "queued-2", "model": "gpt-5"}
        log_activity.enqueue_payload(self.queue_dir, first)
        log_activity.enqueue_payload(self.queue_dir, second)

        sent = []

        def flaky_post(url, body, timeout=5):
            payload = json.loads(body.decode("utf-8"))
            sent.append(payload["session_key"])
            if payload["session_key"] == "queued-2":
                raise RuntimeError("still down")
            return {"ok": True}

        with self.assertLogs("clawtivity.fallback", level="INFO") as captured:
            with mock.patch.object(log_activity, "_http_post_json", side_effect=flaky_post):
                with mock.patch("time.sleep"):
                    log_activity.flush_queue("http://localhost:18730/api/activity", queue_root=self.queue_dir)

        entries = [json.loads(record.split(":", 2)[-1]) for record in captured.output]
        self.assertTrue(any(entry["event"] == "replay_succeeded" for entry in entries))
        self.assertTrue(any(entry["event"] == "replay_failed" for entry in entries))
        attempt_entry = next(entry for entry in entries if entry["event"] == "queue_flush_attempted")
        self.assertEqual(attempt_entry["metrics"]["queue_flush_attempted"], 1)
        succeeded_entry = next(entry for entry in entries if entry["event"] == "replay_succeeded")
        self.assertEqual(succeeded_entry["metrics"]["queue_flush_succeeded"], 1)
        self.assertEqual(succeeded_entry["metrics"]["activities_created"], 1)
        self.assertEqual(succeeded_entry["metrics"]["replay_succeeded"], 1)
        failed_entry = next(entry for entry in entries if entry["event"] == "replay_failed")
        self.assertEqual(failed_entry["metrics"]["queue_flush_failed"], 1)
        self.assertEqual(failed_entry["metrics"]["replay_failed"], 1)
        remaining_files = sorted(self.queue_dir.glob("*.md"))
        self.assertEqual(len(remaining_files), 1)
        remaining_payloads = log_activity._extract_payloads(remaining_files[0].read_text(encoding="utf-8"))
        self.assertEqual([payload["session_key"] for payload in remaining_payloads], ["queued-2"])

    def test_flush_queue_on_success(self):
        queued_payload = {"session_key": "queued-1", "model": "gpt-5"}
        log_activity.enqueue_payload(self.queue_dir, queued_payload)

        sent = []

        def record_post(url, body, timeout=5):
            sent.append(json.loads(body.decode("utf-8")))
            return {"ok": True}

        with mock.patch.object(log_activity, "_http_post_json", side_effect=record_post):
            with mock.patch("time.sleep"):
                ok = log_activity.post_with_retry(
                    {"session_key": "live-1"},
                    "http://localhost:18730/api/activity",
                    queue_root=self.queue_dir,
                )

        self.assertTrue(ok)
        self.assertEqual(len(sent), 2)
        self.assertEqual(sent[0]["session_key"], "live-1")
        self.assertEqual(sent[1]["session_key"], "queued-1")
        self.assertEqual(list(self.queue_dir.glob("*.md")), [])


if __name__ == "__main__":
    unittest.main()
