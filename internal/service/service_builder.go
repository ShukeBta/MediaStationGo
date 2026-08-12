package service

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type serviceContainerBuilder struct {
	cfg     *config.Config
	log     *zap.Logger
	repos   *repository.Container
	version string
	c       *Container
}

func newServiceContainer(cfg *config.Config, log *zap.Logger, repos *repository.Container, version string) *Container {
	ApplyRuntimeSettings(context.Background(), cfg, repos, log)

	builder := &serviceContainerBuilder{
		cfg:     cfg,
		log:     log,
		repos:   repos,
		version: normalizeSystemUpdateVersion(version),
		c: &Container{
			Cfg:  cfg,
			Log:  log,
			Repo: repos,
		},
	}
	builder.startRealtimeServices()
	builder.initProviderServices()
	builder.initContentServices()
	builder.initAccessAndStorageServices()
	builder.initIdentityServices()
	builder.initSiteDownloadServices()
	builder.initImageProxy()
	builder.attachRuntimeContext()
	builder.initResourceImport()
	builder.recoverPipelineIngest()
	builder.recoverResourceImports()
	return builder.c
}

func (b *serviceContainerBuilder) initResourceImport() {
	service, err := NewResourceImportService(b.cfg.ResourceImport, b.log, b.repos, b.c.stopCtx)
	if err != nil {
		if b.log != nil {
			b.log.Error("resource import service initialization failed", zap.Error(err))
		}
		return
	}
	b.c.ResourceImport = service
	if b.c.Subtitle != nil && service != nil {
		subtitleClient, ok := service.client.(subtitlePipelineClient)
		if !ok {
			if b.log != nil {
				b.log.Error("resource pipeline client does not support subtitle operations")
			}
		} else {
			b.c.Subtitle.SetPipelineClient(subtitleClient)
		}
	}
	if b.c.Subscription != nil {
		b.c.Subscription.SetResourceImport(service)
	}
}

func (b *serviceContainerBuilder) recoverResourceImports() {
	if b.c.ResourceImport == nil {
		return
	}
	count, err := b.c.ResourceImport.Recover(context.Background())
	if err != nil {
		if b.log != nil {
			b.log.Error("resource import recovery failed", zap.Error(err))
		}
		return
	}
	if count > 0 && b.log != nil {
		b.log.Info("resource import jobs recovered", zap.Int("count", count))
	}
}

func (b *serviceContainerBuilder) recoverPipelineIngest() {
	if b.c.PipelineIngest == nil {
		return
	}
	count, err := b.c.PipelineIngest.Recover(context.Background())
	if err != nil {
		if b.log != nil {
			b.log.Error("pipeline ingest recovery failed", zap.Error(err))
		}
		return
	}
	if count > 0 && b.log != nil {
		b.log.Info("pipeline ingest jobs recovered", zap.Int("count", count))
	}
}

func (b *serviceContainerBuilder) startRealtimeServices() {
	b.c.WSHub = NewHub(b.log)
	go b.c.WSHub.Run()
	b.c.Tasks = NewTaskTrackerService(b.log, b.c.WSHub)
	b.c.SystemUpdate = NewSystemUpdateService(b.cfg, b.log, b.repos, b.c.Tasks, b.version)

	b.c.SSEHub = NewSSEHub(b.log)
	go b.c.SSEHub.Run()
}

func (b *serviceContainerBuilder) initProviderServices() {
	b.c.FFprobe = NewFFprobeService(b.cfg, b.log)
	b.c.Cache = NewRuntimeCacheService(b.cfg, b.log)
	b.configureMediaSearchBackend()

	b.c.Crypto = NewCryptoService(b.cfg.Secrets.JWTSecret, b.log)
	b.c.APIConfig = NewAPIConfigService(b.log, b.repos, b.c.Crypto)
	b.c.TMDb = NewTMDbProvider(b.cfg, b.log, b.c.APIConfig)
	b.c.Bangumi = NewBangumiProvider(b.cfg, b.log)
	b.c.TheTVDB = NewTheTVDBProvider(b.cfg, b.log)
	b.c.Douban = NewDoubanProvider(b.cfg, b.log)
	b.c.Fanart = NewFanartProvider(b.cfg, b.log)
	b.c.RecognitionWords = NewRecognitionWordsService(b.log, b.repos)

	b.c.Adult = NewAdultProvider(b.log, b.c.APIConfig)
	if b.cfg.FlareSolverr.Enabled {
		b.c.Adult.SetFlareSolverr(b.cfg.FlareSolverr.URL, b.cfg.FlareSolverr.Timeout)
	}
	b.c.Scraper = NewScraperService(
		b.cfg, b.log, b.repos,
		b.c.TMDb, b.c.Bangumi, b.c.TheTVDB, b.c.Fanart,
		b.c.WSHub, b.c.Adult,
	)
	b.c.Scraper.SetRuntimeCache(b.c.Cache)
	b.c.Scraper.SetDouban(b.c.Douban)
}

