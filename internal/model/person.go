package model

import "time"

// Person stores reusable profiles and provider originals. PersonCredit carries
// work membership while Media.Actors preserves the legacy search projection.
type Person struct {
	Base
	Name               string     `gorm:"size:255;not null" json:"name"`
	NameKey            string     `gorm:"size:255;not null;uniqueIndex" json:"name_key"`
	ImageURL           string     `gorm:"type:text" json:"image_url,omitempty"`
	ProfileURL         string     `gorm:"type:text" json:"profile_url,omitempty"`
	Source             string     `gorm:"size:32;index:idx_person_provider" json:"source,omitempty"`
	SourceID           string     `gorm:"size:128;index:idx_person_provider" json:"source_id,omitempty"`
	OriginalName       string     `gorm:"size:255" json:"original_name,omitempty"`
	TranslatedName     string     `gorm:"size:255" json:"translated_name,omitempty"`
	Overview           string     `gorm:"type:text" json:"overview,omitempty"`
	OriginalOverview   string     `gorm:"type:text" json:"original_overview,omitempty"`
	Birthday           string     `gorm:"size:16" json:"birthday,omitempty"`
	Deathday           string     `gorm:"size:16" json:"deathday,omitempty"`
	Birthplace         string     `gorm:"size:255" json:"birthplace,omitempty"`
	Department         string     `gorm:"size:128" json:"department,omitempty"`
	Aliases            string     `gorm:"type:text" json:"aliases,omitempty"`
	DetailsRefreshedAt *time.Time `json:"details_refreshed_at,omitempty"`
}

// PersonCredit retains the provider role independently of the translated display
// role. Media.Actors remains the backwards-compatible name/search projection.
type PersonCredit struct {
	Base
	CreditKey    string `gorm:"size:64;not null;uniqueIndex" json:"-"`
	MediaID      string `gorm:"size:36;not null;index" json:"media_id"`
	PersonID     string `gorm:"size:36;not null;index" json:"person_id"`
	Type         string `gorm:"size:32;not null" json:"type"`
	OriginalRole string `gorm:"type:text" json:"original_role,omitempty"`
	Role         string `gorm:"type:text" json:"role,omitempty"`
	SortOrder    int    `json:"sort_order"`
	Person       Person `gorm:"foreignKey:PersonID" json:"person"`
}

// PersonTranslation caches a prompt-versioned translation in its work context.
// A hash key avoids indexing arbitrarily long provider text on PostgreSQL.
type PersonTranslation struct {
	Base
	CacheKey string `gorm:"size:64;not null;uniqueIndex" json:"-"`
	Text     string `gorm:"type:text" json:"text"`
}
