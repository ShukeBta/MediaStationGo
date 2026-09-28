package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var fd2PPVBaseURL = "https://fd2ppv.cc"

var (
	adultFD2WorkTitlePattern = regexp.MustCompile(`(?is)<h1\b[^>]*class=["'][^"']*\bwork-title\b[^"']*["'][^>]*>(.*?)</h1>`)
	adultFD2WorkBriefPattern = regexp.MustCompile(`(?is)<div\b[^>]*class=["'][^"']*\bwork-brief\b[^"']*["'][^>]*>(.*?)</div>`)
	adultFD2PhotosPattern    = regexp.MustCompile(`(?is)<div\b[^>]*class=["'][^"']*\bwork-original-photos\b[^"']*["'][^>]*>(.*?)</div>`)
	adultFD2MetaPattern      = regexp.MustCompile(`(?is)<div\b[^>]*class=["'][^"']*\bwork-meta-label\b[^"']*["'][^>]*>(.*?)</div>\s*<div\b[^>]*class=["'][^"']*\bwork-meta-value\b[^"']*["'][^>]*>(.*?)</div>`)
	adultFD2DatePattern      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	adultFD2NumberPattern    = regexp.MustCompile(`\d{5,10}`)
)

func adultFC2Number(code string) string {
	if match := adultFC2Pattern.FindStringSubmatch(strings.TrimSpace(code)); len(match) > 1 {
		return match[1]
	}
	return ""
}

func (p *AdultProvider) scrapeFD2PPV(ctx context.Context, code string) (*Match, error) {
	number := adultFC2Number(code)
	if number == "" || strings.TrimSpace(p.flareSolverrURL) == "" {
		return nil, nil
	}
	detailURL := strings.TrimRight(fd2PPVBaseURL, "/") + "/articles/" + url.PathEscape(number)
	body, err := p.fetchFD2PPVText(ctx, detailURL)
	if err != nil {
		return nil, err
	}
	match := parseFD2PPVDetailHTML(body, code, detailURL)
	if match == nil {
		return nil, fmt.Errorf("fd2ppv returned no usable metadata for %s", code)
	}
	return match, nil
}

func parseFD2PPVDetailHTML(body, code, detailURL string) *Match {
	number := adultFC2Number(code)
	if number == "" || strings.TrimSpace(body) == "" {
		return nil
	}
	workTitle := adultFD2TextMatch(body, adultFD2WorkTitlePattern)
	if adultFD2NumberPattern.FindString(workTitle) != number {
		return nil
	}
	title := adultFD2TextMatch(body, adultFD2WorkBriefPattern)
	if title == "" {
		return nil
	}
	photos := adultFD2PhotoURLs(adultFD2TextMatch(body, adultFD2PhotosPattern))
	meta := adultFD2Metadata(body)
	releaseDate := strings.TrimSpace(meta["配信日"])
	if !adultFD2DatePattern.MatchString(releaseDate) {
		releaseDate = ""
	}
	year := 0
	if len(releaseDate) >= 4 {
		year, _ = strconv.Atoi(releaseDate[:4])
	}
	people := adultFD2People(body, detailURL)
	genres := []string{"Adult", "fd2ppv"}
	if value := strings.TrimSpace(meta["カテゴリ"]); value != "" {
		genres = append(genres, value)
	}
	switch strings.TrimSpace(meta["モザイク"]) {
	case "無":
		genres = append(genres, "无码")
	case "有":
		genres = append(genres, "有码")
	}
	if strings.TrimSpace(meta["顔出し"]) == "○" {
		genres = append(genres, "露脸")
	}

	match := &Match{
		OriginalName:    "FC2-PPV-" + number,
		MediaType:       "adult",
		Title:           title,
		Year:            year,
		ReleaseDate:     releaseDate,
		DurationMinutes: adultFD2DurationMinutes(meta["収録時間"]),
		Maker:           strings.TrimSpace(meta["販売者"]),
		Genres:          compactUniqueStrings(genres...),
		People:          people,
		Actors:          personMetadataNames(people),
		NSFW:            true,
	}
	if len(photos) > 0 {
		match.PosterURL = photos[0]
		match.BackdropURL = photos[0]
	}
	if len(photos) > 1 {
		match.PreviewImages = append([]string(nil), photos[1:]...)
	}
	return match
}

func adultFD2TextMatch(body string, pattern *regexp.Regexp) string {
	match := pattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(stripAdultHTML(match[1]))
}

func adultFD2Metadata(body string) map[string]string {
	out := map[string]string{}
	for _, match := range adultFD2MetaPattern.FindAllStringSubmatch(body, -1) {
		if len(match) < 3 {
			continue
		}
		label := strings.TrimSpace(stripAdultHTML(match[1]))
		value := strings.TrimSpace(stripAdultHTML(match[2]))
		if label != "" && value != "" {
			out[label] = value
		}
	}
	return out
}

func adultFD2PhotoURLs(value string) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, raw := range strings.Fields(value) {
		raw = strings.TrimSpace(raw)
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		path := strings.ToLower(u.Path)
		if !strings.HasSuffix(path, ".jpg") && !strings.HasSuffix(path, ".jpeg") &&
			!strings.HasSuffix(path, ".png") && !strings.HasSuffix(path, ".webp") {
			continue
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	return out
}

func adultFD2DurationMinutes(value string) int {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 3 {
		return 0
	}
	hours, errHours := strconv.Atoi(parts[0])
	minutes, errMinutes := strconv.Atoi(parts[1])
	seconds, errSeconds := strconv.Atoi(parts[2])
	if errHours != nil || errMinutes != nil || errSeconds != nil || hours < 0 || minutes < 0 || minutes >= 60 || seconds < 0 || seconds >= 60 {
		return 0
	}
	return (hours*3600 + minutes*60 + seconds) / 60
}

func adultFD2People(body, detailURL string) []PersonMetadata {
	people := make([]PersonMetadata, 0, 2)
	for _, found := range adultAnchorPattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 3 {
			continue
		}
		attrs := adultAttrs(found[1])
		class := " " + strings.ToLower(strings.Join(strings.Fields(attrs["class"]), " ")) + " "
		if !strings.Contains(class, " artisturl ") {
			continue
		}
		name := strings.TrimSpace(stripAdultHTML(found[2]))
		if !validAdultActorName(name) {
			continue
		}
		sourceID := strings.TrimSpace(attrs["data-actress"])
		profileURL := absolutizeURL(detailURL, attrs["href"])
		if sourceID == "" {
			sourceID = strings.TrimPrefix(strings.TrimSpace(attrs["href"]), "/actresses/")
		}
		people = append(people, PersonMetadata{
			Name:       name,
			ProfileURL: profileURL,
			Source:     "fd2ppv",
			SourceID:   sourceID,
		})
	}
	return deduplicatePersonMetadata(people)
}
