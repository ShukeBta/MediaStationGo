package config

// Config 是根配置聚合。
type Config struct {
	App          AppConfig          `mapstructure:"app"`
	Database     DatabaseConfig     `mapstructure:"database"`
	Secrets      SecretsConfig      `mapstructure:"secrets"`
	Logging      LoggingConfig      `mapstructure:"logging"`
	Cache        CacheConfig        `mapstructure:"cache"`
	Search       SearchConfig       `mapstructure:"search"`
	Media        MediaConfig        `mapstructure:"media"`
	Transcoder   TranscoderConfig   `mapstructure:"transcoder"`
	AI           AIConfig           `mapstructure:"ai"`
	FlareSolverr FlareSolverrConfig `mapstructure:"flaresolverr"`
	ApiConfig    ApiConfigConfig    `mapstructure:"api_config"`
	Organizer    OrganizerConfig    `mapstructure:"organizer"`
	License      LicenseConfig      `mapstructure:"license"`
}

// ApiConfigConfig API 配置相关设置。
type ApiConfigConfig struct {
	// AutoEncrypt 是否自动加密敏感字段
	AutoEncrypt bool `mapstructure:"auto_encrypt"`
	// DefaultTimeout 默认请求超时（秒）
	DefaultTimeout int `mapstructure:"default_timeout"`
}

// TranscoderConfig 控制 HLS / ffmpeg 后端。
type TranscoderConfig struct {
	Encoder            string `mapstructure:"encoder"` // "" / nvenc / qsv / vaapi
	Enabled            bool   `mapstructure:"enabled"`
	HardwareAccel      bool   `mapstructure:"hardware_accel"`
	Preset             string `mapstructure:"preset"`
	VideoBitrate       string `mapstructure:"video_bitrate"`
	MaxRate            string `mapstructure:"max_rate"`
	BufSize            string `mapstructure:"buf_size"`
	MaxHeight          int    `mapstructure:"max_height"`
	SegmentSeconds     int    `mapstructure:"segment_seconds"`
	Realtime           bool   `mapstructure:"realtime"`
	Threads            int    `mapstructure:"threads"`
	MaxConcurrent      int    `mapstructure:"max_concurrent"`
	IdleTimeoutSeconds int    `mapstructure:"idle_timeout_seconds"`
}

// AppConfig 保存运行时应用参数。
type AppConfig struct {
	Port        int    `mapstructure:"port"`
	Debug       bool   `mapstructure:"debug"`
	Env         string `mapstructure:"env"`
	DataDir     string `mapstructure:"data_dir"`
	WebDir      string `mapstructure:"web_dir"`
	FFmpegPath  string `mapstructure:"ffmpeg_path"`
	FFprobePath string `mapstructure:"ffprobe_path"`
	// FFprobeMaxConcurrent limits concurrent ffprobe/ffmpeg metadata probes.
	// NAS devices can become unresponsive when a scan starts many probe
	// processes at once, so the default is deliberately conservative.
	FFprobeMaxConcurrent int `mapstructure:"ffprobe_max_concurrent"`
	// CloudScanMaxConcurrent limits concurrent cloud directory list requests
	// inside one mounted cloud library scan.
	CloudScanMaxConcurrent int      `mapstructure:"cloud_scan_max_concurrent"`
	MaxCPUThreads          int      `mapstructure:"max_cpu_threads"`
	VAAPIDevice            string   `mapstructure:"vaapi_device"`
	CORSOrigins            []string `mapstructure:"cors_origins"`
	ServerURL              string   `mapstructure:"server_url"`
}

