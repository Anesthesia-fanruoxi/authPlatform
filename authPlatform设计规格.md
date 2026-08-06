# authPlatform 设计规格

> 状态：设计规格 v1（评审稿）
> 技术栈：Go（后端 + 独立 UI）
> 定位：统一登录系统（鉴权中心）——账号、平台、授权三实体，为多个平台提供登录校验与用户信息分发
> 约束：不维护 token 吊销；token 生命周期由各接入平台自行管理

---

## 1. 背景与目标

### 1.1 背景

内部存在多个平台（线上维护鉴权 CMDB、测试运维 ops-platform 等），各自维护账号体系，重复且不同步。需要一套统一的账号与登录服务：

- 账号只在一处维护（增删改查、改密）。
- 各平台登录时由 authPlatform 校验账号密码，校验通过后平台自行持有 token 并管理其生命周期。
- 用户在 authPlatform 侧配置「可登录哪些平台」；未授权的平台既不能登录，也拉取不到该用户信息。

### 1.2 目标

1. **统一账号**：账号密码唯一身份源，集中用户管理。
2. **平台签发**：每个接入平台单独注册、单独配置加密盐（密钥），平台调用需携带平台身份与签名。
3. **授权控制**：用户 ↔ 平台多对多授权；登录校验与用户信息拉取均受授权约束。
4. **平台自维护 token**：authPlatform 签发不透明 token 后不再管理，不做吊销；token 过期/续期/存储由平台负责。
5. **独立 UI**：authPlatform 自带管理界面（用户管理、平台管理、授权管理），不依赖任何接入平台的前端。

### 1.3 非目标

- 不做平台内权限/角色管理（各平台本地管）。
- 不做 token 吊销、单点登出（SLO）。
- 不做登录页托管（登录页在各平台，authPlatform 只提供校验 API）。
- 不引入 AD/LDAP/Kerberos（未来可作为账号源扩展，当前不涉及）。

---

## 2. 总体架构

```
用户（在各平台的登录页输入账号密码）
   │ ① 提交 {username, password}
   ▼
接入平台（如 ops-platform / CMDB）
   │ ② 转发 POST /api/auth/verify {username, password, platform_id}（带平台签名）
   ▼
authPlatform
   │ ③ 校验：平台身份(签名) → 账号密码 → 用户授权(是否可登录该平台)
   │ ④ 通过：签发不透明 token，落审计日志
   │ ⑤ 返回 {token, user_info, expires_at}
   ▼
接入平台：⑥ 自行存储 token、建立本地会话 → ⑦ 返回登录成功给用户（用户无感）
```

### 2.1 时序（一次登录）

```
平台                 authPlatform
 │  1.用户提交账号密码     │
 │──────────────────────▶│
 │  2.校验平台签名(盐)     │
 │  3.校验账号密码         │
 │  4.校验用户→平台授权     │
 │  5.签发token+审计       │
 │◀──────────────────────│  {token, user_info, expires_at}
 │  6.本地存token/建会话    │
 │  7.向用户返回登录成功     │
```

### 2.2 用户信息拉取（供平台建本地权限表）

```
平台                 authPlatform
 │  1.按 uid/username 查询用户    │
 │─────────────────────────────▶│
 │  2.校验平台签名                │
 │  3.过滤：只返回「授权给该平台」的用户 │
 │◀─────────────────────────────│  非敏感资料（昵称/状态/创建时间，不含密码）
```

---

## 3. 核心实体与职责

| 实体 | 说明 |
|---|---|
| **User（账号）** | 全局唯一账号：用户名、密码哈希、昵称、启用状态、创建时间 |
| **Platform（平台）** | 接入平台：名称、标识 `platform_id`、**独立加密盐（密钥）**、IP 白名单（可选）、启用状态 |
| **UserPlatformGrant（授权）** | 用户 ↔ 平台 多对多：`user_id + platform_id + 状态`，表示该用户被允许登录/被该平台可见 |

