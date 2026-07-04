// Package service — download manager.
//
// DownloadService persists user-initiated downloads, dispatches them to
// the configured client (currently qBittorrent) and pushes live progress
// to the WS hub so the React UI can render a live table.
//
// Settings consumed (system Setting table):
//
//	qbittorrent.url       e.g. http://127.0.0.1:8080
//	qbittorrent.username  qBittorrent WebUI user
//	qbittorrent.password  qBittorrent WebUI password
//	qbittorrent.savepath  optional default save dir
//
// Settings can be updated at runtime via the admin UI; ReloadConfig()
// re-reads them and re-authenticates.
package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// DownloadService is the single download orchestrator.
type DownloadService struct {
	log              *zap.Logger
	repo             *repository.Container
	hub              *Hub
	qb               *QBitClient
	organizer        *OrganizerService
	organizePipeline *OrganizePipelineService
	scanner          *ScannerService
	site             *SiteService
	tasks            *TaskTrackerService
	notify           *NotifyChannelService

	mu              sync.Mutex
	stopCh          chan struct{}
	pollOnce        sync.Once
	organizeOnce    sync.Once
	prevStates      map[string]bool // hash -> wasCompleted
	pollInitialized bool
	liveTorrents    []QBitTorrent
	liveTorrentsAt  time.Time
	now             func() time.Time
	organizeQueue   chan QBitTorrent
	organizeQueued  map[string]struct{}

	preparedMu        sync.Mutex
	preparedDownloads map[string]preparedDownloadEntry
}

type preparedDownloadEntry struct {
	URL       string
	SavePath  string
	Meta      DownloadTaskMeta
	ExpiresAt time.Time
}

func (d *DownloadService) SetScanner(scanner *ScannerService) {
	d.scanner = scanner
}

func (d *DownloadService) SetOrganizePipeline(pipeline *OrganizePipelineService) {
	d.organizePipeline = pipeline
}

func (d *DownloadService) SetTaskTracker(tasks *TaskTrackerService) {
	d.tasks = tasks
}

func (d *DownloadService) SetNotifyChannels(notify *NotifyChannelService) {
	d.notify = notify
}

// ErrDownloadAlreadyExists tells callers that the requested resource is already
// tracked locally or present in qBittorrent. Subscriptions treat this as a
// successful dedup hit, not as a retryable enqueue failure.
var ErrDownloadAlreadyExists = errors.New("download already exists")

// ErrMediaAlreadyInLibrary tells callers that the requested movie/episode is
// already present in the scanned media library and must not be sent to the
// downloader again.
var ErrMediaAlreadyInLibrary = errors.New("media already exists in library")

func IsDownloadDedupError(err error) bool {
	return errors.Is(err, ErrDownloadAlreadyExists) || errors.Is(err, ErrMediaAlreadyInLibrary)
}

// NewDownloadService is the constructor.
func NewDownloadService(log *zap.Logger, repo *repository.Container, hub *Hub, organizer *OrganizerService, site ...*SiteService) *DownloadService {
	var siteSvc *SiteService
	if len(site) > 0 {
		siteSvc = site[0]
	}
	return &DownloadService{
		log:               log,
		repo:              repo,
		hub:               hub,
		qb:                NewQBitClient(log, QBitConfig{}),
		organizer:         organizer,
		site:              siteSvc,
		prevStates:        make(map[string]bool),
		now:               time.Now,
		organizeQueue:     make(chan QBitTorrent, completedTorrentOrganizeQueueSize),
		organizeQueued:    make(map[string]struct{}),
		preparedDownloads: make(map[string]preparedDownloadEntry),
		stopCh:            make(chan struct{}),
	}
}

// Start kicks off the background poller (idempotent).
func (d *DownloadService) Start(ctx context.Context) {
	d.pollOnce.Do(func() {
		_ = d.ReloadConfig(ctx)
		d.startAutoOrganizeWorker(ctx)
		go d.poll(ctx)
	})
}

