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
	"math"
	"net/url"
	"path"
	"regexp"
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
	log       *zap.Logger
	repo      *repository.Container
	hub       *Hub
	qb        *QBitClient
	organizer *OrganizerService
	site      *SiteService

	mu         sync.Mutex
	stopCh     chan struct{}
	pollOnce   sync.Once
	prevStates map[string]bool // hash -> wasCompleted
}

var torrentEpisodeToken = regexp.MustCompile(`(?i)e\d{1,3}`)

// DownloadTaskMeta carries public display metadata for a download. It is
// deliberately separate from the private torrent URL so API responses never
// need to expose tracker tokens.
type DownloadTaskMeta struct {
	Title       string
	PosterURL   string
	BackdropURL string
	Overview    string
}

type DownloadTaskView struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	Title       string    `json:"title"`
	PosterURL   string    `json:"poster_url,omitempty"`
	BackdropURL string    `json:"backdrop_url,omitempty"`
	Overview    string    `json:"overview,omitempty"`
	SavePath    string    `json:"save_path"`
	Status      string    `json:"status"`
	Progress    float32   `json:"progress"`
	State       string    `json:"state,omitempty"`
	DLSpeed     int64     `json:"dlspeed,omitempty"`
	UpSpeed     int64     `json:"upspeed,omitempty"`
	Size        int64     `json:"size,omitempty"`
	Downloaded  int64     `json:"downloaded,omitempty"`
	NumSeeds    int       `json:"num_seeds,omitempty"`
	NumLeechs   int       `json:"num_leechs,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DownloadTorrentView struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	Title       string  `json:"title"`
	PosterURL   string  `json:"poster_url,omitempty"`
	BackdropURL string  `json:"backdrop_url,omitempty"`
	Overview    string  `json:"overview,omitempty"`
	State       string  `json:"state"`
	Progress    float32 `json:"progress"`
	DLSpeed     int64   `json:"dlspeed"`
	UpSpeed     int64   `json:"upspeed"`
	NumSeeds    int     `json:"num_seeds"`
	NumLeechs   int     `json:"num_leechs"`
	Size        int64   `json:"size"`
	Downloaded  int64   `json:"downloaded"`
	SavePath    string  `json:"save_path"`
}

// NewDownloadService is the constructor.
func NewDownloadService(log *zap.Logger, repo *repository.Container, hub *Hub, organizer *OrganizerService, site ...*SiteService) *DownloadService {
	var siteSvc *SiteService
	if len(site) > 0 {
		siteSvc = site[0]
	}
	return &DownloadService{
		log:        log,
		repo:       repo,
		hub:        hub,
		qb:         NewQBitClient(log, QBitConfig{}),
		organizer:  organizer,
		site:       siteSvc,
		prevStates: make(map[string]bool),
		stopCh:     make(chan struct{}),
	}
}

// Start kicks off the background poller (idempotent).
func (d *DownloadService) Start(ctx context.Context) {
	d.pollOnce.Do(func() {
		_ = d.ReloadConfig(ctx)
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

	// Path 1: download_clients 表
	if d.repo.DownloadClient != nil {
		if c, err := d.repo.DownloadClient.FindDefault(ctx); err == nil && c != nil && c.Type == "qbittorrent" {
			cfg.BaseURL = strings.TrimRight(c.Host, "/")
			cfg.Username = c.Username
			cfg.Password = c.Password
		}
	}

	// Path 2: legacy Setting 表（仅在 client 表未配置时回退）
	if cfg.BaseURL == "" {
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

// AddDownload accepts a magnet URL / HTTP URL and persists a tracking row.
func (d *DownloadService) AddDownload(ctx context.Context, userID, urlStr, savePath string) (*model.DownloadTask, error) {
	return d.AddDownloadWithMeta(ctx, userID, urlStr, savePath, DownloadTaskMeta{})
}

func (d *DownloadService) AddDownloadWithMeta(ctx context.Context, userID, urlStr, savePath string, meta DownloadTaskMeta) (*model.DownloadTask, error) {
	if urlStr == "" {
		return nil, errors.New("empty url")
	}
	if savePath == "" {
		savePath, _ = d.repo.Setting.Get(ctx, "qbittorrent.savepath")
	}
	var siteFetchErr error
	if d.site != nil {
		if data, name, err := d.site.FetchTorrentFile(ctx, urlStr); err == nil {
			if err := d.qb.AddTorrentFile(ctx, data, name, savePath); err != nil {
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
	if err := d.qb.AddTorrent(ctx, urlStr, savePath); err != nil {
		if siteFetchErr != nil && !strings.Contains(siteFetchErr.Error(), "no matching PT site") {
			return nil, errors.Join(err, siteFetchErr)
		}
		return nil, err
	}
	return d.createTask(ctx, userID, urlStr, savePath, meta)
}

func (d *DownloadService) createTask(ctx context.Context, userID, urlStr, savePath string, meta DownloadTaskMeta) (*model.DownloadTask, error) {
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = publicDownloadTitle(urlStr)
	}
	t := &model.DownloadTask{
		UserID:      userID,
		Source:      "qbittorrent",
		URL:         urlStr,
		Title:       title,
		PosterURL:   meta.PosterURL,
		BackdropURL: meta.BackdropURL,
		Overview:    meta.Overview,
		SavePath:    savePath,
		Status:      "queued",
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
		ID:          row.ID,
		Source:      row.Source,
		Title:       firstNonEmpty(row.Title, "下载任务"),
		PosterURL:   row.PosterURL,
		BackdropURL: row.BackdropURL,
		Overview:    row.Overview,
		SavePath:    row.SavePath,
		Status:      row.Status,
		Progress:    progress,
		State:       state,
		DLSpeed:     torrent.DLSpeed,
		UpSpeed:     torrent.UpSpeed,
		Size:        size,
		Downloaded:  downloadedBytes(size, progress),
		NumSeeds:    torrent.NumSeeds,
		NumLeechs:   torrent.NumLeech,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func downloadTorrentView(torrent QBitTorrent, row model.DownloadTask) DownloadTorrentView {
	title := torrent.Name
	if row.Title != "" {
		title = row.Title
	}
	return DownloadTorrentView{
		Hash:        torrent.Hash,
		Name:        torrent.Name,
		Title:       firstNonEmpty(title, "下载任务"),
		PosterURL:   row.PosterURL,
		BackdropURL: row.BackdropURL,
		Overview:    row.Overview,
		State:       torrent.State,
		Progress:    torrent.Progress,
		DLSpeed:     torrent.DLSpeed,
		UpSpeed:     torrent.UpSpeed,
		NumSeeds:    torrent.NumSeeds,
		NumLeechs:   torrent.NumLeech,
		Size:        torrent.Size,
		Downloaded:  downloadedBytes(torrent.Size, torrent.Progress),
		SavePath:    torrent.SavePath,
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
	return d.qb.Delete(ctx, hash, withFiles)
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
		// Detect completed downloads and trigger organize
		for _, t := range live {
			hash := t.Hash
			complete := t.Progress >= 1.0
			if complete && !d.prevStates[hash] {
				// Just completed: trigger organize
				go d.onTorrentComplete(ctx, hash, t.SavePath)
			}
			d.prevStates[hash] = complete
		}
		d.hub.Publish("download", map[string]any{"torrents": live})
	}
}

// onTorrentComplete handles a torrent that just finished downloading.
// It tries to find the associated Media record and trigger organize.
func (d *DownloadService) onTorrentComplete(ctx context.Context, hash string, savePath string) {
	if d.organizer == nil || savePath == "" {
		return
	}
	// 仅当显式开启 organizer.auto_after_download / organize.auto 时才在下载完成后整理。
	// 之前的代码错误地把 organizer.smart_classify 也当成"自动整理"开关，
	// 让操作员只想启用"分类子目录"就被动触发了文件 move。
	autoOrganize := false
	if v, err := d.repo.Setting.Get(ctx, "organizer.auto_after_download"); err == nil {
		autoOrganize = v == "true" || v == "1" || v == "on"
	}
	if !autoOrganize {
		if v, err := d.repo.Setting.Get(ctx, "organize.auto"); err == nil {
			autoOrganize = v == "true" || v == "1" || v == "on"
		}
	}
	if !autoOrganize {
		d.log.Info("download completed, auto-organize disabled", zap.String("hash", hash))
		return
	}
	d.log.Info("download completed, triggering organize", zap.String("hash", hash), zap.String("save_path", savePath))
	// Find Media record by path prefix
	var medias []model.Media
	if err := d.repo.DB.WithContext(ctx).Where("path LIKE ?", savePath+"%").Find(&medias).Error; err != nil {
		d.log.Error("find media by path", zap.Error(err))
		return
	}
	for i := range medias {
		if _, err := d.organizer.OrganizeMedia(ctx, medias[i].ID); err != nil {
			d.log.Error("organize media", zap.String("media_id", medias[i].ID), zap.Error(err))
		}
	}
}
