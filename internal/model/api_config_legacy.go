package model

// APIConfig stores third-party data-source configuration. The api_key
// column is encrypted with AES-GCM (see internal/service/crypto.go) so an
// SQLite leak does not expose third-party credentials.
//
// Provider values mirror the original Python project:
//
//	tmdb        — themoviedb.org
//	bangumi     — bgm.tv
//	thetvdb     — thetvdb.com
//	fanart      — fanart.tv
//	douban      — douban.com (cookie)
//	openai      — OpenAI / DeepSeek / Qwen / Ollama (compatible)
type APIConfig struct {
	Base
	Provider    string `gorm:"uniqueIndex;size:32;not null" json:"provider"`
	APIKey      string `gorm:"type:text" json:"-"` // ciphertext (never serialised)
	BaseURL     string `gorm:"size:512" json:"base_url,omitempty"`
	Extra       string `gorm:"type:text" json:"extra,omitempty"` // free-form JSON
	Enabled     bool   `gorm:"default:true" json:"enabled"`
	Description string `gorm:"size:255" json:"description,omitempty"`
}
