// Package service 包含 MediaStationGo 的业务逻辑。
// Handler 反序列化 HTTP 请求，调用 Service 方法，然后序列化响应。
// Services 拥有所有横切策略（认证、扫描、转码等）且不直接处理 HTTP 类型。
package service

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// Container 持有在启动时初始化的每个服务。Handler 接收指向它的指针并选择相关字段。
type Container struct {
	Cfg             *config.Config
	Log             *zap.Logger
	Repo            *repository.Container
	WSHub           *Hub
	SSEHub          *SSEHub
	Auth            *AuthService
	Media           *MediaService
	Scan            *ScannerService
	Stream          *StreamService
	Transcoder      *TranscoderService
	FFprobe         *FFprobeService
	TMDb            *TMDbProvider
	Bangumi         *BangumiProvider
	TheTVDB         *TheTVDBProvider
	Fanart          *FanartProvider
	Scraper         *ScraperService
	Discover        *DiscoverService
	Playback        *PlaybackService
	ImageProxy      *ImageProxy
	Watcher         *WatcherService
	Downloads       *DownloadService
	Subscription    *SubscriptionService
	Subtitle        *SubtitleService
	Stats           *StatsService
	Profile         *ProfileService
	Audit           *AuditService
	NFO             *NFOService
	AI              *AIService
	APIConfig       *APIConfigService
	Crypto          *CryptoService
	Duplicate       *DuplicateService
	FileManager     *FileManagerService
	DLNA            *DLNAService
	Scheduler       *SchedulerService
	Storage         *StorageService
	Emby            *EmbyService
	Backup          *BackupService
	Notifier        *NotifierService
	NotifyChannels  *NotifyChannelService
	TelegramBot     *TelegramBotService
	PlayProfiles    *PlayProfileService
	Permissions     *PermissionService
	StorageCfg      *StorageConfigService
	DownloadClients *DownloadClientService
	Assistant       *AssistantService
	Organizer       *OrganizerService
	Douban          *DoubanProvider
	Token           *TokenService
	ApiConfig       *ApiConfigService
	DownloadMgr     *DownloadManager
	Notify          *NotifyService
	Site            *SiteService

	stopCtx    context.Context
	stopCancel context.CancelFunc
}

