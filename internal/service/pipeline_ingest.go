package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	PipelineIngestStatusAccepted       = "accepted"
	PipelineIngestStatusConverging     = "converging"
	PipelineIngestStatusRunning        = "running"
	PipelineIngestStatusCompleted      = "completed"
	PipelineIngestStatusFailed         = "failed"
	PipelineIngestStatusNeedsAttention = "needs_attention"
	pipelineIngestStableWindow         = 30 * time.Second
	pipelineIngestMaxConvergence       = 10 * time.Minute
)

type PipelineIngestService struct {
	log         *zap.Logger
	repos       *repository.Container
	scanner     *ScannerService
	maintenance *PipelineMaintenanceService
	subtitle    *SubtitleService
	tasks       *TaskTrackerService

	mu             sync.Mutex
	jobs           map[string]*PipelineIngestJob
	recent         []string
	executing      map[string]bool
	enhancing      map[string]bool
	now            func() time.Time
	wait           func(context.Context, time.Duration) error
	stableWindow   time.Duration
	maxConvergence time.Duration
}

func (s *PipelineIngestService) SetSubtitleService(subtitle *SubtitleService) {
	if s != nil {
		s.subtitle = subtitle
	}
}

func NewPipelineIngestService(log *zap.Logger, repos *repository.Container, scanner *ScannerService, maintenance *PipelineMaintenanceService, tasks *TaskTrackerService) *PipelineIngestService {
	return &PipelineIngestService{
		log:            log,
		repos:          repos,
		scanner:        scanner,
		maintenance:    maintenance,
		tasks:          tasks,
		jobs:           make(map[string]*PipelineIngestJob),
		executing:      make(map[string]bool),
		enhancing:      make(map[string]bool),
		now:            time.Now,
		wait:           waitPipelineIngestDuration,
		stableWindow:   pipelineIngestStableWindow,
		maxConvergence: pipelineIngestMaxConvergence,
	}
}

type PipelineIngestRequest struct {
	PipelineMaintenanceTarget
	IdempotencyKey            string   `json:"idempotency_key,omitempty"`
	Title                     string   `json:"title,omitempty"`
	Queries                   []string `json:"queries,omitempty"`
	TargetOpenListPaths       []string `json:"target_openlist_paths,omitempty"`
	RequireTargetPath         bool     `json:"require_target_path,omitempty"`
	PruneDeletedOpenListPaths []string `json:"prune_deleted_openlist_paths,omitempty"`
	FilterSmallVideoMaxBytes  int64    `json:"filter_small_video_max_bytes,omitempty"`
	FilterAdultExtras         bool     `json:"filter_adult_extras,omitempty"`
	Scan                      bool     `json:"scan"`
	RepairMovieExtras         bool     `json:"repair_movie_extras,omitempty"`
	RepairEpisodeVisibility   bool     `json:"repair_episode_visibility,omitempty"`
	ForceSeasonNumber         int      `json:"force_season_number,omitempty"`
	RequireStableTree         bool     `json:"require_stable_tree,omitempty"`
	TargetParentsVerified     bool     `json:"target_parents_verified,omitempty"`
}

