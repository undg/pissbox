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

    def test_explicit_bash_selection_materializes_the_legacy_script(self):
        with patch.dict(os.environ, {"PISSBOX_TEST_LAUNCHER": "bash"}, clear=True):
            self.assertEqual(harness.implementation(), "bash")
            checkout = harness.checkout_root()
            self.assertNotEqual(checkout, harness.SOURCE_ROOT)
            self.assertFalse((checkout / ".pissbox.toml").exists())
            self.assertEqual((checkout / "bin/pissbox").read_bytes(),
                             (harness.SOURCE_ROOT / "bin/pissbox-legacy").read_bytes())
            launcher = harness.materialize_launcher(self.root)
        self.assertFalse(launcher.is_symlink())
        self.assertEqual(launcher.read_bytes(), (harness.SOURCE_ROOT / "bin/pissbox-legacy").read_bytes())
        self.assertTrue(os.access(launcher, os.X_OK))

    def test_go_is_default(self):
        with patch.dict(os.environ, {}, clear=True):
            self.assertEqual(harness.implementation(), "go")

    def test_go_candidate_and_production_wrapper_are_copied_into_each_checkout(self):
        candidate = self.root / "fresh-candidate"
        binary = candidate / ".cache/bin" / harness.binary_name()
        binary.parent.mkdir(parents=True)
        binary.write_text("#!/bin/sh\nexit 0\n")
        binary.chmod(0o755)
        with patch.dict(os.environ, {"PISSBOX_TEST_LAUNCHER": "go"}), patch.object(
            harness, "candidate_checkout", return_value=candidate,
        ):
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

    def test_unknown_selection_is_rejected_instead_of_falling_back(self):
        with patch.dict(os.environ, {"PISSBOX_TEST_LAUNCHER": "typo"}):
            with self.assertRaisesRegex(ValueError, "must be bash or go"):
                harness.checkout_root()


if __name__ == "__main__":
    unittest.main()
