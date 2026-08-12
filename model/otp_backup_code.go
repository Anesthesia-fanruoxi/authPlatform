package model

import "time"

// OTPBackupCode TOTP 备用恢复码（双因子验证兜底）。
// 本期仅建表存储迁移数据（CMDB otp_backup_codes），校验逻辑暂未启用。
type OTPBackupCode struct {
	ID        int64      `gorm:"primaryKey" json:"id"`
	UserID    int64      `gorm:"index;not null" json:"user_id"` // authplatform users.id
	Code      string     `gorm:"size:16;not null" json:"code"`  // 一次性恢复码
	Used      bool       `gorm:"default:false" json:"used"`     // 0=未用 1=已用
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// TableName 恢复码表名。
func (OTPBackupCode) TableName() string { return "otp_backup_codes" }
