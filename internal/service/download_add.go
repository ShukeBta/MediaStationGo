package service

import (
	"context"
	"errors"
	"fmt"
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
	target, err := d.defaultDownloadTarget(ctx)
	if err != nil {
		return nil, d.defaultDownloaderNotConfiguredError(ctx)
	}
	if liveTorrent, ok := d.findLiveTorrentByIdentity(ctx, urlStr, req); ok {
		existingTarget := target
		if strings.TrimSpace(liveTorrent.ClientID) != "" {
			existingTarget = downloadTarget{clientID: liveTorrent.ClientID, typ: firstNonEmpty(liveTorrent.Source, target.typ)}
		}
		task, err := d.createTask(ctx, userID, urlStr, req.savePath, req.meta, existingTarget, liveTorrent.Hash)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.meta.SubscriptionID) != "" {
			return task, nil
		}
		return task, ErrDownloadAlreadyExists
	}
	externalID, err := d.addPreparedDownloadToClient(ctx, urlStr, &req, target)
	if err != nil {
		if errors.Is(err, ErrDownloadAlreadyExists) && strings.TrimSpace(req.meta.SubscriptionID) != "" {
			return d.createTask(ctx, userID, urlStr, req.savePath, req.meta, target, externalID)
		}
		return nil, err
	}
	return d.createTask(ctx, userID, urlStr, req.savePath, req.meta, target, externalID)
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

func (d *DownloadService) addPreparedDownloadToClient(ctx context.Context, urlStr string, req *downloadAddRequest, target downloadTarget) (string, error) {
	var siteFetchErr error
	if d.site != nil {
		if data, name, err := d.site.FetchTorrentFile(ctx, urlStr); err == nil {
			return d.addTorrentFileToTarget(ctx, data, name, req, target)
		} else {
			siteFetchErr = err
		}
	}
	externalID, err := d.addTorrentURLToTarget(ctx, urlStr, req, target)
	if err != nil {
		return externalID, joinTorrentFetchError(err, siteFetchErr)
	}
	return externalID, nil
}

func (d *DownloadService) addTorrentFileToTarget(ctx context.Context, data []byte, name string, req *downloadAddRequest, target downloadTarget) (string, error) {
	if target.legacyQB {
		if err := d.qb.AddTorrentFileWithCategoryAndName(ctx, data, name, req.savePath, req.qbitCategory, req.meta.Title, req.meta.SelectedFiles); err != nil {
			return "", err
		}
		setFetchedTorrentTitle(req, name, nil)
		return torrentInfoHash(data), nil
	}
	if categorized, ok := target.adapter.(CategorizedTorrentDownloadAdapter); ok {
		externalID, err := categorized.AddTorrentFileWithCategory(ctx, data, name, req.savePath, req.qbitCategory)
		setFetchedTorrentTitle(req, name, err)
		return externalID, err
	}
	fileAdapter, ok := target.adapter.(TorrentFileDownloadAdapter)
	if !ok {
		return "", errors.New("configured downloader does not accept torrent files")
	}
	externalID, err := fileAdapter.AddTorrentFile(ctx, data, name, req.savePath)
	setFetchedTorrentTitle(req, name, err)
	return externalID, err
}

func (d *DownloadService) addTorrentURLToTarget(ctx context.Context, urlStr string, req *downloadAddRequest, target downloadTarget) (string, error) {
	if target.legacyQB {
		if err := d.qb.AddTorrentWithCategoryAndName(ctx, urlStr, req.savePath, req.qbitCategory, req.meta.Title, req.meta.SelectedFiles); err != nil {
			return "", err
		}
		return torrentURLInfoHash(urlStr), nil
	}
	if categorized, ok := target.adapter.(CategorizedTorrentDownloadAdapter); ok {
		return categorized.AddTorrentWithCategory(ctx, urlStr, req.savePath, req.qbitCategory)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(urlStr)), "magnet:") {
		return target.adapter.AddMagnet(ctx, urlStr, req.savePath)
	}
	return target.adapter.AddTorrent(ctx, urlStr, req.savePath)
}

func setFetchedTorrentTitle(req *downloadAddRequest, name string, addErr error) {
	if addErr == nil && req != nil && strings.TrimSpace(req.meta.Title) == "" {
		req.meta.Title = strings.TrimSuffix(name, path.Ext(name))
	}
}

func joinTorrentFetchError(addErr, fetchErr error) error {
	if fetchErr != nil && !strings.Contains(fetchErr.Error(), "no matching PT site") {
		return errors.Join(addErr, fetchErr)
	}
	return addErr
}

