package service

import "strings"

func mediaFolderTitle(mediaPath, libraryRoot string) string {
	dir := parentSlashPath(mediaPath)
	root := comparableLibraryRoot(libraryRoot)
	for depth := 0; depth < 5 && dir != ""; depth++ {
		if root != "" && sameSlashPath(dir, root) {
			return libraryRootTitle(libraryRoot)
		}
		base := pathBaseSlash(dir)
		if base == "" || base == "." {
			return ""
		}
		if isTechnicalMediaFolder(base) || strictSeasonFolderMatched(base) {
			dir = parentSlashPath(dir)
			continue
		}
		if isMediaCollectionFolder(base) {
			dir = parentSlashPath(dir)
			continue
		}
		if isGenericMediaCategoryFolder(base) {
			return ""
		}
		title, _ := CleanQuery(base)
		if title == "" {
			title = strings.TrimSpace(base)
		}
		return strings.TrimSpace(title)
	}
	return ""
}

func isTechnicalMediaFolder(name string) bool {
	key := strings.ToLower(strings.TrimSpace(name))
	compact := strings.NewReplacer(" ", "", "_", "", ".", "", "-", "").Replace(key)
	switch compact {
	case "bdmv", "stream", "certificate", "videots", "audiots",
		"subs", "subtitles", "subtitle", "sample", "samples",
		"extra", "extras", "featurette", "featurettes":
		return true
	default:
		return numberedTechnicalFolder(compact, "disc") ||
			numberedTechnicalFolder(compact, "disk") ||
			numberedTechnicalFolder(compact, "cd") ||
			numberedTechnicalFolder(compact, "dvd") ||
			numberedTechnicalFolder(compact, "part")
	}
}

func numberedTechnicalFolder(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) == len(prefix) {
		return false
	}
	for _, r := range value[len(prefix):] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func cleanSlashPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	return strings.TrimRight(value, "/")
}

func comparableLibraryRoot(libraryRoot string) string {
	if info, ok := ParseCloudLibraryMount(libraryRoot); ok {
		if strings.TrimSpace(info.DisplayDir) == "" {
			return "cloud://" + info.Provider
		}
		return "cloud://" + info.Provider + "/" + info.DisplayDir
	}
	return cleanSlashPath(libraryRoot)
}

func sameSlashPath(a, b string) bool {
	return strings.EqualFold(cleanSlashPath(a), cleanSlashPath(b))
}

func parentSlashPath(value string) string {
	value = cleanSlashPath(value)
	if value == "" {
		return ""
	}
	idx := strings.LastIndex(value, "/")
	if idx < 0 {
		return ""
	}
	return strings.TrimRight(value[:idx], "/")
}

func seriesFolderTitle(mediaPath, libraryRoot string) string {
	dir := parentSlashPath(mediaPath)
	if strictSeasonFolderMatched(pathBaseSlash(dir)) {
		dir = parentSlashPath(dir)
	}
	if root := comparableLibraryRoot(libraryRoot); root != "" && sameSlashPath(dir, root) {
		return libraryRootTitle(libraryRoot)
	}
	base := pathBaseSlash(dir)
	if base == "" || base == "." {
		return ""
	}
	if isGenericMediaCategoryFolder(base) || isTechnicalMediaFolder(base) || strictSeasonFolderMatched(base) {
		return ""
	}
	return base
}

func libraryRootTitle(libraryRoot string) string {
	base := ""
	if info, ok := ParseCloudLibraryMount(libraryRoot); ok {
		base = pathBaseSlash(info.DisplayDir)
	} else {
		base = pathBaseSlash(libraryRoot)
	}
	if base == "" || base == "." || isGenericMediaCategoryFolder(base) || isTechnicalMediaFolder(base) || strictSeasonFolderMatched(base) || isMediaCollectionFolder(base) {
		return ""
	}
	return base
}

func isGenericMediaCategoryFolder(name string) bool {
	key := strings.ToLower(strings.TrimSpace(name))
	key = strings.Trim(key, `\/`)
	switch key {
	case "",
		"电影", "movies", "movie",
		"电视剧", "剧集", "tv", "shows", "series",
		"动漫", "动画", "anime", "bangumi",
		"国产剧", "国剧", "大陆剧", "国产电视剧",
		"欧美剧", "欧美电视剧",
		"日韩剧", "日剧", "韩剧",
		"华语电影", "国产电影", "大陆电影",
		"外语电影", "外国电影", "欧美电影", "日韩电影", "演唱会", "音乐会",
		"动画电影", "动漫电影",
		"国漫", "国产动漫", "日番", "日漫", "日本动漫", "日本动画", "韩漫", "韩国动漫", "韩国动画", "美漫", "欧美动漫", "欧美动画", "西方动画", "其他动漫", "其它动漫",
		"综艺", "真人秀",
		"纪录片", "纪录",
		"儿童", "少儿",
		"成人", "番号", "9kg",
		"未分类", "uncategorized":
		return true
	default:
		return false
	}
}

func strictSeasonFolder(name string) int {
	if season, ok := seasonFromDir(name); ok {
		return season
	}
	return 0
}

func strictSeasonFolderMatched(name string) bool {
	_, ok := seasonFromDir(name)
	return ok
}
