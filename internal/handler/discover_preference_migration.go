package handler

// Insert release rails beside the corresponding enabled Chinese popular rail.
// Preserve all existing positions, including release rails already chosen by
// the user; a custom list without Chinese movies/TV and an empty list stay put.
func addChineseReleaseSections(selected []string) []string {
	existing := make(map[string]bool, len(selected))
	for _, key := range selected {
		existing[key] = true
	}
	out := make([]string, 0, len(selected)+4)
	for _, key := range selected {
		out = append(out, key)
		var additions []string
		switch key {
		case "tmdb_chinese_movie":
			additions = []string{"tmdb_chinese_latest_movie", "tmdb_chinese_upcoming_movie"}
		case "tmdb_chinese_tv":
			additions = []string{"tmdb_chinese_latest_tv", "tmdb_chinese_upcoming_tv"}
		}
		for _, addition := range additions {
			if !existing[addition] {
				out = append(out, addition)
				existing[addition] = true
			}
		}
	}
	return out
}
