package service

import (
	"strings"
	"time"
)

type embyArtworkCacheEntry struct {
	primary   string
	backdrop  string
	expiresAt time.Time
}

func (e *EmbyService) invalidateVirtualSeriesCache() {
	if e == nil {
		return
	}
	e.virtualMu.Lock()
	e.virtualArtwork = make(map[string]embyArtworkCacheEntry)
	e.virtualMu.Unlock()
}

// Keep only lightweight artwork URLs; detail records are always read by group key.
func (e *EmbyService) rememberSeriesCardArtwork(group embySeriesGroup) {
	if e == nil || strings.TrimSpace(group.ID) == "" {
		return
	}
	e.virtualMu.Lock()
	defer e.virtualMu.Unlock()
	if e.virtualArtwork == nil || len(e.virtualArtwork) > 7000 {
		e.virtualArtwork = make(map[string]embyArtworkCacheEntry)
	}
	entry := embyArtworkCacheEntry{primary: group.PosterURL, backdrop: group.BackdropURL, expiresAt: time.Now().Add(embyVirtualCacheTTL)}
	e.virtualArtwork[group.ID] = entry
	e.virtualArtwork[group.ID+"-bd"] = entry
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
