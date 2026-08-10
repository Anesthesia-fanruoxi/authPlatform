package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"authplatform/common"
	"authplatform/model"
)

const (
	desktopTokenTTL = 30 * 24 * time.Hour // desktop_token 有效期（30 天）
	pendingTTL      = 60 * time.Second    // 免密登录待确认请求有效期
)

// DesktopLogin POST /api/auth/desktop/login 桌面客户端登录（无平台签名，用账号+TOTP 等凭证）。
// 兼容 verifyRequest 新旧格式；成功返回 desktop_token（仅此一次）+ 用户信息。
func (s *Server) DesktopLogin(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	method, identifier, credential := req.Method, req.Identifier, req.Credential
	if method == "" {
		method = common.LoginMethodUsernamePassword
		identifier = req.Username
		credential = req.Password
	}
	if identifier == "" || credential == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	ip := clientIP(r)
	audit := func(success int, reason string) {
		_ = s.Audit.WriteLogin(r.Context(), identifier, "", success, reason, ip)
	}

	var u *model.User
	var err error
	switch method {
	case common.LoginMethodUsernamePassword, common.LoginMethodUsernameTOTP:
		u, err = s.Users.GetByUsername(r.Context(), identifier)
	case common.LoginMethodEmailPassword:
		u, err = s.Users.GetByEmail(r.Context(), identifier)
	case common.LoginMethodPhoneCode:
		u, err = s.Users.GetByPhone(r.Context(), identifier)
	default:
		Fail(w, CodeBadParam, "不支持的登录方式")
		return
	}
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			s.Limiter.RecordFail(identifier)
			audit(0, "bad_cred")
			Fail(w, CodeBadCred, "账号或密码错误")
			return
		}
		s.internalError(w, err)
		return
	}
	if err := s.Limiter.Check(u.Username); err != nil {
		audit(0, "locked")
		Fail(w, CodeLocked, err.Error())
		return
	}
	okCred, reason, failMsg := s.verifyCredential(r.Context(), u, method, identifier, credential)
	if !okCred {
		s.Limiter.RecordFail(u.Username)
		audit(0, reason)
		Fail(w, CodeBadCred, failMsg)
		return
	}
	if u.Status != 1 {
		audit(0, "disabled")
		Fail(w, CodeDisabled, "账号已禁用")
		return
	}

	token, hash, err := common.NewDesktopToken()
	if err != nil {
		s.internalError(w, err)
		return
	}
	if _, err := s.Desktop.CreateSession(r.Context(), u.ID, hash, desktopTokenTTL); err != nil {
		s.internalError(w, err)
		return
	}
	s.Limiter.Reset(u.Username)
	audit(1, "desktop_ok")
	OK(w, map[string]any{
		"desktop_token": token,
		"expires_at":    time.Now().Add(desktopTokenTTL).Format(time.RFC3339),
		"user": map[string]any{
			"uid":      u.UID,
			"username": u.Username,
			"nickname": u.Nickname,
		},
	})
}

// DesktopInitiate POST /api/auth/desktop/initiate 平台发起桌面免密登录（平台签名）。
// 返回 60 秒有效的一次性 request_id，平台将其推送给桌面客户端等待用户确认。
func (s *Server) DesktopInitiate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	p, ok := s.verifyPlatformRequest(w, r, body)
	if !ok {
		return
	}
	reqID, err := common.NewPendingRequestID()
	if err != nil {
		s.internalError(w, err)
		return
	}
	if _, err := s.Desktop.CreatePending(r.Context(), reqID, p.ID, pendingTTL); err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, map[string]any{"request_id": reqID, "expires_in": int(pendingTTL.Seconds())})
}

// DesktopPoll GET /api/auth/desktop/poll?request_id= 平台轮询确认状态（平台签名）。
func (s *Server) DesktopPoll(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	p, ok := s.verifyPlatformRequest(w, r, body)
	if !ok {
		return
	}
	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	pending, err := s.Desktop.GetPending(r.Context(), requestID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeBadParam, "请求不存在")
			return
		}
		s.internalError(w, err)
		return
	}
	if pending.PlatformID != p.ID {
		Fail(w, CodeBadParam, "请求不属于当前平台")
		return
	}
	if time.Now().After(pending.ExpiresAt) {
		OK(w, map[string]any{"status": "expired"})
		return
	}
	switch pending.Status {
	case "confirmed":
		OK(w, map[string]any{"status": "confirmed"})
	case "used":
		OK(w, map[string]any{"status": "used"})
	default:
		OK(w, map[string]any{"status": "pending"})
	}
}

// DesktopConfirm POST /api/auth/desktop/confirm 客户端确认（desktop_token + request_id）。
func (s *Server) DesktopConfirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DesktopToken string `json:"desktop_token"`
		RequestID    string `json:"request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DesktopToken == "" || req.RequestID == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	sess, err := s.Desktop.GetSessionByToken(r.Context(), req.DesktopToken)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeUnauthorized, "桌面会话无效或已过期，请重新登录客户端")
			return
		}
		s.internalError(w, err)
		return
	}
	pending, err := s.Desktop.GetPending(r.Context(), req.RequestID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeBadParam, "请求不存在")
			return
		}
		s.internalError(w, err)
		return
	}
	if time.Now().After(pending.ExpiresAt) {
		Fail(w, CodeBadParam, "请求已过期")
		return
	}
	if pending.Status != "initiated" {
		Fail(w, CodeBadParam, "请求已处理")
		return
	}
	if err := s.Desktop.ConfirmPending(r.Context(), req.RequestID, sess.UserID); err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, nil)
}

// DesktopExchange POST /api/auth/desktop/exchange 平台用确认后的 request_id 兑换平台 token（平台签名，一次性）。
func (s *Server) DesktopExchange(w http.ResponseWriter, r *http.Request) {
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
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.RequestID == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	pending, err := s.Desktop.GetPending(r.Context(), req.RequestID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeBadParam, "请求不存在")
			return
		}
		s.internalError(w, err)
		return
	}
	if pending.PlatformID != p.ID {
		Fail(w, CodeBadParam, "请求不属于当前平台")
		return
	}
	if time.Now().After(pending.ExpiresAt) {
		Fail(w, CodeBadParam, "请求已过期")
		return
	}
	if pending.Status != "confirmed" {
		Fail(w, CodeBadParam, "请求未确认或已使用")
		return
	}
	u, err := s.Users.GetByID(r.Context(), pending.UserID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if u.Status != 1 {
		_ = s.Audit.WriteLogin(r.Context(), u.Username, p.PlatformID, 0, "disabled", clientIP(r))
		Fail(w, CodeDisabled, "账号已禁用")
		return
	}
	granted, err := s.Grants.Granted(r.Context(), u.ID, p.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !granted {
		_ = s.Audit.WriteLogin(r.Context(), u.Username, p.PlatformID, 0, "unauthorized", clientIP(r))
		Fail(w, CodeUnauthorized, "该用户未授权登录此平台")
		return
	}
	if err := s.Desktop.ConsumePending(r.Context(), req.RequestID); err != nil {
		s.internalError(w, err)
		return
	}
	_ = s.Audit.WriteLogin(r.Context(), u.Username, p.PlatformID, 1, "desktop_ok", clientIP(r))
	s.issueToken(w, r, u, p)
}
