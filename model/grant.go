package model

import "time"

// UserPlatformGrant 用户 ↔ 平台多对多授权。
type UserPlatformGrant struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     int64     `gorm:"uniqueIndex:idx_user_platform;not null" json:"user_id"`
	PlatformID int64     `gorm:"uniqueIndex:idx_user_platform;not null" json:"platform_id"`
	Status     int       `gorm:"default:1" json:"status"` // 1 授权 0 撤销
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (UserPlatformGrant) TableName() string { return "user_platform_grants" }
