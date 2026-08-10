use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;

/// 桌面会话（desktop_token 仅客户端本地持有，认证中心只存哈希）。
#[derive(Serialize, Deserialize, Clone)]
pub struct Session {
    pub base_url: String,
    pub desktop_token: String,
    pub uid: String,
    pub username: String,
    pub nickname: String,
    pub expires_at: String,
}

fn session_path() -> PathBuf {
    dirs::data_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join("authplatform-desktop")
        .join("session.json")
}

pub fn load() -> Option<Session> {
    fs::read_to_string(session_path())
        .ok()
        .and_then(|s| serde_json::from_str(&s).ok())
}

pub fn save(s: &Session) -> std::io::Result<()> {
    let p = session_path();
    if let Some(dir) = p.parent() {
        let _ = fs::create_dir_all(dir);
    }
    fs::write(p, serde_json::to_string_pretty(s).unwrap())
}

pub fn clear() {
    let _ = fs::remove_file(session_path());
}

fn parse_error(v: &serde_json::Value) -> String {
    v["msg"].as_str().unwrap_or("未知错误").to_string()
}

/// 登录认证中心 desktop/login，成功后本地持久化会话。
pub fn login(base_url: &str, method: &str, identifier: &str, credential: &str) -> Result<Session, String> {
    let client = reqwest::blocking::Client::new();
    let url = format!("{}/api/auth/desktop/login", base_url.trim_end_matches('/'));
    let body = serde_json::json!({"method": method, "identifier": identifier, "credential": credential});
    let resp = client
        .post(&url)
        .json(&body)
        .timeout(std::time::Duration::from_secs(10))
        .send()
        .map_err(|e| format!("认证服务调用失败: {}", e))?;
    let v: serde_json::Value = resp.json().map_err(|e| format!("响应解析失败: {}", e))?;
    if v["code"].as_i64().unwrap_or(-1) != 0 {
        return Err(parse_error(&v));
    }
    let d = &v["data"];
    let user = &d["user"];
    let s = Session {
        base_url: base_url.trim_end_matches('/').to_string(),
        desktop_token: d["desktop_token"].as_str().unwrap_or("").to_string(),
        uid: user["uid"].as_str().unwrap_or("").to_string(),
        username: user["username"].as_str().unwrap_or("").to_string(),
        nickname: user["nickname"].as_str().unwrap_or("").to_string(),
        expires_at: d["expires_at"].as_str().unwrap_or("").to_string(),
    };
    save(&s).map_err(|e| format!("保存会话失败: {}", e))?;
    Ok(s)
}

/// 确认免密登录请求 desktop/confirm。
pub fn confirm(base_url: &str, desktop_token: &str, request_id: &str) -> Result<(), String> {
    let client = reqwest::blocking::Client::new();
    let url = format!("{}/api/auth/desktop/confirm", base_url.trim_end_matches('/'));
    let body = serde_json::json!({"desktop_token": desktop_token, "request_id": request_id});
    let resp = client
        .post(&url)
        .json(&body)
        .timeout(std::time::Duration::from_secs(10))
        .send()
        .map_err(|e| format!("认证服务调用失败: {}", e))?;
    let v: serde_json::Value = resp.json().map_err(|e| format!("响应解析失败: {}", e))?;
    if v["code"].as_i64().unwrap_or(-1) != 0 {
        return Err(parse_error(&v));
    }
    Ok(())
}
