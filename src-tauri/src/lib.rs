use std::net::TcpStream;
#[cfg(target_os = "windows")]
use std::os::windows::process::CommandExt;
use std::process::{Child, Command};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
use tauri::Manager;

mod config;
mod error_page;

static SIDECAR: Mutex<Option<Arc<Mutex<Child>>>> = Mutex::new(None);
static SHUTTING_DOWN: AtomicBool = AtomicBool::new(false);

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .setup(|app| {
            #[cfg(mobile)]
            unsafe {
                ffi::StartSerein();
            }

            #[cfg(not(mobile))]
            {
                let port = config::get_port();
                let addr = format!("127.0.0.1:{}", port);
                let ext = if cfg!(target_os = "windows") {
                    ".exe"
                } else {
                    ""
                };
                let target = std::env::var("TARGET").unwrap_or_default();
                let name_long = format!("serein_server-{}{}", target, ext);
                let name_short = format!("serein_server{}", ext);

                // 1) bundled: next to the main executable
                // 2) dev: project_root/build/
                let bin = std::env::current_exe()
                    .ok()
                    .and_then(|p| p.parent().map(|d| d.to_path_buf()))
                    .and_then(|exe_dir| {
                        let bundled = exe_dir.join(&name_short);
                        if bundled.exists() {
                            return Some(bundled);
                        }
                        let bundled_long = exe_dir.join(&name_long);
                        if bundled_long.exists() {
                            return Some(bundled_long);
                        }
                        None
                    })
                    .or_else(|| {
                        let dev = std::env::current_dir()
                            .ok()?
                            .parent()?
                            .join("build")
                            .join(&name_long);
                        if dev.exists() {
                            Some(dev)
                        } else {
                            None
                        }
                    });
                if TcpStream::connect(&addr).is_ok() {
                    error_page::show(
                        &app.get_webview_window("main").unwrap(),
                        "无法启动后端",
                        "Serein 后端使用的端口被占用，请检查",
                    );
                } else if let Some(bin_path) = bin {
                    let bin_path = Arc::new(bin_path);
                    let handle = app.handle().clone();
                    let port = port;
                    std::thread::spawn(move || {
                        let mut crashes: Vec<Instant> = Vec::new();
                        loop {
                            let now = Instant::now();
                            crashes.retain(|t| now.duration_since(*t) < Duration::from_secs(10));
                            if crashes.len() >= 3 {
                                if let Some(window) = handle.get_webview_window("main") {
                                    error_page::show(
                                        &window,
                                        "后端反复崩溃",
                                        "Serein 后端进程多次启动失败，请检查配置或重启应用",
                                    );
                                }
                                break;
                            }

                            let mut cmd = Command::new(bin_path.as_ref());
                            cmd.env("SEREIN_SIDECAR", "1");
                            #[cfg(target_os = "windows")]
                            {
                                cmd.creation_flags(0x08000000);
                            }
                            if let Ok(child) = cmd.spawn() {
                                let child = Arc::new(Mutex::new(child));
                                let c = child.clone();
                                *SIDECAR.lock().unwrap() = Some(child);

                                for _ in 0..30 {
                                    std::thread::sleep(Duration::from_millis(100));
                                    if TcpStream::connect(format!("127.0.0.1:{}", port)).is_ok() {
                                        break;
                                    }
                                }
                                if let Some(w) = handle.get_webview_window("main") {
                                    let url = format!("http://127.0.0.1:{}", port);
                                    let _ = w.eval(&format!("location.replace('{}')", url));
                                }

                                loop {
                                    let exited =
                                        c.lock().unwrap().try_wait().ok().flatten().is_some();
                                    if exited {
                                        break;
                                    }
                                    std::thread::sleep(Duration::from_millis(200));
                                }
                            }

                            if SHUTTING_DOWN.load(Ordering::Relaxed) {
                                break;
                            }
                            crashes.push(Instant::now());
                            std::thread::sleep(Duration::from_millis(500));
                        }
                    });
                    for _ in 0..30 {
                        std::thread::sleep(Duration::from_millis(100));
                        if TcpStream::connect(&addr).is_ok() {
                            break;
                        }
                    }
                }
            }

            setup_menu(app)?;
            Ok(())
        })
        .on_window_event(|win, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                api.prevent_close();
                graceful_exit(win.app_handle().clone());
            }
        })
        .build(tauri::generate_context!())
        .expect("error while running tauri application")
        .run(|_handle, _event| {
            if matches!(_event, tauri::RunEvent::Exit) {
                graceful_exit(_handle.clone());
            }
        });
}

fn graceful_exit(handle: tauri::AppHandle) {
    SHUTTING_DOWN.store(true, Ordering::Relaxed);
    if let Some(w) = handle.get_webview_window("main") {
        error_page::show(&w, "正在退出", "等待后端进程结束...");
    }
    #[cfg(not(mobile))]
    if let Some(child) = SIDECAR.lock().unwrap().take() {
        let _ = child.lock().unwrap().kill();
        // Wait in background, then exit
        std::thread::spawn(move || {
            let _ = child.lock().unwrap().wait();
            handle.exit(0);
        });
    } else {
        handle.exit(0);
    }
    #[cfg(mobile)]
    {
        unsafe {
            ffi::StopSerein();
        }
        handle.exit(0);
    }
}

fn setup_menu(app: &tauri::App) -> Result<(), Box<dyn std::error::Error>> {
    use tauri::menu::{MenuBuilder, PredefinedMenuItem, SubmenuBuilder};

    let app_menu = SubmenuBuilder::new(app, "Serein")
        .item(&PredefinedMenuItem::about(app, Some("关于 Serein"), None)?)
        .separator()
        .item(&PredefinedMenuItem::hide(app, Some("隐藏 Serein"))?)
        .item(&PredefinedMenuItem::hide_others(app, Some("隐藏其他"))?)
        .item(&PredefinedMenuItem::show_all(app, Some("全部显示"))?)
        .separator()
        .item(&PredefinedMenuItem::quit(app, Some("退出 Serein"))?)
        .build()?;

    let edit_menu = SubmenuBuilder::new(app, "编辑")
        .item(&PredefinedMenuItem::undo(app, Some("撤销"))?)
        .item(&PredefinedMenuItem::redo(app, Some("重做"))?)
        .separator()
        .item(&PredefinedMenuItem::cut(app, Some("剪切"))?)
        .item(&PredefinedMenuItem::copy(app, Some("复制"))?)
        .item(&PredefinedMenuItem::paste(app, Some("粘贴"))?)
        .item(&PredefinedMenuItem::select_all(app, Some("全选"))?)
        .build()?;

    let menu = MenuBuilder::new(app)
        .item(&app_menu)
        .item(&edit_menu)
        .build()?;

    app.set_menu(menu)?;
    Ok(())
}

#[cfg(mobile)]
mod ffi {
    extern "C" {
        pub fn StartSerein() -> i32;
        pub fn StopSerein();
    }
}
