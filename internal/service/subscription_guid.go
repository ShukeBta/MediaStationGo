package service

import (
	"fmt"
	"net/url"
	"strings"
)

func stableRSSItemGUID(title, guid, link, enclosureURL string) string {
	parts := []string{"rss", strings.ToLower(strings.TrimSpace(title))}
	for _, raw := range []string{guid, enclosureURL, link} {
		if key := stableDownloadURLKey(raw); key != "" {
			parts = append(parts, key)
			return strings.Join(parts, "|")
		}
		if raw = strings.TrimSpace(raw); raw != "" {
			parts = append(parts, strings.ToLower(raw))
			return strings.Join(parts, "|")
		}
	}
	return strings.Join(parts, "|")
}

func stableSiteSearchGUID(item SearchResult, download string) string {
	parts := []string{
		"site",
		strings.ToLower(strings.TrimSpace(firstNonEmpty(item.SiteID, item.SiteName))),
		strings.ToLower(strings.TrimSpace(item.Category)),
		strings.ToLower(strings.TrimSpace(item.Title)),
		fmt.Sprintf("%d", item.Size),
	}
	if key := stableDownloadURLKey(download); key != "" {
		parts = append(parts, key)
	}
	return strings.Join(parts, "|")
}

func stableDownloadURLKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return strings.ToLower(raw)
	}
	if strings.EqualFold(u.Scheme, "magnet") {
		xt := strings.ToLower(strings.TrimSpace(u.Query().Get("xt")))
		if xt != "" {
			return "magnet:" + xt
		}
		return strings.ToLower(raw)
	}
	if u.Host == "" {
		return strings.ToLower(raw)
	}
	q := u.Query()
	kept := make([]string, 0, 4)
	for _, key := range []string{"id", "tid", "torrent", "torrent_id", "torrentid", "hash", "info_hash"} {
		if value := strings.TrimSpace(q.Get(key)); value != "" {
			kept = append(kept, key+"="+strings.ToLower(value))
		}
	}
	base := strings.ToLower(strings.TrimRight(u.Host, "/") + "/" + strings.TrimLeft(u.Path, "/"))
	if len(kept) > 0 {
		return base + "?" + strings.Join(kept, "&")
	}
	return base
}
