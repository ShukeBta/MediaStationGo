package model

// Person stores reusable person artwork independently from media rows.
// Media membership remains in Media.Actors so existing search and filtering
// behavior is unchanged.
type Person struct {
	Base
	Name       string `gorm:"size:255;not null" json:"name"`
	NameKey    string `gorm:"size:255;not null;uniqueIndex" json:"name_key"`
	ImageURL   string `gorm:"type:text" json:"image_url,omitempty"`
	ProfileURL string `gorm:"type:text" json:"profile_url,omitempty"`
	Source     string `gorm:"size:32" json:"source,omitempty"`
	SourceID   string `gorm:"size:128" json:"source_id,omitempty"`
}
