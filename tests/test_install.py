import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class InstallTests(unittest.TestCase):
    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = tempfile.TemporaryDirectory(prefix="install-", dir=fixtures)
        self.addCleanup(directory.cleanup)
        self.directory = Path(directory.name)
        self.checkout = self.directory / "checkout with spaces"
        (self.checkout / "bin").mkdir(parents=True)
        (self.checkout / "sbx-kit").mkdir()
        self.installer = self.checkout / "bin/install"
        shutil.copy2(ROOT / "bin/install", self.installer)
        self.launcher = self.checkout / "bin/pissbox"
        self.launcher.write_text("#!/bin/sh\nexit 0\n")
        self.launcher.chmod(0o755)
        self.home = self.directory / "home with spaces"
        self.home.mkdir()
        self.target = self.home / ".local/bin/pissbox"
        self.tools = self.directory / "tools"
        self.tools.mkdir()
        self.log = self.directory / "calls.jsonl"
        # Use a closed PATH so missing-dependency tests cannot find real tools.
        for name in ("bash", "cat", "dirname", "readlink", "mkdir", "ln", "python3"):
            (self.tools / name).symlink_to(shutil.which(name))
        stub = '''#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys

name = Path(sys.argv[0]).name
call = [name, *sys.argv[1:]]
with open(os.environ["INSTALL_TEST_LOG"], "a") as log:
    log.write(json.dumps(call) + "\\n")
if " ".join(call).startswith(os.environ.get("INSTALL_TEST_FAIL", "never-fail")):
    print("stub failure", file=sys.stderr)
    sys.exit(37)
if call == ["go", "env", "GOVERSION"]:
    print(os.environ.get("INSTALL_TEST_GO", "go1.25.0"))
if call == ["sbx", "version"]:
    print("sbx version " + os.environ.get("INSTALL_TEST_SBX", "v0.39.0") + " test-hash")
'''
        for name in ("make", "go", "docker", "sbx", "jq"):
            script = self.tools / name
            script.write_text(stub)
            script.chmod(0o755)

    def launch(self, *args, installer=None, path=None, **overrides):
        env = {**os.environ, "HOME": str(self.home), "PATH": str(self.tools) if path is None else path,
               "INSTALL_TEST_LOG": str(self.log), "INSTALL_TEST_FAIL": "never-fail",
               "INSTALL_TEST_GO": "go1.25.0", "INSTALL_TEST_SBX": "v0.39.0",
               **overrides}
        result = subprocess.run(
            [str(installer or self.installer), *args], cwd=self.directory,
            env=env, text=True, capture_output=True, timeout=5,
        )
        calls = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return result, calls

    def test_checks_dependencies_before_building_and_linking(self):
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [
            ["go", "env", "GOVERSION"], ["sbx", "version"],
            ["docker", "info"], ["docker", "buildx", "version"],
            ["sbx", "kit", "validate", str(self.checkout / "sbx-kit")],
            ["make", "-C", str(self.checkout), "build"],
        ])
        self.assertEqual(self.target.readlink(), self.launcher)
        self.assertIn("add " + str(self.target.parent) + " to PATH", result.stderr)
        self.assertFalse((self.home / ".bashrc").exists())

    def test_installer_symlink_resolves_checkout_not_callers_directory(self):
        link = self.directory / "install-link"
        link.symlink_to("checkout with spaces/bin/install")
        result, calls = self.launch(installer=link)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[-1], ["make", "-C", str(self.checkout), "build"])
        self.assertEqual(self.target.readlink(), self.launcher)

    def test_matching_launcher_link_is_left_unchanged(self):
        self.target.parent.mkdir(parents=True)
        self.target.symlink_to(self.launcher)
        inode = self.target.lstat().st_ino
        result, calls = self.launch(path=str(self.target.parent) + os.pathsep + str(self.tools))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.target.lstat().st_ino, inode)
        self.assertEqual(calls[-1], ["make", "-C", str(self.checkout), "build"])
        self.assertNotIn("Warning:", result.stderr)

    def test_repoints_old_broken_and_directory_symlinks_without_touching_targets(self):
        old_file = self.directory / "old launcher"
        old_file.write_text("keep this")
        old_directory = self.directory / "old directory"
        old_directory.mkdir()
        self.target.parent.mkdir(parents=True)
        for old_target in (old_file, old_directory, self.directory / "missing launcher"):
            with self.subTest(old_target=old_target):
                self.target.symlink_to(old_target)
                result, _ = self.launch()
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.target.readlink(), self.launcher)
                self.assertEqual(old_file.read_text(), "keep this")
                self.assertEqual(list(old_directory.iterdir()), [])
                self.target.unlink()

    def test_refuses_regular_file_or_directory_before_any_build(self):
        self.target.parent.mkdir(parents=True)
        self.target.write_text("user-owned command")
        result, calls = self.launch()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Refusing to replace", result.stderr)
        self.assertEqual(calls, [])
        self.assertEqual(self.target.read_text(), "user-owned command")
        self.target.unlink()
        self.target.mkdir()
        result, calls = self.launch()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])
        self.assertTrue(self.target.is_dir())

    def test_missing_dependencies_fail_before_any_build(self):
        for name in ("make", "go", "docker", "sbx", "jq"):
            with self.subTest(name=name):
                tool = self.tools / name
                hidden = self.tools / (name + ".hidden")
                tool.rename(hidden)
                try:
                    result, calls = self.launch()
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("Required command not found: " + name, result.stderr)
                    self.assertEqual(calls, [])
                    self.assertFalse(self.target.is_symlink())
                finally:
                    hidden.rename(tool)

    def test_rejects_old_or_unrecognized_versions_before_building(self):
        for env, message in (
            ({"INSTALL_TEST_GO": "go1.24.9"}, "Go 1.25+"),
            ({"INSTALL_TEST_GO": "unrecognized"}, "Go 1.25+"),
            ({"INSTALL_TEST_SBX": "v0.38.9"}, "SBX v0.39.0+"),
            ({"INSTALL_TEST_SBX": "unrecognized"}, "SBX v0.39.0+"),
        ):
            with self.subTest(env=env):
                result, calls = self.launch(**env)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(message, result.stderr)
                self.assertFalse(any(call[0] == "make" for call in calls))
                self.assertFalse(self.target.is_symlink())

    def test_preflight_or_build_failure_preserves_existing_link_and_status(self):
        self.target.parent.mkdir(parents=True)
        old_target = self.directory / "previous-checkout/bin/pissbox"
        self.target.symlink_to(old_target)
        for command in ("docker info", "docker buildx version", "sbx kit validate", "make -C"):
            with self.subTest(command=command):
                self.log.write_text("")
                result, calls = self.launch(INSTALL_TEST_FAIL=command)
                self.assertEqual(result.returncode, 37, result.stderr)
                self.assertEqual(self.target.readlink(), old_target)
                self.assertTrue(" ".join(calls[-1]).startswith(command))

    def test_help_and_bad_arguments_do_not_check_dependencies_or_build(self):
        for args, status in ((("-h",), 0), (("--help",), 0), (("--unknown",), 1), (("extra",), 1)):
            with self.subTest(args=args):
                result, calls = self.launch(*args, HOME="")
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertEqual(calls, [])
                self.assertFalse(self.target.exists())
                if status == 0:
                    self.assertIn("Usage:", result.stdout)
                    self.assertIn("Optional bin/artifacts transfers require Python 3", result.stdout)


if __name__ == "__main__":
    unittest.main()
