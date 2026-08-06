package common

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"authplatform/model"
)

// EnsureAdmin 在 users 表为空时创建初始管理员账号。
// 仅首次启动生效；已有任何用户（含普通用户）则跳过，避免覆盖已有数据。
func EnsureAdmin(ctx context.Context, users *UserStore, username, password string) error {
	n, err := users.Count(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if n > 0 {
		return nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	uid, err := NewUID()
	if err != nil {
		return err
	}
	admin := &model.User{
		UID:          uid,
		Username:     username,
		PasswordHash: hash,
		Nickname:     "管理员",
		Status:       1,
		IsAdmin:      true,
	}
	if err := users.Create(ctx, admin); err != nil {
		return fmt.Errorf("create initial admin: %w", err)
	}
	return nil
}

// NewUID 生成对外用户标识 u_<24位hex>。
func NewUID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate uid: %w", err)
	}
	return "u_" + hex.EncodeToString(b), nil
}
