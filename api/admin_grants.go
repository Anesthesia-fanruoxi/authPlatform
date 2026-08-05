package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/anesthesia-fanruoxi/authplatform/common"
)

// GrantsMatrix GET /api/admin/grants 返回授权矩阵数据（用户 × 平台 × 现有授权）。
func (s *Server) GrantsMatrix(w http.ResponseWriter, r *http.Request) {
	users, err := s.Users.List(r.Context(), "")
	if err != nil {
		s.internalError(w, err)
		return
	}
	platforms, err := s.Platforms.List(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	grants, err := s.Grants.ListAll(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}

	userOut := make([]map[string]any, 0, len(users))
	for _, u := range users {
		userOut = append(userOut, map[string]any{
			"id":       u.ID,
			"uid":      u.UID,
			"username": u.Username,
			"nickname": u.Nickname,
			"status":   u.Status,
		})
	}
	platformOut := make([]map[string]any, 0, len(platforms))
	for _, p := range platforms {
		platformOut = append(platformOut, map[string]any{
			"id":          p.ID,
			"platform_id": p.PlatformID,
			"name":        p.Name,
			"status":      p.Status,
		})
	}
	grantOut := make([]map[string]any, 0, len(grants))
	for _, g := range grants {
		grantOut = append(grantOut, map[string]any{
			"user_id":     g.UserID,
			"platform_id": g.PlatformID,
			"status":      g.Status,
		})
	}
	OK(w, map[string]any{"users": userOut, "platforms": platformOut, "grants": grantOut})
}

// SetUserGrants POST /api/admin/users/{id}/grants
// 全量替换某用户可登录的平台集合。
func (s *Server) SetUserGrants(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	var req struct {
		PlatformIDs []int64 `json:"platform_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	if _, err := s.Users.GetByID(r.Context(), id); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			Fail(w, CodeBadParam, "用户不存在")
			return
		}
		s.internalError(w, err)
		return
	}
	if err := s.Grants.SetForUser(r.Context(), id, req.PlatformIDs); err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, nil)
}
