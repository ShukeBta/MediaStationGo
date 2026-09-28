package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"gorm.io/gorm"
)

const (
	GeneratedArtworkStatusPending   = "pending"
	GeneratedArtworkStatusRunning   = "running"
	GeneratedArtworkStatusCompleted = "completed"
	GeneratedArtworkStatusFailed    = "failed"
	GeneratedArtworkStatusCanceled  = "canceled"

	generatedArtworkTimeout     = 25 * time.Second
	generatedArtworkMaxAttempts = 2
)

var errGeneratedArtworkMetadataChanged = errors.New("media metadata changed during preview extraction")

type GeneratedArtworkStatus struct {
	LibraryID string `json:"library_id"`
	Enabled   bool   `json:"enabled"`
	Pending   int64  `json:"pending"`
	Running   int64  `json:"running"`
	Completed int64  `json:"completed"`
	Failed    int64  `json:"failed"`
	Canceled  int64  `json:"canceled"`
}

type generatedArtworkRunner func(context.Context, string, map[string]string, float64, string, string) error

// GeneratedArtworkService extracts durable preview images for explicitly
// enabled libraries. A single worker keeps ffmpeg load bounded on small hosts.
type GeneratedArtworkService struct {
	cfg     *config.Config
	log     *zap.Logger
	repo    *repository.Container
	storage *StorageConfigService
	cache   *RuntimeCacheService
	tasks   *TaskTrackerService
	run     generatedArtworkRunner
	runMu   sync.Mutex

	wake      chan struct{}
	startOnce sync.Once

	mu            sync.Mutex
	activeCancel  context.CancelFunc
	activeLibrary string
	batchTask     *TaskHandle
	batchTotal    int64
	batchDone     int64
	batchFailed   int64
	batchSkipped  int64
}

func NewGeneratedArtworkService(
	cfg *config.Config,
	log *zap.Logger,
	repo *repository.Container,
	storage *StorageConfigService,
	cache *RuntimeCacheService,
	tasks *TaskTrackerService,
) *GeneratedArtworkService {
	s := &GeneratedArtworkService{
		cfg: cfg, log: log, repo: repo, storage: storage, cache: cache, tasks: tasks,
		wake: make(chan struct{}, 1),
	}
	s.run = s.runFFmpeg
	return s
}

func (s *GeneratedArtworkService) Start(ctx context.Context) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return
	}
	s.startOnce.Do(func() {
		if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
			Where("generated_artwork_status = ?", GeneratedArtworkStatusRunning).
			Update("generated_artwork_status", GeneratedArtworkStatusPending).Error; err != nil {
			s.logWarn("recover running generated artwork jobs failed", err, "")
		}
		if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
			Where("generated_artwork_status IN ? AND library_id IN (SELECT id FROM libraries WHERE generate_artwork = ?)", []string{GeneratedArtworkStatusPending, GeneratedArtworkStatusRunning}, false).
			Update("generated_artwork_status", GeneratedArtworkStatusCanceled).Error; err != nil {
			s.logWarn("cancel disabled generated artwork jobs failed", err, "")
		}
		var pending int64
		if err := s.pendingQuery(ctx).Count(&pending).Error; err != nil {
			s.logWarn("count pending generated artwork jobs failed", err, "")
		} else if pending > 0 {
			s.startBatchTask(pending, "恢复未完成的缺图预览任务")
		}
		go s.worker(ctx)
		s.signal()
	})
}

func (s *GeneratedArtworkService) QueueMissingForLibrary(ctx context.Context, libraryID string) (int64, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return 0, errors.New("generated artwork service unavailable")
	}
	lib, err := s.repo.Library.FindByID(ctx, strings.TrimSpace(libraryID))
	if err != nil {
		return 0, err
	}
	if lib == nil {
		return 0, gorm.ErrRecordNotFound
	}
	if !lib.GenerateArtwork {
		return 0, errors.New("generated artwork is disabled for this library")
	}
	var count int64
	if err := s.queueableMissingQuery(ctx, lib.ID).Count(&count).Error; err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, nil
	}
	if err := s.queueableMissingQuery(ctx, lib.ID).Updates(map[string]any{
		"generated_artwork_status":   GeneratedArtworkStatusPending,
		"generated_artwork_error":    "",
		"generated_artwork_attempts": gorm.Expr("COALESCE(generated_artwork_attempts, 0)"),
	}).Error; err != nil {
		return 0, err
	}
	s.startBatchTask(count, "生成缺图预览图")
	s.signal()
	return count, nil
}

