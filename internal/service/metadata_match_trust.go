package service

import (
	"regexp"
	"strings"
)

var (
	metadataTrustTokenRE           = regexp.MustCompile(`[\p{L}\p{N}]+`)
	metadataTrustDanglingEpisodeRE = regexp.MustCompile(`(?i)^s\d{1,2}e$`)
)

func automaticMetadataTitleTrusted(query string, match *Match) bool {
	queryKey := metadataTrustKey(query)
	if queryKey == "" || match == nil {
		return false
	}
	titles := make([]string, 0, 2+len(match.Aliases))
	titles = append(titles, match.Title, match.OriginalName)
	titles = append(titles, match.Aliases...)
	for _, title := range titles {
		titleKey := metadataTrustKey(title)
		if titleKey == "" {
			continue
		}
		if queryKey == titleKey || metadataTrustTokenOverlap(queryKey, titleKey) {
			return true
		}
	}
	if metadataTrustLocalizedSearchKeyword(queryKey, match) {
		return true
	}
	return false
}

func metadataTrustKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = bracketedTag.ReplaceAllString(value, " ")
	value = yearPattern.ReplaceAllString(value, " ")
	for _, re := range []*regexp.Regexp{patSEnE, patDanglingSE, patNxE, patEP, patCN, patSeasonOnly, patCNSeason} {
		value = re.ReplaceAllString(value, " ")
	}
	tokens := metadataTrustTokenRE.FindAllString(value, -1)
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" || metadataTrustNoiseToken(token) {
			continue
		}
		out = append(out, token)
	}
	return strings.Join(out, " ")
}

func metadataTrustNoiseToken(token string) bool {
	if token == "x" || token == "×" {
		return true
	}
	if _, ok := noiseTokenSet[token]; ok {
		return true
	}
	if metadataTrustDanglingEpisodeRE.MatchString(token) {
		return true
	}
	return false
}

func metadataTrustTokenOverlap(queryKey, titleKey string) bool {
	queryTokens := metadataTrustSignificantTokens(queryKey)
	titleTokens := metadataTrustSignificantTokens(titleKey)
	if len(queryTokens) <= 1 || len(titleTokens) <= 1 {
		return false
	}
	titleSet := make(map[string]struct{}, len(titleTokens))
	for _, token := range titleTokens {
		titleSet[token] = struct{}{}
	}
	overlap := 0
	for _, token := range queryTokens {
		if _, ok := titleSet[token]; ok {
			overlap++
		}
	}
	queryCoverage := float64(overlap) / float64(len(queryTokens))
	titleCoverage := float64(overlap) / float64(len(titleTokens))
	return queryCoverage >= 0.80 && titleCoverage >= 0.50
}

func metadataTrustLocalizedSearchKeyword(queryKey string, match *Match) bool {
	if match == nil || !metadataMatchHasExternalID(match) {
		return false
	}
	if !metadataTrustStrongCJKQuery(queryKey) {
		return false
	}
	for _, alias := range match.Aliases {
		if aliasKey := metadataTrustKey(alias); aliasKey != "" && aliasKey == queryKey {
			return true
		}
	}
	return false
}

func preferLocalizedSearchTitle(query string, match *Match) {
	if match == nil || containsCJK(match.Title) {
		return
	}
	if !metadataTrustLocalizedSearchKeyword(metadataTrustKey(query), match) {
		return
	}
	localized := strings.TrimSpace(match.SearchKeyword)
	if localized == "" {
		localized = strings.TrimSpace(query)
	}
	if localized == "" {
		return
	}
	if strings.TrimSpace(match.OriginalName) == "" {
		match.OriginalName = strings.TrimSpace(match.Title)
	}
	match.Title = localized
}

func metadataTrustStrongCJKQuery(queryKey string) bool {
	cjk := 0
	for _, r := range queryKey {
		switch {
		case r >= '\u3400' && r <= '\u4dbf':
			cjk++
		case r >= '\u4e00' && r <= '\u9fff':
			cjk++
		case r >= '\uf900' && r <= '\ufaff':
			cjk++
		}
	}
	return cjk >= 4
}

func metadataMatchHasExternalID(match *Match) bool {
	return match != nil &&
		(match.TMDbID > 0 ||
			match.BangumiID > 0 ||
			strings.TrimSpace(match.DoubanID) != "" ||
			strings.TrimSpace(match.TheTVDBID) != "")
}

func metadataTrustSignificantTokens(key string) []string {
	fields := strings.Fields(key)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "the", "a", "an", "of", "and":
			continue
		default:
			out = append(out, field)
		}
	}
	return out
}
