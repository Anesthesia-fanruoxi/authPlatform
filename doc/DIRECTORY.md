# authPlatform 目录结构说明

> 本文档描述当前项目的完整目录/文件结构及各部分职责，并给出**迁移指南**（在新环境重建本项目的最小步骤）。
> 技术栈：Go 1.25（原生 `net/http` 路由 + GORM）+ MySQL 8 + CDN Vue3/Element Plus 前端（无 npm 构建）。

---

## 1. 目录树

```
authPlatform/
├── main.go                          # 程序入口：加载配置 → 连库/建表 → 初始化管理员 → 启动 HTTP
├── go.mod                           # Go 模块定义（module authplatform）
├── go.sum                           # 依赖校验和
├── authPlatform设计规格.md           # 产品设计文档（需求/API/错误码/里程碑）
├── DIRECTORY.md                     # 本文档（目录说明 + 迁移指南）
├── ARCHITECTURE.md                  # 架构规范文档（可复用基础框架模板）
│
├── api/                             # HTTP 接口层（handler + 统一响应），按业务拆文件
│   ├── response.go                  #   统一响应 OK/Fail、业务错误码常量、context key 定义
│   ├── handlers.go                  #   Server 结构（持有各 store 与密钥）、健康检查、管理端登录 Login/Me
│   ├── platform_auth.go             #   平台请求验签公共逻辑（verifyPlatformRequest：签名/状态/IP白名单/新旧盐）+ IP/白名单字段工具
│   ├── admin_users.go               #   管理端用户 CRUD：列表/创建/更新/删除/重置密码
│   ├── admin_platforms.go           #   管理端平台 CRUD + 密钥轮换（双盐过渡/吊销）
│   ├── admin_grants.go              #   管理端授权：矩阵数据 / 设置用户授权集合
│   ├── admin_logs.go                #   管理端审计日志查询
│   ├── auth_verify.go               #   平台登录校验 POST /api/auth/verify
│   ├── auth_account.go              #   平台侧改密/改资料（change-password、update-profile）
│   └── users_api.go                 #   平台侧用户信息拉取（按授权过滤）
│
├── common/                          # 公共能力层（无 HTTP 依赖，可复用/可单测）
│   ├── db.go                        #   GORM 连接（含建库）+ AutoMigrate 建表（所有 model）
│   ├── users.go                     #   UserStore：用户存取（GORM）
│   ├── platforms.go                 #   PlatformStore：平台存取
│   ├── grants.go                    #   GrantStore：授权存取（事务全量替换/级联删除）
│   ├── audit.go                     #   AuditStore：登录审计写入/查询
│   ├── password.go                  #   argon2id 密码哈希（Hash/Verify）
│   ├── policy.go                    #   密码策略校验（≥8 位且含字母数字）
│   ├── token.go                     #   管理会话 token（HMAC-SHA256 无状态签名）
│   ├── opaque.go                    #   不透明 token 签发（32 字节随机 hex）
│   ├── secret.go                    #   平台盐 AES-256-GCM 加解密 + 随机盐生成
│   ├── signature.go                 #   平台请求签名校验（HMAC-SHA256 + ±300s 时间戳窗口）
│   ├── ratelimit.go                 #   登录限流（内存：5 次失败/15 分钟锁定）
│   ├── admin.go                     #   初始管理员引导 + UID 生成
│   └── token.go                     #   管理会话签名/校验
│
├── model/                           # 数据模型（GORM tag 即表结构，AutoMigrate 建表）
│   ├── user.go                      #   users 表（含 is_admin）
│   ├── platform.go                  #   platforms 表（secret 加密存储、双盐字段）
│   ├── grant.go                     #   user_platform_grants 表（用户↔平台授权）
│   └── login_log.go                 #   login_logs 表（登录审计）
│
├── router/                          # 路由层
│   └── router.go                    #   net/http ServeMux（Go 1.22 方法路由）+ adminAuth 中间件 + 日志/recovery + 静态页兜底
│
├── config/                          # 配置层
│   └── config.go                    #   环境变量加载 + 默认值 + DSN 构造
│
└── web/                             # 前端静态资源（Go embed 内嵌，CDN 引入 Vue3 + Element Plus，无构建）
    ├── embed.go                     #   //go:embed index.html js
    ├── index.html                   #   入口页（CDN script 引入 + 布局样式）
    └── js/
        └── app.js                   #   单页应用：api 封装 + 登录页 + 布局导航 + 四个页面组件
```

