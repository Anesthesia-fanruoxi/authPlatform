package common

import (
	"errors"
	"sync"
	"time"
)

// 登录限流（账号维度兜底，防爆破；平台侧自行再做频率限制）：
// 15 分钟内连续失败 5 次，锁定该账号 15 分钟。
const (
	maxLoginFails = 5
	failWindow    = 15 * time.Minute
	lockDuration  = 15 * time.Minute
)

// ErrLocked 账号已被临时锁定。
var ErrLocked = errors.New("登录尝试过多，账号已临时锁定，请稍后再试")

type failInfo struct {
	count      int
	windowTime time.Time
	lockedTill time.Time
}

// RateLimiter 内存版账号维度失败计数与锁定（单实例进程内有效）。
type RateLimiter struct {
	mu    sync.Mutex
	fails map[string]*failInfo
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{fails: make(map[string]*failInfo)}
}

// Check 返回 nil 可继续尝试；返回 ErrLocked 表示账号在锁定期内。
func (r *RateLimiter) Check(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.fails[key]
	if !ok {
		return nil
	}
	if time.Now().Before(f.lockedTill) {
		return ErrLocked
	}
	return nil
}

// RecordFail 记录一次失败；达到阈值即锁定。
func (r *RateLimiter) RecordFail(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	f, ok := r.fails[key]
	if !ok || now.Sub(f.windowTime) > failWindow {
		f = &failInfo{windowTime: now}
		r.fails[key] = f
	}
	f.count++
	if f.count >= maxLoginFails {
		f.lockedTill = now.Add(lockDuration)
		f.count = 0
	}
}

// Reset 登录成功后清零计数。
func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.fails, key)
}
