// Package common TOTP（RFC 6238）实现：基于时间的一次性密码，用于双因子验证。
package common

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	totpPeriod   = 30          // 时间步长（秒）
	totpDigits   = 6           // 密码位数
	totpKeyLen   = 20          // 密钥字节长度（160bit，与 Google Authenticator 一致）
	totpWindow   = 1           // 校验允许的时间步偏差（±1 步容忍时钟漂移）
	totpIssuer   = "authPlatform"
	totpAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
)

// GenerateTOTPSecret 生成 base32 编码的 TOTP 密钥。
func GenerateTOTPSecret() (string, error) {
	raw := make([]byte, totpKeyLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	// base32 无填充编码
	return strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "="), nil
}

// totpCode 计算某个时间步的 TOTP 码（RFC 6238：HMAC-SHA1 + dynamic truncation）。
func totpCode(secret string, t time.Time) (string, error) {
	// 兼容无 padding 的 base32 secret（如 CMDB 导入的 26 字符密钥）
	s := strings.ToUpper(strings.TrimRight(secret, "="))
	if n := len(s) % 8; n != 0 {
		s += strings.Repeat("=", 8-n)
	}
	key, err := base32.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("decode totp secret: %w", err)
	}
	counter := uint64(t.Unix() / totpPeriod)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, code%mod), nil
}

// TOTPCode 返回当前（或指定时间）的 TOTP 码。
func TOTPCode(secret string, t time.Time) (string, error) {
	return totpCode(secret, t)
}

// ValidateTOTP 校验用户输入的 TOTP 码，允许 ±totpWindow 个时间步的偏差。
// 常量时间比较防止时序攻击。
func ValidateTOTP(secret, code string) (bool, error) {
	if secret == "" || code == "" {
		return false, nil
	}
	now := time.Now()
	for offset := -totpWindow; offset <= totpWindow; offset++ {
		expected, err := totpCode(secret, now.Add(time.Duration(offset)*totpPeriod*time.Second))
		if err != nil {
			return false, err
		}
		if hmac.Equal([]byte(expected), []byte(code)) {
			return true, nil
		}
	}
	return false, nil
}

// TOTPURI 生成 otpauth 标准 URI（供二维码/手动录入）。
func TOTPURI(username, secret string) string {
	label := totpIssuer + ":" + username
	return fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=%s&period=%d&digits=%d&algorithm=SHA1",
		label, secret, totpIssuer, totpPeriod, totpDigits)
}
