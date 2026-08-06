# authPlatform 架构规范文档（可复用基础框架模板）

> 本文档既是 authPlatform 的架构说明，也是**后续创建新项目时可复制的基础框架规范**。
> 约定内容：目录组织、分层职责、接口约定、认证体系、安全基线、前后端开发方式。
> 新建项目时：复制本文档 + 按 §7 checklist 裁剪，替换 `authplatform` 模块名即可。

---

## 1. 总体架构

```
┌─────────────── 浏览器（管理端） ───────────────┐
│  CDN Vue3 + Element Plus（web/，Go embed 内嵌） │
│  hash 路由：/users /platforms /grants /logs      │
└──────────────┬──────────────────────────────────┘
               │ JSON API（Bearer 管理会话 token）
┌──────────────▼──────────────────────────────────┐
│                authPlatform（Go 单二进制）        │
│  main.go → router(net/http) → api → common → model│
└──────────────┬──────────────────────────────────┘
               │ 平台签名（HMAC-SHA256 + 时间戳）
┌──────────────▼──────────────────────────────────┐
│          接入平台（登录页在平台侧）                │
│  verify / 用户信息拉取 / 改密 / 改资料             │
└─────────────────────────────────────────────────┘
               ▲
               │ GORM（AutoMigrate 建表）
               ▼
            MySQL 8（users / platforms / user_platform_grants / login_logs）
```

**核心思想**：
- 后端单二进制（静态资源内嵌），前端 CDN 零构建。
- 管理端（UI）与平台侧（API）**两套认证体系**，互不混用。
- 数据访问统一 GORM；建表由 model 定义驱动（AutoMigrate）。

### 1.1 数据边界（定论，避免反复纠结）

```
authPlatform 负责（唯一身份源）       各平台本地负责（authPlatform 不参与）
├── 账号 / 密码 / 启用状态            ├── 部门 / 角色 / 权限
├── 用户 ↔ 平台 授权关系             ├── 业务数据 / 本地会话 / token 生命周期
└── 统一签发 token / 用户基础信息分发  └── 登录限流（平台侧主动防爆破）
```

- **无同步需求**：部门/角色/权限长在各平台本地，不同平台的组织架构本来就不一致，authPlatform 不需要也不应该知道。
- 平台从 `GET /api/users?platform_id=` 拉取用户基础信息（uid/username/nickname/status），**在自己的系统里**建本地权限表。
- authPlatform 只做三件事：验账号密码 → 校验授权 → 发 token；用户在其他平台是什么权限与 authPlatform 无关。

### 1.2 价值主张（为什么集中到 authPlatform）

- **账号事实源唯一**：账号禁用/改密/锁定在 authPlatform 一处生效，所有平台同步拒绝登录——「登录不了 = 账号有问题」只由 authPlatform 判定。
- **安全验证集中一处**：密码策略、账号维度失败锁定（5 次/15 分钟）、登录审计集中在 authPlatform，各平台直接享受兜底防护，无需各自实现账号安全逻辑。
- **边界提醒**（设计文档 §8.3）：登录页托管在各平台侧，平台登录页的验证码 / IP 频率限制等**入口防护**仍由平台自己实现；authPlatform 的账号维度限流是**最后一道兜底**，不承担主要防爆。

---

## 2. 目录组织规范（新项目模板）

```
<project>/
├── main.go            # 入口：config → 连库 → 初始化 → 启动。禁止放业务
├── api/               # HTTP 层。按业务域拆文件（admin_xxx.go / xxx_api.go）
├── common/            # 业务+数据层。不 import api/router；纯逻辑可单测
├── model/             # GORM 模型。一行 struct + tag = 一张表
├── router/            # 路由注册 + 中间件。New(s *api.Server) http.Handler
├── config/            # 环境变量加载（getenv + 默认值，集中一处）
└── web/               # 前端静态资源（embed）+ embed.go
```

**分层铁律**：
1. 依赖方向单向：`main → router → api → common → model`；`config` 由 main 加载后下传。
2. `api` 不写 SQL；`common` 不碰 `http.ResponseWriter`；`model` 不写逻辑。
3. 所有 HTTP 响应走 `api.OK/Fail` 统一包装，禁止 handler 里散落 `w.Write(...)`。

---

## 3. 后端接口约定

### 3.1 统一响应（api/response.go）

```json
成功：{"code": 0,    "msg": "ok", "data": {...}}
失败：{"code": 1003, "msg": "账号或密码错误", "data": null}
```

- HTTP 状态统一 200，业务结果以 `code` 表达（内部系统惯例，简单一致）。
- 纯资源 404 场景（如未授权用户拉取）允许 HTTP 404。

### 3.2 错误码表（新增业务错误沿用此段号分配）

| 段 | 用途 |
|---|---|
| 0 | 成功 |
| 1001-1008 | 业务错误（认证/参数/冲突） |
| 2001 | 内部错误（写日志后返回，不泄露细节） |

