#!/usr/bin/env python3
"""Select source files and run the repository's pinned quality tools."""

import argparse
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
TOOLS = ROOT / ".cache/quality-tools"
GREMLINS = "github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0"
GO_CRAP = "github.com/padiazg/go-crap@v0.5.1"


def run(args, *, cwd=ROOT, **kwargs):
    return subprocess.run([str(arg) for arg in args], cwd=cwd, check=True, **kwargs)


def git(*args):
    return run(["git", *args], stdout=subprocess.PIPE).stdout


def source_files(language):
    directory = ROOT / ("host" if language == "go" else "pi-agent/extensions")
    suffix = ".go" if language == "go" else ".js"
    files = []
    for path in directory.rglob(f"*{suffix}"):
        relative = path.relative_to(ROOT)
        if any(part in {"vendor", "node_modules", "testdata"} for part in relative.parts):
            continue
        if path.name.endswith(("_test.go", ".test.js", ".spec.js")):
            continue
        if path.is_symlink() or not path.is_file():
            continue
        # Standard Go generated-code marker; never mutate generated output.
        if re.search(rb"(?m)^// Code generated .* DO NOT EDIT\.$", path.read_bytes()):
            continue
        files.append(relative.as_posix())
    return sorted(files)


def select_files(args, language):
    supported = set(source_files(language))
    explicit = args.files or shlex.split(os.environ.get("QUALITY_FILES", ""))
    base = args.base or os.environ.get("QUALITY_BASE", "")
    if args.all and (explicit or base):
        raise ValueError("--all cannot be combined with FILES or BASE")
    if explicit:
        candidates = []
        for name in explicit:
            path = ROOT / name
            if path.is_symlink() or not path.is_file():
                raise ValueError(f"Not a regular source file: {name}")
            try:
                candidates.append(path.resolve().relative_to(ROOT).as_posix())
            except ValueError:
                raise ValueError(f"File is outside this checkout: {name}") from None
        known = set(source_files("go")) | set(source_files("js"))
        unsupported = set(candidates) - known
        if unsupported:
            raise ValueError(f"Unsupported source files: {', '.join(sorted(unsupported))}")
    elif base:
        merge_base = git("merge-base", "HEAD", base).decode().strip()
        # Comparing with the working tree includes committed, staged and unstaged
        # changes. --no-renames represents renames as deletion + destination add.
        names = git("diff", "--name-only", "-z", "--no-renames", "--diff-filter=ACMT", merge_base, "--")
        names += git("ls-files", "--others", "--exclude-standard", "-z")
        candidates = [os.fsdecode(name) for name in names.split(b"\0") if name]
    elif args.all or args.command.startswith("crap-"):
        candidates = supported
    else:
        raise ValueError("Choose BASE=<ref> or FILES='path ...'; use make mutation-all for a full scan")
    return sorted(set(candidates) & supported)


def install_tools():
    TOOLS.mkdir(parents=True, exist_ok=True)
    js = TOOLS / "js"
    js.mkdir(exist_ok=True)
    for name in ("package.json", "package-lock.json"):
        shutil.copyfile(ROOT / "tools/quality" / name, js / name)
    run(["npm", "ci", "--ignore-scripts", "--prefix", js])
    bin_dir = go_bin_dir()
    bin_dir.mkdir(parents=True, exist_ok=True)
    env = {**os.environ, "GOBIN": str(bin_dir)}
    for tool in (GREMLINS, GO_CRAP):
        run(["go", "install", tool], env=env)


def go_bin_dir():
    host = run(["go", "env", "GOHOSTOS", "GOHOSTARCH"], stdout=subprocess.PIPE, text=True).stdout.split()
    return TOOLS / "bin" / "-".join(host)


def require_tool(path):
    if not path.is_file():
        raise ValueError(f"Missing {path.relative_to(ROOT)}; run make tools first")
    return path


def exclusions(selected):
    # Gremlins has exclusion regexes, not inclusion globs. Exclude the exact
    # complement, relative to the Go module, with RE2-compatible literal escapes.
    return ["--exclude-files=" + "^" + re.escape(path.relative_to(ROOT / "host").as_posix()) + "$"
            for path in sorted((ROOT / "host").rglob("*.go"))
            if path.relative_to(ROOT).as_posix() not in selected]


