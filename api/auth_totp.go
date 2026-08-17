package api

import (
	"encoding/base32"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"authplatform/common"
)

// SaveTOTP POST /api/auth/totp/save  body {username, secret}
// 平台侧完成双因子绑定后，将 TOTP 密钥上报鉴权中心存储。
// 绑定流程（生成密钥/扫码/验证码确认）全部在平台侧完成，authPlatform 只存储，
// 后续登录时由 authPlatform 用该密钥统一校验（verify 的 totp 登录方式）。
func (s *Server) SaveTOTP(w http.ResponseWriter, r *http.Request) {
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
		Username string `json:"username"`
		Secret   string `json:"secret"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Username == "" || req.Secret == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	u, err := s.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeBadCred, "账号或密码错误")
			return
		}
		s.internalError(w, r, err)
		return
	}
	granted, err := s.Grants.Granted(r.Context(), u.ID, p.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !granted {
		Fail(w, CodeUnauthorized, "该用户未授权此平台")
		return
	}
	// 校验 secret 为合法 base32（与 RFC 6238 一致）
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(req.Secret))
	if err != nil || len(raw) == 0 {
		Fail(w, CodeBadParam, "TOTP 密钥格式非法")
		return
	}
	if err := s.Users.Update(r.Context(), u.ID, map[string]any{"totp_secret": strings.ToUpper(req.Secret), "totp_enabled": true}); err != nil {
		s.internalError(w, r, err)
		return
	}
	OK(w, map[string]any{"totp_enabled": true})
}
