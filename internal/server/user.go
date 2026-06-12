package server

import (
	"context"

	"github.com/Serendipity565/gora/internal/domain"
	"github.com/Serendipity565/gora/internal/errs"
	"github.com/Serendipity565/gora/internal/repository"
	"github.com/Serendipity565/gora/internal/repository/model"
	"golang.org/x/crypto/bcrypt"
)

type UserService interface {
	// Register 注册新用户
	Register(ctx context.Context, user *domain.User) (*domain.UserInfo, error)
	// Login 登录验证，成功返回用户信息
	Login(ctx context.Context, user *domain.User) (*domain.UserInfo, error)
	// GetByID 根据 ID 获取用户信息
	GetByID(ctx context.Context, id uint64) (*domain.UserInfo, error)
	// UpdateProfile 更新用户资料
	UpdateProfile(ctx context.Context, user *domain.User) (*domain.UserInfo, error)
	// EnsureAdmin 启动期 seed：如果 email 对应账号尚未存在则按 user 创建。
	// 已存在则跳过（不会更新现有账号的密码 / 用户名）。
	EnsureAdmin(ctx context.Context, user *domain.User) error
}

type userServiceImpl struct {
	dao repository.UserDAO
}

func NewUserService(dao repository.UserDAO) UserService {
	return &userServiceImpl{
		dao: dao,
	}
}

func (s *userServiceImpl) Register(ctx context.Context, user *domain.User) (*domain.UserInfo, error) {
	// 检查邮箱是否已注册
	existing, err := s.dao.FindOne(ctx, repository.ByEmail(user.Email))
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errs.ErrEmailAlreadyUsed(err)
	}

	// 加密密码
	hashed, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	mUser := &model.User{
		Email:    user.Email,
		Password: string(hashed),
		Username: user.Username,
		Status:   model.UserStatusActive,
	}

	if err := s.dao.Create(ctx, mUser); err != nil {
		return nil, err
	}

	return toUserInfo(mUser), nil
}

func (s *userServiceImpl) Login(ctx context.Context, user *domain.User) (*domain.UserInfo, error) {
	mUser, err := s.dao.FindOne(ctx, repository.ByEmail(user.Email))
	if err != nil {
		return nil, err
	}
	if mUser == nil {
		return nil, errs.ErrUserNotFound(nil)
	}

	// 检查账户状态
	if mUser.Status == model.UserStatusDisabled {
		return nil, errs.ErrUserDisabled(nil)
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(mUser.Password), []byte(user.Password)); err != nil {
		return nil, errs.ErrInvalidPassword(err)
	}

	return toUserInfo(mUser), nil
}

func (s *userServiceImpl) GetByID(ctx context.Context, id uint64) (*domain.UserInfo, error) {
	user, err := s.dao.FindOne(ctx, repository.ByID(id))
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errs.ErrUserNotFound(nil)
	}

	return toUserInfo(user), nil
}

func (s *userServiceImpl) UpdateProfile(ctx context.Context, user *domain.User) (*domain.UserInfo, error) {
	mUser, err := s.dao.FindOne(ctx, repository.ByID(user.ID))
	if err != nil {
		return nil, err
	}
	if mUser == nil {
		return nil, errs.ErrUserNotFound(nil)
	}

	// 仅更新非空字段，password 需要重新 bcrypt 加密
	if user.Username != "" {
		mUser.Username = user.Username
	}
	if user.Password != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		mUser.Password = string(hashed)
	}

	if err := s.dao.Update(ctx, mUser); err != nil {
		return nil, err
	}

	return toUserInfo(mUser), nil
}

// toUserInfo 转为对外 UserInfo，确保不泄露 Password 等敏感字段
func toUserInfo(u *model.User) *domain.UserInfo {
	return &domain.UserInfo{
		ID:       u.ID,
		Email:    u.Email,
		Username: u.Username,
		Status:   int(u.Status),
	}
}

// EnsureAdmin 启动期 seed：若 email 对应账号不存在则按 user 创建；
// 已存在则跳过——不会更新现有账号的密码 / 用户名，避免每次重启被覆盖。
//
// 三个字段中任意一个为空时直接返回 nil（视为"未配置 admin"）。
func (s *userServiceImpl) EnsureAdmin(ctx context.Context, user *domain.User) error {
	if user == nil {
		return nil
	}
	if user.Email == "" || user.Password == "" || user.Username == "" {
		return nil
	}

	existing, err := s.dao.FindOne(ctx, repository.ByEmail(user.Email))
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.dao.Create(ctx, &model.User{
		Email:    user.Email,
		Password: string(hashed),
		Username: user.Username,
		Status:   model.UserStatusActive,
	})
}
