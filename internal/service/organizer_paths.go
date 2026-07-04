package service

import (
	"path/filepath"
	"strings"
)

// sanitizeFilename removes characters not safe for filesystem names.
func sanitizeFilename(s string) string {
	r := strings.NewReplacer(
		"/", " ", "\\", " ", ":", " ", "*", "", "?", "",
		"\"", "", "<", "", ">", "", "|", "",
	)
	return strings.TrimSpace(r.Replace(s))
}

func (o *OrganizerService) organizeRoot(libraryPath, mediaType, category string) string {
	typeDir := o.mediaTypeRootDirForCategory(mediaType, category)
	if typeDir == "" || pathAlreadyEndsWith(libraryPath, typeDir) {
		return libraryPath
	}
	if isGenericMediaRoot(libraryPath) {
		return filepath.Join(libraryPath, typeDir)
	}
	return libraryPath
}

func (o *OrganizerService) mediaTypeRootDirForCategory(mediaType, category string) string {
	if root := o.categoryPhysicalRootDirForType(mediaType, category); root != "" {
		return root
	}
	if root := o.categoryPhysicalRootDir(category); root != "" {
		return root
	}
	return mediaTypeRootDir(mediaType)
}

func (o *OrganizerService) categoryPhysicalRootDirForType(mediaType, category string) string {
	key := normalizeOrganizeCategoryKey(category)
	if key == "" {
		return ""
	}
	categories := o.categoryMap()
	match := func(values ...string) bool {
		for _, value := range values {
			if key == normalizeOrganizeCategoryKey(value) {
				return true
			}
		}
		return false
	}
	switch normalizeMediaType(mediaType, "", "") {
	case "movie":
		if match(
			categoryName(categories, "concert_movie", "演唱会"),
			categoryName(categories, "documentary_movie", "纪录片"),
			categoryName(categories, "animation_movie", "动画电影"),
			categoryName(categories, "chinese_movie", "华语电影"),
			categoryName(categories, "euus_movie", "欧美电影"),
			categoryName(categories, "jk_movie", "日韩电影"),
			"演唱会", "音乐会", "纪录片", "纪录", "动画电影", "动漫电影", "华语电影", "国产电影", "外语电影", "外国电影", "欧美电影", "日韩电影",
		) {
			return "电影"
		}
	case "tv", "variety":
		if match(
			categoryName(categories, "domestic_tv", "国产剧"),
			categoryName(categories, "euus_tv", "欧美剧"),
			categoryName(categories, "jk_tv", "日韩剧"),
			categoryName(categories, "variety", "综艺"),
			categoryName(categories, "documentary", "纪录片"),
			categoryName(categories, "children", "儿童"),
			"国产剧", "国剧", "大陆剧", "华语剧", "国产电视剧", "大陆电视剧", "华语电视剧", "港剧", "台剧", "港台剧",
			"欧美剧", "欧美电视剧", "美剧", "英剧", "日韩剧", "日韩电视剧", "日剧", "韩剧", "泰剧",
			"综艺", "真人秀", "纪录片", "纪录", "儿童", "少儿", "未分类",
		) {
			return "电视剧"
		}
	case "anime":
		if match(
			categoryName(categories, "cn_anime", "国漫"),
			categoryName(categories, "jp_anime", "日番"),
			categoryName(categories, "kr_anime", "韩漫"),
			categoryName(categories, "us_anime", "美漫"),
			categoryName(categories, "other_anime", "其他"),
			"国漫", "国产动漫", "日番", "番剧", "日漫", "日本动漫", "日本动画", "韩漫", "韩国动漫", "美漫", "欧美动漫", "欧美动画", "西方动画", "其他", "其他动漫", "其它动漫",
		) {
			return "动漫"
		}
	case "adult":
		if match(categoryName(categories, "adult", "成人"), "成人", "9kg", "番号", "jav") {
			return "成人"
		}
	}
	return ""
}

