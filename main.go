// authPlatform 统一鉴权中心入口（main 置于项目根目录）。
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"authplatform/api"
	"authplatform/common"
	"authplatform/config"
	"authplatform/model"
	"authplatform/router"
	"gorm.io/gorm"
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
	if err := settings.EnsureDefaults(context.Background()); err != nil {
		log.Fatalf("ensure settings: %v", err)
	}
	backfillPinyin(db)
	startLogCleanup(settings, audit)
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

// startLogCleanup 启动登录日志自动清理：启动 1 分钟后执行一次，此后每 24 小时执行。
// 每次执行读取系统设置的「登录日志保留天数」，删除 login_logs 中超过保留期的记录（request_logs 全量请求日志不清理）。
func startLogCleanup(settings *common.SettingsStore, audit *common.AuditStore) {
	go func() {
		cleanupOnce := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			days := settings.GetLogRetentionDays(ctx)
			cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
			deleted, err := audit.Cleanup(ctx, cutoff)
			if err != nil {
				log.Printf("[cleanup] 清理登录日志失败: %v", err)
				return
			}
			if deleted > 0 {
				log.Printf("[cleanup] 自动清理完成：删除 %d 天前的登录日志 %d 条", days, deleted)
			}
		}
		time.Sleep(1 * time.Minute) // 启动后先执行一次（清理存量）
		cleanupOnce()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanupOnce()
		}
	}()
}

// backfillPinyin 存量数据回填：为昵称非空但 nickname_pinyin 为空的用户补算拼音（幂等，启动后执行一次）。
func backfillPinyin(db *gorm.DB) {
	var users []model.User
	if err := db.Where("nickname <> '' AND nickname_pinyin = ''").Find(&users).Error; err != nil {
		log.Printf("[backfill] query users: %v", err)
		return
	}
	for _, u := range users {
		p := common.Pinyin(u.Nickname)
		if err := db.Model(&u).Update("nickname_pinyin", p).Error; err != nil {
			log.Printf("[backfill] update user %s: %v", u.Username, err)
			continue
		}
		log.Printf("[backfill] %s nickname=%q → pinyin=%q", u.Username, u.Nickname, p)
	}
	if n := len(users); n > 0 {
		log.Printf("[backfill] 完成回填 %d 个用户", n)
	}
}
