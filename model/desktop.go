package model

import "time"

// DesktopSession 桌面客户端会话（desktop_token 哈希存储，支持吊销）。
type DesktopSession struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	UserID    int64     `gorm:"index;not null" json:"user_id"`
	TokenHash string    `gorm:"size:64;uniqueIndex;not null" json:"-"` // sha256(desktop_token)
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `gorm:"default:false" json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 桌面会话表名。
func (DesktopSession) TableName() string { return "desktop_sessions" }

// DesktopPending 桌面免密登录待确认请求（平台发起 → 客户端确认 → 平台换取 token）。
type DesktopPending struct {
	ID         int64     `gorm:"primaryKey" json:"id"`
	RequestID  string    `gorm:"size:64;uniqueIndex;not null" json:"request_id"`
	PlatformID int64     `gorm:"index;not null" json:"platform_id"`
	UserID     int64     `json:"user_id"` // 确认后绑定
	Status     string    `gorm:"size:16;default:'initiated'" json:"status"` // initiated|confirmed|used
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// TableName 桌面待确认请求表名。
func (DesktopPending) TableName() string { return "desktop_pendings" }
