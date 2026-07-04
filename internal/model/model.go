// Package model 定义 GORM 数据模型和自动迁移使用的注册表。
// 每个子系统在 MediaStationGo 中拥有一个表切片；AllModels 返回联合以供 db.AutoMigrate 使用。
package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base 嵌入每个域实体共享的字段:
//
//   - ID:         UUID v4 字符串主键
//   - CreatedAt / UpdatedAt: 由 GORM 管理
//   - DeletedAt:  软删除（查询自动过滤）
type Base struct {
	ID        string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate 如果调用者未提供则生成 UUID。
func (b *Base) BeforeCreate(_ *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	return nil
}

// AllModels returns the slice consumed by gorm.AutoMigrate.
func AllModels() []interface{} {
	return []interface{}{
		&User{},
		&Library{},
		&LibraryRoot{},
		&Series{},
		&Media{},
		&PlaybackHistory{},
		&Favorite{},
		&Playlist{},
		&PlaylistItem{},
		&DownloadTask{},
		&Subscription{},
		&Setting{},
		&Site{},
		&AccessLog{},
		&APIConfig{},
		&UserPermission{},
		&RefreshToken{},
		&ApiConfig{},
		&DownloadClient{},
		&NotifyChannel{},
		&TelegramBinding{},
		&STRMRecord{},
		&PlayProfile{},
		&StorageConfig{},
		&AssistantSession{},
		&AssistantMessage{},
		&RegistrationCode{},
		&SignIn{},
		&UserDevice{},
	}
}
