import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
PI_DIR = ROOT / "pi-agent"


class SettingsProfileTests(unittest.TestCase):
    def profile(self, name):
        return json.loads((PI_DIR / name).read_text())

    def test_tracked_profiles_are_valid_json(self):
        for name in ("settings-co.json", "settings-go.json"):
            with self.subTest(name=name):
                self.profile(name)

    def test_profiles_use_their_expected_providers(self):
        co_profile = self.profile("settings-co.json")
        go_profile = self.profile("settings-go.json")
        self.assertEqual(co_profile["defaultProvider"], "github-copilot")
        self.assertEqual(co_profile["defaultModel"], "gpt-6.1-sol")
        self.assertTrue(
            all(model.startswith("github-copilot/") for model in co_profile["enabledModels"])
        )
        self.assertEqual(go_profile["defaultProvider"], "opencode-go")
        self.assertTrue(
            all(model.startswith("opencode-go/") for model in go_profile["enabledModels"])
        )


if __name__ == "__main__":
    unittest.main()
