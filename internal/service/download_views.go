package service

import (
	"math"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type DownloadTaskView struct {
	ID               string    `json:"id"`
	Source           string    `json:"source"`
	DownloadClientID string    `json:"download_client_id,omitempty"`
	ExternalID       string    `json:"external_id,omitempty"`
	Title            string    `json:"title"`
	PosterURL        string    `json:"poster_url,omitempty"`
	BackdropURL      string    `json:"backdrop_url,omitempty"`
	Overview         string    `json:"overview,omitempty"`
	SavePath         string    `json:"save_path"`
	MediaType        string    `json:"media_type,omitempty"`
	MediaCategory    string    `json:"media_category,omitempty"`
	Status           string    `json:"status"`
	Progress         float32   `json:"progress"`
	State            string    `json:"state,omitempty"`
	DLSpeed          int64     `json:"dlspeed,omitempty"`
	UpSpeed          int64     `json:"upspeed,omitempty"`
	Size             int64     `json:"size,omitempty"`
	Downloaded       int64     `json:"downloaded,omitempty"`
	NumSeeds         int       `json:"num_seeds,omitempty"`
	NumLeechs        int       `json:"num_leechs,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type DownloadTorrentView struct {
	Hash          string  `json:"hash"`
	ClientID      string  `json:"client_id"`
	Source        string  `json:"source"`
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
}

func DownloadViews(rows []model.DownloadTask, live []QBitTorrent) ([]DownloadTaskView, []DownloadTorrentView) {
	liveByKey := map[string]QBitTorrent{}
	for _, torrent := range live {
		key := normalizeTorrentName(torrent.Name)
		if key != "" {
			setLiveTorrentIndex(liveByKey, key, torrent)
			setLiveTorrentIndex(liveByKey, downloadTaskClientTitleKey(torrent.ClientID, key), torrent)
		}
		setLiveTorrentIndex(liveByKey, downloadTaskExternalKey(torrent.ClientID, torrent.Hash), torrent)
		setLiveTorrentIndex(liveByKey, downloadTaskAnyExternalKey(torrent.Hash), torrent)
	}
	taskByKey := tasksByTorrentIdentity(rows)

	taskViews := make([]DownloadTaskView, 0, len(rows))
	for _, row := range rows {
		view := downloadTaskView(row, QBitTorrent{})
		if torrent, ok := findMatchingTorrentForTask(row, liveByKey); ok {
			view = downloadTaskView(row, torrent)
		}
		taskViews = append(taskViews, view)
	}

	torrentViews := make([]DownloadTorrentView, 0, len(live))
	for _, torrent := range live {
		var row model.DownloadTask
		if matched, ok := findMatchingTaskForTorrent(torrent, taskByKey); ok {
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
		ID:               row.ID,
		Source:           row.Source,
		DownloadClientID: row.DownloadClientID,
		ExternalID:       row.ExternalID,
		Title:            firstNonEmpty(row.Title, "下载任务"),
		PosterURL:        row.PosterURL,
		BackdropURL:      row.BackdropURL,
		Overview:         row.Overview,
		SavePath:         row.SavePath,
		MediaType:        row.MediaType,
		MediaCategory:    row.MediaCategory,
		Status:           row.Status,
		Progress:         progress,
		State:            state,
		DLSpeed:          torrent.DLSpeed,
		UpSpeed:          torrent.UpSpeed,
		Size:             size,
		Downloaded:       downloadedBytes(size, progress),
		NumSeeds:         torrent.NumSeeds,
		NumLeechs:        torrent.NumLeech,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func downloadTorrentView(torrent QBitTorrent, row model.DownloadTask) DownloadTorrentView {
	title := torrent.Name
	if row.Title != "" {
		title = row.Title
	}
	return DownloadTorrentView{
		Hash:          torrent.Hash,
		ClientID:      torrent.ClientID,
		Source:        torrent.Source,
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
	}
}

func setLiveTorrentIndex(index map[string]QBitTorrent, key string, torrent QBitTorrent) {
	if key == "" {
		return
	}
	if _, exists := index[key]; !exists {
		index[key] = torrent
	}
}

func findMatchingTorrentForTask(row model.DownloadTask, liveByKey map[string]QBitTorrent) (QBitTorrent, bool) {
	if torrent, ok := liveByKey[downloadTaskExternalKey(row.DownloadClientID, row.ExternalID)]; ok {
		return torrent, true
	}
	if torrent, ok := liveByKey[downloadTaskAnyExternalKey(row.ExternalID)]; ok {
		if strings.TrimSpace(row.DownloadClientID) == "" || row.DownloadClientID == torrent.ClientID {
			return torrent, true
		}
	}
	key := normalizeTorrentName(row.Title)
	if torrent, ok := liveByKey[downloadTaskClientTitleKey(row.DownloadClientID, key)]; ok {
		return torrent, true
	}
	if strings.TrimSpace(row.DownloadClientID) != "" {
		return QBitTorrent{}, false
	}
	return findMatchingTorrent(row.Title, liveByKey)
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
		if strings.HasPrefix(currentKey, "\x00") {
			continue
		}
		if strings.Contains(currentKey, key) || strings.Contains(key, currentKey) {
			return torrent, true
		}
	}
	return QBitTorrent{}, false
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
