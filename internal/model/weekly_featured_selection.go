package model

import "time"

// WeeklyFeaturedSelection records the work shown to one user for one ISO week.
// It keeps weekly recommendations stable across refreshes/restarts and provides
// the history required to back off recently featured works.
type WeeklyFeaturedSelection struct {
	Base
	UserID    string    `gorm:"size:36;not null;index;uniqueIndex:uniq_weekly_featured_user_week,priority:1" json:"user_id"`
	WeekKey   string    `gorm:"size:8;not null;uniqueIndex:uniq_weekly_featured_user_week,priority:2" json:"week_key"`
	WeekStart time.Time `gorm:"not null;index" json:"week_start"`
	WorkKey   string    `gorm:"size:128;not null;index" json:"work_key"`
	MediaID   string    `gorm:"size:128;not null" json:"media_id"`
}
