package service

import (
	"net/url"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func CloudMountProviderLabel(provider string) string {
	switch strings.TrimSpace(provider) {
	case LegacyQuarkProvider:
		return "已停用网盘"
	case cloud.Type115:
		return "115 网盘"
	case cloud.TypeCloudDrive2:
		return "CloudDrive2"
	case cloud.TypeOpenList:
		return "OpenList"
	default:
		if strings.TrimSpace(provider) == "" {
			return "网盘"
		}
		return strings.TrimSpace(provider)
	}
}

func stripCloudProviderDisplayPrefix(name, provider string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, label := range []string{CloudMountProviderLabel(provider), strings.TrimSpace(provider)} {
		label = strings.TrimSpace(label)
		if label == "" || len(name) < len(label) || !strings.EqualFold(name[:len(label)], label) {
			continue
		}
		rest := strings.TrimSpace(name[len(label):])
		rest = strings.TrimLeft(rest, " \t\r\n·・-—–|｜:/\\")
		if rest != "" {
			return strings.TrimSpace(rest)
		}
		if strings.EqualFold(name, label) {
			return ""
		}
	}
	return name
}

func cloudMountDirBase(dir string) string {
	dir = strings.Trim(strings.TrimSpace(strings.ReplaceAll(dir, "\\", "/")), "/")
	if dir == "" {
		return ""
	}
	parts := strings.Split(dir, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if part := strings.TrimSpace(parts[i]); part != "" {
			return part
		}
	}
	return ""
}

func normalizeLibraryMergeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	return strings.Join(strings.Fields(name), " ")
}

func ShadowedCloudLibraryIDSet(libs []model.Library) map[string]bool {
	out := make(map[string]bool)
	for _, lib := range libs {
		if CloudLibraryShadowed(libs, lib) != nil {
			out[lib.ID] = true
		}
	}
	return out
}

func InferCloudMountMediaType(dir, name string) string {
	text := strings.ToLower(dir + " " + name)
	switch {
	case strings.Contains(text, "成人") || strings.Contains(text, "adult") || strings.Contains(text, "jav") || strings.Contains(text, "9kg"):
		return "adult"
	case containsAny(text, "动画电影", "华语电影", "外语电影", "外国电影", "欧美电影", "日韩电影", "韩国电影", "日本电影", "港台电影", "香港电影", "台湾电影", "大陆电影", "国产电影", "纪录片", "演唱会", "音乐会", "电影", "movie", "movies", "film", "films", "documentary", "concert"):
		return "movie"
	case containsAny(text, "综艺", "真人秀", "脱口秀", "晚会", "variety"):
		return "variety"
	case containsAny(text, "国漫", "日漫", "日番", "韩漫", "美漫", "番剧", "动漫", "欧美动漫", "动画剧集", "anime"):
		return "anime"
	case containsAny(text, "国产剧", "大陆剧", "华语剧", "欧美剧", "日韩剧", "韩剧", "日剧", "港剧", "台剧", "泰剧", "英剧", "美剧", "短剧", "电视剧", "剧集", "连续剧", "series", "tv", "shows"):
		return "tv"
	default:
		return "movie"
	}
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}

func cloudMountAncestor(parent, child string) bool {
	parent = strings.Trim(parent, "/")
	child = strings.Trim(child, "/")
	if parent == child {
		return false
	}
	if parent == "" {
		return child != ""
	}
	return strings.HasPrefix(child, parent+"/")
}

func normalizeCloudMountDir(provider, value string) string {
	value = strings.TrimSpace(value)
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	if decoded, err := url.QueryUnescape(value); err == nil {
		value = decoded
	}
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "." || ((provider == cloud.Type115 || provider == LegacyQuarkProvider) && value == "0") {
		return ""
	}
	return value
}
