package model

// AdultPerformerFollow stores one user's followed adult performer.
// SourceID is provider-owned and is used to reconstruct the provider URL,
// so callers cannot persist arbitrary external request targets.
type AdultPerformerFollow struct {
	Base
	UserID     string `gorm:"index;size:36;not null;uniqueIndex:uniq_user_adult_performer" json:"user_id"`
	Name       string `gorm:"size:255;not null" json:"name"`
	NameKey    string `gorm:"size:255;not null" json:"name_key"`
	Source     string `gorm:"size:32;not null;uniqueIndex:uniq_user_adult_performer" json:"source"`
	SourceID   string `gorm:"size:128;not null;uniqueIndex:uniq_user_adult_performer" json:"source_id"`
	ImageURL   string `gorm:"type:text" json:"image_url,omitempty"`
	ProfileURL string `gorm:"type:text" json:"profile_url,omitempty"`
}
