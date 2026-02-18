import unittest
from pathlib import Path


class HookMetadataTests(unittest.TestCase):
    def test_hook_uses_outbound_only_message_event(self):
        hook_md = Path("skills/clawtivity/hook/HOOK.md").read_text(encoding="utf-8")
        self.assertIn('"events": ["message:sent"]', hook_md)
        self.assertNotIn("message:received", hook_md)
        self.assertNotIn("after_agent_turn", hook_md)


if __name__ == "__main__":
    unittest.main()
