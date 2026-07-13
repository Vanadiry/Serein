import shutil
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

CLEAN = [
    "build",
    "src-tauri/target",
    "src-tauri/gen/schemas",
    "web/*.html",
    "web/assets/app.min.js",
]

def on_error(func, path, exc_info):
    import os, stat, errno
    ex = exc_info[1]
    if isinstance(ex, OSError) and ex.errno == errno.ENOTEMPTY:
        shutil.rmtree(path, onexc=on_error)
    else:
        os.chmod(path, stat.S_IWRITE)
        try:
            func(path)
        except Exception:
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
