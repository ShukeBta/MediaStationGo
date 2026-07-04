package service

import (
	"regexp"
	"strings"
)

var (
	seriesPackRE = regexp.MustCompile(`(?i)(complete|batch|合集|全集|全\s*\d+\s*[集话話期]|整季|全季|s\d{1,2}\s*(?:complete|batch|pack)|season\s*\d{1,2}\s*(?:complete|batch|pack)|s\d{1,2}e\d{1,3}\s*[-~–—]\s*(?:s\d{1,2})?e?\d{1,3}|第\s*\d+\s*[-~–—]\s*\d+\s*[集话話期])`)
	seasonOnlyRE = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:s|season)\s*\d{1,2}(?:[\s._-]|$)|第\s*\d+\s*季`)
)

func isSeriesPackTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	if seriesPackRE.MatchString(title) {
		return true
	}
	_, episode := ParseEpisode(title)
	return episode == 0 && seasonOnlyRE.MatchString(title)
}