func (b *serviceContainerBuilder) configureMediaSearchBackend() {
	searchBackend := repository.NewOpenSearchMediaBackend(b.cfg.Search)
	if searchBackend == nil || b.repos == nil || b.repos.Media == nil {
		return
	}
	b.repos.Media.SetSearchBackend(searchBackend)
	if b.log != nil {
		b.log.Info("opensearch media search enabled", zap.String("index", b.cfg.Search.Index), zap.String("url", b.cfg.Search.OpenSearchURL))
	}
}

func (b *serviceContainerBuilder) initContentServices() {
	b.c.Organizer = NewOrganizerService(b.cfg, b.log, b.repos)
	b.c.Organizer.SetProbe(b.c.FFprobe)
	b.c.Organizer.SetScraper(b.c.Scraper)
	b.c.Discover = NewDiscoverService(b.log, b.c.TMDb).SetDouban(b.c.Douban)
	b.c.Transcoder = NewTranscoderService(b.cfg, b.log, b.repos, b.c.WSHub)
	b.c.Scan = NewScannerService(b.cfg, b.log, b.repos, b.c.WSHub, b.c.FFprobe, b.c.Scraper)
	b.c.Scan.SetOrganizer(b.c.Organizer)
	b.c.Scan.SetRuntimeCache(b.c.Cache)
	b.c.OrganizePipeline = NewOrganizePipelineService(b.log, b.repos, b.c.Organizer, b.c.Scan, b.c.Tasks)
	b.c.Watcher = NewWatcherService(b.log, b.repos, b.c.Scan)
	b.c.NFO = NewNFOService(b.log, b.repos)
	b.c.AI = NewAIService(b.cfg, b.log, b.c.APIConfig)
	b.c.Duplicate = NewDuplicateService(b.log, b.repos, b.c.WSHub)
	b.c.FileManager = NewFileManagerService(b.cfg, b.log, b.repos)
	b.c.DLNA = NewDLNAService(b.log)
	b.c.Storage = NewStorageService(b.log, b.repos)
	b.c.Emby = NewEmbyService(b.cfg, b.log, b.repos)
	b.c.Backup = NewBackupService(b.cfg, b.log, b.repos.DB)
	b.c.Notifier = NewNotifierService(b.log, b.repos)
	b.c.NotifyChannels = NewNotifyChannelService(b.log, b.repos)
	b.c.Scan.SetNotifyChannels(b.c.NotifyChannels)
	b.c.Scraper.SetNotifyChannels(b.c.NotifyChannels)
	b.c.Media = NewMediaService(b.cfg, b.log, b.repos).SetRuntimeCache(b.c.Cache).SetAI(b.c.AI).SetTaskTracker(b.c.Tasks)
	b.c.PipelineMaintenance = NewPipelineMaintenanceService(b.log, b.repos).SetRuntimeCache(b.c.Cache)
	b.c.PipelineIngest = NewPipelineIngestService(b.log, b.repos, b.c.Scan, b.c.PipelineMaintenance, b.c.Tasks)
	b.c.PipelineScrape = NewPipelineScrapeService(b.repos, b.c.Scraper)
	b.c.Stream = NewStreamService(b.cfg, b.log, b.repos, b.c.Transcoder)
	b.c.Playback = NewPlaybackService(b.log, b.repos)
	b.c.Stream.SetPlaybackService(b.c.Playback)
	b.c.Emby.SetPlaybackService(b.c.Playback)
	b.c.Subtitle = NewSubtitleService(b.log, b.repos).SetAPIConfig(b.c.APIConfig)
	b.c.Stats = NewStatsService(b.log, b.repos).SetRuntimeCache(b.c.Cache)
	b.c.Profile = NewProfileService(b.log, b.repos)
	b.c.Audit = NewAuditService(b.log, b.repos)
}

