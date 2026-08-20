package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// writeMediaArtworkFilesAfterScrape downloads the scraped poster/backdrop into
// the media folder as Jellyfin/Emby sidecars (<base>-poster.jpg /
// <base>-backdrop.jpg), scoped by the media file's base name so multiple movies
// sharing one directory never overwrite each other's artwork.
//
// Cloud media (cloud:// paths) are skipped entirely — the media folder is not
// writable for cloud mounts, and sidecars would be meaningless. Mirrors the
// local/cloud gating used by writeMediaNFOAfterScrape.
func (s *ScraperService) writeMediaArtworkFilesAfterScrape(ctx context.Context, m *model.Media, lib *model.Library) {
	if s == nil || s.images == nil || m == nil {
		return
	}
	cloudMedia := isCloudMediaPath(m.Path) || (lib != nil && isCloudMediaPath(lib.Path))
	if cloudMedia {
		return
	}
	// Reload the media from the DB so we use the freshly-saved poster/backdrop
	// URLs rather than the stale in-memory values (the caller updates the row
	// before invoking this).
	refreshed, err := s.repo.Media.FindByID(ctx, m.ID)
	if err != nil || refreshed == nil {
		return
	}
	dir := filepath.Dir(resolveMappedDestinationPath(refreshed.Path))
	if dir == "" || dir == "." {
		return
	}
	// Scope sidecar names by the media file's base name (e.g. A.mp4 -> A-poster.jpg)
	// so that multiple movies sharing one directory (A.mp4 + B.mp4) never clash.
	base := strings.TrimSuffix(filepath.Base(refreshed.Path), filepath.Ext(refreshed.Path))
	if base == "" || base == "." {
		return
	}
	if refreshed.PosterURL != "" {
		s.downloadArtworkToPath(ctx, dir, base+"-poster", refreshed.PosterURL)
	}
	if refreshed.BackdropURL != "" {
		s.downloadArtworkToPath(ctx, dir, base+"-backdrop", refreshed.BackdropURL)
	}
}

// downloadArtworkToPath fetches an artwork URL via the image proxy cache and
// writes it under dir/<name>.<ext>. It is best-effort: any failure is logged
// and returns without error so it never breaks scraping.
func (s *ScraperService) downloadArtworkToPath(ctx context.Context, dir, name, raw string) {
	if !isHTTPish(raw) {
		return
	}
	data, ctype, err := s.images.Fetch(ctx, raw)
	if err != nil || len(data) == 0 {
		s.log.Warn("scrape artwork download failed",
			zap.String("name", name),
			zap.String("url", raw),
			zap.Error(err))
		return
	}
	if !isImageContentType(ctype) || isTransparentPlaceholderData(data) {
		return
	}
	s.writeArtworkDataToPath(dir, name, ctype, data)
}

// writeArtworkDataToPath writes in-memory artwork bytes to dir/<name>.<ext>
// using a temp file + rename so readers never observe a partial file. Returns
// the destination path, or "" if the write failed.
func (s *ScraperService) writeArtworkDataToPath(dir, name, ctype string, data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.log.Warn("scrape artwork mkdir failed", zap.String("dir", dir), zap.Error(err))
		return ""
	}
	dst := filepath.Join(dir, name+imageExtForContentType(ctype))
	tmp, err := os.CreateTemp(dir, "img-*.tmp")
	if err != nil {
		s.log.Warn("scrape artwork temp create failed", zap.String("dir", dir), zap.Error(err))
		return ""
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		s.log.Warn("scrape artwork write failed", zap.String("dst", dst), zap.Error(err))
		return ""
	}
	_ = tmp.Close()
	if err := os.Rename(tmp.Name(), dst); err != nil {
		_ = os.Remove(tmp.Name())
		s.log.Warn("scrape artwork rename failed", zap.String("dst", dst), zap.Error(err))
		return ""
	}
	s.log.Debug("scrape artwork written", zap.String("dst", dst))
	return dst
}

// imageExtForContentType maps a detected image MIME type to a file extension.
// Unknown image types fall back to a generic ".img" so we never write an empty
// extension that could confuse media players.
func imageExtForContentType(ctype string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(ctype, ";")[0])) {
	case "image/jpeg", "image/pjpeg":
		return ".jpg"
	case "image/png", "image/x-png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/avif":
		return ".avif"
	default:
		return ".img"
	}
}
