package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

type embyItemsCacheValue struct {
	Items            []map[string]any `json:"items"`
	TotalRecordCount int64            `json:"total_record_count"`
	StartIndex       int              `json:"start_index"`
}

type embyLatestCacheValue struct {
	Items []map[string]any `json:"items"`
}

type embyReadCacheFlight struct {
	done chan struct{}
}

// embyItemsCacheSchemaVersion changes whenever an Items page's public-card
// semantics change. It prevents a Redis-enabled deployment from returning a
// page cached with an older pagination contract after an application upgrade.
const embyItemsCacheSchemaVersion = "v2"

func (e *EmbyService) embyItemsCacheKey(kind string, p ItemsParams) string {
	includeTypes := append([]string(nil), p.IncludeItemTypes...)
	filters := append([]string(nil), p.Filters...)
	ids := append([]string(nil), p.IDs...)
	personIDs := append([]string(nil), p.PersonIDs...)
	genreIDs := append([]string(nil), p.GenreIDs...)
	genres := append([]string(nil), p.Genres...)
	sort.Strings(includeTypes)
	sort.Strings(filters)
	sort.Strings(ids)
	sort.Strings(personIDs)
	sort.Strings(genreIDs)
	sort.Strings(genres)
	sum := sha256.Sum256([]byte(strings.Join([]string{
		kind,
		embyItemsCacheSchemaVersion,
		p.UserID,
		strconv.FormatUint(e.userVisibilityVersion(p.UserID), 10),
		p.ParentID,
		strings.Join(ids, ","),
		strings.Join(personIDs, ","),
		strings.Join(genreIDs, ","),
		strings.Join(genres, ","),
		p.SearchTerm,
		p.NameStartsWith,
		strings.Join(includeTypes, ","),
		strings.Join(filters, ","),
		strconv.FormatBool(p.Recursive),
		p.SortBy,
		p.SortOrder,
		strconv.Itoa(p.StartIndex),
		strconv.Itoa(p.Limit),
		strconv.FormatBool(p.OmitMediaSources),
	}, "|")))
	return "media:emby:" + hex.EncodeToString(sum[:])
}

func (e *EmbyService) embyLatestCacheKey(userID, parentID string, limit int) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"latest",
		userID,
		strconv.FormatUint(e.userVisibilityVersion(userID), 10),
		parentID,
		strconv.Itoa(limit),
	}, "|")))
	return "media:emby:" + hex.EncodeToString(sum[:])
}

func (e *EmbyService) beginEmbyReadCacheFill(key string) (*embyReadCacheFlight, bool) {
	if e == nil || strings.TrimSpace(key) == "" {
		return nil, true
	}
	e.readCacheMu.Lock()
	defer e.readCacheMu.Unlock()
	if e.readCacheInFlight == nil {
		e.readCacheInFlight = map[string]*embyReadCacheFlight{}
	}
	if call, ok := e.readCacheInFlight[key]; ok {
		return call, false
	}
	call := &embyReadCacheFlight{done: make(chan struct{})}
	e.readCacheInFlight[key] = call
	return call, true
}

func (e *EmbyService) finishEmbyReadCacheFill(key string, call *embyReadCacheFlight) {
	if e == nil || call == nil {
		return
	}
	shouldClose := false
	e.readCacheMu.Lock()
	if e.readCacheInFlight != nil && e.readCacheInFlight[key] == call {
		delete(e.readCacheInFlight, key)
		shouldClose = true
	}
	e.readCacheMu.Unlock()
	if shouldClose {
		close(call.done)
	}
}

func waitEmbyReadCacheFill(ctx context.Context, call *embyReadCacheFlight) error {
	if call == nil {
		return nil
	}
	select {
	case <-call.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *EmbyService) mediaCacheTTLSeconds() int {
	if e == nil || e.cfg == nil || e.cfg.Cache.MediaTTLSeconds < 1 {
		return 15
	}
	return e.cfg.Cache.MediaTTLSeconds
}

func (e *EmbyService) embyMediaCacheTTL() time.Duration {
	return time.Duration(e.mediaCacheTTLSeconds()) * time.Second
}