### 3.3 路由规范（Go 1.22+ ServeMux）

```go
mux := http.NewServeMux()
mux.HandleFunc("GET  /api/health", s.Health)
mux.HandleFunc("POST /api/admin/login", s.Login)
mux.HandleFunc("PUT  /api/admin/users/{id}", adminAuth(secret, s.UpdateUser)) // 路径参数 r.PathValue("id")
mux.HandleFunc("/", serveWeb) // 静态兜底
return withRecovery(withLogging(mux))
```

- 方法 + 路径写死在 pattern 里（`"GET /api/xxx"`），天然 405。
- 中间件用**函数包装器**（`adminAuth(secret, next)`）而非框架洋葱模型。
- 需要登录的接口统一 `adminAuth` 包装，通过 `context.WithValue` 传递用户 ID。

### 3.4 认证体系（两套）

**A. 管理后台接口**（`/api/admin/*`，仅管理员）
- 无状态 HMAC token：`payload("userID.exp") + "." + HMAC-SHA256(secret, payload)`。
- `Authorization: Bearer <token>`，`TOKEN_SECRET` 签名，12h 过期。
- 中间件 `adminAuth` **每次请求实时查库**校验 `is_admin && status==1`：管理员被降级/禁用/删除后旧 token 立即失效。
- 普通用户登录管理后台一律 1003（不暴露账号存在性）。

**B. 用户认证接口**（`/api/auth/*`、`/api/users/*`，平台侧）
```
sign = HMAC-SHA256(secret, method|完整RequestURI|timestamp|sha256(body))
Headers: X-Platform-Id / X-Timestamp(±300s 防重放) / X-Sign
```
- 每平台独立盐（AES-GCM 加密存储）；双盐过渡期新旧可验，吊销后旧盐立即失效。
- 签名 path 使用**完整 RequestURI（含 query）**，防 query 篡改绕过授权。
- 平台接口统一先过 `verifyPlatformRequest()`（验签→状态→IP 白名单→返回平台），失败已写响应。

### 3.5 接口划分总表（两类接口互不混用）

| 类别 | 路径 | 认证 | 面向 |
|---|---|---|---|
| 管理后台 | `POST /api/admin/login` | 账号密码（须 is_admin） | 管理员 |
| 管理后台 | `GET /api/admin/me` | Bearer 管理 token + is_admin | 管理员 |
| 管理后台 | `GET/POST /api/admin/users`、`PUT/DELETE /api/admin/users/{id}`、`POST /api/admin/users/{id}/reset-password` | Bearer + is_admin | 管理员 |
| 管理后台 | `GET/POST /api/admin/platforms`、`PUT/DELETE /api/admin/platforms/{id}`、`POST /api/admin/platforms/{id}/rotate-secret` | Bearer + is_admin | 管理员 |
| 管理后台 | `GET /api/admin/grants`、`POST /api/admin/users/{id}/grants` | Bearer + is_admin | 管理员 |
| 管理后台 | `GET /api/admin/logs` | Bearer + is_admin | 管理员 |
| 用户认证 | `POST /api/auth/verify` | 平台签名 | 接入平台 |
| 用户认证 | `POST /api/auth/change-password`、`POST /api/auth/update-profile` | 平台签名 + 授权 | 接入平台 |
| 用户认证 | `GET /api/users/{uid}`、`GET /api/users` | 平台签名 | 接入平台 |

> 规则：`/api/admin/*` 永远走 adminAuth（Bearer），`/api/auth/*`、`/api/users/*` 永远走 verifyPlatformRequest（平台签名），禁止混用。

### 3.7 用户自助操作与双因子（行为在平台，鉴权中心仅存储+统一鉴权）

修改密码、绑定双因子等**操作行为都发生在各接入平台**（平台登录页/个人中心完成），authPlatform 不提供绑定 UI，只负责**存储凭据信息 + 统一鉴权校验**：

| 操作 | 发起方 | 路径 | 鉴权中心做的事 |
|---|---|---|---|
| 修改密码 | 平台侧 | `POST /api/auth/change-password` | 验签+授权+验旧密码 → 更新密码哈希 |
| 修改资料 | 平台侧 | `POST /api/auth/update-profile` | 验签+授权 → 更新昵称 |
| **绑定双因子** | **平台侧完成全流程**（生成密钥/扫码/验证码确认） | `POST /api/auth/totp/save` | 平台绑定成功后把 secret 上报，**仅存储**（验签+授权+base32 格式校验） |
| **双因子校验** | 平台侧登录流程 | `verify`（`totp` 登录方式） | 用存储的 secret 统一校验 6 位验证码 |

