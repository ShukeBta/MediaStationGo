package service

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) localMediaProbeWorker() {
	for task := range s.localMediaProbeQueue {
		s.probeLocalMediaAsync(task)
	}
}

func (s *ScannerService) queueLocalMediaProbe(path string) bool {
	task, ok := s.newLocalMediaProbeTask(path)
	if !ok {
		return false
	}
	s.startLocalMediaProbeWorkers()
	if !s.reserveLocalMediaProbe(task.path) {
		return false
	}
	if s.enqueueLocalMediaProbe(task) {
		return true
	}
	s.releaseLocalMediaProbe(task.path)
	s.logLocalMediaProbeQueueFull(task)
	return false
}

func (s *ScannerService) newLocalMediaProbeTask(path string) (localMediaProbeTask, bool) {
	if s == nil || s.probe == nil {
		return localMediaProbeTask{}, false
	}
	task := localMediaProbeTask{path: strings.TrimSpace(path)}
	return task, task.path != ""
}

func (s *ScannerService) startLocalMediaProbeWorkers() {
	s.localMediaProbeOnce.Do(func() {
		workers := s.ffprobeWorkerCount()
		for i := 0; i < workers; i++ {
			go s.localMediaProbeWorker()
		}
	})
}

func (s *ScannerService) reserveLocalMediaProbe(path string) bool {
	s.localMediaProbeMu.Lock()
	defer s.localMediaProbeMu.Unlock()
	if s.localMediaProbing == nil {
		s.localMediaProbing = make(map[string]struct{})
	}
	if _, ok := s.localMediaProbing[path]; ok {
		return false
	}
	s.localMediaProbing[path] = struct{}{}
	return true
}

func (s *ScannerService) releaseLocalMediaProbe(path string) {
	s.localMediaProbeMu.Lock()
	delete(s.localMediaProbing, path)
	s.localMediaProbeMu.Unlock()
}

func (s *ScannerService) enqueueLocalMediaProbe(task localMediaProbeTask) bool {
	select {
	case s.localMediaProbeQueue <- task:
		return true
	default:
		return false
	}
}

func (s *ScannerService) logLocalMediaProbeQueueFull(task localMediaProbeTask) {
	if s != nil && s.log != nil {
		s.log.Debug("local media probe queue full", zap.String("path", task.path))
	}
}

func (s *ScannerService) probeLocalMediaAsync(task localMediaProbeTask) {
	defer s.releaseLocalMediaProbe(task.path)
	if s == nil || s.probe == nil || strings.TrimSpace(task.path) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	probe, err := s.probe.Probe(ctx, task.path)
	if err != nil {
		if s.log != nil {
			s.log.Debug("local media async probe failed", zap.String("path", task.path), zap.Error(err))
		}
		return
	}
	updates := probeResultUpdates(probe)
	if len(updates) == 0 {
		return
	}
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("path = ?", task.path).Updates(updates).Error; err != nil {
		if s.log != nil {
			s.log.Debug("update local media track metadata failed", zap.String("path", task.path), zap.Error(err))
		}
		return
	}
	if s.hub != nil {
		s.hub.Publish("scan", map[string]any{
			"path":          task.path,
			"track_probed":  true,
			"duration_sec":  probe.DurationSec,
			"video_codec":   probe.VideoCodec,
			"audio_codec":   probe.AudioCodec,
			"width":         probe.Width,
			"height":        probe.Height,
			"probe_message": "本地媒体轨道元数据已后台补齐",
		})
	}
}
