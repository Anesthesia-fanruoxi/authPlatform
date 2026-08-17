package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// YAMLConfig YAML 配置文件结构。
type YAMLConfig struct {
	App struct {
		Addr string `yaml:"addr"`
		Name string `yaml:"name"`
	} `yaml:"app"`
	Database struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
		User string `yaml:"user"`
		Pass string `yaml:"pass"`
		Name string `yaml:"name"`
	} `yaml:"database"`
	Secrets struct {
		TokenSecret string `yaml:"token_secret"`
	} `yaml:"secrets"`
	Admin struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"admin"`
	Token struct {
		TTL string `yaml:"ttl"`
	} `yaml:"token"`
}

// ConfigFilePath 返回配置文件路径：可执行文件同目录已存在则用之，否则用当前工作目录。
// （go run/IDE 启动时 exe 在临时目录，避免把 config.yaml 写进临时目录）
func ConfigFilePath() string {
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "config.yaml"
}

// HasConfigFile 检查配置文件是否存在。
func HasConfigFile() bool {
	_, err := os.Stat(ConfigFilePath())
	return err == nil
}

// loadFromYAML 从 YAML 文件加载配置。
func loadFromYAML(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}
	var yc YAMLConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}
	ttl := time.Duration(12) * time.Hour
	if yc.Token.TTL != "" {
		if d, err := time.ParseDuration(yc.Token.TTL); err == nil {
			ttl = d
		}
	}
	return &Config{
		Addr:          nonEmpty(yc.App.Addr, ":8080"),
		DBHost:        nonEmpty(yc.Database.Host, "127.0.0.1"),
		DBPort:        fmt.Sprintf("%d", nonZero(yc.Database.Port, 3306)),
		DBUser:        nonEmpty(yc.Database.User, "root"),
		DBPass:        yc.Database.Pass,
		DBName:        nonEmpty(yc.Database.Name, "authplatform"),
		TokenSecret:   yc.Secrets.TokenSecret,
		AdminUsername: nonEmpty(yc.Admin.Username, "admin"),
		AdminPassword: nonEmpty(yc.Admin.Password, "admin123"),
		TokenTTL:      ttl,
	}, nil
}

// SaveConfigFile 生成配置文件到指定路径（Web 初始化页保存时调用）。
func SaveConfigFile(path string, yc *YAMLConfig) error {
	data, err := yaml.Marshal(yc)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	// 加注释说明
	header := []byte("# authPlatform 配置文件（首次启动自动生成）\n# 修改后重启服务生效\n\n")
	return os.WriteFile(path, append(header, data...), 0600)
}

// GenerateRandomHex 生成指定字节数的随机 hex 字符串。
func GenerateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func nonEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func nonZero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
