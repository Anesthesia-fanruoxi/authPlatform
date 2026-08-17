package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"authplatform/common"
	"authplatform/config"
)

type Server struct {
	Users     *common.UserStore
	Platforms *common.PlatformStore
	Grants    *common.GrantStore
	Audit     *common.AuditStore
	Settings  *common.SettingsStore
	Tickets   *common.TicketStore
	VerCodes  *common.VerCodeStore
	Secret    string
	TokenTTL  time.Duration
	Limiter   *common.RateLimiter
	// Initialized 完整服务已初始化（防重复配置；setup 模式下为 false）
	Initialized bool
	// OnSetupSaved 热初始化回调：setup 保存配置后由 main 注入，负责连库并切换路由
	OnSetupSaved func(*config.Config) error
}

func (s *Server) Health(w http.ResponseWriter, _ *http.Request) {
	OK(w, map[string]any{"status": "ok"})
}

// LoginRequest 管理端登录请求。
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login 管理端登录：仅 is_admin 用户可登录；失败统一返回 1003 以隐藏账号存在性。
// 支持后台登录 IP 白名单（sys_settings.admin_ip_whitelist，空=不限制）。
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	// 请求头/请求体（脱敏）随审计记录：管理端登录成功/失败/参数错/IP 拒绝都留痕
	reqHeaders := common.SanitizeRequestHeaders(r.Header)
	reqBody := common.SanitizeRequestBody(body)
	audit := func(username string, success int, reason string) {
		if s.Audit != nil {
			_ = s.Audit.WriteLoginDetail(r.Context(), username, "", success, reason, clientIP(r), reqHeaders, reqBody)
		}
	}
	var req LoginRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Username == "" || req.Password == "" {
		audit("", 0, "bad_param")
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	ip := clientIP(r)
	// 后台登录 IP 白名单（内网限制；未配置则不限制）
	if s.Settings != nil {
		wl := s.Settings.GetAdminIPWhitelist(r.Context())
		if len(wl.IPs) > 0 && !ipInList(ip, wl.IPs) {
			audit(req.Username, 0, "ip_denied")
			Fail(w, CodeIPDenied, "IP 不在后台登录白名单")
			return
		}
	}
	// 登录限流 + 黑名单（账号维度）
	if err := s.Limiter.Check(req.Username); err != nil {
		if errors.Is(err, common.ErrBanned) {
			audit(req.Username, 0, "banned")
			Fail(w, CodeLocked, err.Error())
			return
		}
		audit(req.Username, 0, "locked")
		Fail(w, CodeLocked, err.Error())
		return
	}

	// 降级到 MySQL（Users 为 nil 时直接拒绝）
	if s.Users == nil {
		s.Limiter.RecordFail(req.Username)
		audit(req.Username, 0, "bad_cred")
		Fail(w, CodeBadCred, "数据库未连接")
		return
	}
	u, err := s.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			s.Limiter.RecordFail(req.Username)
			audit(req.Username, 0, "bad_cred")
			Fail(w, CodeBadCred, "账号或密码错误")
			return
		}
		s.internalError(w, r, err)
		return
	}
	if !u.IsAdmin {
		s.Limiter.RecordFail(req.Username)
		audit(req.Username, 0, "bad_cred")
		Fail(w, CodeBadCred, "账号或密码错误")
		return
	}
	ok, err := common.VerifyPassword(u.PasswordHash, req.Password)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !ok {
		s.Limiter.RecordFail(req.Username)
		audit(req.Username, 0, "bad_cred")
		Fail(w, CodeBadCred, "账号或密码错误")
		return
	}
	if u.Status != 1 {
		audit(req.Username, 0, "disabled")
		Fail(w, CodeDisabled, "账号已禁用")
		return
	}
	s.Limiter.Reset(req.Username)
	audit(req.Username, 1, "admin_login")
	token, err := common.SignSessionToken(s.Secret, u.ID, s.TokenTTL)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	// 管理后台单点登录：新登录覆盖旧 token，同一账号只生效一个会话
	_ = s.Users.Update(r.Context(), u.ID, map[string]any{"session_hash": common.HashToken(token)})
	OK(w, map[string]any{"token": token, "user": u.SafeUser()})
}

// Me 返回当前登录管理员信息（验证管理 token 有效性）。
func (s *Server) Me(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(CtxKeyUserID).(int64)
	u, err := s.Users.GetByID(r.Context(), userID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	OK(w, u.SafeUser())
}

// MeChangePassword 修改当前登录管理员自己的密码（头像下拉菜单个人设置）POST /api/admin/me/password。
func (s *Server) MeChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OldPassword == "" || req.NewPassword == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	u, err := s.Users.GetByID(r.Context(), userID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ok, _ := common.VerifyPassword(u.PasswordHash, req.OldPassword)
	if !ok {
		Fail(w, CodeBadCred, "原密码错误")
		return
	}
	policy := s.Settings.GetPasswordPolicy(r.Context())
	if err := common.ValidatePasswordWithPolicy(req.NewPassword, policy); err != nil {
		Fail(w, CodeBadParam, err.Error())
		return
	}
	hash, err := common.HashPassword(req.NewPassword)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.Users.Update(r.Context(), userID, map[string]any{"password_hash": hash}); err != nil {
		s.internalError(w, r, err)
		return
	}
	OK(w, nil)
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	common.LogError(r.Method+" "+r.URL.Path, "%v", err)
	Fail(w, CodeInternal, "内部错误")
}
