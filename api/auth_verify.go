package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"authplatform/common"
	"authplatform/model"
)

// tokenTTLHint 签发 token 的建议有效期（平台侧自行管理生命周期，此值仅为参考）。
const tokenTTLHint = 24 * time.Hour

// totpTicketTTL 2FA 待验证会话 ticket 有效期（5 分钟，无状态签名）。
const totpTicketTTL = 5 * time.Minute

// verifyRequest 登录第一步（或单步）请求体。
type verifyRequest struct {
	PlatformID string `json:"platform_id"`
	// 方式与凭证（新格式）。缺省 method 时按旧格式解析 username/password。
	Method     string `json:"method"`
	Identifier string `json:"identifier"`
	Credential string `json:"credential"`
	// 旧格式兼容：username + password
	Username string `json:"username"`
	Password string `json:"password"`
}

// Verify POST /api/auth/verify 平台转发登录校验：
// 验平台签名 → IP 白名单 → 第一步凭证 → 账号状态 → 授权 →
// 单选方式：直接签发不透明 token；多选方式：签发 login_ticket 并返回下一步。
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
	var req verifyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	// 登录方式：平台自定义优先（平台管理里配置），空则使用系统设置的默认模板
	methods := s.platformMethods(r.Context(), p)
	first := methods[0]

	method, identifier, credential := req.Method, req.Identifier, req.Credential
	if method == "" {
		// 兼容旧格式 {username, password}
		method = common.LoginMethodUsernamePassword
		identifier = req.Username
		credential = req.Password
	}
	if identifier == "" || credential == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	if method != first {
		Fail(w, CodeBadParam, "当前第一步登录方式为 "+first)
		return
	}
	ip := clientIP(r)
	audit := func(success int, reason string) {
		_ = s.Audit.WriteLogin(r.Context(), identifier, p.PlatformID, success, reason, ip)
	}

	// 定位用户（按第一步方式）
	var u *model.User
	switch method {
	case common.LoginMethodUsernamePassword:
		u, err = s.Users.GetByUsername(r.Context(), identifier)
	case common.LoginMethodEmailPassword:
		u, err = s.Users.GetByEmail(r.Context(), identifier)
	case common.LoginMethodPhoneCode:
		u, err = s.Users.GetByPhone(r.Context(), identifier)
	default:
		Fail(w, CodeBadParam, "不支持的第一步登录方式")
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
	// 限流 + 黑名单（账号维度，统一按 username 计数）
	if err := s.Limiter.Check(u.Username); err != nil {
		if errors.Is(err, common.ErrBanned) {
			audit(0, "banned")
			Fail(w, CodeLocked, err.Error())
			return
		}
		audit(0, "locked")
		Fail(w, CodeLocked, err.Error())
		return
	}

	// 验证第一步凭证
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

	// 多步骤：签发 ticket 返回下一步
	if len(methods) > 1 {
		tk, err := s.Tickets.Create(u.ID, p.ID, methods)
		if err != nil {
			s.internalError(w, err)
			return
		}
		s.Tickets.MarkDone(tk, method)
		audit(1, "step_ok")
		OK(w, map[string]any{
			"ticket":      tk,
			"step":        1,
			"total_steps": len(methods),
			"next_method": methods[1],
			"expires_in":  int((5 * time.Minute).Seconds()),
			"identifier":  identifierForMethod(u, methods[1]),
		})
		return
	}

	// 单步：直接签发 token
	s.Limiter.Reset(u.Username)
	audit(1, "ok")
	s.issueToken(w, r, u, p)
}

