// Package service — library / media bookkeeping.
package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// MediaService offers high-level CRUD over libraries and media items.
type MediaService struct {
	cfg   *config.Config
	log   *zap.Logger
	repo  *repository.Container
	cache *RuntimeCacheService
}

type MediaVisibility struct {
	IncludeNSFW       bool
	AllowedLibraryIDs []string
	HiddenLibraryIDs  []string
}

type MediaItem struct {
	model.Media
	Versions []model.Media `json:"versions,omitempty"`
}

const maxMediaSearchLimit = 50000
const maxMediaSearchPageSize = 2000

func (v MediaVisibility) Allows(media *model.Media) bool {
	if media == nil {
		return false
	}
	if !v.IncludeNSFW && media.NSFW {
		return false
	}
	for _, id := range v.HiddenLibraryIDs {
		if id == media.LibraryID {
			return false
		}
	}
	if len(v.AllowedLibraryIDs) == 0 {
		return true
	}
	for _, id := range v.AllowedLibraryIDs {
		if id == media.LibraryID {
			return true
		}
	}
	return false
}

// NewMediaService is the constructor.
func NewMediaService(cfg *config.Config, log *zap.Logger, repo *repository.Container) *MediaService {
	return &MediaService{cfg: cfg, log: log, repo: repo}
}

func (s *MediaService) SetRuntimeCache(cache *RuntimeCacheService) *MediaService {
	if s != nil {
		s.cache = cache
	}
	return s
}

// CreateLibrary persists a library after validating that its path exists.
func (s *MediaService) CreateLibrary(ctx context.Context, name, path, kind string) (*model.Library, error) {
	if name == "" || path == "" {
		return nil, errors.New("name and path required")
	}
	abs, err := resolveAccessibleLibraryPath(path)
	if err != nil {
		return nil, err
	}
	kind = inferLibraryKind(name, abs, kind)
	lib := &model.Library{Name: name, Path: abs, Type: kind, Enabled: true}
	if err := s.repo.Library.Create(ctx, lib); err != nil {
		return nil, err
	}
	s.invalidateMediaCache(ctx)
	return lib, nil
}

func inferLibraryKind(name, path, requested string) string {
	requested = normalizeOrganizeMediaType(requested)
	text := strings.ToLower(name + " " + filepath.ToSlash(path))
	switch {
	case containsAnyText(text, "成人", "番号", "jav", "9kg", "adult", "nsfw"):
		return "adult"
	case containsAnyText(text, "综艺", "真人秀", "variety"):
		return "variety"
	case containsAnyText(text, "国漫", "日漫", "日番", "动漫", "动画", "anime", "bangumi") && !containsAnyText(text, "动画电影"):
		return "anime"
	case containsAnyText(text, "电视剧", "国产剧", "欧美剧", "日韩剧", "日剧", "韩剧", "剧集", "tv", "series"):
		return "tv"
	case containsAnyText(text, "电影", "movie", "film"):
		return "movie"
	case containsAnyText(text, "音乐", "歌曲", "music", "song", "songs"):
		return "music"
	}
	if requested != "" {
		return requested
	}
	return "movie"
}

func resolveAccessibleLibraryPath(path string) (string, error) {
	input := strings.TrimSpace(path)
	if input == "" {
		return "", errors.New("path required")
	}
	for _, candidate := range mappedPathCandidates(input) {
		if isAccessibleDir(candidate) {
			return filepath.Clean(candidate), nil
		}
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	return "", fmt.Errorf("path is not an accessible directory: %s", abs)
}

func resolveAccessibleMappedPath(path string) (string, os.FileInfo, error) {
	input := strings.TrimSpace(path)
	if input == "" {
		return "", nil, errors.New("path required")
	}
	candidates := mappedPathCandidates(input)
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil {
			return filepath.Clean(candidate), info, nil
		}
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", nil, fmt.Errorf("invalid path: %w", err)
	}
	return "", nil, fmt.Errorf("path is not accessible: %s", abs)
}

func resolveMappedDestinationPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if _, err := os.Stat(clean); err == nil {
		return clean
	}
	for _, candidate := range mappedPathCandidates(clean) {
		if candidate == clean {
			continue
		}
		return filepath.Clean(candidate)
	}
	return clean
}

func mappedPathCandidates(input string) []string {
	var candidates []string
	add := func(candidate string) {
		candidate = filepath.Clean(filepath.FromSlash(strings.TrimSpace(candidate)))
		if candidate == "" || candidate == "." {
			return
		}
		for _, existing := range candidates {
			if sameLibraryPath(existing, candidate) {
				return
			}
		}
		candidates = append(candidates, candidate)
	}
	clean := filepath.Clean(input)
	add(clean)
	for _, candidate := range dockerVolumePathCandidates(input) {
		add(candidate)
	}
	for _, candidate := range dockerVolumePathCandidates(clean) {
		add(candidate)
	}
	if slashClean := cleanPathForVolumeMapping(input); slashClean != "" {
		add(slashClean)
	}
	if abs, err := filepath.Abs(input); err == nil {
		add(abs)
		for _, candidate := range dockerVolumePathCandidates(abs) {
			add(candidate)
		}
	}
	return candidates
}

func isAccessibleDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func dockerVolumePathCandidates(path string) []string {
	normalized := cleanPathForVolumeMapping(path)
	var candidates []string
	addCandidate := func(candidate string) {
		candidate = filepath.Clean(filepath.FromSlash(candidate))
		for _, existing := range candidates {
			if sameLibraryPath(existing, candidate) {
				return
			}
		}
		candidates = append(candidates, candidate)
	}

	for _, mapping := range []struct {
		env       string
		container string
	}{
		{env: "MEDIASTATION_MEDIA_DIR", container: envOrDefault("MEDIASTATION_MEDIA_CONTAINER_DIR", "/media")},
		{env: "MEDIASTATION_DOWNLOAD_DIR", container: envOrDefault("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", "/downloads")},
	} {
		host := cleanPathForVolumeMapping(os.Getenv(mapping.env))
		if host == "." || host == "" || strings.HasPrefix(host, ".") {
			continue
		}
		if normalized == host {
			addCandidate(mapping.container)
			continue
		}
		if strings.HasPrefix(normalized, host+"/") {
			addCandidate(mapping.container + strings.TrimPrefix(normalized, host))
		}
		container := cleanPathForVolumeMapping(mapping.container)
		if container == "." || container == "" || strings.HasPrefix(container, ".") {
			continue
		}
		if normalized == container {
			addCandidate(host)
			continue
		}
		if strings.HasPrefix(normalized, container+"/") {
			addCandidate(host + strings.TrimPrefix(normalized, container))
		}
	}

	for _, marker := range []struct {
		part      string
		container string
	}{
		{part: "/media", container: envOrDefault("MEDIASTATION_MEDIA_CONTAINER_DIR", "/media")},
		{part: "/downloads", container: envOrDefault("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", "/downloads")},
	} {
		part := strings.TrimRight(marker.part, "/")
		container := strings.TrimRight(filepath.ToSlash(marker.container), "/")
		markerPath := pathAfterWindowsDrivePrefix(normalized)
		if markerPath == part {
			addCandidate(container)
			continue
		}
		if strings.HasPrefix(markerPath, part+"/") {
			addCandidate(container + strings.TrimPrefix(markerPath, part))
		}
	}

	return candidates
}

func cleanPathForVolumeMapping(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "\\", "/")
	path = trimEmbeddedWindowsDrive(path)
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
}

func pathAfterWindowsDrivePrefix(path string) string {
	if len(path) >= 3 && path[1] == ':' && path[2] == '/' && isASCIIAlpha(path[0]) {
		return path[2:]
	}
	return path
}

func trimEmbeddedWindowsDrive(path string) string {
	for i := 0; i+2 < len(path); i++ {
		if !isASCIIAlpha(path[i]) || path[i+1] != ':' || path[i+2] != '/' {
			continue
		}
		if i == 0 || path[i-1] == '/' {
			return path[i:]
		}
	}
	return path
}

func isASCIIAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func sameLibraryPath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// ListLibraries returns every library configured on the server.
func (s *MediaService) ListLibraries(ctx context.Context) ([]model.Library, error) {
	return s.repo.Library.List(ctx)
}

// DeleteLibrary removes a library and its media rows. The on-disk files are
// left untouched.
func (s *MediaService) DeleteLibrary(ctx context.Context, id string) error {
	lib, err := s.repo.Library.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if lib != nil {
		if _, ok := ParseCloudLibraryMount(lib.Path); ok {
			if err := s.repo.Media.PurgeByLibrary(ctx, id); err != nil {
				return err
			}
			err := s.repo.DB.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&model.Library{}).Error
			if err == nil {
				s.invalidateMediaCache(ctx)
			}
			return err
		}
	}
	if err := s.repo.Media.DeleteByLibrary(ctx, id); err != nil {
		return err
	}
	err = s.repo.Library.Delete(ctx, id)
	if err == nil {
		s.invalidateMediaCache(ctx)
	}
	return err
}

// ListMedia paginates media items inside a library.
func (s *MediaService) ListMedia(ctx context.Context, libraryID string, page, pageSize int) ([]model.Media, int64, error) {
	return s.ListMediaVisible(ctx, libraryID, page, pageSize, MediaVisibility{IncludeNSFW: true})
}

func (s *MediaService) ListMediaVisible(ctx context.Context, libraryID string, page, pageSize int, visibility MediaVisibility) ([]model.Media, int64, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 2000 {
		pageSize = 2000
	}
	if page < 1 {
		page = 1
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	libraryIDs, err := MergedLibraryIDsForLibrary(ctx, s.repo, libraryID)
	if err != nil {
		return nil, 0, err
	}
	filter := repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	}
	cacheKey := s.mediaListCacheKey(libraryID, libraryIDs, page, pageSize, filter)
	var cached mediaListCacheValue
	if s.cache != nil && s.cache.GetJSON(ctx, cacheKey, &cached) {
		s.attachLibraryMetadata(ctx, cached.Items)
		return cached.Items, cached.Total, nil
	}
	items, total, err := s.repo.Media.ListByLibrariesFiltered(ctx, libraryIDs, (page-1)*pageSize, pageSize, filter)
	if err != nil {
		return nil, 0, err
	}
	s.attachLibraryMetadata(ctx, items)
	if s.cache != nil {
		s.cache.SetJSON(ctx, cacheKey, mediaListCacheValue{Items: items, Total: total}, time.Duration(s.mediaCacheTTLSeconds())*time.Second)
	}
	return items, total, nil
}

func (s *MediaService) ListMediaVisibleGrouped(ctx context.Context, libraryID string, page, pageSize int, visibility MediaVisibility) ([]MediaItem, int64, error) {
	items, total, err := s.ListMediaVisible(ctx, libraryID, page, pageSize, visibility)
	if err != nil {
		return nil, 0, err
	}
	grouped := groupMediaVersions(items)
	if total <= int64(len(items)) {
		return grouped, int64(len(grouped)), nil
	}
	groupTotal, err := s.countMediaVersionGroups(ctx, libraryID, total, visibility)
	if err != nil {
		return nil, 0, err
	}
	return grouped, groupTotal, nil
}

func (s *MediaService) countMediaVersionGroups(ctx context.Context, libraryID string, rawTotal int64, visibility MediaVisibility) (int64, error) {
	if rawTotal <= 0 {
		return 0, nil
	}
	pageSize := maxMediaSearchPageSize
	if pageSize <= 0 {
		pageSize = 2000
	}
	seen := make(map[string]struct{}, minInt64ToInt(rawTotal, maxMediaSearchLimit))
	var singles int64
	pages := int((rawTotal + int64(pageSize) - 1) / int64(pageSize))
	for page := 1; page <= pages; page++ {
		items, _, err := s.ListMediaVisible(ctx, libraryID, page, pageSize, visibility)
		if err != nil {
			return 0, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			key := mediaVersionGroupKey(item)
			if key == "" {
				singles++
				continue
			}
			seen[key] = struct{}{}
		}
		if len(items) < pageSize {
			break
		}
	}
	return int64(len(seen)) + singles, nil
}

