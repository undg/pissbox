import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "bin/bump"


class BumpTests(unittest.TestCase):
    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        self.directory = Path(tempfile.mkdtemp(prefix="bump-", dir=fixtures))
        self.addCleanup(shutil.rmtree, self.directory, ignore_errors=True)
        home = self.directory / "home"
        home.mkdir()
        self.env = {
            **os.environ,
            "HOME": str(home),
            "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_CONFIG_SYSTEM": os.devnull,
            "GIT_AUTHOR_NAME": "Test",
            "GIT_AUTHOR_EMAIL": "test@example.com",
            "GIT_COMMITTER_NAME": "Test",
            "GIT_COMMITTER_EMAIL": "test@example.com",
            "GIT_AUTHOR_DATE": "2020-01-01T00:00:00Z",
            "GIT_COMMITTER_DATE": "2020-01-01T00:00:00Z",
        }

    def git(self, repo, *args):
        result = subprocess.run(
            ["git", *args], cwd=repo, env=self.env, text=True, capture_output=True, timeout=5,
        )
        if result.returncode:
            self.fail(f"git {' '.join(args)}: {result.stderr}")
        return result

    def new_repo(self, name):
        repo = self.directory / name
        (repo / "bin").mkdir(parents=True)
        shutil.copy2(SCRIPT, repo / "bin/bump")
        self.git(repo, "init", "-q")
        self.git(repo, "commit", "--allow-empty", "-q", "-m", "initial")
        return repo

    def launch(self, repo, *args):
        return subprocess.run(
            [str(repo / "bin/bump"), *args], cwd=repo, env=self.env,
            text=True, capture_output=True, timeout=5,
        )

    def tags(self, repo):
        return self.git(repo, "tag", "--list").stdout.splitlines()

    def test_bumps_from_no_tags(self):
        for mode, expected in (("patch", "v0.0.1"), ("minor", "v0.1.0"), ("major", "v1.0.0")):
            with self.subTest(mode=mode):
                repo = self.new_repo(mode)
                result = self.launch(repo, mode)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, f"Created tag {expected}\n")
                self.assertEqual(self.tags(repo), [expected])
                self.assertEqual(self.git(repo, "cat-file", "-t", expected).stdout.strip(), "tag")

    def test_bumps_from_latest_semver_tag(self):
        for mode, expected in (("patch", "v1.2.4"), ("minor", "v1.3.0"), ("major", "v2.0.0")):
            with self.subTest(mode=mode):
                repo = self.new_repo(mode)
                self.git(repo, "tag", "-a", "v1.2.3", "-m", "old")
                self.git(repo, "commit", "--allow-empty", "-q", "-m", "next")
                result = self.launch(repo, mode)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(expected, self.tags(repo))

    def test_uses_highest_merged_semver_tag(self):
        repo = self.new_repo("latest")
        self.git(repo, "tag", "-a", "v1.9.0", "-m", "old")
        self.git(repo, "commit", "--allow-empty", "-q", "-m", "newer")
        self.git(repo, "tag", "-a", "v1.10.0", "-m", "newest")
        self.git(repo, "commit", "--allow-empty", "-q", "-m", "next")
        result = self.launch(repo, "patch")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("v1.10.1", self.tags(repo))

    def test_rejects_tracked_changes(self):
        repo = self.new_repo("dirty")
        (repo / "tracked").write_text("before\n")
        self.git(repo, "add", "tracked")
        self.git(repo, "commit", "-q", "-m", "tracked")
        (repo / "tracked").write_text("after\n")
        result = self.launch(repo, "patch")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Tracked changes", result.stderr)
        self.assertEqual(self.tags(repo), [])

    def test_allows_untracked_files(self):
        repo = self.new_repo("untracked")
        (repo / "local.toml").write_text("local only\n")
        result = self.launch(repo, "patch")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.tags(repo), ["v0.0.1"])

    def test_rejects_already_tagged_head(self):
        repo = self.new_repo("tagged")
        self.git(repo, "tag", "-a", "v0.1.0", "-m", "release")
        result = self.launch(repo, "patch")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("already tagged v0.1.0", result.stderr)
        self.assertEqual(self.tags(repo), ["v0.1.0"])

    def test_help_and_invalid_arguments_do_not_tag(self):
        repo = self.new_repo("arguments")
        for args, status in ((("-h",), 0), (("--help",), 0), ((), 1), (("release",), 1),
                             (("patch", "minor"), 1), (("--unknown",), 1)):
            with self.subTest(args=args):
                result = self.launch(repo, *args)
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertEqual(self.tags(repo), [])
                if status == 0:
                    self.assertIn("Usage:", result.stdout)


if __name__ == "__main__":
    unittest.main()
