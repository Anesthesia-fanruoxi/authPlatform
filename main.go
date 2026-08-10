// authPlatform 统一鉴权中心入口（main 置于项目根目录）。
package main

import (
	"context"
	"log"
	"net/http"

	"authplatform/api"
	"authplatform/common"
	"authplatform/config"
	"authplatform/router"
)

func main() {
	cfg := config.Load()

	db, err := common.OpenDB(cfg)
	if err != nil {
		log.Fatalf("connect mysql: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("get sql.DB: %v", err)
	}
	defer sqlDB.Close()

	users := common.NewUserStore(db)
	platforms := common.NewPlatformStore(db)
	grants := common.NewGrantStore(db)
	audit := common.NewAuditStore(db)
	settings := common.NewSettingsStore(db)
	desktop := common.NewDesktopStore(db)
	if err := settings.EnsureDefaults(context.Background()); err != nil {
		log.Fatalf("ensure settings: %v", err)
	}
	if err := common.EnsureAdmin(context.Background(), users, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}
	if cfg.AdminPassword == "admin123" {
		log.Printf("[WARN] 正在使用默认管理员密码 admin123，请尽快通过 ADMIN_PASSWORD 环境变量修改")
	}

	limiter := common.NewRateLimiter()
	limiter.SetPolicy(settings.GetLoginLimit(context.Background()))

	server := &api.Server{
		Users:     users,
		Platforms: platforms,
		Grants:    grants,
		Audit:     audit,
		Settings:  settings,
		Tickets:   common.NewTicketStore(),
		VerCodes:  common.NewVerCodeStore(),
		Desktop:   desktop,
		Secret:    cfg.TokenSecret,
		MasterKey: cfg.MasterKey,
		TokenTTL:  cfg.TokenTTL,
		Limiter:   limiter,
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: router.New(server)}
	log.Printf("authPlatform listening on %s", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
