package dao

import (
	"context"
	"errors"

	"github.com/Serendipity565/gora/internal/repository/model"
	"gorm.io/gorm"
)

type UserDAO interface {
	Create(ctx context.Context, user *model.User) error
	Update(ctx context.Context, user *model.User) error

	FindOne(ctx context.Context, opts ...QueryOption) (*model.User, error)
}

type userDAO struct {
	db *gorm.DB
}

func NewUserDAO(gorm *gorm.DB) UserDAO {
	return &userDAO{
		db: gorm,
	}
}

// QueryOption 查询条件用 Option 模式
type QueryOption func(*queryConfig)
type queryConfig struct {
	email string
	id    uint64
	hasID bool
}

// ByEmail 按邮箱精确匹配（登录场景用）。
func ByEmail(email string) QueryOption {
	return func(c *queryConfig) { c.email = email }
}

// ByID 按主键 id 精确匹配。
func ByID(id uint64) QueryOption {
	return func(c *queryConfig) {
		c.id = id
		c.hasID = true
	}
}

// Create 创建用户
func (u *userDAO) Create(ctx context.Context, user *model.User) error {
	return u.db.WithContext(ctx).Create(user).Error
}

// Update 更新用户
func (u *userDAO) Update(ctx context.Context, user *model.User) error {
	return u.db.WithContext(ctx).Save(user).Error
}

// FindOne 根据条件查询单个用户；条件可选，支持 ByID、ByEmail。
//
// 没有任何条件时直接返回 (nil, nil)，避免误读全表第一行；
// 命中条件但记录不存在时也返回 (nil, nil)，把"不存在"与"出错"区分开，
// service 层以 "user == nil" 判定不存在即可，不必再 errors.Is。
func (u *userDAO) FindOne(ctx context.Context, opts ...QueryOption) (*model.User, error) {
	cfg := &queryConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	query := u.db.WithContext(ctx).Model(&model.User{})
	hasCond := false
	if cfg.hasID {
		query = query.Where("id = ?", cfg.id)
		hasCond = true
	}
	if cfg.email != "" {
		query = query.Where("email = ?", cfg.email)
		hasCond = true
	}
	if !hasCond {
		return nil, nil
	}

	var user model.User
	if err := query.First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}
