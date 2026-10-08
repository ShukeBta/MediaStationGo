package service

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// StartupStatus 是启动过程的轻量快照，不查询任务历史或暴露文件路径。
type StartupStatus struct {
	State               string   `json:"state"`
	Stage               string   `json:"stage"`
	ElapsedSeconds      int64    `json:"elapsed_seconds"`
	StageElapsedSeconds int64    `json:"stage_elapsed_seconds"`
	DirectoriesFound    int      `json:"directories_found"`
	DirectoriesWatched  int      `json:"directories_watched"`
	Warnings            []string `json:"warnings"`
}

// StartupState 与任务历史独立，目录遍历期间仍可读取进度。
type StartupState struct {
	mu                    sync.RWMutex
	status                StartupStatus
	started, stageStarted time.Time
	finished              time.Time
}

func NewStartupState() *StartupState {
	now := time.Now()
	return &StartupState{started: now, stageStarted: now, status: StartupStatus{
		State: "starting", Stage: "等待后台初始化", Warnings: []string{},
	}}
}

func (c *Container) StartupStatus() StartupStatus {
	if c == nil || c.Startup == nil {
		return StartupStatus{State: "ready", Stage: "初始化完成", Warnings: []string{}}
	}
	s := c.Startup
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.status
	status.Warnings = append([]string{}, status.Warnings...)
	now := s.finished
	if now.IsZero() {
		now = time.Now()
	}
	status.ElapsedSeconds = int64(now.Sub(s.started) / time.Second)
	status.StageElapsedSeconds = int64(now.Sub(s.stageStarted) / time.Second)
	return status
}

// startupStep 保留现有初始化顺序，并记录每一步实际耗时。
func (c *Container) startupStep(stage string, run func() error) error {
	if err := c.Context().Err(); err != nil {
		return err
	}
	started := time.Now()
	if s := c.Startup; s != nil {
		s.mu.Lock()
		s.status.Stage, s.stageStarted = stage, started
		s.mu.Unlock()
	}
	err := run()
	c.Startup.updateStageWarning(stage, err)
	if c.Log != nil {
		fields := []zap.Field{zap.String("stage", stage), zap.Duration("duration", time.Since(started))}
		if err != nil {
			c.Log.Warn("startup step failed", append(fields, zap.Error(err))...)
		} else {
			c.Log.Info("startup step completed", fields...)
		}
	}
	return err
}

func (s *StartupState) finish(state string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.State, s.finished = state, time.Now()
	if state == "ready" {
		s.status.Stage = "初始化完成"
		s.stageStarted = s.finished
	}
}

func (s *StartupState) updateDirectories(found, watched int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.DirectoriesFound, s.status.DirectoriesWatched = found, watched
}

func (s *StartupState) updateStageWarning(stage string, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	warning := stage + "未完成，请查看服务日志"
	for i, existing := range s.status.Warnings {
		if existing != warning {
			continue
		}
		if err == nil {
			s.status.Warnings = append(s.status.Warnings[:i], s.status.Warnings[i+1:]...)
		}
		return
	}
	if err != nil {
		s.status.Warnings = append(s.status.Warnings, warning)
	}
}
