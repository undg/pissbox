import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class BridgeLauncherTests(unittest.TestCase):
    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        self.checkout = Path(tempfile.mkdtemp(prefix="bridge launcher-", dir=fixtures))
        (self.checkout / "bin").mkdir()
        self.launcher = self.checkout / "bin/pissbox"
        shutil.copy2(ROOT / "bin/pissbox", self.launcher)
        pi = self.checkout / "pi-agent"
        (pi / "extensions").mkdir(parents=True)
        for name in ("settings-co.json", "settings-go.json"):
            (pi / name).write_text("{}")
        shutil.copy2(ROOT / "pi-agent/extensions/pissbox-bridge.js", pi / "extensions")
        os_name = platform.system().lower()
        arch = {"aarch64": "arm64", "arm64": "arm64", "x86_64": "amd64"}[platform.machine()]
        self.binary = self.checkout / f".cache/bin/go-bridge-{os_name}-{arch}"
        self.binary.parent.mkdir(parents=True)
        shutil.copy2(ROOT / "tests/fixtures/go-bridge", self.binary)
        self.binary.chmod(0o755)
        self.log = self.checkout / "helper.jsonl"

    def launch(self, answers="", **overrides):
        env = {
            key: value for key, value in os.environ.items()
            if not key.startswith(("HERDR_", "PISSBOX_", "TEST_SBX_", "TEST_BRIDGE_"))
        }
        env.update(
            PATH=str(ROOT / "tests/fixtures") + os.pathsep + env["PATH"],
            HERDR_ENV="1", HERDR_SOCKET_PATH="host-only.sock", HERDR_PANE_ID="w1:p7",
            TEST_BRIDGE_LOG=str(self.log),
        )
        env.update(overrides)
        result = subprocess.run(
            [str(self.launcher)], cwd=self.checkout, env=env, input=answers,
            capture_output=True, text=True, timeout=5,
        )
        calls = [json.loads(line[4:]) for line in result.stdout.splitlines() if line.startswith("sbx:")]
        events = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return result, calls, events

    def test_helper_uses_host_pane_and_passes_only_mailbox_to_sandbox(self):
        result, calls, events = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([event["kind"] for event in events], ["start", "stop"])
        start = events[0]
        self.assertEqual(start["herdr_pane"], "w1:p7")
        self.assertEqual(start["herdr_socket"], "host-only.sock")
        self.assertIn("PISSBOX_BRIDGE_DIR=" + start["mailbox"], calls[0])
        self.assertNotIn("host-only.sock", calls[0])
        self.assertNotIn("w1:p7", calls[0])
        self.assertFalse((Path(start["mailbox"]) / "ready").exists())

    def test_reattach_gets_a_fresh_mailbox_and_helper_lifetime(self):
        result, calls, events = self.launch("p\n", TEST_SBX_MODE="conflict")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([event["kind"] for event in events], ["start", "stop", "start", "stop"])
        first, second = events[0]["mailbox"], events[2]["mailbox"]
        self.assertNotEqual(first, second)
        self.assertEqual(calls[1], ["run", "--name", "pi-existing", "--env", "PISSBOX_BRIDGE_DIR=" + second])

    def test_shell_choice_stops_helper_without_starting_another(self):
        result, calls, events = self.launch("s\n", TEST_SBX_MODE="conflict")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[1], ["exec", "-it", "pi-existing", "bash"])
        self.assertEqual([event["kind"] for event in events], ["start", "stop"])

    def test_bridge_failure_never_changes_sbx_exit_status(self):
        for status in ("0", "7"):
            with self.subTest(status=status):
                result, calls, events = self.launch(TEST_BRIDGE_START_FAIL="1", TEST_SBX_RUN_STATUS=status)
                self.assertEqual(result.returncode, int(status), result.stderr)
                self.assertIn("bridge failed to start", result.stderr)
                self.assertFalse(any(arg.startswith("PISSBOX_BRIDGE_DIR=") for arg in calls[0]))
                self.assertEqual(events, [])

    def test_missing_binary_warns_but_launches_pi(self):
        self.binary.rename(self.binary.with_suffix(".unavailable"))
        result, calls, events = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("run bin/build-bridge", result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertEqual(events, [])

    def test_outside_herd_does_not_start_helper(self):
        result, calls, events = self.launch(HERDR_ENV="")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("bridge", result.stderr)
        self.assertFalse(any(arg.startswith("PISSBOX_BRIDGE_DIR=") for arg in calls[0]))
        self.assertEqual(events, [])


if __name__ == "__main__":
    unittest.main()
