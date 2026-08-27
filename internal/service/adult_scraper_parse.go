package service

import (
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var javDBGenreTranslations = map[string]string{
	"69": "六九式",
}

func parseAdultDetailHTML(body, code, source, detailURL string) *Match {
	if !adultDetailMatchesRequestedCode(body, code) {
		return nil
	}
	match := &Match{
		OriginalName: code,
		MediaType:    "adult",
		NSFW:         true,
		Genres:       []string{"Adult", source},
	}
	if title := firstAdultTitle(body, code); title != "" {
		match.Title = title
	}
	if match.Title == "" {
		return nil
	}
	if source == "javbus" {
		if m := adultJavBusCoverPattern.FindStringSubmatch(body); len(m) > 1 {
			match.PosterURL = absolutizeURL(detailURL, m[1])
		}
	} else if cover := firstAdultImage(body, "video-cover", "cover", "column-video-cover"); cover != "" {
		match.PosterURL = absolutizeURL(detailURL, cover)
	}
	if sample := firstAdultSampleURL(body); sample != "" {
		match.BackdropURL = absolutizeURL(detailURL, sample)
	}
	if source == "javdb" {
		match.PreviewImages = adultJavDBPreviewImageURLs(body, detailURL)
	}
	if dmmPoster := adultDMMPosterFromSampleURL(match.BackdropURL); dmmPoster != "" {
		match.PosterURL = dmmPoster
	}
	if mgPoster, mgBackdrop := adultMGStageArtworkFromSampleURL(match.BackdropURL); mgPoster != "" {
		match.PosterURL = mgPoster
		if mgBackdrop != "" {
			match.BackdropURL = mgBackdrop
		}
	}
	match.Year = firstYearInText(body)
	match.Rating = adultPanelRating(body)
	if match.Rating <= 0 {
		match.Rating = firstRatingInText(body)
	}
	match.ReleaseDate = adultPanelDate(body)
	if len(match.ReleaseDate) >= 4 {
		match.Year, _ = strconv.Atoi(match.ReleaseDate[:4])
	}
	match.DurationMinutes = adultPanelDurationMinutes(body)
	match.Maker = adultPanelValue(body, "片商", "メーカー", "Maker")
	genres := adultPanelList(body, "類別", "类别", "ジャンル", "Genre")
	if source == "javdb" {
		genres = normalizeJavDBGenreValues(genres)
	}
	match.Genres = compactUniqueStrings(append(match.Genres, genres...)...)
	match.People = firstAdultPeople(body, source, detailURL)
	match.Actors = personMetadataNames(match.People)
	return match
}

func firstAdultSampleURL(body string) string {
	for _, found := range adultAnchorPattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 2 {
			continue
		}
		attrs := adultAttrs(found[1])
		class := " " + strings.ToLower(strings.Join(strings.Fields(attrs["class"]), " ")) + " "
		if !strings.Contains(class, " sample-box ") && !strings.Contains(class, " tile-item ") {
			continue
		}
		if href := strings.TrimSpace(attrs["href"]); href != "" {
			return href
		}
	}
	return ""
}

func adultJavDBPreviewImageURLs(body, detailURL string) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, found := range adultAnchorPattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 2 {
			continue
		}
		attrs := adultAttrs(found[1])
		class := " " + strings.ToLower(strings.Join(strings.Fields(attrs["class"]), " ")) + " "
		if !strings.Contains(class, " sample-box ") && !strings.Contains(class, " tile-item ") {
			continue
		}
		candidate := absolutizeURL(detailURL, attrs["href"])
		if !isLargeJavDBPreviewImageURL(candidate) {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func isLargeJavDBPreviewImageURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	path := strings.ToLower(u.Path)
	if !strings.Contains(path, "/samples/") {
		return false
	}
	for _, marker := range []string{"/thumb", "/small", "thumbnail", "_s_", "_t_", "-small."} {
		if strings.Contains(path, marker) {
			return false
		}
	}
	return strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") ||
		strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".webp")
}

