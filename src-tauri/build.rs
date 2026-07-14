use std::process::Command;

fn main() {
    let go_root = std::env::current_dir()
        .unwrap()
        .parent()
        .unwrap()
        .to_path_buf();
    let out_dir = go_root.join("build");
    std::fs::create_dir_all(&out_dir).ok();

    let target = std::env::var("TARGET").unwrap_or_default();
    let is_mobile = target.contains("android") || target.contains("ios");

    if !is_mobile {
        // Desktop: Go sidecar binary — build before tauri_build validates externalBin
        let ext = if cfg!(target_os = "windows") {
            ".exe"
        } else {
            ""
        };
        let bin_name = format!("serein_server-{}{}", target, ext);
        let out = out_dir.join(&bin_name);
        let status = Command::new("go")
            .args(["build", "-ldflags=-s -w", "-o", out.to_str().unwrap(), "."])
            .current_dir(&go_root)
            .status()
            .expect("failed to run go build");
        if !status.success() {
            panic!("go build failed for sidecar");
        }
    }

    tauri_build::build();

    if is_mobile {
        // FFI mode: compile Go as static library
        let lib_name = if target.contains("windows") {
            "serein.lib"
        } else {
            "libserein.a"
        };
        let out = out_dir.join(lib_name);

        let goos = if target.contains("linux") {
            "linux"
        } else if target.contains("darwin") || target.contains("ios") {
            "darwin"
        } else {
            "windows"
        };
        let goarch = if target.contains("aarch64") || target.contains("arm64") {
            "arm64"
        } else {
            "amd64"
        };

        let status = Command::new("go")
            .args([
                "build",
                "-tags",
                "ffi",
                "-buildmode=c-archive",
                "-o",
                out.to_str().unwrap(),
                ".",
            ])
            .env("CGO_ENABLED", "1")
            .env("GOOS", goos)
            .env("GOARCH", goarch)
            .current_dir(&go_root)
            .status()
            .expect("failed to run go build");

        if !status.success() {
            panic!("go build failed for {goos}/{goarch}");
        }

        println!("cargo:rustc-link-search=native={}", out_dir.display());
        println!("cargo:rustc-link-lib=static=serein");
        if cfg!(target_os = "macos") {
            println!("cargo:rustc-link-lib=framework=CoreFoundation");
            println!("cargo:rustc-link-lib=framework=Security");
        }
    }
}
