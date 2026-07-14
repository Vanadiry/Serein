import errno
import os
import shutil
import stat
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

CLEAN = [
    "build",
    "src-tauri/target",
    "src-tauri/gen/schemas",
    "web/*.html",
    "web/assets/app.min.js",
]


def on_error(_func, path, exc):
    if isinstance(exc, OSError) and exc.errno == errno.ENOTEMPTY:
        shutil.rmtree(path, onexc=on_error)
        return
    os.chmod(path, stat.S_IWRITE)
    try:
        _func(path)
    except OSError:
        shutil.rmtree(path, onexc=on_error)


def main():
    for pattern in CLEAN:
        for p in sorted(ROOT.glob(pattern)):
            if p.is_dir():
                shutil.rmtree(p, onexc=on_error)
            else:
                p.unlink()
            print(f"OK. removed {p}")


if __name__ == "__main__":
    main()
