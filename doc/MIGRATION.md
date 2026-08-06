# CMDB → authPlatform 用户迁移指南

> 场景：CMDB（`sys_users`，RuoYi 系）用户迁移到统一鉴权中心。
> 前提：CMDB 用户**不使用密码登录**，登录方式为「用户名 + TOTP 双因子」；
> authPlatform 已支持登录方式 `username_totp`（用户名 + TOTP 验证码，无密码）。
> CMDB 与 authPlatform **不在同一数据库实例**，分两步：① 在 CMDB 查询导出 → ② 在 authPlatform 插入。

## 1. CMDB 侧查询 SQL（导出用户 + 双因子数据）

在 **CMDB 库**执行，结果可直接导出 CSV（注意导出时保持 `utf8mb4` 字符集，避免中文乱码）：

```sql
-- ① 用户基本信息 + 双因子（列与 authplatform.users 对齐）
SELECT
  user_name            AS username,         -- -> users.username
  nick_name            AS nickname,         -- -> users.nickname
  phone                AS phone,            -- 空值导入时转 NULL（唯一索引防空串冲突）
  email                AS email,            -- 同上
  is_enabled           AS status,           -- 1=启用 0=禁用 -> users.status
  otp_secret           AS totp_secret,      -- base32 明文，两表格式一致 -> users.totp_secret
  otp_enabled          AS totp_enabled,     -- 1/0 -> users.totp_enabled
  created_at,                               -- -> users.created_at
  updated_at                                -- -> users.updated_at
FROM sys_users
-- 提示：只导 otp_enabled=1 的用户可直接使用 username_totp 登录；
-- otp_enabled=0 的用户迁移后无法用该方式登录，需平台侧 POST /api/auth/totp/save 补绑。
-- WHERE otp_enabled = 1
ORDER BY id;

-- ② 备用恢复码（otp_backup_codes JSON 原文，供 §3 展开到 authplatform.otp_backup_codes）
SELECT
  user_name            AS username,         -- 关联 authplatform.users.username
  otp_backup_codes     AS otp_backup_codes_json
FROM sys_users
WHERE otp_backup_codes IS NOT NULL AND otp_backup_codes <> '';
```

**不迁移字段**：`password`（认证中心不用密码登录，插入时写随机占位哈希）、
`role_id / dept_id / online_assets / test_assets`（业务数据归平台本地自管）、
`is_default_pass / allow_password_login`（认证中心无此概念）、`otp_setup_completed`（由 `totp_enabled` 隐含）。

## 2. authPlatform 侧插入 SQL（users 表）

在 **authPlatform 库**执行（`uid` 用 `u_`+16 位 hex 生成；`LEFT JOIN` 跳过已存在用户名，可重复执行；
`NULLIF` 把空 phone/email 转 NULL 避免撞唯一索引；`password_hash` 存随机不可用占位，`username_totp` 路径永不校验密码）：

```sql
INSERT INTO authplatform.users
  (uid, username, password_hash, nickname, phone, email,
   totp_secret, totp_enabled, category, status, is_admin, created_at, updated_at)
SELECT
  CONCAT('u_', LOWER(HEX(RANDOM_BYTES(8)))),
  tmp.username,
  CONCAT('$argon2id$v=19$m=65536,t=3,p=2$', LOWER(HEX(RANDOM_BYTES(16))), '$unused'), -- 随机占位哈希（不可登录）
  tmp.nickname,
  NULLIF(tmp.phone, ''),
  NULLIF(tmp.email, ''),
  tmp.totp_secret,
  IF(tmp.totp_enabled = 1, 1, 0),
  '',                       -- category 迁移后由管理员打标
  IF(tmp.status = 1, 1, 0),
  0,                        -- 普通用户；超管（is_admin=1）另行初始化
  COALESCE(tmp.created_at, NOW()),
  COALESCE(tmp.updated_at, NOW())
FROM (
  -- 把第 1 节导出的数据按下列列名粘贴到此处（可先用临时表/CSV 导入）
  SELECT 'alice' AS username, '爱丽丝' AS nickname, '13800000000' AS phone, 'alice@example.com' AS email,
         1 AS status, 'JBSWY3DPEHPK3PXP' AS totp_secret, 1 AS totp_enabled,
         CAST(NULL AS CHAR) AS otp_backup_codes_json, NOW() AS created_at, NOW() AS updated_at
  UNION ALL
  SELECT 'bob' AS username, '鲍勃' AS nickname, NULL AS phone, NULL AS email,
         1 AS status, NULL AS totp_secret, 0 AS totp_enabled,
         CAST(NULL AS CHAR) AS otp_backup_codes_json, NOW(), NOW()
) tmp
LEFT JOIN authplatform.users u ON u.username = tmp.username COLLATE utf8mb4_general_ci
WHERE u.id IS NULL;
```

> 实际使用：把第 1 节导出结果导入一张临时表（或按上述 `UNION ALL` 行拼接），再执行本 INSERT。
>
> **⚠️ collation 冲突**：CMDB 表为 `utf8mb4_0900_ai_ci`，authPlatform 为 `utf8mb4_general_ci`，
> JOIN 比较用户名时必须显式指定 collation（下方 SQL 已加 `COLLATE utf8mb4_general_ci`）；
> 建临时表导入数据时建议一并指定 `COLLATE utf8mb4_general_ci` 或 `CONVERT TO CHARACTER SET utf8mb4`。

## 3. 恢复码迁移（otp_backup_codes 表，MySQL 8+ JSON_TABLE）

表已由认证中心 AutoMigrate 自动创建（无需手动建表）。恢复码 JSON 展开插入：

```sql
INSERT INTO authplatform.otp_backup_codes (user_id, code, created_at)
SELECT u.id, jt.code, COALESCE(tmp.updated_at, NOW())
FROM (
  -- 与第 2 节同一份临时数据（含 otp_backup_codes_json）
  SELECT 'alice' AS username, '["123456","654321"]' AS otp_backup_codes_json, NOW() AS updated_at
) tmp
JOIN authplatform.users u ON u.username = tmp.username COLLATE utf8mb4_general_ci
JOIN JSON_TABLE(
  IFNULL(NULLIF(tmp.otp_backup_codes_json, ''), '[]'),
  '$[*]' COLUMNS (code VARCHAR(16) PATH '$')
) jt;
```

## 4. 迁移后检查

```sql
-- 用户数
SELECT COUNT(*) FROM authplatform.users WHERE is_admin = 0;
-- 已绑定 TOTP 用户数（可用 username_totp 登录）
SELECT COUNT(*) FROM authplatform.users WHERE is_admin = 0 AND totp_enabled = 1 AND totp_secret <> '';
-- 恢复码条数
SELECT COUNT(*) FROM authplatform.otp_backup_codes;
```
