package service

import (
	"context"
	"fmt"
	"net/url"
	pathpkg "path"
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
	if strings.TrimSpace(m.PartGroupKey) != "" {
		return []model.Media{*m}
	}
	libraryIDs := e.mergedLibraryIDs(ctx, m.LibraryID)
	if len(libraryIDs) == 0 {
		libraryIDs = []string{m.LibraryID}
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ?", libraryIDs).
		Where("season_num = ? AND episode_num = ?", m.SeasonNum, m.EpisodeNum)
	if strings.TrimSpace(m.VersionGroupKey) != "" {
		q = q.Where("version_group_key = ?", m.VersionGroupKey)
	} else if m.TitleCleanupVersion >= mediaTitleExplicitGroupingVersion {
		return []model.Media{*m}
	} else if m.TMDbID > 0 {
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
	if strings.TrimSpace(m.PartGroupKey) != "" {
		return "part-item:" + strings.TrimSpace(m.ID)
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
	if m.TitleCleanupVersion >= mediaTitleExplicitGroupingVersion {
		if key := strings.TrimSpace(m.VersionGroupKey); key != "" {
			return fmt.Sprintf("%s|cleanup-version:%s", libraryGroup, strings.ToLower(key))
		}
		return ""
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

func (e *EmbyService) attachExternalSubtitleStreams(ctx context.Context, m *model.Media, src map[string]any) {
	if e == nil || e.subtitle == nil || m == nil || src == nil || strings.TrimSpace(m.ID) == "" {
		return
	}
	tracks, err := e.subtitle.Discover(ctx, m.ID)
	if err != nil || len(tracks) == 0 {
		return
	}
	streams, ok := src["MediaStreams"].([]map[string]any)
	if !ok {
		return
	}
	nextIndex := nextMediaStreamIndex(streams)
	firstSubtitleIndex := -1
	for trackIndex, track := range tracks {
		streamIndex := nextIndex + trackIndex
		if firstSubtitleIndex < 0 {
			firstSubtitleIndex = streamIndex
		}
		streams = append(streams, embyExternalSubtitleStream(m.ID, streamIndex, trackIndex, track))
	}
	src["MediaStreams"] = streams
	if firstSubtitleIndex >= 0 {
		src["DefaultSubtitleStreamIndex"] = firstSubtitleIndex
	}
}

// attachExternalSubtitleStreamsToItem mirrors Emby item-detail responses:
// clients inspect MediaSources there before requesting PlaybackInfo. Library
// listings deliberately keep the lightweight embedded payload to avoid a
// cloud subtitle discovery request for every listed media item.
func (e *EmbyService) attachExternalSubtitleStreamsToItem(ctx context.Context, item map[string]any, primary *model.Media) error {
	if e == nil || e.repo == nil || e.repo.Media == nil || item == nil || primary == nil {
		return nil
	}
	sources, ok := item["MediaSources"].([]map[string]any)
	if !ok {
		return nil
	}
	for _, source := range sources {
		media := primary
		sourceID, _ := source["Id"].(string)
		sourceID = strings.TrimSpace(sourceID)
		if sourceID != "" && sourceID != primary.ID {
			row, err := e.repo.Media.FindByID(ctx, sourceID)
			if err != nil {
				return err
			}
			if row == nil {
				continue
			}
			media = row
		}
		e.attachExternalSubtitleStreams(ctx, media, source)
	}
	return nil
}

func nextMediaStreamIndex(streams []map[string]any) int {
	next := 0
	for _, stream := range streams {
		index, ok := stream["Index"].(int)
		if ok && index >= next {
			next = index + 1
		}
	}
	return next
}

func embyExternalSubtitleStream(mediaID string, streamIndex, trackIndex int, track SubtitleTrack) map[string]any {
	label := strings.TrimSpace(firstNonEmpty(track.Label, track.Lang, "Subtitle"))
	lang := embySubtitleLanguage(track.Lang)
	localizedDefault := ""
	ext := subtitleDeliveryExtension(track)
	codec := subtitleCodecForExtension(ext)
	if trackIndex == 0 {
		localizedDefault = "Default"
	}
	return map[string]any{
		"Index":                           streamIndex,
		"Type":                            "Subtitle",
		"Codec":                           codec,
		"Language":                        lang,
		"DisplayLanguage":                 embySubtitleDisplayLanguage(lang),
		"DisplayTitle":                    embySubtitleDisplayTitle(label, subtitleCodecDisplayLabel(codec)),
		"Title":                           label,
		"TimeBase":                        "1/1000",
		"IsExternal":                      true,
		"IsExternalUrl":                   false,
		"IsInterlaced":                    false,
		"IsForced":                        false,
		"IsDefault":                       trackIndex == 0,
		"IsHearingImpaired":               false,
		"IsTextSubtitleStream":            true,
		"SupportsExternalStream":          true,
		"DeliveryMethod":                  "External",
		"DeliveryUrl":                     embySubtitleDeliveryURL(mediaID, streamIndex, ext),
		"Path":                            embySubtitleVirtualPath(mediaID, trackIndex, ext, track),
		"Protocol":                        "File",
		"ExtendedVideoType":               "None",
		"ExtendedVideoSubType":            "None",
		"ExtendedVideoSubTypeDescription": "None",
		"AttachmentSize":                  0,
		"LocalizedDefault":                localizedDefault,
		"LocalizedForced":                 "",
		"LocalizedExternal":               "External",
	}
}

func embySubtitleLanguage(lang string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(lang), "_", "-"))
	switch normalized {
	case "sc", "chs", "zh-cn", "zh-hans", "zh-sg":
		return "zh-CN"
	case "tc", "cht", "zh-tw", "zh-hant", "zh-hk":
		return "zh-TW"
	case "jp", "jpn", "ja":
		return "ja"
	case "eng", "en":
		return "en"
	case "":
		return "und"
	default:
		return strings.TrimSpace(lang)
	}
}

func embySubtitleDisplayLanguage(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh-cn", "zh-hans":
		return "Chinese Simplified"
	case "zh-tw", "zh-hant", "zh-hk":
		return "Chinese Traditional"
	case "ja", "jpn":
		return "Japanese"
	case "en", "eng":
		return "English"
	case "und", "":
		return "Unknown"
	default:
		return strings.TrimSpace(lang)
	}
}

func embySubtitleVirtualPath(mediaID string, trackIndex int, ext string, track SubtitleTrack) string {
	filename := strings.TrimSpace(track.Name)
	if _, localFilename, ok := parseLocalSubtitlePath(track.Path); ok {
		filename = strings.TrimSpace(localFilename)
	}
	if filename == "" {
		if parsed, err := url.Parse(strings.TrimSpace(track.Path)); err == nil {
			filename = strings.TrimSpace(parsed.Query().Get("name"))
			if filename == "" && parsed.Scheme == "" {
				filename = pathpkg.Base(parsed.Path)
			}
		}
	}
	if filename == "" || filename == "." || filename == "/" {
		filename = fmt.Sprintf("subtitle-%d%s", trackIndex+1, ext)
	}
	return pathpkg.Join("/subtitles", strings.TrimSpace(mediaID), pathpkg.Base(filename))
}

func subtitleDeliveryExtension(track SubtitleTrack) string {
	if ext := subtitleExtensionFromCodec(track.Codec); ext != "" {
		return ext
	}
	if ext := subtitleExtensionFromPath(track.Path); ext != "" {
		return ext
	}
	return ".vtt"
}

func subtitleExtensionFromCodec(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "webvtt", "vtt":
		return ".vtt"
	case "srt", "ass", "ssa":
		return "." + strings.ToLower(strings.TrimSpace(codec))
	default:
		return ""
	}
}

func subtitleExtensionFromPath(path string) string {
	if parsed, err := url.Parse(strings.TrimSpace(path)); err == nil {
		if name := strings.TrimSpace(parsed.Query().Get("name")); name != "" {
			if ext, ok := normalizeSubtitleExtension(filepath.Ext(name)); ok {
				return ext
			}
		}
	}
	trimmed := strings.TrimSpace(path)
	if idx := strings.IndexAny(trimmed, "?#"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	if ext, ok := normalizeSubtitleExtension(filepath.Ext(trimmed)); ok {
		return ext
	}
	return ""
}

func subtitleCodecForExtension(ext string) string {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".vtt":
		return "webvtt"
	case ".srt":
		return "srt"
	case ".ass":
		return "ass"
	case ".ssa":
		return "ssa"
	default:
		return "webvtt"
	}
}

func subtitleCodecDisplayLabel(codec string) string {
	if strings.EqualFold(strings.TrimSpace(codec), "webvtt") {
		return "WEBVTT"
	}
	return strings.ToUpper(strings.TrimSpace(codec))
}

func embySubtitleDisplayTitle(label, codecLabel string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "Subtitle"
	}
	codecLabel = strings.TrimSpace(codecLabel)
	if codecLabel == "" {
		codecLabel = "WEBVTT"
	}
	upperLabel := strings.ToUpper(label)
	if strings.Contains(strings.ToLower(label), "external") || strings.Contains(upperLabel, codecLabel) {
		return label
	}
	return label + " - " + codecLabel + " - External"
}

