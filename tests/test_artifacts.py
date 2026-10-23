import io
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class ArtifactTests(unittest.TestCase):
    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        self.directory = Path(tempfile.mkdtemp(prefix="artifacts-", dir=fixtures))
        self.addCleanup(shutil.rmtree, self.directory)
        self.source = self.checkout("source checkout")
        self.target = self.checkout("target checkout")
        self.package = self.directory / "runtime artifacts.tar.gz"

    def checkout(self, name):
        root = self.directory / name
        (root / "bin").mkdir(parents=True)
        shutil.copy2(ROOT / "bin/artifacts", root / "bin/artifacts")
        return root

    def run_script(self, root, *args):
        return subprocess.run(
            [str(root / "bin/artifacts"), *map(str, args)],
            cwd=self.directory, capture_output=True, text=True, timeout=10,
        )

    def write(self, root, name, data=b"artifact"):
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        return path

    def archive(self, entries):
        with tarfile.open(self.package, "w:gz") as archive:
            for name, kind, link in entries:
                member = tarfile.TarInfo(name)
                member.type = kind
                member.linkname = link
                member.size = 0
                archive.addfile(member, io.BytesIO())

    def test_round_trip_preserves_files_modes_links_and_excludes_private_state(self):
        binary = self.write(self.source, ".cache/bin/go-bridge-linux-amd64", b"binary")
        binary.chmod(0o755)
        self.write(self.source, ".cache/images/pissbox-local-amd64.tar", b"image")
        image_link = self.source / ".cache/images/pissbox-local.tar"
        image_link.symlink_to("pissbox-local-amd64.tar")
        self.write(self.source, "pi-agent/npm/package.json", b"{}")
        self.write(self.source, "pi-agent/npm/package-lock.json", b"{}")
        dependency = self.write(self.source, "pi-agent/npm/node_modules/tool/cli", b"tool")
        bin_dir = dependency.parent.parent / ".bin"
        bin_dir.mkdir()
        (bin_dir / "tool").symlink_to("../tool/cli")
        os.link(dependency, dependency.with_name("duplicate"))
        excluded_paths = ("pi-agent/auth.json", "pi-agent/sessions/session.jsonl",
                          "pi-agent/settings.json", "pi-agent/skills/private/SKILL.md",
                          ".cache/npm/token", ".cache/bin/unrelated", ".cache/images/other.tar")
        for excluded in excluded_paths:
            self.write(self.source, excluded, b"must not transfer")

        result = self.run_script(self.source, f"--export={self.package}")
        self.assertEqual(result.returncode, 0, result.stderr)
        result = self.run_script(self.target, "--import", self.package)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.target / ".cache/bin/go-bridge-linux-amd64").read_bytes(), b"binary")
        self.assertEqual((self.target / ".cache/bin/go-bridge-linux-amd64").stat().st_mode & 0o777, 0o755)
        self.assertEqual(os.readlink(self.target / image_link.relative_to(self.source)),
                         "pissbox-local-amd64.tar")
        self.assertEqual((self.target / "pi-agent/npm/node_modules/.bin/tool").read_bytes(), b"tool")
        self.assertEqual((self.target / dependency.relative_to(self.source)).stat().st_ino,
                         (self.target / dependency.relative_to(self.source)).with_name("duplicate").stat().st_ino)
        self.assertFalse((self.target / "pi-agent/auth.json").exists())
        with tarfile.open(self.package) as archive:
            for excluded in excluded_paths:
                self.assertNotIn(excluded, archive.getnames())
                self.assertFalse((self.target / excluded).exists())

    def test_short_flags_and_import_equals(self):
        for export_flag, import_args in (("-e", ["-i", str(self.package)]),
                                         ("--export", [f"--import={self.package}"])):
            with self.subTest(export_flag=export_flag):
                self.write(self.source, "pi-agent/npm/package.json")
                result = self.run_script(self.source, export_flag, self.package)
                self.assertEqual(result.returncode, 0, result.stderr)
                result = self.run_script(self.target, *import_args)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.package.unlink()
                shutil.rmtree(self.target / "pi-agent")

    def test_refuses_existing_output_and_preflights_import_conflicts(self):
        self.write(self.source, ".cache/bin/go-bridge-linux-amd64")
        self.write(self.source, "pi-agent/npm/package.json")
        self.assertEqual(self.run_script(self.source, "-e", self.package).returncode, 0)
        original = self.package.read_bytes()
        self.assertNotEqual(self.run_script(self.source, "-e", self.package).returncode, 0)
        self.assertEqual(self.package.read_bytes(), original)
        existing = self.write(self.target, "pi-agent/npm/package.json", b"keep me")
        result = self.run_script(self.target, "-i", self.package)
        self.assertIn("Refusing to overwrite", result.stderr)
        self.assertEqual(existing.read_bytes(), b"keep me")
        self.assertFalse((self.target / ".cache").exists())

    def test_rejects_unsafe_archives_before_writing(self):
        cases = [
            [("../escape", tarfile.REGTYPE, "")],
            [("/absolute", tarfile.REGTYPE, "")],
            [("pi-agent/auth.json", tarfile.REGTYPE, "")],
            [("pi-agent/npm/node_modules/link", tarfile.SYMTYPE, "../../../../escape")],
            [("pi-agent/npm/node_modules/link", tarfile.SYMTYPE, "/absolute")],
            [("pi-agent/npm/node_modules/pipe", tarfile.FIFOTYPE, "")],
            [("pi-agent/npm/package.json", tarfile.REGTYPE, "")] * 2,
            [("pi-agent/npm/node_modules/link", tarfile.LNKTYPE, "pi-agent/auth.json")],
            [("pi-agent/npm/node_modules/link", tarfile.SYMTYPE, "target"),
             ("pi-agent/npm/node_modules/link/file", tarfile.REGTYPE, "")],
            [],
        ]
        for entries in cases:
            with self.subTest(entries=entries):
                self.archive(entries)
                result = self.run_script(self.target, "-i", self.package)
                self.assertNotEqual(result.returncode, 0, result.stderr)
                self.assertFalse((self.target / "pi-agent").exists())

    def test_refuses_symlinked_destination_parents_and_existing_broken_links(self):
        self.archive([("pi-agent/npm/package.json", tarfile.REGTYPE, "")])
        (self.target / "pi-agent").symlink_to(self.source, target_is_directory=True)
        result = self.run_script(self.target, "-i", self.package)
        self.assertIn("not a real directory", result.stderr)
        (self.target / "pi-agent").unlink()
        (self.target / "pi-agent/npm").mkdir(parents=True)
        (self.target / "pi-agent/npm/package.json").symlink_to("missing")
        result = self.run_script(self.target, "-i", self.package)
        self.assertIn("Refusing to overwrite", result.stderr)

    def test_empty_source_bad_arguments_and_help(self):
        result = self.run_script(self.source, "-e", self.package)
        self.assertIn("No artifacts", result.stderr)
        self.assertFalse(self.package.exists())
        for args in ([], ["--export"], ["--import"], ["--unknown"],
                     ["-e", str(self.package), "-i", str(self.package)]):
            with self.subTest(args=args):
                self.assertNotEqual(self.run_script(self.source, *args).returncode, 0)
        for flag in ("-h", "--help"):
            result = self.run_script(self.source, flag)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("--export", result.stdout)

    def test_export_rejects_external_links_and_package_inside_artifacts(self):
        path = self.write(self.source, "pi-agent/npm/node_modules/tool")
        path.unlink()
        path.symlink_to("/outside")
        result = self.run_script(self.source, "-e", self.package)
        self.assertIn("Absolute symlink", result.stderr)
        self.assertFalse(self.package.exists())
        path.unlink()
        result = self.run_script(self.source, "-e", path.with_name("package.tar.gz"))
        self.assertIn("outside the artifact paths", result.stderr)


if __name__ == "__main__":
    unittest.main()
