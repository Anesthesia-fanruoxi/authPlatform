package common

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 管理会话 token：无状态 HMAC 签名，形如 <payload>.<sig>。
// payload = base64url("<userID>.<expUnix>")；sig = hex(HMAC-SHA256(secret, payload))。
// 与平台侧不透明 token 不同，管理会话需要可自校验（无需存库）。

var (
	ErrTokenInvalid = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

// HashToken 计算 token 的 sha256 十六进制（用于管理后台单会话：新登录覆盖旧 token）。
func HashToken(token string) string {
	s := sha256.Sum256([]byte(token))
	return hex.EncodeToString(s[:])
}

func SignSessionToken(secret string, userID int64, ttl time.Duration) (string, error) {
	exp := time.Now().Add(ttl).Unix()
	// nonce 保证每次登录 token 唯一（同一秒多次登录互不相同，支持单点登录互踢）
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d.%d.%s", userID, exp, hex.EncodeToString(nonce))))
	sig := hmacSHA256Hex(secret, payload)
	return payload + "." + sig, nil
}

// VerifySessionToken 校验签名与有效期，返回 userID。
func VerifySessionToken(secret, token string) (int64, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return 0, ErrTokenInvalid
	}
	payload, sig := parts[0], parts[1]
	if !hmac.Equal([]byte(hex.EncodeToString(hmacSHA256(secret, payload))), []byte(sig)) {
		return 0, ErrTokenInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return 0, ErrTokenInvalid
	}
	sub := strings.SplitN(string(raw), ".", 3)
	if len(sub) < 2 {
		return 0, ErrTokenInvalid
	}
	userID, err := strconv.ParseInt(sub[0], 10, 64)
	if err != nil {
		return 0, ErrTokenInvalid
	}
	exp, err := strconv.ParseInt(sub[1], 10, 64)
	if err != nil {
		return 0, ErrTokenInvalid
	}
	if time.Now().Unix() > exp {
		return 0, ErrTokenExpired
	}
	return userID, nil
}

func hmacSHA256(secret, data string) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(data))
	return m.Sum(nil)
}

func hmacSHA256Hex(secret, data string) string {
	return fmt.Sprintf("%x", hmacSHA256(secret, data))
}