func adultPanelDate(body string) string {
	return adultListDatePattern.FindString(adultPanelValue(body, "日期", "發行日期", "发行日期", "発売日", "Release Date"))
}

func adultPanelDurationMinutes(body string) int {
	value := adultPanelValue(body, "時長", "时长", "収録時間", "Runtime")
	match := regexp.MustCompile(`\d+`).FindString(value)
	minutes, _ := strconv.Atoi(match)
	return minutes
}

func adultPanelRating(body string) float32 {
	match := adultListScorePattern.FindStringSubmatch(adultPanelValue(body, "評分", "评分", "評価", "Rating"))
	if len(match) < 2 {
		return 0
	}
	value, _ := strconv.ParseFloat(match[1], 32)
	return float32(value)
}

func adultPanelList(body string, labels ...string) []string {
	value := strings.NewReplacer("，", ",", "、", ",").Replace(adultPanelValue(body, labels...))
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func normalizeJavDBGenreValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if translated, ok := javDBGenreTranslations[value]; ok {
			out = append(out, translated)
			continue
		}
		if _, err := strconv.Atoi(value); err == nil {
			continue
		}
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func adultPanelValue(body string, labels ...string) string {
	for _, found := range adultPanelBlockPattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 2 {
			continue
		}
		value := strings.TrimSpace(stripAdultHTML(found[1]))
		for _, label := range labels {
			for _, separator := range []string{":", "："} {
				prefix := strings.TrimSpace(label) + separator
				if strings.HasPrefix(value, prefix) {
					return strings.TrimSpace(strings.TrimPrefix(value, prefix))
				}
			}
		}
	}
	return ""
}

func firstAdultActors(body string) []string {
	return personMetadataNames(firstAdultPeople(body, "", ""))
}

func firstAdultPeople(body, source, detailURL string) []PersonMetadata {
	people := make([]PersonMetadata, 0, 4)
	for _, found := range adultAnchorPattern.FindAllStringSubmatchIndex(body, -1) {
		if len(found) < 6 {
			continue
		}
		attrs := adultAttrs(body[found[2]:found[3]])
		if !adultActorAnchor(attrs) {
			continue
		}
		inner := body[found[4]:found[5]]
		name := stripAdultHTML(inner)
		if name == "" {
			name = strings.TrimSpace(attrs["title"])
		}
		if name == "" {
			name = adultActorImageName(inner)
		}
		if validAdultActorName(name) {
			profileURL := absolutizeURL(detailURL, attrs["href"])
			sourceID := ""
			if strings.EqualFold(strings.TrimSpace(source), "javdb") {
				sourceID = adultPerformerSourceID(profileURL)
				tailEnd := min(len(body), found[1]+256)
				if sourceID == "" || !adultJavDBFemaleAfterAnchorPattern.MatchString(body[found[1]:tailEnd]) {
					continue
				}
			}
			people = append(people, PersonMetadata{
				Name:       name,
				ImageURL:   adultActorImageURL(inner, detailURL),
				ProfileURL: profileURL,
				Source:     source,
				SourceID:   sourceID,
			})
		}
	}
	for _, found := range adultJSONLDPattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 2 {
			continue
		}
		var value any
		if json.Unmarshal([]byte(html.UnescapeString(strings.TrimSpace(found[1]))), &value) == nil {
			for _, name := range adultActorsFromJSONLD(value) {
				people = append(people, PersonMetadata{Name: name, Source: source})
			}
		}
	}
	return deduplicatePersonMetadata(people)
}

func adultActorImageName(body string) string {
	image := adultImagePattern.FindStringSubmatch(body)
	if len(image) < 2 {
		return ""
	}
	attrs := adultAttrs(image[1])
	return firstText(attrs["alt"], attrs["title"])
}

func adultActorImageURL(body, detailURL string) string {
	image := adultImagePattern.FindStringSubmatch(body)
	if len(image) < 2 {
		return ""
	}
	attrs := adultAttrs(image[1])
	raw := firstText(attrs["data-src"], attrs["data-original"], attrs["data-lazy-src"], attrs["src"])
	return absolutizeURL(detailURL, raw)
}

