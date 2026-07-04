package service

import (
	"strings"
	"time"
)

type embySeriesCacheEntry struct {
	group     embySeriesGroup
	expiresAt time.Time
}

type embySeasonCacheEntry struct {
	season    embySeasonGroup
	expiresAt time.Time
}

type embyArtworkCacheEntry struct {
	primary   string
	backdrop  string
	expiresAt time.Time
}

func (e *EmbyService) rememberSeriesGroup(group embySeriesGroup) {
	if e == nil || strings.TrimSpace(group.ID) == "" {
		return
	}
	expiresAt := time.Now().Add(embyVirtualCacheTTL)
	e.virtualMu.Lock()
	defer e.virtualMu.Unlock()
	if e.virtualSeries == nil {
		e.virtualSeries = make(map[string]embySeriesCacheEntry)
	}
	if e.virtualSeasons == nil {
		e.virtualSeasons = make(map[string]embySeasonCacheEntry)
	}
	if e.virtualArtwork == nil {
		e.virtualArtwork = make(map[string]embyArtworkCacheEntry)
	}
	if len(e.virtualSeries) > 2000 || len(e.virtualSeasons) > 5000 || len(e.virtualArtwork) > 7000 {
		e.virtualSeries = make(map[string]embySeriesCacheEntry)
		e.virtualSeasons = make(map[string]embySeasonCacheEntry)
		e.virtualArtwork = make(map[string]embyArtworkCacheEntry)
	}
	e.virtualSeries[group.ID] = embySeriesCacheEntry{group: group, expiresAt: expiresAt}
	e.virtualArtwork[group.ID] = embyArtworkCacheEntry{primary: group.PosterURL, backdrop: group.BackdropURL, expiresAt: expiresAt}
	e.virtualArtwork[group.ID+"-bd"] = embyArtworkCacheEntry{primary: group.PosterURL, backdrop: group.BackdropURL, expiresAt: expiresAt}
	for _, season := range e.seasonsForSeries(group) {
		e.virtualSeasons[season.ID] = embySeasonCacheEntry{season: season, expiresAt: expiresAt}
		e.virtualArtwork[season.ID] = embyArtworkCacheEntry{primary: season.Series.PosterURL, backdrop: season.Series.BackdropURL, expiresAt: expiresAt}
		e.virtualArtwork[season.ID+"-bd"] = embyArtworkCacheEntry{primary: season.Series.PosterURL, backdrop: season.Series.BackdropURL, expiresAt: expiresAt}
	}
}

func (e *EmbyService) rememberSeasonGroup(season embySeasonGroup) {
	if e == nil || strings.TrimSpace(season.ID) == "" {
		return
	}
	expiresAt := time.Now().Add(embyVirtualCacheTTL)
	e.virtualMu.Lock()
	defer e.virtualMu.Unlock()
	if e.virtualSeasons == nil {
		e.virtualSeasons = make(map[string]embySeasonCacheEntry)
	}
	if e.virtualArtwork == nil {
		e.virtualArtwork = make(map[string]embyArtworkCacheEntry)
	}
	e.virtualSeasons[season.ID] = embySeasonCacheEntry{season: season, expiresAt: expiresAt}
	e.virtualArtwork[season.ID] = embyArtworkCacheEntry{primary: season.Series.PosterURL, backdrop: season.Series.BackdropURL, expiresAt: expiresAt}
	e.virtualArtwork[season.ID+"-bd"] = embyArtworkCacheEntry{primary: season.Series.PosterURL, backdrop: season.Series.BackdropURL, expiresAt: expiresAt}
}

func (e *EmbyService) cachedSeriesGroup(id string) (embySeriesGroup, bool) {
	if e == nil || strings.TrimSpace(id) == "" {
		return embySeriesGroup{}, false
	}
	now := time.Now()
	e.virtualMu.RLock()
	entry, ok := e.virtualSeries[id]
	e.virtualMu.RUnlock()
	if !ok || now.After(entry.expiresAt) {
		if ok {
			e.virtualMu.Lock()
			delete(e.virtualSeries, id)
			e.virtualMu.Unlock()
		}
		return embySeriesGroup{}, false
	}
	return entry.group, true
}

func (e *EmbyService) cachedSeasonGroup(id string) (embySeasonGroup, bool) {
	if e == nil || strings.TrimSpace(id) == "" {
		return embySeasonGroup{}, false
	}
	now := time.Now()
	e.virtualMu.RLock()
	entry, ok := e.virtualSeasons[id]
	e.virtualMu.RUnlock()
	if !ok || now.After(entry.expiresAt) {
		if ok {
			e.virtualMu.Lock()
			delete(e.virtualSeasons, id)
			e.virtualMu.Unlock()
		}
		return embySeasonGroup{}, false
	}
	return entry.season, true
}

func (e *EmbyService) cachedArtworkURL(id, imageType string) (string, bool) {
	if e == nil || strings.TrimSpace(id) == "" {
		return "", false
	}
	now := time.Now()
	e.virtualMu.RLock()
	entry, ok := e.virtualArtwork[id]
	e.virtualMu.RUnlock()
	if !ok || now.After(entry.expiresAt) {
		if ok {
			e.virtualMu.Lock()
			delete(e.virtualArtwork, id)
			e.virtualMu.Unlock()
		}
		return "", false
	}
	switch strings.ToLower(imageType) {
	case "backdrop", "art":
		if entry.backdrop != "" {
			return entry.backdrop, true
		}
	}
	if entry.primary != "" {
		return entry.primary, true
	}
	return entry.backdrop, entry.backdrop != ""
}
