# authPlatform 平台接入接口文档

> 面向**接入平台**（如 ops-platform）的对接说明。authPlatform 是统一鉴权中心：
> 账号/密码/授权/token 由 authPlatform 管理，各平台只做**转发调用**，本地不存账号密码。
>
> 更新：2026-08-06 ｜ 与源码 `api/` 目录一一对应，如需核对实现可直接看对应文件。

---

## 1. 接入准备

1. 管理员在 authPlatform 管理后台「平台管理」注册你的平台，得到：
   - `platform_id`（如 `ops-platform`）
   - `secret`（**独立加密盐，仅创建时展示一次**，务必保存；用于请求签名）
   - 可选配置：IP 白名单、平台自定义登录方式 `login_methods`
2. 在「授权管理」给目标用户勾选你的平台（未授权用户无法登录、也拉取不到）。
3. 调用前自检：`GET /api/health` 返回 `{"code":0,...}` 即服务正常。

---

## 2. 请求签名（所有平台侧接口必需）

每个请求需携带 3 个头，防伪造 + 防重放：

```
X-Platform-Id: <platform_id>
X-Timestamp:   <当前 unix 秒>
X-Sign:        <签名 hex>

sign = HMAC-SHA256(
    secret,
    method + "|" + 完整RequestURI + "|" + timestamp + "|" + sha256(body)hex
)
```

要点：
- `完整RequestURI` = 路径 + 查询串**原样**（如 `/api/users?platform_id=ops-platform`），防 query 篡改绕过授权。
- `body` = 请求体原始字节；GET 无 body 时 `sha256("")`。
- 时间戳允许 ±300 秒，超出返回 `1001`。

**Python 签名函数（可直接复用）**：

```python
import hashlib, hmac, time, json, urllib.request

SECRET = "你的平台secret"
BASE   = "http://127.0.0.1:8080"
PID    = "ops-platform"

def signed_request(method, path, body=None):
    body_bytes = (body if isinstance(body, bytes) else json.dumps(body or {}).encode()) if body is not None else b""
    ts = str(int(time.time()))
    msg = f"{method}|{path}|{ts}|{hashlib.sha256(body_bytes).hexdigest()}"
    sign = hmac.new(SECRET.encode(), msg.encode(), hashlib.sha256).hexdigest()
    req = urllib.request.Request(BASE + path, data=body_bytes, method=method)
    req.add_header("X-Platform-Id", PID)
    req.add_header("X-Timestamp", ts)
    req.add_header("X-Sign", sign)
    if body is not None:
        req.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read())

# 示例
r = signed_request("POST", "/api/auth/verify", {"username": "alice", "password": "alice1234"})
print(r)
```

---

## 3. 统一响应与错误码

所有接口返回 HTTP 200 + JSON：

```json
{"code": 0, "msg": "ok", "data": {...}}
```

| code | 含义 | 处理建议 |
|---|---|---|
| 0 | 成功 | — |
| 1001 | 平台签名无效 / 时间戳过期 | 检查 secret、时间戳、RequestURI 是否与请求一致 |
| 1002 | 平台不存在或已停用 | 检查 platform_id |
| 1003 | 账号或密码错误（或验证码错误） | 提示用户 |
| 1004 | 账号已禁用 | 提示联系管理员 |
| 1005 | 登录尝试过多 / 账号被临时锁定 | 提示稍后再试（5 次失败锁 15 分钟） |
| 1006 | 该用户未授权登录此平台 | 提示联系管理员授权 |
| 1007 | 参数错误 / 登录票据无效或已过期 | 检查请求体 |
| 1009 | IP 不在白名单 | 检查来源 IP（后台配置） |
| 2001 | 内部错误 | 反馈管理员看服务日志 |

---

## 4. 接口明细

### 4.1 登录校验 `POST /api/auth/verify`

登录**第一步**（或唯一一步）。请求体：

```json
{
  "platform_id": "ops-platform",
  "method": "username_password",
  "identifier": "alice",
  "credential": "alice1234"
}
```

- `method` 可选值：`username_password`（identifier=用户名）、`email_password`（identifier=邮箱）、`phone_code`（identifier=手机号，配合 send-code）。
- **兼容旧格式**：只传 `{username, password, platform_id}` 时等价于 `username_password`。
- 必须与平台生效的**第一个**登录方式一致（平台自定义 `login_methods` 优先，否则系统默认模板）。

