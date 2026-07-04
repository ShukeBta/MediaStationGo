package service

import (
	"strings"
	"time"

	"go.uber.org/zap"
)

func (s *ScannerService) cloudMediaProbeWorker() {
	for task := range s.cloudMediaProbeQueue {
		s.probeCloudMediaAsync(task)
	}
}

func (s *ScannerService) queueCloudMediaProbe(typ, ref, path string) bool {
	task, ok := s.newCloudMediaProbeTask(typ, ref, path)
	if !ok || !s.reserveCloudMediaProbe(task, time.Now()) {
		return false
	}
	select {
	case s.cloudMediaProbeQueue <- task:
		return true
	default:
		s.deferCloudMediaProbe(task, cloudMediaProbeQueueFullBackoff)
		s.logCloudMediaProbeQueueFull(task)
		return false
	}
}

func (s *ScannerService) newCloudMediaProbeTask(typ, ref, path string) (cloudMediaProbeTask, bool) {
	if s == nil || s.storage == nil || s.probe == nil {
		return cloudMediaProbeTask{}, false
	}
	task := cloudMediaProbeTask{
		typ:  strings.TrimSpace(typ),
		ref:  strings.TrimSpace(ref),
		path: strings.TrimSpace(path),
	}
	return task, task.typ != "" && task.ref != "" && task.path != ""
}

func (s *ScannerService) reserveCloudMediaProbe(task cloudMediaProbeTask, now time.Time) bool {
	s.cloudMediaProbeMu.Lock()
	defer s.cloudMediaProbeMu.Unlock()
	if until, ok := s.cloudMediaProbeBackoff[task.path]; ok {
		if now.Before(until) {
			return false
		}
		delete(s.cloudMediaProbeBackoff, task.path)
	}
	if _, ok := s.cloudMediaProbing[task.path]; ok {
		return false
	}
	s.cloudMediaProbing[task.path] = struct{}{}
	return true
}

func (s *ScannerService) deferCloudMediaProbe(task cloudMediaProbeTask, backoff time.Duration) {
	s.cloudMediaProbeMu.Lock()
	defer s.cloudMediaProbeMu.Unlock()
	delete(s.cloudMediaProbing, task.path)
	if s.cloudMediaProbeBackoff == nil {
		s.cloudMediaProbeBackoff = make(map[string]time.Time)
	}
	s.cloudMediaProbeBackoff[task.path] = time.Now().Add(backoff)
}

func (s *ScannerService) logCloudMediaProbeQueueFull(task cloudMediaProbeTask) {
	if s == nil || s.log == nil {
		return
	}
	now := time.Now()
	s.cloudMediaProbeWarnMu.Lock()
	shouldWarn := now.Sub(s.cloudMediaProbeLastWarn) >= time.Minute
	if shouldWarn {
		s.cloudMediaProbeLastWarn = now
	}
	s.cloudMediaProbeWarnMu.Unlock()
	if shouldWarn {
		s.log.Warn("cloud media probe queue full; deferring remaining probes (logged at most once per minute)",
			zap.String("provider", task.typ), zap.String("path", task.path))
		return
	}
	s.log.Debug("cloud media probe queue full", zap.String("provider", task.typ), zap.String("path", task.path))
}

func (s *ScannerService) queueCloudMediaProbeWithBudget(typ, ref, path string, budget *int) bool {
	if budget != nil {
		if *budget <= 0 {
			return false
		}
		// Budget is consumed per attempt, not only per successful enqueue, so a
		// full probe queue cannot generate unbounded repeated attempts/logging.
		*budget--
	}
	return s.queueCloudMediaProbe(typ, ref, path)
}
