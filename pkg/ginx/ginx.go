package ginx

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/Serendipity565/gora/api/response"
	"github.com/Serendipity565/gora/pkg/errorx"
	"github.com/Serendipity565/gora/pkg/ijwt"
	"github.com/gin-gonic/gin"
)

const CTX = "claims"

func WrapSSEClaimsAndReq[Req any](fn func(*gin.Context, Req, ijwt.UserClaims) error) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if len(ctx.Errors) > 0 {
			return
		}

		var req Req
		if err := ctx.ShouldBind(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, response.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("请求参数错误: %v", err.Error()),
				Data:    nil,
			})
			return
		}

		claims, err := GetClaims(ctx)
		if err != nil {
			ctx.JSON(http.StatusUnauthorized, response.Response{
				Code:    http.StatusUnauthorized,
				Message: "无效或过期的身份令牌",
				Data:    nil,
			})
			return
		}

		// 设置 SSE header
		ctx.Writer.Header().Set("Content-Type", "text/event-stream")
		ctx.Writer.Header().Set("Cache-Control", "no-cache")
		ctx.Writer.Header().Set("Connection", "keep-alive")
		ctx.Writer.Header().Set("X-Accel-Buffering", "no") // nginx 下很关键
		ctx.Status(http.StatusOK)

		// 不再 JSON 返回
		if err := fn(ctx, req, claims); err != nil {
			ctx.Error(err)
		}
	}
}

func WrapSSEReq[Req any](fn func(*gin.Context, Req) error) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if len(ctx.Errors) > 0 {
			return
		}

		var req Req
		if err := ctx.ShouldBind(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, response.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("请求参数错误: %v", err.Error()),
				Data:    nil,
			})
			return
		}

		// 设置 SSE header
		ctx.Writer.Header().Set("Content-Type", "text/event-stream")
		ctx.Writer.Header().Set("Cache-Control", "no-cache")
		ctx.Writer.Header().Set("Connection", "keep-alive")
		ctx.Writer.Header().Set("X-Accel-Buffering", "no") // nginx 下很关键
		ctx.Status(http.StatusOK)

		// 不再 JSON 返回
		if err := fn(ctx, req); err != nil {
			ctx.Error(err)
		}
	}
}

func WrapClaimsAndReq[Req any](fn func(*gin.Context, Req, ijwt.UserClaims) (response.Response, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		//检查前置中间件是否存在错误,如果存在应当直接返回
		if len(ctx.Errors) > 0 {
			return
		}

		var req Req
		if err := ctx.Bind(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, response.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("请求参数错误: %v", err.Error()),
				Data:    nil,
			})
			return
		}

		claims, err := GetClaims(ctx) // 这一步只是简单的解析 token
		if err != nil {
			ctx.JSON(http.StatusUnauthorized, response.Response{
				Code:    http.StatusUnauthorized,
				Message: "无效或过期的身份令牌",
				Data:    nil,
			})
			return
		}

		res, err := fn(ctx, req, claims)
		if err != nil {
			ctx.Error(err) // 记录错误到ctx.Errors,以便后续中间件处理日志等
			customError := errorx.ToCustomError(err)

			ctx.JSON(customError.HttpCode, response.Response{
				Code:    customError.Code,
				Message: customError.Msg,
				Data:    nil, // 不返回数据
			})
			return
		}
		// 默认成功时的 HTTP 状态码
		ctx.JSON(ctx.Writer.Status(), res)
	}
}

// WrapReq .
func WrapReq[Req any](fn func(*gin.Context, Req) (response.Response, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if len(ctx.Errors) > 0 {
			return
		}

		var req Req
		if err := ctx.Bind(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, response.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("请求参数错误: %v", err.Error()),
				Data:    nil,
			})
			return
		}

		res, err := fn(ctx, req)
		if err != nil {
			ctx.Error(err) // 记录错误到ctx.Errors,以便后续中间件处理日志等
			customError := errorx.ToCustomError(err)

			ctx.JSON(customError.HttpCode, response.Response{
				Code:    customError.Code,
				Message: customError.Msg,
				Data:    nil, // 不返回数据
			})
			return
		}
		// 默认成功时的 HTTP 状态码
		ctx.JSON(ctx.Writer.Status(), res)
	}
}

// Wrap .
func Wrap(fn func(*gin.Context) (response.Response, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if len(ctx.Errors) > 0 {
			return
		}

		res, err := fn(ctx)
		if err != nil {
			ctx.Error(err) // 记录错误到ctx.Errors,以便后续中间件处理日志等
			customError := errorx.ToCustomError(err)

			ctx.JSON(customError.HttpCode, response.Response{
				Code:    customError.Code,
				Message: customError.Msg,
				Data:    nil, // 不返回数据
			})
			return
		}
		// 默认成功时的 HTTP 状态码
		ctx.JSON(ctx.Writer.Status(), res)
	}
}

// WrapClaims .
func WrapClaims(fn func(*gin.Context, ijwt.UserClaims) (response.Response, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if len(ctx.Errors) > 0 {
			return
		}

		claims, err := GetClaims(ctx) // 这一步只是简单的解析 token
		if err != nil {
			ctx.JSON(http.StatusUnauthorized, response.Response{
				Code:    http.StatusUnauthorized,
				Message: "无效或过期的身份令牌",
				Data:    nil,
			})
			return
		}

		res, err := fn(ctx, claims)
		if err != nil {
			ctx.Error(err) // 记录错误到ctx.Errors,以便后续中间件处理日志等
			customError := errorx.ToCustomError(err)

			ctx.JSON(customError.HttpCode, response.Response{
				Code:    customError.Code,
				Message: customError.Msg,
				Data:    nil, // 不返回数据
			})
			return
		}
		// 默认成功时的 HTTP 状态码
		ctx.JSON(ctx.Writer.Status(), res)
	}
}

func SetClaims(ctx *gin.Context, claims ijwt.UserClaims) {
	ctx.Set(CTX, claims)
}

func GetClaims(ctx *gin.Context) (ijwt.UserClaims, error) {
	val, ok := ctx.Get(CTX)
	if !ok {
		ctx.Error(errors.New("claims 不存在"))
		return ijwt.UserClaims{}, errors.New("claims 不存在")
	}
	claims, ok := val.(ijwt.UserClaims)
	if !ok {
		ctx.Error(errors.New("claims 断言失败"))
		return ijwt.UserClaims{}, errors.New("claims 断言失败")
	}
	return claims, nil
}
