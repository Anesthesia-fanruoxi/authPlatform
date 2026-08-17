package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"

	"authplatform/common"
	"authplatform/model"
	"gorm.io/gorm"
)

var platformIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// safePlatform 脱敏平台信息：不泄露 secret 明文；showSecret 时返回完整明文
// （仅用于创建/轮换后的一次性展示）。login_methods 输出解析后的数组。
func safePlatform(p *model.Platform, showSecret bool) map[string]any {
	out := map[string]any{
		"id":                   p.ID,
		"platform_id":          p.PlatformID,
		"name":                 p.Name,
		"ip_whitelist":         p.IPWhitelist,
		"status":               p.Status,
		"created_at":           p.CreatedAt.Format(time.RFC3339),
		"login_methods":        platformLoginMethods(p),
		"login_methods_custom": p.LoginMethods != "",
		"auth_mode":            p.AuthMode,
	}
	if plain := p.SecretEnc; plain != "" {
		if showSecret {
			out["secret"] = plain
		} else if len(plain) > 8 {
			out["secret_masked"] = plain[:8] + "***"
		} else {
			out["secret_masked"] = "***"
		}
	}
	return out
}

// platformLoginMethods 解析平台 login_methods JSON；解析失败返回 nil。
func platformLoginMethods(p *model.Platform) []string {
	if p.LoginMethods == "" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(p.LoginMethods), &list); err != nil {
		return nil
	}
	return list
}

// ListPlatforms 平台列表（secret 脱敏）。
func (s *Server) ListPlatforms(w http.ResponseWriter, r *http.Request) {
	list, err := s.Platforms.List(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, safePlatform(p, false))
	}
	OK(w, map[string]any{"platforms": out})
}

// CreatePlatform 创建平台：生成独立盐明文存储，明文仅此一次返回。
// login_methods 可选：空 = 使用系统设置中的新平台默认登录方式。
func (s *Server) CreatePlatform(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlatformID   string   `json:"platform_id"`
		Name         string   `json:"name"`
		IPWhitelist  string   `json:"ip_whitelist"`
		LoginMethods []string `json:"login_methods"`
		AuthMode     string   `json:"auth_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PlatformID == "" || req.Name == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	if !platformIDRe.MatchString(req.PlatformID) {
		Fail(w, CodeBadParam, "platform_id 只能包含小写字母、数字和连字符")
		return
	}
	authMode, err := common.ValidateAuthMode(req.AuthMode)
	if err != nil {
		Fail(w, CodeBadParam, err.Error())
		return
	}
	var lmText string
	if len(req.LoginMethods) > 0 {
		if _, err := common.ValidateLoginMethods(req.LoginMethods, authMode); err != nil {
			Fail(w, CodeBadParam, err.Error())
			return
		}
		b, _ := json.Marshal(req.LoginMethods)
		lmText = string(b)
	}
	secret, err := common.NewPlatformSecret()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	p := &model.Platform{
		PlatformID:   req.PlatformID,
		Name:         req.Name,
		SecretEnc:    secret,
		IPWhitelist:  req.IPWhitelist,
		LoginMethods: lmText,
		AuthMode:     authMode,
		Status:       1,
	}
	if err := s.Platforms.Create(r.Context(), p); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			Fail(w, CodeBadParam, "platform_id 已存在")
			return
		}
		s.internalError(w, r, err)
		return
	}
	out := safePlatform(p, false)
	out["secret"] = secret // 仅此一次展示明文，请平台侧妥善保存
	OK(w, out)
}

// UpdatePlatform 更新平台（名称/启用状态/IP 白名单）。
func (s *Server) UpdatePlatform(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	var req struct {
		Name         *string   `json:"name"`
		Status       *int      `json:"status"`
		IPWhitelist  *string   `json:"ip_whitelist"`
		LoginMethods *[]string `json:"login_methods"` // nil=不改；空数组=清除（用全局默认）
		AuthMode     *string   `json:"auth_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.IPWhitelist != nil {
		updates["ip_whitelist"] = *req.IPWhitelist
	}
	if req.AuthMode != nil {
		mode, err := common.ValidateAuthMode(*req.AuthMode)
		if err != nil {
			Fail(w, CodeBadParam, err.Error())
			return
		}
		updates["auth_mode"] = mode
	}
	if req.LoginMethods != nil {
		if len(*req.LoginMethods) == 0 {
			updates["login_methods"] = "" // 清除，回退全局默认
		} else {
			mode := common.AuthModeTwoStep
			if req.AuthMode != nil {
				mode = *req.AuthMode
			}
			if _, err := common.ValidateLoginMethods(*req.LoginMethods, mode); err != nil {
				Fail(w, CodeBadParam, err.Error())
				return
			}
			b, _ := json.Marshal(*req.LoginMethods)
			updates["login_methods"] = string(b)
		}
	}
	if len(updates) == 0 {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	if err := s.Platforms.Update(r.Context(), id, updates); err != nil {
		s.internalError(w, r, err)
		return
	}
	p, err := s.Platforms.GetByID(r.Context(), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	OK(w, safePlatform(p, false))
}

// DeletePlatform 删除平台（级联清理授权由 grant store 处理）。
func (s *Server) DeletePlatform(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	if err := s.Grants.DeleteByPlatform(r.Context(), id); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.Platforms.Delete(r.Context(), id); err != nil {
		s.internalError(w, r, err)
		return
	}
	OK(w, nil)
}

// RotateSecret 密钥轮换：直接生成新盐覆盖当前盐（无过渡期，旧盐立即失效），
// 返回新盐明文（仅此一次）。
func (s *Server) RotateSecret(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	_, err = s.Platforms.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeBadParam, "平台不存在")
			return
		}
		s.internalError(w, r, err)
		return
	}
	newSecret, err := common.NewPlatformSecret()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.Platforms.Update(r.Context(), id, map[string]any{
		"secret_enc": newSecret,
	}); err != nil {
		s.internalError(w, r, err)
		return
	}
	OK(w, map[string]any{"secret": newSecret})
}