func minInt64ToInt(a int64, b int) int {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a < int64(b) {
		return int(a)
	}
	return b
}

type mediaListCacheValue struct {
	Items []model.Media `json:"items"`
	Total int64         `json:"total"`
}

func (s *MediaService) mediaListCacheKey(libraryID string, libraryIDs []string, page, pageSize int, filter repository.MediaQueryFilter) string {
	allowed := append([]string(nil), filter.AllowedLibraryIDs...)
	hidden := append([]string(nil), filter.HiddenLibraryIDs...)
	libs := append([]string(nil), libraryIDs...)
	sort.Strings(allowed)
	sort.Strings(hidden)
	sort.Strings(libs)
	sum := sha1.Sum([]byte(strings.Join([]string{
		libraryID,
		strings.Join(libs, ","),
		fmt.Sprintf("%d:%d:%t", page, pageSize, filter.IncludeNSFW),
		strings.Join(allowed, ","),
		strings.Join(hidden, ","),
	}, "|")))
	return "media:list:" + hex.EncodeToString(sum[:])
}

func (s *MediaService) mediaCacheTTLSeconds() int {
	if s == nil || s.cfg == nil || s.cfg.Cache.MediaTTLSeconds < 1 {
		return 15
	}
	return s.cfg.Cache.MediaTTLSeconds
}

func (s *MediaService) invalidateMediaCache(ctx context.Context) {
	if s != nil && s.cache != nil {
		s.cache.DeletePrefix(ctx, "media:")
		s.cache.DeletePrefix(ctx, "stats:")
	}
}

func (s *MediaService) attachLibraryMetadata(ctx context.Context, items []model.Media) {
	if s == nil || s.repo == nil || s.repo.Library == nil || len(items) == 0 {
		return
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return
	}
	byID := make(map[string]model.Library, len(libs))
	for _, lib := range libs {
		byID[lib.ID] = lib
	}
	resolver := newMediaDisplayLibraryResolver(ctx, s.repo, libs)
	for i := range items {
		if lib, ok := byID[items[i].LibraryID]; ok {
			items[i].LibraryName = lib.Name
			items[i].LibraryPath = lib.Path
		}
		if lib, ok := resolver.DisplayLibraryForMedia(items[i]); ok {
			items[i].DisplayLibraryID = lib.ID
			items[i].DisplayLibraryName = lib.Name
			items[i].DisplayLibraryPath = lib.Path
		}
	}
}

type mediaDisplayLibraryResolver struct {
	byID              map[string]model.Library
	displayByID       map[string]model.Library
	displayByMergeKey map[string]model.Library
	displayLibraries  []model.Library
}

func newMediaDisplayLibraryResolver(ctx context.Context, repo *repository.Container, libs []model.Library) mediaDisplayLibraryResolver {
	displayLibraries := FilterDisplayCloudLibraries(ctx, repo, append([]model.Library(nil), libs...))
	resolver := mediaDisplayLibraryResolver{
		byID:              make(map[string]model.Library, len(libs)),
		displayByID:       make(map[string]model.Library, len(displayLibraries)),
		displayByMergeKey: make(map[string]model.Library, len(displayLibraries)),
		displayLibraries:  displayLibraries,
	}
	for _, lib := range libs {
		resolver.byID[lib.ID] = lib
	}
	for _, lib := range displayLibraries {
		resolver.displayByID[lib.ID] = lib
		if key, ok := CloudLibraryMergeKey(lib); ok {
			if _, exists := resolver.displayByMergeKey[key]; !exists {
				resolver.displayByMergeKey[key] = lib
			}
		}
	}
	return resolver
}