func embySubtitleDeliveryURL(mediaID string, streamIndex int, ext string) string {
	if _, ok := SubtitleContentType(ext); !ok {
		ext = ".vtt"
	}
	mediaID = url.PathEscape(strings.TrimSpace(mediaID))
	return fmt.Sprintf("/emby/Videos/%s/%s/Subtitles/%d/Stream%s", mediaID, mediaID, streamIndex, ext)
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

func embyDirectStreamURL(mediaID, container string) string {
	mediaID = strings.TrimSpace(mediaID)
	container = strings.Trim(strings.ToLower(container), ". ")
	if container == "" || container == "strm" {
		return "/Videos/" + mediaID + "/stream"
	}
	return "/Videos/" + mediaID + "/stream." + container
}

func (e *EmbyService) mediaStreams(m *model.Media) []map[string]any {
	streams := []map[string]any{}
	if m.VideoCodec != "" || m.Width > 0 {
		displayTitle := strings.TrimSpace(fmt.Sprintf("%dx%d %s %s", m.Width, m.Height, m.VideoCodec, m.VideoRange))
		streams = append(streams, map[string]any{
			"Codec":            m.VideoCodec,
			"Type":             "Video",
			"Index":            0,
			"Width":            m.Width,
			"Height":           m.Height,
			"BitRate":          m.VideoBitRate,
			"RealFrameRate":    m.FrameRate,
			"AverageFrameRate": m.FrameRate,
			"Profile":          m.VideoProfile,
			"VideoRange":       m.VideoRange,
			"BitDepth":         m.VideoBitDepth,
			"AspectRatio":      "",
			"IsDefault":        true,
			"IsForced":         false,
			"IsExternal":       false,
			"DisplayTitle":     displayTitle,
		})
	}
	if m.AudioCodec != "" {
		streams = append(streams, map[string]any{
			"Codec":         m.AudioCodec,
			"Type":          "Audio",
			"Index":         1,
			"BitRate":       m.AudioBitRate,
			"Channels":      m.AudioChannels,
			"ChannelLayout": m.AudioChannelLayout,
			"SampleRate":    m.AudioSampleRate,
			"IsDefault":     true,
			"IsForced":      false,
			"IsExternal":    false,
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
