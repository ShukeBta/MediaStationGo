package service

import (
	"context"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (d *DownloadService) findExistingDownloadTask(ctx context.Context, req downloadAddRequest) (*model.DownloadTask, bool) {
	key := downloadTaskIdentityKey(req.title)
	if key == "" || d == nil || d.repo == nil || d.repo.Download == nil {
		return nil, false
	}
	rows, err := d.repo.Download.List(ctx)
	if err != nil {
		return nil, false
	}
	subscriptionID := strings.TrimSpace(req.meta.SubscriptionID)
	for i := range rows {
		if subscriptionID != "" {
			if !downloadTaskBlocksReadd(rows[i].Status) {
				continue
			}
			if !downloadTaskInSubscriptionScope(rows[i], req) {
				continue
			}
			if !d.subscriptionDownloadTaskStillLive(ctx, rows[i]) {
				continue
			}
		} else if !downloadTaskBlocksDuplicate(rows[i].Status) {
			continue
		}
		current := downloadTaskIdentityKey(rows[i].Title)
		if downloadTaskCoversAddRequest(rows[i].Title, req) || current == key {
			return &rows[i], true
		}
	}
	return nil, false
}

func (d *DownloadService) subscriptionDownloadTaskStillLive(ctx context.Context, row model.DownloadTask) bool {
	live, ok := d.liveTorrentSnapshot(30 * time.Second)
	if !ok && d != nil && d.qb != nil && d.qb.IsConfigured() {
		var err error
		live, err = d.qb.List(ctx, "")
		if err != nil {
			return true
		}
		ok = true
	}
	if !ok {
		return true
	}
	for _, torrent := range live {
		if downloadTaskMatchesLiveTorrent(row, torrent) {
			return true
		}
	}
	return false
}

func downloadTaskMatchesLiveTorrent(row model.DownloadTask, torrent QBitTorrent) bool {
	torrentName := strings.TrimSpace(torrent.Name)
	if torrentName == "" {
		return false
	}
	req := downloadAddRequest{
		title:    row.Title,
		savePath: row.SavePath,
		meta: DownloadTaskMeta{
			SubscriptionID: row.SubscriptionID,
		},
	}
	if downloadTaskCoversAddRequest(torrentName, req) {
		return true
	}
	rowKey := downloadTaskIdentityKey(row.Title)
	torrentKey := downloadTaskIdentityKey(torrentName)
	if rowKey != "" && torrentKey != "" {
		return rowKey == torrentKey
	}
	if len(episodeRefsFromTitle(row.Title)) > 0 || len(episodeRefsFromTitle(torrentName)) > 0 {
		return false
	}
	rowTorrentKey := normalizeTorrentName(row.Title)
	liveTorrentKey := normalizeTorrentName(torrentName)
	return rowTorrentKey != "" && rowTorrentKey == liveTorrentKey
}

func downloadTaskCoversAddRequest(existing string, req downloadAddRequest) bool {
	if subscriptionRequestHasExplicitEpisodes(req) {
		return downloadExplicitEpisodesCoverRequest(existing, req.title)
	}
	return downloadTitleCoversRequest(existing, req.title)
}

func subscriptionRequestHasExplicitEpisodes(req downloadAddRequest) bool {
	return strings.TrimSpace(req.meta.SubscriptionID) != "" && len(episodeRefsFromTitle(req.title)) > 0
}

func downloadExplicitEpisodesCoverRequest(existing, requested string) bool {
	current := parseDownloadMediaIdentity(existing)
	want := parseDownloadMediaIdentity(requested)
	if current.TitleKey == "" || want.TitleKey == "" {
		return false
	}
	if current.TitleKey != want.TitleKey {
		return false
	}
	if current.Year > 0 && want.Year > 0 && current.Year != want.Year {
		return false
	}
	if len(current.Episodes) == 0 || len(want.Episodes) == 0 {
		return false
	}
	currentEpisodes := map[string]struct{}{}
	for _, ref := range current.Episodes {
		currentEpisodes[episodeKey(ref.Season, ref.Episode)] = struct{}{}
	}
	for _, ref := range want.Episodes {
		if _, ok := currentEpisodes[episodeKey(ref.Season, ref.Episode)]; !ok {
			return false
		}
	}
	return true
}

func downloadTaskInSubscriptionScope(row model.DownloadTask, req downloadAddRequest) bool {
	subscriptionID := strings.TrimSpace(req.meta.SubscriptionID)
	if subscriptionID == "" {
		return true
	}
	rowSubscriptionID := strings.TrimSpace(row.SubscriptionID)
	if rowSubscriptionID != "" {
		return rowSubscriptionID == subscriptionID
	}
	rowSavePath := strings.TrimSpace(row.SavePath)
	requestSavePath := strings.TrimSpace(req.savePath)
	if rowSavePath == "" || requestSavePath == "" {
		return false
	}
	return sameOrChildPath(rowSavePath, requestSavePath) || sameOrChildPath(requestSavePath, rowSavePath)
}

func (d *DownloadService) torrentExistsByIdentity(ctx context.Context, req downloadAddRequest) bool {
	query := downloadTaskIdentityKey(req.title)
	if query == "" {
		return false
	}
	live, err := d.qb.List(ctx, "")
	if err != nil {
		return false
	}
	for _, torrent := range live {
		if !torrentInDownloadRequestScope(torrent, req) {
			continue
		}
		if downloadTaskCoversAddRequest(torrent.Name, req) {
			return true
		}
		current := downloadTaskIdentityKey(torrent.Name)
		if current == "" {
			continue
		}
		if current == query {
			return true
		}
	}
	return false
}

func torrentInDownloadRequestScope(torrent QBitTorrent, req downloadAddRequest) bool {
	if strings.TrimSpace(req.meta.SubscriptionID) == "" {
		return true
	}
	requestSavePath := strings.TrimSpace(req.savePath)
	torrentSavePath := strings.TrimSpace(torrent.SavePath)
	if requestSavePath == "" || torrentSavePath == "" {
		return false
	}
	return sameOrChildPath(torrentSavePath, requestSavePath) || sameOrChildPath(requestSavePath, torrentSavePath)
}
