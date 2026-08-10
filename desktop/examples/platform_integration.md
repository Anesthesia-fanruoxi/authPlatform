# 平台接入「桌面令牌登录」完整示例（新架构：平台 → 桌面端 → 认证中心）

> 面向：业务平台（如 ops-platform）在自己的**登录页**集成「令牌登录」按钮。
>
> ## 架构（2026-08 修订）
>
> ```
> 旧：平台(持 secret, 签名) ──► 认证中心
> 新：平台登录环节 ──► 桌面端(持有各平台 secret, 封装签名) ──► 认证中心
> ```
>
> 好处：
> 1. **所有平台都能通过桌面端快捷登录**——平台只要调桌面端本地接口，无需关心认证中心签名。
> 2. **签名逻辑全部封装在桌面端**——登录环节的 secret、HMAC 签名、initiate/confirm/exchange 流程都在桌面端内部完成，平台前端/后端不再接触登录相关加密盐。
> 3. **减少平台配置**——登录环节平台零配置，secret 由桌面端登录后自动从认证中心拉取。
>
> ## ⚠️ 重要边界：平台后端仍需一个 secret（用于用户数据同步）
>
> 「平台零配置」**仅指登录环节**。平台后端如果要**拉取用户列表/同步用户**（`GET /api/users`、`GET /api/users/{uid}`）、校验 token 等**服务器到服务器**的调用，
> **平台后端仍必须持有自己在认证中心的 secret 签名**——这类调用发生在平台服务器上，与用户电脑上的桌面端无关，桌面端无法代理。
>
> 两个 secret 分工明确：
>
> | secret | 持有方 | 用途 | 能否省 |
> |---|---|---|---|
> | 登录代理 secret | 桌面端（登录后自动拉取） | 替平台走 initiate/confirm/exchange 登录流程 | 可省（平台侧不配） |
> | 数据同步 secret | 平台后端 | 用户列表拉取/同步、token 校验 | **不可省**（服务器调用必须自签） |
>
> 平台后端的数据同步流程见 `doc/接入文档.md` §7（synced_users 同步）。

## 时序总览（同步响应）

```
用户点「令牌登录」
  平台前端 ──POST 127.0.0.1:5712/login {platform_id}────▶ 桌面端（长连接挂起）
                                                         │ 用该平台的 secret 配置签名 initiate（认证中心）
                                                         │ 弹确认窗「平台X请求以 xxx 登录？确认/拒绝」
  用户点确认 ──────────────────────────────────────────▶ │ confirm（desktop_token 会话）→ 认证中心
                                                         │ exchange → 认证中心签发 token
  平台前端 ◀──────── 响应返回 {token, user} ──────────────│
  平台前端 ──POST /api/session {token} ─────────────────▶ 平台后端（无签名）
                                                         │ 用 token 建立平台自己的会话
  平台前端 ◀── 登录成功，跳转平台主页 ──────────────────────
```

> 平台与认证中心之间**不再有直接调用**；`/api/auth/desktop/initiate|poll|exchange` 这些签名接口现在由**桌面端内部**调用（桌面端持有该平台在认证中心的接入配置）。

## 一、平台前端（登录页 JS）——只需要调桌面端本地接口

```html
<button id="desktop-login">桌面令牌登录</button>
```

```js
// login.js —— 平台登录页
$('#desktop-login').on('click', async () => {
  // 1) 向桌面端发起登录（同步等待：桌面端弹窗 → 用户确认 → 返回 token）
  //    请求会挂起直到用户在桌面端确认/拒绝（60s 内）
  let res;
  try {
    res = await fetch('http://127.0.0.1:5712/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ platform_id: 'ops-platforms' }),   // 你的平台 ID
    }).then(r => r.json());
  } catch {
    return alert('未检测到桌面令牌客户端，请先打开并登录客户端');
  }
  if (res.code !== 0) return alert(res.msg || '登录失败或已拒绝');

  // 2) 把 token 交给平台后端建立会话（token 仅返回这一次）
  const r = await fetch('/api/session', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token: res.token, user: res.user }),
  }).then(r => r.json());
  if (r.ok) location.href = '/';
});
```

## 二、平台后端——只收 token 建会话（无签名、无 secret）

```python
# app.py —— 平台后端唯一新增路由
from flask import Flask, request, jsonify, session
import requests

AUTH_BASE = "http://authplatform.example.com"     # 认证中心地址（仅用于校验 token，可选）

app = Flask(__name__)

@app.post("/api/session")
def create_session():
    """前端在桌面端确认后把 token 交到这里，平台用它建立自己的会话。"""
    data = request.get_json()
    token, user = data.get("token"), data.get("user")
    if not token:
        return jsonify(ok=False, msg="缺少 token"), 400
    # 可选：向认证中心校验 token 有效性（认证中心提供 token 校验接口时）
    # r = requests.post(f"{AUTH_BASE}/api/token/verify", json={"token": token}, timeout=3)
    # if r.json()["code"] != 0: return jsonify(ok=False, msg="token 无效"), 401
    # 建会话：把认证中心 uid 映射到平台本地用户，持久化 token 与过期时间
    session["uid"] = user.get("uid")
    session["username"] = user.get("username")
    session["auth_token"] = token
    return jsonify(ok=True, user=user)
```

## 三、桌面端配置（登录后自动拉取授权平台）

桌面端登录认证中心后，自动向认证中心拉取「当前用户被授权的平台列表 + 各平台登录代理 secret」，无需手动配置：

```
桌面端登录（desktop/login，desktop_token）
   └─► 认证中心新增接口：GET /api/auth/desktop/platforms
        （凭 desktop_token 会话身份）
        ◀─ [{platform_id, name, secret(登录代理用)}, ...]
```

桌面端收到平台 `POST /login` 时：按 `platform_id` 在已拉取的列表里找配置 → 签名 `initiate` → 弹确认窗 →
用户确认 → `confirm` → `exchange` → 把 token 同步返回给平台页面。

> 该接口属于新架构的认证中心配套需求（桌面端以客户端身份拉取，非平台签名身份）。
> 注意：这里下发的 secret 仅用于**登录代理**；平台后端做用户同步仍使用自己在认证中心注册的 secret。

## 四、认证中心视角

- 认证中心**无需改动**：`initiate/poll/exchange` 仍是平台签名接口，只是调用者由「平台」变为「桌面端」。
- 每个平台在认证中心仍注册一个 `platform_id + secret`，由管理员把 secret 交给桌面端配置（内网可信设备），或提供后续的桌面端配置管理界面。
- 平台后端如需校验桌面端交来的 token，认证中心应提供 `token 校验接口`（配套新增）。

## 五、注意事项

- **5712 的调用来自浏览器（平台前端）**：`127.0.0.1:5712` 是用户本机，只有前端页面在用户浏览器里能访问。
- **平台页面发起的 /login 会挂起**（同步等待用户确认），前端需设置合理超时（桌面端 60s 确认期）。
- **token 仅返回一次**：桌面端返回后即清除，平台前端应立即交给平台后端建会话。
- **配置安全**：桌面端持有的是认证中心签发给各平台的 secret，等同各平台的"钥匙"；桌面端文件应限制本机权限，生产环境建议加密存储。
- **未登录桌面端**：平台前端会收到 `logged_in=false` 类错误，提示用户先打开并登录桌面客户端。
