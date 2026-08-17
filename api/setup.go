package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"authplatform/config"
)

// setupRequest 初始化页请求体（保存与连通测试共用）。
type setupRequest struct {
	DBHost      string `json:"db_host"`
	DBPort      string `json:"db_port"`
	DBUser      string `json:"db_user"`
	DBPass      string `json:"db_pass"`
	DBName      string `json:"db_name"`
	TokenSecret string `json:"token_secret"`
	AdminUser   string `json:"admin_user"`
	AdminPass   string `json:"admin_pass"`
}

// SetupAvailable 检查是否需要引导配置（供前端判断）。
func (s *Server) SetupAvailable(w http.ResponseWriter, _ *http.Request) {
	OK(w, map[string]any{"available": !s.Initialized && !config.HasConfigFile()})
}

// GenKey 生成随机密钥（32 字节 hex）。
func (s *Server) GenKey(w http.ResponseWriter, r *http.Request) {
	key, err := config.GenerateRandomHex(32)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	OK(w, map[string]any{"key": key})
}

// TestConn POST /api/setup/test 测试 MySQL 连通性。
func (s *Server) TestConn(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	if err := pingMySQL(req); err != nil {
		Fail(w, CodeBadParam, "MySQL 连接失败: "+err.Error())
		return
	}
	OK(w, map[string]any{"message": "连接测试通过"})
}

// SaveSetup 保存引导配置：连通测试 → 生成 config.yaml → 触发热初始化（无需重启）。
func (s *Server) SaveSetup(w http.ResponseWriter, r *http.Request) {
	// 已完成初始化时拒绝（setup 模式下即使存在残留配置文件也允许覆盖重试）
	if s.Initialized {
		Fail(w, CodeBadParam, "配置已存在，请勿重复配置")
		return
	}
	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	// 必填校验
	if req.TokenSecret == "" {
		Fail(w, CodeBadParam, "TOKEN_SECRET 不能为空")
		return
	}
	if req.AdminPass == "" {
		Fail(w, CodeBadParam, "管理员密码不能为空")
		return
	}
	// 保存前连通测试（避免写出不可用的配置）
	if err := pingMySQL(req); err != nil {
		Fail(w, CodeBadParam, "MySQL 连接失败: "+err.Error())
		return
	}

	yc := buildYAMLConfig(req)
	if err := config.SaveConfigFile(config.ConfigFilePath(), yc); err != nil {
		s.internalError(w, r, fmt.Errorf("写入 config.yaml 失败: %w", err))
		return
	}

	// 热初始化：重新加载配置 → 连库 → 切换完整路由
	if s.OnSetupSaved != nil {
		if err := s.OnSetupSaved(config.Load()); err != nil {
			Fail(w, CodeInternal, "配置已保存，但服务初始化失败: "+err.Error())
			return
		}
	}
	OK(w, map[string]any{
		"message":  "配置已保存并生效",
		"config":   config.ConfigFilePath(),
		"username": yc.Admin.Username,
	})
}

// buildYAMLConfig 由请求参数构造 YAML 配置。
func buildYAMLConfig(req setupRequest) *config.YAMLConfig {
	port := 3306
	if v, err := strconv.Atoi(req.DBPort); err == nil && v > 0 {
		port = v
	}
	adminUser := strings.TrimSpace(req.AdminUser)
	if adminUser == "" {
		adminUser = "admin"
	}
	yc := &config.YAMLConfig{}
	yc.App.Addr = ":8080"
	yc.App.Name = "authPlatform"
	yc.Database.Host = nonEmptyStr(req.DBHost, "127.0.0.1")
	yc.Database.Port = port
	yc.Database.User = nonEmptyStr(req.DBUser, "root")
	yc.Database.Pass = req.DBPass
	yc.Database.Name = nonEmptyStr(req.DBName, "authplatform")
	yc.Secrets.TokenSecret = req.TokenSecret
	yc.Admin.Username = adminUser
	yc.Admin.Password = req.AdminPass
	yc.Token.TTL = "12h"
	return yc
}

// pingMySQL 测试 MySQL 连通性（仅连接服务器，不建库；建库由 OpenDB 负责）。
func pingMySQL(req setupRequest) error {
	port := 3306
	if v, err := strconv.Atoi(req.DBPort); err == nil && v > 0 {
		port = v
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?timeout=3s",
		nonEmptyStr(req.DBUser, "root"), req.DBPass, nonEmptyStr(req.DBHost, "127.0.0.1"), port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	db.SetConnMaxLifetime(time.Minute)
	return db.Ping()
}

func nonEmptyStr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
