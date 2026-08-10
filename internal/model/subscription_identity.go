package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type subscriptionIdentityPayload struct {
	Name          string  `json:"name"`
	FeedURL       string  `json:"feed_url"`
	Filter        string  `json:"filter"`
	MediaType     string  `json:"media_type"`
	MediaCategory string  `json:"media_category"`
	SavePath      string  `json:"save_path"`
	SearchMode    string  `json:"search_mode"`
	IMDBID        string  `json:"imdb_id"`
	Resolution    string  `json:"resolution"`
	Quality       string  `json:"quality"`
	Effects       string  `json:"effects"`
	ReleaseGroups string  `json:"release_groups"`
	ExcludeWords  string  `json:"exclude_words"`
	MinSeeders    int     `json:"min_seeders"`
	MaxSeeders    int     `json:"max_seeders"`
	MinSizeGB     float64 `json:"min_size_gb"`
	MaxSizeGB     float64 `json:"max_size_gb"`
	FreeOnly      bool    `json:"free_only"`
	WashEnabled   bool    `json:"wash_enabled"`
	WashPriority  string  `json:"wash_priority"`
	Priority      int     `json:"priority"`
}

// SubscriptionIdentityKey identifies one functional subscription rule. It
// intentionally excludes display metadata and runtime/archive state because
// those fields are backfilled or changed automatically after creation.
func SubscriptionIdentityKey(sub *Subscription) string {
	if sub == nil {
		return ""
	}
	payload := subscriptionIdentityPayload{
		Name:          subscriptionIdentityFold(sub.Name),
		FeedURL:       strings.TrimSpace(sub.FeedURL),
		Filter:        strings.TrimSpace(sub.Filter),
		MediaType:     subscriptionIdentityFold(sub.MediaType),
		MediaCategory: subscriptionIdentityFold(sub.MediaCategory),
		SavePath:      strings.TrimSpace(sub.SavePath),
		SearchMode:    subscriptionIdentityFold(sub.SearchMode),
		IMDBID:        subscriptionIdentityFold(sub.IMDBID),
		Resolution:    subscriptionIdentityFold(sub.Resolution),
		Quality:       subscriptionIdentityFold(sub.Quality),
		Effects:       subscriptionIdentityList(sub.Effects),
		ReleaseGroups: subscriptionIdentityList(sub.ReleaseGroups),
		ExcludeWords:  subscriptionIdentityList(sub.ExcludeWords),
		MinSeeders:    sub.MinSeeders,
		MaxSeeders:    sub.MaxSeeders,
		MinSizeGB:     sub.MinSizeGB,
		MaxSizeGB:     sub.MaxSizeGB,
		FreeOnly:      sub.FreeOnly,
		WashEnabled:   sub.WashEnabled,
		WashPriority:  subscriptionIdentityFold(sub.WashPriority),
		Priority:      sub.Priority,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func RefreshSubscriptionIdentity(sub *Subscription) string {
	key := SubscriptionIdentityKey(sub)
	if sub != nil {
		sub.IdentityKey = key
	}
	return key
}

func subscriptionIdentityFold(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func subscriptionIdentityList(value string) string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = subscriptionIdentityFold(part); part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, ",")
}