- **认证中心不做绑定**：不生成密钥、不提供绑定交互；绑定成败由平台负责，绑定完成只上报 `{username, secret}`。
- **校验集中**：登录时 TOTP 验证码一律由 authPlatform 校验（`verifyCredential` 的 `totp` 分支），平台无需实现 TOTP 算法。
- **认证中心不提供绑定/解绑 UI**（无 generate/enable/disable 接口）：绑定与解绑均由平台侧完成；管理后台用户列表仅展示「是否已绑定」状态（`totp_enabled`）。

---

## 4. 安全基线（直接沿用）

| 项 | 实现 |
|---|---|
| 密码存储 | argon2id（`common/password.go`），禁止明文/MD5/SHA1 |
| 密码策略 | ≥8 位且含字母数字（`common/policy.go`），创建/重置/改密统一校验 |
| 登录限流 | 账号维度 5 次失败/15 分钟锁定（`common/ratelimit.go`），管理端与平台侧共用 |
| 敏感存储 | 平台盐 AES-256-GCM 加密（`MASTER_KEY` 注入），列表接口只返回脱敏前缀 |
| 审计 | 登录成功/失败写入 `login_logs`（只增不改），管理端与平台侧均记录 |
| 授权过滤 | 服务端强制：用户信息拉取仅返回「授权给该平台」的用户，未授权 404 |
| 自保护 | 管理端禁止禁用/删除当前登录账号 |

---

## 5. 数据层规范（GORM）

```go
// model 即表结构
type User struct {
    ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
    Username  string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
    CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}
func (User) TableName() string { return "users" }
```

- 连接时 `gorm.Config{TranslateError: true}`（把 MySQL 唯一冲突翻译为 `gorm.ErrDuplicatedKey`）。
- 建表只靠 `AutoMigrate(&model.A{}, &model.B{}, ...)`，**不手写 DDL/迁移脚本**。
- Store 模式：每实体一个 `XxxStore{db *gorm.DB}`，方法收 `context.Context`，错误统一映射 `ErrNotFound`。
- 更新用 `map[string]any`（避免零值字段被忽略）；多表写操作用 `Transaction`。

---

## 6. 前端规范（CDN Vue3 + Element Plus，零构建）

### 6.1 加载方式（web/index.html）
```html
<link rel="stylesheet" href="https://unpkg.com/element-plus/dist/index.css">
<script src="https://unpkg.com/vue@3/dist/vue.global.prod.js"></script>
<script src="https://unpkg.com/element-plus/dist/index.full.min.js"></script>
<script src="https://unpkg.com/element-plus/dist/locale/zh-cn.min.js"></script>
<script src="/js/app.js"></script>
```
> 所有静态文件在 `web/`，由 `//go:embed` 打进二进制，无 npm/构建环节。

### 6.2 代码组织（web/js/app.js，当前 4 页共 571 行）
```
app.js
├── api 封装        # request()：统一 JSON、注入 Bearer token、code!=0 抛错；各业务方法挂在其上
├── 页面组件        # const XxxPage = { template, setup() }，每页一个 const
├── 根组件 Root     # 未登录→登录卡；已登录→侧边栏布局 + <component :is="pageComponent">
└── createApp(Root).use(ElementPlus, {locale}).mount('#app')
```

### 6.3 新增页面步骤（约定）
1. `api` 对象上加请求方法（走统一 `request()`）。
2. 定义 `const XxxPage = { template, setup }`（用 Element Plus 组件拼界面）。
3. 根组件 `pageComponent` 的 switch 里加路由分支 + 侧边栏菜单项。

### 6.4 通用约定
- 页内数据加载：`loading` ref + `onMounted(load)` + 异常 `ElMessage.error`。
- 危险操作（删除/轮换/重置）先 `ElMessageBox.confirm`。
- 密码等敏感输入用 `show-password`；secret 一次性展示用只读输入框 + 复制按钮。

---

## 7. 新项目起步 checklist

1. `go mod init <module>`；复制 `main.go / router / config / api/response.go / common` 骨架。
2. 按业务域新增 `model` 与 `XxxStore`，加入 `AutoMigrate`。
3. 在 `api` 加 handler，在 `router.New` 注册（需要登录就包 `adminAuth`）。
4. 前端：复制 `web/`，改页面组件与菜单。
5. 环境变量：`APP_ADDR / DB_* / <业务密钥>` 全部收敛在 `config/config.go`。
6. 启动自检：`go vet ./...`、健康检查、管理员登录冒烟。

---

## 8. 配置项速查（config/config.go）

| 变量 | 用途 | 生产建议 |
|---|---|---|
| `APP_ADDR` | 监听地址 | 按部署 |
| `DB_HOST/PORT/USER/PASS/NAME` | MySQL | 强密码 + 独立库 |
| `TOKEN_SECRET` | 管理会话签名 | 随机长字符串，必须注入 |
| `MASTER_KEY` | 平台盐加密主密钥 | `openssl rand -hex 32`，必须注入 |
| `ADMIN_USERNAME/PASSWORD` | 初始管理员 | 首次启动后立即改密 |