func adultActorAnchor(attrs map[string]string) bool {
	href := strings.ToLower(strings.TrimSpace(attrs["href"]))
	class := strings.ToLower(strings.TrimSpace(attrs["class"]))
	return strings.Contains(href, "/actors/") ||
		strings.Contains(href, "/actor/") ||
		strings.Contains(href, "/star/") ||
		strings.Contains(href, "/actress/") ||
		strings.Contains(class, "star-name") ||
		strings.Contains(class, "actor-name") ||
		strings.Contains(class, "actress-name")
}

func adultActorsFromJSONLD(value any) []string {
	actors := make([]string, 0, 4)
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, actorValue := range typed {
				switch strings.ToLower(strings.TrimSpace(key)) {
				case "actor", "actors", "performer", "performers":
					actors = append(actors, adultActorNamesFromJSONValue(actorValue)...)
				}
			}
			if graph, ok := typed["@graph"]; ok {
				walk(graph)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return actors
}

func adultActorNamesFromJSONValue(value any) []string {
	switch typed := value.(type) {
	case string:
		if validAdultActorName(typed) {
			return []string{strings.TrimSpace(typed)}
		}
	case map[string]any:
		if name, ok := typed["name"].(string); ok && validAdultActorName(name) {
			return []string{strings.TrimSpace(name)}
		}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, adultActorNamesFromJSONValue(item)...)
		}
		return out
	}
	return nil
}

func validAdultActorName(value string) bool {
	value = strings.TrimSpace(stripAdultHTML(value))
	if value == "" || len([]rune(value)) > 100 {
		return false
	}
	normalized := strings.ToLower(strings.NewReplacer(
		" ", "", "\t", "", "·", "", "・", "", "/", "", "_", "", "-", "",
	).Replace(value))
	switch normalized {
	case "actor", "actors", "actress", "actresses", "performer", "performers",
		"演员", "演員", "女优", "女優", "男优", "男優", "男性演员", "男性演員",
		"有码", "有碼", "无码", "無碼", "有码女优", "有碼女優", "无码女优", "無碼女優",
		"有码无码", "有碼無碼", "censored", "uncensored", "western", "欧美", "歐美":
		return false
	default:
		return true
	}
}

func firstAdultTitle(body, code string) string {
	for _, found := range adultTitlePattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 2 {
			continue
		}
		title := strings.TrimSpace(stripAdultHTML(found[1]))
		if title == "" {
			continue
		}
		title = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(title, code), strings.ToUpper(code)))
		if title != "" {
			return title
		}
	}
	return ""
}

func adultDetailMatchesRequestedCode(body, code string) bool {
	expected := adultCodeKey(code)
	if expected == "" {
		return false
	}
	for _, found := range adultTitlePattern.FindAllStringSubmatch(body, -1) {
		if len(found) > 1 && adultCodeKey(stripAdultHTML(found[1])) == expected {
			return true
		}
	}
	return adultCodeKey(firstHTMLTitle(body)) == expected
}

func stripAdultHTML(value string) string {
	value = adultTagPattern.ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(html.UnescapeString(value)), " ")
}

func firstAdultImage(body string, classNeedles ...string) string {
	for _, found := range adultImagePattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 2 {
			continue
		}
		attrs := adultAttrs(found[1])
		class := strings.ToLower(attrs["class"])
		for _, needle := range classNeedles {
			if strings.Contains(class, strings.ToLower(needle)) {
				if attrs["src"] != "" {
					return attrs["src"]
				}
				if attrs["data-src"] != "" {
					return attrs["data-src"]
				}
			}
		}
	}
	return ""
}

func adultDMMPosterFromSampleURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || !strings.Contains(strings.ToLower(u.Host), "dmm.co.jp") {
		return ""
	}
	lowerPath := strings.ToLower(u.Path)
	for _, suffix := range []string{"jp-1.jpg", "jp.jpg"} {
		if strings.HasSuffix(lowerPath, suffix) {
			u.Path = u.Path[:len(u.Path)-len(suffix)] + "pl.jpg"
			return u.String()
		}
	}
	return ""
}

