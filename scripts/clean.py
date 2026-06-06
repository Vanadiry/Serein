import shutil
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

CLEAN = [
    "build",
    "web/*.html",
    "web/assets/app.min.js"
]


def main():
    for pattern in CLEAN:
        for p in sorted(ROOT.glob(pattern)):
            if p.is_dir():
                shutil.rmtree(p)
            else:
                p.unlink()
            print(f"OK. removed {p}")


if __name__ == "__main__":
    main()