// VerifyStep POST /api/auth/verify-step 登录后续步骤：
// body {platform_id, ticket, credential}。按 ticket 关联用户推进登录方式列表，
// 最后一步通过后签发最终 token 并销毁 ticket。
func (s *Server) VerifyStep(w http.ResponseWriter, r *http.Request) {
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
		PlatformID string `json:"platform_id"`
		Ticket     string `json:"ticket"`
		Credential string `json:"credential"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Ticket == "" || req.Credential == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	t, ok := s.Tickets.Get(req.Ticket)
	if !ok {
		Fail(w, CodeBadParam, "登录票据无效或已过期")
		return
	}
	if t.PlatformID != p.ID {
		Fail(w, CodeBadParam, "登录票据与平台不匹配")
		return
	}
	doneCount := len(t.DoneMethods)
	if doneCount >= len(t.Methods) {
		s.Tickets.Delete(req.Ticket)
		Fail(w, CodeBadParam, "登录流程已完成")
		return
	}
	next := t.Methods[doneCount]
	u, err := s.Users.GetByID(r.Context(), t.UserID)
	if err != nil {
		s.Tickets.Delete(req.Ticket)
		s.internalError(w, err)
		return
	}
	ip := clientIP(r)
	if u.Status != 1 {
		_ = s.Audit.WriteLogin(r.Context(), u.Username, p.PlatformID, 0, "disabled", ip)
		s.Tickets.Delete(req.Ticket)
		Fail(w, CodeDisabled, "账号已禁用")
		return
	}
	if err := s.Limiter.Check(u.Username); err != nil {
		if errors.Is(err, common.ErrBanned) {
			_ = s.Audit.WriteLogin(r.Context(), u.Username, p.PlatformID, 0, "banned", ip)
			Fail(w, CodeLocked, err.Error())
			return
		}
		_ = s.Audit.WriteLogin(r.Context(), u.Username, p.PlatformID, 0, "locked", ip)
		Fail(w, CodeLocked, err.Error())
		return
	}

	okCred, reason, failMsg := s.verifyCredential(r.Context(), u, next, identifierForMethod(u, next), req.Credential)
	if !okCred {
		// 凭证失败：保留 ticket 允许重试（限流兜底），不销毁登录流程
		s.Limiter.RecordFail(u.Username)
		_ = s.Audit.WriteLogin(r.Context(), u.Username, p.PlatformID, 0, reason, ip)
		Fail(w, CodeBadCred, failMsg)
		return
	}
	s.Tickets.MarkDone(req.Ticket, next)

	if len(t.Methods) > doneCount+1 {
		// 还有后续步骤
		_ = s.Audit.WriteLogin(r.Context(), u.Username, p.PlatformID, 1, "step_ok", ip)
		OK(w, map[string]any{
			"ticket":      req.Ticket,
			"step":        doneCount + 2,
			"total_steps": len(t.Methods),
			"next_method": t.Methods[doneCount+1],
			"expires_in":  int((5 * time.Minute).Seconds()),
			"identifier":  identifierForMethod(u, t.Methods[doneCount+1]),
		})
		return
	}

	// 全部步骤完成，签发最终 token
	s.Tickets.Delete(req.Ticket)
	s.Limiter.Reset(u.Username)
	_ = s.Audit.WriteLogin(r.Context(), u.Username, p.PlatformID, 1, "ok", ip)
	s.issueToken(w, r, u, p)
}

// SendCode POST /api/auth/send-code 发送登录验证码（手机号/邮箱）。
// dev 模式：不接真实短信/邮件服务商，验证码直接返回（dev_code）并打印日志，便于联调；
// 发送器接口预留，后续接入服务商时移除 dev_code 返回。
func (s *Server) SendCode(w http.ResponseWriter, r *http.Request) {
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
		PlatformID string `json:"platform_id"`
		Method     string `json:"method"`
		Identifier string `json:"identifier"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Identifier == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	switch req.Method {
	case common.LoginMethodPhoneCode, common.LoginMethodEmailCode:
	default:
		Fail(w, CodeBadParam, "不支持的发码方式")
		return
	}
	// 用户必须存在且启用（避免向无效账号发码；同时不泄露账号存在性细节）
	var u *model.User
	if req.Method == common.LoginMethodPhoneCode {
		u, err = s.Users.GetByPhone(r.Context(), req.Identifier)
	} else {
		u, err = s.Users.GetByEmail(r.Context(), req.Identifier)
	}
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeBadCred, "账号不存在或已停用")
			return
		}
		s.internalError(w, err)
		return
	}
	if u.Status != 1 {
		Fail(w, CodeDisabled, "账号已禁用")
		return
	}
	code, err := s.VerCodes.Generate(req.Method, req.Identifier)
	if err != nil {
		s.internalError(w, err)
		return
	}
	// dev 模式：验证码直接返回，便于联调（上线接入真实发送器后删除此字段）
	_ = p
	OK(w, map[string]any{
		"dev_code":           code,
		"expires_in_seconds": 300,
		"method":             req.Method,
	})
}

