package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/anesthesia-fanruoxi/authplatform/common"
)

// tokenTTLHint 签发 token 的建议有效期（平台侧自行管理生命周期，此值仅为参考）。
const tokenTTLHint = 24 * time.Hour

// Verify POST /api/auth/verify 平台转发登录校验：
// 验平台签名 → IP 白名单 → 账号密码 → 启用状态 → 授权 → 签发不透明 token + 审计。
func (s *Server) Verify(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	p, ok := s.verifyPlatformRequest(w, r, body)
	if !ok {
		return
	}
	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		PlatformID string `json:"platform_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Username == "" || req.Password == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	ip := clientIP(r)
	audit := func(success int, reason string) {
		_ = s.Audit.WriteLogin(r.Context(), req.Username, p.PlatformID, success, reason, ip)
	}

	if err := s.Limiter.Check(req.Username); err != nil {
		audit(0, "locked")
		Fail(w, CodeLocked, err.Error())
		return
	}
	u, err := s.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			s.Limiter.RecordFail(req.Username)
			audit(0, "bad_cred")
			Fail(w, CodeBadCred, "账号或密码错误")
			return
		}
		s.internalError(w, err)
		return
	}
	okPass, err := common.VerifyPassword(u.PasswordHash, req.Password)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !okPass {
		s.Limiter.RecordFail(req.Username)
		audit(0, "bad_cred")
		Fail(w, CodeBadCred, "账号或密码错误")
		return
	}
	if u.Status != 1 {
		audit(0, "disabled")
		Fail(w, CodeDisabled, "账号已禁用")
		return
	}
	granted, err := s.Grants.Granted(r.Context(), u.ID, p.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !granted {
		audit(0, "unauthorized")
		Fail(w, CodeUnauthorized, "该用户未授权登录此平台")
		return
	}
	token, err := common.NewOpaqueToken()
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.Limiter.Reset(req.Username)
	audit(1, "ok")
	OK(w, map[string]any{
		"token":      token,
		"expires_at": time.Now().Add(tokenTTLHint).Format(time.RFC3339),
		"user": map[string]any{
			"uid":      u.UID,
			"username": u.Username,
			"nickname": u.Nickname,
			"status":   u.Status,
		},
	})
}
