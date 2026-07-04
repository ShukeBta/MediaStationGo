package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func parseCloudLibraryPath(raw string) (typ, dirID string, ok bool) {
	info, ok := ParseCloudLibraryMount(raw)
	if !ok {
		return "", "", false
	}
	return info.Provider, info.ScanDir, true
}

func cloudEntryRef(typ, id, pickCode string) string {
	if typ == "cloud115" && strings.TrimSpace(pickCode) != "" {
		return strings.TrimSpace(pickCode)
	}
	return strings.TrimSpace(id)
}

func cloudMediaPath(typ, ref string) string {
	return "cloud://" + strings.TrimSpace(typ) + "/" + strings.TrimLeft(strings.TrimSpace(ref), "/")
}

func cloudMediaDedupeKey(lib *model.Library, dirID, name string, size int64) string {
	base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	if base == "" {
		return ""
	}
	season, episode := ParseEpisode(name)
	title, year := CleanQuery(name)
	title = normalizeCloudDedupeText(title)
	if (season > 0 || episode > 0) && title != "" {
		return fmt.Sprintf("episode:%s:%s:%d:%d:%d", strings.ToLower(strings.TrimSpace(lib.Type)), title, year, season, episode)
	}
	if (season > 0 || episode > 0) && title == "" {
		return fmt.Sprintf("episode-dir:%s:%s:%d:%d:%d", strings.ToLower(strings.TrimSpace(lib.Type)), normalizeCloudDedupeText(dirID), season, episode, size)
	}
	return fmt.Sprintf("file:%s:%d", normalizeCloudDedupeText(base), size)
}

func normalizeCloudDedupeText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case '.', '_', '-', ' ', '\t', '/', '\\', '[', ']', '(', ')':
			return true
		default:
			return false
		}
	})
	return strings.Join(fields, " ")
}
