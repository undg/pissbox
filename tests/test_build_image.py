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
        self.images = self.root / ".cache/images"
        self.link = self.images / "pissbox-local.tar"
        stub = """#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys

name = Path(sys.argv[0]).name
call = [name, *sys.argv[1:]]
with open(os.environ["BUILD_TEST_LOG"], "a") as log:
    log.write(json.dumps(call) + "\\n")
failure = os.environ.get("BUILD_TEST_FAIL", "")
if failure and " ".join(call).startswith(failure):
    sys.exit(37)
if name == "docker" and sys.argv[1:3] == ["info", "--format"]:
    print(os.environ.get("BUILD_TEST_ARCH", "x86_64"))
if name == "docker" and sys.argv[1:3] == ["image", "save"]:
    archive = Path(sys.argv[-1])
    archive.parent.mkdir(parents=True, exist_ok=True)
    archive.write_bytes(b"stub archive\\n")
"""
        for name in ("docker", "sbx"):
            tool = self.bin / name
            tool.write_text(stub)
            tool.chmod(0o755)
        uname = self.bin / "uname"
        uname.write_text('#!/bin/sh\necho "${BUILD_TEST_UNAME:-Linux}"\n')
        uname.chmod(0o755)

    def launch(self, *args, script=None, fail="", arch="x86_64", env=None):
        result = subprocess.run(
            [str(script or self.script), *args], cwd=self.directory, text=True,
            capture_output=True, timeout=5,
            env={**os.environ, "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
                 "BUILD_TEST_LOG": str(self.log), "BUILD_TEST_FAIL": fail,
                 "BUILD_TEST_ARCH": arch, **(env or {})},
        )
        calls = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return result, calls

    def archive(self, arch):
        return self.images / f"pissbox-local-{arch}.tar"

    def build_calls(self, arch, *build_args):
        build = ["docker", "buildx", "build", "--platform", f"linux/{arch}",
                 "--load", "-t", "pissbox:local"]
        ca = self.root / "sbx-kit/ca.crt"
        if ca.exists():
            build += ["--secret", f"id=custom_ca,src={ca}"]
        build += [*build_args, str(self.root / "sbx-kit")]
        return [build, ["docker", "image", "save", "pissbox:local",
                        "-o", str(self.archive(arch))]]

    def register_calls(self, arch):
        return [["sbx", "template", "load", str(self.archive(arch))]]

    def detected_calls(self):
        return [["docker", "info", "--format", "{{.Architecture}}"]]

    def test_default_run_builds_and_registers_the_host_architecture(self):
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"], *self.detected_calls(),
                                 *self.build_calls("amd64"), *self.register_calls("amd64")])
        self.assertTrue(self.link.is_symlink())
        self.assertEqual(os.readlink(self.link), "pissbox-local-amd64.tar")

    def test_default_run_follows_an_arm_daemon(self):
        result, calls = self.launch(arch="aarch64")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[-1], ["sbx", "template", "load",
                                     str(self.archive("arm64"))])
        self.assertEqual(os.readlink(self.link), "pissbox-local-arm64.tar")

    def test_explicit_architecture_builds_without_registering(self):
        cases = (("--amd64", "x86_64", "amd64"), ("--arm64", "aarch64", "arm64"))
        for flag, daemon, arch in cases:
            with self.subTest(flag=flag):
                self.log.unlink(missing_ok=True)
                result, calls = self.launch(flag, arch=daemon)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls, [["docker", "buildx", "version"],
                                         *self.detected_calls(),
                                         *self.build_calls(arch)])
                self.assertNotIn("emulation", result.stderr)

    def test_cross_architecture_build_registers_emulation_first(self):
        result, calls = self.launch("--arm64", arch="x86_64")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("registering arm64 binfmt handlers", result.stderr)
        self.assertIn("Building pissbox:local for linux/arm64", result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"],
                                 *self.detected_calls(),
                                 ["docker", "run", "--privileged", "--rm",
                                  "tonistiigi/binfmt", "--install", "arm64"],
                                 *self.build_calls("arm64")])

    def test_emulation_registration_failure_stops_the_build(self):
        result, calls = self.launch("--arm64", arch="x86_64", fail="docker run")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("could not register arm64 emulation", result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"],
                                 *self.detected_calls(),
                                 ["docker", "run", "--privileged", "--rm",
                                  "tonistiigi/binfmt", "--install", "arm64"]])

    def test_both_architectures_build_in_fixed_order(self):
        result, calls = self.launch("--arm64", "--amd64")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"],
                                 *self.detected_calls(),
                                 *self.build_calls("amd64"),
                                 ["docker", "run", "--privileged", "--rm",
                                  "tonistiigi/binfmt", "--install", "arm64"],
                                 *self.build_calls("arm64")])
        self.assertEqual(os.readlink(self.link), "pissbox-local-arm64.tar")

    def test_register_alone_uses_the_host_architecture(self):
        self.images.mkdir(parents=True)
        self.archive("amd64").write_bytes(b"stub archive\n")
        result, calls = self.launch("--register")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"], *self.detected_calls(),
                                 *self.register_calls("amd64")])

    def test_register_with_architecture_skips_detection(self):
        self.images.mkdir(parents=True)
        self.archive("arm64").write_bytes(b"stub archive\n")
        result, calls = self.launch("--register", "arm64")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"],
                                 *self.register_calls("arm64")])
        self.assertEqual(os.readlink(self.link), "pissbox-local-arm64.tar")

    def test_register_still_builds_when_architecture_flags_are_given(self):
        result, calls = self.launch("--amd64", "--register")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"],
                                 *self.detected_calls(),
                                 *self.build_calls("amd64"),
                                 *self.register_calls("amd64")])

    def test_register_without_an_archive_fails_before_sbx(self):
        result, calls = self.launch("--register", "arm64")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("archive not found", result.stderr)
        self.assertIn("pissbox-local-arm64.tar", result.stderr)
        self.assertIn("--arm64", result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"]])

    def test_register_rejects_unknown_architecture(self):
        result, calls = self.launch("--register", "riscv64")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("Unknown architecture for --register: riscv64", result.stderr)
        self.assertEqual(calls, [])

    def test_build_options_are_forwarded_to_every_build(self):
        args = ("--build-arg", "PI_VERSION=1.2.3", "--label", "description=two words")
        result, calls = self.launch("--amd64", "--arm64", "--", *args)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"],
                                 *self.detected_calls(),
                                 *self.build_calls("amd64", *args),
                                 ["docker", "run", "--privileged", "--rm",
                                  "tonistiigi/binfmt", "--install", "arm64"],
                                 *self.build_calls("arm64", *args)])

    def test_detection_maps_daemon_architectures(self):
        for reported, arch in (("x86_64", "amd64"), ("amd64", "amd64"),
                               ("aarch64", "arm64"), ("arm64", "arm64")):
            with self.subTest(reported=reported):
                self.log.unlink(missing_ok=True)
                result, calls = self.launch(arch=reported)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(["docker", "buildx", "build", "--platform",
                               f"linux/{arch}", "--load", "-t", "pissbox:local",
                               str(self.root / "sbx-kit")], calls)

    def test_unknown_daemon_architecture_fails(self):
        result, calls = self.launch(arch="riscv64")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("unsupported Docker architecture: riscv64", result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"], *self.detected_calls()])

    def test_archive_symlink_replaces_a_regular_file(self):
        self.images.mkdir(parents=True)
        self.link.write_text("old archive\n")
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.link.is_symlink())
        self.assertEqual(os.readlink(self.link), "pissbox-local-amd64.tar")

    def test_optional_ca_is_forwarded_with_build_arguments(self):
        ca = self.root / "sbx-kit/ca.crt"
        ca.write_text("public CA fixture; Docker is stubbed\n")
        result, calls = self.launch("--", "--no-cache")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"], *self.detected_calls(),
                                 *self.build_calls("amd64", "--no-cache"),
                                 *self.register_calls("amd64")])
        self.assertNotIn("Info:", result.stderr)

    def test_optional_ca_through_symlink(self):
        ca = self.root / "sbx-kit/ca.crt"
        ca.write_text("public CA fixture; Docker is stubbed\n")
        link = self.directory / "build-image-with-ca"
        link.symlink_to(os.path.relpath(self.script, link.parent))
        result, calls = self.launch("--arm64", script=link, arch="aarch64")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"],
                                 *self.detected_calls(),
                                 *self.build_calls("arm64")])

    def test_symlink_launch_and_build_arguments(self):
        link = self.directory / "build-image"
        link.symlink_to(os.path.relpath(self.script, link.parent))
        result, calls = self.launch("--amd64", "--", "--progress=plain", script=link)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"],
                                 *self.detected_calls(),
                                 *self.build_calls("amd64", "--progress=plain")])

    def test_empty_build_options_after_separator(self):
        result, calls = self.launch("--")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"], *self.detected_calls(),
                                 *self.build_calls("amd64"), *self.register_calls("amd64")])

    def test_ca_in_callers_directory_is_not_used(self):
        (self.directory / "ca.crt").write_text("not the checkout CA\n")
        result, calls = self.launch("--arm64", arch="aarch64")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"],
                                 *self.detected_calls(),
                                 *self.build_calls("arm64")])
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
                for option in ("--amd64", "--arm64", "--register"):
                    self.assertIn(option, result.stdout)
                self.assertEqual(calls, [])

    def test_missing_buildx_fails_before_building(self):
        cases = (
            ("Darwin", "brew install docker-buildx", "pacman"),
            ("Linux", "pacman -S docker-buildx", "brew install"),
        )
        for system, hint, absent in cases:
            with self.subTest(system=system):
                self.log.unlink(missing_ok=True)
                result, calls = self.launch(
                    fail="docker buildx version", env={"BUILD_TEST_UNAME": system})
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

    def test_docker_info_failure_is_reported(self):
        result, calls = self.launch(fail="docker info")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("cannot read the Docker daemon architecture", result.stderr)
        self.assertEqual(calls, [["docker", "buildx", "version"],
                                 *self.detected_calls()])

    def test_failures_stop_the_pipeline_and_preserve_exit_status(self):
        steps = (
            ("docker buildx build", 3),
            ("docker image save", 4),
            ("sbx template load", 5),
        )
        for step, count in steps:
            with self.subTest(step=step):
                self.log.unlink(missing_ok=True)
                result, calls = self.launch(fail=step)
                self.assertEqual(result.returncode, 37, result.stderr)
                expected = [["docker", "buildx", "version"], *self.detected_calls(),
                            *self.build_calls("amd64"), *self.register_calls("amd64")]
                self.assertEqual(calls, expected[:count])


if __name__ == "__main__":
    unittest.main()
