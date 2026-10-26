import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class MakeBuildTests(unittest.TestCase):
    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = tempfile.TemporaryDirectory(prefix="make-build-", dir=fixtures)
        self.addCleanup(directory.cleanup)
        self.directory = Path(directory.name)
        (self.directory / "bin").mkdir()
        self.log = self.directory / "calls"
        for name in ("install", "build-image", "build-bridge", "build-launcher", "artifacts"):
            script = self.directory / "bin" / name
            script.write_text(
                '#!/bin/sh\n'
                'printf "%s\\n" "$0" "$@" >> "$MAKE_BUILD_LOG"\n'
                'if [ -n "${MAKE_BUILD_FAIL:-}" ] && [ "${0##*/}" != "$MAKE_BUILD_FAIL" ]; then exit 0; fi\n'
                'exit "${MAKE_BUILD_STATUS:-0}"\n'
            )
            script.chmod(0o755)

    def run_make(self, target, status="0", fail="", variables=()):
        return subprocess.run(
            ["make", "--no-print-directory", "-f", str(ROOT / "Makefile"), target, *variables],
            cwd=self.directory, text=True, capture_output=True, timeout=5,
            env={**os.environ, "MAKE_BUILD_LOG": str(self.log), "MAKE_BUILD_STATUS": status,
                 "MAKE_BUILD_FAIL": fail},
        )

    def test_explicit_build_targets_call_wrappers_even_with_matching_files(self):
        for target, command in (
            ("install", ["bin/install"]),
            ("build-image", ["bin/build-image"]),
            ("build-image-all", ["bin/build-image", "--amd64", "--arm64"]),
            ("build-bridge", ["bin/build-bridge"]),
            ("build-bridge-all", ["bin/build-bridge", "--all"]),
            ("build-launcher", ["bin/build-launcher"]),
            ("build-launcher-all", ["bin/build-launcher", "--all"]),
        ):
            with self.subTest(target=target):
                (self.directory / target).touch()
                self.log.write_text("")
                result = self.run_make(target)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.log.read_text().splitlines(), command)

    def test_artifact_targets_forward_archive_as_one_argument_and_preserve_failures(self):
        archive = "runtime artifacts; touch unexpected.tar.gz"
        for target, flag in (("export-artifacts", "--export"), ("import-artifacts", "--import")):
            for status in ("0", "37"):
                with self.subTest(target=target, status=status):
                    (self.directory / target).touch()
                    self.log.write_text("")
                    result = self.run_make(target, status=status, variables=(f"ARCHIVE={archive}",))
                    self.assertEqual(result.returncode == 0, status == "0", result.stderr)
                    self.assertEqual(self.log.read_text().splitlines(), ["bin/artifacts", flag, archive])
                    self.assertFalse((self.directory / "unexpected.tar.gz").exists())

    def test_artifact_targets_require_an_archive_before_running_the_script(self):
        for target in ("export-artifacts", "import-artifacts"):
            with self.subTest(target=target):
                result = self.run_make(target, variables=("ARCHIVE=",))
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Set ARCHIVE=", result.stderr)
                self.assertFalse(self.log.exists())

    def test_build_runs_both_host_wrappers_even_with_matching_file(self):
        (self.directory / "build").touch()
        result = self.run_make("build")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.log.read_text().splitlines(), ["bin/build-bridge", "bin/build-image"])

    def test_build_fails_if_either_host_wrapper_fails(self):
        for failed_tool, calls in (
            ("build-bridge", ["bin/build-bridge"]),
            ("build-image", ["bin/build-bridge", "bin/build-image"]),
        ):
            with self.subTest(failed_tool=failed_tool):
                self.log.write_text("")
                result = self.run_make("build", status="37", fail=failed_tool)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.log.read_text().splitlines(), calls)
                self.assertIn("Error 37", result.stderr)

    def test_wrapper_failures_fail_make(self):
        for target in ("install", "build-image", "build-image-all", "build-bridge", "build-bridge-all",
                       "build-launcher", "build-launcher-all"):
            with self.subTest(target=target):
                result = self.run_make(target, status="37")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Error 37", result.stderr)

    def test_help_lists_aggregate_and_explicit_build_names(self):
        result = self.run_make("help")
        self.assertEqual(result.returncode, 0, result.stderr)
        names = [line.split()[0] for line in result.stdout.splitlines() if line.startswith("  ")]
        for target in ("install", "build", "build-image", "build-image-all", "build-bridge", "build-bridge-all",
                       "build-launcher", "build-launcher-all"):
            self.assertIn(target, names)
        self.assertNotIn("build-all", names)
        for target in ("export-artifacts", "import-artifacts"):
            self.assertIn(target, names)
        self.assertIn("ARCHIVE=path required", result.stdout)
        self.assertFalse(self.log.exists())


if __name__ == "__main__":
    unittest.main()