// platformMethods 返回平台生效的登录方式：平台自定义（login_methods）优先，
// 为空时回退系统设置的「新平台默认登录方式」。
func (s *Server) platformMethods(ctx context.Context, p *model.Platform) []string {
	if p.LoginMethods != "" {
		var list []string
		if err := json.Unmarshal([]byte(p.LoginMethods), &list); err == nil && len(list) > 0 {
			return list
		}
	}
	return s.Settings.GetLoginMethods(ctx).Methods
}

// verifyCredential 校验单个登录方式的凭证。返回 (是否通过, 审计reason, 失败提示)。
func (s *Server) verifyCredential(ctx context.Context, u *model.User, method, identifier, credential string) (bool, string, string) {
	switch method {
	case common.LoginMethodUsernamePassword:
		if u.Username != identifier {
			return false, "bad_cred", "账号或密码错误"
		}
		ok, err := common.VerifyPassword(u.PasswordHash, credential)
		if err != nil || !ok {
			return false, "bad_cred", "账号或密码错误"
		}
		return true, "", ""
	case common.LoginMethodEmailPassword:
		if !strPtrEq(u.Email, identifier) {
			return false, "bad_cred", "账号或密码错误"
		}
		ok, err := common.VerifyPassword(u.PasswordHash, credential)
		if err != nil || !ok {
			return false, "bad_cred", "账号或密码错误"
		}
		return true, "", ""
	case common.LoginMethodPhoneCode:
		if !strPtrEq(u.Phone, identifier) {
			return false, "bad_code", "验证码错误"
		}
		if !s.VerCodes.Verify(common.LoginMethodPhoneCode, identifier, credential) {
			return false, "bad_code", "验证码错误或已过期"
		}
		return true, "", ""
	case common.LoginMethodTOTP:
		if !u.TOTPEnabled || u.TOTPSecret == "" {
			return false, "totp_disabled", "该用户未启用 TOTP 双因子验证"
		}
		ok, err := common.ValidateTOTP(u.TOTPSecret, credential)
		if err != nil {
			return false, "bad_totp", "TOTP 验证码错误"
		}
		if !ok {
			return false, "bad_totp", "TOTP 验证码错误"
		}
		return true, "", ""
	}
	return false, "bad_cred", "不支持的登录方式"
}

// identifierForMethod 返回指定方式对应的标识（用于提示与验证码定位）。
func identifierForMethod(u *model.User, method string) string {
	switch method {
	case common.LoginMethodUsernamePassword:
		return u.Username
	case common.LoginMethodEmailPassword:
		return strPtrVal(u.Email)
	case common.LoginMethodPhoneCode:
		return strPtrVal(u.Phone)
	case common.LoginMethodTOTP:
		return ""
	}
	return u.Username
}

// strPtrVal 指针字符串取值（nil 返回空串）。
func strPtrVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// strPtrEq 指针字符串与值比较。
func strPtrEq(p *string, s string) bool {
	return p != nil && *p == s
}

// issueToken 签发不透明 token 并返回统一登录成功响应。
// 是否强制改密等业务规则由平台侧自行维护，认证中心只负责校验与签发。
func (s *Server) issueToken(w http.ResponseWriter, r *http.Request, u *model.User, p *model.Platform) {
	token, err := common.NewOpaqueToken()
	if err != nil {
		s.internalError(w, err)
		return
	}
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