// DatabaseConfig 配置 GORM 数据库。默认 auto：
// Docker Compose 主线会注入 PostgreSQL DSN；裸机/旧部署没有 DSN 时回退 SQLite。
type DatabaseConfig struct {
	Type         string `mapstructure:"type"`
	DBPath       string `mapstructure:"db_path"`
	DSN          string `mapstructure:"dsn"`
	WALMode      bool   `mapstructure:"wal_mode"`
	BusyTimeout  int    `mapstructure:"busy_timeout"`
	CacheSize    int    `mapstructure:"cache_size"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

// SecretsConfig 保存 JWT / 第三方 API 密钥（不要提交值）。
type SecretsConfig struct {
	JWTSecret      string `mapstructure:"jwt_secret"`
	TMDbAPIKey     string `mapstructure:"tmdb_api_key"`
	TMDbAPIProxy   string `mapstructure:"tmdb_api_proxy"`
	TMDbImageProxy string `mapstructure:"tmdb_image_proxy"`
	BangumiToken   string `mapstructure:"bangumi_access_token"`
	TheTVDBAPIKey  string `mapstructure:"thetvdb_api_key"`
	FanartAPIKey   string `mapstructure:"fanart_tv_api_key"`
	DoubanCookie   string `mapstructure:"douban_cookie"`
	// 用于加密的密钥，如果为空则使用 JWTSecret
	EncryptionKey string `mapstructure:"encryption_key"`
}

// LoggingConfig 配置 Zap。
type LoggingConfig struct {
	Level          string `mapstructure:"level"`
	Format         string `mapstructure:"format"`
	OutputPath     string `mapstructure:"output_path"`
	EnableRotation bool   `mapstructure:"enable_rotation"`
	MaxSizeMB      int    `mapstructure:"max_size_mb"`
	MaxAgeDays     int    `mapstructure:"max_age_days"`
	MaxBackups     int    `mapstructure:"max_backups"`
}

// CacheConfig 控制磁盘转码/刮削缓存。
type CacheConfig struct {
	CacheDir           string `mapstructure:"cache_dir"`
	MaxDiskUsageMB     int    `mapstructure:"max_disk_usage_mb"`
	TTLHours           int    `mapstructure:"ttl_hours"`
	AutoCleanup        bool   `mapstructure:"auto_cleanup"`
	CleanupIntervalMin int    `mapstructure:"cleanup_interval_min"`
	RedisURL           string `mapstructure:"redis_url"`
	RedisPrefix        string `mapstructure:"redis_prefix"`
	MediaTTLSeconds    int    `mapstructure:"media_ttl_seconds"`
}

type SearchConfig struct {
	Backend       string `mapstructure:"backend"`
	OpenSearchURL string `mapstructure:"opensearch_url"`
	Index         string `mapstructure:"index"`
	Username      string `mapstructure:"username"`
	Password      string `mapstructure:"password"`
}

// MediaConfig 保存默认库位置（用于引导库）。
type MediaConfig struct {
	MoviesDir string `mapstructure:"movies_dir"`
	TVDir     string `mapstructure:"tv_dir"`
	AnimeDir  string `mapstructure:"anime_dir"`
}

// AIConfig 配置可选的 LLM 提供者。
type AIConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	Provider      string `mapstructure:"provider"`
	APIBase       string `mapstructure:"api_base"`
	APIKey        string `mapstructure:"api_key"`
	Model         string `mapstructure:"model"`
	Timeout       int    `mapstructure:"timeout"`
	MaxConcurrent int    `mapstructure:"max_concurrent"`
}

// LicenseConfig configures the optional MediaStationGo license server bridge.
type LicenseConfig struct {
	ServerURL  string `mapstructure:"server_url"`
	HMACSecret string `mapstructure:"hmac_secret"`
	PublicKey  string `mapstructure:"public_key"`
}

// OrganizerConfig 配置媒体文件智能分类整理。
type OrganizerConfig struct {
	SmartClassify     bool              `mapstructure:"smart_classify"`
	AutoAfterDownload bool              `mapstructure:"auto_after_download"`
	Categories        map[string]string `mapstructure:"categories"`
}

// FlareSolverrConfig 配置 FlareSolverr 服务（用于绕过 Cloudflare/WAF）。
type FlareSolverrConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	URL     string `mapstructure:"url"`
	Session string `mapstructure:"session"`
	Timeout int    `mapstructure:"timeout"`
}
