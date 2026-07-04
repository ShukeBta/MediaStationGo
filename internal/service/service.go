// Package service 包含 MediaStationGo 的业务逻辑。
// Handler 反序列化 HTTP 请求，调用 Service 方法，然后序列化响应。
// Services 拥有所有横切策略（认证、扫描、转码等）且不直接处理 HTTP 类型。
package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// Container 持有在启动时初始化的每个服务。Handler 接收指向它的指针并选择相关字段。
type Container struct {
	Cfg              *config.Config
	Log              *zap.Logger
	Repo             *repository.Container
	WSHub            *Hub
	SSEHub           *SSEHub
	Tasks            *TaskTrackerService
	Auth             *AuthService
	Media            *MediaService
	Scan             *ScannerService
	Stream           *StreamService
	Transcoder       *TranscoderService
	FFprobe          *FFprobeService
	TMDb             *TMDbProvider
	Bangumi          *BangumiProvider
	TheTVDB          *TheTVDBProvider
	Fanart           *FanartProvider
	Scraper          *ScraperService
	Discover         *DiscoverService
	Playback         *PlaybackService
	ImageProxy       *ImageProxy
	Watcher          *WatcherService
	Downloads        *DownloadService
	Subscription     *SubscriptionService
	Subtitle         *SubtitleService
	Stats            *StatsService
	Profile          *ProfileService
	Audit            *AuditService
	NFO              *NFOService
	AI               *AIService
	APIConfig        *APIConfigService
	Crypto           *CryptoService
	Duplicate        *DuplicateService
	FileManager      *FileManagerService
	DLNA             *DLNAService
	Scheduler        *SchedulerService
	Storage          *StorageService
	Emby             *EmbyService
	Backup           *BackupService
	Notifier         *NotifierService
	NotifyChannels   *NotifyChannelService
	TelegramBot      *TelegramBotService
	PlayProfiles     *PlayProfileService
	Permissions      *PermissionService
	StorageCfg       *StorageConfigService
	STRM             *STRMService
	SystemUpdate     *SystemUpdateService
	DownloadClients  *DownloadClientService
	Assistant        *AssistantService
	Organizer        *OrganizerService
	OrganizePipeline *OrganizePipelineService
	Douban           *DoubanProvider
	Token            *TokenService
	ApiConfig        *ApiConfigService
	DownloadMgr      *DownloadManager
	Notify           *NotifyService
	Site             *SiteService
	Device           *DeviceService
	Cache            *RuntimeCacheService
	Sessions         *SessionTrackerService
	RecognitionWords *RecognitionWordsService

	stopCtx    context.Context
	stopCancel context.CancelFunc
}

// New 构建服务容器。
func New(cfg *config.Config, log *zap.Logger, repos *repository.Container) *Container {
	return NewWithVersion(cfg, log, repos, "dev")
}

// NewWithVersion 构建带应用版本信息的服务容器。
func NewWithVersion(cfg *config.Config, log *zap.Logger, repos *repository.Container, version string) *Container {
	return newServiceContainer(cfg, log, repos, version)
}

// Boot 启动后台工作进程（watcher, downloads poller, subscription scheduler）。
// 在 AutoMigrate 后调用一次。
func (c *Container) Boot() {
	if err := c.NormalizeLocalLibraryPaths(c.stopCtx); err != nil {
		c.Log.Warn("normalize local library paths failed", zap.Error(err))
	}
	if err := c.NormalizeCloudLibraryTypes(c.stopCtx); err != nil {
		c.Log.Warn("normalize cloud library types failed", zap.Error(err))
	}
	if err := c.Watcher.Start(c.stopCtx); err != nil {
		c.Log.Warn("watcher start failed", zap.Error(err))
	}
	c.Downloads.Start(c.stopCtx)
	c.Subscription.Start(c.stopCtx)
	if err := c.APIConfig.SeedDefaults(c.stopCtx); err != nil {
		c.Log.Warn("api config seed failed", zap.Error(err))
	}
	go c.warmMediaSearchIndex(c.stopCtx)

	// 加载所有已配置的下载客户端
	if err := c.DownloadMgr.LoadAll(c.stopCtx); err != nil {
		c.Log.Warn("failed to load download clients", zap.Error(err))
	}

	// 启动调度器定时任务
	c.Scheduler.Start(c.stopCtx)

	// 云盘存储健康检查
	c.BootCloudStorageHealthCheck(c.stopCtx)

	// 自动扫描云盘媒体库，使内容对所有用户立即可见
	c.BootCloudLibraries(c.stopCtx)

	// Mgo 保号规则巡检：默认关闭，由管理员通过 Telegram Bot 命令开启。
	// 每天触发一次评估；规则里的窗口可随机，不固定。
	if c.Device != nil {
		go c.runInactivitySweeper(c.stopCtx)
	}
}

// Context is canceled when the service container is closing.
func (c *Container) Context() context.Context {
	if c == nil || c.stopCtx == nil {
		return context.Background()
	}
	return c.stopCtx
}

// runInactivitySweeper periodically runs the account-cleanup policy. Kept with
// the historical name to avoid churn in callers.
func (c *Container) runInactivitySweeper(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := c.Device.SweepAccountCleanup(ctx); err != nil {
				c.Log.Warn("account cleanup sweep failed", zap.Error(err))
			} else if n > 0 {
				c.Log.Info("account cleanup sweep removed accounts", zap.Int("count", n))
			}
		}
	}
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
	if c.Cache != nil {
		_ = c.Cache.Close()
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
