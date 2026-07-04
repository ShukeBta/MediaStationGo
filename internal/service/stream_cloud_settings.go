package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func CloudPlaybackSettings(ctx context.Context, repo *repository.Container) CloudPlaybackOptions {
	opts := CloudPlaybackOptions{
		STRMEnabled:          false,
		RedirectProxyEnabled: true,
		PreferredMode:        CloudPlaybackModeRedirectProxy,
	}
	if repo == nil || repo.Setting == nil {
		return opts
	}
	modeRaw, hasMode := settingValue(ctx, repo, CloudPlaybackModeSettingKey)
	if mode := normalizeCloudPlaybackMode(modeRaw); mode != "" {
		opts.PreferredMode = mode
	}
	legacySTRM, hasLegacySTRM := settingValue(ctx, repo, STRMEnabledSettingKey)
	legacySTRMEnabled := hasLegacySTRM && parseBoolSetting(legacySTRM, false)
	if !hasMode && legacySTRMEnabled {
		opts.PreferredMode = CloudPlaybackModeSTRM
	}
	if raw, ok := settingValue(ctx, repo, CloudPlaybackSTRMEnabledSettingKey); ok {
		opts.STRMEnabled = parseBoolSetting(raw, false)
	} else if hasLegacySTRM {
		opts.STRMEnabled = legacySTRMEnabled
	} else if hasMode && opts.PreferredMode == CloudPlaybackModeSTRM {
		opts.STRMEnabled = true
	}
	if raw, ok := settingValue(ctx, repo, CloudPlaybackRedirectEnabledSettingKey); ok {
		opts.RedirectProxyEnabled = parseBoolSetting(raw, true)
	} else if hasMode && opts.PreferredMode == CloudPlaybackModeRedirectProxy {
		opts.RedirectProxyEnabled = true
	}
	if opts.PreferredMode == CloudPlaybackModeSTRM && !opts.STRMEnabled && opts.RedirectProxyEnabled {
		opts.PreferredMode = CloudPlaybackModeRedirectProxy
	}
	if opts.PreferredMode == CloudPlaybackModeRedirectProxy && !opts.RedirectProxyEnabled && opts.STRMEnabled {
		opts.PreferredMode = CloudPlaybackModeSTRM
	}
	return opts
}

func CloudPlaybackMode(ctx context.Context, repo *repository.Container) string {
	opts := CloudPlaybackSettings(ctx, repo)
	switch opts.PreferredMode {
	case CloudPlaybackModeSTRM:
		if opts.STRMEnabled {
			return CloudPlaybackModeSTRM
		}
		if opts.RedirectProxyEnabled {
			return CloudPlaybackModeRedirectProxy
		}
	case CloudPlaybackModeRedirectProxy:
		if opts.RedirectProxyEnabled {
			return CloudPlaybackModeRedirectProxy
		}
		if opts.STRMEnabled {
			return CloudPlaybackModeSTRM
		}
	}
	return ""
}

func STRMPlaybackEnabled(ctx context.Context, repo *repository.Container) bool {
	return CloudPlaybackSettings(ctx, repo).STRMEnabled
}

func cloudPlaybackModeEnabled(ctx context.Context, repo *repository.Container, mode string) bool {
	opts := CloudPlaybackSettings(ctx, repo)
	switch normalizeCloudPlaybackMode(mode) {
	case CloudPlaybackModeSTRM:
		return opts.STRMEnabled
	case CloudPlaybackModeRedirectProxy:
		return opts.RedirectProxyEnabled
	default:
		return opts.STRMEnabled || opts.RedirectProxyEnabled
	}
}

func settingValue(ctx context.Context, repo *repository.Container, key string) (string, bool) {
	if repo == nil || repo.Setting == nil {
		return "", false
	}
	v, err := repo.Setting.Get(ctx, key)
	if err != nil || strings.TrimSpace(v) == "" {
		return "", false
	}
	return v, true
}

func normalizeCloudPlaybackMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "strm", "strmurl", "strm_url", "api_stream", "api-stream":
		return CloudPlaybackModeSTRM
	case "302", "proxy", "reverse_proxy", "redirect", "redirect_proxy", "302_proxy", "302-proxy", "cloud":
		return CloudPlaybackModeRedirectProxy
	default:
		return ""
	}
}
