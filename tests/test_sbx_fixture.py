import json
import os
from pathlib import Path
import subprocess
import sys
import unittest


SBX = Path(__file__).resolve().parent / "fixtures/sbx"
# Darwin flag values: run these comparison checks on Linux too.
PENDIN = 0x20000000
ECHO = 0x8
ICANON = 0x100
ISIG = 0x80


def terminal_state():
    return [0, 0, 0, ECHO | ICANON | ISIG, 9600, 9600, [0] * 20]


class SBXTerminalStateTests(unittest.TestCase):
    def run_fixture(self, expected, actual, platform="darwin"):
        # Substitute only kernel/platform observations. Execute the real fixture
        # in a subprocess so its assertions, exit status, and diagnostics run.
        script = '''
import json, os, runpy, sys, termios
termios.PENDIN = 0x20000000
termios.tcgetattr = lambda fd: json.loads(os.environ["FIXTURE_ACTUAL_TERMIOS"])
sys.platform = os.environ["FIXTURE_PLATFORM"]
sys.argv = [sys.argv[1], "exec", "-it", "fixture", "bash"]
runpy.run_path(sys.argv[0], run_name="__main__")
'''
        env = {key: value for key, value in os.environ.items()
               if not key.startswith("TEST_SBX_")}
        env.update(TEST_SBX_EXPECT_TERMIOS=json.dumps(expected),
                   FIXTURE_ACTUAL_TERMIOS=json.dumps(actual), FIXTURE_PLATFORM=platform)
        return subprocess.run([sys.executable, "-c", script, str(SBX)],
                              env=env, capture_output=True, text=True, timeout=5)

    def test_darwin_allows_only_pending_input_bookkeeping(self):
        expected, actual = terminal_state(), terminal_state()
        actual[3] |= PENDIN
        for before, after in ((expected, actual), (actual, expected)):
            with self.subTest(expected=before[3], actual=after[3]):
                result = self.run_fixture(before, after)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_darwin_still_rejects_unrestored_settings(self):
        for name, flag in (("echo", ECHO), ("canonical", ICANON), ("signals", ISIG)):
            with self.subTest(setting=name):
                expected, actual = terminal_state(), terminal_state()
                actual[3] = (actual[3] ^ flag) | PENDIN
                result = self.run_fixture(expected, actual)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("lflag: expected", result.stderr)
        for index, name in ((4, "ispeed"), (5, "ospeed"), (6, "cc")):
            with self.subTest(setting=name):
                expected, actual = terminal_state(), terminal_state()
                actual[3] |= PENDIN
                if index == 6:
                    actual[6][16] = 1
                else:
                    actual[index] = 4800
                result = self.run_fixture(expected, actual)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(name + ": expected", result.stderr)

    def test_other_platforms_keep_strict_comparison(self):
        expected, actual = terminal_state(), terminal_state()
        actual[3] |= PENDIN
        result = self.run_fixture(expected, actual, platform="linux")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("lflag: expected", result.stderr)


if __name__ == "__main__":
    unittest.main()