func adultMGStageArtworkFromSampleURL(raw string) (string, string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || !strings.Contains(strings.ToLower(u.Host), "image.mgstage.com") {
		return "", ""
	}
	lastSlash := strings.LastIndex(u.Path, "/")
	file := u.Path
	dir := ""
	if lastSlash >= 0 {
		dir = u.Path[:lastSlash]
		file = u.Path[lastSlash+1:]
	}
	lowerFile := strings.ToLower(file)
	const prefix = "cap_e_0_"
	if !strings.HasPrefix(lowerFile, prefix) || !strings.HasSuffix(lowerFile, ".jpg") {
		return "", ""
	}
	poster := u.String()
	backdropURL := *u
	backdropURL.Path = strings.TrimRight(dir, "/") + "/pb_e_" + file[len(prefix):]
	return poster, backdropURL.String()
}

func adultAttrs(raw string) map[string]string {
	out := map[string]string{}
	for _, found := range adultAttrPattern.FindAllStringSubmatch(raw, -1) {
		if len(found) >= 3 {
			out[strings.ToLower(found[1])] = html.UnescapeString(found[2])
		}
	}
	return out
}

func absolutizeURL(base, raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err == nil && u.IsAbs() {
		return raw
	}
	b, err := url.Parse(base)
	if err != nil {
		return raw
	}
	return b.ResolveReference(u).String()
}

func firstYearInText(body string) int {
	m := regexp.MustCompile(`(?:19|20)\d{2}[-/.]\d{1,2}[-/.]\d{1,2}`).FindString(body)
	if len(m) >= 4 {
		year, _ := strconv.Atoi(m[:4])
		return year
	}
	return 0
}

func firstRatingInText(body string) float32 {
	m := regexp.MustCompile(`(?i)(?:score|rating|評分|评分)[^0-9]{0,20}([0-9](?:\.[0-9])?)`).FindStringSubmatch(body)
	if len(m) > 1 {
		v, _ := strconv.ParseFloat(m[1], 32)
		return float32(v)
	}
	return 0
}

func parseOneJavDetailHTML(body, code, detailURL string) *Match {
	expected := oneJavCodeKey(code)
	if expected == "" || strings.TrimSpace(body) == "" {
		return nil
	}
	if oneJavCodeKey(firstHTMLTitle(body)) != expected {
		return nil
	}
	poster := firstAdultImage(body, "image")
	if poster != "" {
		poster = absolutizeURL(detailURL, poster)
	}
	pretty := oneJavPrettyCode(expected)
	return &Match{
		OriginalName: pretty,
		MediaType:    "adult",
		Title:        pretty,
		PosterURL:    poster,
		NSFW:         true,
		Genres:       []string{"Adult", "onejav"},
	}
}

func firstHTMLTitle(body string) string {
	m := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`).FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return stripAdultHTML(m[1])
}

func adultCodeKey(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	if key := oneJavCodeKey(text); key != "" {
		return key
	}
	if code := normalizeAdultCode(text); code != "" {
		return asciiAlnumUpper(code)
	}
	return asciiAlnumUpper(text)
}

func oneJavCodeKey(value string) string {
	if m := adultFC2Pattern.FindStringSubmatch(value); len(m) > 1 {
		return "FC2PPV" + m[1]
	}
	normalized := asciiAlnumUpper(value)
	if m := regexp.MustCompile(`FC2PPV(\d{5,10})`).FindStringSubmatch(normalized); len(m) > 1 {
		return "FC2PPV" + m[1]
	}
	return ""
}

func oneJavPrettyCode(key string) string {
	if m := regexp.MustCompile(`^FC2PPV(\d{5,10})$`).FindStringSubmatch(strings.ToUpper(key)); len(m) > 1 {
		return "FC2-PPV-" + m[1]
	}
	return key
}

func oneJavSlugs(code string) []string {
	if key := oneJavCodeKey(code); key != "" {
		return []string{strings.ToLower(key)}
	}
	return nil
}

func asciiAlnumUpper(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(value) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
