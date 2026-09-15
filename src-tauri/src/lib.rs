use std::io::{BufRead, BufReader};
#[cfg(target_os = "windows")]
use std::os::windows::process::CommandExt;
use std::process::{Child, Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
use tauri::{Manager, WebviewUrl, WebviewWindowBuilder};

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
            setup_desktop(app)?;

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

/// 解析 sidecar stderr
fn split_error(raw: &str) -> (String, String) {
    let mut title = "后端启动失败".to_string();
    let mut body: Vec<&str> = Vec::new();
    for line in raw.lines() {
        if let Some(t) = line.strip_prefix("SEREIN_ERROR=") {
            title = t.trim().to_string();
        } else {
            body.push(line);
        }
    }
    let msg = body.join("\n");
    let msg = msg.trim();
    let msg = if msg.is_empty() {
        "请检查配置或重启应用".to_string()
    } else {
        msg.to_string()
    };
    (title, msg)
}

/// 桌面端：先建初始窗口，再拉起 Go sidecar，等通过 stdout 报告监听地址后导航
#[cfg(not(mobile))]
fn setup_desktop(app: &tauri::App) -> Result<(), Box<dyn std::error::Error>> {
    // 初始页用自包含的 data: 空白页
    let blank: tauri::Url =
        "data:text/html,%3C!doctype%20html%3E%3Cmeta%20charset=utf-8%3E".parse()?;
    let win = WebviewWindowBuilder::new(app, "main", WebviewUrl::External(blank))
        .title("Serein")
        .inner_size(1200.0, 800.0)
        .min_inner_size(1000.0, 600.0)
        .center()
        .resizable(true)
        .fullscreen(false)
        .build()?;

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

    let bin = match bin {
        Some(b) => b,
        None => {
            error_page::show(
                &win,
                "找不到后端程序",
                "Serein 后端 sidecar 缺失，请重新安装",
            );
            return Ok(());
        }
    };

    let handle = app.handle().clone();
    std::thread::spawn(move || {
        let mut crashes: Vec<Instant> = Vec::new();
        loop {
            let now = Instant::now();
            crashes.retain(|t| now.duration_since(*t) < Duration::from_secs(10));
            if crashes.len() >= 3 {
                break;
            }

            let mut cmd = Command::new(&bin);
            cmd.env("SEREIN_SIDECAR", "1");
            cmd.stdout(Stdio::piped()).stderr(Stdio::piped());
            #[cfg(target_os = "windows")]
            {
                cmd.creation_flags(0x08000000);
            }

            let mut child = match cmd.spawn() {
                Ok(c) => c,
                Err(e) => {
                    if let Some(w) = handle.get_webview_window("main") {
                        error_page::show(&w, "无法启动后端", &e.to_string());
                    }
                    break;
                }
            };

            let stdout = child.stdout.take();
            let stderr = child.stderr.take();
            let child = Arc::new(Mutex::new(child));
            *SIDECAR.lock().unwrap() = Some(child.clone());

            // 读 stdout，等待 sidecar 报告监听地址后导航窗口
            if let Some(out) = stdout {
                let h = handle.clone();
                std::thread::spawn(move || {
                    let mut navigated = false;
                    for line in BufReader::new(out).lines().map_while(Result::ok) {
                        if navigated {
                            continue;
                        }
                        if let Some(addr) = line.strip_prefix("SEREIN_ADDR=") {
                            if let Some(w) = h.get_webview_window("main") {
                                if let Ok(url) =
                                    tauri::Url::parse(&format!("http://{}", addr.trim()))
                                {
                                    let _ = w.navigate(url);
                                }
                            }
                            navigated = true;
                        }
                    }
                });
            }

            // 收集 stderr，启动失败时展示原因
            let errbuf = Arc::new(Mutex::new(String::new()));
            let stderr_handle = if let Some(err) = stderr {
                let eb = errbuf.clone();
                Some(std::thread::spawn(move || {
                    for line in BufReader::new(err).lines().map_while(Result::ok) {
                        let mut b = eb.lock().unwrap();
                        b.push_str(&line);
                        b.push('\n');
                    }
                }))
            } else {
                None
            };

            // 等待 sidecar 退出
            loop {
                let exited = child.lock().unwrap().try_wait().ok().flatten().is_some();
                if exited {
                    break;
                }
                std::thread::sleep(Duration::from_millis(200));
            }

            // 等 stderr 读完，避免竞态导致错误信息为空
            if let Some(h) = stderr_handle {
                let _ = h.join();
            }

            if SHUTTING_DOWN.load(Ordering::Relaxed) {
                break;
            }
            crashes.push(Instant::now());

            let raw = errbuf.lock().unwrap().clone();
            let (title, msg) = split_error(&raw);
            if let Some(w) = handle.get_webview_window("main") {
                error_page::show(&w, &title, &msg);
            }
            if raw.contains("SEREIN_ERROR=") {
                break;
            }
            std::thread::sleep(Duration::from_millis(500));
        }
    });

    Ok(())
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
