package response

// UserInfoResponse 用户信息（脱敏后的对外结构）。
type UserInfoResponse struct {
	ID       uint64 `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Status   int    `json:"status"`
}

// RegisterResponse 注册响应：返回用户信息（不包含 token）。
type RegisterResponse struct {
	User UserInfoResponse `json:"user"`
}

// LoginResponse 登录响应：返回 token 与用户信息。
type LoginResponse struct {
	Token string           `json:"token"`
	User  UserInfoResponse `json:"user"`
}

// UpdateProfileResponse 更新用户资料响应：返回更新后的用户信息。
type UpdateProfileResponse struct {
	User UserInfoResponse `json:"user"`
}