### 3.1 职责边界

- **authPlatform 负责**：账号校验、用户 CRUD、平台注册/签发与盐管理、用户↔平台授权、用户信息拉取（按授权过滤）、审计日志。
- **接入平台负责**：登录页、转发凭证、token 存储与生命周期、本地权限/角色、登录限流防爆破。
- **认证中心超管隔离**：认证中心内置超管（`is_admin`）与其他平台超管对称——唯一、只用于认证中心自身管理（登录后台），**不进入任何平台的用户列表/单查接口**（列表跳过、单查 404），不同步到任何平台。

---

## 4. 平台签发与加密协议

### 4.1 平台注册

1. 管理员在 authPlatform UI（或管理 API）创建平台，生成：
   - `platform_id`：唯一标识（如 `ops-platform`）
   - `secret`：**独立随机加密盐**（如 32 字节随机，hex 编码），仅展示一次，平台侧保存
   - 可选：IP 白名单
2. 每个平台**单独盐**，平台间互不知晓；单个平台盐泄露不影响其他平台。
3. 支持密钥轮换：生成新盐（双盐过渡期，新旧盐同时可验，轮换完成后吊销旧盐）。

### 4.2 平台请求签名（防伪造）

平台调用 authPlatform API 需携带签名（请求头或参数）：

```
sign = HMAC-SHA256(secret, method + "|" + path + "|" + timestamp + "|" + body_sha256)
headers:
  X-Platform-Id: <platform_id>
  X-Timestamp: <unix 秒>
  X-Sign: <sign hex>
```

- 时间戳防重放：允许 ±300s 窗口。
- 可选：请求体 AES-GCM 加密（密文放 `X-Encrypted`，用平台盐派生密钥）——默认签名即可，敏感字段传输走 HTTPS。

### 4.3 返回约定

统一响应：`{"code": 0, "msg": "ok", "data": {...}}`；非 0 为业务错误（见错误码表）。

---

## 5. API 设计

### 5.1 登录校验

```
POST /api/auth/verify
请求（平台签名，可选加密体）:
{
  "username": "alice",
  "password": "******",
  "platform_id": "ops-platform"
}
处理:
  1. 验平台签名 + 平台启用状态
  2. 验 IP 白名单（若配置）
  3. 查账号：不存在/密码错 → 1003
  4. 账号禁用 → 1004
  5. 授权校验：该用户未被授权登录此平台 → 1006（不暴露用户是否存在）
  6. 签发不透明 token（secrets 随机 32 字节 hex），TTL 由平台侧自行决定（authPlatform 不吊销，可记录摘要+TTL 供 introspection）
  7. 写审计日志
返回:
{
  "code": 0,
  "data": {
    "token": "<opaque>",
    "expires_at": "2026-08-05T12:00:00+08:00",
    "user": {"uid": "u_xxx", "username": "alice", "nickname": "爱丽丝", "status": "active"}
  }
}
```

### 5.2 用户信息拉取（按授权过滤）

```
GET /api/users/{uid}?platform_id=xxx       （或 POST /api/users/batch）
GET /api/users?platform_id=xxx&keyword=    （平台视角列表）
返回: 仅该平台被授权的用户；未授权用户与认证中心超管（is_admin）对平台完全不可见（404/过滤掉）
字段白名单: uid / username / nickname / phone / email / status / created_at
绝不返回: password_hash 及任何登录凭据
```

> 平台同步时把白名单字段（含手机号/邮箱）写入平台本地用户表，用于展示与授权配置。

### 5.3 用户管理（管理侧，UI 或管理 API）

```
POST   /api/admin/users              创建用户（用户名唯一、密码策略校验）
PUT    /api/admin/users/{id}         更新（昵称/状态/重置密码）
DELETE /api/admin/users/{id}         删除（级联清理授权）
GET    /api/admin/users              用户列表（含各平台授权概览）
POST   /api/admin/users/{id}/grants  配置用户可登录的平台集合
```

