use serde::Deserialize;
use std::path::PathBuf;

#[derive(Deserialize)]
struct ServerConfig {
    port: Option<u16>,
}

#[derive(Deserialize)]
struct ConfigToml {
    serein: Option<ServerConfig>,
}

pub fn get_port() -> u16 {
    let dir = std::env::var("SEREIN_HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|_| {
            let home = dirs_next::home_dir().unwrap_or_default();
            home.join(".vSoft").join("Serein")
        });
    let path = dir.join("config.toml");
    if let Ok(content) = std::fs::read_to_string(&path) {
        if let Ok(cfg) = toml::from_str::<ConfigToml>(&content) {
            if let Some(port) = cfg.serein.and_then(|s| s.port) {
                return port;
            }
        }
    }
    12510 // default
}
