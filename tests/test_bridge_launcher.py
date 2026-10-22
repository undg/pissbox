import json
import os
from pathlib import Path
import platform
import shutil
import signal
import subprocess
import tempfile
import time
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

    def environment(self, **overrides):
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
        return env

    def launch(self, answers="", args=(), **overrides):
        result = subprocess.run(
            [str(self.launcher), *args], cwd=self.checkout,
            env=self.environment(**overrides), input=answers,
            capture_output=True, text=True, timeout=5,
        )
        calls = [json.loads(line[4:]) for line in result.stdout.splitlines() if line.startswith("sbx:")]
        events = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return result, calls, events

    def test_registration_uses_host_pane_and_passes_only_mailbox_to_sandbox(self):
        result, calls, events = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([event["kind"] for event in events], ["register", "unregister"])
        self.assertEqual(events[0]["id"], events[1]["id"])
        self.assertGreater(events[0]["owner_pid"], 1)
        start = events[0]
        self.assertEqual(start["herdr_pane"], "w1:p7")
        self.assertEqual(start["herdr_socket"], "host-only.sock")
        self.assertIn("PISSBOX_BRIDGE_DIR=" + start["mailbox"], calls[0])
        self.assertNotIn("host-only.sock", calls[0])
        self.assertNotIn("w1:p7", calls[0])
        self.assertFalse((Path(start["mailbox"]) / "ready").exists())

    def test_reattach_gets_a_fresh_mailbox_and_registration(self):
        result, calls, events = self.launch("p\n", TEST_SBX_MODE="conflict")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([event["kind"] for event in events], ["register", "unregister", "register", "unregister"])
        first, second = events[0]["mailbox"], events[2]["mailbox"]
        self.assertNotEqual(first, second)
        self.assertEqual(calls[1], ["run", "--name", "pi-existing", "--env", "PISSBOX_BRIDGE_DIR=" + second])

    def test_shell_choice_unregisters_without_registering_another(self):
        result, calls, events = self.launch("s\n", TEST_SBX_MODE="conflict")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[1], ["exec", "-it", "pi-existing", "bash"])
        self.assertEqual([event["kind"] for event in events], ["register", "unregister"])

    def test_quit_and_delete_do_not_register_another_mailbox(self):
        for answers in ("q\n", "d\ny\n"):
            with self.subTest(answers=answers):
                self.log.unlink(missing_ok=True)
                result, calls, events = self.launch(answers, TEST_SBX_MODE="conflict")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual([event["kind"] for event in events], ["register", "unregister"])
                self.assertEqual(len(calls), 2 if answers.startswith("d") else 1)

    def test_help_list_and_delete_do_not_contact_daemon(self):
        for args in (("--help",), ("--ls",), ("--delete", str(self.checkout))):
            with self.subTest(args=args):
                result, calls, events = self.launch(
                    args=args, TEST_SBX_CALL_LOG=str(self.checkout / "sbx.jsonl"),
                )
                self.assertEqual(events, [])
                self.assertEqual(result.returncode, 1 if args[0] == "--delete" else 0, result.stderr)

    def test_custom_state_root_contains_mailbox_not_binary(self):
        state = self.checkout / "custom state"
        result, calls, events = self.launch(PISSBOX_STATE_DIR=str(state))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(Path(events[0]["mailbox"]).parent, state)
        self.assertIn("PISSBOX_STATE_DIR=" + str(state), calls[0])

    def test_missing_extension_warns_but_launches_pi(self):
        (self.checkout / "pi-agent/extensions/pissbox-bridge.js").unlink()
        result, calls, events = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("extension missing", result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertEqual(events, [])

    def test_launcher_crash_unregisters_but_leaves_real_daemon_alive(self):
        if not shutil.which("go"):
            self.skipTest("real bridge process test requires Go")
        self.binary.unlink()
        subprocess.run(
            ["go", "build", "-o", str(self.binary), "."], cwd=ROOT / "go-bridge",
            check=True, capture_output=True, timeout=60,
        )
        runtime = tempfile.mkdtemp(prefix="bc-", dir=ROOT / ".cache/test-fixtures")
        env = self.environment(PISSBOX_BRIDGE_RUNTIME_DIR=runtime, TEST_SBX_MODE="wait")

        def control(operation):
            return subprocess.run(
                [str(self.binary), operation], env=env, capture_output=True, text=True, timeout=6,
            )

        def stop_daemon():
            control("stop")
            deadline = time.monotonic() + 5
            while (Path(runtime) / "control.sock").exists() and time.monotonic() < deadline:
                time.sleep(0.02)
            self.assertFalse((Path(runtime) / "control.sock").exists())

        self.addCleanup(stop_daemon)
        process = subprocess.Popen(
            [str(self.launcher)], cwd=self.checkout, env=env,
            stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            start_new_session=True,
        )
        def kill_launcher_group():
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.communicate(timeout=5)
        self.addCleanup(kill_launcher_group)
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            result = control("status")
            if result.returncode == 0 and json.loads(result.stdout)["registrations"] == 1:
                break
            time.sleep(0.02)
        else:
            self.fail("launcher did not register with real daemon")
        pid = json.loads(result.stdout)["pid"]
        mailbox = next((self.checkout / ".cache").glob("bridge-launch.*"))
        self.assertTrue((mailbox / "ready").exists())
        kill_launcher_group()
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            status = control("status")
            self.assertEqual(status.returncode, 0, status.stderr)
            status = json.loads(status.stdout)
            if status["registrations"] == 0:
                break
            time.sleep(0.02)
        else:
            self.fail("crashed launcher's registration survived")
        self.assertEqual(status["pid"], pid)
        self.assertFalse((mailbox / "ready").exists())

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

    def test_outside_herd_does_not_contact_daemon(self):
        result, calls, events = self.launch(HERDR_ENV="")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("bridge", result.stderr)
        self.assertFalse(any(arg.startswith("PISSBOX_BRIDGE_DIR=") for arg in calls[0]))
        self.assertEqual(events, [])


if __name__ == "__main__":
    unittest.main()
