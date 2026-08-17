// authPlatform 统一鉴权中心入口（main 置于项目根目录）。
package main

import (
	"context"
	"log"
	"net/http"
	"sync"
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

	// 委托 handler：支持 setup 模式 ↔ 完整服务热切换（保存配置后无需重启）
	var mu sync.RWMutex
	var active http.Handler
	swap := func(h http.Handler) {
		mu.Lock()
		active = h
		mu.Unlock()
	}
	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		h := active
		mu.RUnlock()
		h.ServeHTTP(w, r)
	})

	// setup 模式服务：仅 health + 引导配置 API + 初始化页
	setupServer := &api.Server{
		Secret:   cfg.TokenSecret,
		TokenTTL: cfg.TokenTTL,
		Limiter:  common.NewRateLimiter(),
	}
	// 热初始化：保存配置后连库并切换为完整服务
	setupServer.OnSetupSaved = func(ncfg *config.Config) error {
		server, _, err := bootstrap(ncfg)
		if err != nil {
			return err
		}
		swap(router.New(server))
		common.LogInfo("setup", "配置已生效，完整服务已启动")
		return nil
	}

	if config.HasConfigFile() {
		// 有配置文件：直接完整初始化，失败退出
		server, db, err := bootstrap(cfg)
		if err != nil {
			log.Fatalf("初始化失败: %v", err)
		}
		defer closeDB(db)
		active = router.New(server)
	} else {
		// 无配置文件：零配置进入 setup 模式，由 Web 引导页完成配置
		common.LogInfo("setup", "未检测到 config.yaml，进入初始化模式，请打开浏览器完成配置")
		active = router.NewSetupMode(setupServer)
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: root}
	common.LogInfo("startup", "authPlatform listening on %s", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// bootstrap 完整服务初始化：连库建表、组装存储、引导管理员；返回 Server 与 DB（供退出时关闭）。
func bootstrap(cfg *config.Config) (*api.Server, *gorm.DB, error) {
	db, err := common.OpenDB(cfg)
	if err != nil {
		return nil, nil, err
	}
	users := common.NewUserStore(db)
	platforms := common.NewPlatformStore(db)
	grants := common.NewGrantStore(db)
	audit := common.NewAuditStore(db)
	settings := common.NewSettingsStore(db)
	if err := settings.EnsureDefaults(context.Background()); err != nil {
		return nil, nil, err
	}
	backfillPinyin(db)
	startLogCleanup(settings, audit)
	// 管理员账号：仅当 users 表为空时创建（用 config.yaml 中的密码）
	if err := common.EnsureAdmin(context.Background(), users, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		return nil, nil, err
	}

	limiter := common.NewRateLimiter()
	limiter.SetPolicy(settings.GetLoginLimit(context.Background()))

	server := &api.Server{
		Users:       users,
		Platforms:   platforms,
		Grants:      grants,
		Audit:       audit,
		Settings:    settings,
		Tickets:     common.NewTicketStore(),
		VerCodes:    common.NewVerCodeStore(),
		Secret:      cfg.TokenSecret,
		TokenTTL:    cfg.TokenTTL,
		Limiter:     limiter,
		Initialized: true,
	}
	return server, db, nil
}

func closeDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// startLogCleanup 启动登录日志自动清理：启动 1 分钟后执行一次，此后每 24 小时执行。
func startLogCleanup(settings *common.SettingsStore, audit *common.AuditStore) {
	go func() {
		cleanupOnce := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			days := settings.GetLogRetentionDays(ctx)
			cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
			deleted, err := audit.Cleanup(ctx, cutoff)
			if err != nil {
				common.LogError("cleanup", "清理登录日志失败: %v", err)
				return
			}
			if deleted > 0 {
				common.LogInfo("cleanup", "自动清理完成：删除 %d 天前的登录日志 %d 条", days, deleted)
			}
		}
		time.Sleep(1 * time.Minute)
		cleanupOnce()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanupOnce()
		}
	}()
}

// backfillPinyin 存量数据回填：为昵称非空但 nickname_pinyin 为空的用户补算拼音。
func backfillPinyin(db *gorm.DB) {
	var users []model.User
	if err := db.Where("nickname <> '' AND nickname_pinyin = ''").Find(&users).Error; err != nil {
		common.LogError("backfill", "query users: %v", err)
		return
	}
	for _, u := range users {
		p := common.Pinyin(u.Nickname)
		if err := db.Model(&u).Update("nickname_pinyin", p).Error; err != nil {
			common.LogError("backfill", "update user %s: %v", u.Username, err)
			continue
		}
		common.LogInfo("backfill", "%s nickname=%q → pinyin=%q", u.Username, u.Nickname, p)
	}
	if n := len(users); n > 0 {
		common.LogInfo("backfill", "完成回填 %d 个用户", n)
	}
}