### 5.4 平台管理

```
POST   /api/admin/platforms          创建平台（生成 platform_id + secret，secret 仅返回一次）
PUT    /api/admin/platforms/{id}     更新（启用状态/IP 白名单）
POST   /api/admin/platforms/{id}/rotate-secret   轮换盐
GET    /api/admin/platforms          平台列表（secret 脱敏）
```

### 5.5 修改密码 / 修改资料

平台将用户操作转发到 authPlatform：

```
POST /api/auth/change-password   {old_password, new_password, platform_id}（验签名+授权）
POST /api/auth/update-profile    {nickname, email, phone, password, totp_secret, platform_id}（验签名+授权）
```

**`update-profile` 承载所有认证中心资料变更**（约定：逻辑在平台处理、数据在认证中心存储）：
- `nickname` / `email` / `phone`：非空更新；`email`/`phone` 传 `""` 表示清空（存 NULL）；不传表示不修改
- `password`：非空则管理员代改密码（无需旧密码），认证中心哈希存储
- `totp_secret`：非空=重新绑定并启用 TOTP；`""`=清除双因子；不传=不修改

### 5.6 错误码

| code | 含义 |
|---|---|
| 0 | 成功 |
| 1001 | 平台签名无效 / 时间戳过期 |
| 1002 | 平台不存在或已停用 |
| 1003 | 账号或密码错误 |
| 1004 | 账号已禁用 |
| 1005 | 登录尝试过多（authPlatform 侧兜底锁定） |
| 1006 | 该用户未授权登录此平台 |
| 1007 | 参数错误 |
| 1008 | 用户名已存在 |
| 2001 | 内部错误 |

---

## 6. 数据模型（MySQL）

```sql
users (
  id            BIGINT PK AUTO_INCREMENT,
  uid           VARCHAR(32) UNIQUE,      -- 对外标识 u_xxx
  username      VARCHAR(64) UNIQUE,
  password_hash VARCHAR(255),            -- argon2id/bcrypt
  nickname      VARCHAR(64) DEFAULT '',
  status        TINYINT DEFAULT 1,       -- 1 启用 0 禁用
  created_at, updated_at
)

platforms (
  id            BIGINT PK,
  platform_id   VARCHAR(64) UNIQUE,
  name          VARCHAR(128),
  secret        VARCHAR(128),            -- 独立加密盐（密文存储）
  ip_whitelist  TEXT,                    -- JSON 数组，可选
  status        TINYINT DEFAULT 1,
  created_at, updated_at
)

user_platform_grants (
  id           BIGINT PK,
  user_id      BIGINT,
  platform_id  BIGINT,
  status       TINYINT DEFAULT 1,        -- 1 授权 0 撤销
  created_at,
  UNIQUE(user_id, platform_id)
)

login_logs (
  id            BIGINT PK,
  username      VARCHAR(64),
  platform_id   VARCHAR(64),
  success       TINYINT,                 -- 1/0
  reason        VARCHAR(32),             -- ok/bad_cred/disabled/unauthorized
  ip            VARCHAR(45),
  created_at    DATETIME INDEX
)

tokens（可选，仅当需要 introspection 能力）(
  token_hash   VARCHAR(64) PK,           -- sha256(token)
  user_id      BIGINT,
  platform_id  VARCHAR(64),
  expires_at   DATETIME,
  created_at
)
```

- 密码哈希：argon2id（推荐）或 bcrypt，禁止明文/MD5/SHA1。
- `platforms.secret` 加密存储（主密钥 AES-GCM，主密钥环境变量注入）。

---

## 7. 独立 UI 设计（管理界面）

