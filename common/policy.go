package common

import (
	"crypto/rand"
	"encoding/binary"
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

// GeneratePassword 按策略自动生成随机密码（crypto/rand 驱动，保证满足最小长度与必含字符类别）。
func GeneratePassword(p PasswordPolicy) (string, error) {
	if p.MinLength <= 0 {
		p = DefaultPasswordPolicy()
	}
	length := p.MinLength
	if length < 8 {
		length = 8
	}
	if length > 64 {
		length = 64
	}
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const digits = "0123456789"
	const specials = "!@#$%^&*_-+="

	// 必含字符池：每个要求类别至少取一个
	all := make([]byte, 0, 128)
	var must []byte
	require := func(pool string) {
		c, err := randByteFrom(pool)
		if err != nil {
			return
		}
		must = append(must, c)
		all = append(all, []byte(pool)...)
	}
	if p.RequireLetter {
		require(letters)
	}
	if p.RequireDigit {
		require(digits)
	}
	if p.RequireSpecial {
		require(specials)
	}
	// 基础池：即使策略全 false 也保证含字母与数字
	if !p.RequireLetter {
		all = append(all, []byte(letters)...)
	}
	if !p.RequireDigit {
		all = append(all, []byte(digits)...)
	}
	if len(all) == 0 {
		all = []byte(letters + digits)
	}

	need := length - len(must)
	if need < 0 {
		need = 0
	}
	buf := make([]byte, 0, length)
	buf = append(buf, must...)
	for i := 0; i < need; i++ {
		c, err := randByteFrom(string(all))
		if err != nil {
			return "", err
		}
		buf = append(buf, c)
	}
	// Fisher-Yates 洗牌（crypto/rand 驱动）
	for i := len(buf) - 1; i > 0; i-- {
		idx, err := randIndex(i + 1)
		if err != nil {
			return "", err
		}
		buf[i], buf[idx] = buf[idx], buf[i]
	}
	return string(buf), nil
}

// randByteFrom 从 pool 中随机取一个字节。
func randByteFrom(pool string) (byte, error) {
	i, err := randIndex(len(pool))
	if err != nil {
		return 0, err
	}
	return pool[i], nil
}

// randIndex 返回 [0, n) 的随机下标（crypto/rand，rejection sampling 消除模偏差）。
func randIndex(n int) (int, error) {
	if n <= 1 {
		return 0, nil
	}
	limit := uint32(1<<31) - (uint32(1<<31) % uint32(n))
	var v uint32
	for {
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return 0, err
		}
		v = binary.BigEndian.Uint32(b) & 0x7fffffff
		if v < limit {
			break
		}
	}
	return int(v % uint32(n)), nil
}
