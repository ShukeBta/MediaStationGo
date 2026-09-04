package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type embyLibrarySnapshotContextKey struct{}

type embyLibrarySnapshot struct {
	libraries []model.Library
	byID      map[string]int
}

// withEmbyLibrarySnapshot keeps library classification and cloud-library
// merging request-local. Large Emby lists serialize many media rows; loading
// the same library for every row creates an avoidable N+1 query storm.
func (e *EmbyService) withEmbyLibrarySnapshot(ctx context.Context) (context.Context, error) {
	if _, ok := embyLibrarySnapshotFromContext(ctx); ok {
		return ctx, nil
	}
	if e == nil || e.repo == nil || e.repo.Library == nil {
		return ctx, nil
	}
	libraries, err := e.repo.Library.List(ctx)
	if err != nil {
		return ctx, err
	}
	snapshot := &embyLibrarySnapshot{
		libraries: libraries,
		byID:      make(map[string]int, len(libraries)),
	}
	for i := range libraries {
		if id := strings.TrimSpace(libraries[i].ID); id != "" {
			snapshot.byID[id] = i
		}
	}
	return context.WithValue(ctx, embyLibrarySnapshotContextKey{}, snapshot), nil
}

func embyLibrarySnapshotFromContext(ctx context.Context) (*embyLibrarySnapshot, bool) {
	if ctx == nil {
		return nil, false
	}
	snapshot, ok := ctx.Value(embyLibrarySnapshotContextKey{}).(*embyLibrarySnapshot)
	return snapshot, ok && snapshot != nil
}

func embyLibraryFromSnapshot(ctx context.Context, libraryID string) (*model.Library, bool) {
	snapshot, ok := embyLibrarySnapshotFromContext(ctx)
	if !ok {
		return nil, false
	}
	index, found := snapshot.byID[strings.TrimSpace(libraryID)]
	if !found {
		return nil, true
	}
	return &snapshot.libraries[index], true
}
