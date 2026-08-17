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
	// AdminUsername / AdminPassword 初始管理员账号（仅当 users 表为空时创建）
	AdminUsername string
	AdminPassword string
	// TokenTTL 管理会话有效期
	TokenTTL time.Duration
}

// Load 加载配置：优先 config.yaml → 环境变量覆盖。
// 若 config.yaml 不存在，使用环境变量默认值（Web 引导页会负责生成 config.yaml）。
func Load() *Config {
	cfgPath := ConfigFilePath()
	var cfg *Config

	if _, err := os.Stat(cfgPath); err == nil {
		// config.yaml 存在，从 YAML 加载
		var yamlErr error
		cfg, yamlErr = loadFromYAML(cfgPath)
		if yamlErr != nil {
			fmt.Printf("[WARN] 加载 config.yaml 失败: %v，使用默认值\n", yamlErr)
			cfg = loadFromEnv()
		}
	} else {
		// config.yaml 不存在，使用默认值（Web 引导页会负责配置）
		cfg = loadFromEnv()
	}

	// 环境变量覆盖 YAML（便于容器/Docker 部署时临时改值）
	applyEnvOverrides(cfg)
	return cfg
}

// loadFromEnv 纯环境变量加载（向导失败时的降级路径）。
func loadFromEnv() *Config {
	return &Config{
		Addr:          getenv("APP_ADDR", ":8080"),
		DBHost:        getenv("DB_HOST", "127.0.0.1"),
		DBPort:        getenv("DB_PORT", "3306"),
		DBUser:        getenv("DB_USER", "root"),
		DBPass:        getenv("DB_PASS", "root"),
		DBName:        getenv("DB_NAME", "authplatform"),
		TokenSecret:   getenv("TOKEN_SECRET", ""),
		AdminUsername: getenv("ADMIN_USERNAME", "admin"),
		AdminPassword: getenv("ADMIN_PASSWORD", "admin123"),
		TokenTTL:      12 * time.Hour,
	}
}

// applyEnvOverrides 环境变量覆盖（便于容器部署临时改值）。
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("APP_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.DBHost = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		cfg.DBPort = v
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.DBUser = v
	}
	if v := os.Getenv("DB_PASS"); v != "" {
		cfg.DBPass = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.DBName = v
	}
	if v := os.Getenv("TOKEN_SECRET"); v != "" {
		cfg.TokenSecret = v
	}
	if v := os.Getenv("ADMIN_USERNAME"); v != "" {
		cfg.AdminUsername = v
	}
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		cfg.AdminPassword = v
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
