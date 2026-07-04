package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type OrganizeScope string

const (
	OrganizeScopeDirectory OrganizeScope = "directory"
	OrganizeScopeMedia     OrganizeScope = "media"
	OrganizeScopeLibrary   OrganizeScope = "library"
)

type OrganizeTrigger string

const (
	OrganizeTriggerManual    OrganizeTrigger = "manual"
	OrganizeTriggerScheduled OrganizeTrigger = "scheduled"
	OrganizeTriggerDownload  OrganizeTrigger = "download"
)

// OrganizePipelineRequest is the single service-facing entry point for every
// organize → rename → scan → scrape ingest workflow. Handlers, scheduler jobs
// and download-complete hooks should only decide the trigger/scope/options;
// the pipeline owns the execution order and task reporting.
type OrganizePipelineRequest struct {
	Scope              OrganizeScope
	Trigger            OrganizeTrigger
	TaskName           string
	MediaID            string
	LibraryID          string
	PreferredLibraryID string
	SourcePath         string
	DestPath           string
	TransferMode       string
	MediaType          string
	MediaCategory      string
	ScanAfter          *bool
	ScrapeAfter        *bool
	DryRun             bool
	AllowReplace       bool
}

type OrganizePipelineResponse struct {
	Path   string
	Result *OrganizeResult
}

type OrganizePipelineService struct {
	log       *zap.Logger
	repo      *repository.Container
	organizer *OrganizerService
	scanner   *ScannerService
	tasks     *TaskTrackerService
}

func NewOrganizePipelineService(log *zap.Logger, repo *repository.Container, organizer *OrganizerService, scanner *ScannerService, tasks *TaskTrackerService) *OrganizePipelineService {
	return &OrganizePipelineService{
		log:       log,
		repo:      repo,
		organizer: organizer,
		scanner:   scanner,
		tasks:     tasks,
	}
}

func (p *OrganizePipelineService) Run(ctx context.Context, req OrganizePipelineRequest) (*OrganizePipelineResponse, error) {
	if p == nil || p.organizer == nil {
		return nil, errors.New("organize pipeline unavailable")
	}
	opts := OrganizeOptions{
		SourcePath:           strings.TrimSpace(req.SourcePath),
		DestPath:             strings.TrimSpace(req.DestPath),
		MediaType:            strings.TrimSpace(req.MediaType),
		MediaCategory:        strings.TrimSpace(req.MediaCategory),
		DryRun:               req.DryRun,
		AllowReplaceExisting: req.AllowReplace,
	}
	if mode := strings.TrimSpace(req.TransferMode); mode != "" {
		opts.TransferMode = TransferMode(mode)
	}
	task := p.startTask(ctx, req, opts)

	response := &OrganizePipelineResponse{}
	res, path, err := p.runOrganize(ctx, req, opts)
	if err != nil {
		p.finishTask(task, err, "organize", p.failureMessage(req), nil)
		p.recordProblem(ctx, req, nil, err)
		return nil, err
	}
	response.Path = path
	response.Result = res

	if fatalErr := organizeFatalResultError(res); fatalErr != nil {
		p.logOrganizeProblem(req, res, fatalErr)
		p.finishTask(task, fatalErr, "organize", p.failureMessage(req), res)
		p.recordProblem(ctx, req, res, fatalErr)
		return nil, fatalErr
	}
	if res != nil && len(res.Errors) > 0 {
		p.logOrganizeProblem(req, res, nil)
		p.recordProblem(ctx, req, res, nil)
	}

	if task != nil {
		task.Update(TaskUpdate{
			Stage:      "organize",
			SourcePath: res.SourcePath,
			DestPath:   firstNonEmpty(res.DestPath, filepath.Dir(path)),
			Message:    "整理/重命名完成，准备扫描入库",
			Metrics:    OrganizeTaskMetrics(res),
			Details:    OrganizeTaskDetails(res, 8),
		})
	}

	if p.shouldScan(req, res) {
		if task != nil {
			task.Update(TaskUpdate{
				Stage:   "scan_scrape",
				Message: "正在扫描入库并按设置刮削",
				Metrics: OrganizeTaskMetrics(res),
				Details: OrganizeTaskDetails(res, 8),
			})
		}
		scanRoot := organizeScanRoot(res, path)
		if scanRoot == "" && strings.TrimSpace(path) != "" {
			scanRoot = filepath.Dir(path)
		}
		preferredLibraryID := p.scanPreferredLibraryID(ctx, req)
		scrapeAfter := p.scrapeAfter(ctx, req)
		res.Scans, res.Scrapes = p.scanner.ScanAndScrapeLibrariesForPath(ctx, scanRoot, preferredLibraryID, scrapeAfter)
		if scrapeAfter {
			p.syncScrapedOrganizedNames(ctx, opts, res, path)
		}
	} else if p.log != nil && res != nil && !req.DryRun {
		p.log.Info("organize pipeline skipped scan; no destination changes",
			zap.String("trigger", string(req.Trigger)),
			zap.String("scope", string(req.Scope)),
			zap.String("source", res.SourcePath),
			zap.String("dest", res.DestPath),
			zap.Int("organized", res.Organized),
			zap.Int("replaced", res.Replaced),
			zap.Int("reclassified", res.Reclassified),
			zap.Int("skipped", res.Skipped))
	}

	p.finishTask(task, nil, "completed", p.completedMessage(req), res)
	if p.log != nil && res != nil {
		p.log.Info("organize pipeline finished",
			zap.String("trigger", string(req.Trigger)),
			zap.String("scope", string(req.Scope)),
			zap.String("source", res.SourcePath),
			zap.String("dest", firstNonEmpty(res.DestPath, filepath.Dir(path))),
			zap.Int("organized", res.Organized),
			zap.Int("replaced", res.Replaced),
			zap.Int("reclassified", res.Reclassified),
			zap.Int("skipped", res.Skipped),
			zap.Int("scrapes", len(res.Scrapes)),
			zap.Int("errors", len(res.Errors)))
	}
	return response, nil
}