// QueueRefreshForMedia keeps the currently served preview in place while a
// newly discovered duration makes its extraction point stale.
func (s *GeneratedArtworkService) QueueRefreshForMedia(ctx context.Context, mediaID string) (bool, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return false, errors.New("generated artwork service unavailable")
	}
	media, err := s.repo.Media.FindByID(ctx, strings.TrimSpace(mediaID))
	if err != nil {
		return false, err
	}
	if media == nil {
		return false, gorm.ErrRecordNotFound
	}
	lib, err := s.repo.Library.FindByID(ctx, media.LibraryID)
	if err != nil {
		return false, err
	}
	if lib == nil || !lib.Enabled || !lib.GenerateArtwork || !generatedArtworkNeedsGeneration(media) {
		return false, nil
	}
	wasActive := media.GeneratedArtworkStatus == GeneratedArtworkStatusPending || media.GeneratedArtworkStatus == GeneratedArtworkStatusRunning
	res := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("id = ? AND deleted_at IS NULL", media.ID).
		Updates(map[string]any{
			"generated_artwork_status":   GeneratedArtworkStatusPending,
			"generated_artwork_error":    "",
			"generated_artwork_attempts": 0,
		})
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	if !wasActive {
		s.startBatchTask(1, "媒体时长已补齐，重新生成预览图")
	}
	s.signal()
	return true, nil
}

// GenerateAtForMedia synchronously replaces generated artwork using an exact
// timestamp. It shares the same single-run lock as the background worker.
func (s *GeneratedArtworkService) GenerateAtForMedia(ctx context.Context, mediaID string, seek float64) (*model.Media, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return nil, errors.New("generated artwork service unavailable")
	}
	if math.IsNaN(seek) || math.IsInf(seek, 0) || seek < 1 {
		return nil, errors.New("preview timestamp must be at least 1 second")
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()

	media, err := s.repo.Media.FindByID(ctx, strings.TrimSpace(mediaID))
	if err != nil {
		return nil, err
	}
	if media == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if media.SeasonNum != 0 || media.EpisodeNum != 0 {
		return nil, errors.New("episode artwork is managed by metadata scraping")
	}
	if strings.TrimSpace(media.PosterURL) != "" || strings.TrimSpace(media.BackdropURL) != "" {
		return nil, errors.New("scraped artwork takes precedence; remove it before generating a video preview")
	}
	if media.DurationSec <= 0 {
		return nil, errors.New("media duration is unknown; run media probe first")
	}
	if seek >= float64(media.DurationSec-1) {
		return nil, fmt.Errorf("preview timestamp must be earlier than %d seconds", media.DurationSec-1)
	}
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ?", media.ID).Updates(map[string]any{
		"generated_artwork_seek_sec": seek,
		"generated_artwork_hash":     "",
		"generated_artwork_status":   GeneratedArtworkStatusRunning,
		"generated_artwork_error":    "",
		"generated_artwork_attempts": 1,
	}).Error; err != nil {
		return nil, err
	}
	media.GeneratedArtworkSeekSec = seek
	media.GeneratedArtworkHash = ""
	media.GeneratedArtworkStatus = GeneratedArtworkStatusRunning
	media.GeneratedArtworkAttempts = 1

	var task *TaskHandle
	if s.tasks != nil {
		task = s.tasks.Start(TaskKindArtwork, "按时间生成作品预览图", TaskUpdate{
			Stage: "extracting", SourcePath: media.Path,
			Message: fmt.Sprintf("正在截取 %.3f 秒画面", seek),
			Metrics: map[string]int64{"timestamp_ms": int64(math.Round(seek * 1000))},
		})
	}
	jobCtx, cancel := context.WithTimeout(ctx, generatedArtworkTimeout)
	poster, backdrop, hash, generateErr := s.generateOne(jobCtx, media)
	cancel()
	updates := map[string]any{}
	if generateErr != nil {
		updates["generated_artwork_status"] = GeneratedArtworkStatusFailed
		updates["generated_artwork_error"] = generateErr.Error()
	} else {
		updates["generated_poster_url"] = poster
		updates["generated_backdrop_url"] = backdrop
		updates["generated_artwork_hash"] = hash
		updates["generated_artwork_status"] = GeneratedArtworkStatusCompleted
		updates["generated_artwork_error"] = ""
	}
	if err := s.repo.DB.WithContext(context.WithoutCancel(ctx)).Model(&model.Media{}).Where("id = ?", media.ID).Updates(updates).Error; err != nil {
		if task != nil {
			task.Finish(err, TaskUpdate{Stage: "failed", Message: "保存预览图状态失败"})
		}
		return nil, err
	}
	if s.cache != nil {
		s.cache.DeletePrefix(context.WithoutCancel(ctx), "media:")
	}
	if task != nil {
		stage, message := "completed", "作品预览图已更新"
		if generateErr != nil {
			stage, message = "failed", "作品预览图生成失败"
		}
		task.Finish(generateErr, TaskUpdate{
			Stage: stage, Message: message,
			Metrics: map[string]int64{"timestamp_ms": int64(math.Round(seek * 1000))},
		})
	}
	if generateErr != nil {
		return nil, generateErr
	}
	return s.repo.Media.FindByID(context.WithoutCancel(ctx), media.ID)
}

