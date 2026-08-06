package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"authplatform/common"
	"authplatform/model"
)

// verifyPlatformRequest 校验平台请求：签名（新盐优先、旧盐兜底）+ 平台启用状态 + IP 白名单。
// 校验通过返回平台；失败时已写入错误响应，返回 nil, false。
// 签名路径使用完整 RequestURI（含 query 原样），防止 query 篡改绕过授权过滤。
func (s *Server) verifyPlatformRequest(w http.ResponseWriter, r *http.Request, body []byte) (*model.Platform, bool) {
	platformID := r.Header.Get("X-Platform-Id")
	timestamp := r.Header.Get("X-Timestamp")
	sign := r.Header.Get("X-Sign")
	if platformID == "" || timestamp == "" || sign == "" {
		Fail(w, CodeSignInvalid, "平台签名无效")
		return nil, false
	}
	p, err := s.Platforms.GetByPlatformID(r.Context(), platformID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodePlatformDown, "平台不存在或已停用")
			return nil, false
		}
		s.internalError(w, err)
		return nil, false
	}
	if p.Status != 1 {
		Fail(w, CodePlatformDown, "平台不存在或已停用")
		return nil, false
	}
	if p.IPWhitelist != "" && !ipAllowed(p.IPWhitelist, clientIP(r)) {
		Fail(w, CodeBadParam, "IP 不在白名单")
		return nil, false
	}
	secrets := make([]string, 0, 2)
	if sec, err := common.DecryptSecret(s.MasterKey, p.SecretEnc); err == nil {
		secrets = append(secrets, sec)
	}
	if p.SecretOldEnc != "" {
		if sec, err := common.DecryptSecret(s.MasterKey, p.SecretOldEnc); err == nil {
			secrets = append(secrets, sec)
		}
	}
	reqPath := r.URL.RequestURI()
	ok := false
	for _, sec := range secrets {
		if common.VerifyPlatformSignature(sec, r.Method, reqPath, timestamp, string(body), sign) == nil {
			ok = true
			break
		}
	}
	if !ok {
		Fail(w, CodeSignInvalid, "平台签名无效")
		return nil, false
	}
	return p, true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func ipAllowed(whitelistJSON, ip string) bool {
	var list []string
	if err := json.Unmarshal([]byte(whitelistJSON), &list); err != nil {
		return false
	}
	for _, v := range list {
		if v == ip {
			return true
		}
	}
	return false
}

// userWhitelist 对外字段白名单（uid/username/nickname/phone/email/status/created_at，
// 绝不返回 password_hash 及任何登录凭据）。
func userWhitelist(u *model.User) map[string]any {
	phone, email := "", ""
	if u.Phone != nil {
		phone = *u.Phone
	}
	if u.Email != nil {
		email = *u.Email
	}
	return map[string]any{
		"uid":        u.UID,
		"username":   u.Username,
		"nickname":   u.Nickname,
		"phone":      phone,
		"email":      email,
		"status":     u.Status,
		"created_at": u.CreatedAt.Format(time.RFC3339),
	}
}
