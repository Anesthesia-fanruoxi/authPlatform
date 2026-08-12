package model

import "time"

// LoginLog 登录审计日志（只增不改）。
type LoginLog struct {
	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Username   string `gorm:"size:64;index" json:"username"`
	PlatformID string `gorm:"size:64;index" json:"platform_id"`
	Success    int    `gorm:"index" json:"success"`  // 1 成功 0 失败
	Reason     string `gorm:"size:32" json:"reason"` // ok / bad_cred / disabled / unauthorized / locked / sign_invalid
	IP         string `gorm:"size:45" json:"ip"`
	// RequestHeaders 请求头摘要（Authorization 已脱敏），排查用。
	RequestHeaders string `gorm:"type:text" json:"request_headers"`
	// RequestBody 请求体（password/credential/secret 等敏感字段已脱敏），排查用。
	RequestBody string    `gorm:"type:text" json:"request_body"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (LoginLog) TableName() string { return "login_logs" }