func (s *GeneratedArtworkService) queueableMissingQuery(ctx context.Context, libraryID string) *gorm.DB {
	return s.missingQuery(ctx, libraryID).
		Where("media.generated_artwork_status = '' OR media.generated_artwork_status IS NULL OR media.generated_artwork_status = ? OR (media.generated_artwork_status = ? AND COALESCE(media.generated_artwork_attempts, 0) < ?)",
			GeneratedArtworkStatusCanceled, GeneratedArtworkStatusFailed, generatedArtworkMaxAttempts)
}

func (s *GeneratedArtworkService) CancelLibrary(ctx context.Context, libraryID string) (int64, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return 0, errors.New("generated artwork service unavailable")
	}
	libraryID = strings.TrimSpace(libraryID)
	res := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ? AND generated_artwork_status = ?", libraryID, GeneratedArtworkStatusPending).
		Updates(map[string]any{
			"generated_artwork_status": GeneratedArtworkStatusCanceled,
			"generated_artwork_error":  "已取消",
		})
	if res.Error != nil {
		return 0, res.Error
	}
	s.mu.Lock()
	if s.activeLibrary == libraryID && s.activeCancel != nil {
		s.activeCancel()
	}
	s.mu.Unlock()
	s.finishBatchIfIdle(context.WithoutCancel(ctx))
	return res.RowsAffected, nil
}

func (s *GeneratedArtworkService) Status(ctx context.Context, libraryID string) (GeneratedArtworkStatus, error) {
	status := GeneratedArtworkStatus{LibraryID: strings.TrimSpace(libraryID)}
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return status, errors.New("generated artwork service unavailable")
	}
	lib, err := s.repo.Library.FindByID(ctx, status.LibraryID)
	if err != nil {
		return status, err
	}
	if lib == nil {
		return status, gorm.ErrRecordNotFound
	}
	status.Enabled = lib.GenerateArtwork
	type row struct {
		Status string
		Count  int64
	}
	var rows []row
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select("generated_artwork_status AS status, count(*) AS count").
		Where("library_id = ? AND deleted_at IS NULL", status.LibraryID).
		Where("generated_artwork_status <> ''").
		Group("generated_artwork_status").Scan(&rows).Error; err != nil {
		return status, err
	}
	for _, item := range rows {
		switch item.Status {
		case GeneratedArtworkStatusPending:
			status.Pending = item.Count
		case GeneratedArtworkStatusRunning:
			status.Running = item.Count
		case GeneratedArtworkStatusCompleted:
			status.Completed = item.Count
		case GeneratedArtworkStatusFailed:
			status.Failed = item.Count
		case GeneratedArtworkStatusCanceled:
			status.Canceled = item.Count
		}
	}
	return status, nil
}