func (r mediaDisplayLibraryResolver) DisplayLibraryForMedia(media model.Media) (model.Library, bool) {
	if lib, ok := r.bestPathDisplayLibrary(media); ok {
		return lib, true
	}
	if lib, ok := r.displayByID[media.LibraryID]; ok {
		return lib, true
	}
	own, hasOwn := r.byID[media.LibraryID]
	if hasOwn {
		if key, ok := CloudLibraryMergeKey(own); ok {
			if lib, exists := r.displayByMergeKey[key]; exists {
				return lib, true
			}
		}
		return own, true
	}
	return model.Library{}, false
}

func (r mediaDisplayLibraryResolver) bestPathDisplayLibrary(media model.Media) (model.Library, bool) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(media.Path)), "cloud://") {
		mediaInfo, ok := ParseCloudLibraryMount(media.Path)
		if !ok {
			return model.Library{}, false
		}
		var best model.Library
		bestDepth := 0
		for _, lib := range r.displayLibraries {
			info, ok := ParseCloudLibraryMount(lib.Path)
			if !ok || info.Provider != mediaInfo.Provider || !lib.Enabled {
				continue
			}
			dir := strings.Trim(firstNonEmpty(info.DisplayDir, info.ScanDir), "/")
			if dir == "" {
				continue
			}
			mediaDir := strings.Trim(firstNonEmpty(mediaInfo.DisplayDir, mediaInfo.ScanDir), "/")
			if mediaDir != dir && !cloudMountAncestor(dir, mediaDir) {
				continue
			}
			depth := len(strings.Split(dir, "/"))
			if depth > bestDepth {
				best = lib
				bestDepth = depth
			}
		}
		if bestDepth > 0 {
			return best, true
		}
		return model.Library{}, false
	}

	mediaPath := cleanPathForVolumeMapping(media.Path)
	var best model.Library
	bestLen := 0
	for _, lib := range r.displayLibraries {
		if _, ok := ParseCloudLibraryMount(lib.Path); ok || !lib.Enabled {
			continue
		}
		libPath := cleanPathForVolumeMapping(lib.Path)
		if libPath == "" || libPath == "." {
			continue
		}
		if mediaPath != libPath && !strings.HasPrefix(mediaPath, strings.TrimRight(libPath, "/")+"/") {
			continue
		}
		if len(libPath) > bestLen {
			best = lib
			bestLen = len(libPath)
		}
	}
	if bestLen > 0 {
		return best, true
	}
	return model.Library{}, false
}

