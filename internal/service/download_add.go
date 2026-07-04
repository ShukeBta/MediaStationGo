package service

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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

const preparedDownloadTTL = 30 * time.Minute

type downloadAddRequest struct {
	title        string
	savePath     string
	qbitCategory string
	meta         DownloadTaskMeta
}

// AddDownload accepts a magnet URL / HTTP URL and persists a tracking row.
func (d *DownloadService) AddDownload(ctx context.Context, userID, urlStr, savePath string) (*model.DownloadTask, error) {
	return d.AddDownloadWithMeta(ctx, userID, urlStr, savePath, DownloadTaskMeta{})
}

func (d *DownloadService) AddDownloadWithMeta(ctx context.Context, userID, urlStr, savePath string, meta DownloadTaskMeta) (*model.DownloadTask, error) {
	req, err := d.prepareDownloadAdd(ctx, urlStr, savePath, meta)
	if err != nil {
		return nil, err
	}
	if !req.meta.AllowExistingLibrary && d.localMediaAlreadyExists(ctx, req.title) {
		return nil, ErrMediaAlreadyInLibrary
	}
	if existing, ok := d.findExistingDownloadTask(ctx, req); ok {
		d.linkExistingDownloadTaskToSubscription(ctx, existing, req)
		return existing, ErrDownloadAlreadyExists
	}
	_ = d.ReloadConfig(ctx)
	if !d.qb.IsConfigured() {
		return nil, d.defaultDownloaderNotConfiguredError(ctx)
	}
	if d.torrentExistsByIdentity(ctx, req) {
		task, err := d.createTask(ctx, userID, urlStr, req.savePath, req.meta)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.meta.SubscriptionID) != "" {
			return task, nil
		}
		return task, ErrDownloadAlreadyExists
	}
	if err := d.addPreparedDownloadToClient(ctx, urlStr, &req); err != nil {
		if errors.Is(err, ErrDownloadAlreadyExists) && strings.TrimSpace(req.meta.SubscriptionID) != "" {
			return d.createTask(ctx, userID, urlStr, req.savePath, req.meta)
		}
		return nil, err
	}
	return d.createTask(ctx, userID, urlStr, req.savePath, req.meta)
}

func (d *DownloadService) prepareDownloadAdd(ctx context.Context, urlStr, savePath string, meta DownloadTaskMeta) (downloadAddRequest, error) {
	if urlStr == "" {
		return downloadAddRequest{}, errors.New("empty url")
	}
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = publicDownloadTitle(urlStr)
		meta.Title = title
	}
	identityTitle := strings.TrimSpace(meta.IdentityTitle)
	if identityTitle == "" {
		identityTitle = title
		meta.IdentityTitle = identityTitle
	}
	autoClassify := downloadSmartClassifyEnabled(ctx, d.repo, d.organizer)
	savePath, resolvedCategory := d.resolveDownloadSavePath(ctx, savePath, meta, autoClassify)
	if !autoClassify {
		meta.MediaCategory = ""
	} else if strings.TrimSpace(meta.MediaCategory) == "" {
		meta.MediaCategory = resolvedCategory
	}
	return downloadAddRequest{
		title:        identityTitle,
		savePath:     savePath,
		qbitCategory: strings.TrimSpace(meta.MediaCategory),
		meta:         meta,
	}, nil
}

func (d *DownloadService) addPreparedDownloadToClient(ctx context.Context, urlStr string, req *downloadAddRequest) error {
	var siteFetchErr error
	if d.site != nil {
		if data, name, err := d.site.FetchTorrentFile(ctx, urlStr); err == nil {
			if err := d.qb.AddTorrentFileWithCategoryAndName(ctx, data, name, req.savePath, req.qbitCategory, req.meta.Title, req.meta.SelectedFiles); err != nil {
				return err
			}
			if strings.TrimSpace(req.meta.Title) == "" {
				req.meta.Title = strings.TrimSuffix(name, path.Ext(name))
			}
			return nil
		} else {
			siteFetchErr = err
		}
	}
	if err := d.qb.AddTorrentWithCategoryAndName(ctx, urlStr, req.savePath, req.qbitCategory, req.meta.Title, req.meta.SelectedFiles); err != nil {
		if siteFetchErr != nil && !strings.Contains(siteFetchErr.Error(), "no matching PT site") {
			return errors.Join(err, siteFetchErr)
		}
		return err
	}
	return nil
}

