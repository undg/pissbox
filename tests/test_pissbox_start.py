import json
import os
from pathlib import Path
import pty
import subprocess
import unittest


ROOT = Path(__file__).resolve().parents[1]
LAUNCHER = ROOT / "bin" / "pissbox"
PI_DIR = ROOT / "pi-agent"


class LauncherTests(unittest.TestCase):
    def environment(self, **overrides):
        env = {
            key: value
            for key, value in os.environ.items()
            if not key.startswith("TEST_SBX_") and key != "PISSBOX_PI_DIR"
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

    def test_new_sandbox_keeps_original_arguments(self):
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [[
            "run", "--env", f"PISSBOX_PI_DIR={PI_DIR}",
            str(ROOT / "bin" / "../sbx-kit"), str(ROOT), str(PI_DIR),
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
                    str(ROOT / "bin" / "../sbx-kit"), str(cwd), str(PI_DIR),
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
                    str(ROOT / "bin" / "../sbx-kit"), ".", str(PI_DIR),
                ]])

    def test_attach_defaults_to_yes(self):
        result, calls = self.conflict("\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[-1], ["run", "--name", "pi-existing"])
        self.assertIn("Get into it? [Y/n]", result.stderr)
        self.assertNotIn("Do you want to delete", result.stderr)

    def test_attach_accepts_yes_variants(self):
        for answer in ("y", "Y", "yes", "YES", "Yes", "yEs"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(answer + "\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[-1], ["run", "--name", "pi-existing"])

    def test_delete_defaults_to_no(self):
        result, calls = self.conflict("n\n\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertIn("Do you want to delete it? [y/N]", result.stderr)
        self.assertIn("sandbox-local data", result.stderr)

    def test_decline_both_leaves_sandbox_untouched(self):
        for answer in ("n", "N", "no", "NO", "No", "nO"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(f"{answer}\n{answer}\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(calls), 1)

    def test_explicit_yes_deletes_and_exits(self):
        for answer in ("y", "Y", "yes", "YES", "Yes", "yEs"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(f"n\n{answer}\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[1:], [["rm", "--force", "pi-existing"]])

    def test_invalid_answers_repeat_the_question(self):
        result, calls = self.conflict("maybe\nn\nsure\n\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertEqual(result.stderr.count("Please answer yes or no."), 2)
        self.assertEqual(result.stderr.count("Get into it? [Y/n]"), 2)
        self.assertEqual(result.stderr.count("Do you want to delete it? [y/N]"), 2)

    def test_eof_never_attaches_or_deletes(self):
        for answers in ("", "n\n", "n\ny"):
            with self.subTest(answers=answers):
                result, calls = self.conflict(answers)
                self.assertEqual(result.returncode, 1, result.stderr)
                self.assertEqual(len(calls), 1)

    def test_uses_name_reported_by_sbx(self):
        result, calls = self.launch(
            "--name", "requested", answers="\n",
            TEST_SBX_MODE="conflict", TEST_SBX_NAME="actual-name",
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[-1], ["run", "--name", "actual-name"])

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

    def test_attach_and_remove_failures_are_preserved(self):
        for answers, env in (
            ("\n", {"TEST_SBX_ATTACH_STATUS": "9"}),
            ("n\ny\n", {"TEST_SBX_REMOVE_STATUS": "9"}),
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

    def test_help_does_not_launch_sbx_or_validate_pi_directory(self):
        for args in (("-h",), ("--help",), (".", "--help")):
            with self.subTest(args=args):
                result, calls = self.launch(*args, PISSBOX_PI_DIR="missing")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("[Y/n]", result.stdout)
                self.assertEqual(calls, [])

    def test_workspace_and_option_forwarding(self):
        result, calls = self.launch(
            "workspace with spaces", "--name=first", "second workspace",
            "--name", "last", "--pissbox-pi-dir", "sbx-kit", "--", "--path",
            PISSBOX_PI_DIR="missing",
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [[
            "run", "--env", f"PISSBOX_PI_DIR={ROOT / 'sbx-kit'}",
            "--name", "last", str(ROOT / "bin" / "../sbx-kit"),
            "workspace with spaces", "second workspace", "--path", str(ROOT / "sbx-kit"),
        ]])


if __name__ == "__main__":
    unittest.main()
