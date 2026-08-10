// Package service — download manager.
//
// DownloadService persists user-initiated downloads, dispatches them to
// the configured qBittorrent, Transmission, or aria2 client and pushes live progress
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
	"fmt"
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
	manager          *DownloadManager
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
	prevStates      map[string]bool // client/task identity -> wasCompleted
	pollInitialized bool
	liveTorrents    []QBitTorrent
	liveTorrentsAt  time.Time
	now             func() time.Time
	organizeQueue   chan QBitTorrent
	organizeQueued  map[string]struct{}
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

func (d *DownloadService) SetDownloadManager(manager *DownloadManager) {
	d.manager = manager
}

// ErrDownloadAlreadyExists tells callers that the requested resource is already
// tracked locally or present in a downloader. Subscriptions treat this as a
// successful dedup hit, not as a retryable enqueue failure.
var ErrDownloadAlreadyExists = errors.New("download already exists")

// ErrMediaAlreadyInLibrary tells callers that the requested movie/episode is
// already present in the scanned media library and must not be sent to the
// downloader again.
var ErrMediaAlreadyInLibrary = errors.New("media already exists in library")

var ErrDownloadOperationUnsupported = errors.New("download client operation unsupported")

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
		log:            log,
		repo:           repo,
		hub:            hub,
		qb:             NewQBitClient(log, QBitConfig{}),
		organizer:      organizer,
		site:           siteSvc,
		prevStates:     make(map[string]bool),
		now:            time.Now,
		organizeQueue:  make(chan QBitTorrent, completedTorrentOrganizeQueueSize),
		organizeQueued: make(map[string]struct{}),
		stopCh:         make(chan struct{}),
	}
}

// Start kicks off the background poller (idempotent).
func (d *DownloadService) Start(ctx context.Context) {
	d.pollOnce.Do(func() {
		if err := d.ReloadConfig(ctx); err != nil && d.log != nil {
			d.log.Warn("initial download client reload failed", zap.Error(err))
		}
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
	live, err := d.listLiveTorrents(ctx, "")
	if err != nil && len(live) == 0 {
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

// List returns every persisted download task augmented with live downloader data.
func (d *DownloadService) List(ctx context.Context) ([]model.DownloadTask, []QBitTorrent, error) {
	rows, err := d.repo.Download.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	live, err := d.listLiveTorrents(ctx, "")
	if err != nil {
		// A failed client must not hide healthy clients or persisted rows.
		d.log.Debug("download client list failed", zap.Error(err))
		if len(live) == 0 {
			return rows, nil, nil
		}
	}
	return rows, live, nil
}

// Delete removes a task from its native downloader. clientID is optional for
// legacy callers, but disambiguates equal native IDs across multiple clients.
func (d *DownloadService) Delete(ctx context.Context, hash string, withFiles bool, clientID ...string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return errors.New("hash is required")
	}
	requestedClientID := ""
	if len(clientID) > 0 {
		requestedClientID = clientID[0]
	}
	resolvedClientID, torrentName, err := d.resolveOperationClientID(ctx, hash, requestedClientID)
	if err != nil {
		return err
	}
	target, err := d.downloadTargetByID(ctx, resolvedClientID)
	if err != nil {
		return err
	}
	if target.legacyQB {
		err = d.qb.Delete(ctx, hash, withFiles)
	} else {
		err = target.adapter.Remove(ctx, hash, withFiles)
	}
	if err != nil {
		return err
	}
	d.markDownloadTaskDeleted(ctx, hash, torrentName, resolvedClientID)
	stateKey := completedTorrentQueueKey(QBitTorrent{ClientID: resolvedClientID, Hash: hash})
	d.mu.Lock()
	delete(d.prevStates, stateKey)
	delete(d.organizeQueued, stateKey)
	d.mu.Unlock()
	return nil
}

func (d *DownloadService) markDownloadTaskDeleted(ctx context.Context, hash, torrentName string, clientID ...string) {
	if d == nil || d.repo == nil || d.repo.DB == nil {
		return
	}
	rows, err := d.repo.Download.List(ctx)
	if err != nil {
		return
	}
	if matched, ok := findDownloadTaskByHash(rows, hash, clientID...); ok {
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
	matched, ok := findMatchingTaskForTorrent(QBitTorrent{Name: torrentName, Hash: hash, ClientID: firstString(clientID)}, taskByKey)
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

func findDownloadTaskByHash(rows []model.DownloadTask, hash string, clientID ...string) (model.DownloadTask, bool) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return model.DownloadTask{}, false
	}
	wantClientID := strings.TrimSpace(firstString(clientID))
	for _, row := range rows {
		if wantClientID != "" && strings.TrimSpace(row.DownloadClientID) != "" && row.DownloadClientID != wantClientID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(row.ExternalID), hash) {
			return row, true
		}
	}
	for _, row := range rows {
		if wantClientID != "" && strings.TrimSpace(row.DownloadClientID) != "" && row.DownloadClientID != wantClientID {
			continue
		}
		if strings.Contains(strings.ToLower(row.URL), hash) {
			return row, true
		}
	}
	return model.DownloadTask{}, false
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// RelocateTorrent moves a torrent's data to a new save directory while keeping
// it seeding (qBittorrent performs the physical move and resumes seeding).
// 用于「移动 PT 种子文件且转移后继续做种上传」的整盘迁移场景。
func (d *DownloadService) RelocateTorrent(ctx context.Context, hash, location string, clientID ...string) error {
	if strings.TrimSpace(hash) == "" {
		return errors.New("hash is required")
	}
	if strings.TrimSpace(location) == "" {
		return errors.New("location is required")
	}
	requestedClientID := firstString(clientID)
	resolvedClientID := strings.TrimSpace(requestedClientID)
	if resolvedClientID == "" {
		var err error
		resolvedClientID, _, err = d.resolveOperationClientID(ctx, hash, "")
		if err != nil {
			return err
		}
	}
	target, err := d.downloadTargetByID(ctx, resolvedClientID)
	if err != nil {
		return err
	}
	if target.typ != "qbittorrent" {
		return fmt.Errorf("%w: %s does not support torrent relocation; only qBittorrent is supported", ErrDownloadOperationUnsupported, target.typ)
	}
	if target.legacyQB {
		return d.qb.SetLocation(ctx, hash, strings.TrimSpace(location))
	}
	relocator, ok := target.adapter.(TorrentRelocateAdapter)
	if !ok {
		return fmt.Errorf("%w: configured qBittorrent adapter cannot relocate torrents", ErrDownloadOperationUnsupported)
	}
	return relocator.Relocate(ctx, hash, strings.TrimSpace(location))
}
