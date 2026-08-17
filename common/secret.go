package common

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewPlatformSecret 生成平台独立加密盐：32 字节随机数，hex 编码（64 字符）。
func NewPlatformSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}
