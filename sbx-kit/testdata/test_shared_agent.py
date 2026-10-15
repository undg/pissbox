"""Exercise the Dockerfile's real symlink command without Docker or host state."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


DOCKERFILE = Path(__file__).resolve().parents[1] / "Dockerfile"


class SharedAgentTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.home = self.root / "home"
        text = DOCKERFILE.read_text()
        block = text.split("ARG SHARED_PI_AGENT_DIR\n", 1)[1].split("\n\n", 1)[0]
        self.command = block.removeprefix("RUN ").replace("\\\n", "")
        self.command = self.command.replace("/home/agent", str(self.home))

    def run_setup(self, target):
        return subprocess.run(
            ["sh", "-c", self.command],
            env={**os.environ, "SHARED_PI_AGENT_DIR": target},
            capture_output=True,
            text=True,
        )

    def test_dangling_link_then_mounted_directory(self):
        target = self.root / "repository with spaces" / "pi-agent"
        result = self.run_setup(str(target))
        self.assertEqual(result.returncode, 0, result.stderr)
        link = self.home / ".pi" / "agent"
        self.assertTrue(link.is_symlink())
        self.assertEqual(os.readlink(link), str(target))
        self.assertFalse(link.exists())
        target.mkdir(parents=True)
        (link / "settings.json").write_text("{}")
        self.assertEqual((target / "settings.json").read_text(), "{}")

    def test_missing_or_relative_target_fails(self):
        for target in ("", "relative/pi-agent"):
            with self.subTest(target=target):
                self.assertNotEqual(self.run_setup(target).returncode, 0)
        self.assertFalse((self.home / ".pi").exists())

    def test_existing_directory_is_not_silently_nested_or_replaced(self):
        existing = self.home / ".pi" / "agent"
        existing.mkdir(parents=True)
        marker = existing / "auth.json"
        marker.write_text("preserve me")
        self.assertNotEqual(self.run_setup(str(self.root / "shared")).returncode, 0)
        self.assertEqual(marker.read_text(), "preserve me")
        self.assertEqual(list(existing.iterdir()), [marker])


if __name__ == "__main__":
    unittest.main()
