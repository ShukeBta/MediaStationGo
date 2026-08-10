// Package service 定义下载适配器接口和通用数据结构。
package service

import (
	"context"
	"time"
)

// DownloadAdapter 定义下载客户端的统一接口。
// 所有下载客户端（qBittorrent / Transmission / Aria2）必须实现此接口。
type DownloadAdapter interface {
	// Initialize 使用配置初始化客户端连接。
	Initialize(ctx context.Context, cfg DownloadClientConfig) error
	// Ping 测试客户端连接是否可用。
	Ping(ctx context.Context) error
	// AddTorrent 通过 URL（磁力链接或种子 URL）添加下载任务。
	AddTorrent(ctx context.Context, url, savePath string) (string, error)
	// AddMagnet 通过磁力链接添加下载任务。
	AddMagnet(ctx context.Context, magnet, savePath string) (string, error)
	// Pause 暂停指定下载任务。
	Pause(ctx context.Context, hash string) error
	// Resume 恢复指定下载任务。
	Resume(ctx context.Context, hash string) error
	// Remove 移除指定下载任务，deleteFiles 控制是否同时删除文件。
	Remove(ctx context.Context, hash string, deleteFiles bool) error
	// List 列出所有或过滤后的种子任务。filter 可为空字符串表示全部。
	List(ctx context.Context, filter string) ([]TorrentInfo, error)
	// GetInfo 获取指定种子的详细信息。
	GetInfo(ctx context.Context, hash string) (*TorrentInfo, error)
}

// TorrentFileDownloadAdapter is implemented by clients that can accept the
// application-fetched .torrent payload instead of fetching a private URL.
type TorrentFileDownloadAdapter interface {
	AddTorrentFile(ctx context.Context, data []byte, name, savePath string) (string, error)
}

// CategorizedTorrentDownloadAdapter is implemented by qBittorrent, whose
// native category is part of MediaStationGo's automatic classification flow.
type CategorizedTorrentDownloadAdapter interface {
	AddTorrentWithCategory(ctx context.Context, url, savePath, category string) (string, error)
	AddTorrentFileWithCategory(ctx context.Context, data []byte, name, savePath, category string) (string, error)
}

// TorrentRelocateAdapter is intentionally qBittorrent-only: qB can move
// payload data while preserving its seeding task through setLocation.
type TorrentRelocateAdapter interface {
	Relocate(ctx context.Context, hash, location string) error
}

// TorrentInfo 是各种下载客户端的种子信息的统一表示。
type TorrentInfo struct {
	Hash         string    `json:"hash"`
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	Progress     float64   `json:"progress"`
	DLSpeed      int64     `json:"dl_speed"`
	UPSpeed      int64     `json:"up_speed"`
	State        string    `json:"state"`
	SavePath     string    `json:"save_path"`
	NumSeeds     int       `json:"num_seeds"`
	NumLeechs    int       `json:"num_leechs"`
	AddedOn      time.Time `json:"added_on"`
	Category     string    `json:"category"`
	Tags         string    `json:"tags"`
	ContentPath  string    `json:"content_path"`
	CompletionOn int64     `json:"completion_on"`
}

// DownloadClientConfig 是下载客户端的连接配置。
type DownloadClientConfig struct {
	Host     string            `json:"host"`
	Username string            `json:"username"`
	Password string            `json:"password"`
	Extra    map[string]string `json:"extra,omitempty"`
}

// AdapterFactory 根据客户端类型创建适配器实例。
func AdapterFactory(clientType string) DownloadAdapter {
	switch clientType {
	case "qbittorrent":
		return NewQBitAdapter()
	case "transmission":
		return NewTransmissionAdapter()
	case "aria2":
		return NewAria2Adapter()
	default:
		return nil
	}
}
