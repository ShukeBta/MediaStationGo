package service

import "strings"

type tmdbAlternativeTitle struct {
	Country string `json:"iso_3166_1"`
	Title   string `json:"title"`
}

type tmdbTranslation struct {
	Country  string `json:"iso_3166_1"`
	Language string `json:"iso_639_1"`
	Data     struct {
		Title string `json:"title"`
		Name  string `json:"name"`
	} `json:"data"`
}

func applyTMDbChineseTitle(match *Match, alternatives []tmdbAlternativeTitle, translations []tmdbTranslation) {
	if match == nil {
		return
	}
	aliases := make([]string, 0, 2+len(alternatives)+len(translations))
	aliases = append(aliases, match.Title, match.OriginalName)
	for _, alternative := range alternatives {
		aliases = append(aliases, alternative.Title)
	}
	for _, translation := range translations {
		aliases = append(aliases, firstNonEmpty(translation.Data.Title, translation.Data.Name))
	}
	match.Aliases = appendMetadataAliases(match.Aliases, aliases...)

	if !metadataTitleNeedsChineseLocalization(match) {
		return
	}
	localized := preferredTMDbChineseTitle(alternatives, translations)
	if localized == "" {
		return
	}
	if strings.TrimSpace(match.OriginalName) == "" {
		match.OriginalName = strings.TrimSpace(match.Title)
	}
	match.Title = localized
}

func preferredTMDbChineseTitle(alternatives []tmdbAlternativeTitle, translations []tmdbTranslation) string {
	for _, country := range []string{"CN", "SG", "HK", "TW"} {
		for _, alternative := range alternatives {
			if strings.EqualFold(strings.TrimSpace(alternative.Country), country) && chineseTitleCandidate(alternative.Title) {
				return strings.TrimSpace(alternative.Title)
			}
		}
		for _, translation := range translations {
			if !strings.EqualFold(strings.TrimSpace(translation.Language), "zh") ||
				!strings.EqualFold(strings.TrimSpace(translation.Country), country) {
				continue
			}
			if title := strings.TrimSpace(firstNonEmpty(translation.Data.Title, translation.Data.Name)); chineseTitleCandidate(title) {
				return title
			}
		}
	}
	return ""
}

func metadataTitleNeedsChineseLocalization(match *Match) bool {
	if match == nil || strings.TrimSpace(match.Title) == "" {
		return true
	}
	if !containsCJK(match.Title) {
		return true
	}
	for _, r := range match.Title {
		if (r >= '\u3040' && r <= '\u30ff') || (r >= '\uac00' && r <= '\ud7af') {
			return true
		}
	}
	return false
}

func chineseTitleCandidate(title string) bool {
	title = strings.TrimSpace(title)
	return title != "" && containsCJK(title)
}
