"""Select the legacy launcher or one freshly built Go acceptance candidate."""

import atexit
from functools import cache
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile


SOURCE_ROOT = Path(__file__).resolve().parents[1]


def implementation():
    selected = os.environ.get("PISSBOX_TEST_LAUNCHER", "bash")
    if selected not in ("bash", "go"):
        raise ValueError("PISSBOX_TEST_LAUNCHER must be bash or go")
    return selected


def binary_name():
    arch = {"aarch64": "arm64", "arm64": "arm64", "x86_64": "amd64", "AMD64": "amd64"}[platform.machine()]
    return f"pissbox-{platform.system().lower()}-{arch}"


@cache
def fixture_checkout(selected):
    fixtures = SOURCE_ROOT / ".cache/test-fixtures"
    fixtures.mkdir(parents=True, exist_ok=True)
    directory = tempfile.TemporaryDirectory(prefix=f"{selected}-launcher-", dir=fixtures)
    atexit.register(directory.cleanup)
    checkout = Path(directory.name)
    if selected == "go":
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
    shutil.copytree(SOURCE_ROOT / "bin/lib", checkout / "bin/lib")
    pi = checkout / "pi-agent"
    pi.mkdir()
    for name in ("settings-co.json", "settings-go.json"):
        shutil.copy2(SOURCE_ROOT / "pi-agent" / name, pi / name)
    shutil.copytree(SOURCE_ROOT / "pi-agent/extensions", pi / "extensions")
    if selected == "go":
        (checkout / "bin/pissbox").symlink_to(Path("../.cache/bin") / binary.name)
    else:
        shutil.copy2(SOURCE_ROOT / "bin/pissbox", checkout / "bin/pissbox")
    return checkout


def candidate_checkout():
    return fixture_checkout("go")


def checkout_root():
    # Neither implementation should consume the developer checkout's config.
    return candidate_checkout() if implementation() == "go" else fixture_checkout("bash")


def materialize_launcher(checkout):
    """Install the selected launcher into an acceptance test's fake checkout."""
    checkout = Path(checkout)
    launcher = checkout / "bin/pissbox"
    launcher.parent.mkdir(parents=True, exist_ok=True)
    if implementation() == "bash":
        shutil.copy2(SOURCE_ROOT / "bin/pissbox", launcher)
    else:
        binary = checkout / ".cache/bin" / binary_name()
        binary.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(candidate_checkout() / ".cache/bin" / binary.name, binary)
        launcher.symlink_to(Path("../.cache/bin") / binary.name)
    return launcher
