import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
BUILD = ROOT / "bin/build-launcher"


class BuildLauncherTests(unittest.TestCase):
    def launch(self, *args, launcher=BUILD, **overrides):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        cwd = Path(tempfile.mkdtemp(prefix="build-launcher-", dir=fixtures))
        env = {key: value for key, value in os.environ.items() if not key.startswith("TEST_GO_")}
        env.update(PATH=str(ROOT / "tests/fixtures") + os.pathsep + env["PATH"])
        env.update(overrides)
        result = subprocess.run([str(launcher), *args], cwd=cwd, env=env, capture_output=True, text=True, timeout=5)
        calls = [json.loads(line[3:]) for line in result.stdout.splitlines() if line.startswith("go:")]
        return result, calls

    def test_native_build_ignores_target_environment_overrides(self):
        result, calls = self.launch(GOOS="windows", GOARCH="386", CGO_ENABLED="1")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertEqual((calls[0]["os"], calls[0]["arch"], calls[0]["cgo"]), ("linux", "arm64", "0"))
        self.assertEqual(calls[0]["cwd"], str(ROOT / "host"))
        self.assertEqual(calls[0]["args"], ["build", "-trimpath", "-o", str(ROOT / ".cache/bin/pissbox-linux-arm64"), "./cmd/pissbox"])

    def test_all_builds_four_targets_without_cgo(self):
        result, calls = self.launch("--all")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([(call["os"], call["arch"]) for call in calls], [
            ("darwin", "arm64"), ("darwin", "amd64"), ("linux", "arm64"), ("linux", "amd64"),
        ])
        self.assertTrue(all(call["cgo"] == "0" for call in calls))

    def test_symlink_resolves_checkout_resources(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = Path(tempfile.mkdtemp(prefix="build launcher links-", dir=fixtures))
        link = directory / "build launcher"
        link.symlink_to(BUILD)
        result, calls = self.launch(launcher=link)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[0]["cwd"], str(ROOT / "host"))

    def test_help_and_invalid_arguments_do_not_build(self):
        for args, status in ((["-h"], 0), (["--all", "--help"], 0), (["--unknown"], 1), (["workspace"], 1)):
            with self.subTest(args=args):
                result, calls = self.launch(*args)
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertEqual(calls, [])

    def test_build_failure_stops_remaining_targets(self):
        result, calls = self.launch("--all", TEST_GO_STATUS="9")
        self.assertEqual(result.returncode, 9, result.stderr)
        self.assertEqual(len(calls), 1)


if __name__ == "__main__":
    unittest.main()
