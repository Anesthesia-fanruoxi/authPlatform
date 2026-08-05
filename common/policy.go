package common

import (
	"errors"
	"unicode"
)

// ErrWeakPassword 密码策略校验失败（由 authPlatform 统一执行）。
var ErrWeakPassword = errors.New("密码长度至少 8 位，且需同时包含字母和数字")

// ValidatePassword 校验密码强度：长度 ≥ 8，且同时包含字母和数字。
func ValidatePassword(pw string) error {
	if len(pw) < 8 {
		return ErrWeakPassword
	}
	hasLetter, hasDigit := false, false
	for _, r := range pw {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}
