package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var ErrEmbyMediaSourceUnavailable = errors.New("requested media source is not available for this item")

const embyMediaSourcePreferencePrefix = "msgo-version-"

// orderMediaSourcesForUser applies an explicit source selection first, then a
// saved per-user selection. The explicit source must belong to the item's
// current version group and be visible to the authenticated user.
func (e *EmbyService) orderMediaSourcesForUser(
	ctx context.Context,
	primary *model.Media,
	userID string,
	sources []map[string]any,
	requestedSourceID string,
	remember bool,
) ([]map[string]any, error) {
	requestedSourceID = strings.TrimSpace(requestedSourceID)
	userID = strings.TrimSpace(userID)
	if requestedSourceID != "" {
		if len(requestedSourceID) > 128 {
			return nil, ErrEmbyMediaSourceUnavailable
		}
		index := mediaSourceIndex(sources, requestedSourceID)
		if index < 0 {
			return nil, ErrEmbyMediaSourceUnavailable
		}
		selected, err := e.repo.Media.FindByID(ctx, requestedSourceID)
		if err != nil {
			return nil, err
		}
		if selected == nil || !UserDefaultMediaVisibility(ctx, e.repo, userID).Allows(selected) {
			return nil, ErrEmbyMediaSourceUnavailable
		}
		sources = moveMediaSourceFirst(sources, index)
		if remember && userID != "" && len(sources) > 1 {
			scope := e.mediaSourcePreferenceScope(ctx, primary)
			if err := e.repo.MediaPlaybackPreference.SetPreferredMediaSource(ctx, userID, scope, requestedSourceID); err != nil {
				return nil, err
			}
		}
		return sources, nil
	}

	if userID == "" || len(sources) < 2 || e.repo.MediaPlaybackPreference == nil {
		return sources, nil
	}
	scope := e.mediaSourcePreferenceScope(ctx, primary)
	preference, err := e.repo.MediaPlaybackPreference.FindByUserAndMedia(ctx, userID, scope)
	if err != nil {
		return nil, err
	}
	if preference == nil {
		return sources, nil
	}
	index := mediaSourceIndex(sources, preference.PreferredMediaSourceID)
	return moveMediaSourceFirst(sources, index), nil
}

// ResolveMediaSourceID validates an optional direct-stream MediaSourceId
// against the item whose stream route was requested.
func (e *EmbyService) ResolveMediaSourceID(ctx context.Context, itemID, userID, requestedSourceID string) (string, error) {
	requestedSourceID = strings.TrimSpace(requestedSourceID)
	if requestedSourceID == "" {
		return itemID, nil
	}
	primary, err := e.playableMedia(ctx, itemID, userID)
	if err != nil {
		return "", err
	}
	if primary == nil {
		return "", nil
	}
	sources := e.mediaSourcesForItem(ctx, primary, true, false)
	ordered, err := e.orderMediaSourcesForUser(ctx, primary, userID, sources, requestedSourceID, false)
	if err != nil {
		return "", err
	}
	if len(ordered) == 0 {
		return "", ErrEmbyMediaSourceUnavailable
	}
	resolved, _ := ordered[0]["Id"].(string)
	if strings.TrimSpace(resolved) == "" {
		return "", ErrEmbyMediaSourceUnavailable
	}
	return resolved, nil
}

// RememberMediaSourceSelection records the source that an authenticated client
// actually requested. Some clients leave PlaybackInfo.MediaSourceId empty and
// express their selection only through /Videos/{sourceId}/stream.
func (e *EmbyService) RememberMediaSourceSelection(ctx context.Context, mediaSourceID, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	selected, err := e.playableMedia(ctx, mediaSourceID, userID)
	if err != nil {
		return err
	}
	if selected == nil {
		return ErrEmbyMediaSourceUnavailable
	}
	sources := e.mediaSourcesForItem(ctx, selected, true, false)
	if len(sources) < 2 {
		return nil
	}
	_, err = e.orderMediaSourcesForUser(ctx, selected, userID, sources, selected.ID, true)
	return err
}

func (e *EmbyService) mediaSourcePreferenceScope(ctx context.Context, primary *model.Media) string {
	identity := ""
	if primary != nil {
		if match, ok := e.mediaVersionMatch(ctx, primary); ok {
			identity = match.key()
		}
		if strings.TrimSpace(identity) == "" {
			identity = "row:" + strings.TrimSpace(primary.ID)
		}
	}
	sum := sha256.Sum256([]byte(identity))
	return embyMediaSourcePreferencePrefix + hex.EncodeToString(sum[:])
}

func mediaSourceIndex(sources []map[string]any, sourceID string) int {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return -1
	}
	for index, source := range sources {
		id, _ := source["Id"].(string)
		if strings.TrimSpace(id) == sourceID {
			return index
		}
	}
	return -1
}

func moveMediaSourceFirst(sources []map[string]any, index int) []map[string]any {
	if index <= 0 || index >= len(sources) {
		return sources
	}
	out := make([]map[string]any, 0, len(sources))
	out = append(out, sources[index])
	out = append(out, sources[:index]...)
	out = append(out, sources[index+1:]...)
	return out
}
