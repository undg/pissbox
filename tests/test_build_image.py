import json
import os
from pathlib import Path
import shutil
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
        self.root = self.directory / "checkout"
        (self.root / "bin").mkdir(parents=True)
        (self.root / "sbx-kit").mkdir()
        self.script = self.root / "bin/build-image"
        shutil.copy2(SCRIPT, self.script)
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
        uname = self.bin / "uname"
        uname.write_text('#!/bin/sh\necho "${BUILD_TEST_UNAME:-Linux}"\n')
        uname.chmod(0o755)

    def launch(self, *args, script=None, fail="", env=None):
        result = subprocess.run(
            [str(script or self.script), *args], cwd=self.directory, text=True,
            capture_output=True, timeout=5,
            env={**os.environ, "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
                 "BUILD_TEST_LOG": str(self.log), "BUILD_TEST_FAIL": fail, **(env or {})},
        )
        calls = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return result, calls

    def expected_calls(self, *build_args):
        archive = str(self.root / ".cache/images/pissbox-local.tar")
        return [
            ["docker", "buildx", "version"],
            ["docker", "build", "-t", "pissbox:local", *build_args, str(self.root / "sbx-kit")],
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
        link.symlink_to(os.path.relpath(self.script, link.parent))
        args = ("--build-arg", "PI_VERSION=1.2.3", "--label", "description=two words")
        result, calls = self.launch("--", *args, script=link)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, self.expected_calls(*args))

    def test_optional_ca_is_forwarded_with_build_arguments(self):
        ca = self.root / "sbx-kit/ca.crt"
        ca.write_text("public CA fixture; Docker is stubbed\n")
        result, calls = self.launch("--", "--no-cache")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, self.expected_calls(
            "--secret", f"id=custom_ca,src={ca}", "--no-cache"))
        self.assertNotIn("Info:", result.stderr)

    def test_optional_ca_through_symlink(self):
        ca = self.root / "sbx-kit/ca.crt"
        ca.write_text("public CA fixture; Docker is stubbed\n")
        link = self.directory / "build-image-with-ca"
        link.symlink_to(os.path.relpath(self.script, link.parent))
        result, calls = self.launch(script=link)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, self.expected_calls(
            "--secret", f"id=custom_ca,src={ca}"))

    def test_ca_in_callers_directory_is_not_used(self):
        (self.directory / "ca.crt").write_text("not the checkout CA\n")
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, self.expected_calls())
        self.assertFalse((self.root / "sbx-kit/ca.crt").exists())
        self.assertIn("Info:", result.stderr)
        self.assertIn("ca.crt not found", result.stderr)
        self.assertIn("base image's trust store", result.stderr)

    def test_help_never_runs_tools(self):
        for flag in ("-h", "--help"):
            with self.subTest(flag=flag):
                result, calls = self.launch(flag)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("Usage:", result.stdout)
                self.assertEqual(calls, [])

    def test_missing_buildx_fails_before_building(self):
        cases = (
            ("Darwin", "brew install docker-buildx", "pacman"),
            ("Linux", "pacman -S docker-buildx", "brew install"),
        )
        for system, hint, absent in cases:
            with self.subTest(system=system):
                self.log.write_text("")
                result, calls = self.launch(
                    fail="docker buildx", env={"BUILD_TEST_UNAME": system})
                self.assertEqual(result.returncode, 1, result.stderr)
                self.assertIn("docker buildx plugin is missing", result.stderr)
                self.assertIn("https://docs.docker.com/go/buildx/", result.stderr)
                self.assertIn(hint, result.stderr)
                self.assertNotIn(absent, result.stderr)
                self.assertEqual(calls, [["docker", "buildx", "version"]])

    def test_unexpected_arguments_never_run_tools(self):
        for arg in ("--unknown", "workspace"):
            with self.subTest(arg=arg):
                result, calls = self.launch(arg)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("--help", result.stderr)
                self.assertEqual(calls, [])

    def test_failures_stop_the_pipeline_and_preserve_exit_status(self):
        for step, count in (("docker build", 2), ("docker image", 3), ("sbx template", 4)):
            with self.subTest(step=step):
                self.log.write_text("")
                result, calls = self.launch(fail=step)
                self.assertEqual(result.returncode, 37, result.stderr)
                self.assertEqual(calls, self.expected_calls()[:count])


if __name__ == "__main__":
    unittest.main()
