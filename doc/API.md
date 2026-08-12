# authPlatform 接口文档（Web 调用调试版）

> **用途**：用浏览器控制台 / Postman / Apifox 等 Web 工具调用认证中心接口、验证登录流程。
> **完整接入规范**（平台签名协议、时序、业务端对接）见 `doc/接入文档.md`。
> **版本**：当前服务（:8080）。

## 1. 基础信息

| 项 | 值 |
|---|---|
| Base URL | `http://127.0.0.1:8080` |
| 健康检查 | `GET /api/health` → `{"code":0,...}` |
| 响应格式 | `{"code": 0, "msg": "ok", "data": {...}}`，`code != 0` 为失败 |
| 内容类型 | 一律 `Content-Type: application/json`（GET 无 body） |

### 错误码

| code | 含义 |
|---|---|
| 0 | 成功 |
| 1001 | 平台签名无效 |
| 1002 | 平台不存在/已禁用 |
| 1003 | 账号或密码错误（凭据错误） |
| 1004 | 账号已禁用 |
| 1005 | 账号已锁定（限流） |
| 1006 | 未授权 / 会话无效 |
| 1007 | 参数错误 |
| 1008 | 用户名已存在 |
| 1009 | IP 不在后台登录白名单 |
| 2001 | 服务内部错误 |

## 2. 三种调用身份

| 身份 | 认证方式 | 适用接口 |
|---|---|---|
| 管理端 | `Authorization: Bearer <admin_token>` | `/api/admin/*` |
| 平台侧 | `X-Platform-Id` + `X-Timestamp` + `X-Sign`（HMAC 签名） | `/api/auth/*`、`/api/users*` |

### 平台签名算法（HMAC-SHA256）

```
签名串 = "{METHOD}|{完整RequestURI}|{X-Timestamp}|{sha256hex(body)}"
X-Sign = HMAC-SHA256(平台secret, 签名串)   # 小写 hex
X-Timestamp 与服务器时间差须在 ±300s 内
```

**浏览器 JS 签名函数**（可直接粘贴到控制台）：

```js
async function platformSign(secret, method, path, body) {
  const ts = String(Math.floor(Date.now() / 1000));
  const hex = async (buf) => [...new Uint8Array(buf)].map(b => b.toString(16).padStart(2, '0')).join('');
  const sha256 = async (s) => hex(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(s)));
  const signStr = `${method}|${path}|${ts}|${await sha256(body ? JSON.stringify(body) : '')}`;
  const key = await crypto.subtle.importKey('raw', new TextEncoder().encode(secret), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
  const sign = await hex(await crypto.subtle.sign('HMAC', key, new TextEncoder().encode(signStr)));
  return { 'X-Platform-Id': '你的平台ID', 'X-Timestamp': ts, 'X-Sign': sign, 'Content-Type': 'application/json' };
}
```

## 3. 管理后台接口（Web 调试）

### 3.1 管理员登录 → 拿 Bearer token

```
POST /api/admin/login
Body: {"username":"admin","password":"admin123"}
响应: {"code":0,"data":{"token":"...","user":{...}}}
```
> 默认管理员 `admin/admin123`（生产请用 `ADMIN_PASSWORD` 环境变量修改）。

### 3.2 当前管理员信息

```
GET /api/admin/me        Headers: Authorization: Bearer <token>
```

### 3.3 用户管理

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/admin/users?keyword=&category=&status=&totp=&has_category=` | 用户列表（不含超管）；`status` 0/1、`totp` 0/1、`has_category` 0/1、`category` 精确分类 |
| POST | `/api/admin/users` | 创建：`{"username","password","nickname","phone","email","category"}` |
| PUT | `/api/admin/users/{id}` | 更新：`{"nickname","phone","email","status","category"}`（字段可选） |
| POST | `/api/admin/users/{id}/reset-password` | 重置密码：body `{}` 自动生成并返回 `{"password":"..."}`；传 `{"new_password":"..."}` 手动指定 |
| DELETE | `/api/admin/users/{id}` | 删除用户 |
| POST | `/api/admin/users/batch-category` | 批量分类：`{"user_ids":[1,2],"category":"开发"}`（空串=清除） |

### 3.4 平台管理

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/admin/platforms` | 平台列表（含 `secret_masked`） |
| POST | `/api/admin/platforms` | 创建：`{"platform_id","name","login_methods":[...],"auth_mode":"single|two_step"}` |
| PUT | `/api/admin/platforms/{id}` | 更新（`login_methods`、`auth_mode`、`ip_whitelist` 等） |
| POST | `/api/admin/platforms/{id}/rotate-secret` | 轮换密钥（返回新 secret） |
| DELETE | `/api/admin/platforms/{id}` | 删除平台 |

