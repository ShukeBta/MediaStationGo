// Package service 包含 MediaStationGo 的业务逻辑。
// Handler 反序列化 HTTP 请求，调用 Service 方法，然后序列化响应。
// Services 拥有所有横切策略（认证、扫描、转码等）且不直接处理 HTTP 类型。
package service

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// Container 持有在启动时初始化的每个服务。Handler 接收指向它的指针并选择相关字段。
type Container struct {
	Cfg                 *config.Config
	Log                 *zap.Logger
	Repo                *repository.Container
	WSHub               *Hub
	SSEHub              *SSEHub
	Tasks               *TaskTrackerService
	Auth                *AuthService
	Media               *MediaService
	Scan                *ScannerService
	Stream              *StreamService
	Transcoder          *TranscoderService
	FFprobe             *FFprobeService
	TMDb                *TMDbProvider
	TMDbCatalog         *TMDbCatalogService
	Adult               *AdultProvider
	Bangumi             *BangumiProvider
	TheTVDB             *TheTVDBProvider
	Fanart              *FanartProvider
	Scraper             *ScraperService
	Discover            *DiscoverService
	Playback            *PlaybackService
	ImageProxy          *ImageProxy
	Watcher             *WatcherService
	Downloads           *DownloadService
	Subscription        *SubscriptionService
	Subtitle            *SubtitleService
	Stats               *StatsService
	Profile             *ProfileService
	Audit               *AuditService
	NFO                 *NFOService
	AI                  *AIService
	APIConfig           *APIConfigService
	ProxyPool           *ProxyPoolService
	Crypto              *CryptoService
	Duplicate           *DuplicateService
	FileManager         *FileManagerService
	DLNA                *DLNAService
	Scheduler           *SchedulerService
	Storage             *StorageService
	Emby                *EmbyService
	Backup              *BackupService
	Notifier            *NotifierService
	NotifyChannels      *NotifyChannelService
	TelegramBot         *TelegramBotService
	PlayProfiles        *PlayProfileService
	Permissions         *PermissionService
	StorageCfg          *StorageConfigService
	STRM                *STRMService
	SystemUpdate        *SystemUpdateService
	DownloadClients     *DownloadClientService
	Assistant           *AssistantService
	Organizer           *OrganizerService
	OrganizePipeline    *OrganizePipelineService
	Douban              *DoubanProvider
	Token               *TokenService
	ApiConfig           *ApiConfigService
	DownloadMgr         *DownloadManager
	Notify              *NotifyService
	Site                *SiteService
	Device              *DeviceService
	Cache               *RuntimeCacheService
	Sessions            *SessionTrackerService
	PlayerRequestLogs   *PlayerRequestLogService
	RecognitionWords    *RecognitionWordsService
	PipelineMaintenance *PipelineMaintenanceService
	PipelineIngest      *PipelineIngestService
	PipelineScrape      *PipelineScrapeService
	ResourceImport      *ResourceImportService
	GeneratedArtwork    *GeneratedArtworkService
	Danmaku             *DanmakuService
	Startup             *StartupState
	Plugins             *PluginService

	stopCtx    context.Context
	stopCancel context.CancelFunc
	bootMu     sync.Mutex
	bootWG     sync.WaitGroup
	booted     bool
	closing    bool
	closeOnce  sync.Once
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
	c.bootMu.Lock()
	if c.booted || c.closing {
		c.bootMu.Unlock()
		return
	}
	c.booted = true
	c.bootWG.Add(1)
	c.bootMu.Unlock()
	defer c.bootWG.Done()
	ready := false
	defer func() {
		if !ready {
			c.Startup.finish("failed")
		}
	}()
	ctx := c.Context()
	c.PlayerRequestLogs.Start(ctx)
	if err := c.startupStep("检查媒体库路径", func() error { return c.NormalizeLocalLibraryPaths(ctx) }); err != nil {
		return
	}
	_ = c.startupStep("检查云盘媒体库类型", func() error { return c.NormalizeCloudLibraryTypes(ctx) })
	if c.Watcher != nil {
		_ = c.startupStep("建立媒体库目录监听", func() error { return c.Watcher.Start(ctx) })
	}
	if c.Downloads != nil {
		_ = c.startupStep("启动下载轮询", func() error { c.Downloads.Start(ctx); return nil })
	}
	if c.Subscription != nil {
		_ = c.startupStep("启动订阅服务", func() error { c.Subscription.Start(ctx); return nil })
	}
	if c.APIConfig != nil {
		_ = c.startupStep("加载资料源默认配置", func() error { return c.APIConfig.SeedDefaults(ctx) })
		if c.Cfg != nil {
			_ = c.startupStep("迁移豆瓣凭据", func() error { return c.APIConfig.MigrateDoubanCookie(ctx, c.Cfg.Secrets.DoubanCookie) })
		}
	}
	c.startPeopleWorker(ctx)
	_ = c.startupStep("启动搜索与媒体分组预热", func() error {
		go c.warmMediaSearchIndex(ctx)
		go c.warmMediaSeriesKeys(ctx)
		go c.warmMediaVersionKeys(ctx)
		return nil
	})
	if c.GeneratedArtwork != nil {
		_ = c.startupStep("启动封面生成", func() error { c.GeneratedArtwork.Start(ctx); return nil })
	}
	_ = c.startupStep("检查云盘存储与扫描配置", func() error {
		c.BootCloudStorageHealthCheck(ctx)
		c.BootCloudLibraries(ctx)
		return nil
	})
	if c.Device != nil {
		_ = c.startupStep("启动账号规则巡检", func() error { go c.runInactivitySweeper(ctx); return nil })
	}
	if err := c.startupStep("启动任务调度器", func() error {
		if c.Scheduler != nil {
			c.Scheduler.Start(ctx)
		}
		return ctx.Err()
	}); err != nil {
		return
	}
	c.Startup.finish("ready")
	ready = true
	if c.Scheduler != nil && c.Discover != nil && c.Adult != nil {
		go func() {
			if err := c.Scheduler.RunNow(c.stopCtx, "adult_discover_refresh"); err != nil && c.Log != nil {
				c.Log.Warn("adult discover startup refresh failed", zap.Error(err))
			}
		}()
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
	c.closeOnce.Do(c.closeServices)
}

func (c *Container) closeServices() {
	c.bootMu.Lock()
	c.closing = true
	if c.stopCancel != nil {
		c.stopCancel()
	}
	c.bootMu.Unlock()
	c.bootWG.Wait()
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
