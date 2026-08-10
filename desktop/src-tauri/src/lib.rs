use std::sync::Mutex;

use serde::Serialize;
use tauri::{AppHandle, State};

pub mod http_server;
pub mod session;

/// 平台推送的免密登录待确认请求。
#[derive(Serialize, Clone)]
pub struct PendingInfo {
    pub request_id: String,
    pub platform: String,
}

/// 客户端全局状态。
pub struct AppState {
    pub session: Mutex<Option<session::Session>>,
    pub pending: Mutex<Option<PendingInfo>>,
    pub last_push: Mutex<std::time::Instant>, // 推送节流（防弹窗轰炸）
}

/// 登录认证中心（账号/密码或 TOTP），成功后本地持久化会话。
#[tauri::command]
fn login(base_url: String, method: String, identifier: String, credential: String, state: State<AppState>) -> Result<serde_json::Value, String> {
    let s = session::login(&base_url, &method, &identifier, &credential)?;
    *state.session.lock().unwrap() = Some(s.clone());
    Ok(serde_json::json!({"uid": s.uid, "username": s.username, "nickname": s.nickname, "expires_at": s.expires_at}))
}

/// 返回当前会话用户信息（未登录返回 logged_in=false）。
#[tauri::command]
fn identity(state: State<AppState>) -> serde_json::Value {
    let sess = state.session.lock().unwrap();
    match &*sess {
        Some(s) => serde_json::json!({"logged_in": true, "user": {"uid": s.uid, "username": s.username, "nickname": s.nickname}}),
        None => serde_json::json!({"logged_in": false}),
    }
}

/// 返回当前待确认请求（无则 null）。
#[tauri::command]
fn pending(state: State<AppState>) -> Option<PendingInfo> {
    state.pending.lock().unwrap().clone()
}

/// 确认当前待确认请求（调认证中心 desktop/confirm）。
#[tauri::command]
fn confirm_pending(state: State<AppState>) -> Result<String, String> {
    let (base_url, token, request_id) = {
        let sess = state.session.lock().unwrap();
        let s = sess.as_ref().ok_or("客户端未登录")?;
        let p = state.pending.lock().unwrap();
        let pi = p.as_ref().ok_or("没有待确认请求")?;
        (s.base_url.clone(), s.desktop_token.clone(), pi.request_id.clone())
    };
    session::confirm(&base_url, &token, &request_id)?;
    *state.pending.lock().unwrap() = None;
    Ok("已确认".to_string())
}

/// 拒绝当前待确认请求。
#[tauri::command]
fn reject_pending(state: State<AppState>) {
    *state.pending.lock().unwrap() = None;
}

/// 登出：清除本地会话。
#[tauri::command]
fn logout(state: State<AppState>) {
    *state.session.lock().unwrap() = None;
    session::clear();
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .manage(AppState {
            session: Mutex::new(session::load()),
            pending: Mutex::new(None),
            last_push: Mutex::new(std::time::Instant::now() - std::time::Duration::from_secs(10)),
        })
        .setup(|app| {
            let handle: AppHandle = app.handle().clone();
            http_server::start(handle);
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            login, identity, pending, confirm_pending, reject_pending, logout
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