**响应 A — 单步完成（平台只有一种登录方式）：**

```json
{"code":0,"data":{
  "token": "<64位hex不透明token>",
  "expires_at": "2026-08-07T10:00:00+08:00",
  "user": {"uid":"u_xxx","username":"alice","nickname":"爱丽丝","status":1}
}}
```

**响应 B — 多步骤（平台配置了 ≥2 种登录方式，如 password+totp）：**

```json
{"code":0,"data":{
  "ticket":"<5分钟内有效的登录票据>","step":1,"total_steps":2,
  "next_method":"totp","expires_in":300,
  "identifier":""}}
```

→ 继续调 §4.2，直到 `total_steps` 走完拿到 token。

### 4.2 登录后续步骤 `POST /api/auth/verify-step`

```json
{"platform_id":"ops-platform","ticket":"<上一步返回>","credential":"<当前步骤的凭证>"}
```

- `credential`：`totp` 步传 6 位验证码；`phone_code`/`email_code` 步传验证码。
- 未走完返回下一个 `next_method`；**最后一步通过后**返回与响应 A 相同的 `token`。
- ticket 一次性、5 分钟有效；凭证失败**不销毁** ticket 可重试（受限流保护）。

### 4.3 发送验证码 `POST /api/auth/send-code`

```json
{"platform_id":"ops-platform","method":"phone_code","identifier":"13800000000"}
```

- `method`：`phone_code` 或 `email_code`。
- **开发模式**：未接真实短信/邮件服务商，验证码直接返回，便于联调：
  ```json
  {"code":0,"data":{"dev_code":"482913","expires_in_seconds":300,"method":"phone_code"}}
  ```
  接入真实发送器后 `dev_code` 字段会移除，改为平台自行从短信/邮箱获取验证码。

### 4.4 修改密码 `POST /api/auth/change-password`

```json
{"platform_id":"ops-platform","username":"alice","old_password":"旧密码","new_password":"新密码"}
```

- 校验旧密码 + 授权 + 密码策略（≥8 位含字母数字）。
- 返回 `{"code":0}`。

### 4.5 修改资料 `POST /api/auth/update-profile`

```json
{
  "platform_id": "ops-platform",
  "username": "alice",
  "nickname": "爱丽丝",
  "email": "alice@example.com",
  "phone": "13800000000",
  "password": "new_password_123",
  "totp_secret": ""
}
```

- 约定：**变更逻辑在平台处理、数据在认证中心存储**——平台把变更后的字段一次性提交，本接口只负责授权校验与落库。
- 字段规则：
  - `nickname`：非空才更新。
  - `email` / `phone`：**不传=不修改**；`""`=清空（存 NULL，不参与唯一约束）；非空=更新（唯一冲突预检查，已被其他账号使用会报错）。
  - `password`：非空则代改密码（管理员场景，无需旧密码），authPlatform 哈希存储（校验密码策略）。
  - `totp_secret`：**不传=不修改**；非空 base32 密钥=重新绑定并启用 TOTP；`""`=清除（解除双因子）。
- 返回该用户白名单信息（uid/username/nickname/phone/email/status/created_at）。

### 4.6 上报双因子密钥 `POST /api/auth/totp/save`

```json
{"platform_id":"ops-platform","username":"alice","secret":"JBSWY3DPEHPK3PXP"}
```

- **绑定流程在平台侧完成**（生成密钥/扫码/验证码确认），绑定成功后把 base32 格式 secret 上报，authPlatform 仅存储；后续登录的 TOTP 校验由 authPlatform 统一完成（见 §4.2 `totp` 步）。
- 重新绑定/解除亦可走 §4.5 `update-profile` 的 `totp_secret` 字段（非空=绑定，`""`=清除）。
- 返回 `{"code":0,"data":{"totp_enabled":true}}`。

### 4.7 拉取单个用户 `GET /api/users/{uid}?platform_id=ops-platform`

- 仅返回**授权给本平台**的用户；不存在、未授权或**认证中心管理员（is_admin）**一律 HTTP 404（平台侧不可见）。
- 字段白名单：`uid / username / nickname / phone / email / status / created_at`（**绝不包含密码等凭据**）。

