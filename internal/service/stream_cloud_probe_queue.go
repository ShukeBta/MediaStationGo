package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

const (
	playbackCloudProbeTimeout          = 45 * time.Second
	playbackCloudProbePersistTimeout   = 5 * time.Second
	playbackCloudProbeFailureBackoff   = 6 * time.Hour
	playbackCloudProbeQueueFullBackoff = 10 * time.Minute
)

type playbackCloudProbeTask struct {
	mediaID string
	rawURL  string
	headers map[string]string
}

func playbackCloudProbeWorkerCount(cfg *config.Config) int {
	if cfg == nil {
		return 1
	}
	return normalizeFFprobeMaxConcurrent(cfg.App.FFprobeMaxConcurrent)
}

func (s *StreamService) enqueuePlaybackCloudProbe(media *model.Media, link *cloud.DirectLink, clientUA string) bool {
	if s == nil || media == nil || link == nil || !mediaTrackMetadataMissing(media) {
		return false
	}
	task := playbackCloudProbeTask{
		mediaID: strings.TrimSpace(media.ID),
		rawURL:  strings.TrimSpace(link.URL),
		headers: playbackCloudProbeHeaders(link.Headers, clientUA),
	}
	if task.mediaID == "" || task.rawURL == "" {
		return false
	}

	now := time.Now()
	s.cloudTrackProbeMu.Lock()
	if s.probe == nil || s.cloudTrackProbeQueue == nil {
		s.cloudTrackProbeMu.Unlock()
		return false
	}
	if s.cloudTrackProbePending == nil {
		s.cloudTrackProbePending = make(map[string]struct{})
	}
	if s.cloudTrackProbeBackoff == nil {
		s.cloudTrackProbeBackoff = make(map[string]time.Time)
	}
	if until, ok := s.cloudTrackProbeBackoff[task.mediaID]; ok {
		if now.Before(until) {
			s.cloudTrackProbeMu.Unlock()
			return false
		}
		delete(s.cloudTrackProbeBackoff, task.mediaID)
	}
	if _, pending := s.cloudTrackProbePending[task.mediaID]; pending {
		s.cloudTrackProbeMu.Unlock()
		return false
	}
	s.cloudTrackProbePending[task.mediaID] = struct{}{}
	s.cloudTrackProbeMu.Unlock()

	select {
	case s.cloudTrackProbeQueue <- task:
		return true
	default:
		s.finishPlaybackCloudProbe(task.mediaID, playbackCloudProbeQueueFullBackoff)
		s.logPlaybackCloudProbeQueueFull(task.mediaID)
		return false
	}
}

func playbackCloudProbeHeaders(headers map[string]string, clientUA string) map[string]string {
	out := make(map[string]string, len(headers)+1)
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), "User-Agent") {
			continue
		}
		out[key] = value
	}
	// CloudResolve used this exact client UA to obtain the signed link. Override
	// any provider-supplied value so ffprobe reads the URL under the same binding.
	out["User-Agent"] = clientUA
	return out
}

func (s *StreamService) playbackCloudProbeWorker() {
	for task := range s.cloudTrackProbeQueue {
		s.runPlaybackCloudProbe(task)
	}
}

func (s *StreamService) runPlaybackCloudProbe(task playbackCloudProbeTask) {
	backoff := time.Duration(0)
	defer func() {
		s.finishPlaybackCloudProbe(task.mediaID, backoff)
	}()

	probeCtx, cancel := context.WithTimeout(context.Background(), playbackCloudProbeTimeout)
	defer cancel()

	var current model.Media
	if s.repo == nil || s.repo.DB == nil {
		backoff = playbackCloudProbeFailureBackoff
		return
	}
	if err := s.repo.DB.WithContext(probeCtx).Where("id = ?", task.mediaID).First(&current).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			backoff = playbackCloudProbeFailureBackoff
			s.logPlaybackCloudProbeFailure("load media before probe failed", task.mediaID, err)
		}
		return
	}
	if !mediaTrackMetadataMissing(&current) {
		return
	}

	s.cloudTrackProbeMu.Lock()
	prober := s.probe
	s.cloudTrackProbeMu.Unlock()
	if prober == nil {
		backoff = playbackCloudProbeFailureBackoff
		return
	}
	probe, err := prober.ProbeHTTP(probeCtx, task.rawURL, task.headers)
	if err != nil {
		backoff = playbackCloudProbeFailureBackoff
		s.logPlaybackCloudProbeFailure("cloud playback metadata probe failed", task.mediaID, err)
		return
	}

	persistCtx, persistCancel := context.WithTimeout(context.Background(), playbackCloudProbePersistTimeout)
	defer persistCancel()
	var previous model.Media
	if err := s.repo.DB.WithContext(persistCtx).Where("id = ?", task.mediaID).First(&previous).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			backoff = playbackCloudProbeFailureBackoff
			s.logPlaybackCloudProbeFailure("load media before probe update failed", task.mediaID, err)
		}
		return
	}
	if !mediaTrackMetadataMissing(&previous) {
		return
	}
	if err := persistMediaProbeResult(persistCtx, s.repo, s.cache, s.generatedArtwork, s.log, &previous, probe); err != nil {
		backoff = playbackCloudProbeFailureBackoff
		s.logPlaybackCloudProbeFailure("persist cloud playback metadata probe failed", task.mediaID, err)
		return
	}
	if mediaTrackMetadataMissing(&previous) {
		backoff = playbackCloudProbeFailureBackoff
		s.logPlaybackCloudProbeFailure("cloud playback metadata remained incomplete after probe", task.mediaID, ErrMediaProbeIncomplete)
	}
}

func (s *StreamService) finishPlaybackCloudProbe(mediaID string, backoff time.Duration) {
	if s == nil {
		return
	}
	s.cloudTrackProbeMu.Lock()
	defer s.cloudTrackProbeMu.Unlock()
	delete(s.cloudTrackProbePending, mediaID)
	if backoff > 0 {
		if s.cloudTrackProbeBackoff == nil {
			s.cloudTrackProbeBackoff = make(map[string]time.Time)
		}
		s.cloudTrackProbeBackoff[mediaID] = time.Now().Add(backoff)
		return
	}
	delete(s.cloudTrackProbeBackoff, mediaID)
}

func (s *StreamService) logPlaybackCloudProbeFailure(message, mediaID string, err error) {
	if s == nil || s.log == nil {
		return
	}
	s.log.Debug(message, zap.String("media_id", mediaID), zap.Error(err))
}

func (s *StreamService) logPlaybackCloudProbeQueueFull(mediaID string) {
	if s == nil || s.log == nil {
		return
	}
	now := time.Now()
	s.cloudTrackProbeWarnMu.Lock()
	shouldWarn := now.Sub(s.cloudTrackProbeLastWarn) >= time.Minute
	if shouldWarn {
		s.cloudTrackProbeLastWarn = now
	}
	s.cloudTrackProbeWarnMu.Unlock()
	if shouldWarn {
		s.log.Warn("cloud playback metadata probe queue full; dropping probe",
			zap.String("media_id", mediaID))
		return
	}
	s.log.Debug("cloud playback metadata probe queue full", zap.String("media_id", mediaID))
}
