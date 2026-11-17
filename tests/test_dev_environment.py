import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(shutil.which("mise"), "Local dotenv loading requires mise")
class DevEnvironmentTests(unittest.TestCase):
    def setUp(self):
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = tempfile.TemporaryDirectory(prefix="dev-env-", dir=fixtures)
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.project = self.root / "worktree"
        self.project.mkdir()
        shutil.copyfile(ROOT / "mise.toml", self.project / "mise.toml")
        self.env = {key: value for key, value in os.environ.items()
                    if key != "PISSBOX_TEST_LOCAL_SETTING"}
        self.env.update(
            MISE_TRUSTED_CONFIG_PATHS=str(ROOT),
            MISE_STATE_DIR=str(self.root / "mise-state"),
        )

    def load_environment(self):
        # `mise env` evaluates configuration without installing missing tools.
        result = subprocess.run(
            ["mise", "env", "--json"], cwd=self.project, env=self.env,
            capture_output=True, text=True, timeout=15,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        return json.loads(result.stdout)

    def test_missing_dotenv_is_optional(self):
        self.assertNotIn("PISSBOX_TEST_LOCAL_SETTING", self.load_environment())

    def test_linked_dotenv_loads_and_follows_edits_in_main_checkout(self):
        main = self.root / "main"
        main.mkdir()
        dotenv = main / ".env"
        dotenv.write_text("PISSBOX_TEST_LOCAL_SETTING=corporate\n")
        (self.project / ".env").symlink_to("../main/.env")
        self.assertEqual(self.load_environment()["PISSBOX_TEST_LOCAL_SETTING"], "corporate")

        dotenv.write_text("PISSBOX_TEST_LOCAL_SETTING=adjusted\n")
        self.assertEqual(self.load_environment()["PISSBOX_TEST_LOCAL_SETTING"], "adjusted")


if __name__ == "__main__":
    unittest.main()