func (b *serviceContainerBuilder) initAccessAndStorageServices() {
	b.c.PlayProfiles = NewPlayProfileService(b.log, b.repos)
	b.c.Permissions = NewPermissionService(b.log, b.repos)
	b.c.StorageCfg = NewStorageConfigService(b.log, b.repos, b.c.Crypto)
	b.c.GeneratedArtwork = NewGeneratedArtworkService(b.cfg, b.log, b.repos, b.c.StorageCfg, b.c.Cache, b.c.Tasks)
	b.c.Stream.SetCloudProbe(b.c.StorageCfg)
	b.c.Stream.SetCloudTrackProbe(b.c.FFprobe)
	b.c.Stream.SetRuntimeCache(b.c.Cache)
	b.c.Stream.SetGeneratedArtworkService(b.c.GeneratedArtwork)
	b.c.Media.SetCloudMediaDeleter(b.c.StorageCfg)
	b.c.STRM = NewSTRMService(b.log, b.repos, b.cfg)
	b.c.Scan.SetStorageConfig(b.c.StorageCfg)
	b.c.Subtitle.SetStorageConfig(b.c.StorageCfg)
	b.c.PipelineIngest.SetSubtitleService(b.c.Subtitle)
	b.c.Scan.SetSubtitleService(b.c.Subtitle)
	b.c.Scan.SetGeneratedArtworkService(b.c.GeneratedArtwork)
	b.c.StorageCfg.SetChangeHandler(func(provider string) {
		b.c.Subtitle.InvalidateCloudDiscovery("", provider)
	})
	b.c.Emby.SetSubtitleService(b.c.Subtitle)
	b.c.Emby.SetRuntimeCache(b.c.Cache)
	b.c.Emby.SetGeneratedArtworkService(b.c.GeneratedArtwork)
	b.c.DownloadClients = NewDownloadClientService(b.log, b.repos)
	b.c.Assistant = NewAssistantService(b.log, b.repos, b.c.AI)
	b.c.Scheduler = NewSchedulerService(
		b.log, b.repos, b.c.Scan, b.c.Transcoder,
		b.c.Organizer, b.c.StorageCfg, b.c.WSHub, b.cfg.Cache.CacheDir,
	)
	b.c.Scheduler.SetTaskTracker(b.c.Tasks)
	b.c.Scheduler.SetOrganizePipeline(b.c.OrganizePipeline)
	b.c.Scheduler.SetAdultProvider(b.c.Adult)
	b.c.Scheduler.SetDiscover(b.c.Discover)
}

func (b *serviceContainerBuilder) initIdentityServices() {
	b.c.Token = NewTokenService(b.cfg, b.log, b.repos)
	b.c.Auth = NewAuthService(b.cfg, b.log, b.repos, b.c.Token, b.c.Permissions)
	b.c.Sessions = NewSessionTrackerService(b.log)
	b.c.Device = NewDeviceService(b.log, b.repos)
	b.c.Device.SetSessionTracker(b.c.Sessions)
	b.c.TelegramBot = NewTelegramBotService(b.log, b.repos, b.c.Crypto, b.c.Auth)
	b.c.TelegramBot.SetDeviceService(b.c.Device)
	b.c.TelegramBot.SetBackupService(b.c.Backup)
	// Device enforcement notifies users through their Telegram binding before destructive actions.
	b.c.Device.SetNotifier(b.c.TelegramBot.NotifyUserByID)
	b.c.ApiConfig = NewApiConfigService(b.cfg, b.log, b.repos, b.c.Crypto)
	b.c.DownloadMgr = NewDownloadManager(b.log, b.repos, b.c.Crypto)
	b.c.Notify = NewNotifyService(b.log, b.repos, b.c.Crypto)
}

func (b *serviceContainerBuilder) initSiteDownloadServices() {
	b.c.Site = NewSiteService(b.log, b.repos, b.flareSolverrURL())
	b.c.Downloads = NewDownloadService(b.log, b.repos, b.c.WSHub, b.c.Organizer, b.c.Site)
	b.c.Organizer.SetActiveDownloadPathProvider(b.c.Downloads.ActiveDownloadPaths)
	b.c.Downloads.SetScanner(b.c.Scan)
	b.c.Downloads.SetTaskTracker(b.c.Tasks)
	b.c.Downloads.SetOrganizePipeline(b.c.OrganizePipeline)
	b.c.Downloads.SetNotifyChannels(b.c.NotifyChannels)
	b.c.Subscription = NewSubscriptionService(b.cfg, b.log, b.repos, b.c.Downloads, b.c.Site, b.c.WSHub)
	b.c.Subscription.SetScraper(b.c.Scraper)
	b.c.Subscription.SetNotifyChannels(b.c.NotifyChannels)
}

func (b *serviceContainerBuilder) initImageProxy() {
	b.c.ImageProxy = NewImageProxy(b.cfg, b.log)
	b.c.ImageProxy.SetLibraryRootsProvider(b.libraryRoots)
	b.c.Scan.SetImageProxy(b.c.ImageProxy)
	b.c.Scraper.SetImageProxy(b.c.ImageProxy)
	b.c.Discover.SetImageProxy(b.c.ImageProxy)
}

func (b *serviceContainerBuilder) libraryRoots() []string {
	libs, err := b.repos.Library.List(context.Background())
	if err != nil {
		return nil
	}
	roots := make([]string, 0, len(libs))
	for _, l := range libs {
		if len(l.Roots) > 0 {
			for _, root := range l.Roots {
				if !root.Enabled || strings.TrimSpace(root.Path) == "" {
					continue
				}
				roots = append(roots, resolveMappedDestinationPath(root.Path))
			}
			continue
		}
		if strings.TrimSpace(l.Path) != "" {
			roots = append(roots, resolveMappedDestinationPath(l.Path))
		}
	}
	return roots
}

func (b *serviceContainerBuilder) flareSolverrURL() string {
	if b.cfg.FlareSolverr.Enabled && b.cfg.FlareSolverr.URL != "" {
		return b.cfg.FlareSolverr.URL
	}
	return ""
}

func (b *serviceContainerBuilder) attachRuntimeContext() {
	b.c.stopCtx, b.c.stopCancel = context.WithCancel(context.Background())
}
