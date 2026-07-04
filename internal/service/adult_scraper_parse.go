package service

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func parseAdultDetailHTML(body, code, source, detailURL string) *Match {
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
	if m := adultSamplePattern.FindStringSubmatch(body); len(m) > 1 {
		match.BackdropURL = absolutizeURL(detailURL, m[1])
	}
	if dmmPoster := adultDMMPosterFromSampleURL(match.BackdropURL); dmmPoster != "" {
		match.PosterURL = dmmPoster
	}
	match.Year = firstYearInText(body)
	match.Rating = firstRatingInText(body)
	return match
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
