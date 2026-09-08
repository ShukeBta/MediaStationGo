package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type MediaItem struct {
	model.Media
	Versions []model.Media `json:"versions,omitempty"`
	Parts    []model.Media `json:"parts,omitempty"`
}

func normalizeGroupedMediaPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > maxMediaSearchPageSize {
		pageSize = maxMediaSearchPageSize
	}
	return page, pageSize
}

func paginateMediaItems(items []MediaItem, page, pageSize int) []MediaItem {
	page, pageSize = normalizeGroupedMediaPage(page, pageSize)
	if len(items) == 0 {
		// 返回非 nil 空切片：nil 会被 JSON 序列化成 "items": null，
		// 前端 concat(null) 会得到 [null] 并在渲染期崩溃（空库进入白屏）。
		return []MediaItem{}
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []MediaItem{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func firstMediaItems(items []MediaItem, limit int) []MediaItem {
	if len(items) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > maxMediaSearchLimit {
		limit = maxMediaSearchLimit
	}
	if limit > len(items) {
		limit = len(items)
	}
	return items[:limit]
}

func groupMediaVersions(items []model.Media) []MediaItem {
	if len(items) == 0 {
		return nil
	}
	type group struct {
		key     string
		kind    string
		primary model.Media
		rows    []model.Media
	}
	groups := make([]group, 0, len(items))
	byKey := make(map[string]int, len(items))
	for _, item := range items {
		kind := "version"
		key := mediaVersionGroupKey(item)
		if partKey := mediaPartGroupKey(item); partKey != "" {
			kind = "part"
			key = partKey
		}
		if key == "" {
			groups = append(groups, group{primary: item, rows: []model.Media{item}})
			continue
		}
		if idx, ok := byKey[key]; ok {
			groups[idx].rows = append(groups[idx].rows, item)
			if (kind == "part" && betterMediaPart(item, groups[idx].primary)) ||
				(kind != "part" && betterMediaVersion(item, groups[idx].primary)) {
				groups[idx].primary = item
			}
			continue
		}
		byKey[key] = len(groups)
		groups = append(groups, group{key: key, kind: kind, primary: item, rows: []model.Media{item}})
	}
	out := make([]MediaItem, 0, len(groups))
	for _, g := range groups {
		if g.kind == "part" {
			sort.SliceStable(g.rows, func(i, j int) bool { return betterMediaPart(g.rows[i], g.rows[j]) })
		} else {
			sort.SliceStable(g.rows, func(i, j int) bool { return betterMediaVersion(g.rows[i], g.rows[j]) })
		}
		item := MediaItem{Media: g.primary}
		if g.kind == "part" {
			item.Media.Title = firstNonEmpty(g.primary.PartGroupTitle, g.primary.Title)
			item.Media.DisplayTitle = item.Media.Title
			if len(g.rows) > 1 {
				item.Parts = g.rows
			}
		} else if len(g.rows) > 1 {
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
	if m.EpisodeEndNum > m.EpisodeNum || m.EpisodePartNum > 0 {
		end, part := m.EpisodeEndNum, m.EpisodePartNum
		m.EpisodeEndNum, m.EpisodePartNum = 0, 0
		return fmt.Sprintf("%s:end:%d:segment:%d", mediaVersionGroupKey(m), end, part)
	}
	if strings.TrimSpace(m.PartGroupKey) != "" {
		return "part-source:" + strings.TrimSpace(m.ID)
	}
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
		title, _ = mediaVersionTitleKey(title)
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
	if m.TitleCleanupVersion >= mediaTitleExplicitGroupingVersion {
		if key := strings.TrimSpace(m.VersionGroupKey); key != "" {
			return "cleanup-version:" + strings.ToLower(strings.TrimSpace(m.LibraryID)) + ":" + strings.ToLower(key)
		}
		return ""
	}
	title := firstNonEmpty(m.OriginalName, m.Title)
	titleYear := 0
	if title == "" {
		title, _ = CleanQuery(m.Path)
	} else {
		title, titleYear = mediaVersionTitleKey(title)
	}
	if title == "" {
		return ""
	}
	year := m.Year
	if year <= 0 {
		year = titleYear
	}
	if year <= 0 {
		_, year = CleanQuery(m.Path)
	}
	return fmt.Sprintf("movie:%s:%d", title, year)
}

// mediaVersionPersistedKey is the compact, indexed form of the exact key
// used by groupMediaVersions. Hashing keeps fallback title-derived keys
// bounded without changing their equality semantics.
func mediaVersionPersistedKey(m model.Media) string {
	effective := mediaVersionGroupKey(m)
	if part := mediaPartGroupKey(m); part != "" {
		effective = part
	}
	if effective == "" {
		// groupMediaVersions deliberately keeps an unkeyed row as a singleton;
		// persist a row-specific key so SQL pagination can represent that same
		// singleton without treating the projection as incomplete.
		effective = "row:" + strings.TrimSpace(m.ID)
		if effective == "row:" {
			return ""
		}
	}
	sum := sha256.Sum256([]byte(effective))
	return hex.EncodeToString(sum[:])
}

func mediaPartGroupKey(m model.Media) string {
	key := strings.TrimSpace(m.PartGroupKey)
	if key == "" {
		return ""
	}
	return "part:" + strings.ToLower(strings.TrimSpace(m.LibraryID)) + ":" + strings.ToLower(key)
}

func betterMediaPart(candidate, current model.Media) bool {
	if candidate.PartIndex > 0 && current.PartIndex > 0 && candidate.PartIndex != current.PartIndex {
		return candidate.PartIndex < current.PartIndex
	}
	if candidate.PartIndex > 0 && current.PartIndex <= 0 {
		return true
	}
	if candidate.PartIndex <= 0 && current.PartIndex > 0 {
		return false
	}
	return betterMediaVersion(candidate, current)
}

func mediaVersionTitleKey(value string) (string, int) {
	value = strings.TrimSpace(value)
	cleanInput := value
	if _, isVideoFilename := videoExtensions[strings.ToLower(filepath.Ext(value))]; !isVideoFilename {
		// CleanQuery accepts file paths and therefore strips everything after the
		// final dot as an extension. Media titles such as "mtcang.com 跳蛋" are
		// already parsed titles, so protect their domain-like suffix with a known
		// synthetic video extension before applying the shared release cleanup.
		cleanInput += ".mkv"
	}
	cleaned, year := CleanQuery(cleanInput)
	if strings.TrimSpace(cleaned) == "" {
		cleaned = value
	}
	return normalizeMediaVersionText(cleaned), year
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
