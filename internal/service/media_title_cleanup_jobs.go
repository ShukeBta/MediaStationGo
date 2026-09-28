package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	MediaTitleCleanupJobQueued    = "queued"
	MediaTitleCleanupJobRunning   = "running"
	MediaTitleCleanupJobCompleted = "completed"
	MediaTitleCleanupJobFailed    = "failed"
)

var ErrMediaTitleCleanupJobNotFound = errors.New("AI 标题清洗任务不存在")

type MediaTitleCleanupJob struct {
	ID              string                    `json:"id"`
	LibraryID       string                    `json:"library_id"`
	Status          string                    `json:"status"`
	Stage           string                    `json:"stage"`
	Message         string                    `json:"message"`
	Progress        int                       `json:"progress"`
	CompletedGroups int                       `json:"completed_groups"`
	TotalGroups     int                       `json:"total_groups"`
	Preview         *MediaTitleCleanupPreview `json:"preview,omitempty"`
	Error           string                    `json:"error,omitempty"`
	StartedAt       time.Time                 `json:"started_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
	FinishedAt      *time.Time                `json:"finished_at,omitempty"`
}

func (s *MediaService) StartTitleCleanupJob(ctx context.Context, libraryID string, groupLimit int) (*MediaTitleCleanupJob, error) {
	libraryID = strings.TrimSpace(libraryID)
	if _, err := s.titleCleanupLibrary(ctx, libraryID); err != nil {
		return nil, err
	}
	if s.ai == nil || !s.ai.EnabledFor(ctx) {
		return nil, ErrAITitleCleanupUnavailable
	}
	if groupLimit <= 0 {
		groupLimit = 5
	}

	now := time.Now()
	s.titleCleanupMu.Lock()
	if s.titleCleanupJobs == nil {
		s.titleCleanupJobs = make(map[string]*MediaTitleCleanupJob)
	}
	s.pruneTitleCleanupJobsLocked(now)
	for _, existing := range s.titleCleanupJobs {
		if existing.LibraryID == libraryID && (existing.Status == MediaTitleCleanupJobQueued || existing.Status == MediaTitleCleanupJobRunning) {
			job := cloneMediaTitleCleanupJob(existing)
			s.titleCleanupMu.Unlock()
			return job, nil
		}
	}
	job := &MediaTitleCleanupJob{
		ID: uuid.NewString(), LibraryID: strings.TrimSpace(libraryID),
		Status: MediaTitleCleanupJobQueued, Stage: "queued", Message: "清洗任务已排队",
		Progress: 1, StartedAt: now, UpdatedAt: now,
	}
	s.titleCleanupJobs[job.ID] = job
	result := cloneMediaTitleCleanupJob(job)
	s.titleCleanupMu.Unlock()

	go s.runTitleCleanupJob(ctx, job.ID, groupLimit)
	return result, nil
}

func (s *MediaService) GetTitleCleanupJob(libraryID, jobID string) (*MediaTitleCleanupJob, error) {
	s.titleCleanupMu.Lock()
	defer s.titleCleanupMu.Unlock()
	job := s.titleCleanupJobs[strings.TrimSpace(jobID)]
	if job == nil || job.LibraryID != strings.TrimSpace(libraryID) {
		return nil, ErrMediaTitleCleanupJobNotFound
	}
	return cloneMediaTitleCleanupJob(job), nil
}

func (s *MediaService) runTitleCleanupJob(parent context.Context, jobID string, groupLimit int) {
	ctx, cancel := context.WithTimeout(parent, titleCleanupJobTimeout(groupLimit))
	defer cancel()

	job, err := s.getTitleCleanupJobByID(jobID)
	if err != nil {
		return
	}
	var task *TaskHandle
	if s.tasks != nil {
		task = s.tasks.Start(TaskKindTitleCleanup, "AI 清洗媒体标题", TaskUpdate{
			Stage: "preparing", SourcePath: job.LibraryID, Message: "正在准备目录和文件信息",
			Metrics: map[string]int64{"progress": 5},
		})
	}
	s.updateTitleCleanupJob(jobID, func(current *MediaTitleCleanupJob) {
		current.Status = MediaTitleCleanupJobRunning
		current.Stage = "preparing"
		current.Message = "正在准备目录和文件信息"
		current.Progress = 5
	})

	preview, runErr := s.previewTitleCleanup(ctx, job.LibraryID, groupLimit, func(progress mediaTitleCleanupProgress) {
		percent := 5
		switch progress.Stage {
		case mediaTitleCleanupStageCleaning:
			percent = 10
			if progress.TotalGroups > 0 {
				percent = 10 + progress.CompletedGroups*80/progress.TotalGroups
			}
		case "validating":
			percent = 95
		}
		s.updateTitleCleanupJob(jobID, func(current *MediaTitleCleanupJob) {
			current.Status = MediaTitleCleanupJobRunning
			current.Stage = progress.Stage
			current.Message = progress.Message
			current.Progress = percent
			current.CompletedGroups = progress.CompletedGroups
			current.TotalGroups = progress.TotalGroups
		})
		if task != nil {
			task.Update(TaskUpdate{
				Stage: progress.Stage, Message: progress.Message,
				Metrics: map[string]int64{
					"progress": int64(percent), "completed_groups": int64(progress.CompletedGroups), "total_groups": int64(progress.TotalGroups),
				},
			})
		}
	})

	if runErr != nil {
		s.finishTitleCleanupJob(jobID, nil, runErr)
		if task != nil {
			task.Finish(runErr, TaskUpdate{Stage: "failed", Message: "AI 标题清洗失败"})
		}
		return
	}
	s.finishTitleCleanupJob(jobID, preview, nil)
	if task != nil {
		task.Finish(nil, TaskUpdate{
			Stage: "completed", Message: fmt.Sprintf("已生成 %d 条清洗建议", len(preview.Suggestions)),
			Metrics: map[string]int64{"progress": 100, "suggestions": int64(len(preview.Suggestions))},
		})
	}
}

func titleCleanupJobTimeout(groupLimit int) time.Duration {
	if groupLimit <= 0 {
		groupLimit = 5
	}
	if groupLimit > 30 {
		groupLimit = 30
	}
	waves := (groupLimit + 2) / 3
	timeout := time.Duration(waves)*90*time.Second + 30*time.Second
	if timeout < 2*time.Minute {
		return 2 * time.Minute
	}
	return timeout
}

func (s *MediaService) getTitleCleanupJobByID(jobID string) (*MediaTitleCleanupJob, error) {
	s.titleCleanupMu.Lock()
	defer s.titleCleanupMu.Unlock()
	job := s.titleCleanupJobs[strings.TrimSpace(jobID)]
	if job == nil {
		return nil, ErrMediaTitleCleanupJobNotFound
	}
	return cloneMediaTitleCleanupJob(job), nil
}

func (s *MediaService) updateTitleCleanupJob(jobID string, update func(*MediaTitleCleanupJob)) {
	s.titleCleanupMu.Lock()
	defer s.titleCleanupMu.Unlock()
	if job := s.titleCleanupJobs[jobID]; job != nil {
		update(job)
		job.UpdatedAt = time.Now()
	}
}

func (s *MediaService) finishTitleCleanupJob(jobID string, preview *MediaTitleCleanupPreview, err error) {
	now := time.Now()
	s.updateTitleCleanupJob(jobID, func(job *MediaTitleCleanupJob) {
		job.FinishedAt = &now
		if err != nil {
			job.Status = MediaTitleCleanupJobFailed
			job.Stage = "failed"
			job.Message = "AI 标题清洗失败"
			job.Error = err.Error()
			return
		}
		job.Status = MediaTitleCleanupJobCompleted
		job.Stage = "completed"
		job.Message = "清洗建议已生成"
		job.Progress = 100
		job.Preview = preview
	})
}

func (s *MediaService) pruneTitleCleanupJobsLocked(now time.Time) {
	for id, job := range s.titleCleanupJobs {
		if job.FinishedAt != nil && now.Sub(*job.FinishedAt) > time.Hour {
			delete(s.titleCleanupJobs, id)
		}
	}
	if len(s.titleCleanupJobs) <= 50 {
		return
	}
	finished := make([]*MediaTitleCleanupJob, 0, len(s.titleCleanupJobs))
	for _, job := range s.titleCleanupJobs {
		if job.FinishedAt != nil {
			finished = append(finished, job)
		}
	}
	sort.Slice(finished, func(i, j int) bool { return finished[i].UpdatedAt.Before(finished[j].UpdatedAt) })
	for _, job := range finished {
		if len(s.titleCleanupJobs) <= 50 {
			break
		}
		delete(s.titleCleanupJobs, job.ID)
	}
}

func cloneMediaTitleCleanupJob(job *MediaTitleCleanupJob) *MediaTitleCleanupJob {
	if job == nil {
		return nil
	}
	clone := *job
	if job.FinishedAt != nil {
		finished := *job.FinishedAt
		clone.FinishedAt = &finished
	}
	if job.Preview != nil {
		preview := *job.Preview
		preview.Groups = append([]MediaTitleCleanupGroup(nil), job.Preview.Groups...)
		preview.Suggestions = append([]MediaTitleCleanupSuggestion(nil), job.Preview.Suggestions...)
		clone.Preview = &preview
	}
	return &clone
}