type PipelineIngestJob struct {
	ID         string                `json:"id"`
	Status     string                `json:"status"`
	Stage      string                `json:"stage,omitempty"`
	Message    string                `json:"message,omitempty"`
	Error      string                `json:"error,omitempty"`
	Request    PipelineIngestRequest `json:"request"`
	Result     PipelineIngestResult  `json:"result"`
	StartedAt  time.Time             `json:"started_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
}

type PipelineIngestResult struct {
	DeletedMediaPrune   *PipelineDeletedMediaPruneResult `json:"deleted_media_prune,omitempty"`
	Scan                *PipelineIngestScanResult        `json:"scan,omitempty"`
	Media               *PipelineIngestMediaResult       `json:"media,omitempty"`
	MediaItems          []PipelineIngestMediaResult      `json:"media_items,omitempty"`
	IgnoredMedia        []PipelineIngestIgnoredMedia     `json:"ignored_media,omitempty"`
	CloudSubtitles      *CloudSubtitleMaterializeResult  `json:"cloud_subtitles,omitempty"`
	CloudSubtitleStatus string                           `json:"cloud_subtitle_status,omitempty"`
	CloudSubtitleError  string                           `json:"cloud_subtitle_error,omitempty"`
	MovieExtras         *PipelineRepairResult            `json:"movie_extras,omitempty"`
	EpisodeVisibility   *PipelineRepairResult            `json:"episode_visibility,omitempty"`
	Convergence         *PipelineIngestConvergenceResult `json:"convergence,omitempty"`
}

type PipelineIngestTreeManifest struct {
	EntryCount     int    `json:"entry_count"`
	DirectoryCount int    `json:"directory_count"`
	FileCount      int    `json:"file_count"`
	TotalFileSize  int64  `json:"total_file_size"`
	Fingerprint    string `json:"fingerprint"`
}

type PipelineIngestConvergenceResult struct {
	Status           string                     `json:"status"`
	Attempt          int                        `json:"attempt"`
	StableForSeconds int                        `json:"stable_for_seconds"`
	MaxWaitSeconds   int                        `json:"max_wait_seconds"`
	Manifest         PipelineIngestTreeManifest `json:"manifest"`
	Changed          bool                       `json:"changed"`
	ErrorCount       int                        `json:"error_count"`
	Errors           []string                   `json:"errors,omitempty"`
	StartedAt        time.Time                  `json:"started_at"`
	StableSince      *time.Time                 `json:"stable_since,omitempty"`
	ObservedAt       time.Time                  `json:"observed_at"`
	NextProbeAt      *time.Time                 `json:"next_probe_at,omitempty"`
	CompletedAt      *time.Time                 `json:"completed_at,omitempty"`
}

type PipelineIngestScanResult struct {
	LibraryID  string `json:"library_id,omitempty"`
	Visited    int    `json:"visited"`
	Added      int    `json:"added"`
	Updated    int    `json:"updated"`
	Skipped    int    `json:"skipped"`
	Probed     int    `json:"probed"`
	Removed    int64  `json:"removed"`
	ErrorCount int    `json:"error_count"`
}

type PipelineIngestMediaResult struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Path      string `json:"path,omitempty"`
	MatchMode string `json:"match_mode,omitempty"`
	MatchPath string `json:"match_path,omitempty"`
}

func (s *PipelineIngestService) Start(ctx context.Context, req PipelineIngestRequest) (PipelineIngestJob, error) {
	if s == nil || s.repos == nil || s.repos.DB == nil {
		return PipelineIngestJob{}, errors.New("pipeline ingest service unavailable")
	}
	if s.maintenance == nil {
		return PipelineIngestJob{}, errors.New("pipeline maintenance service unavailable")
	}
	req.Category = normalizePipelineCategory(req.Category)
	if strings.TrimSpace(req.Category) == "" {
		return PipelineIngestJob{}, errors.New("category is required")
	}
	if strings.TrimSpace(req.LibraryID) == "" || strings.TrimSpace(req.RootID) == "" {
		return PipelineIngestJob{}, errors.New("library_id and root_id are required")
	}
	if strings.TrimSpace(req.RootOpenListPath) == "" {
		return PipelineIngestJob{}, errors.New("root_openlist_path is required")
	}
	if req.FilterSmallVideoMaxBytes < 0 {
		return PipelineIngestJob{}, errors.New("filter_small_video_max_bytes must not be negative")
	}
	if req.ForceSeasonNumber < 0 || req.ForceSeasonNumber > 99 {
		return PipelineIngestJob{}, errors.New("force_season_number must be between 0 and 99")
	}
	req.Queries = pipelineCompactStrings(append(req.Queries, req.Title))
	req.TargetOpenListPaths = pipelineCompactOpenListPaths(req.TargetOpenListPaths)
	req.PruneDeletedOpenListPaths = pipelineCompactOpenListPaths(req.PruneDeletedOpenListPaths)

	jobID := uuid.NewString()
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = "job:" + jobID
	}
	if existing, found, err := s.findJobByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
		return PipelineIngestJob{}, err
	} else if found {
		return s.reuseJob(existing, req)
	}

	now := s.currentTime()
	status := PipelineIngestStatusRunning
	if req.RequireStableTree {
		status = PipelineIngestStatusAccepted
	}
	job := PipelineIngestJob{
		ID:        jobID,
		Status:    status,
		Stage:     "queued",
		Message:   "queued",
		Request:   req,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := s.storeJob(ctx, job); err != nil {
		if existing, found, lookupErr := s.findJobByIdempotencyKey(ctx, req.IdempotencyKey); lookupErr == nil && found {
			return s.reuseJob(existing, req)
		}
		return PipelineIngestJob{}, err
	}
	s.scheduleJob(job.ID)
	return s.Get(job.ID)
}

func (s *PipelineIngestService) Get(id string) (PipelineIngestJob, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PipelineIngestJob{}, errors.New("job id is required")
	}
	if job, ok := s.cachedJob(id); ok {
		return job, nil
	}
	job, found, err := s.loadJobByID(context.Background(), id)
	if err != nil {
		return PipelineIngestJob{}, err
	}
	if !found {
		return PipelineIngestJob{}, errors.New("pipeline ingest job not found")
	}
	s.cacheJob(job)
	return clonePipelineIngestJob(job), nil
}

func (s *PipelineIngestService) reuseJob(existing PipelineIngestJob, req PipelineIngestRequest) (PipelineIngestJob, error) {
	if !pipelineIngestRequestsEqual(existing.Request, req) {
		return PipelineIngestJob{}, errors.New("pipeline ingest idempotency key already belongs to a different request")
	}
	s.cacheJob(existing)
	if pipelineIngestStatusActive(existing.Status) {
		s.scheduleJob(existing.ID)
	}
	return clonePipelineIngestJob(existing), nil
}

func (s *PipelineIngestService) run(ctx context.Context, id string) {
	defer s.clearExecuting(id)
	job, err := s.Get(id)
	if err != nil {
		if s.log != nil {
			s.log.Error("pipeline ingest job load failed", zap.String("job_id", id), zap.Error(err))
		}
		return
	}
	var task *TaskHandle
	if s.tasks != nil {
		task = s.tasks.Start(TaskKindScan, "pipeline ingest", TaskUpdate{
			Stage:      "queued",
			SourcePath: job.Request.RootOpenListPath,
			Message:    "pipeline ingest queued",
		})
	}
	finishTask := func(err error, stage, message string, metrics map[string]int64) {
		if task != nil {
			task.Finish(err, TaskUpdate{Stage: stage, Message: message, Metrics: metrics})
		}
	}

	if err := s.runJob(ctx, id, task); err != nil {
		var needsAttention *pipelineIngestNeedsAttentionError
		if errors.As(err, &needsAttention) {
			if persistErr := s.markJobNeedsAttention(id, needsAttention.Error()); persistErr != nil && s.log != nil {
				s.log.Error("pipeline ingest needs-attention persistence failed", zap.String("job_id", id), zap.Error(persistErr))
			}
			finishTask(nil, PipelineIngestStatusNeedsAttention, needsAttention.Error(), nil)
			return
		}
		if persistErr := s.failJob(id, err); persistErr != nil && s.log != nil {
			s.log.Error("pipeline ingest failure persistence failed", zap.String("job_id", id), zap.Error(persistErr))
		}
		finishTask(err, "failed", "pipeline ingest failed", nil)
		return
	}
	if err := s.completeJob(id); err != nil {
		if persistErr := s.failJob(id, err); persistErr != nil && s.log != nil {
			s.log.Error("pipeline ingest completion persistence failed", zap.String("job_id", id), zap.Error(persistErr))
		}
		finishTask(err, "failed", "pipeline ingest persistence failed", nil)
		return
	}
	finishTask(nil, "completed", "pipeline ingest completed", nil)
	s.scheduleCloudSubtitleEnhancement(id)
}

func (s *PipelineIngestService) scheduleCloudSubtitleEnhancement(id string) bool {
	if s == nil || s.subtitle == nil {
		return false
	}
	s.mu.Lock()
	if s.enhancing[id] {
		s.mu.Unlock()
		return false
	}
	s.enhancing[id] = true
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.enhancing, id)
			s.mu.Unlock()
		}()
		s.runCloudSubtitleEnhancement(id)
	}()
	return true
}

func (s *PipelineIngestService) runCloudSubtitleEnhancement(id string) {
	job, err := s.Get(id)
	if err != nil || job.Status != PipelineIngestStatusCompleted || job.Result.Media == nil {
		return
	}
	status := strings.TrimSpace(job.Result.CloudSubtitleStatus)
	if status != "pending" && status != "running" {
		return
	}
	if err := s.updateJobResult(id, func(result *PipelineIngestResult) {
		result.CloudSubtitleStatus = "running"
		result.CloudSubtitleError = ""
	}); err != nil {
		if s.log != nil {
			s.log.Warn("pipeline cloud subtitle status update failed", zap.String("job_id", id), zap.Error(err))
		}
		return
	}
	result, enhanceErr := s.subtitle.EnsureCloudSubtitles(context.Background(), job.Result.Media.ID)
	persistErr := s.updateJobResult(id, func(out *PipelineIngestResult) {
		out.CloudSubtitles = &result
		out.CloudSubtitleError = ""
		if enhanceErr != nil {
			out.CloudSubtitleStatus = "failed"
			out.CloudSubtitleError = enhanceErr.Error()
			return
		}
		out.CloudSubtitleStatus = result.Status
	})
	if persistErr != nil && s.log != nil {
		s.log.Warn("pipeline cloud subtitle result persistence failed", zap.String("job_id", id), zap.Error(persistErr))
	}
	if enhanceErr != nil && s.log != nil {
		s.log.Warn("pipeline cloud subtitle enhancement failed", zap.String("job_id", id), zap.Error(enhanceErr))
	}
}

func (s *PipelineIngestService) runJob(ctx context.Context, id string, task *TaskHandle) error {
	job, err := s.Get(id)
	if err != nil {
		return err
	}
	req := job.Request
	target, err := s.maintenance.resolveTarget(ctx, req.PipelineMaintenanceTarget)
	if err != nil {
		return err
	}
	if len(req.PruneDeletedOpenListPaths) > 0 {
		if err := s.updateJob(id, "prune_deleted", "pruning stale deleted media", nil); err != nil {
			return err
		}
		if task != nil {
			task.Update(TaskUpdate{Stage: "prune_deleted", Message: "pruning stale deleted media"})
		}
		result, err := s.maintenance.PruneDeletedMedia(ctx, req.PipelineMaintenanceTarget, req.PruneDeletedOpenListPaths)
		if err != nil {
			return err
		}
		if err := s.updateJobResult(id, func(resultOut *PipelineIngestResult) {
			resultOut.DeletedMediaPrune = &result
		}); err != nil {
			return err
		}
	}
	if req.Scan {
		if s.scanner == nil {
			return errors.New("scanner service unavailable")
		}
		scanMessage := "scanning library root"
		if len(req.TargetOpenListPaths) > 0 {
			scanMessage = "scanning target path"
		}
		if err := s.updateJob(id, "scan", scanMessage, nil); err != nil {
			return err
		}
		if task != nil {
			task.Update(TaskUpdate{Stage: "scan", Message: scanMessage})
		}
		var scanResult *ScanResult
		var ignoredMedia []PipelineIngestIgnoredMedia
		var scanErr error
		if req.RequireStableTree {
			scanResult, ignoredMedia, scanErr = s.scanForPipelineIngestConverged(ctx, id, target, req, task)
		} else {
			finish, ok := s.scanner.TryBeginLocalScan("pipeline-ingest:" + target.LibraryID + ":" + target.RootID)
			if !ok {
				return errors.New("library root scan already running")
			}
			scanResult, ignoredMedia, scanErr = s.scanForPipelineIngest(ctx, target, req)
			finish()
		}
		if scanResult != nil {
			summary := pipelineIngestScanSummary(scanResult)
			if err := s.updateJobResult(id, func(resultOut *PipelineIngestResult) {
				resultOut.Scan = &summary
				resultOut.IgnoredMedia = ignoredMedia
			}); err != nil {
				return err
			}
		}
		if scanErr != nil {
			return scanErr
		}
	}

	if err := s.updateJob(id, "find_media", "finding ingested media", nil); err != nil {
		return err
	}
	media, mediaRows, matchMode, matchPath, err := s.findMedia(ctx, target, req)
	if err != nil {
		return err
	}
	mediaResult := PipelineIngestMediaResult{
		ID:        media.ID,
		Title:     pipelineMediaDisplayTitle(media),
		Path:      media.Path,
		MatchMode: matchMode,
		MatchPath: matchPath,
	}
	if err := s.updateJobResult(id, func(resultOut *PipelineIngestResult) {
		resultOut.Media = &mediaResult
		resultOut.MediaItems = pipelineIngestMediaResults(mediaRows, matchMode, req.TargetOpenListPaths)
		if s.subtitle != nil && resultOut.CloudSubtitleStatus == "" {
			resultOut.CloudSubtitleStatus = "pending"
		}
	}); err != nil {
		return err
	}

	if req.RepairMovieExtras {
		if err := s.updateJob(id, "repair_movie_extras", "repairing movie extras", nil); err != nil {
			return err
		}
		result, err := s.maintenance.RepairMovieExtras(ctx, media.ID, req.PipelineMaintenanceTarget)
		if err != nil {
			return err
		}
		if err := s.updateJobResult(id, func(resultOut *PipelineIngestResult) {
			resultOut.MovieExtras = &result
		}); err != nil {
			return err
		}
	}
	if req.RepairEpisodeVisibility {
		if err := s.updateJob(id, "repair_episode_visibility", "repairing episode visibility", nil); err != nil {
			return err
		}
		result, err := s.maintenance.RepairEpisodeVisibility(ctx, media.ID, req.PipelineMaintenanceTarget)
		if err != nil {
			return err
		}
		if err := s.updateJobResult(id, func(resultOut *PipelineIngestResult) {
			resultOut.EpisodeVisibility = &result
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *PipelineIngestService) scanForPipelineIngest(ctx context.Context, target pipelineResolvedTarget, req PipelineIngestRequest) (*ScanResult, []PipelineIngestIgnoredMedia, error) {
	res, ignored, _, _, err := s.scanForPipelineIngestWithOptions(ctx, target, req, cloudTargetScanOptions{refreshTargetParents: true})
	return res, ignored, err
}

func (s *PipelineIngestService) scanForPipelineIngestWithOptions(ctx context.Context, target pipelineResolvedTarget, req PipelineIngestRequest, options cloudTargetScanOptions) (*ScanResult, []PipelineIngestIgnoredMedia, cloudTreeManifest, bool, error) {
	if len(req.TargetOpenListPaths) > 0 {
		res, ignored, manifest, handled, err := s.scanner.scanLibraryRootOpenListTargetsWithOptions(
			ctx,
			target.LibraryID,
			target.RootID,
			req.TargetOpenListPaths,
			false,
			pipelineIngestCloudCandidateFilter(req),
			req.ForceSeasonNumber,
			options,
		)
		if handled || err != nil {
			return res, pipelineIngestIgnoredMediaResults(ignored), manifest, handled, err
		}
		return nil, nil, cloudTreeManifest{}, false, errors.New("target_openlist_paths were provided but could not be handled by the OpenList target scanner")
	}
	res, err := s.scanner.ScanLibraryRoot(ctx, target.LibraryID, target.RootID)
	return res, nil, cloudTreeManifest{}, true, err
}

func (s *PipelineIngestService) findMedia(ctx context.Context, target pipelineResolvedTarget, req PipelineIngestRequest) (model.Media, []model.Media, string, string, error) {
	if len(req.TargetOpenListPaths) > 0 {
		rows, err := s.findMediaByOpenListPaths(ctx, target, req.TargetOpenListPaths)
		if err != nil {
			return model.Media{}, nil, "", "", err
		}
		if len(rows) > 0 {
			row := choosePipelineIngestMedia(rows, target.RootOpenListPath)
			return row, rows, "path", pipelineMatchedOpenListPath(row.Path, req.TargetOpenListPaths), nil
		}
		if req.RequireTargetPath {
			return model.Media{}, nil, "", "", errors.New("MediaStationGo media not found after target scan")
		}
	}
	for _, query := range req.Queries {
		rows, err := s.findMediaByQuery(ctx, target, query)
		if err != nil {
			return model.Media{}, nil, "", "", err
		}
		if len(rows) > 0 {
			row := choosePipelineIngestMedia(rows, target.RootOpenListPath)
			return row, rows, "query", "", nil
		}
	}
	return model.Media{}, nil, "", "", errors.New("MediaStationGo media not found after scan")
}

type PipelineIngestIgnoredMedia struct {
	OpenListPath string `json:"openlist_path"`
	HidePath     string `json:"hide_path"`
	HidePattern  string `json:"hide_pattern"`
	Reason       string `json:"reason"`
	SizeBytes    int64  `json:"size_bytes"`
}

func (s *PipelineIngestService) findMediaByOpenListPaths(ctx context.Context, target pipelineResolvedTarget, openListPaths []string) ([]model.Media, error) {
	cloudPaths := pipelineCompactCloudPaths(openListPaths)
	if len(cloudPaths) == 0 {
		return nil, nil
	}
	query := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ? AND library_root_id = ?", target.LibraryID, target.RootID)
	parts := make([]string, 0, len(cloudPaths))
	args := make([]any, 0, len(cloudPaths)*2)
	for _, cloudPath := range cloudPaths {
		parts = append(parts, "(path = ? OR path LIKE ?)")
		args = append(args, cloudPath, strings.TrimRight(cloudPath, "/")+"/%")
	}
	var rows []model.Media
	err := query.Where("("+strings.Join(parts, " OR ")+")", args...).
		Order("path ASC, id ASC").
		Find(&rows).Error
	return rows, err
}

func pipelineIngestMediaResults(rows []model.Media, matchMode string, openListPaths []string) []PipelineIngestMediaResult {
	items := make([]PipelineIngestMediaResult, 0, len(rows))
	for _, row := range rows {
		items = append(items, PipelineIngestMediaResult{
			ID:        row.ID,
			Title:     pipelineMediaDisplayTitle(row),
			Path:      row.Path,
			MatchMode: matchMode,
			MatchPath: pipelineMatchedOpenListPath(row.Path, openListPaths),
		})
	}
	return items
}

func (s *PipelineIngestService) findMediaByQuery(ctx context.Context, target pipelineResolvedTarget, query string) ([]model.Media, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	like := "%" + strings.ToLower(query) + "%"
	var rows []model.Media
	err := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ? AND library_root_id = ?", target.LibraryID, target.RootID).
		Where("(LOWER(COALESCE(title, '')) LIKE ? OR LOWER(COALESCE(original_name, '')) LIKE ? OR LOWER(COALESCE(path, '')) LIKE ? OR LOWER(COALESCE(relative_path, '')) LIKE ?)", like, like, like, like).
		Order("updated_at DESC, created_at DESC").
		Limit(50).
		Find(&rows).Error
	return rows, err
}

func choosePipelineIngestMedia(rows []model.Media, rootOpenListPath string) model.Media {
	if len(rows) == 0 {
		return model.Media{}
	}
	candidates := append([]model.Media(nil), rows...)
	sort.SliceStable(candidates, func(i, j int) bool {
		iExtra := pipelineMovieMediaRowLooksLikeExtra(candidates[i], rootOpenListPath)
		jExtra := pipelineMovieMediaRowLooksLikeExtra(candidates[j], rootOpenListPath)
		if iExtra != jExtra {
			return !iExtra
		}
		if candidates[i].SizeBytes != candidates[j].SizeBytes {
			return candidates[i].SizeBytes > candidates[j].SizeBytes
		}
		if !candidates[i].UpdatedAt.Equal(candidates[j].UpdatedAt) {
			return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt)
		}
		return candidates[i].ID > candidates[j].ID
	})
	return candidates[0]
}

func (s *PipelineIngestService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func clonePipelineIngestJob(job PipelineIngestJob) PipelineIngestJob {
	job.Request.Queries = append([]string(nil), job.Request.Queries...)
	job.Request.TargetOpenListPaths = append([]string(nil), job.Request.TargetOpenListPaths...)
	job.Request.PruneDeletedOpenListPaths = append([]string(nil), job.Request.PruneDeletedOpenListPaths...)
	if job.Result.DeletedMediaPrune != nil {
		cloned := *job.Result.DeletedMediaPrune
		cloned.MediaIDs = append([]string(nil), cloned.MediaIDs...)
		job.Result.DeletedMediaPrune = &cloned
	}
	if job.Result.Scan != nil {
		cloned := *job.Result.Scan
		job.Result.Scan = &cloned
	}
	if job.Result.Media != nil {
		cloned := *job.Result.Media
		job.Result.Media = &cloned
	}
	job.Result.MediaItems = append([]PipelineIngestMediaResult(nil), job.Result.MediaItems...)
	job.Result.IgnoredMedia = append([]PipelineIngestIgnoredMedia(nil), job.Result.IgnoredMedia...)
	if job.Result.CloudSubtitles != nil {
		cloned := *job.Result.CloudSubtitles
		job.Result.CloudSubtitles = &cloned
	}
	if job.Result.MovieExtras != nil {
		cloned := *job.Result.MovieExtras
		cloned.OpenListHidePatterns = append([]string(nil), cloned.OpenListHidePatterns...)
		job.Result.MovieExtras = &cloned
	}
	if job.Result.EpisodeVisibility != nil {
		cloned := *job.Result.EpisodeVisibility
		cloned.OpenListHidePatterns = append([]string(nil), cloned.OpenListHidePatterns...)
		job.Result.EpisodeVisibility = &cloned
	}
	if job.Result.Convergence != nil {
		cloned := *job.Result.Convergence
		cloned.Errors = append([]string(nil), cloned.Errors...)
		cloned.StableSince = cloneTimePointer(cloned.StableSince)
		cloned.NextProbeAt = cloneTimePointer(cloned.NextProbeAt)
		cloned.CompletedAt = cloneTimePointer(cloned.CompletedAt)
		job.Result.Convergence = &cloned
	}
	if job.FinishedAt != nil {
		finishedAt := *job.FinishedAt
		job.FinishedAt = &finishedAt
	}
	return job
}

func (job PipelineIngestJob) ptr() *PipelineIngestJob {
	out := job
	return &out
}

func pipelineIngestScanSummary(res *ScanResult) PipelineIngestScanResult {
	if res == nil {
		return PipelineIngestScanResult{}
	}
	return PipelineIngestScanResult{
		LibraryID:  res.LibraryID,
		Visited:    res.Visited,
		Added:      res.Added,
		Updated:    res.Updated,
		Skipped:    res.Skipped,
		Probed:     res.Probed,
		Removed:    res.Removed,
		ErrorCount: res.ErrorCount,
	}
}

func pipelineCompactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func pipelineCompactOpenListPaths(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = pipelineNormalizeOpenListPath(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func pipelineCompactCloudPaths(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		cloudPath := pipelineOpenListPathToCloudPath(value)
		if cloudPath == pipelineOpenListCloudPrefix || seen[cloudPath] {
			continue
		}
		seen[cloudPath] = true
		out = append(out, cloudPath)
	}
	return out
}

func pipelineMatchedOpenListPath(mediaPath string, candidates []string) string {
	openListPath := pipelineCloudPathToOpenListPath(mediaPath)
	for _, candidate := range candidates {
		if pipelinePathIsSameOrChild(openListPath, candidate) {
			return candidate
		}
	}
	return ""
}

func pipelineMediaDisplayTitle(media model.Media) string {
	for _, value := range []string{media.Title, media.OriginalName, media.EpisodeTitle, media.Path} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return media.ID
}
