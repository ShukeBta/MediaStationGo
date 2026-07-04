package model

import "time"

// User 是本地账户。第一个注册的管理员（或种子管理员）获得 "admin" 角色；
// 其他所有用户默认为 "user"。
type User struct {
	Base
	Username           string     `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash       string     `gorm:"size:128;not null" json:"-"`
	Role               string     `gorm:"size:16;not null;default:user" json:"role"`
	Tier               string     `gorm:"size:16;default:free" json:"tier"` // free / plus
	Nickname           string     `gorm:"size:128" json:"nickname,omitempty"`
	Email              string     `gorm:"size:128" json:"email,omitempty"`
	AvatarURL          string     `gorm:"size:255" json:"avatar_url,omitempty"`
	HideAdult          bool       `gorm:"default:true" json:"hide_adult"`
	ForcePasswordReset bool       `gorm:"default:false" json:"force_password_reset"`
	IsActive           bool       `gorm:"default:true" json:"is_active"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	// ExpiredAt is the account expiry time. Nil means the account never
	// expires. When set and in the past, the account is treated as expired
	// (login blocked) until an admin or a redemption code renews it.
	ExpiredAt *time.Time `json:"expired_at,omitempty"`
	// ShareWarnings counts anti-account-sharing warnings, mainly device
	// fingerprint mismatches. Once it exceeds the configured threshold a
	// re-offence disables the account until an admin re-enables it.
	ShareWarnings       int        `gorm:"default:0" json:"share_warnings"`
	LastShareWarnAt     *time.Time `json:"last_share_warn_at,omitempty"`
	IsDefaultAdmin      bool       `gorm:"-" json:"is_default_admin,omitempty"`
	IsProtected         bool       `gorm:"-" json:"is_protected,omitempty"`
	RealtimeOnline      bool       `gorm:"-" json:"realtime_online,omitempty"`
	RealtimeDeviceCount int        `gorm:"-" json:"realtime_device_count,omitempty"`
}
