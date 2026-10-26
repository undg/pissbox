import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from launcher_harness import checkout_root


ROOT = checkout_root()
LAUNCHER = ROOT / "bin/pissbox"
TOOL_PATH = os.environ["PATH"] + os.pathsep + str(ROOT / ".cache/bin")


class IsolatedDirectoriesTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not shutil.which("tomljson", path=TOOL_PATH) or not shutil.which("jq", path=TOOL_PATH):
            raise unittest.SkipTest("Isolation integration tests require tomljson and jq")

    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = tempfile.TemporaryDirectory(prefix="isolation-", dir=fixtures)
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.project = self.root / "project with spaces"
        self.project.mkdir()
        self.config = self.project / ".pissbox.toml"
        self.state = self.root / "state"
        self.pi = self.root / "pi"
        self.pi.mkdir()
        for profile in ("settings-co.json", "settings-go.json"):
            (self.pi / profile).write_text("{}")
        self.tools = self.root / "tools"
        self.tools.mkdir()
        (self.tools / "sbx").symlink_to(ROOT / "tests/fixtures/sbx-isolation")

    def configure(self, paths=None, commands=None):
        self.config.write_text("isolated_directories = " + json.dumps(paths if paths is not None else ["node_modules"]) +
                               "\nsetup = " + json.dumps(commands if commands is not None else ["printf ready > setup-marker"]))

    def launch(self, *args, answers="", **env):
        environment = {key: value for key, value in os.environ.items()
                       if not key.startswith(("PISSBOX_", "TEST_ISOLATION_"))}
        environment.update(PATH=str(self.tools) + os.pathsep + TOOL_PATH,
                           PISSBOX_PI_DIR=str(self.pi), PISSBOX_STATE_DIR=str(self.state),
                           TEST_ISOLATION_ROOT=str(self.root))
        environment.update(env)
        result = subprocess.run([str(LAUNCHER), *args], cwd=self.project, env=environment,
                                input=answers, text=True, capture_output=True, timeout=10)
        log = self.root / "calls.jsonl"
        calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
        return result, calls

    def test_nameless_creation_mounts_before_setup_and_preserves_host_dependencies(self):
        self.configure(["node_modules", "packages/app/node_modules"])
        dependencies = self.project / "node_modules"
        dependencies.mkdir()
        (dependencies / "host-package").write_text("Darwin dependencies")
        (self.project / "pnpm-lock.yaml").write_text("shared lockfile")
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[0][0], "create")
        self.assertNotIn("--name", calls[0])
        self.assertEqual([call[0] for call in calls], ["create", "mount", "exec", "mount", "exec", "exec", "run"])
        self.assertEqual(calls[-1], ["run", "--name", "sbx-assigned-not-guessed"])
        self.assertTrue(all(call[1] == "sbx-assigned-not-guessed" for call in calls if call[0] == "mount"))
        self.assertEqual((self.project / "setup-marker").read_text(), "ready")
        self.assertEqual(list(dependencies.iterdir()), [dependencies / "host-package"])
        self.assertEqual((dependencies / "host-package").read_text(), "Darwin dependencies")
        self.assertEqual((self.project / "pnpm-lock.yaml").read_text(), "shared lockfile")
        self.assertEqual(list((self.project / "packages/app/node_modules").iterdir()), [])
        data = json.loads((self.root / "sandbox.json").read_text())
        for backing in data["mounts"].values():
            self.assertEqual(list(Path(backing).iterdir()), [Path(backing) / ".pissbox-isolation-mount"])
        self.assertEqual((self.root / "home/.pissbox/isolation-ready").read_text().strip(),
                         data["env"]["PISSBOX_ISOLATION_DIR"])

    def test_continue_menu_choice_attaches_without_replaying_isolation(self):
        self.configure()
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("--continue", calls[-1])
        (self.project / "setup-marker").unlink()
        (self.root / "calls.jsonl").unlink()

        result, calls = self.launch(answers="C")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["create", "exec", "run"])
        self.assertEqual(calls[-1], ["run", "--name", "sbx-assigned-not-guessed", "--", "--continue"])
        self.assertFalse((self.project / "setup-marker").exists())

    def test_continue_flag_applies_after_isolation_setup_and_on_yes_attach(self):
        self.configure()
        result, calls = self.launch("-c")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("--continue", calls[0])
        self.assertEqual(calls[-1], ["run", "--name", "sbx-assigned-not-guessed", "--", "--continue"])
        self.assertTrue((self.project / "setup-marker").exists())
        (self.root / "calls.jsonl").unlink()

        result, calls = self.launch("--continue", "-y")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["create", "exec", "run"])
        self.assertEqual(calls[-1], ["run", "--name", "sbx-assigned-not-guessed", "--", "--continue"])

    def test_explicit_name_is_optional_and_forwarded_only_when_requested(self):
        self.configure()
        result, calls = self.launch("--name", "second-instance")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[0][calls[0].index("--name") + 1], "second-instance")
        self.assertEqual(calls[-1], ["run", "--name", "second-instance"])

    def test_absent_config_does_not_accept_inherited_internal_config(self):
        result, calls = self.launch(PROJECT_JSON='{"isolated_directories":["node_modules"]}')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["run"])
        self.assertFalse(list(self.state.glob("isolation.*")))

    def test_unknown_explicit_name_conflict_preserves_error_without_attachment(self):
        self.configure()
        result, calls = self.launch("--name", "second-instance", "-y",
                                    TEST_ISOLATION_UNKNOWN_CREATE_CONFLICT="1")
        self.assertEqual(result.returncode, 8, result.stderr)
        self.assertIn("unrecognized test wording", result.stderr)
        self.assertEqual([call[0] for call in calls], ["create"])
        self.assertFalse((self.project / "node_modules").exists())

    def test_empty_isolation_uses_original_run_without_staging(self):
        self.configure([])
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["run"])
        self.assertFalse(list(self.state.glob("isolation.*")))
        self.assertFalse((self.project / "node_modules").exists())

    def test_files_and_symlink_components_fail_before_settings_state_or_sbx(self):
        target = self.project / "target"
        target.mkdir()
        (self.project / "file").write_text("keep")
        (self.project / "alias").symlink_to(target, target_is_directory=True)
        (self.project / "broken").symlink_to(self.project / "missing")
        for path in ("file", "file/node_modules", "alias", "alias/node_modules", "broken/node_modules"):
            with self.subTest(path=path):
                self.configure([path])
                result, calls = self.launch()
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("never files or symlinks", result.stderr)
                self.assertEqual(calls, [])
                self.assertFalse(self.state.exists())
                self.assertFalse((self.pi / "settings.json").exists())

    def test_invalid_schema_and_mount_path_characters_fail_before_effects(self):
        for paths in (["a/../b"], ["a", "a/b"], ["a:b"], ["a\nb"]):
            with self.subTest(paths=paths):
                self.configure(paths)
                result, calls = self.launch()
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(calls, [])
                self.assertFalse(self.state.exists())
                self.assertFalse((self.pi / "settings.json").exists())
        self.configure()
        for name in ("bad:state", "bad\nstate", "bad\x85state"):
            with self.subTest(state=name):
                result, calls = self.launch(PISSBOX_STATE_DIR=str(self.root / name))
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(calls, [])
                self.assertFalse((self.pi / "settings.json").exists())
        physical = self.root / "physical:state"
        physical.mkdir()
        alias = self.root / "state-alias"
        alias.symlink_to(physical, target_is_directory=True)
        result, calls = self.launch(PISSBOX_STATE_DIR=str(alias / "not-created-yet"))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])
        self.assertFalse((physical / "not-created-yet").exists())
        self.assertFalse((self.pi / "settings.json").exists())

    def test_missing_or_invalid_identity_never_drives_named_actions(self):
        self.configure()
        for flag in ("TEST_ISOLATION_NO_IDENTITY", "TEST_ISOLATION_INVALID_IDENTITY"):
            with self.subTest(flag=flag):
                (self.root / "sandbox.json").unlink(missing_ok=True)
                (self.root / "calls.jsonl").unlink(missing_ok=True)
                shutil.rmtree(self.root / "home", ignore_errors=True)
                result, calls = self.launch(**{flag: "1"})
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual([call[0] for call in calls], ["create"])
                self.assertFalse((self.project / "setup-marker").exists())

    def test_creation_and_mount_failures_preserve_status_without_starting_pi(self):
        self.configure()
        result, calls = self.launch(TEST_ISOLATION_CREATE_STATUS="7")
        self.assertEqual(result.returncode, 7, result.stderr)
        self.assertEqual([call[0] for call in calls], ["create"])
        (self.root / "calls.jsonl").unlink()
        result, calls = self.launch(TEST_ISOLATION_MOUNT_STATUS="9")
        self.assertEqual(result.returncode, 9, result.stderr)
        self.assertEqual([call[0] for call in calls], ["create", "mount"])
        self.assertFalse((self.project / "setup-marker").exists())
        self.assertFalse((self.root / "home/.pissbox/isolation-ready").exists())

    def test_mountpoint_mkdir_failure_stops_before_mount_or_setup(self):
        # Fail at the filesystem boundary, not by stubbing a mkdir subprocess.
        parent = self.project / "read-only"
        parent.mkdir(mode=0o555)
        self.addCleanup(parent.chmod, 0o755)
        path = "read-only/node_modules"
        self.configure([path])
        result, calls = self.launch()
        self.assertNotEqual(result.returncode, 0, result.stderr)
        self.assertIn("permission denied", result.stderr.lower())
        self.assertEqual([call[0] for call in calls], ["create"])
        self.assertFalse((self.project / path).exists())
        self.assertFalse((self.project / "setup-marker").exists())

    def test_symlink_introduced_during_creation_is_refused_before_mount(self):
        self.configure()
        result, calls = self.launch(TEST_ISOLATION_NEW_SYMLINK="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("never files or symlinks", result.stderr)
        self.assertEqual([call[0] for call in calls], ["create"])
        self.assertTrue((self.project / "node_modules").is_symlink())
        self.assertFalse((self.project / "setup-marker").exists())

    def test_uid1000_probe_failure_stops_before_setup_and_pi(self):
        self.configure()
        result, calls = self.launch(TEST_ISOLATION_WRITE_STATUS="13")
        self.assertEqual(result.returncode, 13, result.stderr)
        self.assertEqual([call[0] for call in calls], ["create", "mount", "exec"])
        self.assertFalse((self.project / "setup-marker").exists())
        self.assertFalse((self.root / "home/.pissbox/isolation-ready").exists())

    def test_unverified_mount_is_not_written_and_setup_is_not_run(self):
        self.configure()
        result, calls = self.launch(TEST_ISOLATION_BAD_MOUNT="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual([call[0] for call in calls], ["create", "mount", "exec"])
        self.assertEqual(list((self.project / "node_modules").iterdir()), [])
        self.assertFalse((self.project / "setup-marker").exists())

    def test_final_session_failure_preserves_status_without_menu_or_replay(self):
        self.configure()
        result, calls = self.launch(TEST_ISOLATION_RUN_STATUS="42")
        self.assertEqual(result.returncode, 42, result.stderr)
        self.assertEqual(calls[-1], ["run", "--name", "sbx-assigned-not-guessed"])
        self.assertNotIn("Choose [P/c/s/d/q]", result.stderr)
        self.assertTrue((self.root / "home/.pissbox/isolation-ready").exists())

    def test_setup_failure_blocks_pi_and_shell_even_after_config_removal(self):
        self.configure(commands=["exit 23"])
        result, _ = self.launch()
        self.assertEqual(result.returncode, 23, result.stderr)
        self.config.unlink()
        for answers in ("p", "c", "s"):
            with self.subTest(answers=answers):
                (self.root / "calls.jsonl").unlink()
                result, calls = self.launch(answers=answers)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Isolation setup is incomplete", result.stderr)
                self.assertEqual([call[0] for call in calls], ["run", "exec"])
                self.assertEqual(calls[-1][-1], "pissbox-readiness")
        (self.root / "calls.jsonl").unlink()
        result, calls = self.launch(answers="q\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["run"])
        (self.root / "calls.jsonl").unlink()
        result, calls = self.launch(answers="dy\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["run", "rm"])

    def test_ready_attach_never_remounts_or_replays_setup(self):
        self.configure()
        result, _ = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        (self.project / "setup-marker").unlink()
        (self.root / "calls.jsonl").unlink()
        result, calls = self.launch("-y")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["create", "exec", "run"])
        self.assertEqual(calls[1][-1], "pissbox-readiness")
        self.assertFalse((self.project / "setup-marker").exists())
        (self.root / "calls.jsonl").unlink()
        self.config.unlink()
        result, calls = self.launch(answers="s\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["run", "exec", "exec"])
        self.assertEqual(calls[-1], ["exec", "-it", "sbx-assigned-not-guessed", "bash"])

    def test_named_ready_repeat_supports_pi_shell_and_yes_without_replay(self):
        self.configure()
        result, _ = self.launch("--name", "named-instance")
        self.assertEqual(result.returncode, 0, result.stderr)
        (self.project / "setup-marker").unlink()
        for choice, extra in (("p\n", ()), ("s\n", ()), ("", ("-y",))):
            with self.subTest(choice=choice, extra=extra):
                (self.root / "calls.jsonl").unlink()
                result, calls = self.launch("--name", "named-instance", *extra, answers=choice)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual([call[0] for call in calls], ["create", "exec", "exec" if choice == "s\n" else "run"])
                self.assertEqual(calls[1][-1], "pissbox-readiness")
                expected = ["exec", "-it", "named-instance", "bash"] if choice == "s\n" else ["run", "--name", "named-instance"]
                self.assertEqual(calls[-1], expected)
                self.assertFalse((self.project / "setup-marker").exists())

    def test_named_pending_repeat_denies_pi_shell_and_yes_without_replay(self):
        self.configure(commands=["exit 23"])
        result, _ = self.launch("--name", "pending-instance")
        self.assertEqual(result.returncode, 23, result.stderr)
        self.configure(commands=["touch replayed"])
        for choice, extra in (("p\n", ()), ("s\n", ()), ("", ("-y",))):
            with self.subTest(choice=choice, extra=extra):
                (self.root / "calls.jsonl").unlink()
                result, calls = self.launch("--name", "pending-instance", *extra, answers=choice)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Isolation setup is incomplete", result.stderr)
                self.assertEqual([call[0] for call in calls], ["create", "exec"])
                self.assertFalse((self.project / "replayed").exists())

    def test_named_conflict_must_report_the_requested_name(self):
        self.configure()
        result, calls = self.launch("--name", "requested-instance", "-y",
                                    TEST_ISOLATION_CONFLICT="1", TEST_ISOLATION_REPORTED_NAME="different-instance")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("different-instance", result.stderr)
        self.assertEqual([call[0] for call in calls], ["create"])
        self.assertNotIn("Choose [P/c/s/d/q]", result.stderr)
        self.assertFalse((self.project / "node_modules").exists())

    def test_named_conflict_wording_is_not_used_for_nameless_creation(self):
        self.configure()
        result, calls = self.launch("-y", TEST_ISOLATION_CONFLICT="1",
                                    TEST_ISOLATION_FORCE_NAMED_WORDING="1")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual([call[0] for call in calls], ["create"])
        self.assertNotIn("Choose [P/c/s/d/q]", result.stderr)
        self.assertFalse((self.project / "node_modules").exists())

    def test_old_sandbox_conflict_is_not_staged_and_quit_delete_remain_available(self):
        self.configure()
        result, calls = self.launch("-y", TEST_ISOLATION_CONFLICT="1")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls], ["create", "exec", "run"])
        self.assertEqual(calls[-1], ["run", "--name", "existing-sandbox"])
        self.assertFalse((self.project / "node_modules").exists())
        self.assertFalse((self.project / "setup-marker").exists())


if __name__ == "__main__":
    unittest.main()
