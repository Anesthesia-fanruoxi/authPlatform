package common

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"authplatform/model"
	"gorm.io/gorm"
)

type AuditStore struct {
	db *gorm.DB
}

func NewAuditStore(db *gorm.DB) *AuditStore {
	return &AuditStore{db: db}
}

// WriteLogin 写入一条登录审计日志（成功/失败均记录，只增不改）。
func (s *AuditStore) WriteLogin(ctx context.Context, username, platformID string, success int, reason, ip string) error {
	return s.WriteLoginDetail(ctx, username, platformID, success, reason, ip, "", "")
}

// WriteLoginDetail 写入登录审计日志并附带请求头/请求体摘要（脱敏后），用于登录链路排查。
func (s *AuditStore) WriteLoginDetail(ctx context.Context, username, platformID string, success int, reason, ip, headers, body string) error {
	log := &model.LoginLog{
		Username:       username,
		PlatformID:     platformID,
		Success:        success,
		Reason:         reason,
		IP:             ip,
		RequestHeaders: headers,
		RequestBody:    body,
	}
	return s.db.WithContext(ctx).Create(log).Error
}

// Cleanup 按时间清理过期的登录日志（login_logs）：删除 created_at 早于 before 的记录。
// 全量请求日志（request_logs）不在此清理范围。返回删除条数与错误。
func (s *AuditStore) Cleanup(ctx context.Context, before time.Time) (int64, error) {
	res := s.db.WithContext(ctx).Where("created_at < ?", before).Delete(&model.LoginLog{})
	return res.RowsAffected, res.Error
}

// sensitiveKeys 审计脱敏的敏感字段：值保留前 4 位便于定位，其余打码。
var sensitiveKeys = map[string]bool{
	"password": true, "credential": true, "old_password": true, "new_password": true,
	"secret": true, "totp_secret": true, "token": true, "desktop_token": true,
}

// SanitizeRequestBody 解析 JSON 请求体并脱敏敏感字段；非 JSON 或超长则截断原样返回。
func SanitizeRequestBody(body []byte) string {
	const maxLen = 2000
	if len(body) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err == nil {
		for k := range m {
			m[k] = sanitizeValue(m[k], k)
		}
		if b, err := json.Marshal(m); err == nil {
			return string(b)
		}
	}
	s := string(body)
	if len(s) > maxLen {
		return s[:maxLen] + "...(truncated)"
	}
	return s
}

func sanitizeValue(v any, key string) any {
	if sensitiveKeys[key] {
		if s, ok := v.(string); ok && len(s) > 4 {
			return s[:4] + "***"
		}
		return "***"
	}
	switch t := v.(type) {
	case map[string]any:
		for k := range t {
			t[k] = sanitizeValue(t[k], k)
		}
	case []any:
		for i := range t {
			t[i] = sanitizeValue(t[i], "")
		}
	}
	return v
}

// SanitizeRequestHeaders 序列化请求头（排序、Authorization 脱敏）。
func SanitizeRequestHeaders(h http.Header) string {
	lines := make([]string, 0, len(h))
	for k, v := range h {
		val := strings.Join(v, ", ")
		if strings.EqualFold(k, "Authorization") {
			if len(val) > 16 {
				val = val[:16] + "***"
			} else {
				val = "***"
			}
		}
		lines = append(lines, k+": "+val)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// ListLogin 查询登录日志（可过滤 username/platform_id/success，按时间倒序）。
func (s *AuditStore) ListLogin(ctx context.Context, username, platformID string, success *int, limit int) ([]*model.LoginLog, error) {
	q := s.db.WithContext(ctx).Model(&model.LoginLog{})
	if username != "" {
		q = q.Where("username = ?", username)
	}
	if platformID != "" {
		q = q.Where("platform_id = ?", platformID)
	}
	if success != nil {
		q = q.Where("success = ?", *success)
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var list []*model.LoginLog
	err := q.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}
