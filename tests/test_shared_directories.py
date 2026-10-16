import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
PISSBOX_FILES = ROOT / "sbx-kit" / "files" / "home" / ".pissbox"
SCRIPT = PISSBOX_FILES / "scripts" / "link-shared-directories.sh"
CONFIG = PISSBOX_FILES / "shared-directories.conf"


class SharedDirectoriesTests(unittest.TestCase):
    def setUp(self):
        # Retain generated fixtures inside the ignored sandbox-only state tree.
        fixtures = ROOT / ".cache" / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        self.directory = Path(tempfile.mkdtemp(prefix="shared-", dir=fixtures))
        self.home = self.directory / "sandbox home"
        self.state = self.directory / "shared state"
        self.home.mkdir()
        self.state.mkdir()

    def setup_links(self, *, home=None, state=None, script=SCRIPT):
        env = dict(os.environ, HOME=str(home or self.home))
        env["PISSBOX_STATE_DIR"] = str(self.state if state is None else state)
        return subprocess.run(
            ["bash", str(script)], env=env, text=True,
            capture_output=True, timeout=5,
        )

    def mappings(self):
        return [
            line.split("|")
            for line in CONFIG.read_text().splitlines()
            if line and not line.startswith("#")
        ]

    def test_links_all_configured_directories(self):
        result = self.setup_links()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stderr, "")
        for source, target in self.mappings():
            with self.subTest(target=target):
                link = self.home / target
                self.assertTrue(link.is_symlink())
                self.assertEqual(link.readlink(), self.state / source)
                self.assertTrue(link.is_dir())
        self.assertEqual(
            (self.home / ".local/share/pnpm/store").resolve(),
            self.state / "pnpm/store",
        )
        # Share model files, not the Hugging Face token file.
        self.assertFalse((self.home / ".cache/huggingface").is_symlink())

    def test_data_survives_repeated_setup_and_another_sandbox(self):
        result = self.setup_links()
        self.assertEqual(result.returncode, 0, result.stderr)
        cache = self.home / ".cache/pnpm" / "cached-package"
        cache.write_text("retained")
        for home in (self.home, self.directory / "another home"):
            home.mkdir(exist_ok=True)
            result = self.setup_links(home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((home / ".cache/pnpm/cached-package").read_text(), "retained")

    def test_refuses_existing_directory_file_or_different_symlink(self):
        for kind in ("directory", "file", "symlink", "dangling symlink"):
            with self.subTest(kind=kind):
                home = self.directory / kind
                target = home / ".cache/pnpm"
                target.parent.mkdir(parents=True)
                if kind == "directory":
                    target.mkdir()
                    (target / "keep").write_text("retained")
                elif kind == "file":
                    target.write_text("retained")
                else:
                    other = self.directory / (kind + " destination")
                    if kind == "symlink":
                        other.mkdir()
                    target.symlink_to(other)
                result = self.setup_links(home=home)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Refusing to replace", result.stderr)
                if kind == "directory":
                    self.assertEqual((target / "keep").read_text(), "retained")
                elif kind == "file":
                    self.assertEqual(target.read_text(), "retained")
                else:
                    self.assertEqual(target.readlink(), other)

    def test_requires_absolute_existing_mount(self):
        for state in ("", "relative", self.directory / "missing"):
            with self.subTest(state=state):
                result = self.setup_links(state=state)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((self.home / ".cache").exists())

    def test_new_mapping_needs_only_a_config_row(self):
        kit = self.directory / "kit"
        (kit / "scripts").mkdir(parents=True)
        script = kit / "scripts" / SCRIPT.name
        shutil.copyfile(SCRIPT, script)
        (kit / CONFIG.name).write_text(
            "# Another tool\n\nnew tool/cache|.cache/new tool"
        )
        result = self.setup_links(script=script)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            (self.home / ".cache/new tool").resolve(), self.state / "new tool/cache",
        )


if __name__ == "__main__":
    unittest.main()
