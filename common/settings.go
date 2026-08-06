// Package common 系统设置存储：key-value 持久化 + 内存缓存，供登录流程/密码策略/限流读取。
package common

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"authplatform/model"
	"gorm.io/gorm"
)

// 登录方式枚举（login_methods.methods 的取值）
const (
	LoginMethodUsernamePassword = "username_password" // 用户名 + 密码
	LoginMethodEmailPassword    = "email_password"    // 邮箱 + 密码
	LoginMethodPhoneCode        = "phone_code"        // 手机号 + 验证码
	LoginMethodTOTP             = "totp"              // TOTP 双因子验证码
	LoginMethodEmailCode        = "email_code"        // 邮箱验证码（发码预留，可作第二因子）
)

// PasswordPolicy 密码安全设置。
type PasswordPolicy struct {
	MinLength      int  `json:"min_length"`
	RequireLetter  bool `json:"require_letter"`
	RequireDigit   bool `json:"require_digit"`
	RequireSpecial bool `json:"require_special"`
}

// LoginLimit 登录限流设置。
type LoginLimit struct {
	MaxFails      int `json:"max_fails"`      // 窗口内最大失败次数
	WindowMinutes int `json:"window_minutes"` // 统计窗口（分钟）
	LockMinutes   int `json:"lock_minutes"`   // 锁定时间（分钟）
}

// LoginMethods 登录逻辑设置。
type LoginMethods struct {
	// Methods 登录方式列表，按序执行：len==1 单步；len>1 多步骤（每步验证一个方式）。
	Methods []string `json:"methods"`
}

// AdminIPWhitelist 后台登录 IP 白名单（空数组 = 不限制）。
type AdminIPWhitelist struct {
	IPs []string `json:"ips"`
}

var defaultPasswordPolicy = PasswordPolicy{MinLength: 8, RequireLetter: true, RequireDigit: true, RequireSpecial: false}
var defaultLoginLimit = LoginLimit{MaxFails: 5, WindowMinutes: 15, LockMinutes: 15}
var defaultLoginMethods = LoginMethods{Methods: []string{LoginMethodUsernamePassword}}

// SettingsStore 系统设置存取（读穿缓存：读时若缓存为空则查库并填充）。
type SettingsStore struct {
	db *gorm.DB
}

func NewSettingsStore(db *gorm.DB) *SettingsStore {
	return &SettingsStore{db: db}
}

// EnsureDefaults 启动时写入缺失的默认设置（不覆盖已有值）。
func (s *SettingsStore) EnsureDefaults(ctx context.Context) error {
	defaults := map[string]any{
		"password_policy":    defaultPasswordPolicy,
		"login_limit":        defaultLoginLimit,
		"login_methods":      defaultLoginMethods,
		"admin_ip_whitelist": AdminIPWhitelist{IPs: []string{}},
	}
	for key, val := range defaults {
		var n int64
		if err := s.db.WithContext(ctx).Model(&model.SysSetting{}).Where("`key` = ?", key).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		b, _ := json.Marshal(val)
		if err := s.db.WithContext(ctx).Create(&model.SysSetting{Key: key, Value: string(b)}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *SettingsStore) get(ctx context.Context, key string) (string, error) {
	var st model.SysSetting
	err := s.db.WithContext(ctx).Where("`key` = ?", key).First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return st.Value, err
}

func (s *SettingsStore) getJSON(ctx context.Context, key string, out any) error {
	raw, err := s.get(ctx, key)
	if err != nil {
		return err
	}
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}

// GetPasswordPolicy 返回密码策略；未配置/解析失败回退默认值。
func (s *SettingsStore) GetPasswordPolicy(ctx context.Context) PasswordPolicy {
	var p PasswordPolicy
	if err := s.getJSON(ctx, "password_policy", &p); err != nil {
		log.Printf("[settings] password_policy parse error: %v", err)
		return defaultPasswordPolicy
	}
	if p.MinLength <= 0 {
		p = defaultPasswordPolicy
	}
	return p
}

// GetLoginLimit 返回登录限流配置；未配置/解析失败回退默认值。
func (s *SettingsStore) GetLoginLimit(ctx context.Context) LoginLimit {
	var l LoginLimit
	if err := s.getJSON(ctx, "login_limit", &l); err != nil {
		log.Printf("[settings] login_limit parse error: %v", err)
		return defaultLoginLimit
	}
	if l.MaxFails <= 0 {
		l = defaultLoginLimit
	}
	return l
}

// GetLoginMethods 返回登录方式配置；未配置/解析失败回退默认（用户名+密码）。
func (s *SettingsStore) GetLoginMethods(ctx context.Context) LoginMethods {
	var m LoginMethods
	if err := s.getJSON(ctx, "login_methods", &m); err != nil {
		log.Printf("[settings] login_methods parse error: %v", err)
		return defaultLoginMethods
	}
	if len(m.Methods) == 0 {
		m = defaultLoginMethods
	}
	return m
}

// GetAdminIPWhitelist 返回后台登录 IP 白名单。
func (s *SettingsStore) GetAdminIPWhitelist(ctx context.Context) AdminIPWhitelist {
	var w AdminIPWhitelist
	if err := s.getJSON(ctx, "admin_ip_whitelist", &w); err != nil {
		log.Printf("[settings] admin_ip_whitelist parse error: %v", err)
		return AdminIPWhitelist{}
	}
	return w
}

// Set 保存任意设置（value 序列化为 JSON）。
func (s *SettingsStore) Set(ctx context.Context, key string, val any) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return s.setRaw(ctx, key, string(b))
}

func (s *SettingsStore) setRaw(ctx context.Context, key, raw string) error {
	var st model.SysSetting
	err := s.db.WithContext(ctx).Where("`key` = ?", key).First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.WithContext(ctx).Create(&model.SysSetting{Key: key, Value: raw}).Error
	}
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&st).Update("value", raw).Error
}

// All 返回全部设置（供管理端展示）。
func (s *SettingsStore) All(ctx context.Context) (map[string]any, error) {
	var list []model.SysSetting
	if err := s.db.WithContext(ctx).Find(&list).Error; err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, st := range list {
		var v any
		if err := json.Unmarshal([]byte(st.Value), &v); err == nil {
			out[st.Key] = v
		}
	}
	return out, nil
}

// AllLoginMethods 全部允许的登录方式（顺序即建议展示顺序）。
func AllLoginMethods() []string {
	return []string{LoginMethodUsernamePassword, LoginMethodEmailPassword, LoginMethodPhoneCode, LoginMethodTOTP}
}

// ValidateLoginMethods 校验登录方式列表合法性：
// 非空、全部为已知方式、无重复、TOTP 不能单独作为登录方式。
// 返回规范化后的列表（保持传入顺序）。
func ValidateLoginMethods(methods []string) ([]string, error) {
	allowed := AllLoginMethods()
	if len(methods) == 0 {
		return nil, errors.New("至少选择一种登录方式")
	}
	seen := map[string]bool{}
	for _, method := range methods {
		ok := false
		for _, a := range allowed {
			if method == a {
				ok = true
				break
			}
		}
		if !ok {
			return nil, errors.New("包含不支持的登录方式: " + method)
		}
		if seen[method] {
			return nil, errors.New("登录方式不能重复")
		}
		seen[method] = true
	}
	if len(methods) == 1 && methods[0] == LoginMethodTOTP {
		return nil, errors.New("TOTP 双因子不能单独作为登录方式，请至少再选择一种")
	}
	return methods, nil
}
