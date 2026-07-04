package model

import "time"

// EmbyItemsResponse Emby 标准分页响应包装。
type EmbyItemsResponse struct {
	Items            []EmbyItem `json:"Items"`
	TotalRecordCount int        `json:"TotalRecordCount"`
	StartIndex       int        `json:"StartIndex"`
}

// EmbyItem Emby 媒体项。
type EmbyItem struct {
	Id                 string            `json:"Id"`
	Name               string            `json:"Name"`
	Type               string            `json:"Type"` // Movie / Series / Episode / Season / BoxSet / Folder
	Overview           string            `json:"Overview,omitempty"`
	ProductionYear     int               `json:"ProductionYear,omitempty"`
	PremiereDate       *time.Time        `json:"PremiereDate,omitempty"`
	CommunityRating    float64           `json:"CommunityRating,omitempty"`
	OfficialRating     string            `json:"OfficialRating,omitempty"`
	RunTimeTicks       int64             `json:"RunTimeTicks,omitempty"`
	ParentId           string            `json:"ParentId,omitempty"`
	SeriesId           string            `json:"SeriesId,omitempty"`
	SeasonId           string            `json:"SeasonId,omitempty"`
	IndexNumber        int               `json:"IndexNumber,omitempty"`
	ParentIndexNumber  int               `json:"ParentIndexNumber,omitempty"`
	UserData           *EmbyUserData     `json:"UserData,omitempty"`
	ImageTags          map[string]string `json:"ImageTags,omitempty"`
	BackdropImageTags  []string          `json:"BackdropImageTags,omitempty"`
	Genres             []string          `json:"Genres,omitempty"`
	Studios            []EmbyNameId      `json:"Studios,omitempty"`
	People             []EmbyPerson      `json:"People,omitempty"`
	MediaSources       []EmbyMediaSource `json:"MediaSources,omitempty"`
	RecursiveItemCount int               `json:"RecursiveItemCount,omitempty"`
	ChildCount         int               `json:"ChildCount,omitempty"`
	Status             string            `json:"Status,omitempty"`
	AirDays            []string          `json:"AirDays,omitempty"`
	EndDate            *time.Time        `json:"EndDate,omitempty"`
	ProviderIds        map[string]string `json:"ProviderIds,omitempty"`
	Taglines           []string          `json:"Taglines,omitempty"`
	GenreItems         []EmbyNameId      `json:"GenreItems,omitempty"`
	DateCreated        *time.Time        `json:"DateCreated,omitempty"`
	Path               string            `json:"Path,omitempty"`
	SortName           string            `json:"SortName,omitempty"`
	ForcedSortName     string            `json:"ForcedSortName,omitempty"`
	Width              int               `json:"Width,omitempty"`
	Height             int               `json:"Height,omitempty"`
	Container          string            `json:"Container,omitempty"`
}

// EmbyUserData 用户播放数据。
type EmbyUserData struct {
	PlaybackPositionTicks int64   `json:"PlaybackPositionTicks"`
	PlayCount             int     `json:"PlayCount"`
	IsFavorite            bool    `json:"IsFavorite"`
	Played                bool    `json:"Played"`
	UnplayedItemCount     int     `json:"UnplayedItemCount"`
	PercentagePlayed      float64 `json:"PercentagePlayed"`
	Rating                float64 `json:"Rating,omitempty"`
	PlayedPercentage      float64 `json:"PlayedPercentage,omitempty"`
}

// EmbyMediaSource 媒体源。
type EmbyMediaSource struct {
	Id                   string            `json:"Id"`
	Name                 string            `json:"Name"`
	Path                 string            `json:"Path"`
	Size                 int64             `json:"Size"`
	Container            string            `json:"Container,omitempty"`
	Bitrate              int64             `json:"Bitrate,omitempty"`
	MediaStreams         []EmbyMediaStream `json:"MediaStreams"`
	SupportsTranscoding  bool              `json:"SupportsTranscoding"`
	SupportsDirectStream bool              `json:"SupportsDirectStream"`
	SupportsDirectPlay   bool              `json:"SupportsDirectPlay"`
	TranscodingUrl       string            `json:"TranscodingUrl,omitempty"`
	Protocol             string            `json:"Protocol,omitempty"`
	Type                 string            `json:"Type,omitempty"`
	IsRemote             bool              `json:"IsRemote,omitempty"`
	RunTimeTicks         int64             `json:"RunTimeTicks,omitempty"`
	ETag                 string            `json:"ETag,omitempty"`
	SupportsProbing      bool              `json:"SupportsProbing,omitempty"`
}

// EmbyMediaStream 媒体流（视频/音频/字幕）。
type EmbyMediaStream struct {
	Codec              string `json:"Codec"`
	Type               string `json:"Type"` // Video / Audio / Subtitle
	Language           string `json:"Language,omitempty"`
	DisplayTitle       string `json:"DisplayTitle,omitempty"`
	Index              int    `json:"Index"`
	IsDefault          bool   `json:"IsDefault"`
	IsForced           bool   `json:"IsForced"`
	IsExternal         bool   `json:"IsExternal,omitempty"`
	Height             int    `json:"Height,omitempty"`
	Width              int    `json:"Width,omitempty"`
	BitRate            int64  `json:"BitRate,omitempty"`
	Channels           int    `json:"Channels,omitempty"`
	SampleRate         int    `json:"SampleRate,omitempty"`
	AspectRatio        string `json:"AspectRatio,omitempty"`
	VideoRange         string `json:"VideoRange,omitempty"`
	DeliveryUrl        string `json:"DeliveryUrl,omitempty"`
	DeliveryMethod     string `json:"DeliveryMethod,omitempty"`
	ExternalUrl        string `json:"ExternalUrl,omitempty"`
	ExternalSubtitleId string `json:"ExternalSubtitleId,omitempty"`
	SubtitleFileName   string `json:"SubtitleFileName,omitempty"`
	Title              string `json:"Title,omitempty"`
	Comment            string `json:"Comment,omitempty"`
	Path               string `json:"Path,omitempty"`
}

// EmbyPerson 人员信息。
type EmbyPerson struct {
	Id              string `json:"Id"`
	Name            string `json:"Name"`
	Role            string `json:"Role,omitempty"`
	Type            string `json:"Type,omitempty"`
	PrimaryImageTag string `json:"PrimaryImageTag,omitempty"`
}

// EmbyNameId 名称+ID 对。
type EmbyNameId struct {
	Name string `json:"Name"`
	Id   string `json:"Id"`
}

// EmbyGenre 类型。
type EmbyGenre struct {
	Name string `json:"Name"`
	Id   string `json:"Id,omitempty"`
}

// EmbyPersonInfo 人物信息。
type EmbyPersonInfo struct {
	Id              string     `json:"Id"`
	Name            string     `json:"Name"`
	Type            string     `json:"Type,omitempty"`
	PrimaryImageTag string     `json:"PrimaryImageTag,omitempty"`
	Overview        string     `json:"Overview,omitempty"`
	BirthDate       string     `json:"BirthDate,omitempty"`
	ProductionYear  int        `json:"ProductionYear,omitempty"`
	EndDate         string     `json:"EndDate,omitempty"`
	PremiereDate    *time.Time `json:"PremiereDate,omitempty"`
}
