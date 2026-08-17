package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"authplatform/common"
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
	okPass, err := common.VerifyPassword(u.PasswordHash, req.OldPassword)
	if err != nil {
		s.internalError(w, r, err)
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
		s.internalError(w, r, err)
		return
	}
	if err := s.Users.Update(r.Context(), u.ID, map[string]any{"password_hash": hash}); err != nil {
		s.internalError(w, r, err)
		return
	}
	OK(w, nil)
}

// UpdateProfile POST /api/auth/update-profile
// 平台转发用户资料修改（验平台签名 + 用户授权）：{username, nickname, email, phone, password, totp_secret}。
// 约定：变更逻辑在平台处理、数据在此存储——平台把变更后的字段一次性提交，本接口只负责授权校验与落库。
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
		Username   string  `json:"username"`
		Nickname   string  `json:"nickname"`
		Email      *string `json:"email"`       // nil=不修改；""=清空；非空=更新
		Phone      *string `json:"phone"`       // nil=不修改；""=清空；非空=更新
		Password   *string `json:"password"`    // 平台代改新密码，非空则哈希更新
		TotpSecret *string `json:"totp_secret"` // TOTP 重新绑定密钥，非空设置并启用；"" 清除
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

	updates := map[string]any{}
	if req.Nickname != "" {
		updates["nickname"] = req.Nickname
		updates["nickname_pinyin"] = common.Pinyin(req.Nickname)
	}
	// email/phone：nil 不改；空串清空（存 NULL，不参与唯一约束）；非空做唯一冲突预检查
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email != "" {
			other, err := s.Users.GetByEmail(r.Context(), email)
			if err == nil && other.ID != u.ID {
				Fail(w, CodeBadParam, "邮箱已被其他账号使用")
				return
			}
			updates["email"] = email
		} else {
			updates["email"] = nil
		}
	}
	if req.Phone != nil {
		phone := strings.TrimSpace(*req.Phone)
		if phone != "" {
			other, err := s.Users.GetByPhone(r.Context(), phone)
			if err == nil && other.ID != u.ID {
				Fail(w, CodeBadParam, "手机号已被其他账号使用")
				return
			}
			updates["phone"] = phone
		} else {
			updates["phone"] = nil
		}
	}
	// password：平台代改新密码（管理员场景，无需旧密码）
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		if err := common.ValidatePassword(*req.Password); err != nil {
			Fail(w, CodeBadParam, err.Error())
			return
		}
		hash, err := common.HashPassword(*req.Password)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		updates["password_hash"] = hash
	}
	// totp_secret：TOTP 重新绑定（非空设置并启用；空串清除）
	if req.TotpSecret != nil {
		secret := strings.TrimSpace(*req.TotpSecret)
		if secret != "" {
			updates["totp_secret"] = secret
			updates["totp_enabled"] = true
		} else {
			updates["totp_secret"] = ""
			updates["totp_enabled"] = false
		}
	}
	if len(updates) > 0 {
		if err := s.Users.Update(r.Context(), u.ID, updates); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	updated, err := s.Users.GetByID(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	OK(w, userWhitelist(updated))
}
