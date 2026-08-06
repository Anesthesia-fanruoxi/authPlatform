package common

import (
	"context"

	"authplatform/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GrantStore struct {
	db *gorm.DB
}

func NewGrantStore(db *gorm.DB) *GrantStore {
	return &GrantStore{db: db}
}

// SetForUser 全量替换某用户的平台授权集合（事务：先清后插）。
func (s *GrantStore) SetForUser(ctx context.Context, userID int64, platformIDs []int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserPlatformGrant{}).Error; err != nil {
			return err
		}
		for _, pid := range platformIDs {
			g := &model.UserPlatformGrant{UserID: userID, PlatformID: pid, Status: 1}
			if err := tx.Create(g).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GrantUsers 批量给平台添加授权用户（已授权则跳过，不影响其他用户）。
func (s *GrantStore) GrantUsers(ctx context.Context, platformID int64, userIDs []int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, uid := range userIDs {
			g := &model.UserPlatformGrant{UserID: uid, PlatformID: platformID, Status: 1}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(g).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RevokeUsers 批量移除平台的授权用户（只移除指定用户，不影响其他用户）。
func (s *GrantStore) RevokeUsers(ctx context.Context, platformID int64, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Where("platform_id = ? AND user_id IN ?", platformID, userIDs).Delete(&model.UserPlatformGrant{}).Error
}

func (s *GrantStore) GetByUser(ctx context.Context, userID int64) ([]*model.UserPlatformGrant, error) {
	var list []*model.UserPlatformGrant
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&list).Error
	return list, err
}

func (s *GrantStore) GetByPlatform(ctx context.Context, platformID int64) ([]*model.UserPlatformGrant, error) {
	var list []*model.UserPlatformGrant
	err := s.db.WithContext(ctx).Where("platform_id = ? AND status = 1", platformID).Find(&list).Error
	return list, err
}

func (s *GrantStore) ListAll(ctx context.Context) ([]*model.UserPlatformGrant, error) {
	var list []*model.UserPlatformGrant
	err := s.db.WithContext(ctx).Find(&list).Error
	return list, err
}

// Granted 判断用户是否被授权该平台（status=1 且存在）。
func (s *GrantStore) Granted(ctx context.Context, userID, platformID int64) (bool, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&model.UserPlatformGrant{}).
		Where("user_id = ? AND platform_id = ? AND status = 1", userID, platformID).
		Count(&n).Error
	return n > 0, err
}

// DeleteByUser 级联删除某用户全部授权。
func (s *GrantStore) DeleteByUser(ctx context.Context, userID int64) error {
	return s.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.UserPlatformGrant{}).Error
}

// DeleteByPlatform 级联删除某平台全部授权。
func (s *GrantStore) DeleteByPlatform(ctx context.Context, platformID int64) error {
	return s.db.WithContext(ctx).Where("platform_id = ?", platformID).Delete(&model.UserPlatformGrant{}).Error
}
