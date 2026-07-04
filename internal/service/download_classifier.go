package service

import (
	"context"
	"os"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func downloadDefaultSaveRoot(ctx context.Context, repo *repository.Container) string {
	if repo != nil && repo.Setting != nil {
		if base, _ := repo.Setting.Get(ctx, "qbittorrent.savepath"); strings.TrimSpace(base) != "" {
			return strings.TrimSpace(base)
		}
	}
	for _, key := range []string{"MEDIASTATION_DOWNLOAD_CONTAINER_DIR", "MEDIASTATION_DOWNLOAD_DIR"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func downloadSmartClassifyEnabled(ctx context.Context, repo *repository.Container, organizer *OrganizerService) bool {
	if repo != nil && repo.Setting != nil {
		val, err := repo.Setting.Get(ctx, DownloadSmartClassifySettingKey)
		if err == nil && val != "" {
			return parseBoolSetting(val, true)
		}
		val, err = repo.Setting.Get(ctx, "organizer.smart_classify")
		if err == nil && parseBoolSetting(val, false) {
			return true
		}
	}
	if organizer != nil && organizer.cfg != nil && organizer.cfg.Organizer.SmartClassify {
		return true
	}
	return true
}

func downloadCategoryMap(organizer *OrganizerService) map[string]string {
	if organizer == nil {
		return nil
	}
	return organizer.categoryMap()
}

func downloadSavePathCategoryRoot(root, category string) string {
	root = strings.TrimSpace(root)
	category = strings.TrimSpace(category)
	if root == "" || category == "" {
		return root
	}
	if isWindowsStyleClientPath(root) {
		cleanRoot := strings.ReplaceAll(root, "/", `\`)
		cleanRoot = strings.TrimRight(cleanRoot, `\`)
		if windowsPathBaseEqual(cleanRoot, category) {
			return cleanRoot
		}
		return cleanRoot + `\` + category
	}
	return categoryRoot(root, category)
}

func isWindowsStyleClientPath(path string) bool {
	path = strings.TrimSpace(path)
	return (len(path) >= 2 && isASCIIAlpha(path[0]) && path[1] == ':') ||
		strings.HasPrefix(path, `\\`)
}

func windowsPathBaseEqual(path, base string) bool {
	path = strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(path), "/", `\`), `\`)
	base = strings.Trim(strings.TrimSpace(base), `\/`)
	if path == "" || base == "" {
		return false
	}
	idx := strings.LastIndex(path, `\`)
	if idx >= 0 {
		path = path[idx+1:]
	}
	return strings.EqualFold(path, base)
}
