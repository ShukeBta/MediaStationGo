package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const (
	materializedSubtitleScheme          = "materialized-subtitle"
	materializedSubtitleSource          = "cloud-media"
	maxMaterializedCloudSubtitleBytes   = 8 << 20
	maxMaterializedCloudSubtitleTotal   = 64 << 20
	maxMaterializedCloudSubtitleTracks  = 32
	materializedCloudSubtitleFreshness  = 24 * time.Hour
	materializedCloudSubtitleQueryBatch = 100
)

type CloudSubtitleMaterializeResult struct {
	Status     string `json:"status"`
	MediaID    string `json:"media_id"`
	Provider   string `json:"provider,omitempty"`
	Discovered int    `json:"discovered"`
	Cached     int    `json:"cached"`
	Removed    int    `json:"removed"`
	Reason     string `json:"reason,omitempty"`
	Error      string `json:"error,omitempty"`
}

type CloudSubtitleBackfillResult struct {
	LibraryID string   `json:"library_id,omitempty"`
	Total     int      `json:"total"`
	Processed int      `json:"processed"`
	Cached    int      `json:"cached"`
	Empty     int      `json:"empty"`
	Skipped   int      `json:"skipped"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

type materializedSubtitleIndex struct {
	MediaID   string                           `json:"media_id"`
	Provider  string                           `json:"provider"`
	MediaPath string                           `json:"media_path"`
	CachedAt  string                           `json:"cached_at"`
	Tracks    []materializedSubtitleIndexTrack `json:"tracks"`
}

type materializedSubtitleIndexTrack struct {
	Filename   string `json:"filename"`
	Name       string `json:"name"`
	Lang       string `json:"lang"`
	Label      string `json:"label"`
	SourcePath string `json:"source_path"`
}

func cloudSubtitleMaterializedDirFromEnv() string {
	return strings.TrimSpace(os.Getenv("MEDIASTATION_CLOUD_SUBTITLE_CACHE_DIR"))
}

func (s *SubtitleService) SetMaterializedCacheDir(dir string) {
	if s != nil {
		s.materializedCacheDir = strings.TrimSpace(dir)
	}
}

func (s *SubtitleService) EnsureCloudSubtitles(ctx context.Context, mediaID string) (CloudSubtitleMaterializeResult, error) {
	return s.materializeCloudSubtitles(ctx, mediaID, false)
}

func (s *SubtitleService) RefreshCloudSubtitles(ctx context.Context, mediaID string) (CloudSubtitleMaterializeResult, error) {
	return s.materializeCloudSubtitles(ctx, mediaID, true)
}

func (s *SubtitleService) materializeCloudSubtitles(ctx context.Context, mediaID string, force bool) (CloudSubtitleMaterializeResult, error) {
	result := CloudSubtitleMaterializeResult{Status: "skipped", MediaID: strings.TrimSpace(mediaID)}
	if s == nil || s.repo == nil || s.repo.Media == nil {
		return failedCloudSubtitleMaterializeResult(result, errors.New("subtitle service unavailable"))
	}
	if !safeSubtitleMediaID(result.MediaID) {
		return failedCloudSubtitleMaterializeResult(result, errors.New("invalid media id"))
	}
	media, err := s.repo.Media.FindByID(ctx, result.MediaID)
	if err != nil {
		return failedCloudSubtitleMaterializeResult(result, err)
	}
	if media == nil {
		return failedCloudSubtitleMaterializeResult(result, errors.New("media not found"))
	}
	provider, _, cloudMedia := cloudSubtitleMediaRef(*media)
	if !cloudMedia {
		result.Reason = "not_cloud_media"
		return result, nil
	}
	result.Provider = provider
	if strings.TrimSpace(s.materializedCacheDir) == "" {
		return failedCloudSubtitleMaterializeResult(result, errors.New("cloud subtitle cache directory is not configured"))
	}
	if !force {
		if discovered, cached, current := s.currentMaterializedCloudSubtitles(*media, provider); current {
			result.Discovered = discovered
			result.Cached = cached
			result.Reason = "cache_current"
			return result, nil
		}
	}
	if s.storage == nil {
		return failedCloudSubtitleMaterializeResult(result, errors.New("cloud storage service unavailable"))
	}

	s.materializeMu.Lock()
	defer s.materializeMu.Unlock()
	if !force {
		if discovered, cached, current := s.currentMaterializedCloudSubtitles(*media, provider); current {
			result.Discovered = discovered
			result.Cached = cached
			result.Reason = "cache_current"
			return result, nil
		}
	}

	tracks, err := loadCloudSubtitlesForMaterialization(ctx, s, *media, force)
	if err != nil {
		return failedCloudSubtitleMaterializeResult(result, err)
	}
	tracks = uniqueCloudSubtitleTracks(tracks)
	result.Discovered = len(tracks)
	if len(tracks) > maxMaterializedCloudSubtitleTracks {
		return failedCloudSubtitleMaterializeResult(result, fmt.Errorf("cloud subtitle count %d exceeds limit %d", len(tracks), maxMaterializedCloudSubtitleTracks))
	}

	root, err := filepath.Abs(s.materializedCacheDir)
	if err != nil {
		return failedCloudSubtitleMaterializeResult(result, err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return failedCloudSubtitleMaterializeResult(result, err)
	}
	stageDir, err := os.MkdirTemp(root, "."+media.ID+"-stage-")
	if err != nil {
		return failedCloudSubtitleMaterializeResult(result, err)
	}
	defer os.RemoveAll(stageDir)

	index := materializedSubtitleIndex{
		MediaID:   media.ID,
		Provider:  provider,
		MediaPath: media.Path,
		CachedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Tracks:    make([]materializedSubtitleIndexTrack, 0, len(tracks)),
	}
	totalBytes := 0
	for _, track := range tracks {
		typ, ref, name, ok := parseCloudSubtitlePath(track.Path)
		if !ok || !strings.EqualFold(typ, provider) {
			return failedCloudSubtitleMaterializeResult(result, fmt.Errorf("invalid cloud subtitle path %q", track.Path))
		}
		ext, ok := normalizeSubtitleExtension(filepath.Ext(firstNonEmpty(name, ref)))
		if !ok {
			return failedCloudSubtitleMaterializeResult(result, fmt.Errorf("unsupported cloud subtitle format %q", name))
		}
		body, err := s.storage.CloudReadText(ctx, typ, ref, maxMaterializedCloudSubtitleBytes)
		if err != nil {
			return failedCloudSubtitleMaterializeResult(result, fmt.Errorf("cache cloud subtitle %q: %w", name, err))
		}
		totalBytes += len(body)
		if totalBytes > maxMaterializedCloudSubtitleTotal {
			return failedCloudSubtitleMaterializeResult(result, fmt.Errorf("cloud subtitle total size exceeds limit %d", maxMaterializedCloudSubtitleTotal))
		}
		filename := materializedCloudSubtitleFilename(track.Path, ext)
		if err := os.WriteFile(filepath.Join(stageDir, filename), []byte(body), 0o644); err != nil {
			return failedCloudSubtitleMaterializeResult(result, err)
		}
		index.Tracks = append(index.Tracks, materializedSubtitleIndexTrack{
			Filename:   filename,
			Name:       firstNonEmpty(strings.TrimSpace(track.Name), filename),
			Lang:       track.Lang,
			Label:      track.Label,
			SourcePath: track.Path,
		})
	}
	indexBody, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return failedCloudSubtitleMaterializeResult(result, err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, "tracks.json"), append(indexBody, '\n'), 0o644); err != nil {
		return failedCloudSubtitleMaterializeResult(result, err)
	}
	removed, err := replaceMaterializedSubtitleDir(root, media.ID, stageDir)
	if err != nil {
		return failedCloudSubtitleMaterializeResult(result, err)
	}
	result.Status = "success"
	result.Cached = len(index.Tracks)
	result.Removed = removed
	return result, nil
}

func (s *SubtitleService) currentMaterializedCloudSubtitles(media model.Media, provider string) (discovered, cached int, current bool) {
	index, exists, err := readMaterializedSubtitleIndex(s.materializedCacheDir, media.ID)
	if err != nil || !exists || index.MediaID != media.ID || !strings.EqualFold(index.Provider, provider) || index.MediaPath != media.Path {
		return 0, 0, false
	}
	cachedAt, err := time.Parse(time.RFC3339Nano, index.CachedAt)
	age := time.Since(cachedAt)
	if err != nil || age < 0 || age >= materializedCloudSubtitleFreshness {
		return len(index.Tracks), 0, false
	}
	tracks, err := materializedTracksFromIndex(s.materializedCacheDir, media, index, true)
	if err != nil || len(tracks) != len(index.Tracks) {
		return len(index.Tracks), len(tracks), false
	}
	return len(index.Tracks), len(tracks), true
}

func failedCloudSubtitleMaterializeResult(result CloudSubtitleMaterializeResult, err error) (CloudSubtitleMaterializeResult, error) {
	result.Status = "failed"
	if err != nil {
		result.Error = err.Error()
	}
	return result, err
}

func uniqueCloudSubtitleTracks(tracks []SubtitleTrack) []SubtitleTrack {
	out := make([]SubtitleTrack, 0, len(tracks))
	seen := map[string]struct{}{}
	for _, track := range tracks {
		key := strings.TrimSpace(track.Path)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, track)
	}
	return out
}

func materializedCloudSubtitleFilename(sourcePath, ext string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(sourcePath)))
	return "cloud-" + hex.EncodeToString(sum[:8]) + ext
}

func replaceMaterializedSubtitleDir(root, mediaID, stageDir string) (int, error) {
	targetDir, ok := localSubtitleMediaDir(root, mediaID)
	if !ok {
		return 0, errors.New("materialized subtitle cache path invalid")
	}
	removed := countCachedSubtitleFiles(targetDir)
	backupDir := targetDir + ".old-" + fmt.Sprint(time.Now().UnixNano())
	hadTarget := false
	if _, err := os.Stat(targetDir); err == nil {
		hadTarget = true
		if err := os.Rename(targetDir, backupDir); err != nil {
			return 0, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	if err := os.Rename(stageDir, targetDir); err != nil {
		if hadTarget {
			if rollbackErr := os.Rename(backupDir, targetDir); rollbackErr != nil {
				return 0, fmt.Errorf("install materialized subtitle cache: %w; rollback failed: %v", err, rollbackErr)
			}
		}
		return 0, err
	}
	if hadTarget {
		if err := os.RemoveAll(backupDir); err != nil {
			return 0, fmt.Errorf("remove replaced subtitle cache %q: %w", backupDir, err)
		}
	}
	return removed, nil
}

func countCachedSubtitleFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, ok := normalizeSubtitleExtension(filepath.Ext(entry.Name())); ok {
			count++
		}
	}
	return count
}

func discoverMaterializedCloudSubtitles(s *SubtitleService, media model.Media, strict bool) ([]SubtitleTrack, error) {
	if s == nil || strings.TrimSpace(s.materializedCacheDir) == "" {
		return []SubtitleTrack{}, nil
	}
	index, exists, err := readMaterializedSubtitleIndex(s.materializedCacheDir, media.ID)
	if err != nil {
		if strict {
			return nil, err
		}
		return []SubtitleTrack{}, nil
	}
	if !exists {
		return []SubtitleTrack{}, nil
	}
	provider, _, ok := cloudSubtitleMediaRef(media)
	if !ok || index.MediaID != media.ID || !strings.EqualFold(index.Provider, provider) || index.MediaPath != media.Path {
		return []SubtitleTrack{}, nil
	}
	return materializedTracksFromIndex(s.materializedCacheDir, media, index, strict)
}

func readMaterializedSubtitleIndex(root, mediaID string) (*materializedSubtitleIndex, bool, error) {
	mediaDir, ok := localSubtitleMediaDir(root, mediaID)
	if !ok {
		return nil, false, errors.New("materialized subtitle cache unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(mediaDir, "tracks.json")) // #nosec G304 -- mediaDir is constrained by localSubtitleMediaDir.
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read materialized subtitle index: %w", err)
	}
	var index materializedSubtitleIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, true, fmt.Errorf("parse materialized subtitle index: %w", err)
	}
	return &index, true, nil
}

func materializedTracksFromIndex(root string, media model.Media, index *materializedSubtitleIndex, strict bool) ([]SubtitleTrack, error) {
	mediaDir, ok := localSubtitleMediaDir(root, media.ID)
	if !ok || index == nil {
		return []SubtitleTrack{}, nil
	}
	tracks := make([]SubtitleTrack, 0, len(index.Tracks))
	for _, item := range index.Tracks {
		filename := strings.TrimSpace(item.Filename)
		if !safeLocalSubtitleFilename(filename) {
			if strict {
				return nil, errors.New("materialized subtitle index contains an invalid filename")
			}
			continue
		}
		stat, err := os.Stat(filepath.Join(mediaDir, filename))
		if err != nil || stat.IsDir() || stat.Size() > maxMaterializedCloudSubtitleBytes {
			if strict {
				if err != nil {
					return nil, fmt.Errorf("stat materialized subtitle %q: %w", filename, err)
				}
				return nil, fmt.Errorf("materialized subtitle %q is invalid", filename)
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(filename))
		codec, ok := extToCodec[ext]
		if !ok {
			if strict {
				return nil, fmt.Errorf("materialized subtitle %q has unsupported format", filename)
			}
			continue
		}
		lang := strings.TrimSpace(item.Lang)
		label := strings.TrimSpace(item.Label)
		if lang == "" || label == "" {
			detectedLang, detectedLabel := detectLangLabel(strings.TrimSuffix(firstNonEmpty(item.Name, filename), ext), "")
			if lang == "" {
				lang = detectedLang
			}
			if label == "" {
				label = detectedLabel
			}
		}
		tracks = append(tracks, SubtitleTrack{
			Lang:   lang,
			Label:  label,
			Name:   firstNonEmpty(strings.TrimSpace(item.Name), filename),
			Path:   materializedSubtitleURI(media.ID, filename),
			Codec:  codec,
			Source: materializedSubtitleSource,
		})
	}
	return tracks, nil
}

func materializedSubtitleURI(mediaID, filename string) string {
	return cachedSubtitleURI(materializedSubtitleScheme, mediaID, filename)
}

func parseMaterializedSubtitlePath(raw string) (mediaID, filename string, ok bool) {
	return parseSubtitleURI(raw, materializedSubtitleScheme)
}

func serveMaterializedCloudSubtitle(s *SubtitleService, mediaID, localMediaID, filename, format string, w io.Writer) error {
	if s == nil {
		return errors.New("subtitle service unavailable")
	}
	return serveCachedSubtitleFromRoot(s.materializedCacheDir, mediaID, localMediaID, filename, format, w)
}

func (s *SubtitleService) BackfillCloudSubtitles(ctx context.Context, libraryID string, progress func(CloudSubtitleBackfillResult)) (CloudSubtitleBackfillResult, error) {
	result := CloudSubtitleBackfillResult{LibraryID: strings.TrimSpace(libraryID), Errors: []string{}}
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return result, errors.New("subtitle service unavailable")
	}
	if strings.TrimSpace(s.materializedCacheDir) == "" {
		return result, errors.New("cloud subtitle cache directory is not configured")
	}
	if result.LibraryID != "" {
		library, err := s.repo.Library.FindByID(ctx, result.LibraryID)
		if err != nil {
			return result, err
		}
		if library == nil {
			return result, errors.New("library not found")
		}
	}
	baseQuery := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("path LIKE ?", "cloud://%")
	if result.LibraryID != "" {
		baseQuery = baseQuery.Where("library_id = ?", result.LibraryID)
	}
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return result, err
	}
	result.Total = int(total)
	if progress != nil {
		progress(result)
	}

	afterID := ""
	for {
		var rows []model.Media
		query := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
			Select("id").Where("path LIKE ?", "cloud://%")
		if result.LibraryID != "" {
			query = query.Where("library_id = ?", result.LibraryID)
		}
		if afterID != "" {
			query = query.Where("id > ?", afterID)
		}
		if err := query.Order("id ASC").Limit(materializedCloudSubtitleQueryBatch).Find(&rows).Error; err != nil {
			return result, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			item, err := s.RefreshCloudSubtitles(ctx, row.ID)
			result.Processed++
			if err != nil {
				result.Failed++
				if len(result.Errors) < 100 {
					result.Errors = append(result.Errors, row.ID+": "+err.Error())
				}
			} else if item.Status == "success" && item.Cached == 0 {
				result.Empty++
			} else if item.Status == "success" {
				result.Cached++
			} else {
				result.Skipped++
			}
			if progress != nil {
				progress(result)
			}
		}
		afterID = rows[len(rows)-1].ID
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("cloud subtitle backfill failed for %d of %d media", result.Failed, result.Processed)
	}
	return result, nil
}
