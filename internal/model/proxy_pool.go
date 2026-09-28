package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

// ProxyPoolEntry stores one ordered encrypted proxy URL.
type ProxyPoolEntry struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	URL       string    `gorm:"type:text;not null" json:"-"`
	Position  int       `gorm:"not null;index" json:"position"`
}

func (p *ProxyPoolEntry) BeforeCreate(_ *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	return nil
}
