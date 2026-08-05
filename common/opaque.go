package common

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewOpaqueToken 签发不透明 token：32 字节随机数 hex 编码。
// authPlatform 签发后不管理（不吊销/不续期），生命周期由接入平台自行维护。
func NewOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
