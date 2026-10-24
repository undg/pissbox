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
        shutil.copytree(ROOT / "pi-agent/extensions/lib", pi / "extensions/lib")
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
        calls = [["readiness", args[3]] if args[-1:] == ["pissbox-readiness"] else args
                 for args in calls]
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
        self.assertEqual(calls[1], ["readiness", "pi-existing"])
        self.assertEqual(calls[2], ["run", "--name", "pi-existing", "--env", "PISSBOX_BRIDGE_DIR=" + second])

    def test_shell_choice_unregisters_without_registering_another(self):
        result, calls, events = self.launch("s\n", TEST_SBX_MODE="conflict")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[1:], [["readiness", "pi-existing"], ["exec", "-it", "pi-existing", "bash"]])
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

    def test_missing_messaging_helper_warns_but_launches_pi(self):
        (self.checkout / "pi-agent/extensions/lib/agent-messaging.js").unlink()
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
        runtime = tempfile.mkdtemp(prefix="", dir=ROOT / ".cache/test-fixtures")
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
            if process.poll() is None:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            return process.communicate(timeout=5)
        self.addCleanup(kill_launcher_group)
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            result = control("status")
            if result.returncode == 0 and json.loads(result.stdout)["registrations"] == 1:
                break
            time.sleep(0.02)
        else:
            launcher_status = process.poll()
            stdout, stderr = kill_launcher_group()
            socket = str(Path(runtime) / "control.sock")
            self.fail(
                f"launcher did not register with real daemon; socket={socket!r} "
                f"({len(os.fsencode(socket))} bytes, limit 103); "
                f"launcher status before cleanup={launcher_status}; "
                f"launcher stdout={stdout.decode()!r}; launcher stderr={stderr.decode()!r}; "
                f"last control status={result.returncode}, stdout={result.stdout!r}, stderr={result.stderr!r}"
            )
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

    def test_real_bridge_waits_for_isolation_and_survives_normal_exit_and_crash(self):
        if not shutil.which("go"):
            self.skipTest("real bridge process test requires Go")
        self.binary.unlink()
        subprocess.run(["go", "build", "-o", str(self.binary), "."], cwd=ROOT / "go-bridge",
                       check=True, capture_output=True, timeout=60)
        shutil.copytree(ROOT / "bin/lib", self.checkout / "bin/lib")
        scripts = self.checkout / "sbx-kit/files/home/.pissbox/scripts"
        scripts.mkdir(parents=True)
        shutil.copy2(ROOT / "sbx-kit/files/home/.pissbox/scripts/project-setup.sh", scripts)
        project = self.checkout / "isolated-project"
        project.mkdir()
        (project / ".pissbox.toml").write_text('isolated_directories = ["node_modules"]\nsetup = ["touch setup-done"]\n')
        fixture = self.checkout / "sbx-fixture"
        fixture.mkdir()
        tools = self.checkout / "tools"
        tools.mkdir()
        (tools / "sbx").symlink_to(ROOT / "tests/fixtures/sbx-isolation")
        runtime = Path(tempfile.mkdtemp(prefix="", dir=ROOT / ".cache/test-fixtures"))
        socket = runtime / "control.sock"
        self.assertLessEqual(len(os.fsencode(socket)), 103,
                             f"Private bridge socket exceeds 103 bytes; use a shorter checkout path: {socket}")
        env = self.environment(
            PATH=str(tools) + os.pathsep + os.environ["PATH"], HERDR_ENV="",
            PISSBOX_BRIDGE_RUNTIME_DIR=str(runtime), TEST_ISOLATION_ROOT=str(fixture),
            TEST_ISOLATION_EXPECT_BRIDGE="1", TEST_ISOLATION_PAUSE_FINALIZE="1",
        )

        def control(operation):
            return subprocess.run([str(self.binary), operation], env=env,
                                  capture_output=True, text=True, timeout=6)

        def stop_daemon():
            if socket.exists():
                result = control("stop")
                self.assertEqual(result.returncode, 0, result.stderr)
                deadline = time.monotonic() + 5
                while socket.exists() and time.monotonic() < deadline:
                    time.sleep(0.02)
                self.assertFalse(socket.exists(), "private test daemon did not stop")

        def terminate(process):
            if process.poll() is None:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            return process.communicate(timeout=5)

        def await_registration(count):
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:
                result = control("status")
                if result.returncode == 0:
                    status = json.loads(result.stdout)
                    if status["registrations"] == count:
                        return status
                time.sleep(0.02)
            self.fail(f"expected {count} registrations; control status={result.returncode}, "
                      f"stdout={result.stdout!r}, stderr={result.stderr!r}")

        self.addCleanup(stop_daemon)
        command = [str(self.launcher), "-y", str(project)]
        process = subprocess.Popen(command, cwd=self.checkout, env=env,
                                   stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, start_new_session=True)
        self.addCleanup(terminate, process)
        deadline = time.monotonic() + 5
        while not (fixture / "finalize-waiting").exists() and time.monotonic() < deadline:
            if process.poll() is not None:
                stdout, stderr = process.communicate(timeout=5)
                self.fail(f"launcher failed before staged pause: status={process.returncode}, "
                          f"stdout={stdout!r}, stderr={stderr!r}")
            time.sleep(0.02)
        self.assertTrue((fixture / "finalize-waiting").exists(), "launcher did not reach finalization pause")
        self.assertFalse(socket.exists(), "bridge started before isolation finished")
        self.assertEqual(list((self.checkout / ".cache").glob("bridge-launch.*")), [])
        self.assertFalse((project / "setup-done").exists())
        (fixture / "allow-finalize").touch()
        try:
            stdout, stderr = process.communicate(timeout=10)
        except subprocess.TimeoutExpired:
            launcher_status = process.poll()
            daemon_status = control("status")
            stop_daemon()
            stdout, stderr = process.communicate(timeout=5)
            self.fail(f"launcher status={launcher_status} but captured IO stayed open until private daemon stopped; "
                      f"control status={daemon_status.returncode}, stdout={daemon_status.stdout!r}, "
                      f"stderr={daemon_status.stderr!r}; launcher stdout={stdout!r}, stderr={stderr!r}")
        self.assertEqual(process.returncode, 0, f"stdout={stdout!r}, stderr={stderr!r}")
        self.assertTrue((project / "setup-done").exists())
        observed = [Path(json.loads(line)) for line in (fixture / "bridge-observed.jsonl").read_text().splitlines()]
        self.assertEqual(len(observed), 1)
        self.assertFalse((observed[0] / "ready").exists(), "normal launcher exit did not unregister")
        pid = await_registration(0)["pid"]

        (project / "setup-done").unlink()
        crash = subprocess.Popen(command, cwd=self.checkout,
                                 env={**env, "TEST_ISOLATION_WAIT_RUN": "1"},
                                 stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                 stderr=subprocess.PIPE, start_new_session=True)
        self.addCleanup(terminate, crash)
        self.assertEqual(await_registration(1)["pid"], pid)
        deadline = time.monotonic() + 5
        while len((fixture / "bridge-observed.jsonl").read_text().splitlines()) < 2 and time.monotonic() < deadline:
            time.sleep(0.02)
        observed = [Path(json.loads(line)) for line in (fixture / "bridge-observed.jsonl").read_text().splitlines()]
        self.assertEqual(len(observed), 2, "attached final run did not observe the live bridge mailbox")
        self.assertNotEqual(observed[0], observed[1])
        self.assertTrue((observed[1] / "ready").exists())
        self.assertFalse((project / "setup-done").exists(), "attachment replayed project setup")
        terminate(crash)
        self.assertEqual(await_registration(0)["pid"], pid, "launcher crash killed or replaced the daemon")
        self.assertFalse((observed[1] / "ready").exists(), "owner crash did not release its mailbox")

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

    def test_outside_herd_registers_messaging_without_a_herd_binding(self):
        result, calls, events = self.launch(HERDR_ENV="")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([event["kind"] for event in events], ["register", "unregister"])
        self.assertIsNone(events[0]["herdr_socket"])
        self.assertIsNone(events[0]["herdr_pane"])
        self.assertIn("PISSBOX_BRIDGE_DIR=" + events[0]["mailbox"], calls[0])

    def test_partial_herd_binding_keeps_messaging_available(self):
        result, calls, events = self.launch(HERDR_SOCKET_PATH="")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Herd reporting disabled", result.stderr)
        self.assertEqual([event["kind"] for event in events], ["register", "unregister"])
        self.assertIsNone(events[0]["herdr_socket"])
        self.assertIsNone(events[0]["herdr_pane"])
        self.assertIn("PISSBOX_BRIDGE_DIR=" + events[0]["mailbox"], calls[0])


if __name__ == "__main__":
    unittest.main()
