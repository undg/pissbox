import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import launcher_harness as harness


class LauncherHarnessTests(unittest.TestCase):
    def setUp(self):
        fixtures = harness.SOURCE_ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = tempfile.TemporaryDirectory(prefix="launcher-harness-", dir=fixtures)
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)

    def test_checkout_is_reused_without_private_or_ambient_project_state(self):
        checkout = harness.checkout_root()
        self.assertEqual(harness.checkout_root(), checkout)
        self.assertNotEqual(checkout, harness.SOURCE_ROOT)
        for name in (".pissbox.toml", "pi-agent/auth.json", "pi-agent/sessions", "pi-agent/settings.json"):
            self.assertFalse((checkout / name).exists(), name)
        self.assertTrue(os.access(checkout / ".cache/bin" / harness.binary_name(), os.X_OK))
        self.assertEqual((checkout / "bin/pissbox").read_bytes(),
                         (harness.SOURCE_ROOT / "bin/pissbox").read_bytes())
        self.assertTrue((checkout / "sbx-kit/files/home/.pissbox/scripts/project-setup.sh").is_file())

    def test_go_candidate_and_production_wrapper_are_copied_into_each_checkout(self):
        candidate = self.root / "fresh-candidate"
        binary = candidate / ".cache/bin" / harness.binary_name()
        binary.parent.mkdir(parents=True)
        binary.write_text("#!/bin/sh\nexit 0\n")
        binary.chmod(0o755)
        with patch.object(harness, "checkout_root", return_value=candidate):
            for name in ("first checkout", "second checkout"):
                checkout = self.root / name
                launcher = harness.materialize_launcher(checkout)
                copied = checkout / ".cache/bin" / binary.name
                self.assertEqual(copied.read_bytes(), binary.read_bytes())
                self.assertTrue(os.access(copied, os.X_OK))
                self.assertFalse(launcher.is_symlink())
                self.assertEqual(launcher.read_bytes(), (harness.SOURCE_ROOT / "bin/pissbox").read_bytes())
                link = checkout / "installed launcher"
                link.symlink_to("bin/pissbox")
                self.assertEqual(link.resolve(), launcher)
        self.assertTrue(binary.exists())


if __name__ == "__main__":
    unittest.main()
