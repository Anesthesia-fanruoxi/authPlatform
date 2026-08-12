package api

import (
	"net/http"
	"strconv"
)

// ListRequestLogs 全量请求日志列表 GET /api/admin/request-logs?method=&path=&platform_id=&status=&limit=
func (s *Server) ListRequestLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var status *int
	if v := q.Get("status"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			status = &n
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	list, err := s.Audit.ListRequest(r.Context(), q.Get("method"), q.Get("path"), q.Get("platform_id"), status, limit)
	if err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, map[string]any{"logs": list})
}

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
