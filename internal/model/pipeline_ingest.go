package model

import "time"

type PipelineIngestJobRecord struct {
	ID             string    `gorm:"primaryKey;type:varchar(36)"`
	IdempotencyKey string    `gorm:"uniqueIndex;size:255;not null"`
	Status         string    `gorm:"index;size:16;not null"`
	Stage          string    `gorm:"size:64"`
	Message        string    `gorm:"type:text"`
	Error          string    `gorm:"type:text"`
	RequestJSON    string    `gorm:"type:text;not null"`
	ResultJSON     string    `gorm:"type:text;not null"`
	StartedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"index;not null"`
	FinishedAt     *time.Time
}

func (PipelineIngestJobRecord) TableName() string {
	return "pipeline_ingest_jobs"
}
