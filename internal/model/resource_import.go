package model

import "time"

// ResourceSearchSession stores trusted pipeline search results server-side.
// Browsers can only select a candidate by session ID and array index.
type ResourceSearchSession struct {
	Base
	UserID             string    `gorm:"index;size:36;not null" json:"user_id"`
	LibraryID          string    `gorm:"index;size:36;not null" json:"library_id"`
	LibraryRootID      string    `gorm:"index;size:36;not null" json:"library_root_id"`
	Query              string    `gorm:"size:512;not null" json:"query"`
	Source             string    `gorm:"size:32;not null" json:"source"`
	SubscriptionFollow bool      `gorm:"index;not null;default:false" json:"-"`
	ResultsJSON        string    `gorm:"type:text;not null" json:"-"`
	ExpiresAt          time.Time `gorm:"index;not null" json:"expires_at"`
}

// ResourceImportJob is the durable MSG parent job for one selected candidate.
// CandidateJSON and ResultJSON are never serialized directly to API clients.
type ResourceImportJob struct {
	Base
	UserID                  string     `gorm:"index;size:36;not null" json:"user_id"`
	SubscriptionID          string     `gorm:"index;size:36" json:"subscription_id,omitempty"`
	SubscriptionFollow      bool       `gorm:"index;not null;default:false" json:"subscription_follow,omitempty"`
	WorkKey                 string     `gorm:"index;size:300" json:"work_key,omitempty"`
	SeasonNumber            int        `gorm:"index;not null;default:0" json:"season_number,omitempty"`
	TitleClass              string     `gorm:"size:32" json:"title_class,omitempty"`
	TargetOpenListPath      string     `gorm:"size:2048" json:"target_openlist_path,omitempty"`
	ExistingEpisodesJSON    string     `gorm:"type:text" json:"-"`
	ReservedEpisodesJSON    string     `gorm:"type:text" json:"-"`
	Outcome                 string     `gorm:"index;size:32" json:"outcome,omitempty"`
	ActiveReservationKey    *string    `gorm:"uniqueIndex;size:400" json:"-"`
	LibraryID               string     `gorm:"index;size:36;not null" json:"library_id"`
	LibraryRootID           string     `gorm:"index;size:36;not null" json:"library_root_id"`
	SearchSessionID         string     `gorm:"index;size:36;not null" json:"search_session_id"`
	PipelineSearchSessionID string     `gorm:"size:128" json:"-"`
	PipelineCandidateID     string     `gorm:"size:128" json:"-"`
	CandidateIndex          int        `gorm:"not null" json:"candidate_index"`
	CandidateJSON           string     `gorm:"type:text;not null" json:"-"`
	CandidateTitle          string     `gorm:"size:512" json:"candidate_title"`
	CandidateSource         string     `gorm:"size:255" json:"candidate_source"`
	CandidateSize           int64      `json:"candidate_size"`
	Attempt                 int        `gorm:"not null;default:1" json:"attempt"`
	RetryOfJobID            string     `gorm:"index;size:36" json:"retry_of_job_id,omitempty"`
	IdempotencyKey          string     `gorm:"uniqueIndex;size:255;not null" json:"-"`
	ForceDuplicate          bool       `gorm:"not null;default:false" json:"force_duplicate"`
	UpgradeMediaID          string     `gorm:"index;size:36" json:"upgrade_media_id,omitempty"`
	UpgradeScope            string     `gorm:"size:16;not null;default:''" json:"upgrade_scope,omitempty"`
	KeepOldVersion          bool       `gorm:"not null;default:false" json:"keep_old_version"`
	Status                  string     `gorm:"index;size:32;not null" json:"status"`
	Stage                   string     `gorm:"index;size:32;not null" json:"stage"`
	Message                 string     `gorm:"type:text" json:"-"`
	PublicError             string     `gorm:"type:text" json:"-"`
	Error                   string     `gorm:"type:text" json:"-"`
	PipelineJobID           string     `gorm:"index;size:128" json:"pipeline_job_id,omitempty"`
	MediaID                 string     `gorm:"index;size:128" json:"media_id,omitempty"`
	MediaTitle              string     `gorm:"size:512" json:"media_title,omitempty"`
	ResultJSON              string     `gorm:"type:text" json:"-"`
	CancelRequested         bool       `gorm:"index;not null;default:false" json:"cancel_requested"`
	StartedAt               *time.Time `json:"started_at,omitempty"`
	FinishedAt              *time.Time `json:"finished_at,omitempty"`
}
