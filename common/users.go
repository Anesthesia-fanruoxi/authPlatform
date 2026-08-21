package common

import (
	"context"
	"errors"
	"strings"

	"authplatform/model"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

// subseqLike 构造子序列匹配模式：关键字每字符间插 %（兼容连续子串），转义 LIKE 通配符。
func subseqLike(k string) string {
	var b strings.Builder
	for i, r := range []rune(k) {
		if i > 0 {
			b.WriteByte('%')
		}
		if r == '%' || r == '_' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String() + "%"
}

// UserStore 用户存取：全部字段明文存储。
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

// GetByIdentifier 按登录标识查用户：优先 username（SQL 精确），其次 email/phone 精确匹配。
func (s *UserStore) GetByIdentifier(ctx context.Context, identifier string) (*model.User, error) {
	// 1. username 精确匹配（可索引）
	var u model.User
	err := s.db.WithContext(ctx).Where("username = ?", identifier).First(&u).Error
	if err == nil {
		return &u, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// 2. email / phone 精确匹配
	if u2, err := s.findByField(ctx, "email", identifier); err == nil {
		return u2, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if u2, err := s.findByField(ctx, "phone", identifier); err == nil {
		return u2, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return nil, ErrNotFound
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	return s.findByField(ctx, "email", email)
}

func (s *UserStore) GetByPhone(ctx context.Context, phone string) (*model.User, error) {
	return s.findByField(ctx, "phone", phone)
}

// findByField 按字段值精确匹配单个用户（用于 email/phone）。
func (s *UserStore) findByField(ctx context.Context, column, value string) (*model.User, error) {
	var u model.User
	err := s.db.WithContext(ctx).Where(column+" = ?", value).First(&u).Error
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

// ListByIDs 按 ID 集合返回用户（可选 keyword 子序列匹配用户名/昵称/拼音）。
func (s *UserStore) ListByIDs(ctx context.Context, ids []int64, keyword string) ([]*model.User, error) {
	if len(ids) == 0 {
		return []*model.User{}, nil
	}
	q := s.db.WithContext(ctx).Model(&model.User{}).Where("id IN ?", ids)
	if keyword != "" {
		like := subseqLike(keyword)
		q = q.Where("username LIKE ? OR nickname LIKE ? OR nickname_pinyin LIKE ?", like, like, like)
	}
	var users []*model.User
	if err := q.Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// UserFilter 用户列表筛选条件。
type UserFilter struct {
	Keyword      string // 子序列匹配用户名/昵称/拼音
	ExcludeAdmin bool   // 排除超级管理员
	Category     string // 精确分类（非空时生效）
	HasCategory  *bool  // 已分类(true)/未分类(false)，与 Category 互斥
	Status       *int   // 启用(1)/禁用(0)
	TOTPEnabled  *bool  // 双因子启用(true)/未启用(false)
}

// List 按条件返回用户列表。
func (s *UserStore) List(ctx context.Context, f UserFilter) ([]*model.User, error) {
	q := s.db.WithContext(ctx).Model(&model.User{})
	if f.Keyword != "" {
		like := subseqLike(f.Keyword)
		q = q.Where("username LIKE ? OR nickname LIKE ? OR nickname_pinyin LIKE ?", like, like, like)
	}
	if f.ExcludeAdmin {
		q = q.Where("is_admin = ?", false)
	}
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	if f.HasCategory != nil {
		if *f.HasCategory {
			q = q.Where("category <> ''")
		} else {
			q = q.Where("category = ''")
		}
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.TOTPEnabled != nil {
		q = q.Where("totp_enabled = ?", *f.TOTPEnabled)
	}
	var users []*model.User
	if err := q.Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// Update 以 map 更新指定字段。
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

// UpdateCategory 批量设置用户分类（空串表示清除分类）。
func (s *UserStore) UpdateCategory(ctx context.Context, userIDs []int64, category string) error {
	if len(userIDs) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&model.User{}).
		Where("id IN ?", userIDs).
		Update("category", category).Error
}
