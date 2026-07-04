package service

import (
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func cloudLibraryDisplayKey(lib model.Library) (string, bool) {
	info, ok := ParseCloudLibraryMount(lib.Path)
	if !ok {
		return "", false
	}
	dir := firstNonEmpty(info.DisplayDir, info.ScanDir)
	return info.Provider + "\x00" + dir, true
}

func cloudLibraryPathIsCanonical(lib model.Library) bool {
	info, ok := ParseCloudLibraryMount(lib.Path)
	if !ok {
		return false
	}
	return BuildCloudLibraryPath(info.Provider, info.ScanDir, info.DisplayDir) == strings.TrimSpace(lib.Path)
}

func CloudLibraryMergeKey(lib model.Library) (string, bool) {
	name := strings.TrimSpace(lib.Name)
	if displayName, ok := CloudLibraryDisplayName(lib); ok {
		name = displayName
	}
	name = normalizeLibraryMergeName(name)
	if name == "" {
		return "", false
	}
	typeKey := cloudLibraryMergeTypeKey(lib.Type)
	return typeKey + "\x00" + cloudLibraryMergeNameKey(typeKey, name), true
}

func cloudLibraryMergeTypeKey(libraryType string) string {
	switch strings.ToLower(strings.TrimSpace(libraryType)) {
	case "tv", "anime", "variety":
		return "tvshows"
	default:
		return strings.ToLower(strings.TrimSpace(libraryType))
	}
}

func cloudLibraryMergeNameKey(typeKey, name string) string {
	switch typeKey {
	case "movie":
		switch name {
		case "国产电影", "大陆电影":
			return "华语电影"
		case "华语电影":
			return "华语电影"
		case "外语电影", "外国电影", "欧美电影":
			return "欧美电影"
		case "日韩电影", "日本电影", "韩国电影":
			return "日韩电影"
		case "纪录", "纪录片":
			return "纪录片"
		case "演唱会", "concert":
			return "演唱会"
		case "动画电影", "动漫电影":
			return "动画电影"
		}
	case "tvshows":
		switch name {
		case "国产剧", "大陆剧", "华语剧", "国剧", "国产电视剧", "大陆电视剧", "华语电视剧", "港剧", "台剧", "港台剧":
			return "国产剧"
		case "欧美剧", "欧美电视剧", "美剧", "英剧":
			return "欧美剧"
		case "日韩剧", "日韩电视剧", "日剧", "韩剧", "泰剧":
			return "日韩剧"
		case "国漫", "国产动漫", "国产动画":
			return "国漫"
		case "日番", "日漫", "番剧", "日本动漫", "日本动画":
			return "日番"
		case "韩漫", "韩国动漫", "韩国动画":
			return "韩漫"
		case "美漫", "欧美动漫", "欧美动画", "西方动画":
			return "美漫"
		case "其他", "其他动漫", "其它动漫", "other":
			return "其他"
		case "纪录", "纪录片":
			return "纪录片"
		}
	}
	return name
}
