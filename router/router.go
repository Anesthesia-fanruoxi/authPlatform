// Package router 组装路由、鉴权中间件与静态页挂载（原生 net/http ServeMux，Go 1.22+ 方法路由）。
package router

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/anesthesia-fanruoxi/authplatform/api"
	"github.com/anesthesia-fanruoxi/authplatform/common"
	"github.com/anesthesia-fanruoxi/authplatform/web"
)

// New 组装全部路由。
func New(s *api.Server) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.Health)
	mux.HandleFunc("POST /api/admin/login", s.Login)
	mux.HandleFunc("GET /api/admin/me", adminAuth(s.Users, s.Secret, s.Me))

	// 管理端：用户管理
	mux.HandleFunc("GET /api/admin/users", adminAuth(s.Users, s.Secret, s.ListUsers))
	mux.HandleFunc("POST /api/admin/users", adminAuth(s.Users, s.Secret, s.CreateUser))
	mux.HandleFunc("PUT /api/admin/users/{id}", adminAuth(s.Users, s.Secret, s.UpdateUser))
	mux.HandleFunc("DELETE /api/admin/users/{id}", adminAuth(s.Users, s.Secret, s.DeleteUser))
	mux.HandleFunc("POST /api/admin/users/{id}/reset-password", adminAuth(s.Users, s.Secret, s.ResetPassword))

	// 管理端：平台管理
	mux.HandleFunc("GET /api/admin/platforms", adminAuth(s.Users, s.Secret, s.ListPlatforms))
	mux.HandleFunc("POST /api/admin/platforms", adminAuth(s.Users, s.Secret, s.CreatePlatform))
	mux.HandleFunc("PUT /api/admin/platforms/{id}", adminAuth(s.Users, s.Secret, s.UpdatePlatform))
	mux.HandleFunc("DELETE /api/admin/platforms/{id}", adminAuth(s.Users, s.Secret, s.DeletePlatform))
	mux.HandleFunc("POST /api/admin/platforms/{id}/rotate-secret", adminAuth(s.Users, s.Secret, s.RotateSecret))

	// 管理端：授权管理
	mux.HandleFunc("GET /api/admin/grants", adminAuth(s.Users, s.Secret, s.GrantsMatrix))
	mux.HandleFunc("POST /api/admin/users/{id}/grants", adminAuth(s.Users, s.Secret, s.SetUserGrants))

	// 管理端：审计日志
	mux.HandleFunc("GET /api/admin/logs", adminAuth(s.Users, s.Secret, s.ListLogs))

	// 平台侧：登录校验与用户信息拉取（平台签名认证）
	mux.HandleFunc("POST /api/auth/verify", s.Verify)
	mux.HandleFunc("POST /api/auth/change-password", s.ChangePassword)
	mux.HandleFunc("POST /api/auth/update-profile", s.UpdateProfile)
	mux.HandleFunc("GET /api/users/{uid}", s.GetUserByUID)
	mux.HandleFunc("GET /api/users", s.ListUsersForPlatform)

	mux.HandleFunc("/", serveWeb) // 静态页兜底（未匹配到具体路由的请求）

	return withRecovery(withLogging(mux))
}

// adminAuth 管理后台鉴权中间件：仅允许「有效管理会话 token + 账号为管理员且启用」的用户通过。
// 每次请求实时查库校验 is_admin/status，确保被降级/禁用/删除的管理员立即失去管理权限。
// 与平台侧用户认证（平台签名 verifyPlatformRequest）完全独立。
func adminAuth(users *common.UserStore, secret string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			api.Fail(w, api.CodeUnauthorized, "未登录或登录已过期")
			return
		}
		userID, err := common.VerifySessionToken(secret, token)
		if err != nil {
			api.Fail(w, api.CodeUnauthorized, "未登录或登录已过期")
			return
		}
		u, err := users.GetByID(r.Context(), userID)
		if err != nil || !u.IsAdmin || u.Status != 1 {
			// 用户不存在 / 非管理员 / 已禁用 → 一律拒绝
			api.Fail(w, api.CodeUnauthorized, "未登录或登录已过期")
			return
		}
		ctx := context.WithValue(r.Context(), api.CtxKeyUserID, userID)
		next(w, r.WithContext(ctx))
	}
}

// serveWeb 提供内嵌前端资源；未匹配的 /api/* 返回业务错误。
func serveWeb(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasPrefix(p, "api/") {
		api.Fail(w, api.CodeBadParam, "接口不存在")
		return
	}
	if p == "" {
		p = "index.html"
	}
	data, err := fs.ReadFile(web.FS, p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType(p))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func contentType(p string) string {
	switch path.Ext(p) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}

// withLogging 简单访问日志。
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// withRecovery 捕获 panic 并返回统一内部错误。
func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[PANIC] %v", rec)
				api.Fail(w, api.CodeInternal, "内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
