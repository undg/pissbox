import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
BUILD = ROOT / "bin/build-quality-tools"


class BuildQualityToolsTests(unittest.TestCase):
    def launch(self, *args, launcher=BUILD, cwd=None, **overrides):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        if cwd is None:
            cwd = Path(tempfile.mkdtemp(prefix="build-quality-tools-", dir=fixtures))
        env = {
            key: value for key, value in os.environ.items()
            if not key.startswith("TEST_GO_") and key not in (
                "GOPATH", "GOMODCACHE", "PISSBOX_STATE_DIR", "XDG_CACHE_HOME",
            )
        }
        env.update(PATH=str(ROOT / "tests/fixtures") + os.pathsep + env["PATH"])
        env["PISSBOX_STATE_DIR"] = str(cwd / "shared")
        env["XDG_CACHE_HOME"] = str(cwd / "xdg")
        env.update(overrides)
        result = subprocess.run(
            [str(launcher), *args], cwd=cwd, env=env, capture_output=True, text=True, timeout=10,
        )
        calls = [json.loads(line[3:]) for line in result.stdout.splitlines() if line.startswith("go:")]
        return result, calls, cwd

    def test_native_build_writes_shared_binaries_and_reuses_them(self):
        result, calls, cwd = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([(call["os"], call["arch"]) for call in calls], [("linux", "arm64"), ("linux", "arm64")])
        self.assertEqual([call["args"][-1].split("/")[-1].split("@")[0] for call in calls],
                         ["gremlins", "go-crap"])
        self.assertTrue(all(call["cgo"] == "0" for call in calls))
        self.assertTrue(all(call["gomodcache"] == "/tmp/quality-tools-modcache" for call in calls))
        shared = cwd / "shared"
        for name in ("gremlins", "go-crap"):
            self.assertTrue((shared / f"bin/{name}-linux-arm64").is_file())
            self.assertTrue((shared / f"quality-tools/{name}-linux-arm64.version").is_file())
        second, second_calls, _ = self.launch(cwd=cwd)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(second_calls, [])

    def test_default_state_uses_xdg_cache(self):
        result, calls, cwd = self.launch(PISSBOX_STATE_DIR="")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 2)
        self.assertTrue((cwd / "xdg/pissbox/bin/gremlins-linux-arm64").is_file())
        self.assertTrue((cwd / "xdg/pissbox/quality-tools/go-crap-linux-arm64.version").is_file())

    def test_all_builds_both_tools_for_every_os_and_architecture(self):
        result, calls, cwd = self.launch("--all")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            [(call["os"], call["arch"]) for call in calls],
            [("darwin", "arm64"), ("darwin", "arm64"),
             ("darwin", "amd64"), ("darwin", "amd64"),
             ("linux", "arm64"), ("linux", "arm64"),
             ("linux", "amd64"), ("linux", "amd64")],
        )
        for goos in ("darwin", "linux"):
            for goarch in ("arm64", "amd64"):
                for name in ("gremlins", "go-crap"):
                    self.assertTrue((cwd / f"shared/bin/{name}-{goos}-{goarch}").is_file())

    def test_version_change_rebuilds_only_that_tool(self):
        result, calls, cwd = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        changed = cwd / "bin/build-quality-tools"
        changed.parent.mkdir()
        changed.write_text(BUILD.read_text().replace("gremlins@v0.6.0", "gremlins@v0.6.1"))
        changed.chmod(0o755)
        result, calls, _ = self.launch(launcher=changed, cwd=cwd)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertIn("gremlins@v0.6.1", calls[0]["args"][-1])

    def test_help_and_invalid_arguments_do_not_build(self):
        for args, status in ((["-h"], 0), (["--all", "--help"], 0), (["--unknown"], 1), (["workspace"], 1)):
            with self.subTest(args=args):
                result, calls, cwd = self.launch(*args)
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertEqual(calls, [])
                self.assertFalse((cwd / "shared").exists())

    def test_build_failure_stops_remaining_targets(self):
        result, calls, _ = self.launch("--all", TEST_GO_STATUS="9")
        self.assertEqual(result.returncode, 9, result.stderr)
        self.assertEqual(len(calls), 1)


if __name__ == "__main__":
    unittest.main()
