package service

import (
	"strings"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type embyMediaSearch struct {
	term       string
	startsWith bool
}

func embySearchFromParams(p ItemsParams) embyMediaSearch {
	if term := strings.ToLower(strings.TrimSpace(p.SearchTerm)); term != "" {
		return embyMediaSearch{term: term}
	}
	if term := strings.ToLower(strings.TrimSpace(p.NameStartsWith)); term != "" {
		return embyMediaSearch{term: term, startsWith: true}
	}
	return embyMediaSearch{}
}

func embyHasMediaSearch(p ItemsParams) bool {
	return embySearchFromParams(p).term != ""
}

func applyEmbyMediaSearch(q *gorm.DB, p ItemsParams) *gorm.DB {
	search := embySearchFromParams(p)
	if search.term == "" {
		return q
	}
	if search.startsWith {
		pattern := search.term + "%"
		return q.Where("(LOWER(media.title) LIKE ? OR LOWER(media.original_name) LIKE ?)", pattern, pattern)
	}
	pattern := "%" + search.term + "%"
	return q.Where(
		"(LOWER(media.title) LIKE ? OR LOWER(media.original_name) LIKE ? OR LOWER(media.path) LIKE ? OR LOWER(media.relative_path) LIKE ?)",
		pattern, pattern, pattern, pattern,
	)
}

func embyMediaMatchesSearch(row model.Media, p ItemsParams) bool {
	search := embySearchFromParams(p)
	if search.term == "" {
		return true
	}
	values := []string{
		strings.ToLower(row.Title),
		strings.ToLower(row.OriginalName),
	}
	if search.startsWith {
		for _, value := range values {
			if strings.HasPrefix(value, search.term) {
				return true
			}
		}
		return false
	}
	values = append(values, strings.ToLower(row.Path), strings.ToLower(row.RelativePath))
	for _, value := range values {
		if strings.Contains(value, search.term) {
			return true
		}
	}
	return false
}
