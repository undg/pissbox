import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from launcher_harness import checkout_root


ROOT = checkout_root()


def flag(call, name):
    return call[call.index(name) + 1]


class SecretTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not shutil.which("tomljson") or not shutil.which("jq"):
            raise unittest.SkipTest("Secret setup tests require tomljson and jq")

    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = tempfile.TemporaryDirectory(prefix="secrets-", dir=fixtures)
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.project = self.root / "project"
        self.project.mkdir()
        self.pi = self.root / "pi"
        self.pi.mkdir()
        for profile in ("settings-co.json", "settings-go.json"):
            (self.pi / profile).write_text("{}")
        self.tools = self.root / "tools"
        self.tools.mkdir()
        (self.tools / "sbx").symlink_to(ROOT / "tests/fixtures/sbx-isolation")
        self.global_config = self.root / "config.toml"
        self.global_config.write_text("# isolated launcher test config\n")

    def launch(self, *args, answers="", **env):
        environment = {key: value for key, value in os.environ.items()
                       if not key.startswith(("PISSBOX_", "TEST_ISOLATION_"))}
        environment.update(PATH=str(self.tools) + os.pathsep + os.environ["PATH"],
                           PISSBOX_PI_DIR=str(self.pi), PISSBOX_STATE_DIR=str(self.root / "state"),
                           PISSBOX_CONFIG=str(self.global_config),
                           TEST_ISOLATION_ROOT=str(self.root))
        environment.update(env)
        log = self.root / "calls.jsonl"
        log.unlink(missing_ok=True)
        result = subprocess.run([str(ROOT / "bin/pissbox"), *args], cwd=self.project,
                                env=environment, input=answers, text=True, capture_output=True, timeout=10)
        calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
        return result, calls

    def test_scoped_registration_precedes_real_setup_without_isolated_directories(self):
        (self.project / ".pissbox.toml").write_text('setup = ["printf ready > setup-marker"]')
        command = "touch '" + str(self.project / "host-execution") + "'"
        result, calls = self.launch("--secret", "API", "*.example.com", command)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["secret", "create", "secret", "exec", "run"])
        placeholder = flag(calls[2], "--placeholder")
        self.assertTrue(placeholder.startswith("sbx-cs-"), placeholder)
        self.assertEqual(calls[2], ["secret", "set-custom", "--sandbox", "sbx-assigned-not-guessed",
                                    "--env", "API", "--host", "*.example.com",
                                    "--placeholder", placeholder, "--command", command])
        self.assertIn("API=" + placeholder, calls[1])
        self.assertEqual((self.project / "setup-marker").read_text(), "ready")
        self.assertTrue((self.root / "home/.pissbox/isolation-ready").is_file())
        self.assertFalse((self.project / "host-execution").exists())

        (self.project / "setup-marker").unlink()
        result, calls = self.launch("--secret", "API", "*.example.com", command, "-y")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["secret", "create", "exec", "run"])
        self.assertFalse((self.project / "setup-marker").exists())

    def test_global_config_registers_before_setup_and_cli_overrides_by_name(self):
        marker = self.project / "host-execution"
        original = (f'[secrets.ZED]\nhost = "z.example.com"\ncommand = "touch {marker}"\n'
                    '[secrets.API]\nhost = "global.example.com"\ncommand = "global_command"\n')
        self.global_config.write_text(original)
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["secret", "create", "secret", "secret", "exec", "run"])
        api, zed = flag(calls[2], "--placeholder"), flag(calls[3], "--placeholder")
        self.assertNotEqual(api, zed)
        self.assertEqual(calls[2], ["secret", "set-custom", "--sandbox", "sbx-assigned-not-guessed",
                                    "--env", "API", "--host", "global.example.com",
                                    "--placeholder", api, "--command", "global_command"])
        self.assertEqual(calls[3], ["secret", "set-custom", "--sandbox", "sbx-assigned-not-guessed",
                                    "--env", "ZED", "--host", "z.example.com",
                                    "--placeholder", zed, "--command", f"touch {marker}"])
        self.assertIn("API=" + api, calls[1])
        self.assertIn("ZED=" + zed, calls[1])
        self.assertFalse(marker.exists())
        self.assertEqual(self.global_config.read_text(), original)

        (self.root / "sandbox.json").unlink()
        shutil.rmtree(self.root / "home")
        result, calls = self.launch("--secret", "API", "cli.example.com", "cli_command")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["secret", "create", "secret", "secret", "exec", "run"])
        self.assertEqual(calls[2][calls[2].index("--env") + 1], "API")
        self.assertEqual(calls[2][calls[2].index("--host") + 1], "cli.example.com")
        self.assertEqual(calls[2][calls[2].index("--command") + 1], "cli_command")
        self.assertEqual(calls[3][calls[3].index("--env") + 1], "ZED")
        self.assertIn("API=" + flag(calls[2], "--placeholder"), calls[1])
        self.assertIn("ZED=" + flag(calls[3], "--placeholder"), calls[1])
        self.assertEqual(self.global_config.read_text(), original)

    def test_named_creation_and_later_attach_without_secret_flags(self):
        result, calls = self.launch("--secret", "API", "api.example.com", "/host/bin/get-token",
                                    "--name", "chosen")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[2][calls[2].index("--sandbox") + 1], "chosen")
        self.assertIn("API=" + flag(calls[2], "--placeholder"), calls[1])
        self.assertEqual(calls[-1], ["run", "--name", "chosen"])
        result, calls = self.launch("--name", "chosen", "-y")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["run", "exec", "run"])

    def test_failed_registration_blocks_setup_and_later_pi_shell_attaches(self):
        config = self.project / ".pissbox.toml"
        config.write_text('setup = ["touch must-not-run"]')
        result, calls = self.launch("--secret", "API", "api.example.com", "/host/bin/get-token",
                                    TEST_ISOLATION_SECRET_STATUS="19")
        self.assertEqual(result.returncode, 19, result.stderr)
        self.assertEqual([call[0] for call in calls], ["secret", "create", "secret"])
        self.assertFalse((self.project / "must-not-run").exists())
        self.assertFalse((self.root / "home/.pissbox/isolation-ready").exists())
        config.unlink()
        for choice in ("p", "s"):
            with self.subTest(choice=choice):
                result, calls = self.launch(answers=choice)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("setup is incomplete", result.stderr)
                self.assertEqual([call[0] for call in calls], ["run", "exec"])
                self.assertEqual(calls[-1][-1], "pissbox-readiness")
        result, calls = self.launch(answers="q")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["run"])


if __name__ == "__main__":
    unittest.main()