// New 构建服务容器。
func New(cfg *config.Config, log *zap.Logger, repos *repository.Container) *Container {
	ApplyRuntimeSettings(context.Background(), cfg, repos, log)

	hub := NewHub(log)
	go hub.Run()

	// 初始化 SSE Hub
	sseHub := NewSSEHub(log)
	go sseHub.Run()

	probe := NewFFprobeService(cfg, log)
	crypto := NewCryptoService(cfg.Secrets.JWTSecret, log)
	apiConfig := NewAPIConfigService(log, repos, crypto)
	tmdb := NewTMDbProvider(cfg, log, apiConfig)
	bangumi := NewBangumiProvider(cfg, log)
	thetvdb := NewTheTVDBProvider(cfg, log)
	fanart := NewFanartProvider(cfg, log)
	adult := NewAdultProvider(log, apiConfig)
	scraper := NewScraperService(cfg, log, repos, tmdb, bangumi, thetvdb, fanart, hub, adult)
	organizer := NewOrganizerService(cfg, log, repos)
	organizer.SetProbe(probe)
	discover := NewDiscoverService(log, tmdb)
	transcoder := NewTranscoderService(cfg, log, repos, hub)
	scanner := NewScannerService(cfg, log, repos, hub, probe, scraper)
	watcher := NewWatcherService(log, repos, scanner)
	nfo := NewNFOService(log, repos)
	ai := NewAIService(cfg, log, apiConfig)
	duplicate := NewDuplicateService(log, repos, hub)
	filemanager := NewFileManagerService(cfg, log, repos)
	dlna := NewDLNAService(log)
	storage := NewStorageService(log, repos)
	emby := NewEmbyService(cfg, log, repos)
	backup := NewBackupService(cfg, log, repos.DB)
	notifier := NewNotifierService(log, repos)
	notifyChannels := NewNotifyChannelService(log, repos)
	playProfiles := NewPlayProfileService(log, repos)
	permissions := NewPermissionService(log, repos)
	storageCfg := NewStorageConfigService(log, repos, crypto)
	downloadClients := NewDownloadClientService(log, repos)
	assistant := NewAssistantService(log, repos, ai)
	douban := NewDoubanProvider(cfg, log)
	scheduler := NewSchedulerService(log, repos, scanner, transcoder, hub, cfg.Cache.CacheDir)

	// 初始化认证相关服务
	tokenSvc := NewTokenService(cfg, log, repos)
	authSvc := NewAuthService(cfg, log, repos, tokenSvc, permissions)
	telegramBot := NewTelegramBotService(log, repos, crypto, authSvc)
	apiConfigSvc := NewApiConfigService(cfg, log, repos, crypto)
	downloadMgr := NewDownloadManager(log, repos, crypto)
	notifySvc := NewNotifyService(log, repos, crypto)

	// 构建 FlareSolverr URL（如果启用）
	flareSolverrURL := ""
	if cfg.FlareSolverr.Enabled && cfg.FlareSolverr.URL != "" {
		flareSolverrURL = cfg.FlareSolverr.URL
	}
	siteSvc := NewSiteService(log, repos, flareSolverrURL)
	downloads := NewDownloadService(log, repos, hub, organizer, siteSvc)
	subscription := NewSubscriptionService(cfg, log, repos, downloads, siteSvc, hub)

	// 让图片代理把媒体库根目录视为可读的本地图片位置：海报/封面等
	// sidecar 资源就存放在这些（用户自定义、任意）目录下，否则会被
	// 路径白名单挡掉、退化成占位图导致前端图片不显示。
	imageProxy := NewImageProxy(cfg, log)
	imageProxy.SetLibraryRootsProvider(func() []string {
		libs, err := repos.Library.List(context.Background())
		if err != nil {
			return nil
		}
		roots := make([]string, 0, len(libs))
		for _, l := range libs {
			if strings.TrimSpace(l.Path) != "" {
				roots = append(roots, l.Path)
			}
		}
		return roots
	})

	ctx, cancel := context.WithCancel(context.Background())

	return &Container{
		Cfg:             cfg,
		Log:             log,
		Repo:            repos,
		WSHub:           hub,
		SSEHub:          sseHub,
		Auth:            authSvc,
		Media:           NewMediaService(cfg, log, repos),
		Scan:            scanner,
		Stream:          NewStreamService(cfg, log, repos, transcoder),
		Transcoder:      transcoder,
		FFprobe:         probe,
		TMDb:            tmdb,
		Bangumi:         bangumi,
		TheTVDB:         thetvdb,
		Fanart:          fanart,
		Scraper:         scraper,
		Discover:        discover,
		Playback:        NewPlaybackService(log, repos),
		ImageProxy:      imageProxy,
		Watcher:         watcher,
		Downloads:       downloads,
		Subscription:    subscription,
		Subtitle:        NewSubtitleService(log, repos),
		Stats:           NewStatsService(log, repos),
		Profile:         NewProfileService(log, repos),
		Audit:           NewAuditService(log, repos),
		NFO:             nfo,
		AI:              ai,
		APIConfig:       apiConfig,
		Crypto:          crypto,
		Duplicate:       duplicate,
		FileManager:     filemanager,
		DLNA:            dlna,
		Scheduler:       scheduler,
		Storage:         storage,
		Emby:            emby,
		Backup:          backup,
		Notifier:        notifier,
		NotifyChannels:  notifyChannels,
		TelegramBot:     telegramBot,
		PlayProfiles:    playProfiles,
		Permissions:     permissions,
		StorageCfg:      storageCfg,
		DownloadClients: downloadClients,
		Assistant:       assistant,
		Organizer:       organizer,
		Douban:          douban,
		Token:           tokenSvc,
		ApiConfig:       apiConfigSvc,
		DownloadMgr:     downloadMgr,
		Notify:          notifySvc,
		Site:            siteSvc,
		stopCtx:         ctx,
		stopCancel:      cancel,
	}
}

// Boot 启动后台工作进程（watcher, downloads poller, subscription scheduler）。
// 在 AutoMigrate 后调用一次。
func (c *Container) Boot() {
	if err := c.Watcher.Start(c.stopCtx); err != nil {
		c.Log.Warn("watcher start failed", zap.Error(err))
	}
	c.Downloads.Start(c.stopCtx)
	c.Subscription.Start(c.stopCtx)
	if err := c.APIConfig.SeedDefaults(c.stopCtx); err != nil {
		c.Log.Warn("api config seed failed", zap.Error(err))
	}

	// 加载所有已配置的下载客户端
	if err := c.DownloadMgr.LoadAll(c.stopCtx); err != nil {
		c.Log.Warn("failed to load download clients", zap.Error(err))
	}

	// 启动调度器定时任务
	c.Scheduler.Start(c.stopCtx)
}

// Close 释放 services 持有的任何资源（websocket hub, ffmpeg 转码, fsnotify, 后台轮询器）。
func (c *Container) Close() {
	if c.stopCancel != nil {
		c.stopCancel()
	}
	if c.Scheduler != nil {
		c.Scheduler.Stop()
	}
	if c.Watcher != nil {
		c.Watcher.Stop()
	}
	if c.Subscription != nil {
		c.Subscription.Stop()
	}
	if c.Downloads != nil {
		c.Downloads.Stop()
	}
	if c.Transcoder != nil {
		c.Transcoder.StopAll()
	}
	if c.WSHub != nil {
		c.WSHub.Stop()
	}
	if c.SSEHub != nil {
		c.SSEHub.Stop()
	}
	if c.Scheduler != nil {
		c.Scheduler.Stop()
	}
}

// unused guard
var _ = time.Now
