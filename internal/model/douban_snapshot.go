package model

import "time"

// DoubanSnapshot keeps complete provider evidence separately from editable metadata.
type DoubanSnapshot struct {
	MediaID   string    `gorm:"primaryKey;type:varchar(36)" json:"media_id"`
	DoubanID  string    `gorm:"size:32;index" json:"douban_id"`
	Payload   string    `gorm:"type:text" json:"-"`
	Degraded  bool      `json:"degraded"`
	FetchedAt time.Time `json:"fetched_at"`
}
