package common

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"authplatform/model"
	"gorm.io/gorm"
)

// DesktopStore 桌面会话与免密登录待确认请求存储。
type DesktopStore struct {
	db *gorm.DB
}

// NewDesktopStore 创建桌面存储。
func NewDesktopStore(db *gorm.DB) *DesktopStore {
	return &DesktopStore{db: db}
}

// HashToken 计算 token 的 sha256 十六进制（服务端只存哈希，可吊销）。
func HashToken(token string) string {
	s := sha256.Sum256([]byte(token))
	return hex.EncodeToString(s[:])
}

// NewDesktopToken 生成随机 desktop_token，返回明文与哈希。
func NewDesktopToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	return token, HashToken(token), nil
}

// NewPendingRequestID 生成一次性免密登录 request_id。
func NewPendingRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateSession 创建桌面会话。
func (s *DesktopStore) CreateSession(ctx context.Context, userID int64, tokenHash string, ttl time.Duration) (*model.DesktopSession, error) {
	rec := &model.DesktopSession{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(ttl),
	}
	if err := s.db.WithContext(ctx).Create(rec).Error; err != nil {
		return nil, err
	}
	return rec, nil
}

// GetSessionByToken 按明文 token 查有效会话（未吊销、未过期）。
func (s *DesktopStore) GetSessionByToken(ctx context.Context, token string) (*model.DesktopSession, error) {
	var rec model.DesktopSession
	err := s.db.WithContext(ctx).
		Where("token_hash = ? AND revoked = ? AND expires_at > ?", HashToken(token), false, time.Now()).
		First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// RevokeSession 吊销会话。
func (s *DesktopStore) RevokeSession(ctx context.Context, token string) error {
	return s.db.WithContext(ctx).Model(&model.DesktopSession{}).
		Where("token_hash = ?", HashToken(token)).
		Update("revoked", true).Error
}

// CreatePending 创建免密登录待确认请求。
func (s *DesktopStore) CreatePending(ctx context.Context, requestID string, platformID int64, ttl time.Duration) (*model.DesktopPending, error) {
	rec := &model.DesktopPending{
		RequestID:  requestID,
		PlatformID: platformID,
		Status:     "initiated",
		ExpiresAt:  time.Now().Add(ttl),
	}
	if err := s.db.WithContext(ctx).Create(rec).Error; err != nil {
		return nil, err
	}
	return rec, nil
}

// GetPending 查待确认请求。
func (s *DesktopStore) GetPending(ctx context.Context, requestID string) (*model.DesktopPending, error) {
	var rec model.DesktopPending
	err := s.db.WithContext(ctx).Where("request_id = ?", requestID).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// ConfirmPending 确认请求并绑定用户（仅 initiated → confirmed）。
func (s *DesktopStore) ConfirmPending(ctx context.Context, requestID string, userID int64) error {
	return s.db.WithContext(ctx).Model(&model.DesktopPending{}).
		Where("request_id = ? AND status = ?", requestID, "initiated").
		Updates(map[string]any{"status": "confirmed", "user_id": userID}).Error
}

// ConsumePending 消费请求（confirmed → used，平台换取 token 时调用，一次性）。
// 返回是否成功消费；并发下仅一个调用能成功（原子 UPDATE + RowsAffected 判定）。
func (s *DesktopStore) ConsumePending(ctx context.Context, requestID string) (bool, error) {
	res := s.db.WithContext(ctx).Model(&model.DesktopPending{}).
		Where("request_id = ? AND status = ?", requestID, "confirmed").
		Update("status", "used")
	return res.RowsAffected > 0, res.Error
}
