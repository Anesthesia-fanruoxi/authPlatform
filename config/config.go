// Package config 负责从环境变量加载运行配置，未设置的项使用开发默认值。
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	// Addr HTTP 监听地址
	Addr string
	// DB 连接参数
	DBHost string
	DBPort string
	DBUser string
	DBPass string
	DBName string
	// TokenSecret 管理会话 token 的 HMAC 签名密钥（生产环境必须通过环境变量注入）
	TokenSecret string
	// MasterKey 平台 secret 加密主密钥（32 字节 hex，AES-256-GCM；生产必须注入）
	MasterKey string
	// AdminUsername / AdminPassword 初始管理员账号（仅当 users 表为空时创建）
	AdminUsername string
	AdminPassword string
	// TokenTTL 管理会话有效期
	TokenTTL time.Duration
}

func Load() *Config {
	return &Config{
		Addr:          getenv("APP_ADDR", ":8080"),
		DBHost:        getenv("DB_HOST", "192.168.6.2"),
		DBPort:        getenv("DB_PORT", "3306"),
		DBUser:        getenv("DB_USER", "root"),
		DBPass:        getenv("DB_PASS", "root"),
		DBName:        getenv("DB_NAME", "authplatform"),
		TokenSecret:   getenv("TOKEN_SECRET", "dev-token-secret-change-me"),
		MasterKey:     getenv("MASTER_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
		AdminUsername: getenv("ADMIN_USERNAME", "admin"),
		AdminPassword: getenv("ADMIN_PASSWORD", "admin123"),
		TokenTTL:      12 * time.Hour,
	}
}

// DSN 返回带库名的连接串。
func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&multiStatements=true&loc=Local",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBName)
}

// AdminDSN 返回不带库名的连接串（用于建库）。
func (c *Config) AdminDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/?parseTime=true&multiStatements=true&loc=Local",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort)
}

// DBNameIdent 返回可用于 SQL 标识符的库名（去除反引号防注入）。
func (c *Config) DBNameIdent() string {
	return strings.ReplaceAll(c.DBName, "`", "")
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
