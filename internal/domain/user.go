// Package domain 定义核心业务领域对象，与数据库模型解耦。
package domain

// User 是用户领域实体，包含完整的用户信息。
type User struct {
	ID       uint64 `json:"id"`
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
