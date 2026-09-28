package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var taskLogURL = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)
var taskLogCredential = regexp.MustCompile(`(?i)(token|api[_-]?key|passkey|password|cookie|authorization)["']?\s*[:=]\s*[^\r\n]*`)
var taskLogBearer = regexp.MustCompile(`(?i)bearer\s+[^\s,;]+`)

func redactTaskText(value string) string {
	value = taskLogURL.ReplaceAllString(value, "[URL omitted]")
	value = taskLogCredential.ReplaceAllString(value, "$1=[redacted]")
	value = taskLogBearer.ReplaceAllString(value, "Bearer [redacted]")
	if len(value) > 16384 {
		value = string([]rune(value)[:min(4096, len([]rune(value)))]) + "…"
	}
	return value
}

func durableTask(task BackgroundTask) BackgroundTask {
	task = cloneBackgroundTask(task)
	task.Name, task.Message, task.Error = redactTaskText(task.Name), redactTaskText(task.Message), redactTaskText(task.Error)
	task.SourcePath, task.DestPath = redactTaskText(task.SourcePath), redactTaskText(task.DestPath)
	for i := range task.Details {
		task.Details[i] = redactTaskText(task.Details[i])
	}
	return task
}

// SetPersistence is called once before producers start. Interrupted executions
// are finalized and recent results are restored without rerunning any work.
func (t *TaskTrackerService) SetPersistence(db *gorm.DB) error {
	if t == nil || db == nil {
		return nil
	}
	t.db = db
	var rows []model.TaskExecution
	if err := db.Where("status = ?", TaskStatusRunning).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var task BackgroundTask
		if json.Unmarshal([]byte(row.Snapshot), &task) != nil {
			continue
		}
		now := t.currentTime().UTC()
		task.Status, task.Error, task.UpdatedAt, task.FinishedAt, task.Revision = TaskStatusFailed, "服务重启，任务执行已中断", now, &now, row.Revision+1
		t.persist(task)
	}
	rows = nil
	if err := db.Where("status <> ?", TaskStatusRunning).Order("updated_at DESC, id DESC").Limit(t.maxRecent).Find(&rows).Error; err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, row := range rows {
		var task BackgroundTask
		if json.Unmarshal([]byte(row.Snapshot), &task) == nil {
			task.Revision = row.Revision
			t.recent = append(t.recent, task)
		}
	}
	return nil
}

func (t *TaskTrackerService) persist(task BackgroundTask) {
	if t.db == nil {
		return
	}
	task = durableTask(task)
	t.storeMu.Lock()
	defer t.storeMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous model.TaskExecution
		err := tx.First(&previous, "id = ?", task.ID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && previous.Revision >= task.Revision {
			return nil
		}
		var before BackgroundTask
		_ = json.Unmarshal([]byte(previous.Snapshot), &before)
		encoded, err := json.Marshal(task)
		if err != nil {
			return err
		}
		record := model.TaskExecution{ID: task.ID, Kind: task.Kind, Status: task.Status, StartedAt: task.StartedAt, UpdatedAt: task.UpdatedAt, Revision: task.Revision, Snapshot: string(encoded)}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, UpdateAll: true}).Create(&record).Error; err != nil {
			return err
		}
		entries := make([]model.TaskLogEntry, 0)
		add := func(level, message string) {
			if strings.TrimSpace(message) != "" {
				entries = append(entries, model.TaskLogEntry{ID: uuid.NewString(), TaskID: task.ID, Kind: task.Kind, LoggedAt: task.UpdatedAt.UTC(), Level: level, Message: message})
			}
		}
		if before.Status != task.Status {
			add("info", task.Name+" · "+task.Status)
		}
		if before.Stage != task.Stage {
			add("info", "阶段: "+task.Stage)
		}
		if before.Message != task.Message {
			add("info", task.Message)
		}
		if before.Error != task.Error {
			add("error", task.Error)
		}
		// Compare occurrences, so rolling detail windows do not repeat old lines.
		seen := make(map[string]int)
		for _, line := range before.Details {
			seen[line]++
		}
		for _, line := range task.Details {
			if seen[line] > 0 {
				seen[line]--
				continue
			}
			add("info", line)
		}
		if len(entries) > 0 {
			return tx.CreateInBatches(entries, 100).Error
		}
		return nil
	})
	if err != nil && t.log != nil {
		t.log.Warn("persist task execution failed", zap.Error(err))
	}
}
