package service

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// DownloadTaskMeta carries public display metadata for a download. It is
// deliberately separate from the private torrent URL so API responses never
// need to expose tracker tokens.
type DownloadTaskMeta struct {
	SubscriptionID       string
	Title                string
	PosterURL            string
	BackdropURL          string
	Overview             string
	MediaType            string
	MediaCategory        string
	SourceCategory       string
	OriginalName         string
	OriginalLanguage     string
	Year                 int
	Rating               float32
	Genres               string
	AllowExistingLibrary bool
}

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
	autoClassify := downloadSmartClassifyEnabled(ctx, d.repo, d.organizer)
	savePath, resolvedCategory := d.resolveDownloadSavePath(ctx, savePath, meta, autoClassify)
	if !autoClassify {
		meta.MediaCategory = ""
	} else if strings.TrimSpace(meta.MediaCategory) == "" {
		meta.MediaCategory = resolvedCategory
	}
	return downloadAddRequest{
		title:        title,
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
		if err := d.qb.AddTorrentFileWithCategory(ctx, data, name, req.savePath, req.qbitCategory); err != nil {
			return "", err
		}
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
		if err := d.qb.AddTorrentWithCategory(ctx, urlStr, req.savePath, req.qbitCategory); err != nil {
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
		PosterURL:            meta.PosterURL,
		BackdropURL:          meta.BackdropURL,
		Overview:             meta.Overview,
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
