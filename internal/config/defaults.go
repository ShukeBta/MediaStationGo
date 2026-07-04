package config

import "github.com/spf13/viper"

const (
	defaultDatabaseMaxOpenConns = 4
	defaultDatabaseMaxIdleConns = 2
	defaultLicenseServerURL     = "https://mgosever.3jzs.com"
	defaultLicensePublicKey     = "MCowBQYDK2VwAyEABRXnXy+urjrbKit6Yu/HiezWgP0NdsZW3tsegJWRrtI="
)

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.port", 8080)
	v.SetDefault("app.debug", false)
	v.SetDefault("app.env", "production")
	v.SetDefault("app.data_dir", "./data")
	v.SetDefault("app.web_dir", "./web/dist")
	v.SetDefault("app.ffmpeg_path", "ffmpeg")
	v.SetDefault("app.ffprobe_path", "ffprobe")
	v.SetDefault("app.ffprobe_max_concurrent", 2)
	v.SetDefault("app.cloud_scan_max_concurrent", 8)
	v.SetDefault("app.max_cpu_threads", 2)
	v.SetDefault("app.vaapi_device", "/dev/dri/renderD128")
	v.SetDefault("app.cors_origins", []string{})
	v.SetDefault("app.server_url", "")

	v.SetDefault("database.type", "auto")
	v.SetDefault("database.db_path", "./data/mediastation.db")
	v.SetDefault("database.dsn", "")
	v.SetDefault("database.wal_mode", true)
	v.SetDefault("database.busy_timeout", 5000)
	v.SetDefault("database.cache_size", -20000)
	v.SetDefault("database.max_open_conns", defaultDatabaseMaxOpenConns)
	v.SetDefault("database.max_idle_conns", defaultDatabaseMaxIdleConns)

	v.SetDefault("secrets.jwt_secret", "")

	v.SetDefault("logging.level", "warn")
	v.SetDefault("logging.format", "console")
	v.SetDefault("logging.enable_rotation", true)
	v.SetDefault("logging.max_size_mb", 20)
	v.SetDefault("logging.max_age_days", 30)
	v.SetDefault("logging.max_backups", 10)

	v.SetDefault("cache.cache_dir", "./cache")
	v.SetDefault("cache.cleanup_interval_min", 60)
	v.SetDefault("cache.redis_url", "")
	v.SetDefault("cache.redis_prefix", "mediastationgo")
	v.SetDefault("cache.media_ttl_seconds", 15)

	v.SetDefault("search.backend", "")
	v.SetDefault("search.opensearch_url", "")
	v.SetDefault("search.index", "mediastation_media")
	v.SetDefault("search.username", "")
	v.SetDefault("search.password", "")

	v.SetDefault("ai.enabled", false)
	v.SetDefault("ai.provider", "openai")
	v.SetDefault("ai.api_base", "https://api.openai.com/v1")
	v.SetDefault("ai.model", "gpt-4o-mini")
	v.SetDefault("ai.timeout", 30)
	v.SetDefault("ai.max_concurrent", 3)

	v.SetDefault("flaresolverr.enabled", false)
	v.SetDefault("flaresolverr.url", "http://localhost:8191")
	v.SetDefault("flaresolverr.session", "mediastation")
	v.SetDefault("flaresolverr.timeout", 60)

	v.SetDefault("downloads.smart_classify", true)
	v.SetDefault("organizer.smart_classify", true)
	v.SetDefault("organizer.auto_after_download", false)
	v.SetDefault("organize.scrape_after", true)
	v.SetDefault("scrape.delay_min_ms", 250)
	v.SetDefault("scrape.delay_max_ms", 500)
	v.SetDefault("organizer.categories.concert_movie", "演唱会")
	v.SetDefault("organizer.categories.documentary_movie", "纪录片")
	v.SetDefault("organizer.categories.chinese_movie", "华语电影")
	v.SetDefault("organizer.categories.animation_movie", "动画电影")
	v.SetDefault("organizer.categories.euus_movie", "欧美电影")
	v.SetDefault("organizer.categories.jk_movie", "日韩电影")
	v.SetDefault("organizer.categories.domestic_tv", "国产剧")
	v.SetDefault("organizer.categories.euus_tv", "欧美剧")
	v.SetDefault("organizer.categories.jk_tv", "日韩剧")
	v.SetDefault("organizer.categories.jp_anime", "日番")
	v.SetDefault("organizer.categories.cn_anime", "国漫")
	v.SetDefault("organizer.categories.kr_anime", "韩漫")
	v.SetDefault("organizer.categories.us_anime", "美漫")
	v.SetDefault("organizer.categories.other_anime", "其他")
	v.SetDefault("organizer.categories.variety", "综艺")
	v.SetDefault("organizer.categories.documentary", "纪录片")
	v.SetDefault("organizer.categories.children", "儿童")
	v.SetDefault("organizer.categories.adult", "成人")
	v.SetDefault("recognition_words.enabled", true)
	v.SetDefault("recognition_words.shared_urls", []string{
		"https://raw.githubusercontent.com/Putarku/MoviePilot-Help/main/Words/general.txt",
		"https://raw.githubusercontent.com/Putarku/MoviePilot-Help/main/Words/TV.txt",
		"https://raw.githubusercontent.com/Putarku/MoviePilot-Help/main/Words/anime.txt",
	})

	v.SetDefault("transcoder.encoder", "")
	v.SetDefault("transcoder.enabled", true)
	v.SetDefault("transcoder.hardware_accel", false)
	v.SetDefault("transcoder.preset", "veryfast")
	v.SetDefault("transcoder.video_bitrate", "1500k")
	v.SetDefault("transcoder.max_rate", "1800k")
	v.SetDefault("transcoder.buf_size", "3000k")
	v.SetDefault("transcoder.max_height", 720)
	v.SetDefault("transcoder.segment_seconds", 4)
	v.SetDefault("transcoder.realtime", true)
	v.SetDefault("transcoder.threads", 2)
	v.SetDefault("transcoder.max_concurrent", 1)
	v.SetDefault("transcoder.idle_timeout_seconds", 120)

	// API Config 默认设置
	v.SetDefault("api_config.auto_encrypt", true)
	v.SetDefault("api_config.default_timeout", 30)

	v.SetDefault("license.server_url", defaultLicenseServerURL)
	v.SetDefault("license.hmac_secret", "")
	v.SetDefault("license.public_key", defaultLicensePublicKey)
}
