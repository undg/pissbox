import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "bin/build-image"


class BuildImageTests(unittest.TestCase):
    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        self.directory = Path(tempfile.mkdtemp(prefix="image build-", dir=fixtures))
        self.bin = self.directory / "bin"
        self.bin.mkdir()
        self.log = self.directory / "calls.jsonl"
        stub = """#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys

name = Path(sys.argv[0]).name
with open(os.environ["BUILD_TEST_LOG"], "a") as log:
    log.write(json.dumps([name, *sys.argv[1:]]) + "\\n")
if os.environ.get("BUILD_TEST_FAIL") == name + " " + sys.argv[1]:
    sys.exit(37)
"""
        for name in ("docker", "sbx"):
            tool = self.bin / name
            tool.write_text(stub)
            tool.chmod(0o755)

    def launch(self, *args, script=SCRIPT, fail=""):
        result = subprocess.run(
            [str(script), *args], cwd=self.directory, text=True,
            capture_output=True, timeout=5,
            env={**os.environ, "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
                 "BUILD_TEST_LOG": str(self.log), "BUILD_TEST_FAIL": fail},
        )
        calls = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return result, calls

    def expected_calls(self, *build_args):
        archive = str(ROOT / ".cache/images/pissbox-local.tar")
        return [
            ["docker", "build", "-t", "pissbox:local", *build_args, str(ROOT / "sbx-kit")],
            ["docker", "image", "save", "pissbox:local", "-o", archive],
            ["sbx", "template", "load", archive],
        ]

    def test_build_save_load_from_another_directory(self):
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, self.expected_calls())

    def test_empty_build_options_after_separator(self):
        result, calls = self.launch("--")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, self.expected_calls())

    def test_symlink_launch_and_build_arguments(self):
        link = self.directory / "build-image"
        link.symlink_to(os.path.relpath(SCRIPT, link.parent))
        args = ("--build-arg", "PI_VERSION=1.2.3", "--label", "description=two words")
        result, calls = self.launch("--", *args, script=link)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, self.expected_calls(*args))

    def test_help_never_runs_tools(self):
        for flag in ("-h", "--help"):
            with self.subTest(flag=flag):
                result, calls = self.launch(flag)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("Usage:", result.stdout)
                self.assertEqual(calls, [])

    def test_unexpected_arguments_never_run_tools(self):
        for arg in ("--unknown", "workspace"):
            with self.subTest(arg=arg):
                result, calls = self.launch(arg)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("--help", result.stderr)
                self.assertEqual(calls, [])

    def test_failures_stop_the_pipeline_and_preserve_exit_status(self):
        for step, count in (("docker build", 1), ("docker image", 2), ("sbx template", 3)):
            with self.subTest(step=step):
                self.log.write_text("")
                result, calls = self.launch(fail=step)
                self.assertEqual(result.returncode, 37, result.stderr)
                self.assertEqual(calls, self.expected_calls()[:count])


if __name__ == "__main__":
    unittest.main()
