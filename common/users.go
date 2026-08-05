package common

import (
	"context"
	"errors"

	"github.com/anesthesia-fanruoxi/authplatform/model"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

type UserStore struct {
	db *gorm.DB
}

func NewUserStore(db *gorm.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(ctx context.Context, u *model.User) error {
	return s.db.WithContext(ctx).Create(u).Error
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := s.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *UserStore) GetByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := s.db.WithContext(ctx).First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *UserStore) GetByUID(ctx context.Context, uid string) (*model.User, error) {
	var u model.User
	err := s.db.WithContext(ctx).Where("uid = ?", uid).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &u, err
}

// ListByIDs 按 ID 集合返回用户（可选 keyword 过滤用户名/昵称）。
func (s *UserStore) ListByIDs(ctx context.Context, ids []int64, keyword string) ([]*model.User, error) {
	if len(ids) == 0 {
		return []*model.User{}, nil
	}
	q := s.db.WithContext(ctx).Model(&model.User{}).Where("id IN ?", ids)
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("username LIKE ? OR nickname LIKE ?", like, like)
	}
	var users []*model.User
	if err := q.Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// List 按关键字（用户名/昵称模糊匹配）返回用户列表，按 id 升序。
func (s *UserStore) List(ctx context.Context, keyword string) ([]*model.User, error) {
	q := s.db.WithContext(ctx).Model(&model.User{})
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("username LIKE ? OR nickname LIKE ?", like, like)
	}
	var users []*model.User
	if err := q.Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// Update 以 map 更新指定字段（避免零值被忽略的问题）。
func (s *UserStore) Update(ctx context.Context, id int64, updates map[string]any) error {
	return s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

func (s *UserStore) Delete(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Delete(&model.User{}, id).Error
}

func (s *UserStore) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&model.User{}).Count(&n).Error
	return n, err
}
