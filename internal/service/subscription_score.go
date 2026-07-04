package service

import (
	"regexp"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var (
	dolbyVisionTokenRE = regexp.MustCompile(`\bdv\b`)
	webDLTokenRE       = regexp.MustCompile(`\bweb[\s._-]?dl\b`)
	webRipTokenRE      = regexp.MustCompile(`\bweb[\s._-]?rip\b`)
	bluRayTokenRE      = regexp.MustCompile(`\b(?:blu[\s._-]?ray|bdrip|bdremux|uhd[\s._-]?blu[\s._-]?ray)\b`)
)

const (
	defaultSubscriptionFreePromotionScore = 50_000
	washSubscriptionFreePromotionScore    = 25
)

func subscriptionCandidateScore(sub *model.Subscription, item SearchResult) int {
	title := strings.ToLower(subscriptionSearchResultText(item))
	score := item.Seeders
	if !subscriptionAllowsWash(sub) {
		score += detectDefaultSubscriptionQualityScore(title)*1_000_000 + detectResolutionScore(title)*100_000
		if item.Free {
			score += defaultSubscriptionFreePromotionScore
		}
		return score
	}
	resolutionScore := detectResolutionScore(title)
	qualityScore := detectQualityScore(title)
	effectScore := detectEffectScore(title)

	priority := "balanced"
	if sub != nil && strings.TrimSpace(sub.WashPriority) != "" {
		priority = strings.ToLower(strings.TrimSpace(sub.WashPriority))
	}
	switch priority {
	case "resolution":
		score += resolutionScore*1000 + qualityScore*100 + effectScore*50
	case "quality":
		score += qualityScore*1000 + resolutionScore*200 + effectScore*50
	case "effects":
		score += effectScore*1000 + resolutionScore*200 + qualityScore*100
	case "seeders":
		score += qualityScore*3 + resolutionScore*2 + effectScore
	default:
		score += resolutionScore*500 + qualityScore*300 + effectScore*150
	}
	if item.Free {
		score += washSubscriptionFreePromotionScore
	}
	return score
}

func containsAnyEffect(titleFold, csv string) bool {
	for _, token := range strings.FieldsFunc(strings.ToLower(csv), func(r rune) bool {
		return r == ',' || r == '/' || r == '|' || r == ';' || r == '，'
	}) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		switch token {
		case "dolby-vision", "dolby vision", "dv":
			if strings.Contains(titleFold, "dolby vision") || strings.Contains(titleFold, "dovi") || dolbyVisionTokenRE.MatchString(titleFold) {
				return true
			}
		default:
			if strings.Contains(titleFold, token) {
				return true
			}
		}
	}
	return false
}

func titleMatchesResolution(titleFold, resolution string) bool {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "2160p", "4k", "uhd":
		return strings.Contains(titleFold, "2160p") || strings.Contains(titleFold, "4k") || strings.Contains(titleFold, "uhd")
	case "1080p":
		return strings.Contains(titleFold, "1080p") || strings.Contains(titleFold, "fhd")
	case "720p":
		return strings.Contains(titleFold, "720p")
	default:
		return strings.Contains(titleFold, strings.ToLower(strings.TrimSpace(resolution)))
	}
}

func titleMatchesQuality(titleFold, quality string) bool {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "webdl", "web-dl":
		return webDLTokenRE.MatchString(titleFold)
	case "webrip", "web-rip":
		return webRipTokenRE.MatchString(titleFold)
	case "bluray", "blu-ray":
		return bluRayTokenRE.MatchString(titleFold)
	case "remux":
		return strings.Contains(titleFold, "remux")
	case "hdtv":
		return strings.Contains(titleFold, "hdtv")
	default:
		return strings.Contains(titleFold, strings.ToLower(strings.TrimSpace(quality)))
	}
}

func detectDefaultSubscriptionQualityScore(titleFold string) int {
	switch {
	case titleMatchesQuality(titleFold, "web-dl"):
		return 5
	case titleMatchesQuality(titleFold, "web-rip"):
		return 4
	case titleMatchesQuality(titleFold, "bluray"), titleMatchesQuality(titleFold, "remux"):
		return 3
	case titleMatchesQuality(titleFold, "hdtv"):
		return 2
	default:
		return 1
	}
}

func detectResolutionScore(titleFold string) int {
	switch {
	case titleMatchesResolution(titleFold, "2160p"):
		return 4
	case titleMatchesResolution(titleFold, "1080p"):
		return 3
	case titleMatchesResolution(titleFold, "720p"):
		return 2
	default:
		return 1
	}
}

func detectQualityScore(titleFold string) int {
	switch {
	case titleMatchesQuality(titleFold, "remux"):
		return 5
	case titleMatchesQuality(titleFold, "bluray"):
		return 4
	case titleMatchesQuality(titleFold, "web-dl"):
		return 3
	case titleMatchesQuality(titleFold, "hdtv"):
		return 2
	default:
		return 1
	}
}

func detectEffectScore(titleFold string) int {
	score := 0
	if containsAnyEffect(titleFold, "dolby-vision") {
		score += 4
	}
	if strings.Contains(titleFold, "hdr10+") {
		score += 3
	} else if strings.Contains(titleFold, "hdr") {
		score += 2
	}
	if strings.Contains(titleFold, "atmos") {
		score += 2
	}
	return score
}
