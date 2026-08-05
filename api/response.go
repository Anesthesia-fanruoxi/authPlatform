// Package api 提供 HTTP handlers 与统一响应约定（原生 net/http）。
package api

import (
	"encoding/json"
	"net/http"
)

// 业务错误码（见设计文档 §5.6）
const (
	CodeOK           = 0
	CodeSignInvalid  = 1001
	CodePlatformDown = 1002
	CodeBadCred      = 1003
	CodeDisabled     = 1004
	CodeLocked       = 1005
	CodeUnauthorized = 1006
	CodeBadParam     = 1007
	CodeUserExists   = 1008
	CodeInternal     = 2001
)

// ctxKey 避免 context key 冲突的类型。
type ctxKey string

// CtxKeyUserID 为鉴权中间件写入当前用户 ID 的 context key。
const CtxKeyUserID ctxKey = "userID"

// OK 统一成功响应：{"code":0,"msg":"ok","data":...}
func OK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"code": CodeOK, "msg": "ok", "data": data})
}

// Fail 统一失败响应：{"code":<业务码>,"msg":...,"data":null}
func Fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "msg": msg, "data": nil})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
