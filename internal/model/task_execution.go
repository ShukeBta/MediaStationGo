package model

import "time"

// TaskExecution keeps the latest durable snapshot of a background task.
type TaskExecution struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	Kind      string    `gorm:"size:128;index" json:"kind"`
	Status    string    `gorm:"size:16;index" json:"status"`
	StartedAt time.Time `gorm:"index" json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Revision  uint64    `json:"-"`
	Snapshot  string    `gorm:"type:text" json:"-"`
}

type TaskLogEntry struct {
	ID       string    `gorm:"primaryKey;size:36" json:"id"`
	TaskID   string    `gorm:"size:36;index" json:"task_id"`
	Kind     string    `gorm:"size:128;index" json:"kind"`
	LoggedAt time.Time `gorm:"index" json:"logged_at"`
	Level    string    `gorm:"size:16" json:"level"`
	Message  string    `gorm:"type:text" json:"message"`
}