### 3.5 授权 / 日志 / 设置 / 封禁

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/admin/grants?category=` | 授权矩阵（平台×用户，可带分类筛选） |
| POST | `/api/admin/users/{id}/grants` | 用户授权：`{"platform_ids":[1,2]}` |
| POST | `/api/admin/platforms/{id}/grants` | 列级批量授权：`{"action":"grant\|revoke","user_ids":[...]}` |
| GET | `/api/admin/logs?page=&size=` | 审计日志 |
| GET/PUT | `/api/admin/settings`、`/api/admin/settings/{key}` | 系统设置（密码策略/限流/登录方式/用户分类 `user_categories`） |
| GET/POST/DELETE | `/api/admin/bans` | 登录封禁名单 |

## 4. 平台侧接口（需签名）

### 4.1 登录校验 `POST /api/auth/verify`

```
Headers: X-Platform-Id / X-Timestamp / X-Sign
Body（新格式）: {"platform_id":"ops-platforms","method":"username_password","identifier":"alice","credential":"密码"}
Body（旧格式兼容）: {"username":"alice","password":"密码","platform_id":"ops-platforms"}
```
登录方式 `method`：`username_password`（默认）、`email_password`、`phone_code`、`username_totp`（用户名+TOTP 验证码，无密码）。

**验证模式**（平台配置 `auth_mode`）：
- `two_step`（二次验证，默认）：多选登录方式 = **按顺序全部通过**，第一步过 → `ticket` → 第二步过 → token。
- `single`（单次登录）：多选登录方式 = **任意其一**，客户端任选一种 `method` 通过即直接发 token（不走多步）。

**响应**：
- 单步 / single 模式（任一方式通过）：`{"code":0,"data":{"token":"<64位hex>","expires_at":"...","user":{"uid","username","nickname","status"}}}`
- two_step 且配置 ≥2 种方式（需二步）：`{"code":0,"data":{"ticket":"<5分钟有效>","step":1,"total_steps":2,"next_method":"username_totp","expires_in":300,"identifier":"..."}}` → 调 4.2

### 4.2 登录后续步骤 `POST /api/auth/verify-step`

```
Body: {"platform_id":"ops-platform","ticket":"<上一步返回的 ticket>","credential":"<当前步骤凭证>"}
响应: 未走完返回下一步（ticket + next_method）；最后一步通过返回 token（同单步响应）
```
> 请求体**不含 method**：登录方式顺序由 ticket 关联的流程自动推进；`username_totp` 步传 6 位 TOTP 动态码。

### 4.3 平台代存 TOTP 绑定 `POST /api/auth/totp/save`

```
Body: {"username":"alice","secret":"BASE32SECRET"}   # 平台侧绑定后回写存储
```

### 4.4 用户信息拉取（平台同步用）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/users?platform_id=` | 平台可登录用户列表（返回 uid/username/nickname/phone/email/totp_enabled 等） |
| GET | `/api/users/{uid}` | 单用户详情 |

## 5. Web 快速试登录（浏览器控制台可复制）

### 5.1 管理员登录

```js
const r = await fetch('http://127.0.0.1:8080/api/admin/login', {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'admin', password: 'admin123' }),
}).then(r => r.json());
console.log(r);                        // code=0，data.token 即管理端 Bearer
```

### 5.2 平台签名登录（模拟平台转发）

> `method` 必须在该平台配置的 `login_methods` 内（管理后台「平台管理」可查）；`single` 模式返回 1007「当前平台支持登录方式: a / b」、`two_step` 模式返回 1007「当前第一步登录方式为 xxx」即签名已通过、仅方式不匹配，改用平台配置的方式即可。

```js
const SECRET = '你的平台secret';        // 认证中心「平台管理」里注册/轮换得到
const PATH = '/api/auth/verify';
const body = { platform_id: 'ops-platforms', method: 'username_password', identifier: 'alice', credential: '密码' };
const headers = await platformSign(SECRET, 'POST', PATH, body);   // 用第 2 节的函数
const r = await fetch('http://127.0.0.1:8080' + PATH, { method: 'POST', headers, body: JSON.stringify(body) }).then(r => r.json());
console.log(r);                        // code=0 -> data.token
```

## 6. 常见问题

- **登录方式不匹配**：`verify` 的 `method` 必须在该平台配置的 `login_methods` 内（平台管理页配置）；`single` 模式任一方式通过即可，`two_step` 模式按勾选顺序逐步验证；`username_totp` 用户需已绑定 TOTP，否则返回"该账号未启用 TOTP"。
- **签名失败 1001**：检查 secret 是否正确、`X-Timestamp` 是否 ±300s 内、签名串的 URI 是否与请求完全一致（含 query）、body 是否与签名时一致。
- **限流 1005**：同一账号 5 次失败锁 15 分钟（账号维度）。
