package model

import "time"

// RequestLog 全量 API 请求日志（中间件记录：所有 /api/* 请求的方法/路径/头/体，只增不改）。
type RequestLog struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Method     string    `gorm:"size:8;index" json:"method"`           // GET / POST / PUT / DELETE ...
	Path       string    `gorm:"size:255;index" json:"path"`           // 不含 query
	Query      string    `gorm:"size:255" json:"query"`                // 原始 query 串（? 之后）
	PlatformID string    `gorm:"size:64;index" json:"platform_id"`     // 从 X-Platform-Id 提取（无则空）
	IP         string    `gorm:"size:45" json:"ip"`
	Status     int       `gorm:"index" json:"status"`                  // HTTP 状态码（200/400/404...）
	Headers    string    `gorm:"type:text" json:"headers"`             // 请求头（Authorization 已脱敏）
	Body       string    `gorm:"type:text" json:"body"`                // 请求体（敏感字段已脱敏；GET 无 body 为空）
	CreatedAt  time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 请求日志表名。
func (RequestLog) TableName() string { return "request_logs" }
