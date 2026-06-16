package controller

import (
	"errors"
	"strings"

	"github.com/Serendipity565/gora/api/request"
	"github.com/Serendipity565/gora/api/response"
	"github.com/Serendipity565/gora/internal/errs"
	"github.com/Serendipity565/gora/internal/server"
	"github.com/Serendipity565/gora/pkg/ijwt"
	"github.com/gin-gonic/gin"
)

// ModelHandler 暴露 /api/models 路由组。
type ModelHandler interface {
	List(c *gin.Context) (response.Response, error)
	Current(c *gin.Context, claims ijwt.UserClaims) (response.Response, error)
	Select(c *gin.Context, req request.ModelSelect, claims ijwt.UserClaims) (response.Response, error)
}

type Model struct {
	s server.ModelService
}

func NewModel(s server.ModelService) ModelHandler {
	return &Model{s: s}
}

// List 列出当前配置中所有可选模型。
//
//	@Summary		列出可选模型
//	@Tags			Model
//	@ID				listModels
//	@Produce		json
//	@Success		200	{object}	response.Response
//	@Failure		501	{object}	response.Response	"未配置 ModelSelector"
//	@Router			/api/models [get]
func (h *Model) List(c *gin.Context) (response.Response, error) {
	models, err := h.s.List()
	if err != nil {
		return response.Response{}, mapModelError(err)
	}
	return response.Response{
		Code:    0,
		Message: "success",
		Data:    gin.H{"models": models},
	}, nil
}

// Current 返回某 session 当前使用的模型。
//
//	@Summary		查询当前模型
//	@Description	session_id 通过 query 传入；为空表示默认会话
//	@Tags			Model
//	@ID				getCurrentModel
//	@Produce		json
//	@Param			session_id	query		string	false	"会话 ID"
//	@Success		200			{object}	response.Response
//	@Failure		501			{object}	response.Response	"未配置 ModelSelector"
//	@Router			/api/models/current [get]
func (h *Model) Current(c *gin.Context, claims ijwt.UserClaims) (response.Response, error) {
	userID, err := parseUserID(claims)
	if err != nil {
		return response.Response{}, errs.ErrUserNotFound(err)
	}

	sessionID := strings.TrimSpace(c.Query("session_id"))
	info, explicit, err := h.s.Current(c.Request.Context(), userID, sessionID)
	if err != nil {
		return response.Response{}, mapModelError(err)
	}
	return response.Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"model":    info,
			"explicit": explicit,
		},
	}, nil
}

// Select 设置某 session 使用的模型。
//
//	@Summary		选择模型
//	@Tags			Model
//	@ID				selectModel
//	@Accept			json
//	@Produce		json
//	@Param			request	body		request.ModelSelect	true	"模型选择请求"
//	@Success		200		{object}	response.Response
//	@Failure		400		{object}	response.Response
//	@Failure		501		{object}	response.Response	"未配置 ModelSelector"
//	@Router			/api/models/select [post]
func (h *Model) Select(c *gin.Context, req request.ModelSelect, claims ijwt.UserClaims) (response.Response, error) {
	userID, err := parseUserID(claims)
	if err != nil {
		return response.Response{}, errs.ErrUserNotFound(err)
	}

	info, err := h.s.Select(c.Request.Context(), userID, strings.TrimSpace(req.SessionID), strings.TrimSpace(req.Selector))
	if err != nil {
		return response.Response{}, mapModelError(err)
	}
	return response.Response{
		Code:    0,
		Message: "success",
		Data:    gin.H{"model": info},
	}, nil
}

// mapModelError 把 server 层错误映射成统一的对外 errs。
func mapModelError(err error) error {
	var notConfigured server.ErrModelSelectorNotConfigured
	if errors.As(err, &notConfigured) {
		return errs.ErrModelSelectorNotConfigured(err)
	}
	return errs.ErrModelInvalid(err)
}
