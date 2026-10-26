import json
import os
from pathlib import Path
import pty
import platform
import shutil
import subprocess
import tempfile
import unittest

from launcher_harness import checkout_root, implementation


ROOT = checkout_root()
LAUNCHER = ROOT / "bin" / "pissbox"
PI_DIR = ROOT / "pi-agent"
STATE_DIR = ROOT / ".cache"


def without_optional_bridge_diagnostics(stderr):
    # Dedicated bridge tests assert these warnings; general launcher tests must
    # work with either missing, current, or stale prebuilt optional binaries.
    return "".join(line for line in stderr.splitlines(keepends=True)
                   if not line.startswith(("Warning: Host bridge", "pissbox bridge:")))


class LauncherTests(unittest.TestCase):
    def setUp(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        self.bridge_runtime = tempfile.mkdtemp(prefix="ls-", dir=fixtures)
        self.addCleanup(self.stop_test_bridge)

    def stop_test_bridge(self):
        if not (Path(self.bridge_runtime) / "control.sock").exists():
            return
        arch = {"aarch64": "arm64", "arm64": "arm64", "x86_64": "amd64"}.get(platform.machine())
        binary = STATE_DIR / f"bin/go-bridge-{platform.system().lower()}-{arch}"
        subprocess.run([str(binary), "stop"], env=self.environment(), capture_output=True, timeout=6)

    def environment(self, **overrides):
        env = {
            key: value
            for key, value in os.environ.items()
            if not key.startswith(("TEST_SBX_", "HERDR_"))
            and key not in ("PISSBOX_PI_DIR", "PISSBOX_STATE_DIR", "PISSBOX_BRIDGE_DIR", "PISSBOX_WORKSPACE_DIR")
        }
        env["PATH"] = str(ROOT / "tests" / "fixtures") + os.pathsep + env["PATH"]
        env["PISSBOX_BRIDGE_RUNTIME_DIR"] = self.bridge_runtime
        env.update(overrides)
        return env

    def launch(self, *args, answers="", launcher=LAUNCHER, cwd=ROOT, **env):
        result = subprocess.run(
            [str(launcher), *args],
            cwd=cwd,
            env=self.environment(**env),
            input=answers,
            text=True,
            capture_output=True,
            timeout=5,
        )
        calls = []
        for line in result.stdout.splitlines():
            if not line.startswith("sbx:"):
                continue
            args = json.loads(line.removeprefix("sbx:"))
            if args[-1:] == ["pissbox-readiness"]:
                self.assertEqual(args[:3], ["exec", "--user", "1000"])
                args = ["readiness", args[3]]
            # Optional launch mailbox arguments vary with installed host binary.
            calls.append([arg for i, arg in enumerate(args)
                          if not arg.startswith("PISSBOX_BRIDGE_DIR=")
                          and not (arg == "--env" and i + 1 < len(args)
                                   and args[i + 1].startswith("PISSBOX_BRIDGE_DIR="))])
        result.stderr = without_optional_bridge_diagnostics(result.stderr)
        return result, calls

    def conflict(self, answers, **env):
        return self.launch(answers=answers, TEST_SBX_MODE="conflict", **env)

    def pi_directory(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = Path(tempfile.mkdtemp(prefix="pi-", dir=fixtures))
        for name in ("settings-co.json", "settings-go.json"):
            shutil.copyfile(PI_DIR / name, directory / name)
        return directory

    def manage(self, *args, listing=None, raw_listing=None, **kwargs):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = Path(tempfile.mkdtemp(prefix="management-", dir=fixtures))
        log = directory / "calls.jsonl"
        result, _ = self.launch(
            *args, TEST_SBX_CALL_LOG=str(log),
            TEST_SBX_LISTING=raw_listing if raw_listing is not None else json.dumps(listing if listing is not None else []),
            PISSBOX_PI_DIR="missing", PISSBOX_STATE_DIR=str(LAUNCHER / "missing"),
            **kwargs,
        )
        calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
        return result, calls

    def test_list_groups_exact_workspace_matches_first(self):
        listing = [
            {"name": "other", "agent": "codex", "status": "stopped", "workspaces": [str(ROOT.parent)]},
            {"name": "primary", "agent": "pi", "status": "running", "workspaces": [str(ROOT)]},
            {"name": "secondary", "agent": "pi", "status": "stopped", "workspaces": [str(ROOT / "tests"), str(ROOT)]},
            {"name": "child", "agent": "pi", "status": "running", "workspaces": [str(ROOT / "tests")]},
        ]
        for flag in ("-l", "--ls"):
            with self.subTest(flag=flag):
                result, calls = self.manage(flag, listing=listing)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls, [["ls", "--json"]])
                self.assertNotIn("\033[", result.stdout)
                current, others = result.stdout.split("Other sandboxes")
                self.assertIn("Current directory\n", current)
                self.assertIn(f"  {ROOT}\n", current)
                self.assertIn("primary  [running]", current)
                self.assertIn("secondary  [stopped]", current)
                self.assertNotIn("  other ", current)
                self.assertNotIn("  child ", current)
                self.assertIn("other  [stopped]  (codex)", others)
                self.assertIn("child  [running]", others)
                self.assertNotIn("  primary ", others)
                self.assertNotIn("  secondary ", others)

    def test_null_or_empty_workspaces_are_unassociated_for_listing_and_deletion(self):
        unmounted = [
            {"name": "probe-null", "agent": "volume-diag-a", "status": "stopped", "workspaces": None},
            {"name": "probe-empty", "agent": "pi", "status": "stopped", "workspaces": []},
        ]
        mounted = {"name": "project", "agent": "pi", "status": "running", "workspaces": [str(ROOT)]}
        result, calls = self.manage("-l", listing=[*unmounted, mounted])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["ls", "--json"]])
        current, others = result.stdout.split("Other sandboxes")
        self.assertIn("project  [running]", current)
        self.assertNotIn("probe-", current)
        self.assertIn("probe-null  [stopped]", others)
        self.assertIn("probe-empty  [stopped]", others)
        result, calls = self.manage("-d", ".", "-y", listing=[*unmounted, mounted])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["ls", "--json"], ["rm", "--force", "project"]])
        result, calls = self.manage("-d", ".", "-y", listing=unmounted)
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("No sandboxes associated", result.stderr)
        self.assertEqual(calls, [["ls", "--json"]])

    def test_list_empty_groups_and_uses_callers_directory(self):
        cwd = ROOT / "tests"
        result, calls = self.manage("-l", cwd=cwd)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["ls", "--json"]])
        self.assertIn("Current directory\n", result.stdout)
        self.assertIn(f"  {cwd}\n", result.stdout)
        self.assertEqual(result.stdout.count("(none)"), 2)

    def test_list_header_colors_respect_terminal_and_no_color(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = Path(tempfile.mkdtemp(prefix="list-colors-", dir=fixtures))
        for term, no_color, colored in (("xterm-256color", None, True), ("xterm-256color", "", False), ("dumb", None, False)):
            with self.subTest(term=term, no_color=no_color):
                env = self.environment(
                    TERM=term, TEST_SBX_LISTING="[]",
                    TEST_SBX_CALL_LOG=str(directory / "calls.jsonl"),
                )
                env.pop("NO_COLOR", None)
                if no_color is not None:
                    env["NO_COLOR"] = no_color
                master, slave = pty.openpty()
                try:
                    result = subprocess.run(
                        [str(LAUNCHER), "-l"], cwd=ROOT, env=env,
                        stdin=subprocess.DEVNULL, stdout=slave,
                        stderr=subprocess.PIPE, text=True, timeout=5,
                    )
                    self.assertEqual(result.returncode, 0, result.stderr)
                    output = os.read(master, 8192).decode()
                    if colored:
                        self.assertIn("\033[1;36mCurrent directory\033[0m", output)
                        self.assertIn("\033[1;36mOther sandboxes\033[0m", output)
                        self.assertIn(f"\033[2m{ROOT}\033[0m", output)
                    else:
                        self.assertNotIn("\033[", output)
                        self.assertIn("Current directory", output)
                        self.assertIn("Other sandboxes", output)
                finally:
                    os.close(master)
                    os.close(slave)

    def test_delete_all_exact_matches_after_one_confirmation(self):
        listing = [
            {"name": "first", "agent": "pi", "status": "running", "workspaces": [str(ROOT)]},
            {"name": "other", "agent": "pi", "status": "running", "workspaces": [str(ROOT.parent)]},
            {"name": "second", "agent": "pi", "status": "stopped", "workspaces": [str(ROOT / "tests"), str(ROOT), str(ROOT)]},
        ]
        for args in (("-d", "."), ("--delete", "."), ("--delete=.",)):
            with self.subTest(args=args):
                result, calls = self.manage(*args, listing=listing, answers="yes\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls, [["ls", "--json"], ["rm", "--force", "first", "second"]])
                self.assertEqual(result.stderr.count("[y/N]"), 1)
                self.assertIn("first  [running]", result.stderr)
                self.assertIn("second  [stopped]", result.stderr)
                self.assertNotIn("  other ", result.stderr)
                self.assertIn("Mounted host files remain", result.stderr)
                self.assertTrue(ROOT.is_dir())

    def test_delete_yes_skips_confirmation_and_keeps_exact_matching(self):
        listing = [
            {"name": "first", "agent": "pi", "status": "running", "workspaces": [str(ROOT)]},
            {"name": "other", "agent": "pi", "status": "running", "workspaces": [str(ROOT.parent)]},
            {"name": "second", "agent": "pi", "status": "stopped", "workspaces": [str(ROOT / "tests"), str(ROOT)]},
        ]
        for args in (("--delete", ".", "--yes"), ("--yes", "--delete=."), ("-y", "-d", "."), ("-d", ".", "-y")):
            with self.subTest(args=args):
                result, calls = self.manage(*args, listing=listing)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls, [["ls", "--json"], ["rm", "--force", "first", "second"]])
                self.assertNotIn("[y/N]", result.stderr)
                self.assertIn("Mounted host files remain", result.stderr)

    def test_yes_with_list_only_lists_sandboxes(self):
        result, calls = self.manage("--yes", "--ls")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["ls", "--json"]])

    def test_delete_resolves_spaces_relative_paths_and_symlinks(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = Path(tempfile.mkdtemp(prefix="delete-", dir=fixtures))
        target = directory / "workspace with spaces"
        target.mkdir()
        link = directory / "link"
        link.symlink_to(target)
        listing = [{"name": "match", "agent": "pi", "status": "running", "workspaces": [str(target)]}]
        result, calls = self.manage("-d", "link", cwd=directory, listing=listing, answers="y\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[-1], ["rm", "--force", "match"])
        self.assertTrue(target.is_dir())
        self.assertTrue(link.is_symlink())

    def test_delete_decline_eof_and_invalid_confirmation(self):
        listing = [{"name": "match", "agent": "pi", "status": "running", "workspaces": [str(ROOT)]}]
        for answers, status, deleted in (("\n", 0, False), ("no\n", 0, False), ("", 1, False), ("y", 1, False), ("maybe\nYES\n", 0, True)):
            with self.subTest(answers=answers):
                result, calls = self.manage("-d", ".", listing=listing, answers=answers)
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertEqual(calls[1:], [["rm", "--force", "match"]] if deleted else [])

    def test_management_rejects_missing_invalid_and_conflicting_arguments(self):
        for args in (
            ("-d",), ("--delete", ""), ("--delete=",), ("-d", "--ls"),
            ("-d", "nonexistent-directory"), ("-d", str(LAUNCHER)),
            ("-d", ".", "extra"), ("-l", "."), ("-d", ".", "-l"),
            ("-l", "--name", "box"), ("--name=box", "-d", "."),
            ("--yes", "-d", "nonexistent-directory"), ("--yes", "-d", ".", "-l"),
        ):
            with self.subTest(args=args):
                result, calls = self.manage(*args)
                self.assertNotEqual(result.returncode, 0)
                self.assertTrue(result.stderr)
                self.assertEqual(calls, [])

    def test_delete_no_matches_is_an_error_without_confirmation(self):
        for extra in ((), ("--yes",)):
            with self.subTest(extra=extra):
                result, calls = self.manage("-d", ".", *extra, answers="y\n")
                self.assertEqual(result.returncode, 1)
                self.assertIn("No sandboxes associated", result.stderr)
                self.assertNotIn("[y/N]", result.stderr)
                self.assertEqual(calls, [["ls", "--json"]])

    def test_management_preserves_listing_and_deletion_failures(self):
        for args in (("-l",), ("-d", "."), ("-d", ".", "--yes")):
            with self.subTest(args=args):
                result, calls = self.manage(*args, TEST_SBX_LIST_STATUS="7", answers="y\n")
                self.assertEqual(result.returncode, 7, result.stderr)
                self.assertIn("listing unavailable", result.stderr)
                self.assertEqual(calls, [["ls", "--json"]])
        listing = [{"name": "match", "agent": "pi", "status": "running", "workspaces": [str(ROOT)]}]
        for extra in ((), ("--yes",)):
            with self.subTest(extra=extra):
                result, calls = self.manage("-d", ".", *extra, listing=listing, TEST_SBX_REMOVE_STATUS="9", answers="y\n")
                self.assertEqual(result.returncode, 9, result.stderr)
                self.assertEqual(calls[-1], ["rm", "--force", "match"])

    def test_management_rejects_invalid_metadata_before_confirmation(self):
        invalid_listings = (
            {}, [None],
            [{"name": "-unsafe", "agent": "pi", "status": "running", "workspaces": [str(ROOT)]}],
            [{"name": "match", "agent": "pi", "status": "running", "workspaces": str(ROOT)}],
            [{"name": "match", "agent": "pi", "status": "running", "workspaces": False}],
            [{"name": "match", "agent": "pi", "status": "running"}],
            [{"name": "match", "agent": "pi", "status": "running", "workspaces": ["relative/path"]}],
        )
        for listing in invalid_listings:
            with self.subTest(listing=listing):
                result, calls = self.manage("-d", ".", "--yes", listing=listing)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("[y/N]", result.stderr)
                self.assertEqual(calls, [["ls", "--json"]])

    def test_management_identifies_rejected_sandbox_and_field_without_deleting(self):
        valid = {"name": "project", "agent": "pi", "status": "running", "workspaces": [str(ROOT)]}
        for field, value in (("name", "-unsafe"), ("agent", None), ("status", 1),
                             ("workspaces", False), ("workspaces", ["relative/path"])):
            invalid = {**valid, "name": "broken", field: value}
            for args in (("-l",), ("-d", ".", "--yes")):
                with self.subTest(field=field, value=value, args=args):
                    result, calls = self.manage(*args, listing=[valid, invalid])
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn(f'Invalid sandbox metadata for {json.dumps(invalid["name"])}: {field}', result.stderr)
                    self.assertNotIn("[y/N]", result.stderr)
                    self.assertEqual(calls, [["ls", "--json"]])

    def test_management_rejects_empty_null_and_malformed_json(self):
        for raw_listing in ("", "null", "not json", "[{}"):
            with self.subTest(raw_listing=raw_listing):
                result, calls = self.manage("-d", ".", raw_listing=raw_listing, answers="y\n")
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("[y/N]", result.stderr)
                self.assertEqual(calls, [["ls", "--json"]])

    def test_management_reports_missing_jq_before_calling_sbx(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        commands = Path(tempfile.mkdtemp(prefix="no-jq-", dir=fixtures))
        (commands / "bash").symlink_to(shutil.which("bash"))
        result, calls = self.manage("-l", PATH=str(commands))
        self.assertEqual(result.returncode, 1)
        self.assertIn("install jq", result.stderr)
        self.assertEqual(calls, [])

    def test_repeated_delete_uses_last_directory(self):
        listing = [{"name": "match", "agent": "pi", "status": "running", "workspaces": [str(ROOT)]}]
        result, calls = self.manage("-d", "missing", "--delete=.", listing=listing, answers="y\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[-1], ["rm", "--force", "match"])

    def test_management_help_does_not_query_sbx(self):
        for args in (("-l", "--help"), ("-d", ".", "-h"), ("--yes", "--help")):
            with self.subTest(args=args):
                result, calls = self.manage(*args)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("--delete DIR", result.stdout)
                self.assertIn("--ls", result.stdout)
                self.assertIn("--yes", result.stdout)
                self.assertEqual(calls, [])

    def test_new_sandbox_keeps_original_arguments(self):
        result, calls = self.launch()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [[
            "run", "--env", f"PISSBOX_PI_DIR={PI_DIR}",
            "--env", f"PISSBOX_STATE_DIR={STATE_DIR}",
            "--env", f"PISSBOX_WORKSPACE_DIR={ROOT}",
            str(ROOT / "bin" / "../sbx-kit"), str(ROOT), str(PI_DIR), str(STATE_DIR),
        ]])
        self.assertEqual(result.stderr, "")

    def test_project_setup_is_forwarded_to_sandbox_not_executed_on_host(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            project = Path(directory) / "primary workspace"
            project.mkdir()
            (project / ".pissbox.toml").write_text('setup = ["touch host-marker", "exit 42"]\n')
            result, calls = self.launch(str(project), str(ROOT), cwd=project,
                                        PISSBOX_WORKSPACE_DIR="wrong-inherited-value")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn(f"PISSBOX_WORKSPACE_DIR={project}", calls[0])
            self.assertFalse((project / "host-marker").exists())

    def test_configured_parent_is_added_for_bare_yes_and_dot_launches(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            parent = Path(directory)
            project = parent / "project"
            project.mkdir()
            (project / ".pissbox.toml").write_text('workspaces = [".."]\n')
            kit = str(ROOT / "bin/../sbx-kit")
            for args in ((), ("-y",), (".",), (".", "-y")):
                with self.subTest(args=args):
                    result, calls = self.launch(*args, cwd=project)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    mounts = calls[0][calls[0].index(kit) + 1:]
                    primary = "." if "." in args else str(project)
                    self.assertEqual(mounts, [primary, str(parent), str(PI_DIR), str(STATE_DIR)])

    def test_configured_workspaces_add_to_cli_paths_not_caller_or_secondary_config(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            root = Path(directory)
            project = root / "primary"
            extra = root / "CLI extra"
            caller = root / "caller"
            configured = project / "configured with spaces"
            for path in (project, extra, caller, configured):
                path.mkdir()
            (project / ".pissbox.toml").write_text('workspaces = ["configured with spaces"]\n')
            for path in (caller, extra):
                (path / ".pissbox.toml").write_text("not valid TOML")
            result, calls = self.launch(str(project), str(extra), cwd=caller)
            self.assertEqual(result.returncode, 0, result.stderr)
            kit = str(ROOT / "bin/../sbx-kit")
            self.assertEqual(calls[0][calls[0].index(kit) + 1:],
                             [str(project), str(extra), str(configured), str(PI_DIR), str(STATE_DIR)])

    def test_ignore_config_workspaces_keeps_cli_mounts_and_skips_directory_checks(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            root = Path(directory)
            project = root / "project"
            extra = root / "CLI extra"
            project.mkdir()
            extra.mkdir()
            text = 'workspaces = ["..", "missing"]\nsetup = ["touch host-marker"]\n'
            config = project / ".pissbox.toml"
            config.write_text(text)
            flag = "--ignore-config-workspaces"
            for args in ((flag,), ("-y", flag), (".", flag), (flag, ".", str(extra)),
                         (".", flag, str(extra)), (".", str(extra), flag, flag)):
                with self.subTest(args=args):
                    result, calls = self.launch(*args, cwd=project)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    kit = str(ROOT / "bin/../sbx-kit")
                    expected = ["." if "." in args else str(project)]
                    if str(extra) in args:
                        expected.append(str(extra))
                    expected.extend([str(PI_DIR), str(STATE_DIR)])
                    self.assertEqual(calls[0][calls[0].index(kit) + 1:], expected)
                    self.assertNotIn(flag, calls[0])
                    self.assertFalse((project / "host-marker").exists())
                    self.assertEqual(config.read_text(), text)

    def test_ignore_config_workspaces_still_validates_toml_and_schema(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            project = Path(directory)
            for text in ('workspaces = ["..",', 'workspaces = ".."', 'setup = 5'):
                with self.subTest(text=text):
                    (project / ".pissbox.toml").write_text(text)
                    result, calls = self.launch("--ignore-config-workspaces", cwd=project)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(calls, [])

    def test_ignore_config_workspaces_after_separator_is_a_path_not_a_flag(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            caller = Path(directory)
            flag = "--ignore-config-workspaces"
            project = caller / flag
            project.mkdir()
            (project / ".pissbox.toml").write_text('workspaces = [".."]\n')
            result, calls = self.launch("--", flag, cwd=caller)
            self.assertEqual(result.returncode, 0, result.stderr)
            kit = str(ROOT / "bin/../sbx-kit")
            self.assertEqual(calls[0][calls[0].index(kit) + 1:],
                             [flag, str(caller), str(PI_DIR), str(STATE_DIR)])

    def test_ignore_config_workspaces_does_not_change_existing_reattach(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            project = Path(directory)
            (project / ".pissbox.toml").write_text('workspaces = ["missing"]\n')
            result, calls = self.launch("--ignore-config-workspaces", "-y", cwd=project,
                                        TEST_SBX_MODE="conflict", TEST_SBX_NAME="actual-name")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(calls[1:], [["readiness", "actual-name"], ["run", "--name", "actual-name"]])

    def test_ignore_config_workspaces_is_documented_in_help(self):
        result, calls = self.launch("--ignore-config-workspaces", "--help")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("--ignore-config-workspaces", result.stdout)
        self.assertEqual(calls, [])

    def test_symlinked_config_resolves_workspaces_from_primary_not_link_target(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            root = Path(directory)
            project = root / "project"
            extra = root / "extra"
            rules = root / "shared/nested/rules.toml"
            project.mkdir()
            extra.mkdir()
            rules.parent.mkdir(parents=True)
            rules.write_text('workspaces = ["..", "../extra"]\nsetup = []\n')
            (project / ".pissbox.toml").symlink_to("../shared/nested/rules.toml")
            result, calls = self.launch(cwd=project)
            self.assertEqual(result.returncode, 0, result.stderr)
            kit = str(ROOT / "bin/../sbx-kit")
            self.assertEqual(calls[0][calls[0].index(kit) + 1:],
                             [str(project), str(root), str(extra), str(PI_DIR), str(STATE_DIR)])

    def test_duplicate_cli_configured_and_shared_workspaces_are_removed(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            root = Path(directory)
            project = root / "project"
            extra = root / "extra"
            project.mkdir()
            extra.mkdir()
            (root / "alias").symlink_to(extra, target_is_directory=True)
            entries = [".", "../extra", "../alias", "..", "..", str(PI_DIR), str(STATE_DIR)]
            (project / ".pissbox.toml").write_text("workspaces = " + json.dumps(entries))
            result, calls = self.launch(".", "./", str(project), str(extra), str(extra), cwd=project)
            self.assertEqual(result.returncode, 0, result.stderr)
            kit = str(ROOT / "bin/../sbx-kit")
            self.assertEqual(calls[0][calls[0].index(kit) + 1:],
                             [".", str(extra), str(root), str(PI_DIR), str(STATE_DIR)])

    def test_dash_prefixed_workspace_after_separator_is_not_a_cd_option(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            caller = Path(directory)
            project = caller / "--project"
            project.mkdir()
            result, calls = self.launch("--", "--project", cwd=caller)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("PISSBOX_WORKSPACE_DIR=" + str(project), calls[0])

    def test_configured_paths_are_literal_not_shell_expansions(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            project = Path(directory)
            names = ["$(touch host-marker)", "$HOME", "*"]
            for name in names:
                (project / name).mkdir()
            (project / ".pissbox.toml").write_text("workspaces = " + json.dumps(names))
            result, calls = self.launch(cwd=project)
            self.assertEqual(result.returncode, 0, result.stderr)
            kit = str(ROOT / "bin/../sbx-kit")
            expected = [str(project), *(str(project / name) for name in names), str(PI_DIR), str(STATE_DIR)]
            self.assertEqual(calls[0][calls[0].index(kit) + 1:], expected)
            self.assertFalse((project / "host-marker").exists())

    def test_invalid_project_config_fails_before_settings_state_or_sbx(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            project = Path(directory)
            config = project / ".pissbox.toml"
            state = project / "uncreated-state"
            pi = self.pi_directory()
            for text in ('workspaces = ".."', 'workspaces = [1]', 'workspaces = [""]',
                         'workspaces = ["\\u0000"]', 'workspaces = ["a\\nb"]',
                         'workspaces = ["..",', 'workpsaces = [".."]', 'setup = 5'):
                with self.subTest(text=text):
                    config.write_text(text)
                    result, calls = self.launch(cwd=project, PISSBOX_PI_DIR=str(pi),
                                                PISSBOX_STATE_DIR=str(state))
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(calls, [])
                    self.assertFalse((pi / "settings.json").exists())
                    self.assertFalse(state.exists())

    def test_configured_workspaces_must_be_existing_directories(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            project = Path(directory)
            (project / "plain-file").write_text("not a directory")
            for path in ("missing", "plain-file"):
                with self.subTest(path=path):
                    (project / ".pissbox.toml").write_text("workspaces = " + json.dumps([path]))
                    result, calls = self.launch(cwd=project)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("Configured workspace directory does not exist", result.stderr)
                    self.assertEqual(calls, [])

    def test_broken_project_config_link_fails_on_host_before_sbx(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            project = Path(directory)
            (project / ".pissbox.toml").symlink_to("missing-config")
            result, calls = self.launch(cwd=project)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("readable TOML file", result.stderr)
            self.assertEqual(calls, [])

    def test_config_requires_host_parser_tools_before_launching(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            project = Path(directory)
            (project / ".pissbox.toml").write_text("workspaces = []\n")
            for missing in ("tomljson", "jq"):
                with self.subTest(missing=missing):
                    tools = project / missing
                    tools.mkdir()
                    for tool in ("bash", "dirname", "readlink", "tomljson", "jq"):
                        if tool != missing:
                            (tools / tool).symlink_to(shutil.which(tool))
                    result, calls = self.launch(cwd=project, PATH=str(tools))
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("requires " + missing + " on the host", result.stderr)
                    self.assertEqual(calls, [])

    def test_management_and_help_ignore_project_config(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            project = Path(directory)
            (project / ".pissbox.toml").write_text("not valid TOML")
            result, calls = self.launch("--help", cwd=project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(calls, [])
            result, calls = self.manage("--ls", listing=[], cwd=project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(calls, [["ls", "--json"]])
            listing = [{"name": "existing", "agent": "pi", "status": "running",
                        "workspaces": [str(project)]}]
            result, calls = self.manage("--delete", str(project), "--yes", listing=listing, cwd=project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(calls, [["ls", "--json"], ["rm", "--force", "existing"]])

    def test_configured_workspaces_do_not_change_existing_sandbox_reattach(self):
        fixtures = STATE_DIR / "test-fixtures"
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            project = Path(directory)
            (project / ".pissbox.toml").write_text('workspaces = [".."]\n')
            result, calls = self.launch("-y", cwd=project, TEST_SBX_MODE="conflict",
                                        TEST_SBX_NAME="actual-name")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(calls[1:], [["readiness", "actual-name"], ["run", "--name", "actual-name"]])

    def test_launch_from_another_directory(self):
        cwd = ROOT / "tests" / "fixtures"
        for launcher in (LAUNCHER, "pissbox", cwd / "linked bin" / "pissbox"):
            with self.subTest(launcher=launcher):
                result, calls = self.launch(launcher=launcher, cwd=cwd)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stderr, "")
                self.assertEqual(calls, [[
                    "run", "--env", f"PISSBOX_PI_DIR={PI_DIR}",
                    "--env", f"PISSBOX_STATE_DIR={STATE_DIR}",
                    "--env", f"PISSBOX_WORKSPACE_DIR={cwd}",
                    str(ROOT / "bin" / "../sbx-kit"), str(cwd), str(PI_DIR), str(STATE_DIR),
                ]])

    def test_symlink_keeps_overrides_relative_to_caller(self):
        cwd = ROOT / "tests" / "fixtures"
        for args in ((), ("--pissbox-pi-dir", "../../pi-agent")):
            with self.subTest(args=args):
                result, calls = self.launch(
                    *args, ".", launcher="pissbox", cwd=cwd,
                    PISSBOX_PI_DIR="../../pi-agent" if not args else "missing",
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls, [[
                    "run", "--env", f"PISSBOX_PI_DIR={PI_DIR}",
                    "--env", f"PISSBOX_STATE_DIR={STATE_DIR}",
                    "--env", f"PISSBOX_WORKSPACE_DIR={cwd}/.",
                    str(ROOT / "bin" / "../sbx-kit"), ".", str(PI_DIR), str(STATE_DIR),
                ]])

    def test_state_directory_overrides_are_relative_to_caller(self):
        cwd = ROOT / "tests" / "fixtures"
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = Path(tempfile.mkdtemp(prefix="launcher-", dir=fixtures))
        for index, option in enumerate(("environment", "separate", "equals", "repeated")):
            with self.subTest(option=option):
                state = directory / f"new state {index}"
                relative = os.path.relpath(state, cwd)
                env = {"PISSBOX_STATE_DIR": str(LAUNCHER)}
                if option == "environment":
                    args = ()
                    env["PISSBOX_STATE_DIR"] = relative
                elif option == "separate":
                    args = ("--pissbox-state-dir", relative)
                elif option == "equals":
                    args = (f"--pissbox-state-dir={relative}",)
                else:
                    args = ("--pissbox-state-dir", "unused", f"--pissbox-state-dir={relative}")
                result, calls = self.launch(*args, launcher="pissbox", cwd=cwd, **env)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue(state.is_dir())
                self.assertIn(f"PISSBOX_STATE_DIR={state}", calls[0])
                self.assertEqual(calls[0][-3:], [str(cwd), str(PI_DIR), str(state)])

    def test_state_directory_symlink_is_canonicalized_before_mounting(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        directory = Path(tempfile.mkdtemp(prefix="launcher-", dir=fixtures))
        target = directory / "state"
        target.mkdir()
        link = directory / "link"
        link.symlink_to(target)
        result, calls = self.launch(PISSBOX_STATE_DIR=str(link))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f"PISSBOX_STATE_DIR={target}", calls[0])
        self.assertEqual(calls[0][-1], str(target))

    def test_invalid_state_directory_never_launches_sbx(self):
        for args in (
            ("--pissbox-state-dir",), ("--pissbox-state-dir=",),
            ("--pissbox-state-dir", ""), ("--pissbox-state-dir", "--name=foo"),
            ("--pissbox-state-dir", str(LAUNCHER)),
        ):
            with self.subTest(args=args):
                result, calls = self.launch(*args)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(calls, [])
                self.assertNotEqual(result.stderr, "")

    def test_state_options_can_appear_among_workspaces(self):
        for args in (
            ("one", "--pissbox-state-dir", str(STATE_DIR), "two", "--name=box"),
            ("--name=box", "--pissbox-state-dir", str(STATE_DIR), "one", "two"),
        ):
            with self.subTest(args=args):
                result, calls = self.launch(*args)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[0][-4:], ["one", "two", str(PI_DIR), str(STATE_DIR)])
                self.assertIn("PISSBOX_STATE_DIR=" + str(STATE_DIR), calls[0])
                self.assertIn("box", calls[0])

    @unittest.skipIf(implementation() == "go", "Go hostname seam: TestSettingsProfilesAndRelinking")
    def test_settings_profile_follows_hostname(self):
        for host, profile in (
            ("cm", "settings-go.json"),
            ("di-7415", "settings-go.json"),
            ("other-host", "settings-co.json"),
            ("", "settings-co.json"),
        ):
            with self.subTest(host=host):
                pi = self.pi_directory()
                result, calls = self.launch(
                    "--pissbox-pi-dir", str(pi), TEST_PISSBOX_HOST=host,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue((pi / "settings.json").is_symlink())
                self.assertEqual(os.readlink(pi / "settings.json"), profile)
                self.assertIn(f"PISSBOX_PI_DIR={pi}", calls[0])

    @unittest.skipIf(implementation() == "go", "Go hostname seam: TestSettingsProfilesAndRelinking")
    def test_switching_host_relinks_settings_profile(self):
        pi = self.pi_directory()
        for host, profile in (
            ("cm", "settings-go.json"),
            ("other-host", "settings-co.json"),
            ("di-7415", "settings-go.json"),
        ):
            with self.subTest(host=host):
                result, _ = self.launch(
                    "--pissbox-pi-dir", str(pi), TEST_PISSBOX_HOST=host,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(os.readlink(pi / "settings.json"), profile)

    def test_settings_profile_backs_up_a_regular_file(self):
        pi = self.pi_directory()
        settings = pi / "settings.json"
        settings.write_text("{}\n")
        result, calls = self.launch(
            "--pissbox-pi-dir", str(pi), TEST_PISSBOX_HOST="cm",
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Backing up existing Pi settings file", result.stderr)
        self.assertEqual((pi / "settings.json.replaced").read_text(), "{}\n")
        expected = "settings-go.json"
        if implementation() == "go" and platform.node().split(".", 1)[0] not in ("cm", "di-7415"):
            expected = "settings-co.json"
        self.assertEqual(os.readlink(settings), expected)
        self.assertEqual(len(calls), 1)

    def test_missing_settings_profile_never_launches_sbx(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        pi = Path(tempfile.mkdtemp(prefix="pi-missing-", dir=fixtures))
        missing = "settings-go.json"
        if implementation() == "go" and platform.node().split(".", 1)[0] not in ("cm", "di-7415"):
            missing = "settings-co.json"
        available = "settings-co.json" if missing == "settings-go.json" else "settings-go.json"
        shutil.copyfile(PI_DIR / available, pi / available)
        result, calls = self.launch(
            "--pissbox-pi-dir", str(pi), TEST_PISSBOX_HOST="cm",
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Settings profile does not exist", result.stderr)
        self.assertEqual(calls, [])

    def test_menu_defaults_to_pi(self):
        result, calls = self.conflict("\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[1:], [["readiness", "pi-existing"], ["run", "--name", "pi-existing"]])
        self.assertIn("Choose [P/c/s/d/q]:", result.stderr)
        self.assertNotIn("Do you want to delete", result.stderr)

    def test_continue_menu_choice_resumes_previous_pi_session_without_enter(self):
        for answer in ("c", "C"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(answer)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[1:], [["readiness", "pi-existing"],
                                            ["run", "--name", "pi-existing", "--", "--continue"]])
                self.assertNotIn("--continue", calls[0])
                self.assertIn("c) Continue previous Pi session", result.stderr)

    def test_continue_flags_follow_workspaces_and_work_with_yes(self):
        for flag in ("-c", "--continue"):
            for args in ((flag, ".", "workspace with spaces"),
                         (".", flag, "workspace with spaces"),
                         (".", "workspace with spaces", flag, flag)):
                with self.subTest(args=args):
                    result, calls = self.launch(*args)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(calls[0][-6:], [".", "workspace with spaces", str(PI_DIR),
                                                    str(STATE_DIR), "--", "--continue"])
            for extra, answers in (((), "p"), (("-y",), "")):
                with self.subTest(flag=flag, extra=extra):
                    result, calls = self.launch(flag, *extra, answers=answers, TEST_SBX_MODE="conflict")
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(calls[1:], [["readiness", "pi-existing"],
                                                ["run", "--name", "pi-existing", "--", "--continue"]])

    def test_continue_flag_after_separator_is_a_workspace(self):
        result, calls = self.launch("--", "--continue")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("--", calls[0])
        self.assertEqual(calls[0][-3:], ["--continue", str(PI_DIR), str(STATE_DIR)])

    def test_yes_opens_existing_pi_without_reading_menu_input(self):
        for flag in ("-y", "--yes"):
            for answers in ("", "d\ny\n"):
                with self.subTest(flag=flag, answers=answers):
                    result, calls = self.launch(
                        flag, answers=answers, TEST_SBX_MODE="conflict",
                        TEST_SBX_NAME="actual-name",
                    )
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(calls[1:], [["readiness", "actual-name"], ["run", "--name", "actual-name"]])
                    self.assertNotIn("Choose [P/c/s/d/q]", result.stderr)
                    self.assertNotIn("d) Delete sandbox", result.stderr)
                    self.assertNotIn("[y/N]", result.stderr)

    def test_yes_preserves_multiple_workspaces_in_any_flag_position(self):
        for flag in ("-y", "--yes"):
            for args in ((flag, ".", "workspace with spaces"),
                         (".", flag, "workspace with spaces"),
                         (".", "workspace with spaces", flag)):
                with self.subTest(args=args):
                    result, calls = self.launch(*args)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(calls, [[
                        "run", "--env", f"PISSBOX_PI_DIR={PI_DIR}",
                        "--env", f"PISSBOX_STATE_DIR={STATE_DIR}",
                        "--env", f"PISSBOX_WORKSPACE_DIR={ROOT}/.",
                        str(ROOT / "bin" / "../sbx-kit"), ".", "workspace with spaces",
                        str(PI_DIR), str(STATE_DIR),
                    ]])

    def test_yes_after_separator_is_a_workspace_not_menu_consent(self):
        result, calls = self.launch("--", "--yes", TEST_SBX_MODE="conflict")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertIn("--yes", calls[0])
        self.assertIn("Choose [P/c/s/d/q]", result.stderr)

    def test_yes_preserves_launch_and_attach_failures(self):
        for env, status, call_count in (
            ({"TEST_SBX_MODE": "error"}, 7, 1),
            ({"TEST_SBX_RUN_STATUS": "42"}, 42, 1),
            ({"TEST_SBX_MODE": "conflict", "TEST_SBX_ATTACH_STATUS": "9"}, 9, 3),
        ):
            with self.subTest(env=env):
                result, calls = self.launch("--yes", **env)
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertEqual(len(calls), call_count)
                self.assertNotIn("Choose [P/c/s/d/q]", result.stderr)

    def test_pi_choice(self):
        for answer in ("p", "P"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(answer + "\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[1:], [["readiness", "pi-existing"], ["run", "--name", "pi-existing"]])

    def test_shell_choice(self):
        for answer in ("s", "S"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(answer + "\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[1:], [["readiness", "pi-existing"], ["exec", "-it", "pi-existing", "bash"]])
                self.assertNotIn("Do you want to delete", result.stderr)

    def test_quit_leaves_sandbox_untouched(self):
        for answer in ("q", "Q"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(answer + "\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(calls), 1)
                self.assertNotIn("Do you want to delete", result.stderr)

    def test_delete_defaults_to_no(self):
        for choice in ("d", "D"):
            with self.subTest(choice=choice):
                result, calls = self.conflict(f"{choice}\n\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(calls), 1)
                self.assertIn("Do you want to delete it? [y/N]", result.stderr)
                self.assertIn("sandbox-local data", result.stderr)

    def test_declining_deletion_leaves_sandbox_untouched(self):
        for answer in ("n", "N", "no", "NO", "No", "nO"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(f"d{answer}\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(calls), 1)

    def test_explicit_yes_deletes_and_exits(self):
        for answer in ("y", "Y", "yes", "YES", "Yes", "yEs"):
            with self.subTest(answer=answer):
                result, calls = self.conflict(f"d{answer}\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(calls[1:], [["rm", "--force", "pi-existing"]])

    def test_invalid_answers_repeat_the_question(self):
        result, calls = self.conflict("xydsure\n\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertEqual(result.stderr.count("Please choose p, c, s, d, or q."), 2)
        self.assertEqual(result.stderr.count("Choose [P/c/s/d/q]:"), 3)
        self.assertEqual(result.stderr.count("Please answer yes or no."), 1)
        self.assertEqual(result.stderr.count("Do you want to delete it? [y/N]"), 2)

    def test_eof_never_attaches_opens_shell_or_deletes(self):
        for answers in ("", "x", "d", "dy"):
            with self.subTest(answers=answers):
                result, calls = self.conflict(answers)
                self.assertEqual(result.returncode, 1, result.stderr)
                self.assertEqual(len(calls), 1)

    def test_uses_name_reported_by_sbx(self):
        for answers, expected in (
            ("\n", ["run", "--name", "actual-name"]),
            ("s\n", ["exec", "-it", "actual-name", "bash"]),
            ("dy\n", ["rm", "--force", "actual-name"]),
            ("c", ["run", "--name", "actual-name", "--", "--continue"]),
        ):
            with self.subTest(answers=answers):
                result, calls = self.launch(
                    "--name", "requested", answers=answers,
                    TEST_SBX_MODE="conflict", TEST_SBX_NAME="actual-name",
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                probes = [["readiness", "actual-name"]] if expected[0] != "rm" else []
                self.assertEqual(calls[1:], probes + [expected])

    def test_unrelated_failure_is_not_a_conflict(self):
        result, calls = self.launch(TEST_SBX_MODE="error")
        self.assertEqual(result.returncode, 7)
        self.assertEqual(result.stderr, "error: daemon unavailable\n")
        self.assertEqual(len(calls), 1)

    def test_nonzero_session_exit_does_not_prompt(self):
        result, calls = self.launch(TEST_SBX_RUN_STATUS="42")
        self.assertEqual(result.returncode, 42)
        self.assertEqual(result.stderr, "")
        self.assertEqual(len(calls), 1)

    def test_pi_shell_and_remove_failures_are_preserved(self):
        for answers, env in (
            ("\n", {"TEST_SBX_ATTACH_STATUS": "9"}),
            ("s\n", {"TEST_SBX_EXEC_STATUS": "9"}),
            ("dy\n", {"TEST_SBX_REMOVE_STATUS": "9"}),
            ("c", {"TEST_SBX_ATTACH_STATUS": "9"}),
        ):
            with self.subTest(env=env):
                result, _ = self.conflict(answers, **env)
                self.assertEqual(result.returncode, 9, result.stderr)

    def test_terminal_stdin_and_stdout_are_preserved(self):
        master, slave = pty.openpty()
        try:
            result = subprocess.run(
                [str(LAUNCHER)], cwd=ROOT,
                env=self.environment(TEST_SBX_REQUIRE_TTY="1"),
                stdin=slave, stdout=slave, stderr=subprocess.PIPE,
                text=True, timeout=5,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(without_optional_bridge_diagnostics(result.stderr), "")
        finally:
            os.close(master)
            os.close(slave)

    def test_menu_preserves_terminal_stdin_and_stdout(self):
        for answer in (b"\n", b"p", b"c", b"s"):
            with self.subTest(answer=answer):
                master, slave = pty.openpty()
                try:
                    os.write(master, answer)
                    result = subprocess.run(
                        [str(LAUNCHER)], cwd=ROOT,
                        env=self.environment(
                            TEST_SBX_REQUIRE_TTY="1", TEST_SBX_MODE="conflict",
                        ),
                        stdin=slave, stdout=slave, stderr=subprocess.PIPE,
                        text=True, timeout=5,
                    )
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertIn("Choose [P/c/s/d/q]:", result.stderr)
                finally:
                    os.close(master)
                    os.close(slave)

    def test_help_does_not_launch_sbx_or_validate_pi_directory(self):
        for args in (("-h",), ("--help",), (".", "--help")):
            with self.subTest(args=args):
                result, calls = self.launch(
                    *args, PISSBOX_PI_DIR="missing", PISSBOX_STATE_DIR=str(LAUNCHER / "missing"),
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("[P/c/s/d/q]", result.stdout)
                self.assertEqual(calls, [])

    def test_help_only_needs_cat_not_runtime_commands(self):
        fixtures = STATE_DIR / "test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        commands = Path(tempfile.mkdtemp(prefix="help-path-", dir=fixtures))
        (commands / "cat").symlink_to(shutil.which("cat"))
        for args in (("-h",), ("workspace", "--help")):
            with self.subTest(args=args):
                result = subprocess.run(
                    ([shutil.which("bash")] if implementation() == "bash" else []) + [str(LAUNCHER), *args],
                    cwd=ROOT,
                    env=self.environment(PATH=str(commands)),
                    text=True,
                    capture_output=True,
                    timeout=5,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("Usage:", result.stdout)
                self.assertEqual(result.stderr, "")

    def test_empty_directory_options_fail_before_changing_settings(self):
        for option in ("--pissbox-pi-dir", "--pissbox-state-dir"):
            with self.subTest(option=option):
                pi = self.pi_directory()
                result, calls = self.launch(
                    f"{option}=", PISSBOX_PI_DIR=str(pi),
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(f"{option} requires a directory", result.stderr)
                self.assertFalse((pi / "settings.json").is_symlink())
                self.assertEqual(calls, [])

    def test_empty_environment_directories_use_checkout_defaults(self):
        result, calls = self.launch(PISSBOX_PI_DIR="", PISSBOX_STATE_DIR="")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f"PISSBOX_PI_DIR={PI_DIR}", calls[0])
        self.assertIn(f"PISSBOX_STATE_DIR={STATE_DIR}", calls[0])

    def test_workspace_and_option_forwarding(self):
        pi = self.pi_directory()
        result, calls = self.launch(
            "workspace with spaces", "--name=first", "second workspace",
            "--name", "last", "--pissbox-pi-dir", str(pi), "--", "--path",
            PISSBOX_PI_DIR="missing", TEST_PISSBOX_HOST="cm",
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [[
            "run", "--env", f"PISSBOX_PI_DIR={pi}",
            "--env", f"PISSBOX_STATE_DIR={STATE_DIR}",
            "--env", f"PISSBOX_WORKSPACE_DIR={ROOT}/workspace with spaces",
            "--name", "last", str(ROOT / "bin" / "../sbx-kit"),
            "workspace with spaces", "second workspace", "--path", str(pi),
            str(STATE_DIR),
        ]])


if __name__ == "__main__":
    unittest.main()
