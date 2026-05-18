import subprocess
import os


def main():
    os.makedirs("./build", exist_ok=True)
    subprocess.run(["pnpm", "run", "build"], check=True, cwd=".")

    out = "./build/serein_dev"
    args = ["go", "build", "-o", out]
    subprocess.run(args + ["."], check=True)
    print(f"OK. {out}")

    subprocess.run([out], check=True)


if __name__ == "__main__":
    main()