def go_coverage():
    output = ROOT / ".cache/coverage/go"
    output.mkdir(parents=True, exist_ok=True)
    profile = output / "coverage.out"
    run(["go", "test", "-race", "-count=1", "-coverprofile", profile, "./..."], cwd=ROOT / "host")
    run(["go", "tool", "cover", "-html", profile, "-o", output / "index.html"], cwd=ROOT / "host")
    return profile


def mutation_go(args, selected):
    binary = require_tool(go_bin_dir() / "gremlins")
    output = ROOT / ".cache/mutation/go"
    output.mkdir(parents=True, exist_ok=True)
    temporary = output / "tmp"
    temporary.mkdir(exist_ok=True)
    command = [binary, "unleash", "--integration", "--workers=1", "--output", output / "report.json", *exclusions(selected)]
    if args.dry_run:
        command.append("--dry-run")
    # Disable Go's result cache even when mutants only change subprocess paths.
    env = {
        **os.environ,
        "GOFLAGS": (os.environ.get("GOFLAGS", "") + " -count=1").strip(),
        "TMPDIR": str(temporary),
        # Copies live too deep for Unix socket paths. Use the real checkout's
        # unique test fixtures, never the user's actual daemon runtime directory.
        "PISSBOX_BRIDGE_TEST_FIXTURE_DIR": os.environ.get("PISSBOX_BRIDGE_TEST_FIXTURE_DIR") or str(ROOT / ".cache/test-fixtures"),
    }
    run(command, cwd=ROOT / "host", env=env)


def crap_go(selected):
    binary = require_tool(go_bin_dir() / "go-crap")
    profile = go_coverage()
    output = ROOT / ".cache/crap/go"
    output.mkdir(parents=True, exist_ok=True)
    # go-crap matches absolute file paths, unlike Gremlins.
    excluded = []
    for path in sorted((ROOT / "host").rglob("*.go")):
        if path.relative_to(ROOT).as_posix() not in selected:
            excluded.extend(["--exclude", "^" + re.escape(str(path)) + "$"])
    command = [binary, "scan", "./...", "--coverage-profile", profile, "--no-progress", *excluded]
    run([*command, "--format=json", "--output", output / "report.json"], cwd=ROOT / "host")
    run(command, cwd=ROOT / "host")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["tools", "mutation-go", "mutation-js", "crap-go", "crap-js", "coverage-go", "coverage-js"])
    parser.add_argument("--base", help="Compare the working tree with the merge base of HEAD and this ref")
    parser.add_argument("--all", action="store_true", help="Deliberately select every supported source file")
    parser.add_argument("--list", action="store_true", help="Print selection without invoking tools")
    parser.add_argument("--dry-run", action="store_true", help="Discover mutants and run baseline tests without testing mutants")
    parser.add_argument("files", nargs="*", help="Exact checkout-relative source files (override BASE)")
    args = parser.parse_intermixed_args()
    args.dry_run |= os.environ.get("QUALITY_DRY_RUN", "") == "1"
    if args.command == "tools":
        install_tools()
        return
    if args.command == "coverage-go":
        go_coverage()
        return
    if args.command == "coverage-js":
        run(["node", ROOT / "tools/quality/javascript.mjs", "coverage"])
        return
    language = args.command.rsplit("-", 1)[1]
    selected = select_files(args, language)
    if args.list:
        print(json.dumps(selected))
        return
    if not selected:
        print(f"No matching {language} source changes; nothing to run (old reports, if any, are unchanged).")
        return
    print("Selected source files:\n" + "\n".join(f"  {name}" for name in selected), flush=True)
    if language == "js":
        command = ["node", ROOT / "tools/quality/javascript.mjs", args.command.split("-")[0]]
        if args.dry_run:
            command.append("--dry-run")
        run([*command, *selected])
    elif args.command == "mutation-go":
        mutation_go(args, selected)
    else:
        crap_go(selected)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, FileNotFoundError) as error:
        sys.exit(str(error))
    except subprocess.CalledProcessError as error:
        sys.exit(error.returncode)
