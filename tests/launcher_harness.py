"""Build one fresh Go launcher and use public fixture resources for acceptance tests."""

import atexit
from functools import cache
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile


SOURCE_ROOT = Path(__file__).resolve().parents[1]


def binary_name():
    arch = {"aarch64": "arm64", "arm64": "arm64", "x86_64": "amd64", "AMD64": "amd64"}[platform.machine()]
    return f"pissbox-{platform.system().lower()}-{arch}"


@cache
def checkout_root():
    """Build once per test run, without using installed artifacts or private state."""
    fixtures = SOURCE_ROOT / ".cache/test-fixtures"
    fixtures.mkdir(parents=True, exist_ok=True)
    directory = tempfile.TemporaryDirectory(prefix="go-launcher-", dir=fixtures)
    atexit.register(directory.cleanup)
    checkout = Path(directory.name)
    binary = checkout / ".cache/bin" / binary_name()
    binary.parent.mkdir(parents=True)
    # Build native regardless of a caller's cross-build settings.
    env = {key: value for key, value in os.environ.items() if key not in ("GOOS", "GOARCH")}
    env["CGO_ENABLED"] = "0"
    subprocess.run(
        ["go", "build", "-trimpath", "-o", str(binary), "./cmd/pissbox"],
        cwd=SOURCE_ROOT / "host", env=env, check=True, capture_output=True, text=True, timeout=120,
    )
    # Copy only public fixtures/resources, never credentials or generated Pi state.
    shutil.copytree(SOURCE_ROOT / "sbx-kit", checkout / "sbx-kit")
    shutil.copytree(SOURCE_ROOT / "tests/fixtures", checkout / "tests/fixtures", symlinks=True)
    pi = checkout / "pi-agent"
    pi.mkdir()
    for name in ("settings-co.json", "settings-go.json"):
        shutil.copy2(SOURCE_ROOT / "pi-agent" / name, pi / name)
    shutil.copytree(SOURCE_ROOT / "pi-agent/extensions", pi / "extensions")
    (checkout / "bin").mkdir()
    shutil.copy2(SOURCE_ROOT / "bin/pissbox", checkout / "bin/pissbox")
    return checkout


def materialize_launcher(checkout):
    """Copy the fresh binary and production wrapper into another fake checkout."""
    checkout = Path(checkout)
    launcher = checkout / "bin/pissbox"
    launcher.parent.mkdir(parents=True, exist_ok=True)
    binary = checkout / ".cache/bin" / binary_name()
    binary.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(checkout_root() / ".cache/bin" / binary.name, binary)
    shutil.copy2(SOURCE_ROOT / "bin/pissbox", launcher)
    return launcher
