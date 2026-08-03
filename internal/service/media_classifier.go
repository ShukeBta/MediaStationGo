package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var (
	classifierEpisodeRE = regexp.MustCompile(`(?i)\bS\d{1,2}E\d{1,3}\b|第\s*\d+\s*[集期]|(?:^|[\s._-])E\d{1,3}(?:[\s._-]|$)`)
	classifierSeasonRE  = regexp.MustCompile(`(?i)\bS\d{1,2}\b|第\s*\d+\s*季`)
	classifierJAVCodeRE = regexp.MustCompile(`(?:^|[\s._\-/\[\]()])[A-Z]{2,6}[-_]?\d{3,5}(?:[\s._\-/\[\]()]|$)`)
	classifierMovieRE   = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:movies?|films?)(?:[^a-z0-9]|$)`)
	classifierTVRE      = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:tv|series|shows?|dramas?)(?:[^a-z0-9]|$)`)
	classifierAnimeRE   = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:anime|bangumi)(?:[^a-z0-9]|$)`)
)

const DownloadSmartClassifySettingKey = "downloads.smart_classify"

type mediaClassifyInput struct {
	MediaType string
	Title     string
	Languages []string
	Countries []string
	Genres    []string
	Category  string
}

func classifyMediaCategory(input mediaClassifyInput, categories map[string]string) string {
	mediaType := normalizeMediaType(input.MediaType, input.Title, input.Category)
	genres := normalizeTokens(input.Genres...)
	countries := normalizeTokens(input.Countries...)
	languages := normalizeTokens(input.Languages...)
	rawTitleText := input.Title + " " + strings.Join(input.Genres, " ")
	categoryText := strings.ToLower(input.Category)
	rawText := rawTitleText + " " + input.Category
	text := strings.ToLower(rawText)
	hasMetadata := len(genres) > 0 || len(countries) > 0 || len(languages) > 0
	sourceHint := sourceCategoryHint(input.Category, mediaType, categories)

	isChineseByMetadata := hasAny(languages, "ZH", "ZH-CN", "ZH-TW", "CN", "BO", "ZA") || hasAny(countries, "CN", "TW", "HK", "MO")
	isChineseByText := containsHan(rawTitleText) || containsAnyText(strings.ToLower(rawTitleText), "华语", "国产", "国剧", "国漫")
	isChineseByCategory := containsAnyText(categoryText, "华语", "国产", "国剧", "大陆剧", "国产电视剧", "国产电影", "国漫", "国产动漫", "国产动画")
	isChinese := isChineseByMetadata || (!hasMetadata && isChineseByText)
	isChineseAnime := isChineseByMetadata || (!hasMetadata && containsAnyText(text, "华语", "国产", "国漫", "國漫", "国创", "国产动漫", "国产动画"))
	isJapanese := hasAny(languages, "JA", "JP") || hasAny(countries, "JP") || containsJapaneseKana(rawTitleText) || (!hasMetadata && strings.Contains(text, "日番"))
	isKorean := hasAny(languages, "KO", "KR") || hasAny(countries, "KR", "KP") || containsKoreanHangul(rawTitleText) || (!hasMetadata && containsAnyText(categoryText, "韩漫", "韩国动漫", "韩国动画"))
	isEastAsianByCategory := containsAnyText(categoryText, "日韩剧", "日剧", "韩剧", "日韩电影")
	isEastAsian := isJapanese || isKorean || hasAny(countries, "TH", "IN", "SG") || (!hasMetadata && isEastAsianByCategory)
	isWesternByMetadata := hasAny(countries,
		"US", "GB", "UK", "FR", "DE", "CA", "AU", "NZ", "IE", "NL", "SE", "NO", "DK",
		"FI", "ES", "IT", "PT", "AT", "CH", "BE", "RU",
	)
	isWesternByCategory := containsAnyText(categoryText, "欧美剧", "欧美电视剧", "美剧", "英剧", "欧美电影", "外语电影")
	isWestern := isWesternByMetadata || (!hasMetadata && isWesternByCategory)
	isUSAnime := hasAny(countries, "US")
	hasAnimeText := containsAnyText(text, "动画", "动漫", "番剧", "年番", "国漫", "日番", "韩漫", "美漫", "bangumi", "anime", "b-global", "ani-one", "crunchyroll")
	hasVarietyText := containsAnyText(text, "综艺", "真人秀", "脱口秀", "晚会", "春晚", "gala", "festival gala", "reality", "talk show")
	hasDocumentaryText := containsAnyText(text, "纪录", "纪录片", "documentary", "docu", "national geographic", "natgeo")
	hasConcertText := containsAnyText(text, "演唱会", "音乐会", "concert", "live concert")
	isAdultText := containsAnyText(text, "adult", "nsfw", "成人", "番号", "jav", "9kg", "uncensored", "无码", "有码") || classifierJAVCodeRE.MatchString(strings.ToUpper(rawText))

	hasGenre := func(values ...string) bool {
		for _, value := range values {
			if hasAny(genres, strings.ToUpper(value)) {
				return true
			}
			if isDigits(value) {
				continue
			}
			if strings.Contains(text, strings.ToLower(value)) {
				return true
			}
		}
		return false
	}
	animeCategory := func() string {
		if isChineseAnime {
			return categoryName(categories, "cn_anime", "国漫")
		}
		if isJapanese {
			return categoryName(categories, "jp_anime", "日番")
		}
		if isKorean {
			return categoryName(categories, "kr_anime", "韩漫")
		}
		if isUSAnime || (!hasMetadata && containsAnyText(categoryText, "美漫", "欧美动漫", "欧美动画", "西方动画")) {
			return categoryName(categories, "us_anime", "美漫")
		}
		return categoryName(categories, "other_anime", "其他")
	}

	switch mediaType {
	case "movie":
		if isAdultText {
			return categoryName(categories, "adult", "成人")
		}
		if hasGenre("10402", "MUSIC", "音乐") || hasConcertText {
			return categoryName(categories, "concert_movie", "演唱会")
		}
		if hasGenre("99", "DOCUMENTARY", "纪录", "纪录片") || hasDocumentaryText {
			return categoryName(categories, "documentary_movie", "纪录片")
		}
		if hasGenre("16", "ANIMATION", "动画", "动漫") || hasAnimeText {
			return categoryName(categories, "animation_movie", "动画电影")
		}
		if !hasMetadata && sourceHint != "" {
			return sourceHint
		}
		if isChinese {
			return categoryName(categories, "chinese_movie", "华语电影")
		}
		if hasAny(languages, "JA", "KO") || (!hasAny(languages, "ZH", "ZH-CN", "ZH-TW", "CN", "BO", "ZA") && hasAny(countries, "JP", "KP", "KR")) {
			return categoryName(categories, "jk_movie", "日韩电影")
		}
		return categoryName(categories, "euus_movie", "欧美电影")
	case "anime":
		if isAdultText {
			return categoryName(categories, "adult", "成人")
		}
		if !hasMetadata && sourceHint != "" {
			return sourceHint
		}
		return animeCategory()
	case "variety":
		return categoryName(categories, "variety", "综艺")
	case "tv":
		if isAdultText {
			return categoryName(categories, "adult", "成人")
		}
		// Match MoviePilot's ordered category rules: animation categories are
		// evaluated before documentary / children / variety and region rules.
		if hasGenre("16", "ANIMATION", "动画", "动漫") || hasAnimeText {
			return animeCategory()
		}
		if hasGenre("99", "DOCUMENTARY", "纪录", "纪录片") || hasDocumentaryText {
			return categoryName(categories, "documentary", "纪录片")
		}
		if hasGenre("10762", "KIDS", "儿童") {
			return categoryName(categories, "children", "儿童")
		}
		if hasGenre("10764", "10767", "REALITY", "TALK", "综艺", "真人秀", "脱口秀") || hasVarietyText {
			return categoryName(categories, "variety", "综艺")
		}
		if !hasMetadata && sourceHint != "" {
			return sourceHint
		}
		if isChinese || (!hasMetadata && isChineseByCategory) {
			return categoryName(categories, "domestic_tv", "国产剧")
		}
		if isEastAsian {
			return categoryName(categories, "jk_tv", "日韩剧")
		}
		if isWestern {
			return categoryName(categories, "euus_tv", "欧美剧")
		}
		return categoryName(categories, "unclassified_tv", "未分类")
	case "adult":
		return categoryName(categories, "adult", "成人")
	}
	return ""
}

func normalizeMediaType(mediaType, title, category string) string {
	raw := strings.ToLower(strings.TrimSpace(mediaType))
	switch raw {
	case "movie", "film":
		return "movie"
	case "tv", "series", "show", "drama":
		return "tv"
	case "anime", "animation":
		return "anime"
	case "variety":
		return "variety"
	case "adult", "nsfw":
		return "adult"
	}
	switch {
	case containsAnyText(raw, "成人", "番号", "jav", "nsfw"):
		return "adult"
	case containsAnyText(raw, "综艺", "真人秀"):
		return "variety"
	case (containsAnyText(raw, "国漫", "日漫", "日番", "韩漫", "美漫", "欧美动漫", "其他动漫", "动漫", "动画") || classifierAnimeRE.MatchString(raw)) && !containsAnyText(raw, "动画电影"):
		return "anime"
	case containsAnyText(raw, "电视剧", "剧集", "连续剧", "短剧", "国产剧", "国剧", "大陆剧", "华语剧", "国产电视剧", "大陆电视剧", "华语电视剧", "欧美剧", "欧美电视剧", "美剧", "英剧", "日韩剧", "日韩电视剧", "日剧", "韩剧", "港剧", "台剧", "港台剧", "泰剧") || classifierTVRE.MatchString(raw):
		return "tv"
	case containsAnyText(raw, "电影", "演唱会") || classifierMovieRE.MatchString(raw):
		return "movie"
	}
	text := strings.ToLower(title + " " + category)
	switch {
	case strings.Contains(text, "adult") || strings.Contains(text, "nsfw") || strings.Contains(text, "成人") || strings.Contains(text, "番号") || strings.Contains(text, "jav") || strings.Contains(text, "9kg") || classifierJAVCodeRE.MatchString(strings.ToUpper(title+" "+category)):
		return "adult"
	case containsAnyText(text, "综艺", "真人秀", "脱口秀", "晚会", "春晚", "gala", "festival gala", "reality", "talk show"):
		return "variety"
	case strings.Contains(text, "电影") || classifierMovieRE.MatchString(text):
		return "movie"
	case classifierAnimeRE.MatchString(text) || strings.Contains(text, "动漫") || strings.Contains(text, "动画"):
		return "anime"
	case strings.Contains(text, "variety") || strings.Contains(text, "综艺") || strings.Contains(text, "真人秀"):
		return "variety"
	case classifierEpisodeRE.MatchString(text) || classifierSeasonRE.MatchString(text) || classifierTVRE.MatchString(text) || containsAnyText(text, "剧集", "电视剧", "连续剧", "短剧", "国产电视剧", "大陆电视剧", "华语电视剧", "欧美电视剧", "日韩电视剧", "美剧", "英剧", "港剧", "台剧", "港台剧", "泰剧"):
		return "tv"
	default:
		return "movie"
	}
}

func (o *OrganizerService) categoryMap() map[string]string {
	if o == nil || o.cfg == nil || o.cfg.Organizer.Categories == nil {
		return nil
	}
	return o.cfg.Organizer.Categories
}

func isUnclassifiedTVCategory(category string, categories map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(category), categoryName(categories, "unclassified_tv", "未分类"))
}

func (o *OrganizerService) classifyMedia(ctx context.Context, m *model.Media, mediaType string) string {
	if m == nil {
		return ""
	}
	if m.Languages == "" && m.Countries == "" && m.Genres == "" && o != nil && o.repo != nil && o.repo.Media != nil {
		if fresh, err := o.repo.Media.FindByID(ctx, m.ID); err == nil && fresh != nil {
			m = fresh
		}
	}
	return classifyMediaCategory(mediaClassifyInput{
		MediaType: mediaType,
		Title:     m.Title + " " + m.OriginalName,
		Languages: parseCommaList(m.Languages),
		Countries: parseCommaList(m.Countries),
		Genres:    parseCommaList(m.Genres),
	}, o.categoryMap())
}
