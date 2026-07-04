package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const activeDownloadSnapshotFallbackAge = 2 * time.Minute

func (d *DownloadService) ActiveDownloadPaths(ctx context.Context) []string {
	if d == nil || d.qb == nil {
		return nil
	}
	live, err := d.qb.List(ctx, "")
	if err != nil {
		live = d.LiveTorrentSnapshot(activeDownloadSnapshotFallbackAge)
		if d.log != nil && len(live) == 0 {
			d.log.Debug("active download guard could not list qbittorrent and has no fresh snapshot", zap.Error(err))
		}
	}
	return activeDownloadPathCandidates(live, d.downloadPathMappings(ctx))
}

func (d *DownloadService) downloadPathMappings(ctx context.Context) map[string]string {
	mappings := map[string]string{
		"/var/apps/qBittorrent/shares/qBittorrent/Download": "/downloads",
		"/data/qBittorrent/downloads":                       "/downloads",
		"/downloads/qBittorrent":                            "/downloads",
	}
	for clientPrefix, localPrefix := range d.userPathMappings(ctx) {
		mappings[clientPrefix] = localPrefix
	}
	return mappings
}
