package service

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

// JobStatus is a snapshot suitable for the admin UI.
type JobStatus struct {
	Name     string    `json:"name"`
	Interval string    `json:"interval"`
	LastRun  time.Time `json:"last_run,omitempty"`
	LastErr  string    `json:"last_err,omitempty"`
	Running  bool      `json:"running,omitempty"`
	Started  time.Time `json:"started_at,omitempty"`
}

// Status returns the current state of every registered job.
func (s *SchedulerService) Status() []JobStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]JobStatus, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, JobStatus{
			Name:     j.name,
			Interval: j.interval.String(),
			LastRun:  j.lastRun,
			LastErr:  j.lastErr,
			Running:  j.running,
			Started:  j.started,
		})
	}
	return out
}

// RunNow triggers a single run of the named job synchronously.
func (s *SchedulerService) RunNow(ctx context.Context, name string) error {
	j := s.jobByName(name)
	if j == nil {
		return ErrSchedulerJobNotFound
	}
	return s.runOnce(context.WithValue(ctx, schedulerManualRunKey{}, true), j)
}

// RunNowAsync triggers a named job in the background and returns immediately.
// The job is detached from the HTTP request cancellation so a browser timeout,
// route change, or reverse-proxy disconnect cannot kill long organize/scan work.
func (s *SchedulerService) RunNowAsync(ctx context.Context, name string) error {
	j := s.jobByName(name)
	if j == nil {
		return ErrSchedulerJobNotFound
	}
	runCtx := context.Background()
	if ctx != nil {
		runCtx = context.WithoutCancel(ctx)
	}
	runCtx = context.WithValue(runCtx, schedulerManualRunKey{}, true)
	if err := s.beginRun(j); err != nil {
		return err
	}
	go func() {
		if err := s.runReserved(runCtx, j); err != nil && s.log != nil {
			s.log.Warn("manual scheduled job failed", zap.String("name", name), zap.Error(err))
		}
	}()
	return nil
}

func (s *SchedulerService) loop(ctx context.Context, j *scheduledJob) {
	s.loopWithInitialDelay(ctx, j, 15*time.Second)
}

func (s *SchedulerService) loopWithInitialDelay(ctx context.Context, j *scheduledJob, initialDelay time.Duration) {
	delay := initialDelay
	for {
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-s.stopCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
		if err := s.runOnce(ctx, j); err != nil {
			if errors.Is(err, ErrSchedulerJobAlreadyRunning) {
				s.log.Debug("scheduled job skipped; previous run still active", zap.String("name", j.name))
				delay = j.interval
				continue
			}
			s.log.Warn("scheduled job failed",
				zap.String("name", j.name), zap.Error(err))
		}
		delay = j.interval
	}
}

func (s *SchedulerService) runOnce(ctx context.Context, j *scheduledJob) error {
	if err := s.beginRun(j); err != nil {
		return err
	}
	return s.runReserved(ctx, j)
}

func (s *SchedulerService) jobByName(name string) *scheduledJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.name == name {
			return j
		}
	}
	return nil
}

func (s *SchedulerService) beginRun(j *scheduledJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.running {
		return ErrSchedulerJobAlreadyRunning
	}
	j.running = true
	j.started = s.currentTime()
	return nil
}

func (s *SchedulerService) runReserved(ctx context.Context, j *scheduledJob) error {
	err := j.run(ctx)
	s.mu.Lock()
	j.lastRun = s.currentTime()
	if err != nil {
		j.lastErr = err.Error()
	} else {
		j.lastErr = ""
	}
	j.running = false
	j.started = time.Time{}
	lastErr := j.lastErr
	s.mu.Unlock()
	if s.hub != nil {
		s.hub.Publish("scheduler", map[string]any{
			"name":  j.name,
			"ok":    err == nil,
			"error": lastErr,
		})
	}
	return err
}

func (s *SchedulerService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}
