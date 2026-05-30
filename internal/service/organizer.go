// Package service — media file organizer (auto-rename + move).
//
// OrganizerService takes a source file (typically from a download
// completion) and moves/renames it into the configured library directory
// using a Jinja2-like template. The default templates:
//
//	movie: {Title} ({Year})/{Title} ({Year}).{Ext}
//	tv:    {Title}/Season {Season:02d}/{Title} - S{Season:02d}E{Episode:02d}.{Ext}
//
// Organizing moves the source file. On the same filesystem this is an
// os.Rename; across filesystems it falls back to streaming copy + source
// removal, so operators should keep downloads and media on the same volume
// when they want instant moves and no temporary duplicate disk usage.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// OrganizerService moves/renames files into library structures.
type OrganizerService struct {
	cfg  *config.Config
	log  *zap.Logger
	repo *repository.Container
}

// NewOrganizerService is the constructor.
func NewOrganizerService(cfg *config.Config, log *zap.Logger, repo *repository.Container) *OrganizerService {
	return &OrganizerService{cfg: cfg, log: log, repo: repo}
}

// OrganizeResult reports what happened.
type OrganizeResult struct {
	Organized int      `json:"organized"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors,omitempty"`
}

// OrganizeOptions carries per-request overrides for an organize operation.
// 空值表示沿用系统设置中的默认值。
//
// 整理是「从源目录整理到目的地目录」：SourcePath 指定待整理文件所在的源目录，
// DestPath 指定整理输出的目的地目录。两者相互独立，不再混用同一个目录。
type OrganizeOptions struct {
	// SourcePath 本次整理的源目录（待整理文件所在目录），覆盖 organize.source_dir
	// 设置与媒体库路径。仅整理位于该目录下的媒体；留空表示整个媒体库。
	SourcePath string
	// DestPath 本次整理的目的地根路径（整理输出到哪里），覆盖 organize.target_dir 设置。
	// 留空则使用设置中的默认目的地目录，再退回媒体库路径。
	DestPath string
	// TransferMode 本次整理的转移方式，覆盖 organize.transfer_mode 设置。
	TransferMode TransferMode
}

// OrganizeMedia moves a single media file into the target library directory.
// It auto-detects whether the media is a movie or TV episode based on the
// parsed season/episode numbers and builds the destination path accordingly.
// When smart classify is enabled, it adds a category subfolder (e.g., "华语电影").
func (o *OrganizerService) OrganizeMedia(ctx context.Context, mediaID string) (string, error) {
	return o.OrganizeMediaWithOptions(ctx, mediaID, OrganizeOptions{})
}

