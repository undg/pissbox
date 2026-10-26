import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class LauncherWrapperTests(unittest.TestCase):
    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = tempfile.TemporaryDirectory(prefix="launcher-wrapper-", dir=fixtures)
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.checkout = self.root / "checkout with spaces"
        self.launcher = self.checkout / "bin/pissbox"
        self.launcher.parent.mkdir(parents=True)
        shutil.copy2(ROOT / "bin/pissbox", self.launcher)
        self.tools = self.root / "tools"
        self.tools.mkdir()
        uname = self.tools / "uname"
        uname.write_text('#!/bin/sh\ncase "$1" in -s) echo "$WRAPPER_OS";; -m) echo "$WRAPPER_ARCH";; esac\n')
        uname.chmod(0o755)
        self.log = self.root / "called.json"
        self.env = {**os.environ, "PATH": str(self.tools) + os.pathsep + os.environ["PATH"],
                    "WRAPPER_OS": "Linux", "WRAPPER_ARCH": "aarch64", "WRAPPER_LOG": str(self.log),
                    "GOOS": "windows", "GOARCH": "386"}
        # Any accidental fallback/build must fail and leave a visible trace.
        for script in (self.checkout / "bin/pissbox-legacy", self.checkout / "bin/build-launcher"):
            script.write_text('#!/bin/sh\necho forbidden > "$WRAPPER_LOG"\nexit 99\n')
            script.chmod(0o755)

    def binary(self, os_name="linux", arch="arm64"):
        binary = self.checkout / f".cache/bin/pissbox-{os_name}-{arch}"
        binary.parent.mkdir(parents=True, exist_ok=True)
        binary.write_text(
            f'#!{shutil.which("python3")}\n'
            'import json, os, sys\n'
            'from pathlib import Path\n'
            'Path(os.environ["WRAPPER_LOG"]).write_text(json.dumps({"args": sys.argv[1:], '
            '"cwd": os.getcwd(), "pid": os.getpid(), "binary": sys.argv[0]}))\n'
            'print("candidate stderr", file=sys.stderr)\n'
            'sys.exit(37)\n'
        )
        binary.chmod(0o755)
        return binary

    def run_wrapper(self, *args, command=None):
        return subprocess.Popen([str(command or self.launcher), *args], cwd=self.root,
                                env=self.env, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def test_native_selection_preserves_arguments_cwd_pid_and_status(self):
        args = ["workspace with spaces", "--", "", "--name=literal;value", "*"]
        for system, machine, os_name, arch in (
            ("Linux", "aarch64", "linux", "arm64"),
            ("Linux", "x86_64", "linux", "amd64"),
            ("Darwin", "arm64", "darwin", "arm64"),
            ("Darwin", "x86_64", "darwin", "amd64"),
        ):
            with self.subTest(system=system, machine=machine):
                binary = self.binary(os_name, arch)
                self.env.update(WRAPPER_OS=system, WRAPPER_ARCH=machine)
                process = self.run_wrapper(*args)
                stdout, stderr = process.communicate(timeout=5)
                self.assertEqual(process.returncode, 37, stderr)
                self.assertEqual(stdout, "")
                self.assertEqual(stderr, "candidate stderr\n")
                self.assertEqual(json.loads(self.log.read_text()), {
                    "args": args, "cwd": str(self.root), "pid": process.pid, "binary": str(binary),
                })

    def test_relative_and_absolute_symlink_chain_and_path_launch(self):
        binary = self.binary()
        links = self.root / "installed commands"
        links.mkdir()
        relative = self.root / "relative launcher"
        relative.symlink_to("checkout with spaces/bin/pissbox")
        installed = links / "pissbox"
        installed.symlink_to(relative)
        self.env["PATH"] = str(links) + os.pathsep + self.env["PATH"]
        process = self.run_wrapper("--help", command="pissbox")
        _, stderr = process.communicate(timeout=5)
        self.assertEqual(process.returncode, 37, stderr)
        observed = json.loads(self.log.read_text())
        self.assertEqual(observed["binary"], str(binary))
        self.assertEqual(observed["cwd"], str(self.root))
        self.assertEqual(observed["args"], ["--help"])

    def test_missing_nonexecutable_or_directory_binary_reports_build_without_fallback(self):
        binary = self.checkout / ".cache/bin/pissbox-linux-arm64"
        for kind in ("missing", "nonexecutable", "directory"):
            with self.subTest(kind=kind):
                if kind == "nonexecutable":
                    binary.parent.mkdir(parents=True)
                    binary.write_text("not executable")
                elif kind == "directory":
                    binary.unlink()
                    binary.mkdir()
                process = self.run_wrapper("--help")
                _, stderr = process.communicate(timeout=5)
                self.assertEqual(process.returncode, 1, stderr)
                self.assertIn(str(binary), stderr)
                self.assertIn(str(self.checkout / "bin/build-launcher"), stderr)
                self.assertFalse(self.log.exists())

    def test_unsupported_platform_fails_without_running_any_binary_or_fallback(self):
        self.binary()
        for system, machine, diagnostic in (
            ("FreeBSD", "aarch64", "operating system"),
            ("Linux", "riscv64", "architecture"),
        ):
            with self.subTest(system=system, machine=machine):
                self.env.update(WRAPPER_OS=system, WRAPPER_ARCH=machine)
                process = self.run_wrapper()
                _, stderr = process.communicate(timeout=5)
                self.assertEqual(process.returncode, 1, stderr)
                self.assertIn("Unsupported launcher " + diagnostic, stderr)
                self.assertFalse(self.log.exists())


if __name__ == "__main__":
    unittest.main()
