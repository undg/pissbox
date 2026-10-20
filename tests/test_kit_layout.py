import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
FILES = ROOT / "sbx-kit" / "files"
SPEC = ROOT / "sbx-kit" / "spec.yaml"

# SBX v2 static files only map these two subtrees into the sandbox. Anything
# else under files/ is ignored, which silently drops bundled tools.
ALLOWED_ROOTS = {"home", "workspace"}


class KitLayoutTests(unittest.TestCase):
    def test_static_files_only_live_under_mapped_roots(self):
        misplaced = sorted(
            str(path.relative_to(FILES))
            for path in FILES.rglob("*")
            if path.is_file() and path.relative_to(FILES).parts[0] not in ALLOWED_ROOTS
        )
        self.assertEqual(
            misplaced,
            [],
            "SBX ignores static files outside files/home/ and files/workspace/",
        )

    def test_npm_uses_shared_cache_after_links_are_created(self):
        spec = SPEC.read_text()
        cache_command = 'npm config set cache "$HOME/.cache/npm" --location=user'
        self.assertIn(cache_command, spec)
        self.assertLess(spec.index("link-shared-directories.sh"), spec.index(cache_command))

    def test_colgrep_is_marked_executable_on_path(self):
        spec = SPEC.read_text()
        self.assertIn("$HOME/.local/bin/colgrep", spec)
        self.assertIn("chmod 0755", spec)


if __name__ == "__main__":
    unittest.main()
