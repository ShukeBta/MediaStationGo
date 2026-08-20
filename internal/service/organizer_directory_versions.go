package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// existingVersionPaths returns existing destination files that represent the
// same media, combining two strategies and de-duplicating by path:
//
//  1. DB identity: media rows already scanned into the destination root whose
//     title (case-insensitive) + year [or + season/episode] match the source.
//     This is robust to directory case/layout differences.
//  2. Filesystem: video files inside the computed destination folder (matching
//     the SxxExx tag for episodes). Covers destinations that were not scanned.
func (o *OrganizerService) existingVersionPaths(ctx context.Context, destRoot, destDir, title, episodeTag string, year, season, episode int) []string {
	return mergeExistingVersionPaths(
		o.existingByIdentity(ctx, destRoot, title, year, season, episode),
		o.existingByFolder(destDir, episodeTag),
	)
}

func mergeExistingVersionPaths(groups ...[]string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		if p == "" {
			return
		}
		c := filepath.Clean(p)
		if _, ok := seen[c]; ok {
			return
		}
		if _, err := os.Stat(c); err != nil {
			return
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	for _, group := range groups {
		for _, p := range group {
			add(p)
		}
	}
	return out
}

func (o *OrganizerService) allExistingPathsInDB(ctx context.Context, paths []string) bool {
	if o == nil || o.repo == nil || o.repo.DB == nil || len(paths) == 0 {
		return false
	}
	cleaned := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		cleaned = append(cleaned, path)
	}
	if len(cleaned) == 0 {
		return false
	}
	var count int64
	if err := o.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Where("path IN ?", cleaned).
		Count(&count).Error; err != nil {
		return false
	}
	return count == int64(len(cleaned))
}

// existingByIdentity finds scanned destination media matching the parsed
// identity (case-insensitive title + year for movies; title + season/episode
// for episodes), located under destRoot.
func (o *OrganizerService) existingByIdentity(ctx context.Context, destRoot, title string, year, season, episode int) []string {
	if o.repo == nil || o.repo.DB == nil {
		return nil
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	q := o.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("deleted_at IS NULL").
		Where("LOWER(title) = ?", strings.ToLower(title))
	if season > 0 || episode > 0 {
		q = q.Where("season_num = ? AND episode_num = ?", season, episode)
	} else if year > 0 {
		q = q.Where("year = ?", year)
	}
	var rows []model.Media
	if err := q.Find(&rows).Error; err != nil {
		return nil
	}
	var out []string
	for _, r := range rows {
		if r.Path != "" && pathWithin(r.Path, destRoot) {
			out = append(out, r.Path)
		}
	}
	return out
}

func (o *OrganizerService) existingByExternalIdentity(ctx context.Context, destRoot string, match *Match, season, episode int) []string {
	if o.repo == nil || o.repo.DB == nil || match == nil {
		return nil
	}
	var conds []string
	var args []any
	if match.TMDbID > 0 {
		conds = append(conds, "tm_db_id = ?")
		args = append(args, match.TMDbID)
	}
	if match.BangumiID > 0 {
		conds = append(conds, "bangumi_id = ?")
		args = append(args, match.BangumiID)
	}
	if strings.TrimSpace(match.DoubanID) != "" {
		conds = append(conds, "douban_id = ?")
		args = append(args, strings.TrimSpace(match.DoubanID))
	}
	if strings.TrimSpace(match.TheTVDBID) != "" {
		conds = append(conds, "thetvdb_id = ?")
		args = append(args, strings.TrimSpace(match.TheTVDBID))
	}
	if len(conds) == 0 {
		return nil
	}
	q := o.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("deleted_at IS NULL").
		Where("("+strings.Join(conds, " OR ")+")", args...)
	if season > 0 || episode > 0 {
		q = q.Where("season_num = ? AND episode_num = ?", season, episode)
	}
	var rows []model.Media
	if err := q.Find(&rows).Error; err != nil {
		return nil
	}
	var out []string
	for _, row := range rows {
		if row.Path != "" && pathWithin(row.Path, destRoot) {
			out = append(out, row.Path)
		}
	}
	return out
}

// existingByFolder returns video files already present in destDir that
// represent the same media. For an episode (episodeTag != "") it matches files
// carrying the same SxxExx tag; for a movie it matches every video file in the
// movie folder.
func (o *OrganizerService) existingByFolder(destDir, episodeTag string) []string {
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return nil
	}
	tag := strings.ToLower(episodeTag)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if _, ok := videoExtensions[strings.ToLower(filepath.Ext(name))]; !ok {
			continue
		}
		if tag != "" && !strings.Contains(strings.ToLower(name), tag) {
			continue
		}
		out = append(out, filepath.Join(destDir, name))
	}
	return out
}

