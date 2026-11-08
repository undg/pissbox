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


if __name__ == "__main__":
    unittest.main()