func (s *GeneratedArtworkService) worker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
		for ctx.Err() == nil {
			media, err := s.claimNext(ctx)
			if err != nil {
				s.logWarn("claim generated artwork job failed", err, "")
				break
			}
			if media == nil {
				s.finishBatchIfIdle(ctx)
				break
			}
			s.process(ctx, media)
		}
	}
}

func (s *GeneratedArtworkService) claimNext(ctx context.Context) (*model.Media, error) {
	var media model.Media
	err := s.pendingQuery(ctx).Order("media.created_at ASC, media.id ASC").First(&media).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	res := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("id = ? AND generated_artwork_status = ?", media.ID, GeneratedArtworkStatusPending).
		Updates(map[string]any{
			"generated_artwork_status":   GeneratedArtworkStatusRunning,
			"generated_artwork_attempts": media.GeneratedArtworkAttempts + 1,
			"generated_artwork_error":    "",
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	media.GeneratedArtworkStatus = GeneratedArtworkStatusRunning
	media.GeneratedArtworkAttempts++
	return &media, nil
}

func (s *GeneratedArtworkService) process(parent context.Context, media *model.Media) {
	jobCtx, cancel := context.WithTimeout(parent, generatedArtworkTimeout)
	s.mu.Lock()
	s.activeCancel = cancel
	s.activeLibrary = media.LibraryID
	s.mu.Unlock()

	s.runMu.Lock()
	poster, backdrop, hash, err := s.generateOne(jobCtx, media)
	s.runMu.Unlock()
	canceled := errors.Is(err, context.Canceled) || errors.Is(jobCtx.Err(), context.Canceled)
	cancel()
	s.mu.Lock()
	s.activeCancel = nil
	s.activeLibrary = ""
	s.mu.Unlock()

	updates := map[string]any{}
	switch {
	case err == nil:
		updates["generated_poster_url"] = poster
		updates["generated_backdrop_url"] = backdrop
		updates["generated_artwork_hash"] = hash
		updates["generated_artwork_status"] = GeneratedArtworkStatusCompleted
		updates["generated_artwork_error"] = ""
		s.recordBatchResult(false, false)
	case canceled:
		updates["generated_artwork_status"] = GeneratedArtworkStatusCanceled
		updates["generated_artwork_error"] = "已取消"
		s.recordBatchResult(false, true)
	case media.GeneratedArtworkAttempts < generatedArtworkMaxAttempts:
		updates["generated_artwork_status"] = GeneratedArtworkStatusPending
		updates["generated_artwork_error"] = err.Error()
	case err != nil:
		updates["generated_artwork_status"] = GeneratedArtworkStatusFailed
		updates["generated_artwork_error"] = err.Error()
		s.recordBatchResult(true, false)
	}
	if dbErr := s.repo.DB.WithContext(context.WithoutCancel(parent)).Model(&model.Media{}).Where("id = ?", media.ID).Updates(updates).Error; dbErr != nil {
		s.logWarn("persist generated artwork result failed", dbErr, media.ID)
		return
	}
	if err != nil && !canceled {
		s.logWarn("generated artwork failed", err, media.ID)
	}
	if s.cache != nil {
		s.cache.DeletePrefix(context.WithoutCancel(parent), "media:")
	}
	if updates["generated_artwork_status"] == GeneratedArtworkStatusPending {
		s.signal()
	}
}

func (s *GeneratedArtworkService) generateOne(ctx context.Context, media *model.Media) (string, string, string, error) {
	if media == nil {
		return "", "", "", ErrMediaNotFound
	}
	current, err := s.repo.Media.FindByID(ctx, media.ID)
	if err != nil || current == nil {
		return "", "", "", err
	}
	lib, err := s.repo.Library.FindByID(ctx, current.LibraryID)
	if err != nil || lib == nil {
		return "", "", "", err
	}
	if !lib.Enabled {
		return "", "", "", context.Canceled
	}
	if !generatedArtworkNeedsGeneration(current) {
		return "", "", "", context.Canceled
	}
	input, headers, err := s.mediaInput(ctx, current)
	if err != nil {
		return "", "", "", err
	}
	hash := generatedArtworkFingerprint(current)
	dir := filepath.Join(s.cfg.App.DataDir, "generated-artwork", current.ID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", "", "", err
	}
	poster := filepath.Join(dir, hash+"-primary.jpg")
	backdrop := filepath.Join(dir, hash+"-backdrop.jpg")
	if generatedArtworkFilesUsable(poster, backdrop) {
		return poster, backdrop, hash, nil
	}
	tmpPoster := filepath.Join(dir, "."+hash+"-primary.tmp.jpg")
	tmpBackdrop := filepath.Join(dir, "."+hash+"-backdrop.tmp.jpg")
	s.removeGeneratedArtworkFile(tmpPoster)
	s.removeGeneratedArtworkFile(tmpBackdrop)
	defer s.removeGeneratedArtworkFile(tmpPoster)
	defer s.removeGeneratedArtworkFile(tmpBackdrop)
	if err := s.run(ctx, input, headers, generatedArtworkSeekForMedia(current), tmpPoster, tmpBackdrop); err != nil {
		return "", "", "", err
	}
	latest, err := s.repo.Media.FindByID(ctx, current.ID)
	if err != nil {
		return "", "", "", err
	}
	if latest == nil {
		return "", "", "", ErrMediaNotFound
	}
	if generatedArtworkFingerprint(latest) != hash {
		return "", "", "", errGeneratedArtworkMetadataChanged
	}
	if !generatedArtworkFilesUsable(tmpPoster, tmpBackdrop) {
		return "", "", "", errors.New("ffmpeg did not produce usable preview images")
	}
	if err := os.Rename(tmpPoster, poster); err != nil {
		return "", "", "", err
	}
	if err := os.Rename(tmpBackdrop, backdrop); err != nil {
		return "", "", "", err
	}
	s.removeOldGeneratedArtwork(dir, poster, backdrop)
	return poster, backdrop, hash, nil
}

func (s *GeneratedArtworkService) mediaInput(ctx context.Context, media *model.Media) (string, map[string]string, error) {
	if typ, ref, ok := parseCloudMediaPlaybackURL(media.STRMURL); ok {
		if s.storage == nil {
			return "", nil, errors.New("cloud storage service unavailable")
		}
		link, err := s.storage.CloudResolve(ctx, typ, ref, cloudMediaInternalUserAgent)
		if err != nil {
			return "", nil, fmt.Errorf("resolve cloud media: %w", err)
		}
		if link == nil || strings.TrimSpace(link.URL) == "" {
			return "", nil, errors.New("cloud media resolved to an empty URL")
		}
		return link.URL, cloudMediaInternalHeaders(link.Headers), nil
	}
	path := filepath.Clean(strings.TrimSpace(media.Path))
	if path == "" || strings.HasPrefix(strings.ToLower(path), "cloud:") {
		return "", nil, errors.New("media has no resolvable source")
	}
	if stat, err := os.Stat(path); err != nil || stat.IsDir() {
		if err == nil {
			err = errors.New("media source is a directory")
		}
		return "", nil, err
	}
	return path, nil, nil
}

func (s *GeneratedArtworkService) runFFmpeg(ctx context.Context, input string, headers map[string]string, seek float64, poster, backdrop string) error {
	bin, err := resolveLocalExecutable(s.cfg.App.FFmpegPath, "ffmpeg")
	if err != nil {
		return err
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-threads", "1", "-filter_threads", "1", "-filter_complex_threads", "1",
		"-ss", strconv.FormatFloat(seek, 'f', 3, 64),
	}
	if headerText := ffmpegHeaderText(headers); headerText != "" {
		args = append(args, "-headers", headerText)
	}
	args = append(args,
		"-i", input,
		"-an", "-sn", "-dn",
		"-filter_complex", "[0:v]split=2[poster_src][backdrop_src];[poster_src]scale=600:900:force_original_aspect_ratio=increase,crop=600:900[poster];[backdrop_src]scale=1280:720:force_original_aspect_ratio=increase,crop=1280:720[backdrop]",
		"-map", "[poster]", "-frames:v", "1", "-q:v", "3", "-y", poster,
		"-map", "[backdrop]", "-frames:v", "1", "-q:v", "3", "-y", backdrop,
	)
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput() // #nosec G204 -- executable is resolved and all arguments are internally constructed.
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		message = sanitizeGeneratedArtworkError(message, input)
		return fmt.Errorf("ffmpeg preview extraction failed: %s", message)
	}
	return nil
}

func sanitizeGeneratedArtworkError(message, input string) string {
	message = strings.TrimSpace(message)
	if input = strings.TrimSpace(input); input != "" {
		message = strings.ReplaceAll(message, input, "[media-source]")
	}
	const maxLength = 1000
	if len(message) > maxLength {
		message = message[:maxLength]
	}
	return message
}

func (s *GeneratedArtworkService) missingQuery(ctx context.Context, libraryID string) *gorm.DB {
	return s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("media.library_id = ? AND media.deleted_at IS NULL", libraryID).
		Where("media.season_num = 0 AND media.episode_num = 0").
		Where("COALESCE(TRIM(media.poster_url), '') = '' AND COALESCE(TRIM(media.backdrop_url), '') = ''").
		Where("COALESCE(TRIM(media.generated_poster_url), '') = '' OR COALESCE(TRIM(media.generated_backdrop_url), '') = ''")
}

func generatedArtworkNeedsGeneration(media *model.Media) bool {
	if media == nil || media.SeasonNum != 0 || media.EpisodeNum != 0 ||
		strings.TrimSpace(media.PosterURL) != "" || strings.TrimSpace(media.BackdropURL) != "" {
		return false
	}
	if strings.TrimSpace(media.GeneratedPosterURL) == "" || strings.TrimSpace(media.GeneratedBackdropURL) == "" {
		return true
	}
	return strings.TrimSpace(media.GeneratedArtworkHash) != generatedArtworkFingerprint(media)
}

func generatedArtworkFingerprint(media *model.Media) string {
	if media == nil {
		return "unknown"
	}
	parts := []string{
		strings.TrimSpace(media.Path),
		strconv.FormatInt(media.SizeBytes, 10),
		strconv.Itoa(media.DurationSec),
	}
	if media.GeneratedArtworkSeekSec > 0 {
		parts = append(parts, "seek="+strconv.FormatFloat(media.GeneratedArtworkSeekSec, 'f', 3, 64))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:24]
}

func generatedArtworkSeekForMedia(media *model.Media) float64 {
	if media != nil && media.GeneratedArtworkSeekSec > 0 {
		return media.GeneratedArtworkSeekSec
	}
	if media == nil {
		return generatedArtworkSeek(0)
	}
	return generatedArtworkSeek(media.DurationSec)
}

func generatedArtworkSeek(durationSec int) float64 {
	if durationSec <= 0 {
		return 10
	}
	seek := float64(durationSec) * 0.10
	if durationSec >= 120 && seek < 60 {
		seek = 60
	}
	if seek > 300 {
		seek = 300
	}
	if seek >= float64(durationSec-5) {
		seek = float64(durationSec) / 2
	}
	if seek < 0 {
		return 0
	}
	return seek
}

func generatedArtworkFilesUsable(paths ...string) bool {
	for _, path := range paths {
		stat, err := os.Stat(path)
		if err != nil || stat.IsDir() || stat.Size() < 1024 {
			return false
		}
	}
	return true
}

func (s *GeneratedArtworkService) removeOldGeneratedArtwork(dir string, keep ...string) {
	allowed := make(map[string]struct{}, len(keep))
	for _, path := range keep {
		allowed[filepath.Clean(path)] = struct{}{}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			s.logWarn("read generated artwork directory failed", err, "")
		}
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if _, ok := allowed[filepath.Clean(path)]; ok {
			continue
		}
		if err := os.Remove(path); err != nil {
			s.logWarn("remove stale generated artwork failed", err, "")
		}
	}
}

