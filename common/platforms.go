package common

import (
	"context"
	"errors"

	"github.com/anesthesia-fanruoxi/authplatform/model"
	"gorm.io/gorm"
)

type PlatformStore struct {
	db *gorm.DB
}

func NewPlatformStore(db *gorm.DB) *PlatformStore {
	return &PlatformStore{db: db}
}

func (s *PlatformStore) Create(ctx context.Context, p *model.Platform) error {
	return s.db.WithContext(ctx).Create(p).Error
}

func (s *PlatformStore) GetByID(ctx context.Context, id int64) (*model.Platform, error) {
	var p model.Platform
	err := s.db.WithContext(ctx).First(&p, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (s *PlatformStore) GetByPlatformID(ctx context.Context, platformID string) (*model.Platform, error) {
	var p model.Platform
	err := s.db.WithContext(ctx).Where("platform_id = ?", platformID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &p, err
}

// List 平台列表（按 id 升序）。
func (s *PlatformStore) List(ctx context.Context) ([]*model.Platform, error) {
	var list []*model.Platform
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *PlatformStore) Update(ctx context.Context, id int64, updates map[string]any) error {
	return s.db.WithContext(ctx).Model(&model.Platform{}).Where("id = ?", id).Updates(updates).Error
}

func (s *PlatformStore) Delete(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Delete(&model.Platform{}, id).Error
}
