package service

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (e *EmbyService) applyUserMediaVisibility(ctx context.Context, q *gorm.DB, userID string) *gorm.DB {
	visibility := e.mediaVisibility(ctx, userID)
	if !visibility.IncludeNSFW {
		q = q.Where("nsfw = ?", false)
		if hidden := visibility.HiddenLibraryIDs; len(hidden) > 0 {
			q = q.Where("library_id NOT IN ?", hidden)
		}
	}
	if len(visibility.AllowedLibraryIDs) > 0 {
		q = q.Where("library_id IN ?", visibility.AllowedLibraryIDs)
	}
	return q
}

func (e *EmbyService) filterMediaRowsForUser(ctx context.Context, rows []model.Media, userID string) []model.Media {
	visibility := e.mediaVisibility(ctx, userID)
	if visibility.IncludeNSFW && len(visibility.AllowedLibraryIDs) == 0 {
		return rows
	}
	allowed := map[string]bool{}
	for _, id := range visibility.AllowedLibraryIDs {
		allowed[id] = true
	}
	hiddenLibraries := map[string]bool{}
	for _, id := range visibility.HiddenLibraryIDs {
		hiddenLibraries[id] = true
	}
	out := rows[:0]
	for _, row := range rows {
		if row.NSFW && !visibility.IncludeNSFW {
			continue
		}
		if hiddenLibraries[row.LibraryID] {
			continue
		}
		if len(allowed) > 0 && !allowed[row.LibraryID] {
			continue
		}
		out = append(out, row)
	}
	return out
}

func (e *EmbyService) mediaVisibility(ctx context.Context, userID string) MediaVisibility {
	if e == nil {
		return MediaVisibility{IncludeNSFW: true}
	}
	key := strings.TrimSpace(userID)
	now := time.Now()
	e.visibilityMu.RLock()
	entry, ok := e.visibilityCache[key]
	e.visibilityMu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return cloneMediaVisibility(entry.visibility)
	}

	visibility := UserDefaultMediaVisibility(ctx, e.repo, userID)
	if !visibility.IncludeNSFW {
		visibility.HiddenLibraryIDs = e.hiddenLibraryIDs(ctx, visibility)
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, e.repo, visibility)
	visibility = cloneMediaVisibility(visibility)

	e.visibilityMu.Lock()
	if e.visibilityCache == nil {
		e.visibilityCache = make(map[string]embyVisibilityCacheEntry)
	}
	if len(e.visibilityCache) > 1000 {
		e.visibilityCache = make(map[string]embyVisibilityCacheEntry)
	}
	e.visibilityCache[key] = embyVisibilityCacheEntry{
		visibility: cloneMediaVisibility(visibility),
		expiresAt:  now.Add(embyVisibilityCacheTTL),
	}
	e.visibilityMu.Unlock()

	return visibility
}

func (e *EmbyService) mergedLibraryIDs(ctx context.Context, libraryID string) []string {
	ids, err := MergedLibraryIDsForLibrary(ctx, e.repo, libraryID)
	if err != nil || len(ids) == 0 {
		return []string{libraryID}
	}
	return ids
}

func cloneMediaVisibility(visibility MediaVisibility) MediaVisibility {
	if visibility.AllowedLibraryIDs != nil {
		visibility.AllowedLibraryIDs = append([]string(nil), visibility.AllowedLibraryIDs...)
	}
	if visibility.HiddenLibraryIDs != nil {
		visibility.HiddenLibraryIDs = append([]string(nil), visibility.HiddenLibraryIDs...)
	}
	return visibility
}

func (e *EmbyService) libraryVisibleFromCachedVisibility(lib model.Library, visibility MediaVisibility) bool {
	if len(visibility.AllowedLibraryIDs) > 0 {
		allowed := false
		for _, id := range visibility.AllowedLibraryIDs {
			if id == lib.ID {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	if visibility.IncludeNSFW {
		return true
	}
	for _, id := range visibility.HiddenLibraryIDs {
		if id == lib.ID {
			return false
		}
	}
	return true
}

func (e *EmbyService) hiddenLibraryIDs(ctx context.Context, visibility MediaVisibility) []string {
	if visibility.IncludeNSFW {
		return nil
	}
	libs, err := e.repo.Library.List(ctx)
	if err != nil {
		return nil
	}
	shadowed := ShadowedCloudLibraryIDSet(libs)
	ids := make([]string, 0)
	for _, lib := range libs {
		if shadowed[lib.ID] || !LibraryVisibleForUser(ctx, e.repo, lib, visibility) {
			ids = append(ids, lib.ID)
		}
	}
	return ids
}
