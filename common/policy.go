package common

import (
	"errors"
	"fmt"
	"unicode"
)

// ErrWeakPassword 密码策略校验失败（策略由 sys_settings.password_policy 配置，此处为默认文案）。
var ErrWeakPassword = errors.New("密码不满足安全策略要求")

// DefaultPasswordPolicy 默认密码策略（与历史行为一致：≥8 位且含字母和数字）。
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{MinLength: 8, RequireLetter: true, RequireDigit: true}
}

// ValidatePassword 使用默认策略校验（兼容历史调用方，如初始化流程）。
func ValidatePassword(pw string) error {
	return ValidatePasswordWithPolicy(pw, DefaultPasswordPolicy())
}

// ValidatePasswordWithPolicy 按系统设置中的密码策略校验密码强度。
func ValidatePasswordWithPolicy(pw string, p PasswordPolicy) error {
	if p.MinLength <= 0 {
		p = DefaultPasswordPolicy()
	}
	if len(pw) < p.MinLength {
		return fmt.Errorf("密码长度至少 %d 位", p.MinLength)
	}
	hasLetter, hasDigit, hasSpecial := false, false, false
	for _, r := range pw {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSpecial = true
		}
	}
	if p.RequireLetter && !hasLetter {
		return fmt.Errorf("密码需包含字母")
	}
	if p.RequireDigit && !hasDigit {
		return fmt.Errorf("密码需包含数字")
	}
	if p.RequireSpecial && !hasSpecial {
		return fmt.Errorf("密码需包含特殊字符")
	}
	return nil
}
