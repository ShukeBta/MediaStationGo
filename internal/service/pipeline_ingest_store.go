package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

func (s *PipelineIngestService) Recover(ctx context.Context) (int, error) {
	if s == nil || s.repos == nil || s.repos.DB == nil {
		return 0, errors.New("pipeline ingest service unavailable")
	}
	var records []model.PipelineIngestJobRecord
	if err := s.repos.DB.WithContext(ctx).
		Where("status IN ?", []string{PipelineIngestStatusAccepted, PipelineIngestStatusConverging, PipelineIngestStatusRunning}).
		Order("started_at ASC").
		Find(&records).Error; err != nil {
		return 0, err
	}
	for _, record := range records {
		job, err := pipelineIngestJobFromRecord(record)
		if err != nil {
			return 0, err
		}
		s.cacheJob(job)
		s.scheduleJob(job.ID)
	}
	recovered := len(records)
	if s.subtitle != nil {
		var completed []model.PipelineIngestJobRecord
		if err := s.repos.DB.WithContext(ctx).
			Where("status = ?", PipelineIngestStatusCompleted).
			Order("updated_at ASC").
			Find(&completed).Error; err != nil {
			return recovered, err
		}
		for _, record := range completed {
			job, err := pipelineIngestJobFromRecord(record)
			if err != nil {
				return recovered, err
			}
			status := strings.TrimSpace(job.Result.CloudSubtitleStatus)
			if status != "pending" && status != "running" {
				continue
			}
			s.cacheJob(job)
			if s.scheduleCloudSubtitleEnhancement(job.ID) {
				recovered++
			}
		}
	}
	return recovered, nil
}

func (s *PipelineIngestService) storeJob(ctx context.Context, job PipelineIngestJob) error {
	record, err := pipelineIngestRecordFromJob(job)
	if err != nil {
		return err
	}
	if err := s.repos.DB.WithContext(ctx).Create(&record).Error; err != nil {
		return err
	}
	s.cacheJob(job)
	return nil
}

func (s *PipelineIngestService) findJobByIdempotencyKey(ctx context.Context, key string) (PipelineIngestJob, bool, error) {
	var record model.PipelineIngestJobRecord
	err := s.repos.DB.WithContext(ctx).Where("idempotency_key = ?", strings.TrimSpace(key)).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PipelineIngestJob{}, false, nil
	}
	if err != nil {
		return PipelineIngestJob{}, false, err
	}
	job, err := pipelineIngestJobFromRecord(record)
	return job, err == nil, err
}

func (s *PipelineIngestService) loadJobByID(ctx context.Context, id string) (PipelineIngestJob, bool, error) {
	var record model.PipelineIngestJobRecord
	err := s.repos.DB.WithContext(ctx).Where("id = ?", strings.TrimSpace(id)).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PipelineIngestJob{}, false, nil
	}
	if err != nil {
		return PipelineIngestJob{}, false, err
	}
	job, err := pipelineIngestJobFromRecord(record)
	return job, err == nil, err
}

func (s *PipelineIngestService) updateJob(id, stage, message string, mutate func(*PipelineIngestJob)) error {
	job, err := s.Get(id)
	if err != nil {
		return err
	}
	if stage != "" {
		job.Stage = stage
	}
	if message != "" {
		job.Message = message
	}
	job.UpdatedAt = s.currentTime()
	if mutate != nil {
		mutate(&job)
	}
	if err := s.persistJob(context.Background(), job); err != nil {
		return err
	}
	s.cacheJob(job)
	return nil
}

func (s *PipelineIngestService) updateJobResult(id string, mutate func(*PipelineIngestResult)) error {
	return s.updateJob(id, "", "", func(job *PipelineIngestJob) {
		mutate(&job.Result)
	})
}

func (s *PipelineIngestService) completeJob(id string) error {
	now := s.currentTime()
	return s.updateJob(id, "completed", "completed", func(job *PipelineIngestJob) {
		job.Status = PipelineIngestStatusCompleted
		job.Error = ""
		job.FinishedAt = &now
	})
}

func (s *PipelineIngestService) failJob(id string, runErr error) error {
	now := s.currentTime()
	return s.updateJob(id, "failed", "failed", func(job *PipelineIngestJob) {
		job.Status = PipelineIngestStatusFailed
		job.Error = runErr.Error()
		job.FinishedAt = &now
	})
}

