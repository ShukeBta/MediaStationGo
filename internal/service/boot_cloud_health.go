package service

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
)

const cloudStorageMissingConfigWarnPrefix = "cloud.storage.missing_config_warned."

// BootCloudStorageHealthCheck验证所有已配置的云盘存储在启动时是否可用
func (c *Container) BootCloudStorageHealthCheck(ctx context.Context) {
	if c == nil || c.StorageCfg == nil {
		return
	}

	configs, err := c.StorageCfg.List(ctx)
	if err != nil {
		c.Log.Warn("boot: cloud storage health check failed to list configs", zap.Error(err))
		return
	}

	cloudConfigs := make([]StorageView, 0)
	for _, cfg := range configs {
		if cfg.Enabled && IsAdminCloudConfigurable(cfg.Type) {
			cloudConfigs = append(cloudConfigs, cfg)
		}
	}

	if len(cloudConfigs) == 0 {
		c.Log.Info("boot: no enabled cloud storage configured")
		return
	}

	c.Log.Info("boot: checking cloud storage health", zap.Int("count", len(cloudConfigs)))

	for _, cfg := range cloudConfigs {
		go func(typ string) {
			checkCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			provider, err := c.StorageCfg.CloudProvider(checkCtx, typ)
			if err != nil {
				if c.warnMissingCloudStorageConfigOnce(checkCtx, typ, err) {
					return
				}
				c.Log.Warn("boot: cloud storage unavailable", zap.String("type", typ), zap.Error(err))
				return
			}

			if err := provider.Ping(checkCtx); err != nil {
				if c.warnMissingCloudStorageConfigOnce(checkCtx, typ, err) {
					return
				}
				c.Log.Warn("boot: cloud storage ping failed", zap.String("type", typ), zap.Error(err))
			} else {
				c.Log.Info("boot: cloud storage healthy", zap.String("type", typ))
			}
		}(cfg.Type)
	}
}

func (c *Container) warnMissingCloudStorageConfigOnce(ctx context.Context, typ string, err error) bool {
	reason := cloudStorageMissingConfigReason(err)
	if reason == "" {
		return false
	}
	if c == nil || c.Repo == nil || c.Repo.Setting == nil {
		if c != nil && c.Log != nil {
			c.Log.Warn("boot: cloud storage config incomplete; skipping health check", zap.String("type", typ), zap.String("reason", reason), zap.Error(err))
		}
		return true
	}
	key := cloudStorageMissingConfigWarnPrefix + strings.TrimSpace(typ) + "." + reason
	if value, getErr := c.Repo.Setting.Get(ctx, key); getErr == nil && strings.EqualFold(strings.TrimSpace(value), "true") {
		return true
	}
	if c.Log != nil {
		c.Log.Warn("boot: cloud storage config incomplete; skipping health check", zap.String("type", typ), zap.String("reason", reason), zap.Error(err))
	}
	if setErr := c.Repo.Setting.Set(ctx, key, "true"); setErr != nil && c.Log != nil {
		c.Log.Debug("remember cloud storage config warning failed", zap.String("type", typ), zap.Error(setErr))
	}
	return true
}

func cloudStorageMissingConfigReason(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "missing cookie") || (strings.Contains(msg, "missing") && strings.Contains(msg, "cookie")):
		return "missing_cookie"
	case strings.Contains(msg, "missing webdav url"):
		return "missing_webdav_url"
	default:
		return ""
	}
}
