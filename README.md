# authPlatform 统一鉴权中心

内部统一登录与授权服务：集中管理账号、平台、授权三大实体，为各内部平台提供登录校验与用户信息分发。平台只转发凭据，不保存任何账号密码。

## 核心功能

- **用户管理**：账号 CRUD、分类、禁用、密码重置（argon2id 哈希）、黑名单
- **平台接入**：平台注册、独立签名盐、密钥轮换、IP 白名单、启停
- **授权管理**：用户↔平台授权矩阵，平台拉取用户信息按授权过滤
- **登录校验**：多种登录方式（用户名/邮箱/手机 + 密码、TOTP 双因子、验证码），支持单步与多步验证
- **安全防护**：请求签名（HMAC-SHA256 + 时间戳防重放）、登录限流、全量登录审计
- **零配置初始化**：首次启动无配置文件时自动进入 Web 初始化页，填写后热生效，无需重启

## 技术栈

Go 1.25（原生 `net/http` 路由 + GORM）+ MySQL 8 + Vue3/Element Plus（本地 vendor，无 npm 构建），编译产出单二进制，前端已内嵌。

## 快速开始

```bash
# 1. 前置：Go 1.25+、MySQL 8（无需预建库）
go mod tidy

# 2. 构建
go build -o authPlatform.exe .

# 3. 首次启动（无 config.yaml 自动进入 Web 初始化模式）
./authPlatform.exe
```

浏览器打开 `http://localhost:8080`，在初始化页填写 MySQL 连接、TOKEN_SECRET、管理员账号，保存后自动建库建表并切换为完整服务。

已有 `config.yaml` 时直接读取配置启动完整服务；环境变量（`DB_HOST`/`TOKEN_SECRET` 等）可覆盖配置文件。

## 文档索引

| 文档 | 说明 |
|---|---|
| [doc/接入文档.md](doc/接入文档.md) | 平台方接入对接：签名协议、接口、错误码 |
| [doc/接口调试文档.md](doc/接口调试文档.md) | Web 端/调试工具用的全量接口清单 |
| [doc/架构设计规范.md](doc/架构设计规范.md) | 架构说明 + 可复用的架构规范模板 |
| [doc/目录结构说明.md](doc/目录结构说明.md) | 目录/文件职责、分层依赖、迁移指南 |
| [doc/CMDB用户迁移指南.md](doc/CMDB用户迁移指南.md) | CMDB sys_users 用户迁移方案 |
| [doc/authPlatform设计规划.md](doc/authPlatform设计规划.md) | 产品设计规划（定位/功能/路线） |

## 目录概览

```
main.go     程序入口（setup 模式 ↔ 完整服务热切换）
api/        HTTP 接口层（管理端 + 平台侧 + Web 初始化）
common/     公共能力层（存取/签名/限流/TOTP 等，无 HTTP 依赖）
model/      数据模型（GORM tag 即表结构，AutoMigrate 建表）
router/     路由与中间件
config/     配置加载（config.yaml + 环境变量）
web/        前端静态资源（embed 内嵌，含初始化页）
doc/        项目文档
scripts/    本地联调脚本
```

详见 [doc/目录结构说明.md](doc/目录结构说明.md)。
