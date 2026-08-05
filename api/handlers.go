package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/anesthesia-fanruoxi/authplatform/common"
)

type Server struct {
	Users     *common.UserStore
	Platforms *common.PlatformStore
	Grants    *common.GrantStore
	Audit     *common.AuditStore
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
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	// 登录限流（账号维度）
	if err := s.Limiter.Check(req.Username); err != nil {
		_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "locked", clientIP(r))
		Fail(w, CodeLocked, err.Error())
		return
	}
	u, err := s.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			s.Limiter.RecordFail(req.Username)
			_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "bad_cred", clientIP(r))
			Fail(w, CodeBadCred, "账号或密码错误")
			return
		}
		s.internalError(w, err)
		return
	}
	if !u.IsAdmin {
		s.Limiter.RecordFail(req.Username)
		_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "bad_cred", clientIP(r))
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
		_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "bad_cred", clientIP(r))
		Fail(w, CodeBadCred, "账号或密码错误")
		return
	}
	if u.Status != 1 {
		_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 0, "disabled", clientIP(r))
		Fail(w, CodeDisabled, "账号已禁用")
		return
	}
	s.Limiter.Reset(req.Username)
	_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 1, "ok", clientIP(r))
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

func (s *Server) internalError(w http.ResponseWriter, err error) {
	log.Printf("[ERROR] %v", err)
	Fail(w, CodeInternal, "内部错误")
}
