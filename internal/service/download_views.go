package service

import (
	"math"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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
