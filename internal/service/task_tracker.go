package service

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"

	TaskKindOrganize = "organize"
	TaskKindScan     = "scan"
	TaskKindScrape   = "scrape"
	TaskKindUpdate   = "update"
)

// BackgroundTask is the compact, operator-facing shape shown on the live tasks
// page. It tracks long-running work that is not represented by a download or
// transcode job, such as organize → scan → scrape ingest flows.
type BackgroundTask struct {
	ID         string           `json:"id"`
	Kind       string           `json:"kind"`
	Name       string           `json:"name"`
	Status     string           `json:"status"`
	Stage      string           `json:"stage,omitempty"`
	SourcePath string           `json:"source_path,omitempty"`
	DestPath   string           `json:"dest_path,omitempty"`
	Message    string           `json:"message,omitempty"`
	Error      string           `json:"error,omitempty"`
	Details    []string         `json:"details,omitempty"`
	Metrics    map[string]int64 `json:"metrics,omitempty"`
	StartedAt  time.Time        `json:"started_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
}

type TaskUpdate struct {
	Stage      string
	SourcePath string
	DestPath   string
	Message    string
	Details    []string
	Metrics    map[string]int64
}

type TaskSnapshot struct {
	Active []BackgroundTask `json:"active"`
	Recent []BackgroundTask `json:"recent"`
}

type TaskTrackerService struct {
	log *zap.Logger
	hub *Hub

	mu        sync.Mutex
	active    map[string]*BackgroundTask
	recent    []BackgroundTask
	maxRecent int
	now       func() time.Time
}

type TaskHandle struct {
	tracker *TaskTrackerService
	id      string
}

func NewTaskTrackerService(log *zap.Logger, hub *Hub) *TaskTrackerService {
	return &TaskTrackerService{
		log:       log,
		hub:       hub,
		active:    make(map[string]*BackgroundTask),
		maxRecent: 30,
		now:       time.Now,
	}
}

func (t *TaskTrackerService) Start(kind, name string, update TaskUpdate) *TaskHandle {
	if t == nil {
		return nil
	}
	now := t.currentTime()
	task := &BackgroundTask{
		ID:         uuid.NewString(),
		Kind:       kind,
		Name:       name,
		Status:     TaskStatusRunning,
		Stage:      update.Stage,
		SourcePath: update.SourcePath,
		DestPath:   update.DestPath,
		Message:    update.Message,
		Metrics:    cloneTaskMetrics(update.Metrics),
		StartedAt:  now,
		UpdatedAt:  now,
	}
	t.mu.Lock()
	t.active[task.ID] = task
	snapshot := cloneBackgroundTask(*task)
	t.mu.Unlock()
	t.publish(snapshot)
	return &TaskHandle{tracker: t, id: task.ID}
}

func (h *TaskHandle) Update(update TaskUpdate) {
	if h == nil || h.tracker == nil {
		return
	}
	h.tracker.update(h.id, update)
}

func (h *TaskHandle) Finish(err error, update TaskUpdate) {
	if h == nil || h.tracker == nil {
		return
	}
	h.tracker.finish(h.id, err, update)
}

func (t *TaskTrackerService) Snapshot() TaskSnapshot {
	if t == nil {
		return TaskSnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	active := make([]BackgroundTask, 0, len(t.active))
	for _, task := range t.active {
		active = append(active, cloneBackgroundTask(*task))
	}
	recent := make([]BackgroundTask, 0, len(t.recent))
	for _, task := range t.recent {
		recent = append(recent, cloneBackgroundTask(task))
	}
	return TaskSnapshot{Active: active, Recent: recent}
}

func (t *TaskTrackerService) update(id string, update TaskUpdate) {
	now := t.currentTime()
	t.mu.Lock()
	task, ok := t.active[id]
	if !ok {
		t.mu.Unlock()
		return
	}
	applyTaskUpdate(task, update)
	task.UpdatedAt = now
	snapshot := cloneBackgroundTask(*task)
	t.mu.Unlock()
	t.publish(snapshot)
}

func (t *TaskTrackerService) finish(id string, err error, update TaskUpdate) {
	now := t.currentTime()
	t.mu.Lock()
	task, ok := t.active[id]
	if !ok {
		t.mu.Unlock()
		return
	}
	applyTaskUpdate(task, update)
	task.UpdatedAt = now
	task.FinishedAt = &now
	if err != nil {
		task.Status = TaskStatusFailed
		task.Error = err.Error()
	} else {
		task.Status = TaskStatusCompleted
	}
	delete(t.active, id)
	snapshot := cloneBackgroundTask(*task)
	t.recent = append([]BackgroundTask{snapshot}, t.recent...)
	if t.maxRecent <= 0 {
		t.maxRecent = 30
	}
	if len(t.recent) > t.maxRecent {
		t.recent = t.recent[:t.maxRecent]
	}
	t.mu.Unlock()
	t.publish(snapshot)
}

func (t *TaskTrackerService) currentTime() time.Time {
	if t != nil && t.now != nil {
		return t.now()
	}
	return time.Now()
}

func (t *TaskTrackerService) publish(task BackgroundTask) {
	if t == nil || t.hub == nil {
		return
	}
	t.hub.Publish("task", task)
}

func applyTaskUpdate(task *BackgroundTask, update TaskUpdate) {
	if update.Stage != "" {
		task.Stage = update.Stage
	}
	if update.SourcePath != "" {
		task.SourcePath = update.SourcePath
	}
	if update.DestPath != "" {
		task.DestPath = update.DestPath
	}
	if update.Message != "" {
		task.Message = update.Message
	}
	if update.Details != nil {
		task.Details = append([]string(nil), update.Details...)
	}
	if update.Metrics != nil {
		task.Metrics = cloneTaskMetrics(update.Metrics)
	}
}

func cloneBackgroundTask(task BackgroundTask) BackgroundTask {
	task.Metrics = cloneTaskMetrics(task.Metrics)
	if task.Details != nil {
		task.Details = append([]string(nil), task.Details...)
	}
	if task.FinishedAt != nil {
		finishedAt := *task.FinishedAt
		task.FinishedAt = &finishedAt
	}
	return task
}

func cloneTaskMetrics(metrics map[string]int64) map[string]int64 {
	if len(metrics) == 0 {
		return nil
	}
	out := make(map[string]int64, len(metrics))
	for key, value := range metrics {
		out[key] = value
	}
	return out
}
