package model

import "time"

// Platform 接入平台。secret 加密存储，明文仅创建/轮换时返回一次。
type Platform struct {
	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	PlatformID string `gorm:"size:64;uniqueIndex;not null" json:"platform_id"`
	Name       string `gorm:"size:128;not null" json:"name"`
	// SecretEnc 当前盐（AES-GCM 密文，base64）
	SecretEnc string `gorm:"size:512;not null" json:"-"`
	// SecretOldEnc 旧盐（密钥轮换双盐过渡期保留，可空；吊销后清空）
	SecretOldEnc string `gorm:"size:512;default:''" json:"-"`
	// Secret 明文盐（仅内存，不落库）
	Secret string `gorm:"-" json:"-"`
	// IPWhitelist JSON 数组文本，如 ["1.2.3.4"]，可空
	IPWhitelist string `gorm:"type:text" json:"ip_whitelist"`
	// LoginMethods 本平台登录方式（JSON 数组文本，如 ["username_password","totp"]）。
	// 空 = 使用系统设置中的「新平台默认登录方式」。
	LoginMethods string    `gorm:"type:text" json:"-"`
	Status       int       `gorm:"default:1" json:"status"` // 1 启用 0 停用
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Platform) TableName() string { return "platforms" }
