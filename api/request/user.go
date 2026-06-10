package request

// RegisterRequest 注册请求体。
type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=64"`
	Username string `json:"username" binding:"required,min=1,max=50"`
}

// LoginRequest 登录请求体。
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// UpdateProfileRequest 更新用户资料请求体。
// 用户身份从 JWT 中获取，此处不接收 ID；Username / Password 至少传一个，未填字段保持不变。
type UpdateProfileRequest struct {
	Username string `json:"username" binding:"omitempty,min=1,max=50"`
	Password string `json:"password" binding:"omitempty,min=6,max=64"`
}
