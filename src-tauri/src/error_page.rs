fn esc(s: &str) -> String {
    s.replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
}

fn data_url(html: &str) -> String {
    let mut out = String::with_capacity(html.len() * 2);
    for b in html.bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(b as char)
            }
            _ => out.push_str(&format!("%{:02X}", b)),
        }
    }
    format!("data:text/html;charset=utf-8,{out}")
}

/// 用一个自包含的 data: 页面展示错误信息。
pub fn show(window: &tauri::WebviewWindow, title: &str, message: &str) {
    let _ = window.set_title(&format!("Serein — {title}"));
    let html = format!(
        "<!doctype html><html lang=zh-CN><head><meta charset=utf-8><title>{t}</title><style>body{{font-family:system-ui;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;background:#1d1d1d;color:rgba(255,255,255,.88)}}h2{{color:#ef5350;margin:0 0 8px}}p{{color:rgba(255,255,255,.52);margin:0;white-space:pre-wrap}}</style></head><body><div style=text-align:center;max-width:400px><h2>⚠ {t}</h2><p>{m}</p></div></body></html>",
        t = esc(title),
        m = esc(message)
    );
    if let Ok(u) = tauri::Url::parse(&data_url(&html)) {
        let _ = window.navigate(u);
    }
}
