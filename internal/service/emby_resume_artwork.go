package service

import (
	"context"
	"fmt"
	"strings"
)

func (e *EmbyService) attachResumeSeriesArtwork(ctx context.Context, userID string, items []map[string]any) error {
	seriesIDs := make([]string, 0, len(items))
	seenSeries := make(map[string]struct{}, len(items))
	for _, item := range items {
		needsArtwork, err := resumeEpisodeNeedsSeriesArtwork(item)
		if err != nil {
			return err
		}
		if !needsArtwork {
			continue
		}
		seriesID := strings.TrimSpace(embyPayloadString(item, "SeriesId", "seriesId"))
		if seriesID == "" {
			return fmt.Errorf("resume episode %q is missing SeriesId", embyPayloadString(item, "Id", "id"))
		}
		if _, exists := seenSeries[seriesID]; exists {
			continue
		}
		seenSeries[seriesID] = struct{}{}
		seriesIDs = append(seriesIDs, seriesID)
	}

	seriesItems := make(map[string]map[string]any, len(seriesIDs))
	for _, seriesID := range seriesIDs {
		group, ok, err := e.findSeriesGroup(ctx, seriesID, userID)
		if err != nil {
			return fmt.Errorf("resolve resume series artwork %q: %w", seriesID, err)
		}
		if !ok {
			return fmt.Errorf("resolve resume series artwork %q: series not found", seriesID)
		}
		seriesItem := e.seriesPayload(group)
		if resolvedID := strings.TrimSpace(embyPayloadString(seriesItem, "Id", "id")); resolvedID != seriesID {
			return fmt.Errorf("resolve resume series artwork %q: payload id is %q", seriesID, resolvedID)
		}
		seriesItems[seriesID] = seriesItem
	}

	for _, item := range items {
		needsArtwork, err := resumeEpisodeNeedsSeriesArtwork(item)
		if err != nil {
			return err
		}
		if !needsArtwork {
			continue
		}
		seriesID := strings.TrimSpace(embyPayloadString(item, "SeriesId", "seriesId"))
		seriesItem, ok := seriesItems[seriesID]
		if !ok {
			continue
		}
		if err := embyAttachResumeSeriesArtwork(item, seriesItem); err != nil {
			return err
		}
	}
	return nil
}

func resumeEpisodeNeedsSeriesArtwork(item map[string]any) (bool, error) {
	if !strings.EqualFold(strings.TrimSpace(embyPayloadString(item, "Type", "type")), "Episode") {
		return false, nil
	}
	imageTags, ok := item["ImageTags"].(map[string]string)
	if !ok {
		return false, fmt.Errorf("resume episode %q has invalid ImageTags type %T", embyPayloadString(item, "Id", "id"), item["ImageTags"])
	}
	backdropTags, ok := item["BackdropImageTags"].([]string)
	if !ok {
		return false, fmt.Errorf("resume episode %q has invalid BackdropImageTags type %T", embyPayloadString(item, "Id", "id"), item["BackdropImageTags"])
	}
	hasPrimary := strings.TrimSpace(imageTags["Primary"]) != ""
	hasEpisodeThumb := strings.TrimSpace(imageTags["Thumb"]) != ""
	hasOwnBackdrop := len(embyPayloadStringSlice(backdropTags)) > 0
	// Resume/NextUp cards with an episode still must stay episode-owned after
	// a client refresh. Advertising an inherited series backdrop alongside an
	// own Thumb lets clients replace the correct still with a cached series
	// backdrop. Parent artwork remains the explicit fallback for episodes that
	// do not have their own still.
	return !hasPrimary || (!hasEpisodeThumb && !hasOwnBackdrop), nil
}

func embyAttachResumeSeriesArtwork(item, seriesItem map[string]any) error {
	itemID := strings.TrimSpace(embyPayloadString(item, "Id", "id"))
	seriesID := strings.TrimSpace(embyPayloadString(seriesItem, "Id", "id"))
	imageTags, ok := item["ImageTags"].(map[string]string)
	if !ok {
		return fmt.Errorf("resume episode %q has invalid ImageTags type %T", itemID, item["ImageTags"])
	}
	backdropTags, ok := item["BackdropImageTags"].([]string)
	if !ok {
		return fmt.Errorf("resume episode %q has invalid BackdropImageTags type %T", itemID, item["BackdropImageTags"])
	}
	seriesImageTags, ok := seriesItem["ImageTags"].(map[string]string)
	if !ok {
		return fmt.Errorf("resume series %q has invalid ImageTags type %T", seriesID, seriesItem["ImageTags"])
	}
	seriesBackdropTags, ok := seriesItem["BackdropImageTags"].([]string)
	if !ok {
		return fmt.Errorf("resume series %q has invalid BackdropImageTags type %T", seriesID, seriesItem["BackdropImageTags"])
	}

	if strings.TrimSpace(imageTags["Primary"]) == "" {
		if primaryTag := strings.TrimSpace(seriesImageTags["Primary"]); primaryTag != "" {
			imageTags["Primary"] = primaryTag
			item["PrimaryImageTag"] = primaryTag
			item["PrimaryImageItemId"] = seriesID
		}
	}
	if len(embyPayloadStringSlice(backdropTags)) == 0 {
		if inheritedTags := embyPayloadStringSlice(seriesBackdropTags); len(inheritedTags) > 0 {
			item["BackdropImageItemId"] = seriesID
			item["ParentBackdropItemId"] = seriesID
			item["ParentBackdropImageTags"] = append([]string(nil), inheritedTags...)
		}
	}
	return nil
}
