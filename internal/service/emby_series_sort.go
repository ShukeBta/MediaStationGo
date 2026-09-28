package service

import (
	"cmp"
	"sort"
	"strings"
)

type embySeriesSortTerm struct {
	field      string
	descending bool
}

func embySeriesSortTerms(p ItemsParams) []embySeriesSortTerm {
	directions := strings.Split(p.SortOrder, ",")
	terms := []embySeriesSortTerm{}
	seen := map[string]bool{}
	for i, raw := range strings.Split(p.SortBy, ",") {
		field := strings.ToLower(strings.TrimSpace(raw))
		switch field {
		case "name", "sortname":
			field = "name"
		case "datecreated", "datelastcontentadded":
			field = "created"
		case "premieredate":
			field = "premiere"
		case "productionyear":
			field = "year"
		case "communityrating":
			field = "rating"
		default:
			continue
		}
		if seen[field] {
			continue
		}
		seen[field] = true
		direction := directions[0]
		if i < len(directions) {
			direction = directions[i]
		}
		terms = append(terms, embySeriesSortTerm{field, strings.EqualFold(strings.TrimSpace(direction), "Descending")})
	}
	if len(terms) == 0 {
		terms = append(terms, embySeriesSortTerm{"name", strings.EqualFold(strings.TrimSpace(directions[0]), "Descending")})
		seen["name"] = true
	}
	if !seen["name"] {
		terms = append(terms, embySeriesSortTerm{"name", false})
	}
	return terms
}

// Missing values form a name-sorted tail, never compare a timestamp against
// a name pairwise (that would violate transitivity and destabilize pagination).
func compareEmbySeries(a, b embySeriesGroup, terms []embySeriesSortTerm) int {
	for _, term := range terms {
		order := 0
		missingA, missingB := false, false
		switch term.field {
		case "name":
			order = strings.Compare(a.Name, b.Name)
		case "created":
			missingA, missingB = a.CreatedAt.IsZero(), b.CreatedAt.IsZero()
			order = a.CreatedAt.Compare(b.CreatedAt)
		case "premiere":
			ad, av := embyPremiereDate(a.ReleaseDate)
			bd, bv := embyPremiereDate(b.ReleaseDate)
			missingA, missingB = !av, !bv
			order = ad.Compare(bd)
		case "year":
			missingA, missingB = a.Year <= 0, b.Year <= 0
			order = cmp.Compare(a.Year, b.Year)
		case "rating":
			missingA, missingB = a.Rating <= 0, b.Rating <= 0
			order = cmp.Compare(a.Rating, b.Rating)
		}
		if missingA != missingB {
			if missingA {
				return 1
			}
			return -1
		}
		if missingA && missingB {
			if nameOrder := strings.Compare(a.Name, b.Name); nameOrder != 0 {
				return nameOrder
			}
			return strings.Compare(a.ID, b.ID)
		}
		if order != 0 {
			if term.descending {
				return -order
			}
			return order
		}
	}
	return strings.Compare(a.ID, b.ID)
}

func sortSeriesGroupsByClient(groups []embySeriesGroup, p ItemsParams) {
	terms := embySeriesSortTerms(p)
	sort.Slice(groups, func(i, j int) bool { return compareEmbySeries(groups[i], groups[j], terms) < 0 })
}
