package service

import (
	"context"
	"fmt"
	"net/url"
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
	container = strings.Trim(strings.ToLower(container), ". ")
	if container == "" || container == "strm" {
		return "/Videos/" + mediaID + "/stream"
	}
	return "/Videos/" + mediaID + "/stream." + container
}

func (e *EmbyService) mediaStreams(ctx context.Context, m *model.Media) []map[string]any {
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
	streams = e.appendSubtitleStreams(ctx, streams, m)
	return streams
}

// appendSubtitleStreams discovers sideloaded external subtitle tracks next to
// the video and advertises them as Emby Subtitle MediaStreams so third-party
// clients can request them via /Videos/:id/Subtitles/:index/Stream. When no
// service is wired, discovery fails, or there are no tracks, the existing
// stream list is returned unchanged.
//
// The streams follow the official Emby external-subtitle contract:
//   - Codec reports the source codec (ass/ssa/subrip/vtt), matching the RAW
//     bytes served at the DeliveryUrl (see ServeSubtitleStream / ServeRaw).
//   - Index is stable and global across the MediaSource (Video 0, Audio 1,
//     subtitles 2, 3, ...). It never shifts to 1 when audio is absent.
//   - IsDefault is false (external subtitles are never auto-selected).
//   - Path and DeliveryMethod identify the sidecar file.
func (e *EmbyService) appendSubtitleStreams(ctx context.Context, streams []map[string]any, m *model.Media) []map[string]any {
	if e == nil || e.subtitle == nil || m == nil {
		return streams
	}
	tracks, err := e.subtitle.Discover(ctx, m.ID)
	if err != nil || len(tracks) == 0 {
		return streams
	}
	// Subtitle indexes continue after Video (0) and Audio (1). When Audio is
	// absent (e.g. STRM media), the fallback "unknown" Video stream fills slot 0
	// and subtitles still start at 1; when Audio is present they start at 2.
	next := 1
	if m.AudioCodec != "" {
		next = 2
	}
	mediaID := strings.TrimSpace(m.ID)
	for _, t := range tracks {
		index := next
		next++
		codec := subtitleCodecFromExt(t.Codec)
		streams = append(streams, map[string]any{
			"Codec":          codec,
			"DeliveryFormat": codec,
			"Container":      codec,
			"Type":           "Subtitle",
			"Index":          index,
			"IsExternal":     true,
			"IsForced":       false,
			"IsDefault":      false,
			"Language":       t.Lang,
			"DisplayTitle":   subtitleDisplayTitle(t),
			"Path":           strings.TrimSpace(t.Path),
			"DeliveryMethod": "External",
			// Official Emby / Swagger shape:
			//   /Videos/{Id}/{MediaSourceId}/Subtitles/{Index}/Stream.{Format}
			// MediaSourceId is a bare path parameter whose value is the
			// MediaSource.Id (== m.ID here). No "mediasource_" prefix: that
			// prefix only appears in real servers because their MediaSource.Id
			// value itself starts with it, not because the route template says so.
			"IsTextSubtitleStream":     true, // external sidecar files are always text subtitles
			"SupportsExternalStream":   true,
			"DeliveryUrl":              "/Videos/" + mediaID + "/" + mediaID + "/Subtitles/" + fmt.Sprint(index) + "/Stream." + codec,
		})
	}
	return streams
}

// subtitleCodecFromExt maps a subtitle source codec identifier (which is the
// track's extension-derived codec, e.g. "srt"/"ass"/"ssa"/"vtt") to the codec
// name Emby clients expect in MediaStreams. SRT is advertised as "subrip" (the
// official Emby/Jellyfin naming) while ASS/SSA/VTT keep their names.
func subtitleCodecFromExt(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "srt":
		return "subrip"
	default:
		return strings.ToLower(strings.TrimSpace(codec))
	}
}

// SubtitleContentType returns a Content-Type for a subtitle codec name.
func SubtitleContentType(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "srt", "subrip":
		return "application/x-subrip; charset=utf-8"
	case "vtt":
		return "text/vtt; charset=utf-8"
	default: // ass, ssa
		return "text/plain; charset=utf-8"
	}
}

// SubtitleCodecFromFormat maps a subtitle format suffix from a delivery URL
// (e.g. "ass", "srt", "subrip", "vtt", "ssa") to the codec name used for the
// Content-Type header. Unknown formats are returned lowercase unchanged.
func SubtitleCodecFromFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "srt", "subrip":
		return "subrip"
	case "vtt":
		return "vtt"
	case "ass":
		return "ass"
	case "ssa":
		return "ssa"
	default:
		return strings.ToLower(strings.TrimSpace(format))
	}
}

func subtitleDisplayTitle(t SubtitleTrack) string {
	// Prefer an explicit language label (e.g. "中文", "en"), otherwise fall back
	// to a friendly "字幕 (ASS)"-style label matching official Emby instead of a
	// bare "und" or a raw filename.
	if label := strings.TrimSpace(t.Label); label != "" && !strings.EqualFold(label, "und") {
		return label
	}
	codec := subtitleCodecFromExt(t.Codec)
	codecDisplay := strings.ToUpper(strings.TrimSpace(codec))
	if codecDisplay == "" {
		codecDisplay = "Subtitle"
	}
	return "字幕 (" + codecDisplay + ")"
}
