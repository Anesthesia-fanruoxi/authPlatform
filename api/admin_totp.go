// Package api 用户 TOTP 双因子管理（管理端）。
package api

import (
	"encoding/json"
	"net/http"

	"github.com/anesthesia-fanruoxi/authplatform/common"
)

// GenerateTOTP POST /api/admin/users/{id}/totp/generate
// 生成新的 TOTP 密钥（base32）并落库，返回 otpauth URI 供绑定（二维码/手动录入）。
// 生成后 TOTPEnabled 保持原状态；如需重新绑定请配合 enable 接口再次校验。
func (s *Server) GenerateTOTP(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	u, err := s.Users.GetByID(r.Context(), id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	secret, err := common.GenerateTOTPSecret()
	if err != nil {
		s.internalError(w, err)
		return
	}
	// 重新生成密钥后视为未完成绑定，需重新 enable
	if err := s.Users.Update(r.Context(), id, map[string]any{"totp_secret": secret, "totp_enabled": false}); err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, map[string]any{
		"secret":       secret,
		"uri":          common.TOTPURI(u.Username, secret),
		"totp_enabled": false,
	})
}

// EnableTOTP POST /api/admin/users/{id}/totp/enable  body {code}
// 校验用户输入的 TOTP 码与当前密钥一致后启用双因子。
func (s *Server) EnableTOTP(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	u, err := s.Users.GetByID(r.Context(), id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if u.TOTPSecret == "" {
		Fail(w, CodeBadParam, "请先生成 TOTP 密钥")
		return
	}
	ok, err := common.ValidateTOTP(u.TOTPSecret, req.Code)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !ok {
		Fail(w, CodeBadParam, "TOTP 验证码错误")
		return
	}
	if err := s.Users.Update(r.Context(), id, map[string]any{"totp_enabled": true}); err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, map[string]any{"totp_enabled": true})
}

// DisableTOTP POST /api/admin/users/{id}/totp/disable 关闭并清除 TOTP 绑定。
func (s *Server) DisableTOTP(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	if err := s.Users.Update(r.Context(), id, map[string]any{"totp_secret": "", "totp_enabled": false}); err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, map[string]any{"totp_enabled": false})
}
