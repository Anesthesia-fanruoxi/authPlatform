package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// ErrInvalidMasterKey 主密钥格式非法。
var ErrInvalidMasterKey = errors.New("MASTER_KEY 必须为 32 字节的 hex 字符串")

// NewPlatformSecret 生成平台独立加密盐：32 字节随机数，hex 编码（64 字符）。
func NewPlatformSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// EncryptSecret 使用 AES-256-GCM 加密明文，输出 base64(nonce||ciphertext)。
// masterKeyHex 为 32 字节 hex 主密钥（环境变量注入，与设计文档 §6 一致）。
func EncryptSecret(masterKeyHex, plain string) (string, error) {
	key, err := parseMasterKey(masterKeyHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

// DecryptSecret 解密 EncryptSecret 的输出。
func DecryptSecret(masterKeyHex, enc string) (string, error) {
	key, err := parseMasterKey(masterKeyHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}

func parseMasterKey(masterKeyHex string) ([]byte, error) {
	key, err := hex.DecodeString(masterKeyHex)
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidMasterKey
	}
	return key, nil
}