// preparedQBitClient 返回"先暂停加入 qB、选择文件后再开始"流程使用的
// qBittorrent 客户端及对应下载目标。旧版 qbittorrent.* 配置直接用 d.qb;
// 默认下载器为托管 qBittorrent 时 d.qb 会被清空,按其凭据构建(并缓存)
// 独立客户端;其他下载器没有文件级暂停/选择能力,返回明确错误。
func (d *DownloadService) preparedQBitClient(ctx context.Context) (*QBitClient, downloadTarget, error) {
	target, err := d.defaultDownloadTarget(ctx)
	if err != nil {
		return nil, downloadTarget{}, d.defaultDownloaderNotConfiguredError(ctx)
	}
	if target.legacyQB {
		return d.qb, target, nil
	}
	if target.typ != "qbittorrent" || d.manager == nil {
		return nil, downloadTarget{}, fmt.Errorf("下载前选择文件仅支持 qBittorrent,当前默认下载器为 %s", target.typ)
	}
	clientCfg, ok := d.manager.clientConfig(target.clientID)
	if !ok {
		return nil, downloadTarget{}, errors.New("download client not found or not initialized")
	}
	endpoint, err := normalizeDownloadClientEndpoint("qbittorrent", clientCfg.Host)
	if err != nil {
		return nil, downloadTarget{}, err
	}
	cfg := QBitConfig{BaseURL: endpoint, Username: clientCfg.Username, Password: clientCfg.Password}
	key := target.clientID + "\x00" + cfg.BaseURL + "\x00" + cfg.Username + "\x00" + cfg.Password
	d.preparedMu.Lock()
	defer d.preparedMu.Unlock()
	if d.preparedQB == nil || d.preparedQBKey != key {
		d.preparedQB = NewQBitClient(d.log, cfg)
		d.preparedQBKey = key
	}
	return d.preparedQB, target, nil
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
	_ = d.ReloadConfig(ctx)
	qb, _, err := d.preparedQBitClient(ctx)
	if err != nil {
		return nil, err
	}
	live, liveErr := qb.List(ctx, "")
	if existing, ok := d.findExistingDownloadTask(ctx, req); ok {
		if liveErr != nil || torrentExistsInListByIdentity(live, req.title) {
			_ = existing
			return nil, ErrDownloadAlreadyExists
		}
	}
	if liveErr == nil {
		if torrent, ok := findLikelyPreparedTorrentByIdentity(live, req.title); ok {
			return d.reusePreparedTorrent(ctx, qb, torrent.Hash, urlStr, req.savePath, req.meta)
		}
		if torrentExistsInListByIdentity(live, req.title) {
			return nil, ErrDownloadAlreadyExists
		}
	}
	var siteFetchErr error
	if d.site != nil {
		if data, name, err := d.site.FetchTorrentFile(ctx, urlStr); err == nil {
			hash, files, err := qb.PrepareTorrentFileWithCategoryAndName(ctx, data, name, req.savePath, req.qbitCategory, req.meta.Title)
			if err != nil {
				if errors.Is(err, ErrDownloadAlreadyExists) {
					if prepared, ok := d.reusePreparedTorrentByHash(ctx, qb, hash, urlStr, req.savePath, req.meta); ok {
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
	hash, files, err := qb.PrepareTorrentWithCategoryAndName(ctx, urlStr, req.savePath, req.qbitCategory, req.meta.Title)
	if err != nil {
		if errors.Is(err, ErrDownloadAlreadyExists) {
			if prepared, ok := d.reusePreparedTorrentByHash(ctx, qb, hash, urlStr, req.savePath, req.meta); ok {
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

func (d *DownloadService) reusePreparedTorrent(ctx context.Context, qb *QBitClient, hash, urlStr, savePath string, meta DownloadTaskMeta) (*PreparedDownload, error) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return nil, ErrDownloadAlreadyExists
	}
	files, err := qb.Files(ctx, hash)
	if err != nil {
		return nil, err
	}
	d.rememberPreparedDownload(hash, urlStr, savePath, meta)
	return &PreparedDownload{Hash: hash, Files: files}, nil
}

func (d *DownloadService) reusePreparedTorrentByHash(ctx context.Context, qb *QBitClient, hash, urlStr, savePath string, meta DownloadTaskMeta) (*PreparedDownload, bool) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return nil, false
	}
	live, err := qb.List(ctx, "")
	if err != nil {
		return nil, false
	}
	for _, torrent := range live {
		if strings.EqualFold(torrent.Hash, hash) && isLikelyPreparedTorrent(torrent) {
			prepared, err := d.reusePreparedTorrent(ctx, qb, hash, urlStr, savePath, meta)
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
	qb, target, err := d.preparedQBitClient(ctx)
	if err != nil {
		return nil, err
	}
	if len(selectedFileIndexes) > 0 {
		if err := qb.ApplySelectedFileIndexes(ctx, hash, selectedFileIndexes); err != nil {
			return nil, err
		}
	} else if err := qb.Resume(ctx, hash); err != nil {
		return nil, err
	}
	return d.createTask(ctx, userID, urlStr, req.savePath, req.meta, target, strings.ToLower(hash))
}

func (d *DownloadService) CancelPreparedDownload(ctx context.Context, hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return errors.New("hash is required")
	}
	_ = d.ReloadConfig(ctx)
	qb, _, err := d.preparedQBitClient(ctx)
	if err != nil {
		return err
	}
	d.forgetPreparedDownload(hash)
	return qb.Delete(ctx, hash, false)
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

func (d *DownloadService) createTask(ctx context.Context, userID, urlStr, savePath string, meta DownloadTaskMeta, target downloadTarget, externalID string) (*model.DownloadTask, error) {
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = publicDownloadTitle(urlStr)
	}
	t := &model.DownloadTask{
		UserID:               userID,
		SubscriptionID:       strings.TrimSpace(meta.SubscriptionID),
		DownloadClientID:     target.clientID,
		ExternalID:           strings.TrimSpace(externalID),
		Source:               target.typ,
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
