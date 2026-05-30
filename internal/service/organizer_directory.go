// Package service — organize an arbitrary source directory (e.g. the download
// directory) into the destination library with dedup + 洗版 (resolution
// replacement).
//
// Unlike OrganizeLibraryWithOptions, which only touches model.Media rows that
// already belong to a registered library, OrganizeDirectory walks the source
// directory on disk directly. This lets operators organize the whole download
// directory (/downloads or a NAS direct-read path configured by the operator)
// even though it is not a registered library.
//
// Two protections requested by operators:
//
//   - 去重：目的地已存在同一媒体时不再从来源整理过去（避免重复 / 多倍占用存储）。
//   - 洗版：若来源分辨率高于目的地已存在的版本，则用高分辨率替换低分辨率。
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// OrganizeSourceCandidate is a selectable organize source directory surfaced to
// the UI so operators can organize an arbitrary directory (such as the download
// directory) and not only registered libraries.
type OrganizeSourceCandidate struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	Kind  string `json:"kind"` // "download" | "media"
}

// OrganizeSourceCandidates returns the configured directories that are valid
// organize sources (download dir + media dir). It uses the container-visible
// paths; in NAS direct-read mode those equal the host paths the operator sees.
func (o *OrganizerService) OrganizeSourceCandidates() []OrganizeSourceCandidate {
	out := []OrganizeSourceCandidate{}
	seen := map[string]struct{}{}
	add := func(label, path, kind string) {
		path = strings.TrimSpace(path)
		if path == "" || path == "." || strings.HasPrefix(path, ".") {
			return
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		out = append(out, OrganizeSourceCandidate{Label: label, Path: clean, Kind: kind})
	}
	add("下载目录", envOrDefault("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", "/downloads"), "download")
	add("媒体目录", envOrDefault("MEDIASTATION_MEDIA_CONTAINER_DIR", "/media"), "media")
	return out
}

// defaultDestRoot resolves the destination root for a directory organize:
// explicit override → organize.target_dir setting → media container dir.
func (o *OrganizerService) defaultDestRoot(ctx context.Context, override string) string {
	if r := strings.TrimSpace(override); r != "" {
		return r
	}
	if o.repo != nil && o.repo.Setting != nil {
		if v, err := o.repo.Setting.Get(ctx, "organize.target_dir"); err == nil && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return envOrDefault("MEDIASTATION_MEDIA_CONTAINER_DIR", "/media")
}

// OrganizeDirectory organizes every video file found under opts.SourcePath into
// the destination root, applying dedup + 洗版 (resolution replacement).
func (o *OrganizerService) OrganizeDirectory(ctx context.Context, opts OrganizeOptions) (*OrganizeResult, error) {
	source := strings.TrimSpace(opts.SourcePath)
	if source == "" {
		return nil, errors.New("source path required")
	}
	source = filepath.Clean(source)
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("source directory not accessible: %s", source)
	}
	dest := filepath.Clean(o.defaultDestRoot(ctx, opts.DestPath))
	if dest == "" || dest == "." {
		return nil, errors.New("destination path required")
	}
	mode := o.resolveTransferMode(ctx, opts.TransferMode)
	res := &OrganizeResult{}
	walkErr := walk(source, func(path string, wi walkInfo) error {
		if wi.isDir {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := videoExtensions[ext]; !ok {
			return nil
		}
		if err := o.organizeSourceFile(ctx, path, dest, mode, res); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", filepath.Base(path), err.Error()))
		}
		return nil
	})
	if walkErr != nil {
		return res, walkErr
	}
	o.log.Info("organize directory finished",
		zap.String("source", source),
		zap.String("dest", dest),
		zap.String("mode", string(mode)),
		zap.Int("organized", res.Organized),
		zap.Int("replaced", res.Replaced),
		zap.Int("skipped", res.Skipped),
	)
	return res, nil
}

// organizeSourceFile organizes a single video file from the source directory
// into destRoot, applying dedup + 洗版.
func (o *OrganizerService) organizeSourceFile(ctx context.Context, src, destRoot string, mode TransferMode, res *OrganizeResult) error {
	ext := filepath.Ext(src)
	title, year := CleanQuery(src)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(src), ext)
	}
	// CleanQuery lowercases the parsed title; title-case it so organized output
	// matches typical library casing (and stays consistent for dedup).
	parsedTitle := title
	title = sanitizeFilename(titleCaseWords(title))
	if title == "" {
		title = "Unknown"
	}
	season, episode := ParseEpisode(src)

	var destDir, dst, episodeTag string
	if season > 0 || episode > 0 {
		// TV/动漫/综艺等剧集：{destRoot}/{Title}/Season XX/{Title} - SxxExx.ext
		episodeTag = fmt.Sprintf("S%02dE%02d", season, episode)
		destDir = filepath.Join(destRoot, title, fmt.Sprintf("Season %02d", season))
		dst = filepath.Join(destDir, fmt.Sprintf("%s - %s%s", title, episodeTag, ext))
	} else {
		// 电影：{destRoot}/{Title} ({Year})/{Title} ({Year}).ext
		folder := title
		if year > 0 {
			folder = fmt.Sprintf("%s (%d)", title, year)
		}
		destDir = filepath.Join(destRoot, folder)
		dst = filepath.Join(destDir, folder+ext)
	}

	// 源文件已经位于目标位置：无需处理。
	if filepath.Clean(src) == filepath.Clean(dst) {
		res.Skipped++
		return nil
	}

	// 去重候选：合并「目的地媒体库已扫描入库的同一媒体（按标题/年份/季集匹配，
	// 不受目录大小写或布局影响）」与「目标文件夹内已存在的同名视频文件」。
	existing := o.existingVersionPaths(ctx, destRoot, destDir, parsedTitle, episodeTag, year, season, episode)
	if len(existing) > 0 {
		srcArea := o.resolutionArea(ctx, src)
		bestArea := 0
		for _, e := range existing {
			if a := o.resolutionArea(ctx, e); a > bestArea {
				bestArea = a
			}
		}
		// 洗版：仅当来源与已存在版本的分辨率都可判定、且来源更高时才替换；
		// 任一方分辨率未知时保守跳过，绝不删除无法判定的已存在文件。
		if srcArea > 0 && bestArea > 0 && srcArea > bestArea {
			if err := o.replaceVersions(ctx, src, existing, dst, mode); err != nil {
				return err
			}
			o.log.Info("organize replaced lower-resolution media",
				zap.String("from", src),
				zap.String("to", dst),
				zap.Int("src_area", srcArea),
				zap.Int("existing_area", bestArea),
			)
			res.Replaced++
			return nil
		}
		// 去重：目的地已存在同一媒体且不低于来源分辨率，跳过不再整理过去。
		o.log.Debug("organize skip duplicate",
			zap.String("src", src), zap.String("dest_dir", destDir))
		res.Skipped++
		return nil
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		res.Skipped++
		return nil
	}
	if err := transferFile(src, dst, mode); err != nil {
		return err
	}
	if err := transferSidecarNFO(src, dst, mode); err != nil {
		o.log.Warn("organize sidecar nfo failed",
			zap.String("from", src), zap.String("to", dst), zap.Error(err))
	}
	res.Organized++
	return nil
}

