#!/usr/bin/env python3
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

OUTPUT = Path(__file__).resolve().parent / "openvsx.toml"
DISPLAY_NAME = "VSIX: OpenVSX"
ORDER = 99
TYPE = "openvsx"

VERSIONED = re.compile(r"^(?P<id>.+?)-(?P<version>\d+\.\d+\.\d+(?:[-+].*)?)$")


def cli_candidates():
    env = os.environ.get("VSCODIUM_BIN")
    if env:
        yield env
    found = shutil.which("codium")
    if found:
        yield found
    home = Path.home()
    if sys.platform == "darwin":
        yield "/Applications/VSCodium.app/Contents/Resources/app/bin/codium"
        yield str(home / "Applications/VSCodium.app/Contents/Resources/app/bin/codium")
    elif sys.platform.startswith("win"):
        for base in (
            os.environ.get("LOCALAPPDATA"),
            os.environ.get("PROGRAMFILES"),
            os.environ.get("PROGRAMFILES(X86)"),
        ):
            if not base:
                continue
            yield str(Path(base) / "Programs" / "VSCodium" / "bin" / "codium.cmd")
            yield str(Path(base) / "VSCodium" / "bin" / "codium.cmd")
    else:
        yield "/usr/bin/codium"
        yield "/usr/share/codium/bin/codium"
        yield "/snap/bin/codium"
        yield "/var/lib/flatpak/exports/bin/com.vscodium.codium"


def from_cli(binary):
    try:
        result = subprocess.run(
            [binary, "--list-extensions"],
            capture_output=True,
            text=True,
            timeout=60,
        )
    except (OSError, subprocess.SubprocessError):
        return None
    if result.returncode != 0:
        return None
    ids = [line.strip() for line in result.stdout.splitlines() if line.strip()]
    return ids or None


def extension_dirs():
    env = os.environ.get("VSCODE_EXTENSIONS")
    if env:
        yield Path(env)
    home = Path.home()
    yield home / ".vscode-oss" / "extensions"


def from_dirs():
    for directory in extension_dirs():
        if not directory.is_dir():
            continue
        ids = []
        for entry in directory.iterdir():
            if not entry.is_dir():
                continue
            match = VERSIONED.match(entry.name)
            if match:
                ids.append(match.group("id"))
        if ids:
            return ids
    return None


def read_ids():
    for binary in cli_candidates():
        if Path(binary).exists():
            ids = from_cli(binary)
            if ids:
                return ids
    return from_dirs()


def toml_string(value):
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def main():
    ids = read_ids()
    if not ids:
        print("No VSCodium extensions found", file=sys.stderr)
        return 1

    ids = sorted(set(ids), key=str.lower)
    lines = [
        "# Generated automatically by scripts/openvsx-tracker.py",
        f"display_name = {toml_string(DISPLAY_NAME)}",
        f"order = {ORDER}",
        f"type = {toml_string(TYPE)}",
        "",
        "[tracker]",
        "app_id = [",
    ]
    lines.extend(f"  {toml_string(item)}," for item in ids)
    lines.append("]")
    OUTPUT.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"Wrote {len(ids)} extensions to {OUTPUT}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