func (s *GeneratedArtworkService) removeGeneratedArtworkFile(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logWarn("remove generated artwork file failed", err, "")
	}
}

func (s *GeneratedArtworkService) pendingQuery(ctx context.Context) *gorm.DB {
	return s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Joins("JOIN libraries ON libraries.id = media.library_id AND libraries.deleted_at IS NULL").
		Where("media.deleted_at IS NULL AND libraries.enabled = ? AND libraries.generate_artwork = ?", true, true).
		Where("media.season_num = 0 AND media.episode_num = 0").
		Where("COALESCE(TRIM(media.poster_url), '') = '' AND COALESCE(TRIM(media.backdrop_url), '') = ''").
		Where("media.generated_artwork_status = ? AND COALESCE(media.generated_artwork_attempts, 0) < ?", GeneratedArtworkStatusPending, generatedArtworkMaxAttempts)
}

func (s *GeneratedArtworkService) pendingOrRunningQuery(ctx context.Context) *gorm.DB {
	return s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Joins("JOIN libraries ON libraries.id = media.library_id AND libraries.deleted_at IS NULL").
		Where("media.deleted_at IS NULL AND libraries.enabled = ? AND libraries.generate_artwork = ?", true, true).
		Where("media.generated_artwork_status IN ?", []string{GeneratedArtworkStatusPending, GeneratedArtworkStatusRunning})
}

