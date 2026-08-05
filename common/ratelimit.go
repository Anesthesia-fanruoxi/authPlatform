package common

import (
	"errors"
	"sync"
	"time"
)

// 登录限流（账号维度兜底，防爆破；平台侧自行再做频率限制）。
// 策略由 sys_settings.login_limit 配置，默认：15 分钟内连续失败 5 次，锁定 15 分钟。
// 数据保存在内存（单实例进程内有效）：重启即清零；如需持久化/多实例共享，可迁移 Redis 或 MySQL。
var (
	// ErrLocked 账号已被自动锁定（限流触发）。
	ErrLocked = errors.New("登录尝试过多，账号已临时锁定，请稍后再试")
	// ErrBanned 账号已被管理员加入黑名单。
	ErrBanned = errors.New("账号已被加入黑名单")
)

const (
	// BanSourceAuto 自动锁定（限流触发）；BanSourceManual 管理员手动加入。
	BanSourceAuto   = "auto"
	BanSourceManual = "manual"
)

// BanRecord 黑名单/锁定记录（内存存储）。
type BanRecord struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Source    string    `json:"source"` // auto / manual
	Reason    string    `json:"reason"`
	Operator  string    `json:"operator"` // 手动操作管理员；自动锁定为空
	ExpiresAt time.Time `json:"expires_at"` // 零值 = 永久
	CreatedAt time.Time `json:"created_at"`
}

// IsExpired 判断记录是否已过期（永久记录永不失效）。
func (b BanRecord) IsExpired(now time.Time) bool {
	return !b.ExpiresAt.IsZero() && now.After(b.ExpiresAt)
}

// IsActive 判断记录当前是否生效。
func (b BanRecord) IsActive(now time.Time) bool {
	return b.ExpiresAt.IsZero() || now.Before(b.ExpiresAt)
}

type failInfo struct {
	count      int
	windowTime time.Time
	lockedTill time.Time
}

// RateLimiter 内存版账号维度限流 + 黑名单（单实例进程内有效）。
// 策略可在运行时通过 SetPolicy 更新（与 sys_settings.login_limit 联动）。
type RateLimiter struct {
	mu    sync.Mutex
	fails map[string]*failInfo
	bans  map[string]*BanRecord // username -> record
	banSeq int64

	maxFails int
	window   time.Duration
	lock     time.Duration
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		fails:    make(map[string]*failInfo),
		bans:     make(map[string]*BanRecord),
		maxFails: 5,
		window:   15 * time.Minute,
		lock:     15 * time.Minute,
	}
}

// SetPolicy 更新限流策略（maxFails<=0 时忽略该参数，保持原值）。
func (r *RateLimiter) SetPolicy(limit LoginLimit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit.MaxFails > 0 {
		r.maxFails = limit.MaxFails
	}
	if limit.WindowMinutes > 0 {
		r.window = time.Duration(limit.WindowMinutes) * time.Minute
	}
	if limit.LockMinutes > 0 {
		r.lock = time.Duration(limit.LockMinutes) * time.Minute
	}
}

// Check 检查账号是否被禁止登录：手动黑名单优先，其次自动锁定，再失败计数锁定。
// 返回 nil 可继续尝试；ErrBanned=黑名单；ErrLocked=限流锁定。
func (r *RateLimiter) Check(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if b, ok := r.bans[key]; ok {
		if b.Source == BanSourceManual {
			if b.IsActive(now) {
				return ErrBanned
			}
			// 手动记录过期：保留展示（列表标记过期），但不再拦截
		} else if b.Source == BanSourceAuto {
			if b.IsActive(now) {
				return ErrLocked
			}
			delete(r.bans, key) // 自动锁定到期，惰性清除
		}
	}
	f, ok := r.fails[key]
	if ok && now.Before(f.lockedTill) {
		return ErrLocked
	}
	return nil
}

// RecordFail 记录一次失败；达到阈值即锁定并写入自动黑名单。
func (r *RateLimiter) RecordFail(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	f, ok := r.fails[key]
	if !ok || now.Sub(f.windowTime) > r.window {
		f = &failInfo{windowTime: now}
		r.fails[key] = f
	}
	f.count++
	if f.count >= r.maxFails {
		f.lockedTill = now.Add(r.lock)
		f.count = 0
		if b, exists := r.bans[key]; !exists || b.Source != BanSourceManual {
			r.banSeq++
			r.bans[key] = &BanRecord{
				ID:        r.banSeq,
				Username:  key,
				Source:    BanSourceAuto,
				Reason:    "连续登录失败触发自动锁定",
				ExpiresAt: now.Add(r.lock),
				CreatedAt: now,
			}
		}
	}
}

// Reset 登录成功后清零计数并清除自动锁定（手动黑名单保留）。
func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.fails, key)
	if b, ok := r.bans[key]; ok && b.Source == BanSourceAuto {
		delete(r.bans, key)
	}
}

// Ban 管理员手动加入黑名单（expiresAt 零值=永久）。同时清空失败计数。
func (r *RateLimiter) Ban(key, reason, operator string, expiresAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.fails, key)
	r.banSeq++
	r.bans[key] = &BanRecord{
		ID:        r.banSeq,
		Username:  key,
		Source:    BanSourceManual,
		Reason:    reason,
		Operator:  operator,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
}

// Unban 解除黑名单/锁定（自动与手动均解除），并清空失败计数。
func (r *RateLimiter) Unban(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.bans, key)
	delete(r.fails, key)
}

// ListBans 返回全部黑名单记录快照（含已过期的自动记录会在下次 Check 时清除）。
func (r *RateLimiter) ListBans() []BanRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]BanRecord, 0, len(r.bans))
	for _, b := range r.bans {
		out = append(out, *b)
	}
	// 按加入时间倒序
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].CreatedAt.After(out[j-1].CreatedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
