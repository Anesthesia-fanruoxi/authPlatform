package api

import (
	"errors"
	"net/http"

	"authplatform/common"
)

// GetUserByUID GET /api/users/{uid}?platform_id=xxx
// 仅返回「授权给该平台」的用户；未授权/不存在一律 404（平台侧不可见）。
func (s *Server) GetUserByUID(w http.ResponseWriter, r *http.Request) {
	p, ok := s.verifyPlatformRequest(w, r, nil)
	if !ok {
		return
	}
	u, err := s.Users.GetByUID(r.Context(), r.PathValue("uid"))
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.internalError(w, r, err)
		return
	}
	if u.IsAdmin {
		// 认证中心管理员（含 admin）不对任何平台可见
		http.NotFound(w, r)
		return
	}
	granted, err := s.Grants.Granted(r.Context(), u.ID, p.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !granted {
		http.NotFound(w, r) // 未授权用户对平台不可见
		return
	}
	OK(w, userWhitelist(u))
}

// ListUsersForPlatform GET /api/users?platform_id=xxx&keyword=
// 平台视角用户列表：仅返回授权给该平台的用户（服务端强制过滤）。
func (s *Server) ListUsersForPlatform(w http.ResponseWriter, r *http.Request) {
	p, ok := s.verifyPlatformRequest(w, r, nil)
	if !ok {
		return
	}
	grants, err := s.Grants.GetByPlatform(r.Context(), p.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ids := make([]int64, 0, len(grants))
	for _, g := range grants {
		ids = append(ids, g.UserID)
	}
	users, err := s.Users.ListByIDs(r.Context(), ids, r.URL.Query().Get("keyword"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	safe := make([]map[string]any, 0, len(users))
	for _, u := range users {
		if u.IsAdmin {
			continue // 认证中心管理员（含 admin）不对任何平台可见
		}
		safe = append(safe, userWhitelist(u))
	}
	OK(w, map[string]any{"users": safe})
}
