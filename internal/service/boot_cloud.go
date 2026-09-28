package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// BootCloudLibraries optionally scans cloud libraries after startup. It is
// disabled by default for huge cloud mounts; normal automatic refresh is handled
// by the nightly cloud_sync scheduler window, and operators can still scan
// manually at any time.
func (c *Container) BootCloudLibraries(ctx context.Context) {
	if c == nil || c.Repo == nil || c.Scan == nil {
		return
	}
	if !bootCloudLibraryScanEnabled(ctx, c.Repo) {
		c.Log.Info("boot: cloud library scans disabled; use manual scan or nightly cloud sync")
		return
	}
	libs, err := c.Repo.Library.List(ctx)
	if err != nil {
		c.Log.Warn("boot cloud libraries: list failed", zap.Error(err))
		return
	}
	libs = FilterScannableCloudLibraries(ctx, c.Repo, libs)
	cloudLibs := make([]model.Library, 0)
	for _, lib := range libs {
		if !lib.Enabled {
			continue
		}
		if _, ok := ParseCloudLibraryMount(lib.Path); ok {
			cloudLibs = append(cloudLibs, lib)
		}
	}
	if len(cloudLibs) == 0 {
		return
	}
	c.Log.Info("boot: scheduling cloud library scans", zap.Int("count", len(cloudLibs)))
	// 延迟3秒后启动，避免和系统初始化任务冲突
	time.AfterFunc(3*time.Second, func() {
		if ctx.Err() == nil {
			c.runBootCloudLibraryScanQueue(ctx, cloudLibs)
		}
	})
}

func (c *Container) runBootCloudLibraryScanQueue(ctx context.Context, cloudLibs []model.Library) {
	for _, lib := range cloudLibs {
		if ctx.Err() != nil {
			return
		}
		libID := lib.ID
		libName := lib.Name
		scanCtx, cancel := cloudScanContext(ctx, cloudScanTimeout(ctx, c.Repo, 24*time.Hour))
		c.Log.Info("boot: scanning cloud library", zap.String("id", libID), zap.String("name", libName))
		if _, err := c.Scan.ScanLibraryWithoutAutoScrape(scanCtx, libID); err != nil {
			c.Log.Warn("boot: cloud library scan failed", zap.String("id", libID), zap.String("name", libName), zap.Error(err))
		} else {
			c.Log.Info("boot: cloud library scan completed", zap.String("id", libID), zap.String("name", libName))
		}
		cancel()
	}
}

func bootCloudLibraryScanEnabled(ctx context.Context, repo *repository.Container) bool {
	if repo == nil || repo.Setting == nil {
		return false
	}
	value, err := repo.Setting.Get(ctx, "cloud.boot_scan_enabled")
	if err != nil {
		return false
	}
	return parseBoolSetting(value, false)
}
