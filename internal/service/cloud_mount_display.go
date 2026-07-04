package service

import (
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func NormalizeCloudLibraryDisplayNames(libs []model.Library) []model.Library {
	out := make([]model.Library, 0, len(libs))
	for _, lib := range libs {
		if displayName, ok := CloudLibraryDisplayName(lib); ok && displayName != "" {
			lib.Name = displayName
		} else if displayName := CanonicalLibraryDisplayName(lib); displayName != "" {
			lib.Name = displayName
		}
		out = append(out, lib)
	}
	return out
}

func NormalizeCloudLibraryDisplay(libs []model.Library) []model.Library {
	return normalizeDisplayLibraries(libs)
}

func normalizeDisplayLibraries(libs []model.Library) []model.Library {
	out := make([]model.Library, 0, len(libs))
	for _, lib := range libs {
		if displayName, ok := CloudLibraryDisplayName(lib); ok && displayName != "" {
			lib.Name = displayName
		} else if displayName := CanonicalLibraryDisplayName(lib); displayName != "" {
			lib.Name = displayName
		}
		if displayType := CanonicalLibraryDisplayType(lib); displayType != "" {
			lib.Type = displayType
		}
		if displayPath := CanonicalLibraryDisplayPath(lib); displayPath != "" {
			lib.Path = displayPath
		}
		out = append(out, lib)
	}
	return out
}

func CloudLibraryDisplayName(lib model.Library) (string, bool) {
	info, ok := ParseCloudLibraryMount(lib.Path)
	if !ok {
		return "", false
	}
	name := stripCloudProviderDisplayPrefix(strings.TrimSpace(lib.Name), info.Provider)
	dir := firstNonEmpty(info.DisplayDir, info.ScanDir)
	if name == "" || strings.EqualFold(name, CloudMountProviderLabel(info.Provider)) {
		if base := cloudMountDirBase(dir); base != "" {
			name = base
		}
	}
	if name == "" {
		name = CloudMountProviderLabel(info.Provider)
	}
	if canonical := canonicalLibraryCategoryName(lib.Type, name); canonical != "" {
		name = canonical
	} else if canonical := canonicalLibraryCategoryNameAny(name); canonical != "" {
		name = canonical
	}
	return name, true
}

func CanonicalLibraryDisplayName(lib model.Library) string {
	if canonical := canonicalLibraryCategoryName(lib.Type, lib.Name); canonical != "" {
		return canonical
	}
	return canonicalLibraryCategoryNameAny(lib.Name)
}

func CanonicalLibraryDisplayType(lib model.Library) string {
	if displayName, ok := CloudLibraryDisplayName(lib); ok {
		if typ := canonicalLibraryCategoryDisplayType(displayName); typ != "" {
			return typ
		}
	}
	if typ := canonicalLibraryCategoryDisplayType(lib.Name); typ != "" {
		return typ
	}
	return canonicalLibraryCategoryDisplayType(pathBaseSlash(lib.Path))
}

func CanonicalLibraryDisplayPath(lib model.Library) string {
	raw := strings.TrimSpace(lib.Path)
	if raw == "" {
		return ""
	}
	if info, ok := ParseCloudLibraryMount(raw); ok {
		dir := firstNonEmpty(info.DisplayDir, info.ScanDir)
		displayDir := canonicalLibraryDisplayDir(dir)
		if displayDir == "" {
			return raw
		}
		if CloudLibraryAutoCategory(lib) {
			return BuildCloudAutoCategoryLibraryPathWithScanDir(info.Provider, info.ScanDir, displayDir)
		}
		return BuildCloudLibraryPath(info.Provider, info.ScanDir, displayDir)
	}
	return canonicalLocalLibraryDisplayPath(raw)
}

func canonicalLibraryCategoryName(libraryType, name string) string {
	typeKey := cloudLibraryMergeTypeKey(libraryType)
	name = normalizeLibraryMergeName(name)
	switch typeKey {
	case "movie":
		switch name {
		case "国产电影", "大陆电影":
			return "华语电影"
		case "外语电影", "外国电影":
			return "欧美电影"
		case "日本电影", "韩国电影":
			return "日韩电影"
		case "音乐会", "concert":
			return "演唱会"
		case "纪录":
			return "纪录片"
		case "动漫电影":
			return "动画电影"
		}
	case "tvshows":
		switch name {
		case "国剧", "大陆剧", "华语剧", "国产电视剧", "大陆电视剧", "华语电视剧", "港剧", "台剧", "港台剧":
			return "国产剧"
		case "欧美电视剧", "美剧", "英剧", "未分类", "uncategorized":
			return "欧美剧"
		case "日韩电视剧", "日剧", "韩剧", "泰剧":
			return "日韩剧"
		case "真人秀":
			return "综艺"
		case "纪录":
			return "纪录片"
		case "少儿":
			return "儿童"
		case "国产动漫", "国产动画":
			return "国漫"
		case "日漫", "番剧", "日本动漫", "日本动画":
			return "日番"
		case "韩国动漫", "韩国动画":
			return "韩漫"
		case "欧美动漫", "欧美动画", "西方动画":
			return "美漫"
		case "其他动漫", "其它动漫", "other":
			return "其他"
		}
	case "adult":
		switch name {
		case "9kg", "番号", "jav", "nsfw", "adult":
			return "成人"
		}
	}
	return ""
}

func canonicalLibraryCategoryNameAny(name string) string {
	for _, libraryType := range []string{"movie", "tv", "anime", "adult"} {
		if canonical := canonicalLibraryCategoryName(libraryType, name); canonical != "" {
			return canonical
		}
	}
	return ""
}

func canonicalLibraryDisplayDir(raw string) string {
	parts := strmSlashParts(raw)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(canonicalLibraryDisplayParts(parts), "/")
}

func canonicalLocalLibraryDisplayPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	sep := "/"
	if strings.Contains(value, "\\") {
		sep = "\\"
	}
	slash := strings.ReplaceAll(value, "\\", "/")
	prefix := ""
	for strings.HasPrefix(slash, "/") {
		prefix += "/"
		slash = strings.TrimPrefix(slash, "/")
	}
	parts := strings.Split(slash, "/")
	canonical := canonicalLibraryDisplayParts(parts)
	if len(canonical) == 0 {
		return raw
	}
	out := prefix + strings.Join(canonical, "/")
	if sep == "\\" {
		out = strings.ReplaceAll(out, "/", "\\")
	}
	return out
}

func canonicalLibraryDisplayParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		if canonical := canonicalLibraryCategoryNameAny(part); canonical != "" {
			part = canonical
		}
		if len(out) > 0 && normalizeLibraryMergeName(out[len(out)-1]) == normalizeLibraryMergeName(part) {
			continue
		}
		out = append(out, part)
	}
	return out
}

func canonicalLibraryCategoryDisplayType(name string) string {
	switch normalizeLibraryMergeName(name) {
	case "演唱会", "音乐会", "动画电影", "动漫电影", "华语电影", "国产电影", "大陆电影", "欧美电影", "外语电影", "外国电影", "日韩电影", "日本电影", "韩国电影":
		return "movie"
	case "国产剧", "国剧", "大陆剧", "华语剧", "国产电视剧", "大陆电视剧", "华语电视剧", "港剧", "台剧", "港台剧", "欧美剧", "欧美电视剧", "美剧", "英剧", "未分类", "uncategorized", "日韩剧", "日韩电视剧", "日剧", "韩剧", "泰剧", "综艺", "真人秀", "儿童", "少儿":
		return "tv"
	case "国漫", "国产动漫", "国产动画", "日番", "日漫", "番剧", "日本动漫", "日本动画", "韩漫", "韩国动漫", "韩国动画", "美漫", "欧美动漫", "欧美动画", "西方动画", "其他", "其他动漫", "其它动漫", "other":
		return "anime"
	case "成人", "9kg", "番号", "jav", "nsfw", "adult":
		return "adult"
	default:
		return ""
	}
}
