package domain

type User struct {
	ID       uint   `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Username string `json:"username"`
	Status   int    `json:"status"`
}

// UserInfo 对外暴露的用户信息（脱敏）
type UserInfo struct {
	ID       uint64 `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Status   int    `json:"status"`
}
