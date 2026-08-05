// authPlatform 统一鉴权中心入口（main 置于项目根目录）。
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/anesthesia-fanruoxi/authplatform/api"
	"github.com/anesthesia-fanruoxi/authplatform/common"
	"github.com/anesthesia-fanruoxi/authplatform/config"
	"github.com/anesthesia-fanruoxi/authplatform/router"
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
	if err := common.EnsureAdmin(context.Background(), users, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}
	if cfg.AdminPassword == "admin123" {
		log.Printf("[WARN] 正在使用默认管理员密码 admin123，请尽快通过 ADMIN_PASSWORD 环境变量修改")
	}

	server := &api.Server{
		Users:     users,
		Platforms: platforms,
		Grants:    grants,
		Audit:     audit,
		Secret:    cfg.TokenSecret,
		MasterKey: cfg.MasterKey,
		TokenTTL:  cfg.TokenTTL,
		Limiter:   common.NewRateLimiter(),
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: router.New(server)}
	log.Printf("authPlatform listening on %s", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
