package model

import "time"

// SysSetting 系统设置（key-value 存储，value 为 JSON 文本）。
// 设置键：
//   - password_policy  密码安全设置  {"min_length":8,"require_letter":true,"require_digit":true,"require_special":false}
//   - login_limit      登录限流设置  {"max_fails":5,"window_minutes":15,"lock_minutes":15}
//   - login_methods    登录方式设置  {"methods":["username_password","totp"]}
//   - admin_ip_whitelist 后台登录 IP 白名单（JSON 数组，空数组=不限制）
type SysSetting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (SysSetting) TableName() string { return "sys_settings" }
