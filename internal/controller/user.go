package controller

import (
	"strconv"

	"github.com/Serendipity565/gora/api/request"
	"github.com/Serendipity565/gora/api/response"
	"github.com/Serendipity565/gora/internal/domain"
	"github.com/Serendipity565/gora/internal/errs"
	"github.com/Serendipity565/gora/internal/server"
	"github.com/Serendipity565/gora/pkg/ijwt"
	"github.com/gin-gonic/gin"
)

// parseUserID 从 JWT claims 中解析 uint64 用户 ID。
func parseUserID(uc ijwt.UserClaims) (uint64, error) {
	return strconv.ParseUint(uc.UserId, 10, 64)
}

type UserHandler interface {
	Register(c *gin.Context, req request.RegisterRequest) (response.Response, error)
	Login(c *gin.Context, req request.LoginRequest) (response.Response, error)
	UpdateProfile(c *gin.Context, req request.UpdateProfileRequest, uc ijwt.UserClaims) (response.Response, error)
}

type User struct {
	jwt *ijwt.JWT
	s   server.UserService
}

func NewUser(jwt *ijwt.JWT, s server.UserService) UserHandler {
	return &User{
		jwt: jwt,
		s:   s,
	}
}

// Register 注册新用户
//
//	@Summary		用户注册
//	@Description	使用邮箱、密码、用户名注册新用户
//	@Tags			User
//	@ID				registerUser
//	@Accept			json
//	@Produce		json
//	@Param			request	body		request.RegisterRequest								true	"注册请求参数"
//	@Success		200		{object}	response.Response{data=response.RegisterResponse}	"注册成功，返回用户信息"
//	@Failure		400		{object}	response.Response									"请求参数错误 / 邮箱已被使用"
//	@Failure		500		{object}	response.Response									"服务器错误"
//	@Router			/api/user/register [post]
func (h *User) Register(c *gin.Context, req request.RegisterRequest) (response.Response, error) {
	info, err := h.s.Register(c.Request.Context(), &domain.User{
		Email:    req.Email,
		Password: req.Password,
		Username: req.Username,
	})
	if err != nil {
		return response.Response{}, err
	}

	return response.Response{
		Code:    0,
		Message: "success",
		Data: response.RegisterResponse{
			User: toUserInfoResponse(info),
		},
	}, nil
}

// Login 登录
//
//	@Summary		用户登录
//	@Description	使用邮箱与密码登录，成功后返回 JWT 令牌与用户信息
//	@Tags			User
//	@ID				loginUser
//	@Accept			json
//	@Produce		json
//	@Param			request	body		request.LoginRequest							true	"登录请求参数"
//	@Success		200		{object}	response.Response{data=response.LoginResponse}	"登录成功"
//	@Failure		400		{object}	response.Response								"请求参数错误"
//	@Failure		401		{object}	response.Response								"密码错误"
//	@Failure		403		{object}	response.Response								"账号已被禁用"
//	@Failure		404		{object}	response.Response								"用户不存在"
//	@Router			/api/user/login [post]
func (h *User) Login(c *gin.Context, req request.LoginRequest) (response.Response, error) {
	info, err := h.s.Login(c.Request.Context(), &domain.User{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return response.Response{}, err
	}

	token, err := h.jwt.SetJWTToken(info.ID, info.Email)
	if err != nil {
		return response.Response{}, errs.InternalServerError(err)
	}

	c.Header("Authorization", token)

	return response.Response{
		Code:    0,
		Message: "success",
		Data: response.LoginResponse{
			Token: token,
			User:  toUserInfoResponse(info),
		},
	}, nil
}

// UpdateProfile 更新用户资料
//
//	@Summary		更新用户资料
//	@Description	根据 JWT 中的用户身份更新当前用户的用户名 / 密码（两者至少传一个，未填字段保持不变）
//	@Tags			User
//	@ID				updateProfile
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string												true	"Bearer Token"
//	@Param			request			body		request.UpdateProfileRequest						true	"更新用户资料请求参数"
//	@Success		200				{object}	response.Response{data=response.UpdateProfileResponse}	"更新成功，返回更新后的用户信息"
//	@Failure		400				{object}	response.Response									"请求参数错误"
//	@Failure		401				{object}	response.Response									"未登录或 token 无效"
//	@Failure		404				{object}	response.Response									"用户不存在"
//	@Failure		500				{object}	response.Response									"服务器错误"
//	@Router			/api/user/profile [post]
func (h *User) UpdateProfile(c *gin.Context, req request.UpdateProfileRequest, uc ijwt.UserClaims) (response.Response, error) {
	id, err := parseUserID(uc)
	if err != nil {
		return response.Response{}, errs.ErrUserNotFound(err)
	}

	info, err := h.s.UpdateProfile(c.Request.Context(), &domain.User{
		ID:       id,
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		return response.Response{}, err
	}

	return response.Response{
		Code:    0,
		Message: "success",
		Data: response.UpdateProfileResponse{
			User: toUserInfoResponse(info),
		},
	}, nil
}

// toUserInfoResponse 把 domain.UserInfo 转成对外响应 DTO。
func toUserInfoResponse(u *domain.UserInfo) response.UserInfoResponse {
	if u == nil {
		return response.UserInfoResponse{}
	}
	return response.UserInfoResponse{
		ID:       u.ID,
		Email:    u.Email,
		Username: u.Username,
		Status:   u.Status,
	}
}