func (s *PipelineIngestService) markJobNeedsAttention(id, message string) error {
	now := s.currentTime()
	return s.updateJob(id, PipelineIngestStatusNeedsAttention, message, func(job *PipelineIngestJob) {
		job.Status = PipelineIngestStatusNeedsAttention
		job.Error = ""
		job.FinishedAt = &now
	})
}

func (s *PipelineIngestService) persistJob(ctx context.Context, job PipelineIngestJob) error {
	record, err := pipelineIngestRecordFromJob(job)
	if err != nil {
		return err
	}
	result := s.repos.DB.WithContext(ctx).Model(&model.PipelineIngestJobRecord{}).
		Where("id = ?", job.ID).
		Updates(map[string]any{
			"status":       record.Status,
			"stage":        record.Stage,
			"message":      record.Message,
			"error":        record.Error,
			"request_json": record.RequestJSON,
			"result_json":  record.ResultJSON,
			"updated_at":   record.UpdatedAt,
			"finished_at":  record.FinishedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("pipeline ingest job persistence target not found")
	}
	return nil
}

func (s *PipelineIngestService) cachedJob(id string) (PipelineIngestJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	if job == nil {
		return PipelineIngestJob{}, false
	}
	return clonePipelineIngestJob(*job), true
}

func (s *PipelineIngestService) cacheJob(job PipelineIngestJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = clonePipelineIngestJob(job).ptr()
	recent := make([]string, 0, len(s.recent)+1)
	recent = append(recent, job.ID)
	for _, existingID := range s.recent {
		if existingID != job.ID {
			recent = append(recent, existingID)
		}
	}
	if len(recent) > 100 {
		for _, oldID := range recent[100:] {
			if !s.executing[oldID] {
				delete(s.jobs, oldID)
			}
		}
		recent = recent[:100]
	}
	s.recent = recent
}

func (s *PipelineIngestService) scheduleJob(id string) bool {
	s.mu.Lock()
	if s.executing[id] {
		s.mu.Unlock()
		return false
	}
	s.executing[id] = true
	s.mu.Unlock()
	go s.run(context.Background(), id)
	return true
}

func (s *PipelineIngestService) clearExecuting(id string) {
	s.mu.Lock()
	delete(s.executing, id)
	s.mu.Unlock()
}

func pipelineIngestRecordFromJob(job PipelineIngestJob) (model.PipelineIngestJobRecord, error) {
	requestJSON, err := json.Marshal(job.Request)
	if err != nil {
		return model.PipelineIngestJobRecord{}, err
	}
	resultJSON, err := json.Marshal(job.Result)
	if err != nil {
		return model.PipelineIngestJobRecord{}, err
	}
	return model.PipelineIngestJobRecord{
		ID:             job.ID,
		IdempotencyKey: job.Request.IdempotencyKey,
		Status:         job.Status,
		Stage:          job.Stage,
		Message:        job.Message,
		Error:          job.Error,
		RequestJSON:    string(requestJSON),
		ResultJSON:     string(resultJSON),
		StartedAt:      job.StartedAt,
		UpdatedAt:      job.UpdatedAt,
		FinishedAt:     cloneTimePointer(job.FinishedAt),
	}, nil
}

func pipelineIngestJobFromRecord(record model.PipelineIngestJobRecord) (PipelineIngestJob, error) {
	var request PipelineIngestRequest
	if err := json.Unmarshal([]byte(record.RequestJSON), &request); err != nil {
		return PipelineIngestJob{}, err
	}
	var result PipelineIngestResult
	if err := json.Unmarshal([]byte(record.ResultJSON), &result); err != nil {
		return PipelineIngestJob{}, err
	}
	return PipelineIngestJob{
		ID:         record.ID,
		Status:     record.Status,
		Stage:      record.Stage,
		Message:    record.Message,
		Error:      record.Error,
		Request:    request,
		Result:     result,
		StartedAt:  record.StartedAt,
		UpdatedAt:  record.UpdatedAt,
		FinishedAt: cloneTimePointer(record.FinishedAt),
	}, nil
}

func pipelineIngestRequestsEqual(left, right PipelineIngestRequest) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
