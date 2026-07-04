package service

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func (o *OrganizerService) inferOrganizeDirectoryLayout(src, sourceRoot string) organizeDirectoryLayout {
	for _, name := range organizeDirectoryCategoryCandidates(src, sourceRoot) {
		if mediaType, category := o.mediaTypeForDirectoryCategory(name); mediaType != "" && category != "" {
			return organizeDirectoryLayout{MediaType: mediaType, Category: category}
		}
	}
	return organizeDirectoryLayout{}
}

func organizeDirectoryCategoryCandidates(src, sourceRoot string) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || value == "." || value == string(os.PathSeparator) {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}

	cleanSourceRoot := filepath.Clean(sourceRoot)
	for _, part := range organizePathNameParts(cleanSourceRoot) {
		add(part)
	}
	rel, err := filepath.Rel(cleanSourceRoot, filepath.Clean(src))
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return out
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		return out
	}
	for _, part := range strings.Split(dir, string(os.PathSeparator)) {
		add(part)
	}
	return out
}

func organizePathNameParts(path string) []string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return nil
	}
	volume := filepath.VolumeName(clean)
	if volume != "" {
		clean = strings.TrimPrefix(clean, volume)
	}
	clean = strings.Trim(clean, string(os.PathSeparator))
	if clean == "" {
		base := filepath.Base(filepath.Clean(path))
		if base == "." || base == string(os.PathSeparator) {
			return nil
		}
		return []string{base}
	}
	parts := strings.Split(clean, string(os.PathSeparator))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && part != "." {
			out = append(out, part)
		}
	}
	return out
}

func (o *OrganizerService) mediaTypeForDirectoryCategory(name string) (string, string) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return "", ""
	}
	if hit, ok := o.directoryCategoryTypes()[key]; ok {
		return hit.MediaType, hit.Category
	}
	return "", ""
}

func (o *OrganizerService) directoryCategoryTypes() map[string]organizeDirectoryLayout {
	categories := o.categoryMap()
	out := map[string]organizeDirectoryLayout{}
	add := func(category, mediaType string) {
		category = strings.TrimSpace(category)
		if category == "" {
			return
		}
		out[strings.ToLower(category)] = organizeDirectoryLayout{
			MediaType: mediaType,
			Category:  category,
		}
	}
	addConfigured := func(key, fallback, mediaType string) {
		add(fallback, mediaType)
		add(categoryName(categories, key, fallback), mediaType)
	}
	addAlias := func(alias, canonicalKey, fallback, mediaType string) {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			return
		}
		out[strings.ToLower(alias)] = organizeDirectoryLayout{
			MediaType: mediaType,
			Category:  categoryName(categories, canonicalKey, fallback),
		}
	}
	addConfigured("concert_movie", "演唱会", "movie")
	addConfigured("documentary_movie", "纪录片", "movie")
	addConfigured("animation_movie", "动画电影", "movie")
	addConfigured("chinese_movie", "华语电影", "movie")
	addConfigured("jk_movie", "日韩电影", "movie")
	addConfigured("euus_movie", "欧美电影", "movie")
	addAlias("外语电影", "euus_movie", "欧美电影", "movie")
	addAlias("外国电影", "euus_movie", "欧美电影", "movie")
	addConfigured("domestic_tv", "国产剧", "tv")
	addAlias("国剧", "domestic_tv", "国产剧", "tv")
	addAlias("大陆剧", "domestic_tv", "国产剧", "tv")
	addAlias("华语剧", "domestic_tv", "国产剧", "tv")
	addAlias("国产电视剧", "domestic_tv", "国产剧", "tv")
	addAlias("大陆电视剧", "domestic_tv", "国产剧", "tv")
	addAlias("华语电视剧", "domestic_tv", "国产剧", "tv")
	addAlias("港剧", "domestic_tv", "国产剧", "tv")
	addAlias("台剧", "domestic_tv", "国产剧", "tv")
	addAlias("港台剧", "domestic_tv", "国产剧", "tv")
	addConfigured("euus_tv", "欧美剧", "tv")
	addAlias("欧美电视剧", "euus_tv", "欧美剧", "tv")
	addAlias("美剧", "euus_tv", "欧美剧", "tv")
	addAlias("英剧", "euus_tv", "欧美剧", "tv")
	addConfigured("jk_tv", "日韩剧", "tv")
	addAlias("日韩电视剧", "jk_tv", "日韩剧", "tv")
	addAlias("日剧", "jk_tv", "日韩剧", "tv")
	addAlias("韩剧", "jk_tv", "日韩剧", "tv")
	addAlias("泰剧", "jk_tv", "日韩剧", "tv")
	addConfigured("cn_anime", "国漫", "anime")
	addConfigured("jp_anime", "日番", "anime")
	addConfigured("kr_anime", "韩漫", "anime")
	addConfigured("us_anime", "美漫", "anime")
	addConfigured("other_anime", "其他", "anime")
	addAlias("欧美动漫", "us_anime", "美漫", "anime")
	addAlias("欧美动画", "us_anime", "美漫", "anime")
	addAlias("西方动画", "us_anime", "美漫", "anime")
	addAlias("其他动漫", "other_anime", "其他", "anime")
	addAlias("其它动漫", "other_anime", "其他", "anime")
	addConfigured("variety", "综艺", "variety")
	addConfigured("documentary", "纪录片", "tv")
	addConfigured("children", "儿童", "tv")
	addAlias("未分类", "euus_tv", "欧美剧", "tv")
	addAlias("uncategorized", "euus_tv", "欧美剧", "tv")
	addConfigured("adult", "成人", "adult")
	addAlias("9KG", "adult", "成人", "adult")
	addAlias("番号", "adult", "成人", "adult")
	addAlias("JAV", "adult", "成人", "adult")
	return out
}

// titleCaseWords upper-cases the first letter of each ASCII word; CJK and other
// non-ASCII leading characters are left untouched. Roman numerals (ii, iii, iv,
// etc.) are fully upper-cased so sequels keep their canonical casing.
func titleCaseWords(s string) string {
	fields := strings.Fields(s)
	for i, w := range fields {
		if isRomanNumeral(w) {
			fields[i] = strings.ToUpper(w)
			continue
		}
		r := []rune(w)
		if len(r) > 0 && r[0] < 128 {
			r[0] = unicode.ToUpper(r[0])
			fields[i] = string(r)
		}
	}
	return strings.Join(fields, " ")
}

// sequelNumerals is a conservative whitelist of multi-letter Roman numerals
// used for movie/series sequels. A whitelist avoids false positives on normal
// English words that happen to be valid numerals (e.g. "mix", "civ", "mi").
var sequelNumerals = map[string]struct{}{
	"ii": {}, "iii": {}, "iv": {}, "vi": {}, "vii": {}, "viii": {},
	"ix": {}, "xi": {}, "xii": {}, "xiii": {}, "xiv": {}, "xv": {},
}

func isRomanNumeral(w string) bool {
	_, ok := sequelNumerals[strings.ToLower(w)]
	return ok
}