func (d *DownloadService) PrepareDownloadWithMeta(ctx context.Context, userID, urlStr, savePath string, meta DownloadTaskMeta) (*PreparedDownload, error) {
	_ = userID
	req, err := d.prepareDownloadAdd(ctx, urlStr, savePath, meta)
	if err != nil {
		return nil, err
	}
	if !req.meta.AllowExistingLibrary && d.localMediaAlreadyExists(ctx, req.title) {
		return nil, ErrMediaAlreadyInLibrary
	}
	if existing, ok := d.findExistingDownloadTask(ctx, req); ok {
		if d.qb.IsConfigured() {
			live, liveErr := d.qb.List(ctx, "")
			if liveErr != nil || torrentExistsInListByIdentity(live, req.title) {
				_ = existing
				return nil, ErrDownloadAlreadyExists
			}
		}
	}
	_ = d.ReloadConfig(ctx)
	if !d.qb.IsConfigured() {
		return nil, d.defaultDownloaderNotConfiguredError(ctx)
	}
	live, liveErr := d.qb.List(ctx, "")
	if existing, ok := d.findExistingDownloadTask(ctx, req); ok {
		if liveErr != nil || torrentExistsInListByIdentity(live, req.title) {
			_ = existing
			return nil, ErrDownloadAlreadyExists
		}
	}
	if liveErr == nil {
		if torrent, ok := findLikelyPreparedTorrentByIdentity(live, req.title); ok {
			return d.reusePreparedTorrent(ctx, torrent.Hash, urlStr, req.savePath, req.meta)
		}
		if torrentExistsInListByIdentity(live, req.title) {
			return nil, ErrDownloadAlreadyExists
		}
	}
	var siteFetchErr error
	if d.site != nil {
		if data, name, err := d.site.FetchTorrentFile(ctx, urlStr); err == nil {
			hash, files, err := d.qb.PrepareTorrentFileWithCategoryAndName(ctx, data, name, req.savePath, req.qbitCategory, req.meta.Title)
			if err != nil {
				if errors.Is(err, ErrDownloadAlreadyExists) {
					if prepared, ok := d.reusePreparedTorrentByHash(ctx, hash, urlStr, req.savePath, req.meta); ok {
						return prepared, nil
					}
				}
				return nil, err
			}
			d.rememberPreparedDownload(hash, urlStr, req.savePath, req.meta)
			return &PreparedDownload{Hash: hash, Files: files}, nil
		} else {
			siteFetchErr = err
		}
	}
	hash, files, err := d.qb.PrepareTorrentWithCategoryAndName(ctx, urlStr, req.savePath, req.qbitCategory, req.meta.Title)
	if err != nil {
		if errors.Is(err, ErrDownloadAlreadyExists) {
			if prepared, ok := d.reusePreparedTorrentByHash(ctx, hash, urlStr, req.savePath, req.meta); ok {
				return prepared, nil
			}
		}
		if siteFetchErr != nil && !strings.Contains(siteFetchErr.Error(), "no matching PT site") {
			return nil, errors.Join(err, siteFetchErr)
		}
		return nil, err
	}
	d.rememberPreparedDownload(hash, urlStr, req.savePath, req.meta)
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
	req, err := d.prepareDownloadAdd(ctx, urlStr, savePath, meta)
	if err != nil {
		return nil, err
	}
	_ = d.ReloadConfig(ctx)
	if !d.qb.IsConfigured() {
		return nil, d.defaultDownloaderNotConfiguredError(ctx)
	}
	if len(selectedFileIndexes) > 0 {
		if err := d.qb.ApplySelectedFileIndexes(ctx, hash, selectedFileIndexes); err != nil {
			return nil, err
		}
	} else if err := d.qb.Resume(ctx, hash); err != nil {
		return nil, err
	}
	return d.createTask(ctx, userID, urlStr, req.savePath, req.meta)
}

func (d *DownloadService) CancelPreparedDownload(ctx context.Context, hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return errors.New("hash is required")
	}
	_ = d.ReloadConfig(ctx)
	if !d.qb.IsConfigured() {
		return d.defaultDownloaderNotConfiguredError(ctx)
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
	if strings.TrimSpace(current.OriginalName) == "" {
		current.OriginalName = prepared.OriginalName
	}
	if strings.TrimSpace(current.OriginalLanguage) == "" {
		current.OriginalLanguage = prepared.OriginalLanguage
	}
	if current.Year <= 0 {
		current.Year = prepared.Year
	}
	if current.Rating <= 0 {
		current.Rating = prepared.Rating
	}
	if strings.TrimSpace(current.Genres) == "" {
		current.Genres = prepared.Genres
	}
	if !current.AllowExistingLibrary {
		current.AllowExistingLibrary = prepared.AllowExistingLibrary
	}
	return current
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
	return state != "" && torrent.Progress <= 0.0001 && (strings.Contains(state, "pause") || strings.Contains(state, "stop"))
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
