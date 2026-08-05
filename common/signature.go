package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// signWindowSeconds 时间戳防重放窗口：允许 ±300s。
const signWindowSeconds = 300

var (
	ErrSignInvalid = errors.New("平台签名无效")
	ErrSignExpired = errors.New("平台签名时间戳过期")
)

// VerifyPlatformSignature 校验平台请求签名（设计文档 §4.2）：
// sign = HMAC-SHA256(secret, method|path|timestamp|body_sha256_hex)
// method/path 为实际请求的方法与路径（如 POST /api/auth/verify），
// body 为原始请求体（空串时 body_sha256 为 sha256("")）。
func VerifyPlatformSignature(secret, method, path, timestamp, body, sign string) error {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrSignInvalid
	}
	now := time.Now().Unix()
	if now-ts > signWindowSeconds || ts-now > signWindowSeconds {
		return ErrSignExpired
	}
	bodyHash := sha256.Sum256([]byte(body))
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s|%s|%s|%x", method, path, timestamp, bodyHash)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sign)) {
		return ErrSignInvalid
	}
	return nil
}