func groupMediaVersions(items []model.Media) []MediaItem {
	if len(items) == 0 {
		return nil
	}
	type group struct {
		key     string
		primary model.Media
		rows    []model.Media
	}
	groups := make([]group, 0, len(items))
	byKey := make(map[string]int, len(items))
	for _, item := range items {
		key := mediaVersionGroupKey(item)
		if key == "" {
			groups = append(groups, group{primary: item, rows: []model.Media{item}})
			continue
		}
		if idx, ok := byKey[key]; ok {
			groups[idx].rows = append(groups[idx].rows, item)
			if betterMediaVersion(item, groups[idx].primary) {
				groups[idx].primary = item
			}
			continue
		}
		byKey[key] = len(groups)
		groups = append(groups, group{key: key, primary: item, rows: []model.Media{item}})
	}
	out := make([]MediaItem, 0, len(groups))
	for _, g := range groups {
		sort.SliceStable(g.rows, func(i, j int) bool {
			return betterMediaVersion(g.rows[i], g.rows[j])
		})
		item := MediaItem{Media: g.primary}
		if len(g.rows) > 1 {
			item.Versions = g.rows
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func mediaVersionGroupKey(m model.Media) string {
	if m.SeasonNum > 0 || m.EpisodeNum > 0 {
		switch {
		case m.TMDbID > 0:
			return fmt.Sprintf("episode:tmdb:%d:%d:%d", m.TMDbID, m.SeasonNum, m.EpisodeNum)
		case m.BangumiID > 0:
			return fmt.Sprintf("episode:bangumi:%d:%d:%d", m.BangumiID, m.SeasonNum, m.EpisodeNum)
		case strings.TrimSpace(m.DoubanID) != "":
			return fmt.Sprintf("episode:douban:%s:%d:%d", strings.ToLower(strings.TrimSpace(m.DoubanID)), m.SeasonNum, m.EpisodeNum)
		case strings.TrimSpace(m.TheTVDBID) != "":
			return fmt.Sprintf("episode:thetvdb:%s:%d:%d", strings.ToLower(strings.TrimSpace(m.TheTVDBID)), m.SeasonNum, m.EpisodeNum)
		}
		title := firstNonEmpty(m.OriginalName, m.Title)
		if title == "" {
			title, _ = CleanQuery(m.Path)
		}
		title = normalizeMediaVersionText(title)
		if title == "" {
			return ""
		}
		return strings.Join([]string{
			"episode",
			strings.ToLower(strings.TrimSpace(m.LibraryID)),
			title,
			fmt.Sprintf("%d:%d", m.SeasonNum, m.EpisodeNum),
		}, "|")
	}
	switch {
	case m.TMDbID > 0:
		return fmt.Sprintf("tmdb:%d", m.TMDbID)
	case m.BangumiID > 0:
		return fmt.Sprintf("bangumi:%d", m.BangumiID)
	case strings.TrimSpace(m.DoubanID) != "":
		return "douban:" + strings.ToLower(strings.TrimSpace(m.DoubanID))
	case strings.TrimSpace(m.TheTVDBID) != "":
		return "thetvdb:" + strings.ToLower(strings.TrimSpace(m.TheTVDBID))
	}
	title := firstNonEmpty(m.OriginalName, m.Title)
	if title == "" {
		title, _ = CleanQuery(m.Path)
	}
	title = normalizeMediaVersionText(title)
	if title == "" {
		return ""
	}
	year := m.Year
	if year <= 0 {
		_, year = CleanQuery(m.Path)
	}
	return fmt.Sprintf("movie:%s:%d", title, year)
}

func normalizeMediaVersionText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case '.', '_', '-', ' ', '\t', '/', '\\', '[', ']', '(', ')', '（', '）', '【', '】':
			return true
		default:
			return false
		}
	})
	out := fields[:0]
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, noise := noiseTokenSet[field]; noise {
			continue
		}
		out = append(out, field)
	}
	return strings.Join(out, " ")
}

func betterMediaVersion(candidate, current model.Media) bool {
	candidateCloud := isCloudMediaVersion(candidate)
	currentCloud := isCloudMediaVersion(current)
	if candidateCloud != currentCloud {
		return !candidateCloud
	}
	candidatePixels := candidate.Width * candidate.Height
	currentPixels := current.Width * current.Height
	if candidatePixels != currentPixels {
		return candidatePixels > currentPixels
	}
	if candidate.SizeBytes != current.SizeBytes {
		return candidate.SizeBytes > current.SizeBytes
	}
	return candidate.CreatedAt.After(current.CreatedAt)
}

func isCloudMediaVersion(media model.Media) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(media.Path)), "cloud://") ||
		strings.Contains(strings.ToLower(strings.TrimSpace(media.STRMURL)), "/api/cloud/play/")
}

// SearchMedia performs a simple LIKE search across titles.
func (s *MediaService) SearchMedia(ctx context.Context, query string, limit int) ([]model.Media, error) {
	return s.SearchMediaVisible(ctx, query, limit, MediaVisibility{IncludeNSFW: true})
}