// replaceVersions promotes a higher-resolution src into dst, then removes the
// old lower-resolution files (and their NFO sidecars + DB rows). dst may
// coincide with one of the existing versions being superseded, so the new file
// is first transferred to a temporary staging name inside the destination
// directory. Only after the transfer succeeds are the old versions removed and
// the staged file renamed into place. This keeps the lower-res versions intact
// if the transfer fails (hardlink across mounts, disk full, etc.) instead of
// destroying them before the new file exists.
func (o *OrganizerService) replaceVersions(ctx context.Context, src string, existing []string, dst string, mode TransferMode) error {
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0o755); err != nil { // #nosec G301 -- organized media directories must remain readable by NAS/player users.
		return err
	}
	// Unique staging name so dst (which may already exist as the lower-res
	// version) is never the transfer target and never clobbered early.
	stage := dst + ".replace" + randomSuffix()
	cleanup := func() {
		_ = os.Remove(stage)
		_ = os.Remove(nfoPath(stage))
		removeStagedArtwork(stage)
	}
	if err := transferFile(src, stage, mode); err != nil {
		cleanup()
		return err
	}
	if err := transferSidecarNFO(src, stage, mode); err != nil {
		o.log.Warn("organize replace sidecar nfo failed",
			zap.String("from", src), zap.String("to", dst), zap.Error(err))
	}
	if err := transferSidecarArtwork(src, stage, mode); err != nil {
		o.log.Warn("organize replace sidecar artwork failed",
			zap.String("from", src), zap.String("to", dst), zap.Error(err))
	}
	// New file is safely staged; the transfer succeeded so it is now safe to
	// supersede the existing lower-res versions.
	for _, e := range existing {
		if nfo := nfoPath(e); nfo != "" {
			_ = os.Remove(nfo)
		}
		if err := os.Remove(e); err != nil && !os.IsNotExist(err) {
			o.log.Warn("organize replace remove existing failed",
				zap.String("path", e), zap.Error(err))
		}
		if o.repo != nil && o.repo.DB != nil {
			_ = o.repo.DB.WithContext(ctx).Where("path = ?", e).Delete(&model.Media{}).Error
		}
	}
	// Move staged file + sidecars into the final path.
	if err := os.Rename(stage, dst); err != nil {
		cleanup()
		return err
	}
	moveSidecarRename(nfoPath(stage), nfoPath(dst))
	moveStagedArtwork(stage, dst)
	return nil
}

// randomSuffix returns a short random suffix for staging filenames.
func randomSuffix() string {
	return strconv.Itoa(int(time.Now().UnixNano()) & 0xffffff)
}

// moveStagedArtwork renames artwork sidecars staged alongside `stage` into
// their final names next to `dst`.
func moveStagedArtwork(stage, dst string) {
	stageBase := strings.TrimSuffix(filepath.Base(stage), filepath.Ext(stage))
	dstBase := strings.TrimSuffix(filepath.Base(dst), filepath.Ext(dst))
	stageDir := filepath.Dir(stage)
	for _, suffix := range artworkSidecarSuffixes {
		for _, ext := range artworkSidecarExtensions {
			srcPath := filepath.Join(stageDir, stageBase+suffix+ext)
			if _, err := os.Stat(srcPath); err != nil {
				continue
			}
			_ = os.Rename(srcPath, filepath.Join(stageDir, dstBase+suffix+ext))
		}
	}
}

// moveSidecarRename renames a staged sidecar to its final name if it exists.
func moveSidecarRename(from, to string) {
	if from == "" || to == "" {
		return
	}
	if _, err := os.Stat(from); err != nil {
		return
	}
	_ = os.Rename(from, to)
}

// removeStagedArtwork removes artwork sidecars that were staged alongside
// `stage`, used when a replace fails and its staged outputs must be cleaned up.
func removeStagedArtwork(stage string) {
	stageBase := strings.TrimSuffix(filepath.Base(stage), filepath.Ext(stage))
	stageDir := filepath.Dir(stage)
	for _, suffix := range artworkSidecarSuffixes {
		for _, ext := range artworkSidecarExtensions {
			path := filepath.Join(stageDir, stageBase+suffix+ext)
			_ = os.Remove(path)
		}
	}
}

// resolutionArea returns the pixel area (width*height) of a video file for 洗版
// comparison. It prefers ffprobe; when unavailable it falls back to a
// resolution token in the filename (2160p/1080p/720p). Returns 0 when the
// resolution cannot be determined, in which case the caller treats the file as
// "unknown" and never performs a destructive replace.
func (o *OrganizerService) resolutionArea(ctx context.Context, path string) int {
	// Prefer a scanned media row's stored dimensions. The destination library
	// is normally scanned with ffprobe, so its files have accurate Width/Height
	// even after organize stripped the resolution token from the filename.
	if o.repo != nil && o.repo.DB != nil {
		var m model.Media
		if err := o.repo.DB.WithContext(ctx).
			Select("width", "height").
			Where("path = ?", path).
			Limit(1).Take(&m).Error; err == nil && m.Width > 0 && m.Height > 0 {
			return m.Width * m.Height
		}
	}
	if o.probe != nil {
		if pr, err := o.probe.Probe(ctx, path); err == nil && pr != nil && pr.Width > 0 && pr.Height > 0 {
			return pr.Width * pr.Height
		}
	}
	switch detectResolutionScore(strings.ToLower(filepath.Base(path))) {
	case 4:
		return 3840 * 2160
	case 3:
		return 1920 * 1080
	case 2:
		return 1280 * 720
	default:
		return 0
	}
}
