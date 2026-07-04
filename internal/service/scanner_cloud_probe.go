package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) probeCloudMediaAsync(task cloudMediaProbeTask) {
	defer func() {
		s.cloudMediaProbeMu.Lock()
		delete(s.cloudMediaProbing, task.path)
		s.cloudMediaProbeMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	probe, err := s.probeCloudFileMetadata(ctx, task.typ, task.ref)
	if err != nil {
		if s.log != nil {
			s.log.Debug("cloud media async probe failed", zap.String("provider", task.typ), zap.String("path", task.path), zap.Error(err))
		}
		s.cloudMediaProbeMu.Lock()
		if s.cloudMediaProbeBackoff == nil {
			s.cloudMediaProbeBackoff = make(map[string]time.Time)
		}
		s.cloudMediaProbeBackoff[task.path] = time.Now().Add(cloudMediaProbeFailureBackoff)
		s.cloudMediaProbeMu.Unlock()
		return
	}
	updates := probeResultUpdates(probe)
	if len(updates) == 0 {
		return
	}
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("path = ?", task.path).Updates(updates).Error; err != nil {
		if s.log != nil {
			s.log.Debug("update cloud media track metadata failed", zap.String("path", task.path), zap.Error(err))
		}
		return
	}
	s.cloudMediaProbeMu.Lock()
	delete(s.cloudMediaProbeBackoff, task.path)
	s.cloudMediaProbeMu.Unlock()
	if s.hub != nil {
		s.hub.Publish("scan", map[string]any{
			"path":          task.path,
			"cloud":         true,
			"track_probed":  true,
			"duration_sec":  probe.DurationSec,
			"video_codec":   probe.VideoCodec,
			"audio_codec":   probe.AudioCodec,
			"width":         probe.Width,
			"height":        probe.Height,
			"probe_message": "云盘媒体轨道元数据已后台补齐",
		})
	}
}

func (s *ScannerService) ffprobeWorkerCount() int {
	if s == nil || s.cfg == nil {
		return 1
	}
	return normalizeFFprobeMaxConcurrent(s.cfg.App.FFprobeMaxConcurrent)
}

func (s *ScannerService) cloudScanWorkerCount() int {
	if s == nil || s.cfg == nil {
		return 4
	}
	return normalizeCloudScanMaxConcurrent(s.cfg.App.CloudScanMaxConcurrent)
}

func normalizeCloudScanMaxConcurrent(n int) int {
	if n <= 0 {
		return 1
	}
	if n > 16 {
		return 16
	}
	return n
}

func (s *ScannerService) probeCloudFileMetadata(ctx context.Context, typ, ref string) (*ProbeResult, error) {
	if s == nil || s.probe == nil || s.storage == nil {
		return nil, errors.New("cloud probe unavailable")
	}
	link, err := s.storage.CloudResolve(ctx, typ, ref, "")
	if err != nil {
		return nil, err
	}
	return s.probe.ProbeHTTP(ctx, link.URL, link.Headers)
}

func probeResultUpdates(probe *ProbeResult) map[string]any {
	updates := map[string]any{}
	if probe == nil {
		return updates
	}
	if probe.DurationSec > 0 {
		updates["duration_sec"] = probe.DurationSec
	}
	if probe.Width > 0 {
		updates["width"] = probe.Width
	}
	if probe.Height > 0 {
		updates["height"] = probe.Height
	}
	if strings.TrimSpace(probe.VideoCodec) != "" {
		updates["video_codec"] = probe.VideoCodec
	}
	if strings.TrimSpace(probe.AudioCodec) != "" {
		updates["audio_codec"] = probe.AudioCodec
	}
	if probe.Container != "" {
		updates["container"] = probe.Container
	}
	return updates
}