func (p *OrganizePipelineService) runOrganize(ctx context.Context, req OrganizePipelineRequest, opts OrganizeOptions) (*OrganizeResult, string, error) {
	switch req.Scope {
	case OrganizeScopeMedia:
		path, err := p.organizer.OrganizeMediaWithOptions(ctx, strings.TrimSpace(req.MediaID), opts)
		if err != nil {
			return nil, "", err
		}
		res := &OrganizeResult{
			Organized:  1,
			SourcePath: opts.SourcePath,
			DestPath:   filepath.Dir(path),
			DryRun:     opts.DryRun,
			Items: []OrganizePreviewItem{{
				Target: path,
				Action: "organize",
			}},
		}
		return res, path, nil
	case OrganizeScopeLibrary:
		res, err := p.organizer.OrganizeLibraryWithOptions(ctx, strings.TrimSpace(req.LibraryID), opts)
		return res, "", err
	default:
		res, err := p.organizer.OrganizeDirectory(ctx, opts)
		return res, "", err
	}
}

func (p *OrganizePipelineService) shouldScan(req OrganizePipelineRequest, res *OrganizeResult) bool {
	if req.DryRun || p == nil || p.scanner == nil || res == nil {
		return false
	}
	if req.ScanAfter != nil && !*req.ScanAfter {
		return false
	}
	return OrganizeResultNeedsVisibilitySync(res)
}

func (p *OrganizePipelineService) scrapeAfter(ctx context.Context, req OrganizePipelineRequest) bool {
	if req.ScrapeAfter != nil {
		return *req.ScrapeAfter
	}
	return OrganizeScrapeAfterEnabled(ctx, p.repo)
}

func (p *OrganizePipelineService) scanPreferredLibraryID(ctx context.Context, req OrganizePipelineRequest) string {
	if preferred := strings.TrimSpace(req.PreferredLibraryID); preferred != "" {
		return preferred
	}
	switch req.Scope {
	case OrganizeScopeMedia:
		if p == nil || p.repo == nil || p.repo.Media == nil {
			return ""
		}
		media, err := p.repo.Media.FindByID(ctx, strings.TrimSpace(req.MediaID))
		if err != nil || media == nil {
			return ""
		}
		return strings.TrimSpace(media.LibraryID)
	case OrganizeScopeLibrary:
		return strings.TrimSpace(req.LibraryID)
	default:
		return ""
	}
}