// existingVersionPaths returns existing destination files that represent the
// same media, combining two strategies and de-duplicating by path:
//
//  1. DB identity: media rows already scanned into the destination root whose
//     title (case-insensitive) + year [or + season/episode] match the source.
//     This is robust to directory case/layout differences.
//  2. Filesystem: video files inside the computed destination folder (matching
//     the SxxExx tag for episodes). Covers destinations that were not scanned.
func (o *OrganizerService) existingVersionPaths(ctx context.Context, destRoot, destDir, title, episodeTag string, year, season, episode int) []string {
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
	for _, p := range o.existingByIdentity(ctx, destRoot, title, year, season, episode) {
		add(p)
	}
	for _, p := range o.existingByFolder(destDir, episodeTag) {
		add(p)
	}
	return out
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

// titleCaseWords upper-cases the first letter of each ASCII word; CJK and other
// non-ASCII leading characters are left untouched. Roman numerals (ii, iii, iv,
// …) are fully upper-cased so sequels like "Wandering Earth II" keep their
// canonical casing instead of becoming "Ii".
func titleCaseWords(s string) string {
	fields := strings.Fields(s)
	for i, w := range fields {
		if isRomanNumeral(w) {
			fields[i] = strings.ToUpper(w)
			continue
		}
		r := []rune(w)
		if len(r) > 0 && r[0] < 128 {
			r[0] = unicode.ToUpper(r[0])
			fields[i] = string(r)
		}
	}
	return strings.Join(fields, " ")
}

// sequelNumerals is a conservative whitelist of multi-letter Roman numerals
// used for movie/series sequels. A whitelist avoids false positives on normal
// English words that happen to be valid numerals (e.g. "mix", "civ", "mi").
var sequelNumerals = map[string]struct{}{
	"ii": {}, "iii": {}, "iv": {}, "vi": {}, "vii": {}, "viii": {},
	"ix": {}, "xi": {}, "xii": {}, "xiii": {}, "xiv": {}, "xv": {},
}

func isRomanNumeral(w string) bool {
	_, ok := sequelNumerals[strings.ToLower(w)]
	return ok
}

// replaceVersions removes the existing lower-resolution files (and their NFO
// sidecars + DB rows) and transfers src into dst.
func (o *OrganizerService) replaceVersions(ctx context.Context, src string, existing []string, dst string, mode TransferMode) error {
	for _, e := range existing {
		if err := os.Remove(e); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove existing %s: %w", e, err)
		}
		if nfo := nfoPath(e); nfo != "" {
			_ = os.Remove(nfo)
		}
		if o.repo != nil && o.repo.DB != nil {
			_ = o.repo.DB.WithContext(ctx).Where("path = ?", e).Delete(&model.Media{}).Error
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := transferFile(src, dst, mode); err != nil {
		return err
	}
	if err := transferSidecarNFO(src, dst, mode); err != nil {
		o.log.Warn("organize sidecar nfo failed",
			zap.String("from", src), zap.String("to", dst), zap.Error(err))
	}
	return nil
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