Go 后端提供 API，前端为独立 SPA，**Vue 3 + Element Plus 走 CDN 引入（无 npm 构建环节）**，静态页面由 Go `embed` 内嵌提供（`web/` 目录直接挂载，浏览器从 CDN 加载 `vue.global.js` / `element-plus`）。

页面：

| 页面 | 功能 |
|---|---|
| 登录页 | 管理员登录（本系统管理员账号，可与普通用户同表加角色字段） |
| 用户管理 | 用户增删改查、启用/禁用、重置密码 |
| 平台管理 | 平台增删改查、**签发/查看 secret（仅创建时展示一次）**、IP 白名单、密钥轮换 |
| 授权管理 | 用户 ↔ 平台授权矩阵（勾选式：用户行 × 平台列） |
| 审计日志 | 登录成功/失败记录查询 |

---

## 8. 安全要求

1. HTTPS 强制；签名 HMAC-SHA256 + 时间戳窗口防重放。
2. 密码 argon2id/bcrypt；密码策略（长度/复杂度）由 authPlatform 统一执行。
3. 登录限流：authPlatform 侧兜底（账号维度失败锁定，如 5 次/15 分钟）；**平台侧主动防爆破**（各平台自己实现频率限制，authPlatform 不承担主要防爆）。
4. 密钥轮换机制；secret 脱敏展示、加密存储。
5. 授权过滤在**服务端**强制执行（拉取用户信息必须按平台过滤）。
6. 审计日志记录关键事件，日志只增不改。

---

## 9. 技术选型与目录结构（建议）

- 语言：Go 1.21+
- Web 框架：Gin 或标准库 `net/http`（本项目接口少，标准库亦可；推荐 Gin 提升效率）
- DB：MySQL 8（或 SQLite 起步，部署时切换——建议直接用 MySQL，避免迁移）
- 密码哈希：`golang.org/x/crypto/argon2` 或 `bcrypt`
- 签名/加密：标准库 `crypto/hmac`、`crypto/aes`、`crypto/cipher`
- 审计：MySQL 表（login_logs）

```
authPlatform/
├── cmd/server/main.go        # 入口
├── internal/
│   ├── config/               # 配置加载（yaml/env）
│   ├── model/                # 数据模型
│   ├── store/                # DB 访问
│   ├── api/                  # HTTP handlers（auth/admin/…）
│   ├── auth/                 # 密码哈希、token 签发、签名校验、加密盐
│   └── audit/                # 审计日志
├── web/                      # 独立 UI（SPA 或内嵌静态资源）
├── migrations/               # 表结构
└── DESIGN.md                 # 本文档
```

---

## 10. 里程碑与验收

| 里程碑 | 内容 | 验收 |
|---|---|---|
| M1 | 骨架 + 配置 + users 表 + 管理员登录 | 管理员可登录 UI，用户表可用 |
| M2 | 用户管理 CRUD + 密码策略 + 限流 | UI 完成用户增删改查/启禁用/重置密码 |
| M3 | 平台管理 + 独立盐签发 + 签名校验 | 创建平台获 secret，带签名调用验签通过/伪造拒绝 |
| M4 | 登录校验 + 授权 + 用户信息拉取 | 授权用户可登录；未授权用户登录被拒、拉取不可见 |
| M5 | 审计 + UI（授权矩阵/日志页）+ 安全加固 | 全流程联调通过，含密钥轮换 |

---

## 11. 开放问题（评审确认）

1. token 是否需要 introspection（`tokens` 表）？默认：不做，平台自管。
2. 管理员账号与普通用户同表（加 `is_admin`）还是独立表？默认：同表加角色。
3. UI 技术栈：✅ **已确认** —— Vue 3 + Element Plus 走 CDN，Go embed 内嵌静态页，无 npm 编译。
4. 数据库：✅ **已确认** —— 直接 MySQL 8。
5. 是否需要平台侧「用户资料同步」批量接口（全量拉取本平台授权用户）？默认提供 `GET /api/users?platform_id=` 列表。
