package api

import (
	"net/http"
	"strconv"
)

// ListLogs GET /api/admin/logs?username=&platform_id=&success=&limit=
// 登录审计日志查询（按时间倒序）。
func (s *Server) ListLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var success *int
	if v := q.Get("success"); v == "1" || v == "0" {
		n, _ := strconv.Atoi(v)
		success = &n
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	list, err := s.Audit.ListLogin(r.Context(), q.Get("username"), q.Get("platform_id"), success, limit)
	if err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, map[string]any{"logs": list})
}
