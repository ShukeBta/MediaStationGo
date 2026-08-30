package service

import "strings"

func embyAttachImageOwnerIDs(item map[string]any) {
	if item == nil || embyPayloadIsCollectionFolder(item) {
		return
	}
	itemID := strings.TrimSpace(embyPayloadString(item, "Id", "id", "ItemId", "itemId"))
	if itemID == "" {
		return
	}
	imageTags := embyPayloadStringMap(item["ImageTags"])
	primaryTag := strings.TrimSpace(imageTags["Primary"])
	seriesID := strings.TrimSpace(embyPayloadString(item, "SeriesId", "seriesId"))
	itemType := strings.ToLower(strings.TrimSpace(embyPayloadString(item, "Type", "type")))

	primaryOwnerID := ""
	if primaryTag != "" {
		primaryOwnerID = itemID
	} else if seriesID != "" && (itemType == "episode" || itemType == "season") {
		primaryOwnerID = seriesID
	}
	if primaryOwnerID != "" {
		item["PrimaryImageItemId"] = primaryOwnerID
	}
	if primaryTag != "" {
		item["PrimaryImageTag"] = primaryTag
	}
}

func embyPayloadIsCollectionFolder(item map[string]any) bool {
	itemType := strings.ToLower(strings.TrimSpace(embyPayloadString(item, "Type", "type")))
	return itemType == "collectionfolder"
}

func embyPayloadString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
	}
	return ""
}

func embyPayloadStringMap(value any) map[string]string {
	switch typed := value.(type) {
	case map[string]string:
		return typed
	case map[string]any:
		out := make(map[string]string, len(typed))
		for key, raw := range typed {
			if text, ok := raw.(string); ok {
				out[key] = text
			}
		}
		return out
	default:
		return nil
	}
}

func embyPayloadStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, raw := range typed {
			if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	default:
		return nil
	}
}
