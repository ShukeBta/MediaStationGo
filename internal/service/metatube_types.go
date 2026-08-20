package service

import "time"

// MetaTubeSearchResult 表示 MetaTube 搜索结果项。
type MetaTubeSearchResult struct {
	ID          string   `json:"id"`
	Number      string   `json:"number"`
	Title       string   `json:"title"`
	Provider    string   `json:"provider"`
	Homepage    string   `json:"homepage,omitempty"`
	Actors      []string `json:"actors,omitempty"`
	CoverURL    string   `json:"cover_url,omitempty"`
	BigCoverURL string   `json:"big_cover_url,omitempty"`
	ThumbURL    string   `json:"thumb_url,omitempty"`
	BigThumbURL string   `json:"big_thumb_url,omitempty"`
	Score       float32  `json:"score,omitempty"`
	ReleaseDate string   `json:"release_date,omitempty"`
}

// MetaTubeMovieInfo 表示 MetaTube 影片完整详情。
type MetaTubeMovieInfo struct {
	ID                 string    `json:"id"`
	Number             string    `json:"number"`
	Title              string    `json:"title"`
	Summary            string    `json:"summary,omitempty"`
	Provider           string    `json:"provider"`
	Homepage           string    `json:"homepage,omitempty"`
	Director           string    `json:"director,omitempty"`
	Directors          []string  `json:"directors,omitempty"`
	Actors             []string  `json:"actors,omitempty"`
	Maker              string    `json:"maker,omitempty"`
	Label              string    `json:"label,omitempty"`
	Series             string    `json:"series,omitempty"`
	Genres             []string  `json:"genres,omitempty"`
	Score              float32   `json:"score,omitempty"`
	Runtime            int       `json:"runtime,omitempty"`
	ReleaseDate        string    `json:"release_date,omitempty"`
	ThumbURL           string    `json:"thumb_url,omitempty"`
	BigThumbURL        string    `json:"big_thumb_url,omitempty"`
	CoverURL           string    `json:"cover_url,omitempty"`
	BigCoverURL        string    `json:"big_cover_url,omitempty"`
	PreviewVideoURL    string    `json:"preview_video_url,omitempty"`
	PreviewVideoHLSURL string    `json:"preview_video_hls_url,omitempty"`
	PreviewImages      []string  `json:"preview_images,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
}

// MetaTubeActorInfo 表示 MetaTube 演员详情。
type MetaTubeActorInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	JapaneseName string   `json:"japanese_name,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	Images       []string `json:"images,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Hobby        string   `json:"hobby,omitempty"`
	Skill        string   `json:"skill,omitempty"`
	Birthday     string   `json:"birthday,omitempty"`
	BloodType    string   `json:"blood_type,omitempty"`
	CupSize      string   `json:"cup_size,omitempty"`
	Measurements string   `json:"measurements,omitempty"`
	Height       int      `json:"height,omitempty"`
}

// MetaTubeProvidersResponse 表示 /v1/providers 响应。
type MetaTubeProvidersResponse struct {
	Data struct {
		MovieProviders map[string]string `json:"movie_providers"`
		ActorProviders map[string]string `json:"actor_providers"`
	} `json:"data"`
}

// MetaTubeConfig 表示 MetaTube 客户端运行配置。
type MetaTubeConfig struct {
	ServerURL       string `json:"server_url"`
	Token           string `json:"token"`
	DefaultProvider string `json:"default_provider,omitempty"`
	EnableActor     bool   `json:"enable_actor"`
	EnableTrailer   bool   `json:"enable_trailer"`
	CropCover       bool   `json:"crop_cover"`
	Badge           bool   `json:"badge"`
}

// MetaTubeTestResult 表示 MetaTube 连通性测试结果。
type MetaTubeTestResult struct {
	Success   bool     `json:"success"`
	LatencyMs int64    `json:"latency_ms"`
	Providers []string `json:"providers,omitempty"`
	Error     string   `json:"error,omitempty"`
}
