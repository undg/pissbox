import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "bin" / "update-colgrep"
FIXTURES = ROOT / "tests" / "fixtures" / "colgrep"
REPOSITORY = "https://github.com/lightonai/next-plaid"


class UpdateColgrepTests(unittest.TestCase):
    def setUp(self):
        fixtures = ROOT / "shared-state" / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        self.directory = Path(tempfile.mkdtemp(prefix="colgrep-", dir=fixtures))
        self.checkout = self.directory / "checkout with spaces"
        self.script = self.checkout / "bin" / "update-colgrep"
        self.script.parent.mkdir(parents=True)
        shutil.copy2(SCRIPT, self.script)
        self.assets = self.directory / "release assets"
        self.assets.mkdir()
        self.log = self.directory / "curl.jsonl"
        self.destination = self.checkout / "sbx-kit/files/usr/local/bin/colgrep"
        self.caller = self.directory / "caller"
        self.caller.mkdir()
        self.make_release()

    def make_release(self, *, payload=b"colgrep version one", member_kind=None):
        package = "colgrep-aarch64-unknown-linux-gnu"
        archive = self.assets / (package + ".tar.xz")
        with tarfile.open(archive, "w:xz") as release:
            for name, data in (("LICENSE", b"license"), ("colgrep", payload)):
                if name == "colgrep" and member_kind == "missing":
                    continue
                member = tarfile.TarInfo(package + "/" + name)
                if name == "colgrep" and member_kind == "symlink":
                    member.type = tarfile.SYMTYPE
                    member.linkname = "LICENSE"
                    release.addfile(member)
                else:
                    member.size = len(data)
                    release.addfile(member, io.BytesIO(data))
        checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
        archive.with_suffix(".xz.sha256").write_text(f"{checksum} *{archive.name}\n")

    def update(self, *args, script=None, **overrides):
        env = {
            key: value for key, value in os.environ.items()
            if not key.startswith("TEST_COLGREP_")
        }
        env.pop("BASH_ENV", None)
        env.update(
            PATH=str(FIXTURES) + os.pathsep + env["PATH"],
            TEST_COLGREP_ASSETS=str(self.assets),
            TEST_COLGREP_LOG=str(self.log),
        )
        env.update(overrides)
        return subprocess.run(
            [str(script or self.script), *args], cwd=self.caller,
            env=env, text=True, capture_output=True, timeout=10,
        )

    def tool_path(self, *checksums):
        directory = self.directory / ("tools-" + "-".join(checksums))
        directory.mkdir()
        for tool in ("bash", "dirname", "readlink", "mkdir", "tar", "install", "python3", *checksums):
            (directory / tool).symlink_to(shutil.which(tool))
        for tool in ("curl", "uname"):
            (directory / tool).symlink_to(FIXTURES / tool)
        (directory / "xz").symlink_to(shutil.which("xz") or FIXTURES / "xz")
        return str(directory)

    def calls(self):
        if not self.log.exists():
            return []
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def seed_binary(self):
        self.destination.parent.mkdir(parents=True, exist_ok=True)
        self.destination.write_bytes(b"keep existing binary")
        self.destination.chmod(0o755)

    def test_downloads_fixed_sandbox_target_without_host_detection(self):
        result = self.update()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.destination.read_bytes(), b"colgrep version one")
        self.assertEqual(stat.S_IMODE(self.destination.stat().st_mode), 0o755)
        self.assertIn("v1.0.0 (aarch64 Linux)", result.stdout)
        self.assertEqual([call[-1] for call in self.calls()], [
            REPOSITORY + "/releases/latest",
            REPOSITORY + "/releases/download/v1.0.0/colgrep-aarch64-unknown-linux-gnu.tar.xz",
            REPOSITORY + "/releases/download/v1.0.0/colgrep-aarch64-unknown-linux-gnu.tar.xz.sha256",
        ])
        cache = self.checkout / "shared-state/downloads/colgrep/v1.0.0/aarch64"
        self.assertTrue((cache / "colgrep-aarch64-unknown-linux-gnu.tar.xz").is_file())
        self.assertFalse((cache / "colgrep-aarch64-unknown-linux-gnu").exists())
        self.assertEqual(list(self.caller.iterdir()), [])

    def test_supported_checksum_tools(self):
        for tool in ("sha256sum", "shasum"):
            if shutil.which(tool) is None:
                continue
            with self.subTest(tool=tool):
                result = self.update(PATH=self.tool_path(tool))
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.destination.read_bytes(), b"colgrep version one")

    def test_missing_checksum_tool_fails_before_downloading(self):
        self.seed_binary()
        result = self.update(PATH=self.tool_path())
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Requires sha256sum", result.stderr)
        self.assertEqual(self.calls(), [])
        self.assertEqual(self.destination.read_bytes(), b"keep existing binary")

    def test_rerun_updates_binary_from_new_release(self):
        result = self.update()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.make_release(payload=b"colgrep version two")
        result = self.update(TEST_COLGREP_VERSION="v2.0.0")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.destination.read_bytes(), b"colgrep version two")
        self.assertIn("v2.0.0", result.stdout)
        self.assertIn("/releases/download/v2.0.0/", self.calls()[-1][-1])

    def test_checksum_failure_keeps_existing_binary(self):
        self.seed_binary()
        checksum = self.assets / "colgrep-aarch64-unknown-linux-gnu.tar.xz.sha256"
        checksum.write_text("0" * 64 + " *colgrep-aarch64-unknown-linux-gnu.tar.xz\n")
        result = self.update()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("FAILED", result.stdout)
        self.assertEqual(self.destination.read_bytes(), b"keep existing binary")

    def test_empty_binary_keeps_existing_binary(self):
        self.seed_binary()
        self.make_release(payload=b"")
        result = self.update()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("no binary data", result.stderr)
        self.assertEqual(self.destination.read_bytes(), b"keep existing binary")

    def test_download_failures_keep_existing_binary(self):
        self.seed_binary()
        for suffix in ("/latest", ".tar.xz", ".sha256"):
            with self.subTest(suffix=suffix):
                result = self.update(TEST_COLGREP_FAIL_SUFFIX=suffix)
                self.assertEqual(result.returncode, 22, result.stderr)
                self.assertIn("fixture curl failure", result.stderr)
                self.assertEqual(self.destination.read_bytes(), b"keep existing binary")

    def test_invalid_archive_members_keep_existing_binary(self):
        self.seed_binary()
        for kind in ("missing", "symlink"):
            with self.subTest(kind=kind):
                self.make_release(member_kind=kind)
                result = self.update()
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.destination.read_bytes(), b"keep existing binary")

    def test_invalid_release_redirect_keeps_existing_binary(self):
        self.seed_binary()
        for url in (
            "https://example.com/releases/tag/v1.0.0",
            REPOSITORY + "/releases/latest",
            REPOSITORY + "/releases/tag/..",
        ):
            with self.subTest(url=url):
                result = self.update(TEST_COLGREP_RELEASE_URL=url)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.destination.read_bytes(), b"keep existing binary")

    def test_invalid_arguments_do_not_download(self):
        for args in (
            ("--arch", "amd64"), ("--arch=aarch64",),
            ("--unknown",), ("unexpected",), ("",),
        ):
            with self.subTest(args=args):
                result = self.update(*args)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.calls(), [])
                self.assertFalse(self.destination.exists())

    def test_help_does_not_download_or_create_directories(self):
        for args in (("-h",), ("--help",)):
            with self.subTest(args=args):
                result = self.update(*args)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("Usage:", result.stdout)
                self.assertEqual(self.calls(), [])
                self.assertFalse((self.checkout / "shared-state").exists())
                self.assertFalse(self.destination.exists())

    def test_symlink_finds_checkout_without_changing_caller_directory(self):
        link = self.caller / "updater"
        link.symlink_to(Path("..") / "checkout with spaces/bin/update-colgrep")
        result = self.update(script=link)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.destination.exists())
        self.assertEqual(list(self.caller.iterdir()), [link])


if __name__ == "__main__":
    unittest.main()
