// Package api 系统设置管理（密码安全 / 登录限流 / 登录方式 / 后台 IP 白名单）。
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"authplatform/common"
)

// ListSettings GET /api/admin/settings 返回全部系统设置。
func (s *Server) ListSettings(w http.ResponseWriter, r *http.Request) {
	all, err := s.Settings.All(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, all)
}

// UpdateSettings PUT /api/admin/settings/{key} 更新单类设置。
// key: password_policy | login_limit | login_methods | admin_ip_whitelist
func (s *Server) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	switch key {
	case "password_policy", "login_limit", "login_methods", "admin_ip_whitelist", "user_categories":
	default:
		Fail(w, CodeBadParam, "不支持的设置项")
		return
	}
	raw, err := readBody(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	// 校验并保存（存解析后的结构化值，避免 []byte 被序列化为 base64）
	var saveErr error
	switch key {
	case "password_policy":
		var p common.PasswordPolicy
		if err := json.Unmarshal(raw, &p); err != nil || p.MinLength < 6 || p.MinLength > 64 {
			Fail(w, CodeBadParam, "密码策略参数错误（min_length 6-64）")
			return
		}
		if !p.RequireLetter && !p.RequireDigit && !p.RequireSpecial {
			Fail(w, CodeBadParam, "至少需要启用一种复杂度要求")
			return
		}
		saveErr = s.Settings.Set(r.Context(), key, p)
	case "login_limit":
		var l common.LoginLimit
		if err := json.Unmarshal(raw, &l); err != nil || l.MaxFails < 1 || l.WindowMinutes < 1 || l.LockMinutes < 1 {
			Fail(w, CodeBadParam, "限流参数错误")
			return
		}
		saveErr = s.Settings.Set(r.Context(), key, l)
	case "login_methods":
		var m common.LoginMethods
		if err := json.Unmarshal(raw, &m); err != nil {
			Fail(w, CodeBadParam, "登录方式参数错误")
			return
		}
		if _, err := common.ValidateLoginMethods(m.Methods, common.AuthModeTwoStep); err != nil {
			Fail(w, CodeBadParam, err.Error())
			return
		}
		saveErr = s.Settings.Set(r.Context(), key, m)
	case "admin_ip_whitelist":
		var wl struct {
			IPs []string `json:"ips"`
		}
		if err := json.Unmarshal(raw, &wl); err != nil {
			Fail(w, CodeBadParam, "IP 白名单参数错误")
			return
		}
		saveErr = s.Settings.Set(r.Context(), key, wl)
	case "user_categories":
		var c common.UserCategories
		if err := json.Unmarshal(raw, &c); err != nil || len(c.Items) == 0 || len(c.Items) > 30 {
			Fail(w, CodeBadParam, "分类参数错误（1-30 个分类）")
			return
		}
		seen := map[string]bool{}
		for i, it := range c.Items {
			it = strings.TrimSpace(it)
			if it == "" || len([]rune(it)) > 32 || seen[it] {
				Fail(w, CodeBadParam, "分类不能为空/重复/超长（≤32 字符）")
				return
			}
			seen[it] = true
			c.Items[i] = it
		}
		saveErr = s.Settings.Set(r.Context(), key, c)
	}
	if saveErr != nil {
		s.internalError(w, saveErr)
		return
	}
	// 限流策略立即生效
	if key == "login_limit" {
		s.Limiter.SetPolicy(s.Settings.GetLoginLimit(r.Context()))
	}
	OK(w, nil)
}

// ipAllowed 判断 IP 是否在列表中。
func ipInList(ip string, list []string) bool {
	for _, v := range list {
		if v == ip {
			return true
		}
	}
	return false
}

// readBody 读取并返回请求体原始字节。
func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}
