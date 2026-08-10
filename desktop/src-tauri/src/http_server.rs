use std::io::Read;

use tauri::{AppHandle, Emitter, Manager};
use tiny_http::{Header, Method, Response, Server};

use crate::{AppState, PendingInfo};

/// 启动本地 HTTP 服务（127.0.0.1:5712）。
/// - GET /identity   平台网页探测：返回当前桌面会话用户信息
/// - POST /pending   平台推送免密登录请求：触发客户端确认弹窗
pub fn start(app: AppHandle) {
    std::thread::spawn(move || {
        let server = match Server::http("127.0.0.1:5712") {
            Ok(s) => s,
            Err(e) => {
                eprintln!("本地服务启动失败: {}", e);
                return;
            }
        };
        for mut request in server.incoming_requests() {
            let url = request.url().to_string();
            let method = request.method().clone();
            let mut body = Vec::new();
            if method == Method::Post {
                let _ = request.as_reader().take(8192).read_to_end(&mut body);
            }
            let (status, payload) = route(&app, &url, &method, &body);
            let hdr = |n: &str, v: &str| Header::from_bytes(n.as_bytes(), v.as_bytes()).unwrap();
            let response = Response::from_string(payload)
                .with_status_code(status)
                .with_header(hdr("Access-Control-Allow-Origin", "*"))
                .with_header(hdr("Access-Control-Allow-Methods", "GET, POST, OPTIONS"))
                .with_header(hdr("Access-Control-Allow-Headers", "Content-Type"))
                .with_header(hdr("Content-Type", "application/json; charset=utf-8"));
            let _ = request.respond(response);
        }
    });
}

fn route(app: &AppHandle, url: &str, method: &Method, body: &[u8]) -> (u16, String) {
    if *method == Method::Options {
        return (204, String::new());
    }
    let state = app.state::<AppState>();
    if url.starts_with("/identity") {
        let logged_in = state.session.lock().unwrap().is_some();
        // 仅返回登录状态，不暴露用户信息（防任意网页身份探测）
        return (200, serde_json::json!({"logged_in": logged_in}).to_string());
    }
    if url.starts_with("/pending") && *method == Method::Post {
        return match serde_json::from_slice::<serde_json::Value>(body) {
            Ok(v) => {
                let request_id = v["request_id"].as_str().unwrap_or("").to_string();
                let platform = v["platform"].as_str().unwrap_or("").to_string();
                // request_id 必须为 32 位十六进制（认证中心 initiate 签发格式），防伪造请求刷弹窗
                if request_id.len() != 32 || !request_id.chars().all(|c| c.is_ascii_hexdigit()) {
                    return (400, serde_json::json!({"code": 1, "msg": "request_id 格式错误"}).to_string());
                }
                // 推送节流：1 秒内只接受一次，防弹窗轰炸
                {
                    let mut last = state.last_push.lock().unwrap();
                    if last.elapsed().as_secs() < 1 {
                        return (429, serde_json::json!({"code": 1, "msg": "推送过于频繁"}).to_string());
                    }
                    *last = std::time::Instant::now();
                }
                *state.pending.lock().unwrap() = Some(PendingInfo { request_id, platform });
                let _ = app.emit("desktop-pending", ());
                (200, serde_json::json!({"code": 0}).to_string())
            }
            Err(e) => (
                400,
                serde_json::json!({"code": 1, "msg": format!("JSON 解析失败: {}", e)}).to_string(),
            ),
        };
    }
    if url.starts_with("/status") {
        let p = state.pending.lock().unwrap();
        return (
            200,
            serde_json::json!({"pending": p.is_some(), "platform": p.as_ref().map(|x| x.platform.clone())}).to_string(),
        );
    }
    (404, serde_json::json!({"code": 1, "msg": "not found"}).to_string())
}

