import json
import os
from pathlib import Path
import pty
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
LAUNCHER = ROOT / "bin" / "pissbox"
PI_DIR = ROOT / "pi-agent"
STATE_DIR = ROOT / ".cache"


class LauncherTests(unittest.TestCase):
    def environment(self, **overrides):
        env = {
            key: value
            for key, value in os.environ.items()
            if not key.startswith("TEST_SBX_")
            and key not in ("PISSBOX_PI_DIR", "PISSBOX_STATE_DIR")
        }
        env["PATH"] = str(ROOT / "tests" / "fixtures") + os.pathsep + env["PATH"]
        env.update(overrides)
        return env

    def launch(self, *args, answers="", launcher=LAUNCHER, cwd=ROOT, **env):
        result = subprocess.run(
            [str(launcher), *args],
            cwd=cwd,
            env=self.environment(**env),
            input=answers,
            text=True,
            capture_output=True,
            timeout=5,
        )
        calls = [
            json.loads(line.removeprefix("sbx:"))
            for line in result.stdout.splitlines()
            if line.startswith("sbx:")
        ]
        return result, calls

    def conflict(self, answers, **env):
        return self.launch(answers=answers, TEST_SBX_MODE="conflict", **env)

    def pi_directory(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = Path(tempfile.mkdtemp(prefix="pi-", dir=fixtures))
        for name in ("settings-co.json", "settings-go.json"):
            shutil.copyfile(PI_DIR / name, directory / name)
        return directory

    def test_new_sandbox_keeps_original_arguments(self):
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [[
            "run", "--env", f"PISSBOX_PI_DIR={PI_DIR}",
            "--env", f"PISSBOX_STATE_DIR={STATE_DIR}",
            str(ROOT / "bin" / "../sbx-kit"), str(ROOT), str(PI_DIR), str(STATE_DIR),
        ]])
        self.assertEqual(result.stderr, "")

    def test_launch_from_another_directory(self):
        cwd = ROOT / "tests" / "fixtures"
        for launcher in (LAUNCHER, "pissbox", cwd / "linked bin" / "pissbox"):
            with self.subTest(launcher=launcher):
                result, calls = self.launch(launcher=launcher, cwd=cwd)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stderr, "")
                self.assertEqual(calls, [[
                    "run", "--env", f"PISSBOX_PI_DIR={PI_DIR}",
                    "--env", f"PISSBOX_STATE_DIR={STATE_DIR}",
                    str(ROOT / "bin" / "../sbx-kit"), str(cwd), str(PI_DIR), str(STATE_DIR),
                ]])

    def test_symlink_keeps_overrides_relative_to_caller(self):
        cwd = ROOT / "tests" / "fixtures"
        for args in ((), ("--pissbox-pi-dir", "../../pi-agent")):
            with self.subTest(args=args):
                result, calls = self.launch(
                    *args, ".", launcher="pissbox", cwd=cwd,
                    PISSBOX_PI_DIR="../../pi-agent" if not args else "missing",
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls, [[
                    "run", "--env", f"PISSBOX_PI_DIR={PI_DIR}",
                    "--env", f"PISSBOX_STATE_DIR={STATE_DIR}",
                    str(ROOT / "bin" / "../sbx-kit"), ".", str(PI_DIR), str(STATE_DIR),
                ]])

    def test_state_directory_overrides_are_relative_to_caller(self):
        cwd = ROOT / "tests" / "fixtures"
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = Path(tempfile.mkdtemp(prefix="launcher-", dir=fixtures))
        for index, option in enumerate(("environment", "separate", "equals", "repeated")):
            with self.subTest(option=option):
                state = directory / f"new state {index}"
                relative = os.path.relpath(state, cwd)
                env = {"PISSBOX_STATE_DIR": str(LAUNCHER)}
                if option == "environment":
                    args = ()
                    env["PISSBOX_STATE_DIR"] = relative
                elif option == "separate":
                    args = ("--pissbox-state-dir", relative)
                elif option == "equals":
                    args = (f"--pissbox-state-dir={relative}",)
                else:
                    args = ("--pissbox-state-dir", "unused", f"--pissbox-state-dir={relative}")
                result, calls = self.launch(*args, launcher="pissbox", cwd=cwd, **env)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue(state.is_dir())
                self.assertIn(f"PISSBOX_STATE_DIR={state}", calls[0])
                self.assertEqual(calls[0][-3:], [str(cwd), str(PI_DIR), str(state)])

    def test_state_directory_symlink_is_canonicalized_before_mounting(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = Path(tempfile.mkdtemp(prefix="launcher-", dir=fixtures))
        target = directory / "state"
        target.mkdir()
        link = directory / "link"
        link.symlink_to(target)
        result, calls = self.launch(PISSBOX_STATE_DIR=str(link))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f"PISSBOX_STATE_DIR={target}", calls[0])
        self.assertEqual(calls[0][-1], str(target))

    def test_invalid_state_directory_never_launches_sbx(self):
        for args in (
            ("--pissbox-state-dir",), ("--pissbox-state-dir=",),
            ("--pissbox-state-dir", ""), ("--pissbox-state-dir", "--name=foo"),
            ("--pissbox-state-dir", str(LAUNCHER)),
        ):
            with self.subTest(args=args):
                result, calls = self.launch(*args)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(calls, [])
                self.assertNotEqual(result.stderr, "")

    def test_state_options_can_appear_among_workspaces(self):
        for args in (
            ("one", "--pissbox-state-dir", str(STATE_DIR), "two", "--name=box"),
            ("--name=box", "--pissbox-state-dir", str(STATE_DIR), "one", "two"),
        ):
            with self.subTest(args=args):
                result, calls = self.launch(*args)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[0][-4:], ["one", "two", str(PI_DIR), str(STATE_DIR)])
                self.assertIn("PISSBOX_STATE_DIR=" + str(STATE_DIR), calls[0])
                self.assertIn("box", calls[0])

    def test_settings_profile_follows_hostname(self):
        for host, profile in (
            ("cm", "settings-go.json"),
            ("di-7415", "settings-go.json"),
            ("other-host", "settings-co.json"),
            ("", "settings-co.json"),
        ):
            with self.subTest(host=host):
                pi = self.pi_directory()
                result, calls = self.launch(
                    "--pissbox-pi-dir", str(pi), TEST_PISSBOX_HOST=host,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue((pi / "settings.json").is_symlink())
                self.assertEqual(os.readlink(pi / "settings.json"), profile)
                self.assertIn(f"PISSBOX_PI_DIR={pi}", calls[0])

    def test_switching_host_relinks_settings_profile(self):
        pi = self.pi_directory()
        for host, profile in (
            ("cm", "settings-go.json"),
            ("other-host", "settings-co.json"),
            ("di-7415", "settings-go.json"),
        ):
            with self.subTest(host=host):
                result, _ = self.launch(
                    "--pissbox-pi-dir", str(pi), TEST_PISSBOX_HOST=host,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(os.readlink(pi / "settings.json"), profile)

    def test_settings_profile_backs_up_a_regular_file(self):
        pi = self.pi_directory()
        settings = pi / "settings.json"
        settings.write_text("{}\n")
        result, calls = self.launch(
            "--pissbox-pi-dir", str(pi), TEST_PISSBOX_HOST="cm",
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Backing up existing Pi settings file", result.stderr)
        self.assertEqual((pi / "settings.json.replaced").read_text(), "{}\n")
        self.assertEqual(os.readlink(settings), "settings-go.json")
        self.assertEqual(len(calls), 1)

    def test_missing_settings_profile_never_launches_sbx(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        pi = Path(tempfile.mkdtemp(prefix="pi-missing-", dir=fixtures))
        shutil.copyfile(PI_DIR / "settings-co.json", pi / "settings-co.json")
        result, calls = self.launch(
            "--pissbox-pi-dir", str(pi), TEST_PISSBOX_HOST="cm",
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Settings profile does not exist", result.stderr)
        self.assertEqual(calls, [])

    def test_menu_defaults_to_pi(self):
        result, calls = self.conflict("\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[1:], [["run", "--name", "pi-existing"]])
        self.assertIn("Choose [P/s/d/q]:", result.stderr)
        self.assertNotIn("Do you want to delete", result.stderr)

    def test_pi_choice(self):
        for answer in ("p", "P"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(answer + "\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[1:], [["run", "--name", "pi-existing"]])

    def test_shell_choice(self):
        for answer in ("s", "S"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(answer + "\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[1:], [["exec", "-it", "pi-existing", "bash"]])
                self.assertNotIn("Do you want to delete", result.stderr)

    def test_quit_leaves_sandbox_untouched(self):
        for answer in ("q", "Q"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(answer + "\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(calls), 1)
                self.assertNotIn("Do you want to delete", result.stderr)

    def test_delete_defaults_to_no(self):
        for choice in ("d", "D"):
            with self.subTest(choice=choice):
                result, calls = self.conflict(f"{choice}\n\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(calls), 1)
                self.assertIn("Do you want to delete it? [y/N]", result.stderr)
                self.assertIn("sandbox-local data", result.stderr)

    def test_declining_deletion_leaves_sandbox_untouched(self):
        for answer in ("n", "N", "no", "NO", "No", "nO"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(f"d\n{answer}\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(calls), 1)

    def test_explicit_yes_deletes_and_exits(self):
        for answer in ("y", "Y", "yes", "YES", "Yes", "yEs"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(f"d\n{answer}\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[1:], [["rm", "--force", "pi-existing"]])

    def test_invalid_answers_repeat_the_question(self):
        result, calls = self.conflict("maybe\ny\nd\nsure\n\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertEqual(result.stderr.count("Please choose p, s, d, or q."), 2)
        self.assertEqual(result.stderr.count("Choose [P/s/d/q]:"), 3)
        self.assertEqual(result.stderr.count("Please answer yes or no."), 1)
        self.assertEqual(result.stderr.count("Do you want to delete it? [y/N]"), 2)

    def test_eof_never_attaches_opens_shell_or_deletes(self):
        for answers in ("", "p", "s", "q", "invalid\n", "d\n", "d\ny"):
            with self.subTest(answers=answers):
                result, calls = self.conflict(answers)
                self.assertEqual(result.returncode, 1, result.stderr)
                self.assertEqual(len(calls), 1)

    def test_uses_name_reported_by_sbx(self):
        for answers, expected in (
            ("\n", ["run", "--name", "actual-name"]),
            ("s\n", ["exec", "-it", "actual-name", "bash"]),
            ("d\ny\n", ["rm", "--force", "actual-name"]),
        ):
            with self.subTest(answers=answers):
                result, calls = self.launch(
                    "--name", "requested", answers=answers,
                    TEST_SBX_MODE="conflict", TEST_SBX_NAME="actual-name",
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[1:], [expected])

    def test_unrelated_failure_is_not_a_conflict(self):
        result, calls = self.launch(TEST_SBX_MODE="error")
        self.assertEqual(result.returncode, 7)
        self.assertEqual(result.stderr, "error: daemon unavailable\n")
        self.assertEqual(len(calls), 1)

    def test_nonzero_session_exit_does_not_prompt(self):
        result, calls = self.launch(TEST_SBX_RUN_STATUS="42")
        self.assertEqual(result.returncode, 42)
        self.assertEqual(result.stderr, "")
        self.assertEqual(len(calls), 1)

    def test_pi_shell_and_remove_failures_are_preserved(self):
        for answers, env in (
            ("\n", {"TEST_SBX_ATTACH_STATUS": "9"}),
            ("s\n", {"TEST_SBX_EXEC_STATUS": "9"}),
            ("d\ny\n", {"TEST_SBX_REMOVE_STATUS": "9"}),
        ):
            with self.subTest(env=env):
                result, _ = self.conflict(answers, **env)
                self.assertEqual(result.returncode, 9, result.stderr)

    def test_terminal_stdin_and_stdout_are_preserved(self):
        master, slave = pty.openpty()
        try:
            result = subprocess.run(
                [str(LAUNCHER)], cwd=ROOT,
                env=self.environment(TEST_SBX_REQUIRE_TTY="1"),
                stdin=slave, stdout=slave, stderr=subprocess.PIPE,
                text=True, timeout=5,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stderr, "")
        finally:
            os.close(master)
            os.close(slave)

    def test_menu_preserves_terminal_stdin_and_stdout(self):
        for answer in (b"\n", b"p\n", b"s\n"):
            with self.subTest(answer=answer):
                master, slave = pty.openpty()
                try:
                    os.write(master, answer)
                    result = subprocess.run(
                        [str(LAUNCHER)], cwd=ROOT,
                        env=self.environment(
                            TEST_SBX_REQUIRE_TTY="1", TEST_SBX_MODE="conflict",
                        ),
                        stdin=slave, stdout=slave, stderr=subprocess.PIPE,
                        text=True, timeout=5,
                    )
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertIn("Choose [P/s/d/q]:", result.stderr)
                finally:
                    os.close(master)
                    os.close(slave)

    def test_help_does_not_launch_sbx_or_validate_pi_directory(self):
        for args in (("-h",), ("--help",), (".", "--help")):
            with self.subTest(args=args):
                result, calls = self.launch(
                    *args, PISSBOX_PI_DIR="missing", PISSBOX_STATE_DIR=str(LAUNCHER / "missing"),
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("[P/s/d/q]", result.stdout)
                self.assertEqual(calls, [])

    def test_help_only_needs_cat_not_runtime_commands(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        commands = Path(tempfile.mkdtemp(prefix="help-path-", dir=fixtures))
        (commands / "cat").symlink_to(shutil.which("cat"))
        for args in (("-h",), ("workspace", "--help")):
            with self.subTest(args=args):
                result = subprocess.run(
                    [shutil.which("bash"), str(LAUNCHER), *args],
                    cwd=ROOT,
                    env=self.environment(PATH=str(commands)),
                    text=True,
                    capture_output=True,
                    timeout=5,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("Usage:", result.stdout)
                self.assertEqual(result.stderr, "")

    def test_empty_directory_options_fail_before_changing_settings(self):
        for option in ("--pissbox-pi-dir", "--pissbox-state-dir"):
            with self.subTest(option=option):
                pi = self.pi_directory()
                result, calls = self.launch(
                    f"{option}=", PISSBOX_PI_DIR=str(pi),
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(f"{option} requires a directory", result.stderr)
                self.assertFalse((pi / "settings.json").is_symlink())
                self.assertEqual(calls, [])

    def test_empty_environment_directories_use_checkout_defaults(self):
        result, calls = self.launch(PISSBOX_PI_DIR="", PISSBOX_STATE_DIR="")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f"PISSBOX_PI_DIR={PI_DIR}", calls[0])
        self.assertIn(f"PISSBOX_STATE_DIR={STATE_DIR}", calls[0])

    def test_workspace_and_option_forwarding(self):
        pi = self.pi_directory()
        result, calls = self.launch(
            "workspace with spaces", "--name=first", "second workspace",
            "--name", "last", "--pissbox-pi-dir", str(pi), "--", "--path",
            PISSBOX_PI_DIR="missing", TEST_PISSBOX_HOST="cm",
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [[
            "run", "--env", f"PISSBOX_PI_DIR={pi}",
            "--env", f"PISSBOX_STATE_DIR={STATE_DIR}",
            "--name", "last", str(ROOT / "bin" / "../sbx-kit"),
            "workspace with spaces", "second workspace", "--path", str(pi),
            str(STATE_DIR),
        ]])


if __name__ == "__main__":
    unittest.main()
