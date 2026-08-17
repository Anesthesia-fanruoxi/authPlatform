package model

import "time"

// Platform 接入平台。secret 明文存储，明文仅创建/轮换时返回一次。
type Platform struct {
	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	PlatformID string `gorm:"size:64;uniqueIndex;not null" json:"platform_id"`
	Name       string `gorm:"size:128;not null" json:"name"`
	// SecretEnc 当前盐（明文存储；字段名保留历史命名）
	SecretEnc string `gorm:"size:512;not null" json:"-"`
	// Secret 明文盐（仅内存，不落库）
	Secret string `gorm:"-" json:"-"`
	// IPWhitelist JSON 数组文本，如 ["1.2.3.4"]，可空
	IPWhitelist string `gorm:"type:text" json:"ip_whitelist"`
	// LoginMethods 本平台登录方式（JSON 数组文本，如 ["username_password","username_totp"]）。
	// 空 = 使用系统设置中的「新平台默认登录方式」。
	LoginMethods string `gorm:"type:text" json:"-"`
	// AuthMode 验证模式：single=单次登录（多选=任一即可）；two_step=二次验证（多选=按顺序全部通过）。默认 two_step。
	AuthMode  string    `gorm:"size:16;default:'two_step'" json:"auth_mode"`
	Status    int       `gorm:"default:1" json:"status"` // 1 启用 0 停用
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Platform) TableName() string { return "platforms" }
