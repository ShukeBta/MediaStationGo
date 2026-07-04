package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	cloudAutoSyncEnabledKey      = "cloud.auto_sync_enabled"
	cloudSyncIntervalSecondsKey  = "cloud.sync_interval_seconds"
	cloudLastAutoSyncDateKey     = "cloud.last_auto_sync_date"
	cloudAutoSyncWindowStartHour = 23
	cloudAutoSyncWindowEndHour   = 5
)

// jobUploadLocalToCloud copies local media files into the configured external
// storage backend. It is opt-in and never deletes the local source files.
func (s *SchedulerService) jobUploadLocalToCloud(ctx context.Context) error {
	manual, _ := ctx.Value(schedulerManualRunKey{}).(bool)
	if s.storageCfg == nil || (!manual && !s.autoCloudUploadEnabled(ctx)) {
		return nil
	}
	input := s.cloudUploadInput(ctx)
	if strings.TrimSpace(input.Type) == "" || strings.TrimSpace(input.SourcePath) == "" {
		return nil
	}
	res, err := s.storageCfg.UploadLocal(ctx, input)
	if s.log != nil && res != nil {
		s.log.Info("cloud upload finished",
			zap.String("type", input.Type),
			zap.String("source", res.SourcePath),
			zap.String("dest", res.DestPath),
			zap.Int("uploaded", res.Uploaded),
			zap.Int("skipped", res.Skipped),
			zap.Int64("bytes", res.Bytes),
			zap.Int("errors", len(res.Errors)),
		)
	}
	return err
}

func (s *SchedulerService) cloudUploadInput(ctx context.Context) CloudUploadInput {
	get := func(key string) string {
		if s.repo == nil || s.repo.Setting == nil {
			return ""
		}
		v, _ := s.repo.Setting.Get(ctx, key)
		return strings.TrimSpace(v)
	}
	return CloudUploadInput{
		Type:            get(CloudUploadProviderKey),
		SourcePath:      get(CloudUploadSourceDirKey),
		DestPath:        get(CloudUploadDestPathKey),
		Recursive:       parseBoolSetting(get(CloudUploadRecursiveKey), true),
		IncludeSidecars: parseBoolSetting(get(CloudUploadSidecarsKey), true),
		Overwrite:       parseBoolSetting(get(CloudUploadOverwriteKey), false),
		TransferMode:    get(CloudUploadTransferModeKey),
	}
}

func (s *SchedulerService) autoCloudUploadEnabled(ctx context.Context) bool {
	if s.repo == nil || s.repo.Setting == nil {
		return false
	}
	v, err := s.repo.Setting.Get(ctx, CloudUploadAutoEnabledKey)
	if err != nil {
		return false
	}
	return parseBoolSetting(v, false)
}

func (s *SchedulerService) cloudUploadInterval(ctx context.Context) time.Duration {
	const fallback = time.Hour
	if s.repo == nil || s.repo.Setting == nil {
		return fallback
	}
	v, err := s.repo.Setting.Get(ctx, CloudUploadIntervalSecondsKey)
	if err != nil {
		return fallback
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || seconds <= 0 {
		return fallback
	}
	if seconds < 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

// jobSyncCloudLibraries keeps mounted cloud:// libraries refreshed without
// enabling full disk scans. It imports remote cloud files as STRM-backed media
// rows; the actual bytes stay on the provider and playback continues through
// /api/cloud/play 302/proxy.
func (s *SchedulerService) jobSyncCloudLibraries(ctx context.Context) error {
	manual, _ := ctx.Value(schedulerManualRunKey{}).(bool)
	if s.scanner == nil || (!manual && !s.autoCloudSyncDue(ctx, s.currentTime())) {
		return nil
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return err
	}
	libs = FilterScannableCloudLibraries(ctx, s.repo, libs)
	var firstErr error
	for _, l := range libs {
		if !l.Enabled {
			continue
		}
		if _, ok := ParseCloudLibraryMount(l.Path); !ok {
			continue
		}
		if _, err := s.scanner.ScanLibraryWithoutAutoScrape(ctx, l.ID); err != nil {
			s.log.Warn("cloud sync failed", zap.String("library", l.ID), zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		return firstErr
	}
	if !manual {
		_ = s.markCloudAutoSyncCompleted(ctx, s.currentTime())
	}
	return nil
}

func (s *SchedulerService) autoCloudSyncEnabled(ctx context.Context) bool {
	if s.repo == nil || s.repo.Setting == nil {
		return false
	}
	v, err := s.repo.Setting.Get(ctx, cloudAutoSyncEnabledKey)
	if err != nil {
		return false
	}
	return parseBoolSetting(v, false)
}

func (s *SchedulerService) autoCloudSyncDue(ctx context.Context, now time.Time) bool {
	if !s.autoCloudSyncEnabled(ctx) || !cloudAutoSyncInWindow(now) {
		return false
	}
	if s.repo == nil || s.repo.Setting == nil {
		return true
	}
	last, err := s.repo.Setting.Get(ctx, cloudLastAutoSyncDateKey)
	if err != nil {
		return true
	}
	return strings.TrimSpace(last) != cloudAutoSyncWindowDate(now)
}

func cloudAutoSyncInWindow(now time.Time) bool {
	hour := now.In(time.Local).Hour()
	if cloudAutoSyncWindowStartHour == cloudAutoSyncWindowEndHour {
		return true
	}
	if cloudAutoSyncWindowStartHour < cloudAutoSyncWindowEndHour {
		return hour >= cloudAutoSyncWindowStartHour && hour < cloudAutoSyncWindowEndHour
	}
	return hour >= cloudAutoSyncWindowStartHour || hour < cloudAutoSyncWindowEndHour
}

func cloudAutoSyncWindowDate(now time.Time) string {
	local := now.In(time.Local)
	if cloudAutoSyncWindowStartHour > cloudAutoSyncWindowEndHour && local.Hour() < cloudAutoSyncWindowEndHour {
		local = local.AddDate(0, 0, -1)
	}
	return local.Format(cloudAutoSyncCompletedDateForm)
}

func (s *SchedulerService) markCloudAutoSyncCompleted(ctx context.Context, now time.Time) error {
	if s.repo == nil || s.repo.Setting == nil {
		return nil
	}
	return s.repo.Setting.Set(ctx, cloudLastAutoSyncDateKey, cloudAutoSyncWindowDate(now))
}

func (s *SchedulerService) cloudSyncInterval(ctx context.Context) time.Duration {
	const fallback = 30 * time.Minute
	if s.repo == nil || s.repo.Setting == nil {
		return fallback
	}
	v, err := s.repo.Setting.Get(ctx, cloudSyncIntervalSecondsKey)
	if err != nil {
		return fallback
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || seconds <= 0 {
		return fallback
	}
	if seconds < 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}
