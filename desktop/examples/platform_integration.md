# 平台接入「桌面令牌登录」完整示例（前后端分工）

> 面向：业务平台（如 ops-platform）在自己的**登录页**集成「令牌登录」按钮。
> 核心分工原则：
> - **带 secret 的认证中心调用**（initiate / poll / exchange）→ **平台后端**做（secret 绝不能放前端）。
> - **客户端本地探测与推送**（`127.0.0.1:5712`）→ **必须平台前端 JS** 做。因为桌面客户端运行在**用户浏览器所在的电脑**上；平台后端服务器访问 `127.0.0.1:5712` 是它自己的机器，够不到用户电脑。

## 时序总览

```
用户点「令牌登录」
  平台前端 ──GET /api/desktop/start──────────────▶ 平台后端
                                                   │ 签名 initiate（认证中心）
                                                   ◀── request_id
  平台前端 ──GET 127.0.0.1:5712/identity ─────────▶ 桌面客户端（用户电脑，CORS 已开）
  平台前端 ──POST 127.0.0.1:5712/pending{request_id, platform} → 客户端弹确认窗
  平台前端 ──GET /api/desktop/status?request_id= ─▶ 平台后端（轮询，5s）
                                                   │ 签名 poll（认证中心）
  用户点确认 ─────────────────────────────────────▶ 桌面客户端 ──POST desktop/confirm──▶ 认证中心
  平台后端 poll 到 confirmed
  平台前端 ──POST /api/desktop/finish{request_id} ▶ 平台后端
                                                   │ 签名 exchange（认证中心）→ token
                                                   │ 用 token 建平台自己的 session
  平台前端 ◀── 登录成功，跳转平台主页
```

## 一、平台后端（Python/Flask 示例）

```python
# app.py —— 平台后端三个路由（只做认证中心签名调用）
import hashlib, hmac, json, time
import requests

AUTH_BASE = "http://authplatform.example.com"     # 认证中心地址
PLATFORM_ID = "ops-platforms"                     # 本平台在认证中心的 ID
SECRET = "你的平台secret"                          # 平台独立盐（认证中心平台管理里看/轮换）

def _signed(method, path, payload=None):
    body = json.dumps(payload).encode() if payload is not None else b""
    ts = str(int(time.time()))
    msg = f"{method}|{path}|{ts}|{hashlib.sha256(body).hexdigest()}"
    sign = hmac.new(SECRET.encode(), msg.encode(), hashlib.sha256).hexdigest()
    headers = {"X-Platform-Id": PLATFORM_ID, "X-Timestamp": ts, "X-Sign": sign,
               "Content-Type": "application/json"}
    url = AUTH_BASE + path
    r = requests.post(url, data=body, headers=headers, timeout=5) if method == "POST" \
        else requests.get(url, headers=headers, timeout=5)
    r.raise_for_status()
    return r.json()

# ① 前端点「令牌登录」→ 后端发起，把 request_id 交给前端
def desktop_start():
    r = _signed("POST", "/api/auth/desktop/initiate", {})
    return {"request_id": r["data"]["request_id"], "expires_in": r["data"]["expires_in"]}

# ② 前端轮询确认状态（也可以由后端轮询后推送到前端，见下方前端代码用轮询后端）
def desktop_status(request_id):
    r = _signed("GET", f"/api/auth/desktop/poll?request_id={request_id}")
    return {"status": r["data"]["status"]}   # pending / confirmed / used / expired

# ③ 用户确认后兑换 token，并建立平台自己的会话
def desktop_finish(request_id, user_ip):
    r = _signed("POST", "/api/auth/desktop/exchange", {"request_id": request_id})
    if r["code"] != 0:
        return {"ok": False, "msg": r["msg"]}
    token, user = r["data"]["token"], r["data"]["user"]
    # ↓ 用认证中心 token 换平台自己的 session（持久化 token 与用户绑定）
    create_platform_session(user["uid"], user["username"], token, user_ip)
    return {"ok": True, "user": user}
```

## 二、平台前端（登录页 JS 示例）

```html
<button id="desktop-login">桌面令牌登录</button>
```

```js
// login.js —— 平台登录页
const AUTH_PORT = 5712;                       // 桌面客户端本地端口

$('#desktop-login').on('click', async () => {
  // 1) 让后端发起，拿 request_id
  const { request_id } = await fetch('/api/desktop/start').then(r => r.json());

  // 2) 探测用户电脑上的桌面客户端（跨域到 127.0.0.1:5712，CORS 已开启）
  let identity;
  try {
    identity = await fetch(`http://127.0.0.1:${AUTH_PORT}/identity`, { mode: 'cors' }).then(r => r.json());
  } catch { return alert('未检测到桌面令牌客户端，请先打开并登录客户端'); }
  if (!identity.logged_in) return alert('桌面客户端未登录，请先在客户端登录');

  // 3) 推送确认请求 → 客户端弹出确认窗
  await fetch(`http://127.0.0.1:${AUTH_PORT}/pending`, {
    method: 'POST', mode: 'cors', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ request_id, platform: 'ops-platforms' }),
  });

  // 4) 轮询后端确认状态（5s 一次，60s 内）
  const deadline = Date.now() + 60000;
  while (Date.now() < deadline) {
    const { status } = await fetch(`/api/desktop/status?request_id=${request_id}`).then(r => r.json());
    if (status === 'confirmed') break;
    if (status === 'expired' || status === 'used') return alert('请求已过期，请重试');
    await new Promise(r => setTimeout(r, 5000));
  }

  // 5) 确认完成，让后端兑换 token 并建立平台会话
  const r = await fetch('/api/desktop/finish', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ request_id }),
  }).then(r => r.json());
  if (r.ok) location.href = '/';            // 登录成功，进入平台
  alert(r.msg || '登录失败');
});
```

## 三、注意事项

- **5712 的调用必须来自浏览器（前端）**：`127.0.0.1` 是用户本机，只有前端页面在用户浏览器里能访问到；后端调它没意义。
- **认证中心的调用必须来自后端**：initiate/poll/exchange 需要平台 secret 签名，secret 放前端等于泄露。
- **前端拿不到用户信息**：`/identity` 只返回 `logged_in`（防身份探测）；客户端确认窗自己会显示当前登录用户，用户在客户端核对即可。
- **60s 有效期**：从 initiate 到 exchange 全程必须在 60s 内完成，超时需重新发起。
- **一次性**：exchange 成功后 request_id 作废；并发只允许一次成功（认证中心已防重放）。
- **部署注意**：客户端 `5712` 端口只在用户本机开放；若平台需支持"手机扫码/远程机器"，属于另一套方案（可用 request_id 换成二维码+手机 App 确认）。
