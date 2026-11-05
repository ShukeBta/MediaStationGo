package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const subscriptionDVReenqueueCap = 3

type completedDVPayloadResult struct {
	SourcePath         string
	RemovedVideoFiles  int
	RemainingVideoFiles int
	TriggeredReenqueue bool
}

func (d *DownloadService) guardCompletedSubscriptionTorrent(ctx context.Context, torrent QBitTorrent, task *model.DownloadTask) completedDVPayloadResult {
	result := completedDVPayloadResult{}
	if task == nil || strings.TrimSpace(task.SubscriptionID) == "" {
		return result
	}
	defer d.markCompletionGuardChecked(ctx, task)
	if d == nil || d.completionProbe == nil || d.subscriptionRunner == nil || d.repo == nil || d.repo.DB == nil {
		if d != nil && d.log != nil {
			d.log.Warn("DV profile guard skipped because a required dependency is unavailable",
				zap.String("subscription_id", task.SubscriptionID),
				zap.String("torrent_hash", torrent.Hash),
				zap.String("action", "guard_skipped_dependency_missing"))
		}
		return result
	}

	var sub model.Subscription
	if err := d.repo.DB.WithContext(ctx).Where("id = ?", task.SubscriptionID).First(&sub).Error; err != nil {
		if d.log != nil {
			d.log.Warn("DV profile guard skipped because subscription could not be loaded",
				zap.String("subscription_id", task.SubscriptionID), zap.String("torrent_hash", torrent.Hash), zap.Error(err),
				zap.String("action", "guard_skipped_subscription_unavailable"))
		}
		return result
	}
	if !subscriptionDVGuardEnabled(&sub, d.subscriptionRunner.selectionDefaultExcludeWords(ctx)) {
		if d.log != nil {
			d.log.Debug("DV profile guard skipped because effective subscription exclusions do not include Dolby Vision",
				zap.String("subscription_id", sub.ID), zap.String("subscription", sub.Name),
				zap.String("torrent_hash", torrent.Hash), zap.String("action", "guard_skipped_exclusion_missing"))
		}
		return result
	}

	source := d.completedTorrentSource(ctx, torrent)
	result.SourcePath = source
	if source == "" {
		d.logDVGuardAction(&sub, task, torrent, "", nil, 0, 0, "probe_inconclusive", zap.String("reason", "completed payload path is not accessible"))
		return result
	}
	files, err := completedTorrentVideoFiles(ctx, source)
	if err != nil || len(files) == 0 {
		fields := []zap.Field{zap.String("reason", "completed payload contains no readable video files")}
		if err != nil {
			fields = append(fields, zap.Error(err))
		}
		d.logDVGuardAction(&sub, task, torrent, source, nil, 0, 0, "probe_inconclusive", fields...)
		return result
	}

	releaseTitle := strings.TrimSpace(task.Title)
	if releaseTitle == "" {
		releaseTitle = strings.TrimSpace(torrent.Name)
	}
	releaseIdentity := rejectedReleaseIdentity(releaseTitle, task.IdentityKey)
	if releaseIdentity == "" {
		d.logDVGuardAction(&sub, task, torrent, source, nil, 0, 0, "probe_inconclusive", zap.String("reason", "release identity is empty"))
		return result
	}

	deletedEpisodes := map[string]struct{}{}
	reenqueueAllowed := false
	for _, videoPath := range files {
		if err := ctx.Err(); err != nil {
			d.logDVGuardAction(&sub, task, torrent, videoPath, nil, 0, 0, "probe_inconclusive", zap.Error(err))
			break
		}
		info, statErr := os.Stat(videoPath)
		if statErr != nil || !info.Mode().IsRegular() {
			d.logDVGuardAction(&sub, task, torrent, videoPath, nil, 0, 0, "probe_inconclusive", zap.String("reason", "video file is missing or unreadable"), zap.Error(statErr))
			continue
		}
		probe, probeErr := d.completionProbe.Probe(ctx, videoPath)
		if probeErr != nil {
			d.logDVGuardAction(&sub, task, torrent, videoPath, probe, 0, 0, "probe_inconclusive", zap.Error(probeErr))
			continue
		}
		if doviProbeInconclusive(probe) {
			d.logDVGuardAction(&sub, task, torrent, videoPath, probe, 0, 0, "probe_inconclusive", zap.String("reason", "ffprobe did not provide complete JSON DOVI configuration"))
			continue
		}
		if !isDolbyVisionProfile5(probe) {
			d.logDVGuardAction(&sub, task, torrent, videoPath, probe, 0, 0, "skipped_not_p5")
			continue
		}

		refs := episodeRefsFromTitle(videoPath)
		if len(refs) == 0 && len(files) == 1 {
			refs = episodeRefsFromTitle(releaseTitle)
		}
		series := isSubscriptionSeriesType(sub.MediaType)
		if len(refs) == 0 && series {
			d.logDVGuardAction(&sub, task, torrent, videoPath, probe, 0, 0, "probe_inconclusive", zap.String("reason", "episode identity could not be parsed from filename or single-file release title"))
			continue
		}
		if len(refs) == 0 {
			refs = []episodeRef{{Season: 0, Episode: 0}}
		}
		for i := range refs {
			if series && refs[i].Season <= 0 {
				refs[i].Season = sub.SeasonNumber
				if refs[i].Season <= 0 {
					refs[i].Season = 1
				}
			}
		}

		counts := make(map[string]int, len(refs))
		persistenceFailed := false
		for _, ref := range refs {
			count, recordErr := d.subscriptionRunnerRecordRejection(ctx, sub.ID, ref.Season, ref.Episode, releaseIdentity, releaseTitle)
			if recordErr != nil {
				persistenceFailed = true
				d.logDVGuardAction(&sub, task, torrent, videoPath, probe, ref.Season, ref.Episode, "rejection_store_failed", zap.Error(recordErr))
				continue
			}
			key := episodeKey(ref.Season, ref.Episode)
			counts[key] = count
			if count <= subscriptionDVReenqueueCap {
				reenqueueAllowed = true
			} else if d.log != nil {
				d.logDVGuardAction(&sub, task, torrent, videoPath, probe, ref.Season, ref.Episode, "cap_reached")
			}
		}
		if persistenceFailed {
			// Do not remove content unless the blacklist can survive restart.
			continue
		}
		if err := removeMediaAndNFO(videoPath); err != nil {
			d.logDVGuardAction(&sub, task, torrent, videoPath, probe, firstEpisodeSeason(refs), firstEpisodeNumber(refs), "rejected_delete_failed", zap.Error(err))
			continue
		}
		result.RemovedVideoFiles++
		for _, ref := range refs {
			key := episodeKey(ref.Season, ref.Episode)
			if _, done := deletedEpisodes[key]; done {
				continue
			}
			deletedEpisodes[key] = struct{}{}
			if err := d.deleteMatchingRejectedLibraryMedia(ctx, &sub, task, torrent, releaseTitle, videoPath, ref.Season, ref.Episode); err != nil && d.log != nil {
				d.log.Warn("DV profile rejection could not fully clean the media library row",
					zap.String("subscription_id", sub.ID), zap.String("subscription", sub.Name),
					zap.Int("season", ref.Season), zap.Int("episode", ref.Episode),
					zap.String("torrent_hash", torrent.Hash), zap.String("release_title", releaseTitle),
					zap.Error(err), zap.String("action", "library_cleanup_failed"))
			}
		}
		for _, ref := range refs {
			d.logDVGuardAction(&sub, task, torrent, videoPath, probe, ref.Season, ref.Episode, "rejected_deleted", zap.Int("episode_rejected_count", counts[episodeKey(ref.Season, ref.Episode)]))
		}
	}

	if result.RemovedVideoFiles > 0 {
		result.RemainingVideoFiles = countExistingVideoFiles(files)
		d.markCompletedDownloadTaskRejected(ctx, task)
		if reenqueueAllowed {
			d.runSubscriptionAfterDVRejection(ctx, &sub, task, torrent, releaseTitle, releaseIdentity)
			result.TriggeredReenqueue = true
		}
	}
	return result
}

