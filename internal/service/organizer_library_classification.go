package service

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func organizeLibraryModelType(mediaType string) string {
	switch normalizeOrganizeMediaType(mediaType) {
	case "tv", "anime", "variety":
		return "tv"
	case "adult", "movie":
		return "movie"
	default:
		return "movie"
	}
}

func organizeLibraryTypeName(mediaType string) string {
	switch normalizeOrganizeMediaType(mediaType) {
	case "tv":
		return "电视剧"
	case "anime":
		return "动漫"
	case "variety":
		return "综艺"
	case "adult":
		return "成人"
	default:
		return "电影"
	}
}

func (o *OrganizerService) organizeCategoryAliases(mediaType, category string) map[string]struct{} {
	aliases := map[string]struct{}{}
	add := func(values ...string) {
		for _, value := range values {
			key := normalizeOrganizeCategoryKey(value)
			if key != "" {
				aliases[key] = struct{}{}
			}
		}
	}
	categories := o.categoryMap()
	switch normalizeOrganizeCategoryKey(category) {
	case normalizeOrganizeCategoryKey(categoryName(categories, "jp_anime", "日番")), "日番", "日漫", "日本动漫", "日本動畫", "日本动画":
		add("日番", categoryName(categories, "jp_anime", "日番"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "cn_anime", "国漫")), "国漫", "国产动漫", "國漫":
		add("国漫", categoryName(categories, "cn_anime", "国漫"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "kr_anime", "韩漫")), "韩漫", "韩国动漫", "韩国动画":
		add("韩漫", categoryName(categories, "kr_anime", "韩漫"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "us_anime", "美漫")), "美漫", "欧美动漫", "欧美动画", "西方动画":
		add("美漫", categoryName(categories, "us_anime", "美漫"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "other_anime", "其他")), "其他", "其他动漫", "其它动漫":
		add("其他", categoryName(categories, "other_anime", "其他"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "domestic_tv", "国产剧")), "国产剧", "国剧", "大陆剧", "华语剧", "国产电视剧", "大陆电视剧", "华语电视剧", "港剧", "台剧", "港台剧":
		add("国产剧", "国剧", "大陆剧", "华语剧", "国产电视剧", "大陆电视剧", "华语电视剧", "港剧", "台剧", "港台剧", categoryName(categories, "domestic_tv", "国产剧"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "euus_tv", "欧美剧")), "欧美剧", "欧美电视剧", "美剧", "英剧":
		add("欧美剧", "欧美电视剧", "美剧", "英剧", categoryName(categories, "euus_tv", "欧美剧"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "jk_tv", "日韩剧")), "日韩剧", "日韩电视剧", "日剧", "韩剧", "泰剧":
		add("日韩剧", "日韩电视剧", "日剧", "韩剧", "泰剧", categoryName(categories, "jk_tv", "日韩剧"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "variety", "综艺")), "综艺", "真人秀":
		add("综艺", categoryName(categories, "variety", "综艺"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "documentary", "纪录片")), "纪录片", "纪录":
		add("纪录片", categoryName(categories, "documentary", "纪录片"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "children", "儿童")), "儿童", "少儿":
		add("儿童", categoryName(categories, "children", "儿童"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "chinese_movie", "华语电影")), "华语电影", "国产电影", "大陆电影":
		add("华语电影", categoryName(categories, "chinese_movie", "华语电影"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "euus_movie", "欧美电影")), "欧美电影", "外语电影", "外国电影":
		add("欧美电影", categoryName(categories, "euus_movie", "欧美电影"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "jk_movie", "日韩电影")), "日韩电影", "日本电影", "韩国电影":
		add("日韩电影", categoryName(categories, "jk_movie", "日韩电影"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "concert_movie", "演唱会")), "演唱会", "音乐会":
		add("演唱会", categoryName(categories, "concert_movie", "演唱会"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "animation_movie", "动画电影")), "动画电影", "动漫电影":
		add("动画电影", categoryName(categories, "animation_movie", "动画电影"))
	case normalizeOrganizeCategoryKey(categoryName(categories, "adult", "成人")), "成人", "9kg", "番号", "jav":
		add("成人", categoryName(categories, "adult", "成人"))
	}
	return aliases
}

func libraryMatchesOrganizeCategory(lib model.Library, aliases map[string]struct{}) bool {
	for _, value := range []string{lib.Name, filepath.Base(filepath.Clean(lib.Path))} {
		if _, ok := aliases[normalizeOrganizeCategoryKey(value)]; ok {
			return true
		}
	}
	return false
}

func organizeLibraryTypeScore(mediaType, libraryType string) int {
	libraryType = normalizeOrganizeMediaType(libraryType)
	if mediaType == "" || libraryType == "" {
		return 1
	}
	if mediaType == libraryType {
		return 8
	}
	if mediaType == "anime" && libraryType == "tv" {
		return 5
	}
	if mediaType == "variety" && libraryType == "tv" {
		return 5
	}
	return 0
}

func normalizeOrganizeCategoryKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func pathDepth(path string) int {
	path = filepath.Clean(path)
	if path == "." || path == string(os.PathSeparator) {
		return 0
	}
	return len(strings.Split(path, string(os.PathSeparator)))
}
