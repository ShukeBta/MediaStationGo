package service

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (e *EmbyService) mediaSourcesForItem(ctx context.Context, m *model.Media, asEmbedded, directOnly bool) []map[string]any {
	siblings := e.mediaVersionSiblings(ctx, m)
	if len(siblings) == 0 {
		return []map[string]any{e.mediaSource(ctx, m, asEmbedded, directOnly)}
	}
	sources := make([]map[string]any, 0, len(siblings))
	for i := range siblings {
		media := siblings[i]
		sources = append(sources, e.mediaSource(ctx, &media, asEmbedded, directOnly))
	}
	return sources
}

func (e *EmbyService) mediaVersionSiblings(ctx context.Context, m *model.Media) []model.Media {
	if e == nil || e.repo == nil || e.repo.DB == nil || m == nil || strings.TrimSpace(m.ID) == "" {
		return nil
	}
	libraryIDs := e.mergedLibraryIDs(ctx, m.LibraryID)
	if len(libraryIDs) == 0 {
		libraryIDs = []string{m.LibraryID}
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ?", libraryIDs).
		Where("season_num = ? AND episode_num = ?", m.SeasonNum, m.EpisodeNum)
	if m.TMDbID > 0 {
		q = q.Where("tm_db_id = ?", m.TMDbID)
	} else if m.BangumiID > 0 {
		q = q.Where("bangumi_id = ?", m.BangumiID)
	} else {
		title := strings.TrimSpace(m.Title)
		if title == "" {
			title = strings.TrimSpace(m.OriginalName)
		}
		if title == "" {
			return []model.Media{*m}
		}
		q = q.Where("LOWER(title) = ?", strings.ToLower(title))
		if m.Year > 0 {
			q = q.Where("year = ?", m.Year)
		}
	}
	var rows []model.Media
	if err := q.Find(&rows).Error; err != nil || len(rows) == 0 {
		return []model.Media{*m}
	}
	rows = e.collapseExactPathRows(rows)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ID == m.ID {
			return true
		}
		if rows[j].ID == m.ID {
			return false
		}
		return preferMediaVersion(rows[i], rows[j])
	})
	return rows
}

func (e *EmbyService) collapseExactPathRows(rows []model.Media) []model.Media {
	if len(rows) < 2 {
		return rows
	}
	out := rows[:0]
	seen := map[string]struct{}{}
	for _, row := range rows {
		path := strings.TrimSpace(row.Path)
		if path != "" {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
		}
		out = append(out, row)
	}
	return out
}

func (e *EmbyService) mediaVersionKey(ctx context.Context, m *model.Media) string {
	if e == nil || m == nil {
		return ""
	}
	ids := e.mergedLibraryIDs(ctx, m.LibraryID)
	sort.Strings(ids)
	libraryGroup := strings.Join(ids, ",")
	if libraryGroup == "" {
		libraryGroup = strings.TrimSpace(m.LibraryID)
	}
	if m.TMDbID > 0 {
		return fmt.Sprintf("%s|tmdb:%d|s:%d|e:%d", libraryGroup, m.TMDbID, m.SeasonNum, m.EpisodeNum)
	}
	if m.BangumiID > 0 {
		return fmt.Sprintf("%s|bangumi:%d|s:%d|e:%d", libraryGroup, m.BangumiID, m.SeasonNum, m.EpisodeNum)
	}
	title := strings.ToLower(strings.TrimSpace(m.Title))
	if title == "" {
		title = strings.ToLower(strings.TrimSpace(m.OriginalName))
	}
	if title == "" {
		return ""
	}
	return fmt.Sprintf("%s|title:%s|y:%d|s:%d|e:%d", libraryGroup, title, m.Year, m.SeasonNum, m.EpisodeNum)
}

func preferMediaVersion(candidate, current model.Media) bool {
	candidateCloud := strings.TrimSpace(candidate.STRMURL) != "" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(candidate.Path)), "cloud://")
	currentCloud := strings.TrimSpace(current.STRMURL) != "" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(current.Path)), "cloud://")
	if candidateCloud != currentCloud {
		return !candidateCloud
	}
	if candidate.Width != current.Width {
		return candidate.Width > current.Width
	}
	if candidate.SizeBytes != current.SizeBytes {
		return candidate.SizeBytes > current.SizeBytes
	}
	return candidate.CreatedAt.After(current.CreatedAt)
}

func embySTRMStreamURL(mediaID string) string {
	return "/api/stream/" + url.PathEscape(strings.TrimSpace(mediaID))
}

func embyDirectStreamURL(mediaID, container string) string {
	mediaID = strings.TrimSpace(mediaID)
	container = embyNormalizeStreamContainer(container)
	if container == "" || container == "strm" {
		return "/Videos/" + mediaID + "/stream"
	}
	return "/Videos/" + mediaID + "/stream." + container
}

func embyPlaybackContainer(raw, mediaPath string) string {
	pathExt := embyNormalizeStreamContainer(strings.TrimPrefix(strings.ToLower(filepath.Ext(mediaPath)), "."))
	if pathExt != "" && pathExt != "strm" {
		return pathExt
	}
	raw = strings.Trim(strings.ToLower(raw), ". ")
	if raw == "" {
		return pathExt
	}
	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '/'
	})
	if len(tokens) == 0 {
		return embyNormalizeStreamContainer(raw)
	}
	normalized := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if value := embyNormalizeStreamContainer(token); value != "" {
			normalized = append(normalized, value)
		}
	}
	for _, preferred := range []string{"mkv", "mp4", "mov", "webm", "avi", "ts", "m2ts", "wmv", "flv", "rmvb", "mpg"} {
		for _, value := range normalized {
			if value == preferred {
				return value
			}
		}
	}
	if len(normalized) > 0 {
		return normalized[0]
	}
	return ""
}

func embyNormalizeStreamContainer(container string) string {
	container = strings.Trim(strings.ToLower(container), ". ")
	switch container {
	case "", "unknown":
		return ""
	case "matroska":
		return "mkv"
	case "quicktime":
		return "mov"
	case "mpegts":
		return "ts"
	case "mpeg":
		return "mpg"
	case "asf":
		return "wmv"
	default:
		return container
	}
}

func (e *EmbyService) mediaStreams(m *model.Media) []map[string]any {
	streams := []map[string]any{}
	if m.VideoCodec != "" || m.Width > 0 {
		streams = append(streams, map[string]any{
			"Codec":        m.VideoCodec,
			"Type":         "Video",
			"Index":        0,
			"Width":        m.Width,
			"Height":       m.Height,
			"AspectRatio":  "",
			"IsDefault":    true,
			"IsForced":     false,
			"IsExternal":   false,
			"DisplayTitle": fmt.Sprintf("%dx%d %s", m.Width, m.Height, m.VideoCodec),
		})
	}
	if m.AudioCodec != "" {
		streams = append(streams, map[string]any{
			"Codec":      m.AudioCodec,
			"Type":       "Audio",
			"Index":      1,
			"IsDefault":  true,
			"IsForced":   false,
			"IsExternal": false,
		})
	}
	if len(streams) == 0 {
		streams = append(streams, map[string]any{
			"Codec":        "unknown",
			"Type":         "Video",
			"Index":        0,
			"IsDefault":    true,
			"IsForced":     false,
			"IsExternal":   false,
			"DisplayTitle": "Video",
		})
	}
	return streams
}
