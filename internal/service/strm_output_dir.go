package service

import (
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func strmLibraryOutputSubdir(lib model.Library) string {
	parts := strmLibraryCategoryParts(lib)
	if len(parts) == 0 {
		return sanitizeFilename(lib.Name)
	}
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if safe := sanitizeFilename(part); safe != "" {
			clean = append(clean, safe)
		}
	}
	if len(clean) == 0 {
		return sanitizeFilename(lib.Name)
	}
	return filepath.Join(clean...)
}

func strmLibrarySpecificOutputDir(base string, lib *model.Library) string {
	outputDir := filepath.Clean(strings.TrimSpace(base))
	if outputDir == "" || outputDir == "." || lib == nil {
		return outputDir
	}
	subdir := strmLibraryOutputSubdir(*lib)
	if subdir == "" || strmPathHasSuffix(outputDir, subdir) || pathAlreadyEndsWith(outputDir, filepath.Base(subdir)) {
		return outputDir
	}
	if tail := strmMissingCategoryTail(outputDir, subdir); tail != "" {
		return filepath.Join(outputDir, tail)
	}
	return filepath.Join(outputDir, subdir)
}

func strmMissingCategoryTail(outputDir, subdir string) string {
	parts := strmSlashParts(subdir)
	if len(parts) < 2 || !pathAlreadyEndsWith(outputDir, parts[0]) {
		return ""
	}
	return filepath.Join(parts[1:]...)
}

func strmPathHasSuffix(pathValue, suffix string) bool {
	pathValue = strings.Trim(filepath.ToSlash(filepath.Clean(strings.TrimSpace(pathValue))), "/")
	suffix = strings.Trim(filepath.ToSlash(filepath.Clean(strings.TrimSpace(suffix))), "/")
	if pathValue == "" || suffix == "" || suffix == "." {
		return false
	}
	if strings.EqualFold(pathValue, suffix) {
		return true
	}
	return strings.HasSuffix(strings.ToLower(pathValue), "/"+strings.ToLower(suffix))
}

func strmLibraryCategoryParts(lib model.Library) []string {
	if parts := strmCategoryPartsFromPath(strmLibraryPathParts(lib.Path)); len(parts) > 0 {
		return parts
	}
	if parts := strmCategoryPartsFromPath(strmNameParts(lib.Name)); len(parts) > 0 {
		return parts
	}
	if root := mediaTypeRootDir(lib.Type); root != "" {
		return []string{root}
	}
	return nil
}

func strmLibraryPathParts(raw string) []string {
	if info, ok := ParseCloudLibraryMount(raw); ok {
		return strmSlashParts(info.DisplayDir)
	}
	clean := cleanPathForVolumeMapping(raw)
	clean = strings.Trim(pathAfterWindowsDrivePrefix(clean), "/")
	return strmSlashParts(clean)
}

func strmNameParts(name string) []string {
	name = strings.NewReplacer("·", "/", ">", "/", "｜", "/", "|", "/", "\\", "/").Replace(name)
	return strmSlashParts(name)
}

func strmSlashParts(raw string) []string {
	raw = strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/")), "/")
	if raw == "" || raw == "." {
		return nil
	}
	fields := strings.Split(raw, "/")
	parts := make([]string, 0, len(fields))
	for _, part := range fields {
		part = strings.TrimSpace(part)
		if part != "" && part != "." {
			parts = append(parts, part)
		}
	}
	return parts
}

func strmCategoryPartsFromPath(parts []string) []string {
	for i, part := range parts {
		if root := strmCanonicalRoot(part); root != "" {
			return append([]string{root}, strmSanitizedTail(parts[i+1:])...)
		}
		if root := strmCategoryRoot(part); root != "" {
			return []string{root, strmCanonicalCategory(part)}
		}
	}
	return nil
}

func strmSanitizedTail(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return out
}

func strmCanonicalRoot(part string) string {
	key := strings.ToLower(strings.TrimSpace(part))
	switch key {
	case "电影", "movie", "movies", "film", "films":
		return "电影"
	case "电视剧", "剧集", "tv", "tvs", "series", "show", "shows":
		return "电视剧"
	case "动漫", "动画", "anime", "bangumi":
		return "动漫"
	case "成人", "adult", "adults", "jav", "nsfw", "9kg":
		return "成人"
	default:
		return ""
	}
}

func strmCategoryRoot(part string) string {
	key := strings.ToLower(strings.TrimSpace(part))
	switch key {
	case "演唱会", "音乐会", "纪录片", "纪录", "动画电影", "动漫电影", "华语电影", "国产电影", "外语电影", "外国电影", "欧美电影", "日韩电影", "日本电影", "韩国电影":
		return "电影"
	case "国产剧", "欧美剧", "日韩剧", "日剧", "韩剧", "综艺", "真人秀", "儿童", "少儿", "未分类":
		return "电视剧"
	case "国漫", "国产动漫", "日番", "番剧", "日漫", "日本动漫", "日本动画", "韩漫", "韩国动漫", "韩国动画", "美漫", "欧美动漫", "欧美动画", "西方动画", "其他", "其他动漫", "其它动漫":
		return "动漫"
	case "番号":
		return "成人"
	default:
		return ""
	}
}

func strmCanonicalCategory(part string) string {
	key := strings.ToLower(strings.TrimSpace(part))
	switch key {
	case "音乐会":
		return "演唱会"
	case "纪录":
		return "纪录片"
	case "动漫电影":
		return "动画电影"
	case "国产电影":
		return "华语电影"
	case "外语电影", "外国电影":
		return "欧美电影"
	case "日本电影", "韩国电影":
		return "日韩电影"
	case "日剧", "韩剧":
		return "日韩剧"
	case "真人秀":
		return "综艺"
	case "少儿":
		return "儿童"
	case "国产动漫":
		return "国漫"
	case "番剧", "日漫", "日本动漫", "日本动画":
		return "日番"
	case "韩国动漫", "韩国动画":
		return "韩漫"
	case "欧美动漫", "欧美动画", "西方动画":
		return "美漫"
	case "其他动漫", "其它动漫":
		return "其他"
	case "番号":
		return "成人"
	default:
		return strings.TrimSpace(part)
	}
}