// OrganizeMediaWithOptions is OrganizeMedia with per-request overrides for the
// target path and transfer mode.
func (o *OrganizerService) OrganizeMediaWithOptions(ctx context.Context, mediaID string, opts OrganizeOptions) (string, error) {
	m, err := o.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return "", errors.New("media not found")
	}
	lib, err := o.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil || lib == nil {
		return "", errors.New("library not found")
	}
	baseRoot := o.resolveBaseRoot(ctx, lib, opts.DestPath)
	mode := o.resolveTransferMode(ctx, opts.TransferMode)
	if isSeriesLibraryType(lib.Type) {
		if err := o.refreshEpisodeIdentity(m, lib); err != nil {
			return "", err
		}
	}
	ext := filepath.Ext(m.Path)
	title := sanitizeFilename(m.Title)
	if title == "" {
		title = "Unknown"
	}

	// Determine category folder (if smart classify is enabled)
	category := o.SmartClassify(ctx, m)

	var dst string
	if isSeriesLibraryType(lib.Type) {
		// TV: {baseRoot}/[分类]/{Title}/Season XX/{Title} - SxxExx.ext
		season := fmt.Sprintf("Season %02d", m.SeasonNum)
		epTag := fmt.Sprintf("S%02dE%02d", m.SeasonNum, m.EpisodeNum)
		root := o.organizeRoot(baseRoot, lib.Type, category)
		dir := filepath.Join(categoryRoot(root, category), title, season)
		dst = filepath.Join(dir, fmt.Sprintf("%s - %s%s", title, epTag, ext))
	} else {
		// Movie: {baseRoot}/[分类]/{Title} ({Year})/{Title} ({Year}).ext
		folder := title
		if m.Year > 0 {
			folder = fmt.Sprintf("%s (%d)", title, m.Year)
		}
		root := o.organizeRoot(baseRoot, lib.Type, category)
		dir := filepath.Join(categoryRoot(root, category), folder)
		dst = filepath.Join(dir, folder+ext)
	}

	// Skip if already in place.
	if m.Path == dst {
		return dst, nil
	}

	// Refuse to overwrite an existing different file. 当多个 release（如
	// 不同字幕组、不同源）刮削后被统一改名，原本不重复的文件会被映射到
	// 同一个目标路径，盲目 move 会导致后者覆盖前者，造成数据丢失。
	if _, err := os.Stat(dst); err == nil {
		o.log.Warn("organize skipped: destination already exists",
			zap.String("media", m.ID),
			zap.String("from", m.Path),
			zap.String("to", dst))
		return dst, nil
	}

	// Create directories.
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}

	// Transfer the file according to the resolved mode. move 删除源；
	// copy/hardlink/symlink 保留源文件，从而让下载器可继续做种。
	if err := transferFile(m.Path, dst, mode); err != nil {
		return "", err
	}

	// Update the database row.
	if err := o.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"path":        dst,
			"season_num":  m.SeasonNum,
			"episode_num": m.EpisodeNum,
		}).Error; err != nil {
		return dst, err
	}
	if err := transferSidecarNFO(m.Path, dst, mode); err != nil {
		o.log.Warn("organize sidecar nfo failed",
			zap.String("media", m.ID),
			zap.String("from", nfoPath(m.Path)),
			zap.String("to", nfoPath(dst)),
			zap.Error(err))
	}
	o.log.Info("organized",
		zap.String("media", m.ID),
		zap.String("from", m.Path),
		zap.String("to", dst),
		zap.String("category", category),
		zap.String("mode", string(mode)),
	)
	return dst, nil
}

