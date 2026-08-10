// Package model 定义核心数据模型（GORM 定义即表结构，AutoMigrate 建表）。
package model

import "time"

type User struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UID          string    `gorm:"size:32;uniqueIndex;not null" json:"uid"`
	Username     string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	Nickname         string    `gorm:"size:64;default:''" json:"nickname"`
	NicknamePinyin   string    `gorm:"size:128;default:''" json:"nickname_pinyin"`
	// Phone/Email 可空（NULL 不参与唯一约束，允许空值重复）
	Phone        *string   `gorm:"size:20;uniqueIndex" json:"phone"`
	Email        *string   `gorm:"size:128;uniqueIndex" json:"email"`
	// TOTPSecret 双因子 TOTP 密钥（base32，空表示未启用 TOTP）
	TOTPSecret  string    `gorm:"size:64;default:''" json:"-"`
	TOTPEnabled bool      `gorm:"default:false" json:"totp_enabled"`
	// Category 用户分类（开发/测试/运营/风控/数分等，管理员在系统设置维护），用于快捷授权平台。
	Category    string    `gorm:"size:64;default:''" json:"category"`
	Status      int       `gorm:"default:1" json:"status"` // 1 启用 0 禁用
	IsAdmin     bool      `gorm:"default:false" json:"is_admin"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名（默认 users，显式声明更清晰）。
func (User) TableName() string { return "users" }

// SafeUser 返回对外可见的用户信息（绝不包含 password_hash 与 TOTPSecret）。
func (u *User) SafeUser() map[string]any {
	phone, email := "", ""
	if u.Phone != nil {
		phone = *u.Phone
	}
	if u.Email != nil {
		email = *u.Email
	}
	return map[string]any{
		"id":           u.ID,
		"uid":          u.UID,
		"username":     u.Username,
		"nickname":        u.Nickname,
		"nickname_pinyin": u.NicknamePinyin,
		"phone":        phone,
		"email":        email,
		"totp_enabled": u.TOTPEnabled,
		"category":     u.Category,
		"status":       u.Status,
		"is_admin":     u.IsAdmin,
		"created_at":   u.CreatedAt.Format(time.RFC3339),
	}
}
