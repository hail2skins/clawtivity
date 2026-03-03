import json
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

    def tearDown(self):
        shutil.rmtree(self._tmp, ignore_errors=True)

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