// resolveBaseRoot picks the organize destination root (目的地目录): a
// per-request override wins, then the organize.target_dir setting, then the
// library's own path.
func (o *OrganizerService) resolveBaseRoot(ctx context.Context, lib *model.Library, override string) string {
	if r := strings.TrimSpace(override); r != "" {
		return r
	}
	if o.repo != nil && o.repo.Setting != nil {
		if v, err := o.repo.Setting.Get(ctx, "organize.target_dir"); err == nil && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return lib.Path
}

// resolveSourceRoot picks the organize source root (源目录，待整理文件所在目录):
// a per-request override wins, then the organize.source_dir setting, then the
// library's own path. Library organize only touches media located under this
// root, so operators can point at a specific download/staging folder.
func (o *OrganizerService) resolveSourceRoot(ctx context.Context, lib *model.Library, override string) string {
	if r := strings.TrimSpace(override); r != "" {
		return r
	}
	if o.repo != nil && o.repo.Setting != nil {
		if v, err := o.repo.Setting.Get(ctx, "organize.source_dir"); err == nil && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return lib.Path
}

// resolveTransferMode picks the transfer mode: a per-request override wins,
// otherwise the organize.transfer_mode setting (default move). When the
// effective mode is move and 做种保种 (organize.keep_seeding) is enabled, it is
// upgraded to hardlink so the source stays in place for the torrent client.
func (o *OrganizerService) resolveTransferMode(ctx context.Context, override TransferMode) TransferMode {
	mode := override
	if mode == "" {
		mode = TransferMove
		if o.repo != nil && o.repo.Setting != nil {
			if v, err := o.repo.Setting.Get(ctx, "organize.transfer_mode"); err == nil && strings.TrimSpace(v) != "" {
				mode = parseTransferMode(v)
			}
		}
	}
	if mode == TransferMove && o.keepSeedingEnabled(ctx) {
		// 移动会删除源文件导致 qBittorrent 停止做种；保种开启时改用硬链接
		//（跨盘自动退化为复制），既规范命名又保留源文件继续做种上传。
		return TransferHardlink
	}
	return mode
}

// keepSeedingEnabled reports whether 做种保种 is on. Defaults to true so an
// unconfigured instance never silently breaks seeding on organize.
func (o *OrganizerService) keepSeedingEnabled(ctx context.Context) bool {
	if o.repo == nil || o.repo.Setting == nil {
		return true
	}
	v, err := o.repo.Setting.Get(ctx, "organize.keep_seeding")
	if err != nil || strings.TrimSpace(v) == "" {
		return true
	}
	return v == "true" || v == "1" || v == "on"
}

// OrganizeLibrary organizes every media row in a library whose file is
// not already in the expected path structure.
func (o *OrganizerService) OrganizeLibrary(ctx context.Context, libraryID string) (*OrganizeResult, error) {
	return o.OrganizeLibraryWithOptions(ctx, libraryID, OrganizeOptions{})
}

// OrganizeLibraryWithOptions is OrganizeLibrary with per-request overrides for
// the target path and transfer mode.
func (o *OrganizerService) OrganizeLibraryWithOptions(ctx context.Context, libraryID string, opts OrganizeOptions) (*OrganizeResult, error) {
	lib, err := o.repo.Library.FindByID(ctx, libraryID)
	if err != nil || lib == nil {
		return nil, errors.New("library not found")
	}
	var rows []model.Media
	if err := o.repo.DB.WithContext(ctx).
		Where("library_id = ? AND deleted_at IS NULL", libraryID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	// 源目录（待整理）：仅整理位于该目录下的媒体；留空 = 整个媒体库。
	sourceRoot := o.resolveSourceRoot(ctx, lib, opts.SourcePath)
	// 目的地目录：已位于该根下的文件视为已整理；受 dest_path 覆盖与设置影响。
	baseRoot := o.resolveBaseRoot(ctx, lib, opts.DestPath)
	res := &OrganizeResult{}
	for i := range rows {
		// 不在源目录内的文件跳过（不属于本次「从源目录整理」的范围）。
		if !pathWithin(rows[i].Path, sourceRoot) {
			res.Skipped++
			continue
		}
		if pathWithin(rows[i].Path, baseRoot) {
			res.Skipped++
			continue
		}
		dst, err := o.OrganizeMediaWithOptions(ctx, rows[i].ID, opts)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", rows[i].Title, err.Error()))
			continue
		}
		if dst == rows[i].Path {
			res.Skipped++
		} else {
			res.Organized++
		}
	}
	return res, nil
}

func (o *OrganizerService) refreshEpisodeIdentity(m *model.Media, lib *model.Library) error {
	season, episode := m.SeasonNum, m.EpisodeNum

	if parsedSeason, parsedEpisode := ParseEpisode(m.Path); parsedSeason > 0 || parsedEpisode > 0 {
		if parsedSeason > 0 {
			season = parsedSeason
		}
		if parsedEpisode > 0 {
			episode = parsedEpisode
		}
	}

	if local, err := ReadLocalMetadata(m.Path, lib.Path, true); err == nil && local != nil {
		if local.SeasonNum > 0 {
			season = local.SeasonNum
		}
		if local.EpisodeNum > 0 {
			episode = local.EpisodeNum
		}
	} else if err != nil {
		o.log.Warn("organize read local metadata failed", zap.String("path", m.Path), zap.Error(err))
	}

	if season <= 0 || episode <= 0 {
		return fmt.Errorf("cannot determine season/episode for %s", m.Path)
	}
	m.SeasonNum = season
	m.EpisodeNum = episode
	return nil
}

// moveFile tries os.Rename first (instant on same fs), then falls back
// to copy + remove for cross-device moves.
//
// 重要：如果 dst 已经存在，moveFile 会直接报错而不是覆盖。OrganizeMedia
// 已经在调用前做过 stat 检查，这里是第二道防线。
func moveFile(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("destination already exists: %s", dst)
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-device: stream copy → remove. This can temporarily consume the
	// destination file size while copying, but the source is removed after the
	// copy succeeds.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// O_EXCL 保证不会覆盖已存在的目标。
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, werr := io.Copy(f, in); werr != nil {
		f.Close()
		os.Remove(dst)
		return werr
	}
	if cerr := f.Close(); cerr != nil {
		return cerr
	}
	return os.Remove(src)
}

// transferSidecarNFO moves/copies/links the .nfo sidecar alongside its media
// using the same transfer mode, so metadata follows the organized file.
func transferSidecarNFO(srcMedia, dstMedia string, mode TransferMode) error {
	src := nfoPath(srcMedia)
	dst := nfoPath(dstMedia)
	if src == dst {
		return nil
	}
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return transferFile(src, dst, mode)
}

// sanitizeFilename removes characters not safe for filesystem names.
func sanitizeFilename(s string) string {
	r := strings.NewReplacer(
		"/", " ", "\\", " ", ":", " ", "*", "", "?", "",
		"\"", "", "<", "", ">", "", "|", "",
	)
	return strings.TrimSpace(r.Replace(s))
}

func (o *OrganizerService) organizeRoot(libraryPath, mediaType, category string) string {
	if strings.TrimSpace(category) == "" {
		return libraryPath
	}
	typeDir := mediaTypeRootDir(mediaType)
	if typeDir == "" || pathAlreadyEndsWith(libraryPath, typeDir) {
		return libraryPath
	}
	if isGenericMediaRoot(libraryPath) {
		return filepath.Join(libraryPath, typeDir)
	}
	return libraryPath
}

func categoryRoot(root, category string) string {
	if strings.TrimSpace(category) == "" || pathAlreadyEndsWith(root, category) {
		return root
	}
	return filepath.Join(root, category)
}

func pathWithin(path, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if strings.EqualFold(cleanPath, cleanRoot) {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func mediaTypeRootDir(mediaType string) string {
	switch normalizeMediaType(mediaType, "", "") {
	case "movie":
		return "电影"
	case "tv", "anime", "variety":
		return "电视剧"
	default:
		return ""
	}
}

func isGenericMediaRoot(path string) bool {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(filepath.Clean(path))))
	switch base {
	case "media", "medias", "library", "libraries", "organized", "整理":
		return true
	default:
		return false
	}
}

func pathAlreadyEndsWith(path, suffix string) bool {
	base := strings.TrimSpace(filepath.Base(filepath.Clean(path)))
	return strings.EqualFold(base, suffix)
}

func isSeriesLibraryType(mediaType string) bool {
	switch normalizeMediaType(mediaType, "", "") {
	case "tv", "anime", "variety":
		return true
	default:
		return false
	}
}

// isSmartClassifyEnabled checks if smart classify is enabled.
// It first checks the database setting, then falls back to config.yaml.
func (o *OrganizerService) isSmartClassifyEnabled(ctx context.Context) bool {
	// Try database first
	if o.repo != nil && o.repo.Setting != nil {
		val, err := o.repo.Setting.Get(ctx, "organizer.smart_classify")
		if err == nil && val != "" {
			return val == "true" || val == "1" || val == "on"
		}
	}
	// Fallback to config.yaml
	return o.cfg.Organizer.SmartClassify
}

// SmartClassify determines the subcategory folder based on media metadata.
// It returns the category folder name (e.g., "华语电影", "欧美剧", "日番").
// Returns empty string if smart classify is disabled or metadata is insufficient.
func (o *OrganizerService) SmartClassify(ctx context.Context, m *model.Media) string {
	// Check if smart classify is enabled (from database first, then config)
	smartClassify := o.isSmartClassifyEnabled(ctx)
	if !smartClassify {
		return ""
	}

	// Determine media type from library
	lib, err := o.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil || lib == nil {
		return ""
	}
	return o.classifyMedia(ctx, m, lib.Type)
}

// parseCommaList splits a comma-separated string into a slice of trimmed strings.
func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// contains checks if a string slice contains a specific string.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
