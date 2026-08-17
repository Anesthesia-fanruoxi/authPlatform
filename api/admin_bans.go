// Package api 黑名单管理（限流自动锁定 + 管理员手动拉黑，内存存储）。
package api

import (
	"authplatform/common"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// ListBans GET /api/admin/bans 返回全部黑名单/锁定记录。
func (s *Server) ListBans(w http.ResponseWriter, _ *http.Request) {
	list := s.Limiter.ListBans()
	now := time.Now()
	out := make([]map[string]any, 0, len(list))
	for _, b := range list {
		rec := map[string]any{
			"id":         b.ID,
			"username":   b.Username,
			"source":     b.Source,
			"reason":     b.Reason,
			"operator":   b.Operator,
			"created_at": b.CreatedAt.Format(time.RFC3339),
		}
		if b.ExpiresAt.IsZero() {
			rec["expires_at"] = ""
			rec["status"] = "permanent" // 永久
		} else if b.IsExpired(now) {
			rec["expires_at"] = b.ExpiresAt.Format(time.RFC3339)
			rec["status"] = "expired" // 已过期
		} else {
			rec["expires_at"] = b.ExpiresAt.Format(time.RFC3339)
			rec["status"] = "active" // 生效中
		}
		out = append(out, rec)
	}
	OK(w, map[string]any{"bans": out})
}

// AddBan POST /api/admin/bans 手动将账号加入黑名单。
// body: {username, reason?, expires_at?}（expires_at 为空=永久，支持 RFC3339 或 "2006-01-02 15:04:05"）
func (s *Server) AddBan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username  string `json:"username"`
		Reason    string `json:"reason"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	// 账号必须存在（按 username/邮箱/手机 均可定位）
	if _, err := s.Users.GetByIdentifier(r.Context(), req.Username); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeBadParam, "账号不存在")
			return
		}
		s.internalError(w, r, err)
		return
	}
	var expiresAt time.Time
	if req.ExpiresAt != "" {
		t, perr := parseFlexTime(req.ExpiresAt)
		if perr != nil {
			Fail(w, CodeBadParam, "到期时间格式错误")
			return
		}
		expiresAt = t
	}
	// 操作管理员
	operator := ""
	if uid, ok := r.Context().Value(CtxKeyUserID).(int64); ok {
		if u, err := s.Users.GetByID(r.Context(), uid); err == nil {
			operator = u.Username
		}
	}
	reason := req.Reason
	if reason == "" {
		reason = "管理员手动加入黑名单"
	}
	s.Limiter.Ban(req.Username, reason, operator, expiresAt)
	_ = s.Audit.WriteLogin(r.Context(), req.Username, "", 1, "ban_add", clientIP(r))
	OK(w, map[string]any{"banned": true})
}

// RemoveBan DELETE /api/admin/bans/{username} 解除黑名单/锁定。
func (s *Server) RemoveBan(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	s.Limiter.Unban(username)
	_ = s.Audit.WriteLogin(r.Context(), username, "", 1, "ban_unlock", clientIP(r))
	OK(w, map[string]any{"unbanned": true})
}

// parseFlexTime 宽松解析时间（RFC3339 / 本地日期时间 / 日期）。
func parseFlexTime(s string) (time.Time, error) {
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("bad time format")
}
