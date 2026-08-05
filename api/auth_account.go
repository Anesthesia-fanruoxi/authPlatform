package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/anesthesia-fanruoxi/authplatform/common"
)

// ChangePassword POST /api/auth/change-password
// 平台转发用户改密请求（验平台签名 + 用户授权）：{username, old_password, new_password}。
func (s *Server) ChangePassword(w http.ResponseWriter, r *http.Request) {
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
		Username    string `json:"username"`
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Username == "" || req.OldPassword == "" || req.NewPassword == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	u, err := s.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeBadCred, "账号或密码错误")
			return
		}
		s.internalError(w, err)
		return
	}
	granted, err := s.Grants.Granted(r.Context(), u.ID, p.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !granted {
		Fail(w, CodeUnauthorized, "该用户未授权此平台")
		return
	}
	okPass, err := common.VerifyPassword(u.PasswordHash, req.OldPassword)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !okPass {
		Fail(w, CodeBadCred, "账号或密码错误")
		return
	}
	if err := common.ValidatePassword(req.NewPassword); err != nil {
		Fail(w, CodeBadParam, err.Error())
		return
	}
	hash, err := common.HashPassword(req.NewPassword)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if err := s.Users.Update(r.Context(), u.ID, map[string]any{"password_hash": hash}); err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, nil)
}

// UpdateProfile POST /api/auth/update-profile
// 平台转发用户资料修改（验平台签名 + 用户授权）：{username, nickname}。
func (s *Server) UpdateProfile(w http.ResponseWriter, r *http.Request) {
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
		Nickname string `json:"nickname"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Username == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	u, err := s.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeBadCred, "账号或密码错误")
			return
		}
		s.internalError(w, err)
		return
	}
	granted, err := s.Grants.Granted(r.Context(), u.ID, p.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !granted {
		Fail(w, CodeUnauthorized, "该用户未授权此平台")
		return
	}
	if err := s.Users.Update(r.Context(), u.ID, map[string]any{"nickname": req.Nickname}); err != nil {
		s.internalError(w, err)
		return
	}
	updated, err := s.Users.GetByID(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, userWhitelist(updated))
}