func isDolbyVisionProfile5(result *ProbeResult) bool {
	return result != nil && result.Document != nil && result.HasDOVIConfiguration &&
		result.HasDVProfile && result.HasDVBLCompatibilityID &&
		result.DVProfile == 5 && result.DVBLCompatibilityID == 0
}

func doviProbeInconclusive(result *ProbeResult) bool {
	return result == nil || result.Document == nil ||
		result.HasDOVIConfiguration && (!result.HasDVProfile || !result.HasDVBLCompatibilityID)
}

func completedTorrentVideoFiles(ctx context.Context, source string) ([]string, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !isCompletedVideoPath(source) {
			return nil, nil
		}
		return []string{source}, nil
	}
	files := make([]string, 0)
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if isCompletedVideoPath(path) {
			files = append(files, path)
		}
		if len(files) > 10000 {
			return errors.New("completed torrent contains too many video files")
		}
		return nil
	})
	return files, err
}

func isCompletedVideoPath(path string) bool {
	_, ok := videoExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func countExistingVideoFiles(files []string) int {
	count := 0
	for _, file := range files {
		if info, err := os.Stat(file); err == nil && info.Mode().IsRegular() {
			count++
		}
	}
	return count
}

func (d *DownloadService) subscriptionRunnerRecordRejection(ctx context.Context, subID string, season, episode int, identity, title string) (int, error) {
	// The runner interface stays narrow for tests; persistent writes use the
	// same repository database as subscriptions and download tasks.
	if d == nil || d.repo == nil || d.repo.DB == nil {
		return 0, errRejectedReleaseStoreUnavailable
	}
	store := &SubscriptionService{repo: d.repo, log: d.log}
	return store.recordRejectedSubscriptionEpisode(ctx, subID, season, episode, identity, title)
}

func (d *DownloadService) markCompletedDownloadTaskRejected(ctx context.Context, task *model.DownloadTask) {
	if d == nil || d.repo == nil || d.repo.DB == nil || task == nil || task.ID == "" {
		return
	}
	if err := d.repo.DB.WithContext(ctx).Model(&model.DownloadTask{}).Where("id = ?", task.ID).Update("status", "deleted").Error; err != nil && d.log != nil {
		d.log.Warn("rejected subscription download task could not be cleared from availability",
			zap.String("subscription_id", task.SubscriptionID), zap.String("task_id", task.ID), zap.Error(err))
	}
}

func (d *DownloadService) markCompletionGuardChecked(ctx context.Context, task *model.DownloadTask) {
	if d == nil || d.repo == nil || d.repo.DB == nil || task == nil || task.ID == "" {
		return
	}
	if err := d.repo.DB.WithContext(ctx).Model(&model.DownloadTask{}).Where("id = ?", task.ID).Update("completion_guard_checked", true).Error; err != nil && d.log != nil {
		d.log.Warn("subscription download completion guard state could not be saved",
			zap.String("subscription_id", task.SubscriptionID), zap.String("task_id", task.ID), zap.Error(err))
	}
}

func (d *DownloadService) runSubscriptionAfterDVRejection(ctx context.Context, sub *model.Subscription, task *model.DownloadTask, torrent QBitTorrent, title, identity string) {
	if d == nil || d.subscriptionRunner == nil || sub == nil {
		return
	}
	before := map[string]struct{}{}
	if rows, err := d.repo.Download.List(ctx); err == nil {
		for _, row := range rows {
			before[row.ID] = struct{}{}
		}
	}
	queued, err := d.subscriptionRunner.RunNow(ctx, sub.ID)
	newTitles := make([]string, 0)
	if rows, listErr := d.repo.Download.List(ctx); listErr == nil {
		for _, row := range rows {
			if row.SubscriptionID != sub.ID {
				continue
			}
			if _, existed := before[row.ID]; !existed {
				newTitles = append(newTitles, row.Title)
			}
		}
	}
	fields := d.dvGuardFields(sub, task, torrent, "", nil, 0, 0)
	action := "reenqueue_triggered"
	if err != nil {
		action = "reenqueue_failed"
	}
	fields = append(fields,
		zap.String("release_title", title), zap.String("release_identity", identity),
		zap.Int("reenqueue_count", queued), zap.Strings("reenqueue_release_titles", newTitles),
		zap.String("action", action),
	)
	if err != nil {
		fields = append(fields, zap.Error(err))
		if d.log != nil {
			d.log.Error("subscription re-selection after DV P5 rejection failed", fields...)
		}
		return
	}
	if d.log != nil {
		d.log.Info("subscription re-selection after DV P5 rejection finished", fields...)
	}
}

func (d *DownloadService) deleteMatchingRejectedLibraryMedia(ctx context.Context, sub *model.Subscription, task *model.DownloadTask, torrent QBitTorrent, releaseTitle, rejectedPath string, season, episode int) error {
	if d == nil || d.repo == nil || d.repo.DB == nil || sub == nil || episode < 0 {
		return nil
	}
	var rows []model.Media
	if err := d.repo.DB.WithContext(ctx).Where("season_num = ? AND episode_num = ?", season, episode).Find(&rows).Error; err != nil {
		return err
	}
	queries := subscriptionAvailabilityQueries(sub)
	for _, row := range rows {
		rowPath := strings.TrimSpace(row.Path)
		if rowPath == "" {
			continue
		}
		sameRejectedPath := samePath(rowPath, rejectedPath)
		if !sameRejectedPath && !availabilityTitleMatchesAny(strings.Join([]string{row.Path, row.Title, row.OriginalName}, " "), queries) {
			continue
		}
		if !sameRejectedPath && !strings.EqualFold(filepath.Ext(rowPath), ".strm") {
			probe, err := d.completionProbe.Probe(ctx, rowPath)
			if err != nil || doviProbeInconclusive(probe) {
				d.logDVGuardAction(sub, task, torrent, rowPath, probe, season, episode, "probe_inconclusive", zap.String("reason", "organized media probe did not return JSON tracks"), zap.Error(err))
				continue
			}
			if !isDolbyVisionProfile5(probe) {
				d.logDVGuardAction(sub, task, torrent, rowPath, probe, season, episode, "skipped_not_p5")
				continue
			}
		}
		if err := removeMediaAndNFO(rowPath); err != nil {
			return err
		}
		if d.repo.STRM != nil {
			records, err := d.repo.STRM.FindByMediaID(ctx, row.ID)
			if err != nil {
				return err
			}
			for _, record := range records {
				if err := removeMediaAndNFO(record.FilePath); err != nil {
					return err
				}
				if err := d.repo.STRM.Delete(ctx, record.ID); err != nil {
					return err
				}
			}
		}
		if d.mediaService != nil {
			if err := d.mediaService.SoftDeleteBy(ctx, row.ID, "", "version"); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else {
			if err := d.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ?", row.ID).Update("deletion_kind", "version").Error; err != nil {
				return err
			}
			if err := d.repo.DB.WithContext(ctx).Delete(&model.Media{}, "id = ?", row.ID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *DownloadService) logDVGuardAction(sub *model.Subscription, task *model.DownloadTask, torrent QBitTorrent, path string, probe *ProbeResult, season, episode int, action string, extra ...zap.Field) {
	if d == nil || d.log == nil {
		return
	}
	fields := d.dvGuardFields(sub, task, torrent, path, probe, season, episode)
	hasCount := false
	for _, field := range extra {
		if field.Key == "episode_rejected_count" {
			hasCount = true
			break
		}
	}
	fields = append(fields, extra...)
	if !hasCount {
		fields = append(fields, zap.Int("episode_rejected_count", 0))
	}
	fields = append(fields, zap.String("action", action))
	if action == "cap_reached" || action == "rejection_store_failed" || action == "rejected_delete_failed" {
		d.log.Error("DV profile guard action", fields...)
		return
	}
	d.log.Info("DV profile guard action", fields...)
}

func (d *DownloadService) dvGuardFields(sub *model.Subscription, task *model.DownloadTask, torrent QBitTorrent, path string, probe *ProbeResult, season, episode int) []zap.Field {
	releaseTitle, identity := "", ""
	if task != nil {
		releaseTitle = strings.TrimSpace(task.Title)
	}
	if releaseTitle == "" {
		releaseTitle = strings.TrimSpace(torrent.Name)
	}
	if task != nil {
		identity = rejectedReleaseIdentity(releaseTitle, task.IdentityKey)
	}
	fields := []zap.Field{
		zap.String("subscription_id", ""), zap.String("subscription", ""),
				zap.Int("season", season), zap.Int("episode", episode),
				zap.String("torrent_hash", torrent.Hash), zap.String("release_title", releaseTitle),
		zap.String("release_identity", identity), zap.String("probe_path", path),
		zap.Int("dv_profile", 0), zap.Bool("has_dv_profile", false),
		zap.Int("dv_bl_signal_compatibility_id", 0), zap.Bool("has_dv_bl_signal_compatibility_id", false),
		zap.Bool("has_dovi_configuration", false),
	}
	if sub != nil {
		fields[0] = zap.String("subscription_id", sub.ID)
		fields[1] = zap.String("subscription", sub.Name)
	}
	if probe != nil {
		fields[8] = zap.Int("dv_profile", probe.DVProfile)
		fields[9] = zap.Bool("has_dv_profile", probe.HasDVProfile)
		fields[10] = zap.Int("dv_bl_signal_compatibility_id", probe.DVBLCompatibilityID)
		fields[11] = zap.Bool("has_dv_bl_signal_compatibility_id", probe.HasDVBLCompatibilityID)
		fields[12] = zap.Bool("has_dovi_configuration", probe.HasDOVIConfiguration)
	}
	return fields
}

func firstEpisodeSeason(refs []episodeRef) int {
	if len(refs) == 0 {
		return 0
	}
	return refs[0].Season
}

func firstEpisodeNumber(refs []episodeRef) int {
	if len(refs) == 0 {
		return 0
	}
	return refs[0].Episode
}
