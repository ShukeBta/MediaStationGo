package model

import "time"

// EmbyVirtualFolder 虚拟文件夹（媒体库）。
type EmbyVirtualFolder struct {
	Name           string             `json:"Name"`
	Locations      []string           `json:"Locations"`
	CollectionType string             `json:"CollectionType"`
	LibraryOptions EmbyLibraryOptions `json:"LibraryOptions,omitempty"`
	RefreshStatus  *EmbyRefreshStatus `json:"RefreshStatus,omitempty"`
	ItemId         string             `json:"ItemId"`
}

// EmbyLibraryOptions 媒体库选项。
type EmbyLibraryOptions struct {
	PreferredMetadataLanguage     string `json:"PreferredMetadataLanguage"`
	MetadataCountryCode           string `json:"MetadataCountryCode"`
	EnableRealtimeMonitor         bool   `json:"EnableRealtimeMonitor"`
	EnableAutomaticSeriesGrouping bool   `json:"EnableAutomaticSeriesGrouping"`
}

// EmbyRefreshStatus 刷新状态。
type EmbyRefreshStatus struct {
	LastRefreshResult string    `json:"LastRefreshResult"`
	LastRefreshedAt   time.Time `json:"LastRefreshedAt"`
	IsActive          bool      `json:"IsActive"`
}

// EmbyItemsCounts 项目计数。
type EmbyItemsCounts struct {
	MovieCount      int `json:"MovieCount"`
	SeriesCount     int `json:"SeriesCount"`
	EpisodeCount    int `json:"EpisodeCount"`
	ArtistCount     int `json:"ArtistCount"`
	AlbumCount      int `json:"AlbumCount"`
	SongCount       int `json:"SongCount"`
	MusicVideoCount int `json:"MusicVideoCount"`
	BookCount       int `json:"BookCount"`
	BoxSetCount     int `json:"BoxSetCount"`
}

// EmbyHubResponse Hub 响应。
type EmbyHubResponse struct {
	Items []EmbyHubItem `json:"Items"`
}

// EmbyHubItem Hub 条目。
type EmbyHubItem struct {
	Id         string     `json:"Id"`
	Name       string     `json:"Name"`
	Type       string     `json:"Type"`
	Items      []EmbyItem `json:"Items"`
	TotalCount int        `json:"TotalCount,omitempty"`
	ImageUrl   string     `json:"ImageUrl,omitempty"`
}