func (s *GeneratedArtworkService) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *GeneratedArtworkService) startBatchTask(total int64, message string) {
	if total <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batchTotal += total
	if s.batchTask == nil && s.tasks != nil {
		s.batchTask = s.tasks.Start(TaskKindArtwork, "生成缺图预览图", TaskUpdate{
			Stage:   "queued",
			Message: message,
			Metrics: s.batchMetricsLocked(),
		})
	} else if s.batchTask != nil {
		s.batchTask.Update(TaskUpdate{Stage: "queued", Message: message, Metrics: s.batchMetricsLocked()})
	}
}

func (s *GeneratedArtworkService) recordBatchResult(failed, skipped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if failed {
		s.batchFailed++
	} else if skipped {
		s.batchSkipped++
	} else {
		s.batchDone++
	}
	if s.batchTask != nil {
		s.batchTask.Update(TaskUpdate{Stage: "generating", Message: "正在生成缺图预览", Metrics: s.batchMetricsLocked()})
	}
}

func (s *GeneratedArtworkService) finishBatchIfIdle(ctx context.Context) {
	var pending int64
	if err := s.pendingOrRunningQuery(ctx).Count(&pending).Error; err != nil {
		s.logWarn("count active generated artwork jobs failed", err, "")
		return
	}
	if pending > 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.batchTask == nil {
		return
	}
	metrics := s.batchMetricsLocked()
	if s.batchFailed > 0 {
		s.batchTask.Finish(errors.New("部分预览图生成失败"), TaskUpdate{Stage: "completed", Message: "缺图预览任务结束", Metrics: metrics})
	} else if s.batchSkipped > 0 && s.batchDone == 0 {
		s.batchTask.Cancel(TaskUpdate{Stage: "canceled", Message: "缺图预览任务已取消", Metrics: metrics})
	} else {
		s.batchTask.Finish(nil, TaskUpdate{Stage: "completed", Message: "缺图预览任务完成", Metrics: metrics})
	}
	s.batchTask = nil
	s.batchTotal = 0
	s.batchDone = 0
	s.batchFailed = 0
	s.batchSkipped = 0
}

func (s *GeneratedArtworkService) batchMetricsLocked() map[string]int64 {
	return map[string]int64{
		"queued":    s.batchTotal,
		"generated": s.batchDone,
		"failed":    s.batchFailed,
		"skipped":   s.batchSkipped,
	}
}

func (s *GeneratedArtworkService) logWarn(message string, err error, mediaID string) {
	if s.log == nil {
		return
	}
	fields := []zap.Field{zap.Error(err)}
	if mediaID != "" {
		fields = append(fields, zap.String("media_id", mediaID))
	}
	s.log.Warn(message, fields...)
}
