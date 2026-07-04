package service

import "strings"

type sourceCategoryHintDef struct {
	Key       string
	Fallback  string
	MediaType string
	Aliases   []string
}

var sourceCategoryHints = []sourceCategoryHintDef{
	{Key: "concert_movie", Fallback: "演唱会", MediaType: "movie", Aliases: []string{"concert", "音乐会"}},
	{Key: "documentary_movie", Fallback: "纪录片", MediaType: "movie", Aliases: []string{"纪录", "documentary"}},
	{Key: "animation_movie", Fallback: "动画电影", MediaType: "movie"},
	{Key: "chinese_movie", Fallback: "华语电影", MediaType: "movie"},
	{Key: "jk_movie", Fallback: "日韩电影", MediaType: "movie"},
	{Key: "euus_movie", Fallback: "欧美电影", MediaType: "movie", Aliases: []string{"外语电影", "外国电影", "western movie", "foreign movie"}},
	{Key: "domestic_tv", Fallback: "国产剧", MediaType: "tv"},
	{Key: "euus_tv", Fallback: "欧美剧", MediaType: "tv"},
	{Key: "jk_tv", Fallback: "日韩剧", MediaType: "tv", Aliases: []string{"日剧", "韩剧", "泰剧"}},
	{Key: "cn_anime", Fallback: "国漫", MediaType: "anime"},
	{Key: "jp_anime", Fallback: "日番", MediaType: "anime"},
	{Key: "kr_anime", Fallback: "韩漫", MediaType: "anime"},
	{Key: "us_anime", Fallback: "美漫", MediaType: "anime", Aliases: []string{"欧美动漫", "欧美动画", "西方动画"}},
	{Key: "other_anime", Fallback: "其他", MediaType: "anime", Aliases: []string{"其他动漫", "其它动漫", "other"}},
	{Key: "variety", Fallback: "综艺", MediaType: "variety"},
	{Key: "documentary", Fallback: "纪录片", MediaType: "tv"},
	{Key: "children", Fallback: "儿童", MediaType: "tv"},
	{Key: "euus_tv", Fallback: "欧美剧", MediaType: "tv", Aliases: []string{"未分类", "uncategorized"}},
	{Key: "adult", Fallback: "成人", MediaType: "adult", Aliases: []string{"9KG", "番号", "JAV", "adult", "nsfw"}},
}

func sourceCategoryHint(category, mediaType string, categories map[string]string) string {
	tokens := sourceCategoryTokens(category)
	if len(tokens) == 0 {
		return ""
	}
	for _, hint := range sourceCategoryHints {
		if !sourceCategoryCompatible(mediaType, hint.MediaType) {
			continue
		}
		names := append([]string{hint.Fallback, categoryName(categories, hint.Key, hint.Fallback)}, hint.Aliases...)
		for _, name := range names {
			if _, ok := tokens[strings.ToLower(strings.TrimSpace(name))]; ok {
				return categoryName(categories, hint.Key, hint.Fallback)
			}
		}
	}
	return ""
}

func sourceCategoryTokens(category string) map[string]struct{} {
	category = strings.TrimSpace(category)
	if category == "" {
		return nil
	}
	normalized := strings.NewReplacer("\\", " ", "/", " ", "|", " ", ",", " ", ";", " ").Replace(category)
	out := map[string]struct{}{
		strings.ToLower(category): {},
	}
	for _, field := range strings.Fields(normalized) {
		out[strings.ToLower(strings.TrimSpace(field))] = struct{}{}
	}
	return out
}

func sourceCategoryCompatible(mediaType, categoryMediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	categoryMediaType = strings.ToLower(strings.TrimSpace(categoryMediaType))
	if mediaType == "" || categoryMediaType == "" || mediaType == categoryMediaType {
		return true
	}
	if mediaType == "tv" && (categoryMediaType == "anime" || categoryMediaType == "variety") {
		return true
	}
	if categoryMediaType == "adult" {
		return true
	}
	return false
}
