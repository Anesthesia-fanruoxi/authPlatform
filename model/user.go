// Package model 定义核心数据模型（GORM 定义即表结构，AutoMigrate 建表）。
package model

import "time"

type User struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UID          string    `gorm:"size:32;uniqueIndex;not null" json:"uid"`
	Username     string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	Nickname     string    `gorm:"size:64;default:''" json:"nickname"`
	Status       int       `gorm:"default:1" json:"status"` // 1 启用 0 禁用
	IsAdmin      bool      `gorm:"default:false" json:"is_admin"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名（默认 users，显式声明更清晰）。
func (User) TableName() string { return "users" }

// SafeUser 返回对外可见的用户信息（绝不包含 password_hash）。
func (u *User) SafeUser() map[string]any {
	return map[string]any{
		"uid":        u.UID,
		"username":   u.Username,
		"nickname":   u.Nickname,
		"status":     u.Status,
		"is_admin":   u.IsAdmin,
		"created_at": u.CreatedAt.Format(time.RFC3339),
	}
}
