import subprocess
import os
import sys
import shutil
import json
import glob as _glob
import platform as _platform
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def build_frontend():
    subprocess.run(["pnpm", "run", "build"], check=True, cwd=ROOT)


def get_version():
    conf = json.loads((ROOT / "src-tauri" / "tauri.conf.json").read_text())
    return conf["version"]


MACHINE = _platform.machine()
SYSTEM = _platform.system()
TARGET = {
    "Darwin": "aarch64-apple-darwin" if MACHINE == "arm64" else "x86_64-apple-darwin",
    "Linux": "x86_64-unknown-linux-gnu",
    "Windows": "x86_64-pc-windows-msvc",
}.get(SYSTEM, "unknown")

PLATFORM = {"Darwin": "macos", "Linux": "linux", "Windows": "windows"}.get(
    SYSTEM, "unknown"
)
ARCH = "arm64" if MACHINE == "arm64" else "amd64"
EXT = ".exe" if SYSTEM == "Windows" else ""
VERSION = get_version()

SERVER_TARGETS = [
    ("macos", "darwin", "arm64"),
    ("macos", "darwin", "amd64"),
    ("linux", "linux", "amd64"),
    ("linux", "linux", "arm64"),
    ("windows", "windows", "amd64"),
]


def cmd_server():
    """Cross-compile Server for all platforms → build/server/"""
    build_frontend()
    out_dir = ROOT / "build" / "server"
    out_dir.mkdir(parents=True, exist_ok=True)
    for label, goos, goarch in SERVER_TARGETS:
        ext = ".exe" if goos == "windows" else ""
        name = f"serein-server-{VERSION}-{label}-{goarch}{ext}"
        out = out_dir / name
        env = {**os.environ, "GOOS": goos, "GOARCH": goarch}
        subprocess.run(
            ["go", "build", "-ldflags=-s -w", "-o", str(out), "."],
            cwd=ROOT,
            env=env,
            check=True,
        )
        print(f"OK. {name}")


def copy_current_server():
    """Copy sidecar as current-platform Server → build/server/"""
    sidecar = ROOT / "build" / f"serein_server-{TARGET}{EXT}"
    server_dir = ROOT / "build" / "server"
    server_dir.mkdir(parents=True, exist_ok=True)
    if sidecar.exists():
        name = f"serein-server-{VERSION}-{PLATFORM}-{ARCH}{EXT}"
        shutil.copy(sidecar, server_dir / name)
        print(f"OK. server → {name}")


def collect_desktop():
    """Collect Desktop bundles → build/desktop/"""
    desk_dir = ROOT / "build" / "desktop"
    if desk_dir.exists():
        shutil.rmtree(desk_dir)
    desk_dir.mkdir(parents=True, exist_ok=True)
    bundle = ROOT / "src-tauri" / "target" / "release" / "bundle"
    for pat, ext in [
        ("dmg/*.dmg", "dmg"),
        ("nsis/*.exe", "exe"),
        ("appimage/*.AppImage", "AppImage"),
    ]:
        matches = _glob.glob(str(bundle / pat))
        if matches:
            target = desk_dir / f"serein-desktop-{VERSION}-{PLATFORM}-{ARCH}.{ext}"
            shutil.copy(matches[0], target)
            print(f"OK. desktop → {target.name}")


def cmd_desktop():
    """Build Desktop + current-platform Server"""
    build_frontend()
    subprocess.run(["pnpm", "tauri", "build"], cwd=ROOT, check=True)
    copy_current_server()
    collect_desktop()


def cmd_dev():
    """Build frontend + Go sidecar, run Tauri dev"""
    build_frontend()
    go_out = ROOT / "build" / f"serein_server-{TARGET}{EXT}"
    subprocess.run(
        ["go", "build", "-ldflags=-s -w", "-o", str(go_out), "."], cwd=ROOT, check=True
    )
    print(f"OK. {go_out}")
    subprocess.run(["pnpm", "tauri", "dev"], cwd=ROOT, check=True)


def cmd_sdev():
    """Build frontend + Server binary, then run"""
    build_frontend()
    go_out = ROOT / "build" / "serein_dev"
    go_out.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(["go", "build", "-o", str(go_out), "."], cwd=ROOT, check=True)
    print(f"OK. {go_out}")
    subprocess.run([str(go_out)], cwd=ROOT)


COMMANDS = {
    "server": cmd_server,
    "desktop": cmd_desktop,
    "dev": cmd_dev,
    "sdev": cmd_sdev,
}


def main():
    if len(sys.argv) < 2 or sys.argv[1] not in COMMANDS:
        print("Usage: python3 scripts/build.py <command>")
        print("Commands: " + ", ".join(COMMANDS.keys()))
        sys.exit(1)
    COMMANDS[sys.argv[1]]()


if __name__ == "__main__":
    main()
