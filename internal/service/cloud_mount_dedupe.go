package service

import (
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func betterDisplayCloudLibrary(candidate, current model.Library, counts map[string]int64) bool {
	candidateCount := counts[candidate.ID]
	currentCount := counts[current.ID]
	if (candidateCount > 0) != (currentCount > 0) {
		return candidateCount > 0
	}
	if candidate.Enabled != current.Enabled {
		return candidate.Enabled
	}
	candidateCanonical := cloudLibraryPathIsCanonical(candidate)
	currentCanonical := cloudLibraryPathIsCanonical(current)
	if candidateCanonical != currentCanonical {
		return candidateCanonical
	}
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.After(current.CreatedAt)
	}
	return candidate.ID > current.ID
}

func mergeDisplayCloudLibraries(libs []model.Library) []model.Library {
	if len(libs) == 0 {
		return libs
	}
	localByKey := make(map[string]struct{}, len(libs))
	for _, lib := range libs {
		if _, ok := ParseCloudLibraryMount(lib.Path); ok || !lib.Enabled {
			continue
		}
		if key, ok := CloudLibraryMergeKey(lib); ok {
			localByKey[key] = struct{}{}
		}
	}
	out := make([]model.Library, 0, len(libs))
	for _, lib := range libs {
		if displayName, ok := CloudLibraryDisplayName(lib); ok && displayName != "" {
			lib.Name = displayName
			if key, ok := CloudLibraryMergeKey(lib); ok {
				if _, exists := localByKey[key]; exists && !CloudLibraryAutoCategory(lib) {
					continue
				}
			}
		} else if displayName := CanonicalLibraryDisplayName(lib); displayName != "" {
			lib.Name = displayName
		}
		out = append(out, lib)
	}
	return out
}

func dedupeDisplayLibrariesByMergeKey(libs []model.Library, counts map[string]int64) []model.Library {
	if len(libs) == 0 {
		return libs
	}
	out := make([]model.Library, 0, len(libs))
	byKey := make(map[string]int, len(libs))
	for _, lib := range libs {
		if displayName := CanonicalLibraryDisplayName(lib); displayName != "" {
			lib.Name = displayName
		}
		if CloudLibraryAutoCategory(lib) {
			out = append(out, lib)
			continue
		}
		key, ok := CloudLibraryMergeKey(lib)
		if !ok {
			out = append(out, lib)
			continue
		}
		if prev, exists := byKey[key]; exists {
			if betterCanonicalDisplayLibrary(lib, out[prev], counts) {
				out[prev] = lib
			}
			continue
		}
		byKey[key] = len(out)
		out = append(out, lib)
	}
	return out
}

func betterCanonicalDisplayLibrary(candidate, current model.Library, counts map[string]int64) bool {
	candidateScore := canonicalDisplayLibraryScore(candidate)
	currentScore := canonicalDisplayLibraryScore(current)
	if candidateScore != currentScore {
		return candidateScore > currentScore
	}
	candidateCount := counts[candidate.ID]
	currentCount := counts[current.ID]
	if (candidateCount > 0) != (currentCount > 0) {
		return candidateCount > 0
	}
	if candidate.Enabled != current.Enabled {
		return candidate.Enabled
	}
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.After(current.CreatedAt)
	}
	return candidate.ID > current.ID
}

func canonicalDisplayLibraryScore(lib model.Library) int {
	score := 0
	if canonical := CanonicalLibraryDisplayName(lib); canonical == "" || strings.EqualFold(strings.TrimSpace(lib.Name), canonical) {
		score += 4
	}
	if canonical := canonicalLibraryCategoryName(lib.Type, pathBaseSlash(lib.Path)); canonical == "" {
		score += 2
	}
	if _, ok := ParseCloudLibraryMount(lib.Path); !ok {
		score++
	}
	return score
}