---

## 2. 分层与依赖方向

```
main.go ──▶ router ──▶ api ──▶ common ──▶ model
              │         │        │
              │         └── web（embed 静态资源，供 router 挂载）
              └── config（main 加载后逐层下传）
```

- **main.go**：只做组装（配置 → 连库 → 初始化 → 启动），不含业务逻辑。
- **router**：只做路由注册与中间件包装，不写业务。
- **api**：HTTP 层——解析请求、调用 common、包装统一响应。不直接操作 SQL。
- **common**：业务与数据访问——不感知 HTTP。新建项目时此层可整体复用。
- **model**：纯数据定义（GORM tag）。
- 依赖方向单向向下，禁止 `common` 引用 `api`、`model` 引用上层。

---

## 3. 依赖清单（go.mod）

| 依赖 | 用途 |
|---|---|
| `gorm.io/gorm` v1.31.2 + `gorm.io/driver/mysql` v1.6.0 | ORM 与 MySQL 驱动 |
| `github.com/go-sql-driver/mysql` v1.10.0 | 建库阶段直连驱动（GORM 底层复用） |
| `golang.org/x/crypto` v0.31.0 | argon2id 密码哈希 |

> 说明：路由使用 Go 标准库 `net/http`（1.22+ 方法路由），**无 Web 框架依赖**；前端 **无 npm/构建依赖**（CDN 引入）。

---

## 4. 环境变量与默认值（config/config.go）

| 变量 | 默认值 | 说明 |
|---|---|---|
| `APP_ADDR` | `:8080` | HTTP 监听地址 |
| `DB_HOST` / `DB_PORT` | `192.168.6.2` / `3306` | MySQL 地址 |
| `DB_USER` / `DB_PASS` | `root` / 空 | MySQL 凭据 |
| `DB_NAME` | `authplatform` | 库名（不存在会自动创建） |
| `TOKEN_SECRET` | `dev-token-secret-change-me` | 管理会话 token 签名密钥（**生产必须注入**） |
| `MASTER_KEY` | 开发默认 64 hex | 平台盐 AES-256-GCM 主密钥，32 字节 hex（**生产必须注入**） |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / `admin123` | 首次启动创建的初始管理员（表非空则跳过） |

---

## 5. 数据库说明

- **无需手动迁移**：启动时 `db.AutoMigrate(&model.User{}, &model.Platform{}, &model.UserPlatformGrant{}, &model.LoginLog{})` 自动建表/补列（幂等）。
- 表：`users`、`platforms`、`user_platform_grants`、`login_logs`。
- 字符集 utf8mb4；`platforms.secret_enc` 为 AES-GCM 密文，明文仅创建/轮换时返回一次。

---

## 6. 迁移指南（新环境重建）

```bash
# 1. 前置：Go 1.25+（go.mod 声明 go 1.25.0）、MySQL 8（无需预建库）
# 2. 拉代码后安装依赖
go mod tidy

# 3. 构建（产出单二进制，前端已内嵌，无需额外拷贝 web/）
go build -o authPlatform.exe .

# 4. 配置环境变量（Windows PowerShell 示例）
$env:DB_HOST   = "你的mysql地址"
$env:DB_USER   = "root"
$env:DB_PASS   = "你的密码"
$env:DB_NAME   = "authplatform"
$env:TOKEN_SECRET = "<随机字符串>"
$env:MASTER_KEY   = "<64位hex>"   # 32 字节随机数 hex，可用: openssl rand -hex 32
$env:ADMIN_PASSWORD = "<初始管理员密码>"

# 5. 启动（首次启动自动建库建表 + 创建初始管理员）
./authPlatform.exe

# 6. 验证
curl http://localhost:8080/api/health   # → {"code":0,"data":{"status":"ok"},"msg":"ok"}
# 浏览器打开 http://localhost:8080 用 admin 登录
```

> 迁移注意：
> 1. `MASTER_KEY` 变更会导致已存平台盐无法解密（历史 secret 失效），**迁移时保持与旧环境一致**。
> 2. `TOKEN_SECRET` 变更会使已登录管理会话全部失效（可接受）。
> 3. 数据迁移：直接拷贝 MySQL 库即可（表结构由 AutoMigrate 兼容补列）。