func (s *MediaService) SearchMediaVisible(ctx context.Context, query string, limit int, visibility MediaVisibility) ([]model.Media, error) {
	if limit <= 0 {
		limit = 50
	} else if limit > maxMediaSearchLimit {
		limit = maxMediaSearchLimit
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	items, err := s.repo.Media.SearchFiltered(ctx, query, limit, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
	if err != nil {
		return nil, err
	}
	s.attachLibraryMetadata(ctx, items)
	return items, nil
}

func (s *MediaService) SearchMediaVisibleGrouped(ctx context.Context, query string, limit int, visibility MediaVisibility) ([]MediaItem, error) {
	items, err := s.SearchMediaVisible(ctx, query, limit, visibility)
	if err != nil {
		return nil, err
	}
	return groupMediaVersions(items), nil
}

func (s *MediaService) SearchMediaVisiblePage(ctx context.Context, query string, page, pageSize int, visibility MediaVisibility) ([]model.Media, int64, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > maxMediaSearchPageSize {
		pageSize = maxMediaSearchPageSize
	}
	if page < 1 {
		page = 1
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	items, total, err := s.repo.Media.SearchFilteredPage(ctx, query, (page-1)*pageSize, pageSize, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
	if err != nil {
		return nil, 0, err
	}
	s.attachLibraryMetadata(ctx, items)
	return items, total, nil
}

func (s *MediaService) SearchMediaVisiblePageGrouped(ctx context.Context, query string, page, pageSize int, visibility MediaVisibility) ([]MediaItem, int64, error) {
	items, _, err := s.SearchMediaVisiblePage(ctx, query, page, pageSize, visibility)
	if err != nil {
		return nil, 0, err
	}
	grouped := groupMediaVersions(items)
	return grouped, int64(len(grouped)), nil
}

// GetMedia returns a single media row.
func (s *MediaService) GetMedia(ctx context.Context, id string) (*model.Media, error) {
	media, err := s.repo.Media.FindByID(ctx, id)
	if err != nil || media == nil {
		return media, err
	}
	items := []model.Media{*media}
	s.attachLibraryMetadata(ctx, items)
	*media = items[0]
	return media, nil
}

const maxRecycleBinRecords = 200

// SoftDelete moves a media row to the recycle bin (gorm soft delete).
// The on-disk file is kept; admins can purge it later.
func (s *MediaService) SoftDelete(ctx context.Context, id string) error {
	media, err := s.repo.Media.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if media != nil && isCloudMediaPath(media.Path) {
		err := s.repo.DB.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&model.Media{}).Error
		if err == nil {
			s.invalidateMediaCache(ctx)
		}
		return err
	}
	err = s.repo.DB.WithContext(ctx).Where("id = ?", id).Delete(&model.Media{}).Error
	if err == nil {
		if pruneErr := pruneRecycleBinRows(ctx, s.repo.DB, maxRecycleBinRecords); pruneErr != nil {
			return pruneErr
		}
		s.invalidateMediaCache(ctx)
	}
	return err
}

// RestoreDeleted unsets DeletedAt for a single media row.
func (s *MediaService) RestoreDeleted(ctx context.Context, id string) error {
	err := s.repo.DB.WithContext(ctx).Unscoped().Model(&model.Media{}).
		Where("id = ?", id).Update("deleted_at", nil).Error
	if err == nil {
		s.invalidateMediaCache(ctx)
	}
	return err
}

// ListRecycleBin returns every soft-deleted row, newest first.
func (s *MediaService) ListRecycleBin(ctx context.Context, limit int) ([]model.Media, error) {
	if err := pruneRecycleBinRows(ctx, s.repo.DB, maxRecycleBinRecords); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxRecycleBinRecords {
		limit = maxRecycleBinRecords
	}
	var rows []model.Media
	err := s.repo.DB.Unscoped().
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func pruneRecycleBinRows(ctx context.Context, db *gorm.DB, keep int) error {
	if db == nil {
		return nil
	}
	if keep <= 0 {
		keep = maxRecycleBinRecords
	}
	var rows []struct {
		ID string
	}
	if err := db.WithContext(ctx).Unscoped().
		Model(&model.Media{}).
		Select("id").
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc").
		Limit(100000).
		Offset(keep).
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.ID != "" {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return db.WithContext(ctx).Unscoped().Where("id IN ?", ids).Delete(&model.Media{}).Error
}

// PurgeDeleted permanently removes a soft-deleted row from the database.
func (s *MediaService) PurgeDeleted(ctx context.Context, id string) error {
	err := s.repo.DB.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&model.Media{}).Error
	if err == nil {
		s.invalidateMediaCache(ctx)
	}
	return err
}
