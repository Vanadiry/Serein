pub fn show(window: &tauri::WebviewWindow, title: &str, message: &str) {
    let html = format!(
        r#"document.open();document.write('<!doctype html><html lang=zh-CN><head><meta charset=utf-8><title>{title}</title><style>body{{font-family:system-ui;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;background:#1d1d1d;color:rgba(255,255,255,.88)}}h2{{color:#ef5350;margin:0 0 8px}}p{{color:rgba(255,255,255,.52);margin:0}}</style></head><body><div style=text-align:center;max-width:400px><h2>⚠ {title}</h2><p>{message}</p></div></body></html>');document.close();"#,
        title = title.replace('\'', "\\'"),
        message = message.replace('\'', "\\'")
    );
    let _ = window.eval(&html);
}
