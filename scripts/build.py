import subprocess
import os

TARGETS = [
    ("darwin", "arm64"),
    ("darwin", "amd64"),
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("windows", "amd64"),
]


def get_version():
    return subprocess.check_output(["git", "describe", "--tags", "--abbrev=0"], text=True).strip()


def main():
    version = get_version()
    os.makedirs("./build", exist_ok=True)

    for goos, goarch in TARGETS:
        ext = ".exe" if goos == "windows" else ""
        out = f"./build/serein-{version}-{goos}-{goarch}{ext}"
        env = {**os.environ, "GOOS": goos, "GOARCH": goarch}
        args = ["go", "build", "-ldflags=-s -w", "-o", out]
        subprocess.run(args + ["."], env=env, check=True)
        print(f"OK  {goos}/{goarch}  →  {out}")


if __name__ == "__main__":
    main()
