package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// embyVersionIdentity is the row-derived part of the existing Emby version
// identity. The merged-library identity remains request-local because library
// topology can change independently of a media row.
func embyVersionIdentity(m model.Media) string {
	if partKey := mediaPartGroupKey(m); partKey != "" {
		return partKey
	}
	if m.TMDbID > 0 {
		return fmt.Sprintf("tmdb:%d|s:%d|e:%d", m.TMDbID, m.SeasonNum, m.EpisodeNum)
	}
	if m.BangumiID > 0 {
		return fmt.Sprintf("bangumi:%d|s:%d|e:%d", m.BangumiID, m.SeasonNum, m.EpisodeNum)
	}
	if m.TitleCleanupVersion >= mediaTitleExplicitGroupingVersion {
		if key := strings.TrimSpace(m.VersionGroupKey); key != "" {
			return "cleanup-version:" + strings.ToLower(key)
		}
		return "row:" + strings.TrimSpace(m.ID)
	}
	title := strings.ToLower(strings.TrimSpace(m.Title))
	if title == "" {
		title = strings.ToLower(strings.TrimSpace(m.OriginalName))
	}
	if title == "" {
		return "row:" + strings.TrimSpace(m.ID)
	}
	return fmt.Sprintf("title:%s|y:%d|s:%d|e:%d", title, m.Year, m.SeasonNum, m.EpisodeNum)
}

func embyVersionPersistedKey(m model.Media) string {
	identity := embyVersionIdentity(m)
	if identity == "" || identity == "row:" {
		return ""
	}
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}
