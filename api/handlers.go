package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"authplatform/common"
)

type Server struct {
	Users     *common.UserStore
	Platforms *common.PlatformStore
	Grants    *common.GrantStore
	Audit     *common.AuditStore
	Settings  *common.SettingsStore // 系统设置（密码策略/限流/登录方式/后台IP白名单）
	Tickets   *common.TicketStore   // 多步骤登录临时票据
	VerCodes  *common.VerCodeStore  // 登录验证码（dev 模式）
	Secret    string
	MasterKey string
	TokenTTL  time.Duration
	Limiter   *common.RateLimiter // 登录限流（账号维度，管理端与平台侧共用）
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
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	ip := clientIP(r)
	// 后台登录 IP 白名单（内网限制；未配置则不限制）
	wl := s.Settings.GetAdminIPWhitelist(r.Context())
	if len(wl.IPs) > 0 && !ipInList(ip, wl.IPs) {
		_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "ip_denied", ip)
		Fail(w, CodeIPDenied, "IP 不在后台登录白名单")
		return
	}
	// 登录限流 + 黑名单（账号维度）
	if err := s.Limiter.Check(req.Username); err != nil {
		if errors.Is(err, common.ErrBanned) {
			_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "banned", ip)
			Fail(w, CodeLocked, err.Error())
			return
		}
		_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "locked", ip)
		Fail(w, CodeLocked, err.Error())
		return
	}
	u, err := s.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			s.Limiter.RecordFail(req.Username)
			_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "bad_cred", ip)
			Fail(w, CodeBadCred, "账号或密码错误")
			return
		}
		s.internalError(w, err)
		return
	}
	if !u.IsAdmin {
		s.Limiter.RecordFail(req.Username)
		_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "bad_cred", ip)
		Fail(w, CodeBadCred, "账号或密码错误")
		return
	}
	ok, err := common.VerifyPassword(u.PasswordHash, req.Password)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !ok {
		s.Limiter.RecordFail(req.Username)
		_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "bad_cred", ip)
		Fail(w, CodeBadCred, "账号或密码错误")
		return
	}
	if u.Status != 1 {
		_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "disabled", ip)
		Fail(w, CodeDisabled, "账号已禁用")
		return
	}
	s.Limiter.Reset(req.Username)
	_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 1, "ok", ip)
	token, err := common.SignSessionToken(s.Secret, u.ID, s.TokenTTL)
	if err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, map[string]any{"token": token, "user": u.SafeUser()})
}

// Me 返回当前登录管理员信息（验证管理 token 有效性）。
func (s *Server) Me(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(CtxKeyUserID).(int64)
	u, err := s.Users.GetByID(r.Context(), userID)
	if err != nil {
		s.internalError(w, err)
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
		s.internalError(w, err)
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
		s.internalError(w, err)
		return
	}
	if err := s.Users.Update(r.Context(), userID, map[string]any{"password_hash": hash}); err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, nil)
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	log.Printf("[ERROR] %v", err)
	Fail(w, CodeInternal, "内部错误")
}
