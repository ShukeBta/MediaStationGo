// Package model — Telegram Bot 相关数据模型：注册兑换码、签到记录、用户设备会话。
package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RegistrationCodeKind 区分兑换码用途。
const (
	// RegistrationCodeRegister 用于注册一个新账号（兑换后自动绑定 / 创建账号）。
	RegistrationCodeRegister = "register"
	// RegistrationCodeRenew 用于给已有账号续期（延长到期时间 DurationDays 天）。
	RegistrationCodeRenew = "renew"
)

// RegistrationCode 是兑换码。管理员生成后发给用户，用户通过 Bot 兑换：
//   - register：创建并绑定一个新账号；兑换时按 DurationDays 设置账号有效期。
//   - renew：给当前绑定账号延长 DurationDays 天有效期。
//
// MaxUses 控制最多可兑换次数，旧数据/零值按 1 次处理。UsedAt 表示达到最大
// 次数后的耗尽时间；ExpiresAt 是兑换码本身的有效期。
type RegistrationCode struct {
	Base
	Code         string     `gorm:"uniqueIndex;size:32;not null" json:"code"`
	Kind         string     `gorm:"size:16;not null;default:register" json:"kind"`
	DurationDays int        `gorm:"default:0" json:"duration_days"` // 账号有效期天数；0 表示永久
	CreatedByID  string     `gorm:"size:36" json:"created_by_id,omitempty"`
	UsedByUserID string     `gorm:"index;size:36" json:"used_by_user_id,omitempty"`
	MaxUses      int        `gorm:"default:1" json:"max_uses"`
	UsedCount    int        `gorm:"default:0" json:"used_count"`
	UsedAt       *time.Time `json:"used_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"` // 兑换码本身的有效期
}

// BeforeCreate 生成 UUID。
func (c *RegistrationCode) BeforeCreate(_ *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	return nil
}

// EffectiveMaxUses returns the configured max uses, treating legacy zero values
// as one-use codes.
func (c *RegistrationCode) EffectiveMaxUses() int {
	if c == nil || c.MaxUses <= 0 {
		return 1
	}
	return c.MaxUses
}

// IsUsed 报告兑换码是否已耗尽。
func (c *RegistrationCode) IsUsed() bool {
	return c != nil && (c.UsedAt != nil || c.UsedCount >= c.EffectiveMaxUses())
}

// IsExpired 报告兑换码自身是否过期（与账号有效期无关）。
func (c *RegistrationCode) IsExpired() bool {
	return c.ExpiresAt != nil && time.Now().After(*c.ExpiresAt)
}

// SignIn 记录单个用户的连续签到天数（不挂钩积分）。
type SignIn struct {
	Base
	UserID     string    `gorm:"uniqueIndex;size:36;not null" json:"user_id"`
	LastSignIn time.Time `json:"last_sign_in"`
	StreakDays int       `gorm:"default:0" json:"streak_days"` // 当前连续签到天数
	TotalDays  int       `gorm:"default:0" json:"total_days"`  // 累计签到天数
}

// BeforeCreate 生成 UUID。
func (s *SignIn) BeforeCreate(_ *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	return nil
}

// UserDevice 记录一个用户在某台终端设备上的登录渠道，用于设备管控：
//   - 登录终端数：按 Fingerprint 去重后的近期活跃终端数。
//   - 并发播放数：按 Fingerprint 去重后的播放终端数。
//   - 设备指纹：同一终端不同 App 共用 Fingerprint，Client 只作为渠道标签。
//   - 观看时长：结合 PlaybackHistory 统计随机窗口内的观看时长。
type UserDevice struct {
	Base
	UserID      string     `gorm:"index;size:36;not null;uniqueIndex:uniq_user_device" json:"user_id"`
	DeviceID    string     `gorm:"size:128;not null;uniqueIndex:uniq_user_device" json:"device_id"`
	DeviceName  string     `gorm:"size:128" json:"device_name,omitempty"`
	Client      string     `gorm:"size:128" json:"client,omitempty"`
	Fingerprint string     `gorm:"size:64" json:"fingerprint,omitempty"`
	LastIP      string     `gorm:"size:64" json:"last_ip,omitempty"`
	FirstSeenAt time.Time  `json:"first_seen_at"`
	LastSeenAt  time.Time  `gorm:"index" json:"last_seen_at"`
	LastPlayAt  *time.Time `gorm:"index" json:"last_play_at,omitempty"`
	Warnings    int        `gorm:"default:0" json:"warnings"`   // 指纹不匹配累计告警次数
	Kicked      bool       `gorm:"default:false" json:"kicked"` // 被一键踢下线（强制重新登录）
	Realtime    bool       `gorm:"-" json:"realtime,omitempty"`
	Online      bool       `gorm:"-" json:"online,omitempty"`
	Playing     bool       `gorm:"-" json:"playing,omitempty"`
}

// BeforeCreate 生成 UUID。
func (d *UserDevice) BeforeCreate(_ *gorm.DB) error {
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	return nil
}

// BotModels 返回 Bot 相关模型，供 AutoMigrate 使用。
func BotModels() []interface{} {
	return []interface{}{
		&RegistrationCode{},
		&SignIn{},
		&UserDevice{},
	}
}
