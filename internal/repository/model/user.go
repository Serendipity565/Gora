// Package model 定义 GORM 数据库模型。
package model

import (
	"time"

	"gorm.io/gorm"
)

// UserStatus 表示用户账户状态。
type UserStatus uint8

const (
	UserStatusActive   UserStatus = 1 // 正常
	UserStatusDisabled UserStatus = 2 // 禁用
)

// User 是 GORM 用户表模型。
type User struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"`

	Email    string     `gorm:"uniqueIndex;size:100;not null"` // 邮箱唯一，登录凭证
	Password string     `gorm:"size:255;not null" json:"-"`    // 加密后的密码，禁止序列化输出
	Username string     `gorm:"size:50;not null"`              // 昵称（不用于登录）
	Status   UserStatus `gorm:"default:1"`                     // 1:正常 2:禁用

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"` // 软删除字段
}

func (User) TableName() string {
	return "user"
}