func (o *OrganizerService) categoryPhysicalRootDir(category string) string {
	key := normalizeOrganizeCategoryKey(category)
	if key == "" {
		return ""
	}
	categories := o.categoryMap()
	match := func(values ...string) bool {
		for _, value := range values {
			if key == normalizeOrganizeCategoryKey(value) {
				return true
			}
		}
		return false
	}
	switch {
	case match(
		categoryName(categories, "cn_anime", "国漫"),
		categoryName(categories, "jp_anime", "日番"),
		categoryName(categories, "kr_anime", "韩漫"),
		categoryName(categories, "us_anime", "美漫"),
		categoryName(categories, "other_anime", "其他"),
		"国漫", "国产动漫", "日番", "番剧", "日漫", "日本动漫", "日本动画", "韩漫", "韩国动漫", "美漫", "欧美动漫", "欧美动画", "西方动画", "其他", "其他动漫", "其它动漫",
	):
		return "动漫"
	case match(
		categoryName(categories, "domestic_tv", "国产剧"),
		categoryName(categories, "euus_tv", "欧美剧"),
		categoryName(categories, "jk_tv", "日韩剧"),
		categoryName(categories, "variety", "综艺"),
		categoryName(categories, "documentary", "纪录片"),
		categoryName(categories, "children", "儿童"),
		"国产剧", "国剧", "大陆剧", "华语剧", "国产电视剧", "大陆电视剧", "华语电视剧", "港剧", "台剧", "港台剧",
		"欧美剧", "欧美电视剧", "美剧", "英剧", "日韩剧", "日韩电视剧", "日剧", "韩剧", "泰剧",
		"综艺", "真人秀", "纪录片", "纪录", "儿童", "少儿", "未分类",
	):
		return "电视剧"
	case match(
		categoryName(categories, "concert_movie", "演唱会"),
		categoryName(categories, "documentary_movie", "纪录片"),
		categoryName(categories, "animation_movie", "动画电影"),
		categoryName(categories, "chinese_movie", "华语电影"),
		categoryName(categories, "euus_movie", "欧美电影"),
		categoryName(categories, "jk_movie", "日韩电影"),
		"演唱会", "音乐会", "动画电影", "动漫电影", "华语电影", "国产电影", "外语电影", "外国电影", "欧美电影", "日韩电影",
	):
		return "电影"
	case match(categoryName(categories, "adult", "成人"), "成人", "9kg", "番号", "jav"):
		return "成人"
	default:
		return ""
	}
}

func categoryRoot(root, category string) string {
	if strings.TrimSpace(category) == "" || pathAlreadyEndsWith(root, category) {
		return root
	}
	return filepath.Join(root, category)
}

func pathWithin(path, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if strings.EqualFold(cleanPath, cleanRoot) {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func mediaTypeRootDir(mediaType string) string {
	switch normalizeMediaType(mediaType, "", "") {
	case "movie":
		return "电影"
	case "anime":
		return "动漫"
	case "tv", "variety":
		return "电视剧"
	case "adult":
		return "成人"
	default:
		return ""
	}
}

func isGenericMediaRoot(path string) bool {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(filepath.Clean(path))))
	switch base {
	case "media", "medias", "library", "libraries", "organized", "整理":
		return true
	default:
		return false
	}
}

// organizeStagingFolderNames lists manual-organize style staging folder names.
// These workspaces should not remain as first-level category folders after an
// organize operation; redirectOrganizeStagingRoot lifts them to the media root.
func organizeStagingFolderNames() map[string]struct{} {
	return map[string]struct{}{
		"手动整理": {}, "手动整理入库": {}, "待整理": {}, "待分类": {},
		"manual": {}, "manual_organize": {}, "manualorganize": {}, "staging": {}, "inbox": {},
	}
}

func isOrganizeStagingDir(path string) bool {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(filepath.Clean(path))))
	if base == "" {
		return false
	}
	_, ok := organizeStagingFolderNames()[base]
	return ok
}

func redirectOrganizeStagingRoot(root string) string {
	cleaned := filepath.Clean(strings.TrimSpace(root))
	if cleaned == "" || cleaned == "." {
		return root
	}
	for isOrganizeStagingDir(cleaned) {
		parent := filepath.Dir(cleaned)
		if parent == cleaned || parent == "." || parent == string(filepath.Separator) {
			break
		}
		cleaned = parent
	}
	return cleaned
}

func normalizeOrganizeDestinationRoot(root string) string {
	cleaned := redirectOrganizeStagingRoot(root)
	if collectionRoot := organizeMediaCollectionRoot(cleaned); collectionRoot != "" {
		return collectionRoot
	}
	return cleaned
}

func normalizeMappedOrganizeDestinationRoot(root string) string {
	return normalizeOrganizeDestinationRoot(resolveMappedDestinationPath(root))
}

func pathAlreadyEndsWith(path, suffix string) bool {
	base := strings.TrimSpace(filepath.Base(filepath.Clean(path)))
	return strings.EqualFold(base, suffix)
}

func isSeriesLibraryType(mediaType string) bool {
	switch normalizeMediaType(mediaType, "", "") {
	case "tv", "anime", "variety":
		return true
	default:
		return false
	}
}
