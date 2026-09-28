package service

import "regexp"

// A collection is not evidence of a single season. Do not let S01-S06
// manufacture a season-one conflict for a file explicitly marked S06E02.
var seasonCollectionPattern = regexp.MustCompile(`(?i)(?:\b(?:s|season)[ ._-]*\d{1,2}\s*[-~–—至到]\s*(?:(?:s|season)[ ._-]*)?\d{1,2}\b|第?[0-9一二三四五六七八九十百零两]+\s*[-~–—至到]\s*第?[0-9一二三四五六七八九十百零两]+季|全[0-9一二三四五六七八九十百零两]+季)`)
var adjacentEpisodePattern = regexp.MustCompile(`(?i)s\d{1,2}e\d{1,3}[ ._-]*e\d{1,3}`)
var dottedCombinedEpisodePattern = regexp.MustCompile(`(?i)\bse\d{1,2}[._]\d{1,3}[._]\d{1,3}(?:\D|$)`)
var fractionalEpisodePattern = regexp.MustCompile(`(?i)(?:第[0-9一二三四五六七八九十百零两]+季[ ._-]*|(?:s\d+)?e(?:p)?[ ._-]*|^)(\d{1,3})\.\d{1,2}(?:[^0-9]|$)`)
var splitEpisodePattern = regexp.MustCompile(`(?i)(?:第[0-9一二三四五六七八九十百零两]+季[ ._-]*|s\d+e|^)(\d{1,3})_(?:0[1-9])(?:_end)?$`)

func episodeStructureIssue(name string) string {
	if patSEnERange.MatchString(name) || patCNRange.MatchString(name) || adjacentEpisodePattern.MatchString(name) || dottedCombinedEpisodePattern.MatchString(name) {
		return "multi_episode_file"
	}
	if fractionalEpisodePattern.MatchString(name) {
		return "nonstandard_episode_number"
	}
	if splitEpisodePattern.MatchString(name) {
		return "split_episode_file"
	}
	if len(patSEnE.FindAllString(name, -1)) > 1 {
		return "multiple_episode_markers"
	}
	return ""
}
