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
	"crypto/sha1"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

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

var torrentEpisodeToken = regexp.MustCompile(`(?i)e\d{1,3}`)

const settingDownloadClientsManaged = "download_clients.managed"

const completedTorrentOrganizeQueueSize = 64

var completedTorrentOrganizeCooldown = 3 * time.Second

const preparedDownloadTTL = 30 * time.Minute

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

// DownloadTaskMeta carries public display metadata for a download. It is
// deliberately separate from the private torrent URL so API responses never
// need to expose tracker tokens.
type DownloadTaskMeta struct {
	SubscriptionID       string
	Title                string
	IdentityTitle        string
	PosterURL            string
	BackdropURL          string
	Overview             string
	IMDBID               string
	TMDbID               int
	DoubanID             string
	MediaType            string
	MediaCategory        string
	SourceCategory       string
	SelectedFiles        []string
	OriginalName         string
	OriginalLanguage     string
	Year                 int
	Rating               float32
	Genres               string
	AllowExistingLibrary bool
}

type PreparedDownload struct {
	Hash  string            `json:"hash"`
	Files []QBitTorrentFile `json:"files"`
}

type DownloadTaskView struct {
	ID            string    `json:"id"`
	Source        string    `json:"source"`
	Title         string    `json:"title"`
	PosterURL     string    `json:"poster_url,omitempty"`
	BackdropURL   string    `json:"backdrop_url,omitempty"`
	Overview      string    `json:"overview,omitempty"`
	SavePath      string    `json:"save_path"`
	MediaType     string    `json:"media_type,omitempty"`
	MediaCategory string    `json:"media_category,omitempty"`
	Status        string    `json:"status"`
	Progress      float32   `json:"progress"`
	State         string    `json:"state,omitempty"`
	DLSpeed       int64     `json:"dlspeed,omitempty"`
	UpSpeed       int64     `json:"upspeed,omitempty"`
	Size          int64     `json:"size,omitempty"`
	Downloaded    int64     `json:"downloaded,omitempty"`
	NumSeeds      int       `json:"num_seeds,omitempty"`
	NumLeechs     int       `json:"num_leechs,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type DownloadTorrentView struct {
	Hash          string  `json:"hash"`
	Name          string  `json:"name"`
	Title         string  `json:"title"`
	PosterURL     string  `json:"poster_url,omitempty"`
	BackdropURL   string  `json:"backdrop_url,omitempty"`
	Overview      string  `json:"overview,omitempty"`
	MediaType     string  `json:"media_type,omitempty"`
	MediaCategory string  `json:"media_category,omitempty"`
	State         string  `json:"state"`
	Progress      float32 `json:"progress"`
	DLSpeed       int64   `json:"dlspeed"`
	UpSpeed       int64   `json:"upspeed"`
	NumSeeds      int     `json:"num_seeds"`
	NumLeechs     int     `json:"num_leechs"`
	Size          int64   `json:"size"`
	Downloaded    int64   `json:"downloaded"`
	SavePath      string  `json:"save_path"`
	AddedOn       int64   `json:"added_on,omitempty"`
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
		organizeQueue:  make(chan QBitTorrent, completedTorrentOrganizeQueueSize),
		organizeQueued: make(map[string]struct{}),
		stopCh:         make(chan struct{}),
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

// ReloadConfig rebuilds the qBittorrent client from the configured
// download clients (preferred) or the legacy Setting table (fallback).
//
// 配置来源优先级：
//
//  1. download_clients 表中 type=qbittorrent 且 is_default=true 且 enabled=true
//     的行（侧边栏「下载器」页面写入的数据）。
//  2. system Setting 表中的 qbittorrent.url / username / password
//     （旧版「系统设置」表单写入的数据；保留作向后兼容）。
//
// 这避免了两套配置各跑各的：之前操作员明明已经在「下载器」页面填好
// 默认 qb，但实际下载链路读的还是 Setting 表，导致一直连不上。
func (d *DownloadService) ReloadConfig(ctx context.Context) error {
	cfg := QBitConfig{}
	hasConfiguredClients := false
	managedByDownloadClients := false

	// Path 1: download_clients 表
	if d.repo.DownloadClient != nil {
		hasConfiguredClients, _ = d.repo.DownloadClient.HasAnyIncludingDeleted(ctx)
		if c, err := d.repo.DownloadClient.FindDefault(ctx); err == nil && c != nil && c.Type == "qbittorrent" {
			cfg.BaseURL = strings.TrimRight(c.Host, "/")
			cfg.Username = c.Username
			cfg.Password = c.Password
		} else if c, err := d.soleEnabledQBitClient(ctx); err == nil && c != nil {
			cfg.BaseURL = strings.TrimRight(c.Host, "/")
			cfg.Username = c.Username
			cfg.Password = c.Password
		}
	}
	if d.repo.Setting != nil {
		managedRaw, _ := d.repo.Setting.Get(ctx, settingDownloadClientsManaged)
		managedByDownloadClients = strings.EqualFold(strings.TrimSpace(managedRaw), "true")
	}

	// Path 2: legacy Setting 表。
	// 仅在旧部署“从未使用过 download_clients 表”时回退。只要操作员曾经
	// 配置过下载器，删除/禁用全部下载器就表示应停止投递，不能再偷偷用
	// qbittorrent.* 旧设置继续往下载器添加任务。
	if cfg.BaseURL == "" && !hasConfiguredClients && !managedByDownloadClients {
		get := func(k string) string {
			v, _ := d.repo.Setting.Get(ctx, k)
			return v
		}
		cfg.BaseURL = get("qbittorrent.url")
		cfg.Username = get("qbittorrent.username")
		cfg.Password = get("qbittorrent.password")
	}

	d.qb.Configure(cfg)
	return nil
}

func (d *DownloadService) soleEnabledQBitClient(ctx context.Context) (*model.DownloadClient, error) {
	if d == nil || d.repo == nil || d.repo.DownloadClient == nil {
		return nil, nil
	}
	rows, err := d.repo.DownloadClient.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	var selected *model.DownloadClient
	for i := range rows {
		if rows[i].Type != "qbittorrent" {
			continue
		}
		if selected != nil {
			return nil, nil
		}
		row := rows[i]
		selected = &row
	}
	return selected, nil
}

// AddDownload accepts a magnet URL / HTTP URL and persists a tracking row.
func (d *DownloadService) AddDownload(ctx context.Context, userID, urlStr, savePath string) (*model.DownloadTask, error) {
	return d.AddDownloadWithMeta(ctx, userID, urlStr, savePath, DownloadTaskMeta{})
}

func (d *DownloadService) normalizeDownloadRequest(ctx context.Context, urlStr, savePath string, meta DownloadTaskMeta) (string, string, string, DownloadTaskMeta, error) {
	if urlStr == "" {
		return "", "", "", meta, errors.New("empty url")
	}
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = publicDownloadTitle(urlStr)
		meta.Title = title
	}
	identityTitle := strings.TrimSpace(meta.IdentityTitle)
	if identityTitle == "" {
		identityTitle = title
	}
	autoClassify := downloadSmartClassifyEnabled(ctx, d.repo, d.organizer)
	savePath, resolvedCategory := d.resolveDownloadSavePath(ctx, savePath, meta, autoClassify)
	if !autoClassify {
		meta.MediaCategory = ""
	} else if strings.TrimSpace(meta.MediaCategory) == "" {
		meta.MediaCategory = resolvedCategory
	}
	return title, identityTitle, savePath, meta, nil
}

func (d *DownloadService) AddDownloadWithMeta(ctx context.Context, userID, urlStr, savePath string, meta DownloadTaskMeta) (*model.DownloadTask, error) {
	title, identityTitle, savePath, meta, err := d.normalizeDownloadRequest(ctx, urlStr, savePath, meta)
	if err != nil {
		return nil, err
	}
	if !meta.AllowExistingLibrary && d.localMediaAlreadyExists(ctx, title) {
		return nil, ErrMediaAlreadyInLibrary
	}
	if existing, ok := d.findExistingDownloadTask(ctx, identityTitle, strings.TrimSpace(meta.SubscriptionID) != ""); ok && d.qb.IsConfigured() {
		live, liveErr := d.qb.List(ctx, "")
		if liveErr != nil || torrentExistsInListByIdentity(live, identityTitle) {
			return existing, ErrDownloadAlreadyExists
		}
	}
	_ = d.ReloadConfig(ctx)
	if !d.qb.IsConfigured() {
		return nil, errors.New("no default downloader configured")
	}
	live, liveErr := d.qb.List(ctx, "")
	if existing, ok := d.findExistingDownloadTask(ctx, identityTitle, strings.TrimSpace(meta.SubscriptionID) != ""); ok {
		if liveErr != nil || torrentExistsInListByIdentity(live, identityTitle) {
			return existing, ErrDownloadAlreadyExists
		}
	}
	if liveErr == nil && torrentExistsInListByIdentity(live, identityTitle) {
		task, err := d.createTask(ctx, userID, urlStr, savePath, meta)
		if err != nil {
			return nil, err
		}
		return task, ErrDownloadAlreadyExists
	}
	var siteFetchErr error
	qbitCategory := strings.TrimSpace(meta.MediaCategory)
	if d.site != nil {
		if data, name, err := d.site.FetchTorrentFile(ctx, urlStr); err == nil {
			if err := d.qb.AddTorrentFileWithCategoryAndName(ctx, data, name, savePath, qbitCategory, title, meta.SelectedFiles); err != nil {
				return nil, err
			}
			if strings.TrimSpace(meta.Title) == "" {
				meta.Title = strings.TrimSuffix(name, path.Ext(name))
			}
			return d.createTask(ctx, userID, urlStr, savePath, meta)
		} else {
			siteFetchErr = err
		}
	}
	if err := d.qb.AddTorrentWithCategoryAndName(ctx, urlStr, savePath, qbitCategory, title, meta.SelectedFiles); err != nil {
		if siteFetchErr != nil && !strings.Contains(siteFetchErr.Error(), "no matching PT site") {
			return nil, errors.Join(err, siteFetchErr)
		}
		return nil, err
	}
	return d.createTask(ctx, userID, urlStr, savePath, meta)
}

func (d *DownloadService) PrepareDownloadWithMeta(ctx context.Context, userID, urlStr, savePath string, meta DownloadTaskMeta) (*PreparedDownload, error) {
	_ = userID
	title, identityTitle, savePath, meta, err := d.normalizeDownloadRequest(ctx, urlStr, savePath, meta)
	if err != nil {
		return nil, err
	}
	if !meta.AllowExistingLibrary && d.localMediaAlreadyExists(ctx, title) {
		return nil, ErrMediaAlreadyInLibrary
	}
	if existing, ok := d.findExistingDownloadTask(ctx, identityTitle, strings.TrimSpace(meta.SubscriptionID) != ""); ok && d.qb.IsConfigured() {
		_ = existing
		live, liveErr := d.qb.List(ctx, "")
		if liveErr != nil || torrentExistsInListByIdentity(live, identityTitle) {
			return nil, ErrDownloadAlreadyExists
		}
	}
	_ = d.ReloadConfig(ctx)
	if !d.qb.IsConfigured() {
		return nil, errors.New("no default downloader configured")
	}
	live, liveErr := d.qb.List(ctx, "")
	if existing, ok := d.findExistingDownloadTask(ctx, identityTitle, strings.TrimSpace(meta.SubscriptionID) != ""); ok {
		_ = existing
		if liveErr != nil || torrentExistsInListByIdentity(live, identityTitle) {
			return nil, ErrDownloadAlreadyExists
		}
	}
	if liveErr == nil {
		if torrent, ok := findLikelyPreparedTorrentByIdentity(live, identityTitle); ok {
			return d.reusePreparedTorrent(ctx, torrent.Hash, urlStr, savePath, meta)
		}
		if torrentExistsInListByIdentity(live, identityTitle) {
			return nil, ErrDownloadAlreadyExists
		}
	}
	var siteFetchErr error
	qbitCategory := strings.TrimSpace(meta.MediaCategory)
	if d.site != nil {
		if data, name, err := d.site.FetchTorrentFile(ctx, urlStr); err == nil {
			hash, files, err := d.qb.PrepareTorrentFileWithCategoryAndName(ctx, data, name, savePath, qbitCategory, title)
			if err != nil {
				if errors.Is(err, ErrDownloadAlreadyExists) {
					if prepared, ok := d.reusePreparedTorrentByHash(ctx, hash, urlStr, savePath, meta); ok {
						return prepared, nil
					}
				}
				return nil, err
			}
			d.rememberPreparedDownload(hash, urlStr, savePath, meta)
			return &PreparedDownload{Hash: hash, Files: files}, nil
		} else {
			siteFetchErr = err
		}
	}
	hash, files, err := d.qb.PrepareTorrentWithCategoryAndName(ctx, urlStr, savePath, qbitCategory, title)
	if err != nil {
		if errors.Is(err, ErrDownloadAlreadyExists) {
			if prepared, ok := d.reusePreparedTorrentByHash(ctx, hash, urlStr, savePath, meta); ok {
				return prepared, nil
			}
		}
		if siteFetchErr != nil && !strings.Contains(siteFetchErr.Error(), "no matching PT site") {
			return nil, errors.Join(err, siteFetchErr)
		}
		return nil, err
	}
	d.rememberPreparedDownload(hash, urlStr, savePath, meta)
	return &PreparedDownload{Hash: hash, Files: files}, nil
}

func (d *DownloadService) reusePreparedTorrent(ctx context.Context, hash, urlStr, savePath string, meta DownloadTaskMeta) (*PreparedDownload, error) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return nil, ErrDownloadAlreadyExists
	}
	files, err := d.qb.Files(ctx, hash)
	if err != nil {
		return nil, err
	}
	d.rememberPreparedDownload(hash, urlStr, savePath, meta)
	return &PreparedDownload{Hash: hash, Files: files}, nil
}

func (d *DownloadService) reusePreparedTorrentByHash(ctx context.Context, hash, urlStr, savePath string, meta DownloadTaskMeta) (*PreparedDownload, bool) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return nil, false
	}
	live, err := d.qb.List(ctx, "")
	if err != nil {
		return nil, false
	}
	for _, torrent := range live {
		if strings.EqualFold(torrent.Hash, hash) && isLikelyPreparedTorrent(torrent) {
			prepared, err := d.reusePreparedTorrent(ctx, hash, urlStr, savePath, meta)
			if err != nil {
				return nil, false
			}
			return prepared, true
		}
	}
	return nil, false
}

func (d *DownloadService) ConfirmPreparedDownload(ctx context.Context, userID, hash, urlStr, savePath string, meta DownloadTaskMeta, selectedFileIndexes []int) (*model.DownloadTask, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return nil, errors.New("hash is required")
	}
	if prepared, ok := d.takePreparedDownload(hash); ok {
		if strings.TrimSpace(urlStr) == "" {
			urlStr = prepared.URL
		}
		if strings.TrimSpace(savePath) == "" {
			savePath = prepared.SavePath
		}
		meta = mergePreparedDownloadMeta(meta, prepared.Meta)
	}
	_, _, savePath, meta, err := d.normalizeDownloadRequest(ctx, urlStr, savePath, meta)
	if err != nil {
		return nil, err
	}
	_ = d.ReloadConfig(ctx)
	if !d.qb.IsConfigured() {
		return nil, errors.New("no default downloader configured")
	}
	if len(selectedFileIndexes) > 0 {
		if err := d.qb.ApplySelectedFileIndexes(ctx, hash, selectedFileIndexes); err != nil {
			return nil, err
		}
	} else if err := d.qb.Resume(ctx, hash); err != nil {
		return nil, err
	}
	return d.createTask(ctx, userID, urlStr, savePath, meta)
}

func (d *DownloadService) CancelPreparedDownload(ctx context.Context, hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return errors.New("hash is required")
	}
	_ = d.ReloadConfig(ctx)
	if !d.qb.IsConfigured() {
		return errors.New("no default downloader configured")
	}
	d.forgetPreparedDownload(hash)
	return d.qb.Delete(ctx, hash, false)
}

func (d *DownloadService) rememberPreparedDownload(hash, urlStr, savePath string, meta DownloadTaskMeta) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if d == nil || hash == "" {
		return
	}
	d.preparedMu.Lock()
	defer d.preparedMu.Unlock()
	d.prunePreparedDownloadsLocked(time.Now())
	if d.preparedDownloads == nil {
		d.preparedDownloads = map[string]preparedDownloadEntry{}
	}
	d.preparedDownloads[hash] = preparedDownloadEntry{
		URL:       urlStr,
		SavePath:  savePath,
		Meta:      meta,
		ExpiresAt: time.Now().Add(preparedDownloadTTL),
	}
}

func (d *DownloadService) takePreparedDownload(hash string) (preparedDownloadEntry, bool) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if d == nil || hash == "" {
		return preparedDownloadEntry{}, false
	}
	d.preparedMu.Lock()
	defer d.preparedMu.Unlock()
	d.prunePreparedDownloadsLocked(time.Now())
	entry, ok := d.preparedDownloads[hash]
	if ok {
		delete(d.preparedDownloads, hash)
	}
	return entry, ok
}

func (d *DownloadService) forgetPreparedDownload(hash string) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if d == nil || hash == "" {
		return
	}
	d.preparedMu.Lock()
	defer d.preparedMu.Unlock()
	delete(d.preparedDownloads, hash)
}

func (d *DownloadService) prunePreparedDownloadsLocked(now time.Time) {
	if len(d.preparedDownloads) == 0 {
		return
	}
	for hash, entry := range d.preparedDownloads {
		if now.After(entry.ExpiresAt) {
			delete(d.preparedDownloads, hash)
		}
	}
}

func mergePreparedDownloadMeta(current, prepared DownloadTaskMeta) DownloadTaskMeta {
	if strings.TrimSpace(current.SubscriptionID) == "" {
		current.SubscriptionID = prepared.SubscriptionID
	}
	if strings.TrimSpace(current.Title) == "" {
		current.Title = prepared.Title
	}
	if strings.TrimSpace(current.IdentityTitle) == "" {
		current.IdentityTitle = prepared.IdentityTitle
	}
	if strings.TrimSpace(current.PosterURL) == "" {
		current.PosterURL = prepared.PosterURL
	}
	if strings.TrimSpace(current.BackdropURL) == "" {
		current.BackdropURL = prepared.BackdropURL
	}
	if strings.TrimSpace(current.Overview) == "" {
		current.Overview = prepared.Overview
	}
	if strings.TrimSpace(current.IMDBID) == "" {
		current.IMDBID = prepared.IMDBID
	}
	if current.TMDbID <= 0 {
		current.TMDbID = prepared.TMDbID
	}
	if strings.TrimSpace(current.DoubanID) == "" {
		current.DoubanID = prepared.DoubanID
	}
	if strings.TrimSpace(current.MediaType) == "" {
		current.MediaType = prepared.MediaType
	}
	if strings.TrimSpace(current.MediaCategory) == "" {
		current.MediaCategory = prepared.MediaCategory
	}
	if strings.TrimSpace(current.SourceCategory) == "" {
		current.SourceCategory = prepared.SourceCategory
	}
	if len(current.SelectedFiles) == 0 {
		current.SelectedFiles = append([]string(nil), prepared.SelectedFiles...)
	}
	if !current.AllowExistingLibrary {
		current.AllowExistingLibrary = prepared.AllowExistingLibrary
	}
	return current
}

func (d *DownloadService) resolveDownloadSavePath(ctx context.Context, explicitSavePath string, meta DownloadTaskMeta, autoClassify bool) (string, string) {
	if strings.TrimSpace(explicitSavePath) != "" {
		if !autoClassify {
			return explicitSavePath, ""
		}
		return explicitSavePath, strings.TrimSpace(meta.MediaCategory)
	}
	base := downloadDefaultSaveRoot(ctx, d.repo)
	if strings.TrimSpace(base) == "" {
		return "", strings.TrimSpace(meta.MediaCategory)
	}
	mediaType := normalizeMediaType(meta.MediaType, meta.Title, meta.SourceCategory)
	category := strings.TrimSpace(meta.MediaCategory)
	if category == "" {
		category = classifyMediaCategory(mediaClassifyInput{
			MediaType: mediaType,
			Title:     meta.Title,
			Category:  meta.SourceCategory,
		}, downloadCategoryMap(d.organizer))
	}
	if !autoClassify || category == "" {
		return base, ""
	}
	return downloadSavePathCategoryRoot(base, sanitizeFilename(category)), category
}

func (d *DownloadService) localMediaAlreadyExists(ctx context.Context, title string) bool {
	if d == nil || d.repo == nil || d.repo.DB == nil {
		return false
	}
	if !d.repo.DB.Migrator().HasTable(&model.Media{}) {
		return false
	}
	queries := localAvailabilityTitleCandidates(title)
	if len(queries) == 0 {
		return false
	}
	var rows []model.Media
	db := d.repo.DB.WithContext(ctx).Model(&model.Media{})
	for i, query := range queries {
		like := "%" + query + "%"
		clause := "title LIKE ? OR original_name LIKE ? OR path LIKE ?"
		if i == 0 {
			db = db.Where(clause, like, like, like)
		} else {
			db = db.Or(clause, like, like, like)
		}
	}
	if err := db.
		Order("season_num asc, episode_num asc, created_at desc").
		Limit(200).
		Find(&rows).Error; err != nil || len(rows) == 0 {
		return false
	}

	wantSeason, wantEpisode := ParseEpisode(title)
	if wantSeason <= 0 {
		wantSeason = 1
	}
	if wantEpisode <= 0 {
		return true
	}
	for _, row := range rows {
		rowSeason := row.SeasonNum
		rowEpisode := row.EpisodeNum
		if rowSeason <= 0 || rowEpisode <= 0 {
			parsedSeason, parsedEpisode := ParseEpisode(row.Path)
			if rowSeason <= 0 {
				rowSeason = parsedSeason
			}
			if rowEpisode <= 0 {
				rowEpisode = parsedEpisode
			}
		}
		if rowSeason <= 0 {
			rowSeason = 1
		}
		if rowEpisode == wantEpisode && rowSeason == wantSeason {
			return true
		}
		if rowEpisode <= 0 && isSeriesPackTitle(row.Title+" "+row.OriginalName+" "+row.Path) {
			return true
		}
	}
	return false
}

func localAvailabilityTitleCandidates(title string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 6)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	add(availabilityQuery(title, ""))
	if cleaned, _ := CleanQuery(title); cleaned != "" {
		for _, candidate := range titleCandidates(cleaned) {
			add(candidate)
			fields := strings.Fields(candidate)
			for i := len(fields) - 1; i >= 1; i-- {
				prefix := strings.Join(fields[:i], " ")
				if containsCJK(prefix) {
					add(prefix)
				}
			}
		}
	}
	return out
}

func (d *DownloadService) findExistingDownloadTask(ctx context.Context, title string, allowDeletedReadd bool) (*model.DownloadTask, bool) {
	key := downloadTaskIdentityKey(title)
	if key == "" || d == nil || d.repo == nil || d.repo.Download == nil {
		return nil, false
	}
	terminalStatuses := downloadTaskNonBlockingStatuses(allowDeletedReadd)
	if d.repo.DB != nil && d.repo.DB.Migrator().HasColumn(&model.DownloadTask{}, "identity_key") {
		var row model.DownloadTask
		err := d.repo.DB.WithContext(ctx).
			Where("identity_key = ? AND LOWER(status) NOT IN ?", key, terminalStatuses).
			Order("created_at desc").
			First(&row).Error
		if err == nil {
			return &row, true
		}
	}
	rows, err := d.findExistingDownloadTaskFallbackRows(ctx, terminalStatuses)
	if err != nil {
		return nil, false
	}
	for i := range rows {
		current := strings.TrimSpace(rows[i].IdentityKey)
		if current == "" {
			current = downloadTaskIdentityKey(rows[i].Title)
		}
		if current == key || strings.Contains(current, key) || strings.Contains(key, current) {
			return &rows[i], true
		}
	}
	return nil, false
}

func (d *DownloadService) findExistingDownloadTaskFallbackRows(ctx context.Context, terminalStatuses []string) ([]model.DownloadTask, error) {
	if d == nil || d.repo == nil || d.repo.DB == nil {
		return nil, nil
	}
	q := d.repo.DB.WithContext(ctx).Model(&model.DownloadTask{}).
		Where("LOWER(status) NOT IN ?", terminalStatuses).
		Order("created_at desc").
		Limit(500)
	if d.repo.DB.Migrator().HasColumn(&model.DownloadTask{}, "identity_key") {
		q = q.Where("identity_key = '' OR identity_key IS NULL")
	}
	var rows []model.DownloadTask
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func downloadTaskNonBlockingStatuses(_ bool) []string {
	return []string{"failed", "error", "deleted", "removed", "cancelled", "canceled"}
}

func downloadTaskBlocksDuplicate(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "deleted", "removed", "cancelled", "canceled":
		return false
	default:
		return true
	}
}

func downloadTaskBlocksReadd(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "deleted", "removed", "cancelled", "canceled":
		return false
	default:
		return true
	}
}

func (d *DownloadService) torrentExistsByIdentity(ctx context.Context, title string) bool {
	query := downloadTaskIdentityKey(title)
	if query == "" {
		return false
	}
	live, err := d.qb.List(ctx, "")
	if err != nil {
		return false
	}
	for _, torrent := range live {
		current := downloadTaskIdentityKey(torrent.Name)
		if current == "" {
			continue
		}
		if current == query || strings.Contains(current, query) || strings.Contains(query, current) {
			return true
		}
	}
	return false
}

func torrentExistsInListByIdentity(live []QBitTorrent, title string) bool {
	_, ok := findTorrentInListByIdentity(live, title)
	return ok
}

func findLikelyPreparedTorrentByIdentity(live []QBitTorrent, title string) (QBitTorrent, bool) {
	torrent, ok := findTorrentInListByIdentity(live, title)
	if !ok || !isLikelyPreparedTorrent(torrent) {
		return QBitTorrent{}, false
	}
	return torrent, true
}

func findTorrentInListByIdentity(live []QBitTorrent, title string) (QBitTorrent, bool) {
	query := downloadTaskIdentityKey(title)
	if query == "" {
		return QBitTorrent{}, false
	}
	for _, torrent := range live {
		current := downloadTaskIdentityKey(torrent.Name)
		if current == "" {
			continue
		}
		if current == query || strings.Contains(current, query) || strings.Contains(query, current) {
			return torrent, true
		}
	}
	return QBitTorrent{}, false
}

func isLikelyPreparedTorrent(torrent QBitTorrent) bool {
	state := strings.ToLower(strings.TrimSpace(torrent.State))
	if state == "" {
		return false
	}
	if torrent.Progress > 0.0001 {
		return false
	}
	return strings.Contains(state, "pause") || strings.Contains(state, "stop")
}

func downloadTaskIdentityKey(name string) string {
	if key := downloadMediaIdentityKey(name); key != "" {
		return key
	}
	return normalizedDownloadTitleKey(name)
}

func downloadMediaIdentityKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	title, year := CleanQuery(name)
	titleKey := normalizeAvailabilityComparable(title)
	if titleKey == "" {
		titleKey = normalizeAvailabilityComparable(availabilityQuery(name, ""))
	}
	if titleKey == "" {
		return ""
	}
	season, episode := ParseEpisode(name)
	parts := []string{titleKey}
	if year > 0 {
		parts = append(parts, fmt.Sprintf("y%d", year))
	}
	if episode > 0 {
		if season <= 0 {
			season = 1
		}
		parts = append(parts, fmt.Sprintf("s%02de%03d", season, episode))
	}
	return strings.Join(parts, "|")
}

func normalizedDownloadTitleKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (d *DownloadService) createTask(ctx context.Context, userID, urlStr, savePath string, meta DownloadTaskMeta) (*model.DownloadTask, error) {
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = publicDownloadTitle(urlStr)
	}
	t := &model.DownloadTask{
		UserID:               userID,
		SubscriptionID:       strings.TrimSpace(meta.SubscriptionID),
		Source:               "qbittorrent",
		URL:                  urlStr,
		Title:                title,
		IdentityKey:          downloadTaskIdentityKey(firstNonEmpty(meta.IdentityTitle, title)),
		PosterURL:            meta.PosterURL,
		BackdropURL:          meta.BackdropURL,
		Overview:             meta.Overview,
		IMDBID:               NormalizeIMDBID(meta.IMDBID),
		TMDbID:               meta.TMDbID,
		DoubanID:             NormalizeDoubanID(meta.DoubanID),
		SavePath:             savePath,
		MediaType:            meta.MediaType,
		MediaCategory:        meta.MediaCategory,
		OriginalName:         meta.OriginalName,
		OriginalLanguage:     meta.OriginalLanguage,
		Year:                 meta.Year,
		Rating:               meta.Rating,
		Genres:               meta.Genres,
		Status:               "queued",
		AllowExistingLibrary: meta.AllowExistingLibrary,
	}
	if err := d.repo.Download.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func publicDownloadTitle(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "下载任务"
	}
	if u, err := url.Parse(raw); err == nil {
		if dn := strings.TrimSpace(u.Query().Get("dn")); dn != "" {
			if decoded, err := url.QueryUnescape(dn); err == nil && strings.TrimSpace(decoded) != "" {
				return strings.TrimSpace(decoded)
			}
			return dn
		}
		if u.Host != "" {
			base := path.Base(u.Path)
			if base != "." && base != "/" && base != "" {
				base = strings.TrimSuffix(base, path.Ext(base))
				if base != "" {
					return base
				}
			}
			return u.Host
		}
	}
	if strings.HasPrefix(strings.ToLower(raw), "magnet:") {
		return "磁力下载"
	}
	return "下载任务"
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
		current := normalizeTorrentName(torrent.Name)
		if current == "" {
			continue
		}
		if strings.Contains(current, query) || strings.Contains(query, current) {
			return true
		}
	}
	return false
}

func normalizeTorrentName(name string) string {
	name = torrentEpisodeToken.ReplaceAllString(strings.ToLower(name), "")
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
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

func DownloadViews(rows []model.DownloadTask, live []QBitTorrent) ([]DownloadTaskView, []DownloadTorrentView) {
	liveByKey := map[string]QBitTorrent{}
	for _, torrent := range live {
		key := normalizeTorrentName(torrent.Name)
		if key != "" {
			liveByKey[key] = torrent
		}
	}
	taskByKey := map[string]model.DownloadTask{}
	for _, row := range rows {
		key := normalizeTorrentName(row.Title)
		if key != "" {
			taskByKey[key] = row
		}
	}

	taskViews := make([]DownloadTaskView, 0, len(rows))
	for _, row := range rows {
		view := downloadTaskView(row, QBitTorrent{})
		if torrent, ok := findMatchingTorrent(row.Title, liveByKey); ok {
			view = downloadTaskView(row, torrent)
		}
		taskViews = append(taskViews, view)
	}

	torrentViews := make([]DownloadTorrentView, 0, len(live))
	for _, torrent := range live {
		var row model.DownloadTask
		if matched, ok := findMatchingTask(torrent.Name, taskByKey); ok {
			row = matched
		}
		torrentViews = append(torrentViews, downloadTorrentView(torrent, row))
	}
	sort.SliceStable(torrentViews, func(i, j int) bool {
		return torrentViews[i].AddedOn > torrentViews[j].AddedOn
	})
	return taskViews, torrentViews
}

func downloadTaskView(row model.DownloadTask, torrent QBitTorrent) DownloadTaskView {
	progress := row.Progress
	state := row.Status
	if torrent.Name != "" {
		progress = torrent.Progress
		state = torrent.State
	}
	size := torrent.Size
	return DownloadTaskView{
		ID:            row.ID,
		Source:        row.Source,
		Title:         firstNonEmpty(row.Title, "下载任务"),
		PosterURL:     row.PosterURL,
		BackdropURL:   row.BackdropURL,
		Overview:      row.Overview,
		SavePath:      row.SavePath,
		MediaType:     row.MediaType,
		MediaCategory: row.MediaCategory,
		Status:        row.Status,
		Progress:      progress,
		State:         state,
		DLSpeed:       torrent.DLSpeed,
		UpSpeed:       torrent.UpSpeed,
		Size:          size,
		Downloaded:    downloadedBytes(size, progress),
		NumSeeds:      torrent.NumSeeds,
		NumLeechs:     torrent.NumLeech,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func downloadTorrentView(torrent QBitTorrent, row model.DownloadTask) DownloadTorrentView {
	title := torrent.Name
	if row.Title != "" {
		title = row.Title
	}
	return DownloadTorrentView{
		Hash:          torrent.Hash,
		Name:          torrent.Name,
		Title:         firstNonEmpty(title, "下载任务"),
		PosterURL:     row.PosterURL,
		BackdropURL:   row.BackdropURL,
		Overview:      row.Overview,
		MediaType:     row.MediaType,
		MediaCategory: firstNonEmpty(row.MediaCategory, torrent.Category),
		State:         torrent.State,
		Progress:      torrent.Progress,
		DLSpeed:       torrent.DLSpeed,
		UpSpeed:       torrent.UpSpeed,
		NumSeeds:      torrent.NumSeeds,
		NumLeechs:     torrent.NumLeech,
		Size:          torrent.Size,
		Downloaded:    downloadedBytes(torrent.Size, torrent.Progress),
		SavePath:      torrent.SavePath,
		AddedOn:       torrent.AddedOn,
	}
}

func findMatchingTorrent(title string, liveByKey map[string]QBitTorrent) (QBitTorrent, bool) {
	key := normalizeTorrentName(title)
	if key == "" {
		return QBitTorrent{}, false
	}
	if torrent, ok := liveByKey[key]; ok {
		return torrent, true
	}
	for currentKey, torrent := range liveByKey {
		if strings.Contains(currentKey, key) || strings.Contains(key, currentKey) {
			return torrent, true
		}
	}
	return QBitTorrent{}, false
}

func findMatchingTask(title string, taskByKey map[string]model.DownloadTask) (model.DownloadTask, bool) {
	key := normalizeTorrentName(title)
	if key == "" {
		return model.DownloadTask{}, false
	}
	if row, ok := taskByKey[key]; ok {
		return row, true
	}
	for currentKey, row := range taskByKey {
		if strings.Contains(key, currentKey) || strings.Contains(currentKey, key) {
			return row, true
		}
	}
	return model.DownloadTask{}, false
}

func downloadedBytes(size int64, progress float32) int64 {
	if size <= 0 || progress <= 0 {
		return 0
	}
	if progress > 1 {
		progress = 1
	}
	return int64(math.Round(float64(size) * float64(progress)))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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

// poll fans out qBittorrent /torrents/info every 5 s as WS events. The
// payload is opaque to the client; the React store merges by hash.
func (d *DownloadService) poll(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	// prevStates tracks previous completion states to detect "just finished"
	if d.prevStates == nil {
		d.prevStates = make(map[string]bool)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-t.C:
		}
		live, err := d.qb.List(ctx, "")
		if err != nil {
			continue
		}
		rows, _ := d.repo.Download.List(ctx)
		taskByKey := tasksByTorrentIdentity(rows)
		d.processDownloadSnapshot(ctx, live, taskByKey)
		d.hub.Publish("download", map[string]any{"torrents": live})
	}
}

func (d *DownloadService) processDownloadSnapshot(ctx context.Context, live []QBitTorrent, taskByKey map[string]model.DownloadTask) {
	d.mu.Lock()
	if d.prevStates == nil {
		d.prevStates = make(map[string]bool)
	}
	firstSnapshot := !d.pollInitialized
	if firstSnapshot {
		d.pollInitialized = true
	}
	d.mu.Unlock()

	for _, torrent := range live {
		stateKey := completedTorrentQueueKey(torrent)
		complete := torrent.Progress >= 1.0
		matchedTask, hasTask := findMatchingTaskByTorrentIdentity(torrent.Name, taskByKey)
		autoOrganize := d.downloadAutoOrganizeEnabled(ctx)
		catchupRecorded := hasTask && d.completedTorrentCatchupRecorded(ctx, torrent)
		taskNeedsOrganize := hasTask && !catchupRecorded &&
			autoOrganize &&
			(downloadTaskNeedsCompletion(matchedTask) || recentlyCompletedTorrent(torrent, time.Now()))
		d.syncDownloadTaskProgress(ctx, torrent, taskByKey)
		if stateKey == "" {
			continue
		}

		shouldQueue := false
		d.mu.Lock()
		wasComplete, wasSeen := d.prevStates[stateKey]
		switch {
		case complete && (firstSnapshot || !wasSeen):
			// 首次快照里已完成的种子：此前一律标记「已见过」并跳过整理，
			// 导致「下载完成时应用恰好不在线/正在重启」的种子永远不会被
			// 自动整理入库。现在对最近完成的种子补一次整理
			// （onTorrentComplete 内部仍受 organize.auto 开关约束，且
			// 整理对已存在的目标文件幂等跳过）。
			d.prevStates[stateKey] = true
			if taskNeedsOrganize {
				shouldQueue = true
			}
		case complete && !wasComplete:
			shouldQueue = true
		case complete && taskNeedsOrganize:
			shouldQueue = true
		case complete:
			d.prevStates[stateKey] = true
		default:
			d.prevStates[stateKey] = false
		}
		d.mu.Unlock()

		if shouldQueue && d.enqueueCompletedTorrent(torrent) {
			d.mu.Lock()
			d.prevStates[stateKey] = true
			d.mu.Unlock()
		}
	}
}

func (d *DownloadService) startAutoOrganizeWorker(ctx context.Context) {
	d.mu.Lock()
	if d.organizeQueue == nil {
		d.organizeQueue = make(chan QBitTorrent, completedTorrentOrganizeQueueSize)
	}
	if d.organizeQueued == nil {
		d.organizeQueued = make(map[string]struct{})
	}
	d.mu.Unlock()
	d.organizeOnce.Do(func() {
		go d.autoOrganizeWorker(ctx)
	})
}

func (d *DownloadService) enqueueCompletedTorrent(torrent QBitTorrent) bool {
	key := completedTorrentQueueKey(torrent)
	if key == "" {
		return false
	}
	d.mu.Lock()
	if d.organizeQueue == nil {
		d.organizeQueue = make(chan QBitTorrent, completedTorrentOrganizeQueueSize)
	}
	if d.organizeQueued == nil {
		d.organizeQueued = make(map[string]struct{})
	}
	if _, ok := d.organizeQueued[key]; ok {
		d.mu.Unlock()
		return true
	}
	select {
	case d.organizeQueue <- torrent:
		d.organizeQueued[key] = struct{}{}
		d.mu.Unlock()
		return true
	default:
		d.mu.Unlock()
		if d.log != nil {
			d.log.Warn("auto organize queue full; will retry completed torrent later",
				zap.String("hash", torrent.Hash),
				zap.String("name", torrent.Name))
		}
		return false
	}
}

func (d *DownloadService) autoOrganizeWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case torrent := <-d.organizeQueue:
			d.onTorrentComplete(ctx, torrent)
			d.markCompletedTorrentOrganizeDone(torrent)
			if completedTorrentOrganizeCooldown <= 0 {
				continue
			}
			timer := time.NewTimer(completedTorrentOrganizeCooldown)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-d.stopCh:
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

func (d *DownloadService) markCompletedTorrentOrganizeDone(torrent QBitTorrent) {
	key := completedTorrentQueueKey(torrent)
	if key == "" {
		return
	}
	d.mu.Lock()
	delete(d.organizeQueued, key)
	d.mu.Unlock()
}

// completedTorrentCatchupWindow 限定重启补整理只覆盖最近完成的种子，
// 防止每次启动都把全部历史种子重新过一遍整理流程。
const completedTorrentCatchupWindow = 24 * time.Hour

const completedTorrentCatchupSettingPrefix = "download.auto_organized."
const completedTorrentNotifySettingPrefix = "download.completed_notified."

func (d *DownloadService) downloadAutoOrganizeEnabled(ctx context.Context) bool {
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return false
	}
	if v, err := d.repo.Setting.Get(ctx, "organizer.auto_after_download"); err == nil && parseBoolSetting(v, false) {
		return true
	}
	if v, err := d.repo.Setting.Get(ctx, "organize.auto"); err == nil && parseBoolSetting(v, false) {
		return true
	}
	return false
}

// recentlyCompletedTorrent 报告该种子是否在补整理时间窗内完成。
// qBittorrent 未提供 completion_on 时保守地返回 false。
func recentlyCompletedTorrent(torrent QBitTorrent, now time.Time) bool {
	if torrent.CompletionOn <= 0 {
		return false
	}
	completed := time.Unix(torrent.CompletionOn, 0)
	return now.Sub(completed) <= completedTorrentCatchupWindow
}

func (d *DownloadService) completedTorrentCatchupRecorded(ctx context.Context, torrent QBitTorrent) bool {
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return false
	}
	key := completedTorrentCatchupSettingKey(torrent)
	if key == "" {
		return false
	}
	value, err := d.repo.Setting.Get(ctx, key)
	if err != nil {
		return false
	}
	return parseBoolSetting(value, false)
}

func (d *DownloadService) markCompletedTorrentCatchupRecorded(ctx context.Context, torrent QBitTorrent) {
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return
	}
	key := completedTorrentCatchupSettingKey(torrent)
	if key == "" {
		return
	}
	if err := d.repo.Setting.Set(ctx, key, "true"); err != nil && d.log != nil {
		d.log.Debug("mark completed torrent catchup failed",
			zap.String("hash", torrent.Hash),
			zap.String("name", torrent.Name),
			zap.Error(err))
	}
}

func completedTorrentCatchupSettingKey(torrent QBitTorrent) string {
	key := completedTorrentQueueKey(torrent)
	if key == "" {
		return ""
	}
	sum := sha1.Sum([]byte(key))
	return completedTorrentCatchupSettingPrefix + fmt.Sprintf("%x", sum[:])
}

func (d *DownloadService) completedTorrentNotified(ctx context.Context, torrent QBitTorrent) bool {
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return false
	}
	key := completedTorrentNotifySettingKey(torrent)
	if key == "" {
		return false
	}
	value, err := d.repo.Setting.Get(ctx, key)
	if err != nil {
		return false
	}
	return parseBoolSetting(value, false)
}

func (d *DownloadService) markCompletedTorrentNotified(ctx context.Context, torrent QBitTorrent) {
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return
	}
	key := completedTorrentNotifySettingKey(torrent)
	if key == "" {
		return
	}
	if err := d.repo.Setting.Set(ctx, key, "true"); err != nil && d.log != nil {
		d.log.Debug("mark completed torrent notification failed",
			zap.String("hash", torrent.Hash),
			zap.String("name", torrent.Name),
			zap.Error(err))
	}
}

func completedTorrentNotifySettingKey(torrent QBitTorrent) string {
	key := completedTorrentQueueKey(torrent)
	if key == "" {
		return ""
	}
	sum := sha1.Sum([]byte(key))
	return completedTorrentNotifySettingPrefix + fmt.Sprintf("%x", sum[:])
}

func completedTorrentQueueKey(torrent QBitTorrent) string {
	hash := strings.ToLower(strings.TrimSpace(torrent.Hash))
	if hash != "" {
		return hash
	}
	parts := []string{torrent.Name, torrent.ContentPath, torrent.SavePath}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	key := strings.Join(parts, "|")
	if strings.Trim(key, "|") == "" {
		return ""
	}
	return strings.ToLower(key)
}

func (d *DownloadService) syncDownloadTaskProgress(ctx context.Context, torrent QBitTorrent, taskByKey map[string]model.DownloadTask) {
	if d == nil || d.repo == nil || d.repo.DB == nil || strings.TrimSpace(torrent.Name) == "" {
		return
	}
	matched, ok := findMatchingTaskByTorrentIdentity(torrent.Name, taskByKey)
	if !ok {
		return
	}
	status := torrent.State
	if torrent.Progress >= 1 {
		status = "completed"
	}
	if strings.TrimSpace(status) == "" {
		status = matched.Status
	}
	updates := map[string]any{}
	if math.Abs(float64(matched.Progress-torrent.Progress)) > 0.0001 {
		updates["progress"] = torrent.Progress
	}
	if status != "" && status != matched.Status {
		updates["status"] = status
	}
	if len(updates) == 0 {
		return
	}
	_ = d.repo.DB.WithContext(ctx).Model(&model.DownloadTask{}).Where("id = ?", matched.ID).Updates(updates).Error
}

func tasksByIdentity(rows []model.DownloadTask) map[string]model.DownloadTask {
	out := make(map[string]model.DownloadTask, len(rows))
	for _, row := range rows {
		key := strings.TrimSpace(row.IdentityKey)
		if key == "" {
			key = downloadTaskIdentityKey(row.Title)
		}
		if key != "" {
			out[key] = row
		}
	}
	return out
}

func tasksByTorrentIdentity(rows []model.DownloadTask) map[string]model.DownloadTask {
	out := make(map[string]model.DownloadTask, len(rows))
	for _, row := range rows {
		key := normalizeTorrentName(row.Title)
		if key != "" {
			out[key] = row
		}
	}
	return out
}

func findMatchingTaskByIdentity(title string, taskByKey map[string]model.DownloadTask) (model.DownloadTask, bool) {
	key := downloadTaskIdentityKey(title)
	if key == "" {
		return model.DownloadTask{}, false
	}
	if row, ok := taskByKey[key]; ok {
		return row, true
	}
	for currentKey, row := range taskByKey {
		if strings.Contains(key, currentKey) || strings.Contains(currentKey, key) {
			return row, true
		}
	}
	return model.DownloadTask{}, false
}

func findMatchingTaskByTorrentIdentity(title string, taskByKey map[string]model.DownloadTask) (model.DownloadTask, bool) {
	key := normalizeTorrentName(title)
	if key == "" {
		return model.DownloadTask{}, false
	}
	if row, ok := taskByKey[key]; ok {
		return row, true
	}
	for currentKey, row := range taskByKey {
		if strings.Contains(key, currentKey) || strings.Contains(currentKey, key) {
			return row, true
		}
	}
	return model.DownloadTask{}, false
}

func downloadTaskNeedsCompletion(task model.DownloadTask) bool {
	if task.Progress < 1 {
		return true
	}
	return strings.ToLower(strings.TrimSpace(task.Status)) != "completed"
}

// onTorrentComplete handles a torrent that just finished downloading.
// It organizes the completed torrent payload directly. Relying on existing
// Media rows is too late for freshly-downloaded files: they usually have not
// been scanned into the library yet.
func (d *DownloadService) onTorrentComplete(ctx context.Context, torrent QBitTorrent) {
	taskRow, hasTask := d.completedTorrentTask(ctx, torrent)
	d.notifyDownloadComplete(ctx, torrent, taskRow)
	if d.organizer == nil {
		return
	}
	// 仅当显式开启 organizer.auto_after_download / organize.auto 时才在下载完成后整理。
	// 之前的代码错误地把 organizer.smart_classify 也当成"自动整理"开关，
	// 让操作员只想启用"分类子目录"就被动触发了文件 move。
	autoOrganize := d.downloadAutoOrganizeEnabled(ctx)
	if !autoOrganize {
		d.log.Info("download completed, auto-organize disabled", zap.String("hash", torrent.Hash))
		return
	}
	source := d.completedTorrentSource(ctx, torrent)
	if source == "" {
		d.log.Warn("download completed but payload path is not accessible",
			zap.String("hash", torrent.Hash),
			zap.String("name", torrent.Name),
			zap.String("save_path", torrent.SavePath),
			zap.String("content_path", torrent.ContentPath))
		return
	}
	allowReplace := hasTask && taskRow.AllowExistingLibrary
	d.log.Info("download completed, triggering directory organize",
		zap.String("hash", torrent.Hash),
		zap.String("name", torrent.Name),
		zap.String("source", source),
		zap.Bool("allow_replace_existing", allowReplace))
	resWrap, err := d.ensureOrganizePipeline().Run(ctx, OrganizePipelineRequest{
		Scope:         OrganizeScopeDirectory,
		Trigger:       OrganizeTriggerDownload,
		TaskName:      d.downloadOrganizeTaskName(torrent, allowReplace),
		SourcePath:    source,
		MediaType:     downloadTaskMediaType(taskRow),
		MediaCategory: firstNonEmpty(downloadTaskMediaCategory(taskRow), torrent.Category),
		AllowReplace:  allowReplace,
	})
	if err != nil {
		d.log.Error("auto organize completed torrent failed",
			zap.String("hash", torrent.Hash),
			zap.String("source", source),
			zap.Error(err))
		return
	}
	res := resWrap.Result
	if res == nil {
		res = &OrganizeResult{}
	}
	d.markCompletedTorrentCatchupRecorded(context.Background(), torrent)
	d.log.Info("auto organize completed torrent finished",
		zap.String("hash", torrent.Hash),
		zap.String("source", source),
		zap.String("dest", firstNonEmpty(res.DestPath, "")),
		zap.Int("organized", res.Organized),
		zap.Int("replaced", res.Replaced),
		zap.Int("skipped", res.Skipped),
		zap.Int("scrapes", len(res.Scrapes)),
		zap.Int("errors", len(res.Errors)))
}

func (d *DownloadService) notifyDownloadComplete(ctx context.Context, torrent QBitTorrent, task *model.DownloadTask) {
	if d == nil || d.notify == nil {
		return
	}
	if d.completedTorrentNotified(ctx, torrent) {
		return
	}
	d.markCompletedTorrentNotified(ctx, torrent)
	name := strings.TrimSpace(torrent.Name)
	if name == "" {
		name = strings.TrimSpace(filepath.Base(torrent.ContentPath))
	}
	if task != nil && strings.TrimSpace(task.Title) != "" {
		name = strings.TrimSpace(task.Title)
	}
	if name == "" {
		name = "下载任务"
	}
	body := fmt.Sprintf("任务：%s\n保存路径：%s\nHash：%s", name, firstNonEmpty(torrent.ContentPath, torrent.SavePath), torrent.Hash)
	data := map[string]interface{}{}
	// resource_title 供 Telegram 模板从发布名提取季集(SxxEyy)与版本(分辨率/编码/
	// 字幕组等)信息;隐藏不直接展示。优先用 torrent 原始名(信息最全)。
	if rt := strings.TrimSpace(torrent.Name); rt != "" {
		data["resource_title"] = rt
	}
	if task != nil {
		if strings.TrimSpace(task.PosterURL) != "" {
			data["poster_url"] = task.PosterURL
		}
		if strings.TrimSpace(task.BackdropURL) != "" {
			data["backdrop_url"] = task.BackdropURL
		}
		if strings.TrimSpace(task.MediaType) != "" {
			data["media_type"] = task.MediaType
		}
		if strings.TrimSpace(task.MediaCategory) != "" {
			data["media_category"] = task.MediaCategory
		}
		if strings.TrimSpace(task.Title) != "" {
			data["title"] = task.Title
		}
		if strings.TrimSpace(task.Overview) != "" {
			data["overview"] = truncateTelegramText(task.Overview, 180)
		}
		if strings.TrimSpace(task.OriginalName) != "" {
			data["original_title"] = task.OriginalName
		}
		if strings.TrimSpace(task.OriginalLanguage) != "" {
			data["original_language"] = task.OriginalLanguage
		}
		if task.Year > 0 {
			data["year"] = task.Year
		}
		if task.Rating > 0 {
			data["rating"] = task.Rating
		}
		if strings.TrimSpace(task.Genres) != "" {
			data["genres"] = task.Genres
		}
		if imdbURL := imdbExternalURL(task.IMDBID); imdbURL != "" {
			data["imdb_url"] = imdbURL
		}
		if tmdbURL := tmdbExternalURL(task.TMDbID, task.MediaType); tmdbURL != "" {
			data["tmdb_url"] = tmdbURL
		}
		if doubanURL := doubanExternalURL(task.DoubanID); doubanURL != "" {
			data["douban_url"] = doubanURL
		}
		if sub := d.downloadTaskSubscription(ctx, task); sub != nil {
			mergeNotifyData(data, subscriptionNotifyData(sub))
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d.notify.BroadcastEvent(ctx, NotifyEvent{
			Type:    EventDownloadComplete,
			Title:   "MediaStationGo 下载完成",
			Message: body,
			Data:    data,
		})
	}()
}

func (d *DownloadService) downloadTaskSubscription(ctx context.Context, task *model.DownloadTask) *model.Subscription {
	if d == nil || d.repo == nil || d.repo.DB == nil || task == nil || strings.TrimSpace(task.SubscriptionID) == "" {
		return nil
	}
	var sub model.Subscription
	if err := d.repo.DB.WithContext(ctx).First(&sub, "id = ?", task.SubscriptionID).Error; err != nil {
		return nil
	}
	return &sub
}

func (d *DownloadService) downloadOrganizeTaskName(torrent QBitTorrent, allowReplace bool) string {
	name := strings.TrimSpace(torrent.Name)
	if name == "" {
		name = "下载完成自动整理"
	}
	if allowReplace {
		name += "（允许洗版）"
	}
	return name
}

func (d *DownloadService) ensureOrganizePipeline() *OrganizePipelineService {
	if d.organizePipeline != nil {
		return d.organizePipeline
	}
	return NewOrganizePipelineService(d.log, d.repo, d.organizer, d.scanner, d.tasks)
}

func (d *DownloadService) completedTorrentTask(ctx context.Context, torrent QBitTorrent) (*model.DownloadTask, bool) {
	if d == nil || d.repo == nil || d.repo.Download == nil {
		return nil, false
	}
	rows, err := d.repo.Download.List(ctx)
	if err != nil || len(rows) == 0 {
		return nil, false
	}
	taskByKey := tasksByTorrentIdentity(rows)
	if task, ok := findMatchingTaskByTorrentIdentity(torrent.Name, taskByKey); ok {
		return &task, true
	}
	if strings.TrimSpace(torrent.ContentPath) != "" {
		if task, ok := findMatchingTaskByTorrentIdentity(filepath.Base(torrent.ContentPath), taskByKey); ok {
			return &task, true
		}
	}
	return nil, false
}

func downloadTaskMediaType(task *model.DownloadTask) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.MediaType)
}

func downloadTaskMediaCategory(task *model.DownloadTask) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.MediaCategory)
}

// DownloadPathMappingsSettingKey 允许用户自定义「下载器路径 → 本程序路径」
// 映射，每行一条，格式 `客户端路径=本地路径`（也接受 `=>` 或单个 `:` 分隔）。
// qBittorrent 与本程序常在不同容器/主机里，对同一份数据看到的路径不同；
// 此前映射表是写死的三条猜测，对不上时整理静默失败。
const DownloadPathMappingsSettingKey = "download.path_mappings"

func (d *DownloadService) completedTorrentSource(ctx context.Context, torrent QBitTorrent) string {
	// 常见路径映射：qBittorrent容器路径 -> MediaStationGo容器路径
	mappings := map[string]string{
		"/var/apps/qBittorrent/shares/qBittorrent/Download": "/downloads",
		"/data/qBittorrent/downloads":                       "/downloads",
		"/downloads/qBittorrent":                            "/downloads",
	}
	// 用户自定义映射优先（可覆盖内置猜测）。
	for clientPrefix, localPrefix := range d.userPathMappings(ctx) {
		mappings[clientPrefix] = localPrefix
	}
	for _, candidate := range []string{
		torrent.ContentPath,
		filepath.Join(torrent.SavePath, torrent.Name),
	} {
		clean := strings.TrimSpace(candidate)
		if clean == "" || clean == "." {
			continue
		}
		// 尝试直接访问或路径映射
		if translated := translateClientPath(clean, mappings); translated != "" {
			return translated
		}
		// 复用 compose 注入的 MEDIASTATION_DOWNLOAD_DIR/MEDIA_DIR 宿主机↔容器
		// 映射（与媒体库路径换算同一套规则），覆盖「qB 跑在宿主机、
		// 本程序在容器里」的最常见部署形态。
		for _, mapped := range mappedPathCandidates(clean) {
			if mapped == clean {
				continue
			}
			if _, err := os.Stat(mapped); err == nil {
				return mapped
			}
		}
	}
	return ""
}

// userPathMappings 解析用户配置的下载器路径映射。
func (d *DownloadService) userPathMappings(ctx context.Context) map[string]string {
	out := map[string]string{}
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return out
	}
	raw, err := d.repo.Setting.Get(ctx, DownloadPathMappingsSettingKey)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var from, to string
		switch {
		case strings.Contains(line, "=>"):
			parts := strings.SplitN(line, "=>", 2)
			from, to = parts[0], parts[1]
		case strings.Contains(line, "="):
			parts := strings.SplitN(line, "=", 2)
			from, to = parts[0], parts[1]
		case strings.Count(line, ":") == 1:
			parts := strings.SplitN(line, ":", 2)
			from, to = parts[0], parts[1]
		default:
			continue
		}
		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)
		if from != "" && to != "" {
			out[from] = to
		}
	}
	return out
}
