package model

// StorageConfig holds the connection settings for one external storage
// backend (Alist / S3 / WebDAV). Type column makes the row poly-typed
// — Config is a JSON blob whose shape is determined by Type.
//
//	alist  → {server, token}
//	s3     → {endpoint, region, bucket, access_key, secret_key, force_path_style}
//	webdav → {url, username, password}
type StorageConfig struct {
	Base
	Type      string `gorm:"uniqueIndex;size:16;not null" json:"type"`
	Config    string `gorm:"type:text;not null" json:"-"` // ciphertext
	Enabled   bool   `gorm:"default:true" json:"enabled"`
	LastError string `gorm:"size:512" json:"last_error,omitempty"`
}

// AssistantSession groups a multi-turn chat with the AI assistant.
type AssistantSession struct {
	Base
	UserID string `gorm:"index;size:36;not null" json:"user_id"`
	Title  string `gorm:"size:255" json:"title,omitempty"`
}

// AssistantMessage is one entry in an AssistantSession transcript.
//
// Role is "user" | "assistant" | "system".  The optional OperationID
// links a message to an action the assistant proposed (so the UI can
// offer Undo).
type AssistantMessage struct {
	Base
	SessionID   string `gorm:"index;size:36;not null" json:"session_id"`
	Role        string `gorm:"size:16;not null" json:"role"`
	Content     string `gorm:"type:text;not null" json:"content"`
	OperationID string `gorm:"size:36" json:"operation_id,omitempty"`
}
