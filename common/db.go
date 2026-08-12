// Package common 提供公共能力：数据库连接与建表（GORM）、密码哈希、会话 token、用户存取、初始管理员引导。
package common

import (
	"database/sql"
	"fmt"

	"authplatform/config"
	"authplatform/model"
	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// OpenDB 连接 MySQL（GORM）：先确保目标数据库存在，再建立连接并 AutoMigrate 建表。
func OpenDB(cfg *config.Config) (*gorm.DB, error) {
	// 1. 连接服务器（不带库），必要时创建数据库
	admin, err := sql.Open("mysql", cfg.AdminDSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql (admin): %w", err)
	}
	if err := admin.Ping(); err != nil {
		admin.Close()
		return nil, fmt.Errorf("ping mysql %s:%s: %w", cfg.DBHost, cfg.DBPort, err)
	}
	createDB := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci", cfg.DBNameIdent())
	if _, err := admin.Exec(createDB); err != nil {
		admin.Close()
		return nil, fmt.Errorf("create database: %w", err)
	}
	admin.Close()

	// 2. GORM 连接目标库（TranslateError 将 MySQL 错误翻译为 gorm.ErrDuplicatedKey 等）
	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("open mysql (gorm): %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)

	// 2.5 历史数据修复：phone/email 唯一索引要求空值使用 NULL（'' 会冲突）
	mig := db.Migrator()
	if mig.HasColumn(&model.User{}, "phone") {
		if err := mig.AlterColumn(&model.User{}, "Phone"); err != nil { // 依据模型改为可空
			return nil, fmt.Errorf("alter phone column: %w", err)
		}
		if err := db.Exec("UPDATE users SET phone = NULL WHERE phone = ''").Error; err != nil {
			return nil, fmt.Errorf("fix phone column: %w", err)
		}
	}
	if mig.HasColumn(&model.User{}, "email") {
		if err := mig.AlterColumn(&model.User{}, "Email"); err != nil {
			return nil, fmt.Errorf("alter email column: %w", err)
		}
		if err := db.Exec("UPDATE users SET email = NULL WHERE email = ''").Error; err != nil {
			return nil, fmt.Errorf("fix email column: %w", err)
		}
	}

	// 3. 按 model 定义建表（AutoMigrate）
	if err := db.AutoMigrate(&model.User{}, &model.Platform{}, &model.UserPlatformGrant{}, &model.LoginLog{}, &model.SysSetting{}, &model.OTPBackupCode{}, &model.RequestLog{}); err != nil {
		return nil, fmt.Errorf("automigrate: %w", err)
	}
	return db, nil
}
