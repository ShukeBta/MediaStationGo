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
	if visibility, ok := ctx.Value(peopleVisibilityKey{}).(MediaVisibility); ok {
		return cloneMediaVisibility(visibility)
	}
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
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, e.repo, visibility)
	if !visibility.IncludeNSFW {
		visibility.HiddenLibraryIDs = e.hiddenLibraryIDs(ctx, visibility)
	}
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

// InvalidateUserVisibility makes administrator access changes effective on
// the next Emby-compatible request instead of waiting for the short cache TTL.
func (e *EmbyService) InvalidateUserVisibility(userID string) {
	if e == nil {
		return
	}
	e.visibilityMu.Lock()
	key := strings.TrimSpace(userID)
	delete(e.visibilityCache, key)
	if e.visibilityVersion == nil {
		e.visibilityVersion = make(map[string]uint64)
	}
	e.visibilityVersion[key]++
	e.visibilityMu.Unlock()
}

func (e *EmbyService) userVisibilityVersion(userID string) uint64 {
	if e == nil {
		return 0
	}
	e.visibilityMu.RLock()
	version := e.visibilityVersion[strings.TrimSpace(userID)]
	e.visibilityMu.RUnlock()
	return version
}

func (e *EmbyService) mergedLibraryIDs(ctx context.Context, libraryID string) []string {
	// collapseMediaVersionRows 等路径按媒体行调用这里，而底层要做
	// FindByID + 全量 Library.List，大库一次列表就是上万次查询；结果
	// 只随库配置变化，缓存两分钟。
	if ids, ok := e.cachedMergedLibraryIDs(libraryID); ok {
		return ids
	}
	ids, err := MergedLibraryIDsForLibrary(ctx, e.repo, libraryID)
	if err != nil || len(ids) == 0 {
		return []string{libraryID}
	}
	e.storeMergedLibraryIDs(libraryID, ids)
	return ids
}

func (e *EmbyService) cachedMergedLibraryIDs(libraryID string) ([]string, bool) {
	e.mergedIDsMu.RLock()
	defer e.mergedIDsMu.RUnlock()
	entry, ok := e.mergedIDsCache[libraryID]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return append([]string(nil), entry.ids...), true
}

func (e *EmbyService) storeMergedLibraryIDs(libraryID string, ids []string) {
	e.mergedIDsMu.Lock()
	defer e.mergedIDsMu.Unlock()
	if e.mergedIDsCache == nil {
		e.mergedIDsCache = make(map[string]embyMergedIDsCacheEntry)
	}
	if len(e.mergedIDsCache) > 1000 {
		e.mergedIDsCache = make(map[string]embyMergedIDsCacheEntry)
	}
	e.mergedIDsCache[libraryID] = embyMergedIDsCacheEntry{ids: append([]string(nil), ids...), expires: time.Now().Add(embyLibraryShapeCacheTTL)}
}

type embyMergedIDsCacheEntry struct {
	ids     []string
	expires time.Time
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
	if !lib.Enabled {
		return false
	}
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
	var libs []model.Library
	if snapshot, ok := embyLibrarySnapshotFromContext(ctx); ok {
		libs = snapshot.libraries
	} else {
		var err error
		libs, err = e.repo.Library.List(ctx)
		if err != nil {
			return nil
		}
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