// Stop terminates the poller.
func (d *DownloadService) Stop() {
	close(d.stopCh)
}

func (d *DownloadService) TorrentExistsByName(ctx context.Context, name string) bool {
	query := normalizeTorrentName(name)
	if query == "" {
		return false
	}
	live, err := d.qb.List(ctx, "")
	if err != nil {
		return false
	}
	for _, torrent := range live {
		if downloadTitleCoversRequest(torrent.Name, name) {
			return true
		}
		current := normalizeTorrentName(torrent.Name)
		if current == "" {
			continue
		}
		if current == query {
			return true
		}
	}
	return false
}

// List returns every persisted download task augmented with live data
// from qBittorrent when available.
func (d *DownloadService) List(ctx context.Context) ([]model.DownloadTask, []QBitTorrent, error) {
	rows, err := d.repo.Download.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	live, err := d.qb.List(ctx, "")
	if err != nil {
		// Network failure shouldn't break the page — return rows with no
		// live data and let the UI render the persisted snapshot.
		d.log.Debug("qbittorrent list failed", zap.Error(err))
		return rows, nil, nil
	}
	return rows, live, nil
}

// Delete removes a torrent (and optionally its files) from qBittorrent.
func (d *DownloadService) Delete(ctx context.Context, hash string, withFiles bool) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return errors.New("hash is required")
	}
	var torrentName string
	if live, err := d.qb.List(ctx, ""); err == nil {
		for _, torrent := range live {
			if strings.EqualFold(torrent.Hash, hash) || len(live) == 1 {
				torrentName = torrent.Name
				break
			}
		}
	}
	if err := d.qb.Delete(ctx, hash, withFiles); err != nil {
		return err
	}
	d.markDownloadTaskDeleted(ctx, hash, torrentName)
	stateKey := strings.ToLower(hash)
	d.mu.Lock()
	delete(d.prevStates, stateKey)
	delete(d.organizeQueued, stateKey)
	d.mu.Unlock()
	return nil
}

func (d *DownloadService) markDownloadTaskDeleted(ctx context.Context, hash, torrentName string) {
	if d == nil || d.repo == nil || d.repo.DB == nil {
		return
	}
	rows, err := d.repo.Download.List(ctx)
	if err != nil {
		return
	}
	if matched, ok := findDownloadTaskByHash(rows, hash); ok {
		_ = d.repo.DB.WithContext(ctx).Model(&model.DownloadTask{}).
			Where("id = ?", matched.ID).
			Updates(map[string]any{
				"status":   "deleted",
				"progress": matched.Progress,
			}).Error
		return
	}
	if strings.TrimSpace(torrentName) == "" {
		return
	}
	taskByKey := tasksByTorrentIdentity(rows)
	matched, ok := findMatchingTaskByTorrentIdentity(torrentName, taskByKey)
	if !ok {
		return
	}
	_ = d.repo.DB.WithContext(ctx).Model(&model.DownloadTask{}).
		Where("id = ?", matched.ID).
		Updates(map[string]any{
			"status":   "deleted",
			"progress": matched.Progress,
		}).Error
}

func findDownloadTaskByHash(rows []model.DownloadTask, hash string) (model.DownloadTask, bool) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return model.DownloadTask{}, false
	}
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.URL), hash) {
			return row, true
		}
	}
	return model.DownloadTask{}, false
}

// RelocateTorrent moves a torrent's data to a new save directory while keeping
// it seeding (qBittorrent performs the physical move and resumes seeding).
// 用于「移动 PT 种子文件且转移后继续做种上传」的整盘迁移场景。
func (d *DownloadService) RelocateTorrent(ctx context.Context, hash, location string) error {
	if strings.TrimSpace(hash) == "" {
		return errors.New("hash is required")
	}
	if strings.TrimSpace(location) == "" {
		return errors.New("location is required")
	}
	return d.qb.SetLocation(ctx, hash, strings.TrimSpace(location))
}
