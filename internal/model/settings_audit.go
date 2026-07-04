package model

import "time"

// Setting 是单个键/值系统级偏好（供管理 UI 使用）。
type Setting struct {
	Key       string    `gorm:"primaryKey;size:128" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AccessLog 是结构化审计跟踪条目。存储在 SQLite 中供管理活动面板使用。
type AccessLog struct {
	Base
	UserID string `gorm:"index;size:36" json:"user_id"`
	Action string `gorm:"size:64;not null" json:"action"`
	Target string `gorm:"size:255" json:"target"`
	IP     string `gorm:"size:64" json:"ip"`
	Detail string `gorm:"type:text" json:"detail"`
}