### 4.8 拉取用户列表 `GET /api/users?platform_id=ops-platform&keyword=可选`

- 返回本平台**已授权**的用户列表（服务端强制过滤；**认证中心管理员不返回**，不同步到任何平台）：
  ```json
  {"code":0,"data":{"users":[{"uid":"u_xxx","username":"alice","nickname":"爱丽丝","phone":"13800000000","email":"alice@example.com","status":1,"created_at":"..."}]}}
  ```

---

### 4.9 管理后台接口（控制台使用，需登录态 `Authorization: Bearer <token>`）

> 登录：`POST /api/admin/login {username, password}` → `{token, user}`；登录成功后后续请求携带 `Authorization: Bearer <token>`。
> **超级管理员（is_admin=1）不出现在用户列表与授权矩阵中**，其个人信息（如修改密码）通过个人设置完成，由平台侧按需调用 `POST /api/admin/me/password {old_password, new_password}`。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/admin/me` | 当前登录管理员信息 |
| POST | `/api/admin/me/password` | 修改自己的密码（校验原密码） |
| GET | `/api/admin/users?keyword=&category=` | 用户列表（不含超管；支持关键字/分类筛选） |
| POST | `/api/admin/users` | 创建用户 `{username, password, nickname, phone, email, category}` |
| PUT | `/api/admin/users/{id}` | 更新 `{nickname, phone, email, status, category}` |
| DELETE | `/api/admin/users/{id}` | 删除用户 |
| POST | `/api/admin/users/{id}/reset-password` | 重置密码 `{new_password}` |
| GET | `/api/admin/grants?category=` | 授权矩阵数据（用户×平台，不含超管，支持分类筛选） |
| POST | `/api/admin/users/{id}/grants` | 全量设置用户可登录平台 `{platform_ids: [1,2]}` |
| GET/PUT | `/api/admin/platforms`、`/api/admin/platforms/{id}` | 平台管理（创建时返回一次明文 secret） |
| POST | `/api/admin/platforms/{id}/rotate-secret` | 密钥轮换（双盐过渡，第二次轮换吊销旧盐） |
| GET | `/api/admin/logs?username=&platform_id=&success=&limit=` | 审计日志 |
| GET/PUT | `/api/admin/settings`、`/api/admin/settings/{key}` | 系统设置（含 `user_categories` 用户分类列表 `{items:[...]}`） |

**用户分类**：分类（开发/测试/运营/风控/数分等）由管理员在系统设置维护（可自定义增删），用于用户管理标识与授权管理按分类筛选（快捷授权）；分类归属是认证中心存储的用户属性，权限/部门/角色等业务数据仍由各平台自行维护。

---

## 5. 登录流程示例（完整可跑）

```python
# 单步登录（默认配置：username_password）
r = signed_request("POST", "/api/auth/verify",
    {"username": "alice", "password": "alice1234"})
if r["code"] == 0 and "token" in r["data"]:
    token = r["data"]["token"]          # 平台自行保存 token，自行管理生命周期（不吊销）
else:
    print("登录失败:", r["msg"])

# 两步登录（username_password + totp）
r1 = signed_request("POST", "/api/auth/verify",
    {"method": "username_password", "identifier": "alice", "credential": "alice1234"})
ticket = r1["data"]["ticket"]
# 用户输入 Authenticator 上的 6 位验证码
code = input("TOTP code: ")
r2 = signed_request("POST", "/api/auth/verify-step",
    {"ticket": ticket, "credential": code})
# r2["data"]["token"] 即最终 token
```

---

## 6. 常见问题

| 现象 | 原因/排查 |
|---|---|
| `1001` 签名无效 | secret 是否写对；RequestURI 是否**含 query 原样**；时间戳偏差 >300s |
| `1006` 未授权 | 管理员未在「授权管理」给该用户勾选你的平台 |
| 拉取用户 404 | 该用户未授权你的平台（或不存在）——对平台完全不可见 |
| `1005` 锁定 | 该账号 5 次失败已锁 15 分钟（账号维度，全平台共享） |

> 审计：每次登录成功/失败都会写入 authPlatform 审计日志（管理后台「审计日志」页可查，含 reason：ok/bad_cred/bad_code/bad_totp/unauthorized/locked/banned/disabled）。
