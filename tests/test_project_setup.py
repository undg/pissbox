import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "sbx-kit/files/home/.pissbox/scripts/run-project-setup.sh"
SPEC = ROOT / "sbx-kit/spec.yaml"
TOOL_PATH = os.environ["PATH"] + os.pathsep + str(ROOT / ".cache/bin")


class ProjectSetupTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not shutil.which("tomljson", path=TOOL_PATH) or not shutil.which("jq", path=TOOL_PATH):
            raise unittest.SkipTest("Project setup integration tests require tomljson and jq; see sbx-kit/README.md")

    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = tempfile.TemporaryDirectory(dir=fixtures)
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.home = self.root / "home"
        scripts = self.home / ".pissbox/scripts"
        scripts.mkdir(parents=True)
        (scripts / SCRIPT.name).symlink_to(SCRIPT)
        self.workspace = self.root / "project with spaces"
        self.workspace.mkdir()
        self.config = self.workspace / ".pissbox.toml"

    def run_setup(self, workspace=None):
        # Exercise the kit command with the real TOML parser and an isolated home.
        command = next(line.split("command: '", 1)[1][:-1]
                       for line in SPEC.read_text().splitlines()
                       if SCRIPT.name in line)
        return subprocess.run(
            ["sh", "-c", command], cwd=self.root,
            env={**os.environ, "HOME": str(self.home), "PATH": TOOL_PATH,
                 "PISSBOX_WORKSPACE_DIR": str(self.workspace) if workspace is None else workspace},
            text=True, capture_output=True, timeout=5,
        )

    def test_missing_config_is_a_noop_and_legacy_script_is_not_executed(self):
        (self.workspace / ".pissbox").write_text("exit 42\n")
        result = self.run_setup()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stderr, "")
        self.assertEqual(list(self.workspace.iterdir()), [self.workspace / ".pissbox"])

    def test_empty_config_or_setup_is_a_noop(self):
        for config in ("# no project setup\n", "setup = []\n", 'setup = [""]\n'):
            with self.subTest(config=config):
                self.config.write_text(config)
                result = self.run_setup()
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stderr, "")

    def test_runs_commands_in_order_in_one_shell_in_primary_workspace(self):
        self.config.write_text(r"""
# Literal TOML strings preserve shell quotes, dollar signs and backslashes.
setup = [
  'steps=(one two)',
  'printf "%s\n" "${steps[@]}" > steps',
  'printf "%s\n" "$PWD" > working-directory',
  'export SETUP_MESSAGE="hello from setup"',
  'mkdir nested',
  'cd nested',
  '''
printf "%s\n" "$SETUP_MESSAGE" > message
printf "%s\n" "$HOME" >> message
''',
]
""")
        result = self.run_setup()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.workspace / "steps").read_text(), "one\ntwo\n")
        self.assertEqual((self.workspace / "working-directory").read_text(), f"{self.workspace}\n")
        self.assertEqual((self.workspace / "nested/message").read_text(), f"hello from setup\n{self.home}\n")
        self.assertFalse(self.config.stat().st_mode & 0o111)

    def test_errors_stop_remaining_commands_and_propagate_status(self):
        for failure, status in (("exit 23", 23), ("false", 1),
                                ("false | true", 1), ('echo "${UNSET_PROJECT_SETUP_VALUE:?}"', 1)):
            with self.subTest(failure=failure):
                self.config.write_text("setup = " + json.dumps([failure, "touch marker"]))
                result = self.run_setup()
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertFalse((self.workspace / "marker").exists())

    def test_invalid_toml_or_schema_runs_no_commands(self):
        for config in (
            'setup = ["touch marker",',
            'setup = ["touch marker"]\nsetup = []',
            'setup = "touch marker"',
            'setup = ["touch marker", 7]',
            'setup = ["touch marker", ["nested"]]',
            'setup = ["touch marker", "\\u0000"]',
            'setup = ["touch marker"]\nsetpu = []',
            '[setup]\ncommands = ["touch marker"]',
        ):
            with self.subTest(config=config):
                self.config.write_text(config)
                result = self.run_setup()
                self.assertNotEqual(result.returncode, 0, result.stderr)
                self.assertFalse((self.workspace / "marker").exists())

    def test_invalid_config_path_is_not_silently_skipped(self):
        self.config.mkdir()
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("readable TOML file", result.stderr)
        self.config.rmdir()
        self.config.symlink_to(self.workspace / "missing")
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("readable TOML file", result.stderr)

    def test_missing_or_relative_workspace_is_rejected(self):
        for workspace in ("", "project with spaces", str(self.root / "missing")):
            with self.subTest(workspace=workspace):
                result = self.run_setup(workspace)
                self.assertNotEqual(result.returncode, 0)

    def test_hook_is_last_creation_step_and_not_the_pi_entrypoint(self):
        spec = SPEC.read_text()
        self.assertIn("entrypoint:\n    - pi\n    - --offline\n", spec)
        install = spec.split("  install:\n", 1)[1]
        commands = [line for line in install.splitlines() if "- command:" in line]
        self.assertIn(SCRIPT.name, commands[-1])
        self.assertIn('user: "1000"', install.split(SCRIPT.name, 1)[1])


if __name__ == "__main__":
    unittest.main()
