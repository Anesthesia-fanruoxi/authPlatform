package common

import (
	"context"

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
	log := &model.LoginLog{
		Username:   username,
		PlatformID: platformID,
		Success:    success,
		Reason:     reason,
		IP:         ip,
	}
	return s.db.WithContext(ctx).Create(log).Error
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
