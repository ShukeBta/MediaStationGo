package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var ErrEmbyMediaSourceUnavailable = errors.New("requested media source is not available for this item")

// ResolvePlaybackMedia authorizes and resolves one physical media source for
// the Emby video route. It avoids building an Item response (including subtitle
// discovery) and lets the handler reuse the loaded row while serving bytes.
func (e *EmbyService) ResolvePlaybackMedia(ctx context.Context, itemID, userID, requestedSourceID string, remember bool) (*model.Media, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" || e == nil || e.repo == nil || e.repo.Media == nil {
		return nil, nil
	}
	primary, err := e.repo.Media.FindByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if primary == nil {
		primary, err = e.playableMedia(ctx, itemID, userID)
		if err != nil || primary == nil {
			return primary, err
		}
	} else if !e.mediaVisibility(ctx, userID).Allows(primary) {
		return nil, nil
	}

	siblings := e.mediaVersionSiblings(ctx, primary)
	if len(siblings) == 0 {
		siblings = []model.Media{*primary}
	}
	selected := primary
	requestedSourceID = strings.TrimSpace(requestedSourceID)
	if requestedSourceID != "" {
		if len(requestedSourceID) > 128 {
			return nil, ErrEmbyMediaSourceUnavailable
		}
		selected = nil
		for i := range siblings {
			if siblings[i].ID == requestedSourceID {
				selected = &siblings[i]
				break
			}
		}
		if selected == nil || !UserDefaultMediaVisibility(ctx, e.repo, userID).Allows(selected) {
			return nil, ErrEmbyMediaSourceUnavailable
		}
	}
	if remember && strings.TrimSpace(userID) != "" && len(siblings) > 1 && e.repo.MediaPlaybackPreference != nil {
		scope := e.mediaSourcePreferenceScope(ctx, primary)
		if err := e.repo.MediaPlaybackPreference.SetPreferredMediaSource(ctx, userID, scope, selected.ID); err != nil {
			return nil, err
		}
	}
	return selected, nil
}

// ResolveInternalSTRMTargetMedia follows generated MSG /api/stream references
// inside the current request so the client does not make an extra public-domain
// round trip through the reverse proxy. Each referenced row is checked against
// the same user's media visibility before its bytes are served.
func (e *EmbyService) ResolveInternalSTRMTargetMedia(ctx context.Context, media *model.Media, userID string, request *http.Request) (*model.Media, bool, error) {
	if e == nil || e.repo == nil || e.repo.Media == nil || media == nil {
		return media, false, nil
	}
	current := media
	seen := map[string]struct{}{strings.TrimSpace(media.ID): {}}
	resolved := false
	for depth := 0; depth < 8; depth++ {
		target, err := mediaSTRMTarget(current)
		if err != nil {
			return nil, resolved, err
		}
		id, ok := internalSTRMStreamMediaID(target, request, PublicServerURL(ctx, e.repo, e.cfg))
		if !ok {
			return current, resolved, nil
		}
		resolved = true
		if _, exists := seen[id]; exists {
			return nil, true, ErrMediaNotFound
		}
		seen[id] = struct{}{}
		next, err := e.repo.Media.FindByID(ctx, id)
		if err != nil {
			return nil, true, err
		}
		if next == nil || !e.mediaVisibility(ctx, userID).Allows(next) {
			return nil, true, nil
		}
		current = next
	}
	return nil, true, ErrMediaNotFound
}

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
