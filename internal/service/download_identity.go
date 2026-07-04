package service

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"
)

var torrentEpisodeToken = regexp.MustCompile(`(?i)e\d{1,3}`)
var downloadPackTitleToken = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:complete|batch|pack|合集|全集|整季|全季)(?:[\s._-]|$)`)

func localAvailabilityTitleCandidates(title string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 6)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	add(availabilityQuery(title, ""))
	if cleaned, _ := CleanQuery(title); cleaned != "" {
		for _, candidate := range titleCandidates(cleaned) {
			add(candidate)
			fields := strings.Fields(candidate)
			for i := len(fields) - 1; i >= 1; i-- {
				prefix := strings.Join(fields[:i], " ")
				if containsCJK(prefix) {
					add(prefix)
				}
			}
		}
	}
	return out
}

func downloadTaskBlocksDuplicate(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "removed", "cancelled", "canceled":
		return false
	default:
		return true
	}
}

func downloadTaskBlocksReadd(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "deleted", "removed", "cancelled", "canceled":
		return false
	default:
		return true
	}
}

func downloadTaskIdentityKey(name string) string {
	if key := downloadMediaIdentityKey(name); key != "" {
		return key
	}
	return normalizedDownloadTitleKey(name)
}

type downloadMediaIdentity struct {
	TitleKey string
	Year     int
	Episodes []episodeRef
	Pack     bool
}

func parseDownloadMediaIdentity(name string) downloadMediaIdentity {
	title, year := CleanQuery(name)
	if isSeriesPackTitle(name) {
		title = downloadPackTitleToken.ReplaceAllString(title, " ")
	}
	titleKey := normalizeAvailabilityComparable(title)
	if titleKey == "" {
		titleKey = normalizeAvailabilityComparable(availabilityQuery(name, ""))
	}
	return downloadMediaIdentity{
		TitleKey: titleKey,
		Year:     year,
		Episodes: episodeRefsFromTitle(name),
		Pack:     isSeriesPackTitle(name),
	}
}

func downloadTitleCoversRequest(existing, requested string) bool {
	current := parseDownloadMediaIdentity(existing)
	want := parseDownloadMediaIdentity(requested)
	if current.TitleKey == "" || want.TitleKey == "" {
		currentKey := normalizedDownloadTitleKey(existing)
		wantKey := normalizedDownloadTitleKey(requested)
		return currentKey != "" && wantKey != "" && (currentKey == wantKey || strings.Contains(currentKey, wantKey) || strings.Contains(wantKey, currentKey))
	}
	if current.TitleKey != want.TitleKey {
		return false
	}
	if current.Year > 0 && want.Year > 0 && current.Year != want.Year {
		return false
	}
	if downloadIdentityCoversWholeSeason(existing, current) {
		return true
	}
	if len(current.Episodes) == 0 || len(want.Episodes) == 0 {
		return len(current.Episodes) == len(want.Episodes)
	}
	currentEpisodes := map[string]struct{}{}
	for _, ref := range current.Episodes {
		currentEpisodes[episodeKey(ref.Season, ref.Episode)] = struct{}{}
	}
	for _, ref := range want.Episodes {
		if _, ok := currentEpisodes[episodeKey(ref.Season, ref.Episode)]; !ok {
			return false
		}
	}
	return true
}

func downloadIdentityCoversWholeSeason(title string, identity downloadMediaIdentity) bool {
	if !identity.Pack || len(identity.Episodes) > 0 {
		return false
	}
	return seriesPackRE.MatchString(title)
}

func downloadMediaIdentityKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	identity := parseDownloadMediaIdentity(name)
	titleKey := identity.TitleKey
	if titleKey == "" {
		return ""
	}
	parts := []string{titleKey}
	if identity.Year > 0 {
		parts = append(parts, fmt.Sprintf("y%d", identity.Year))
	}
	if len(identity.Episodes) > 0 {
		first := identity.Episodes[0]
		last := identity.Episodes[len(identity.Episodes)-1]
		parts = append(parts, fmt.Sprintf("s%02de%03d", first.Season, first.Episode))
		if len(identity.Episodes) > 1 {
			parts = append(parts, fmt.Sprintf("to%03d", last.Episode))
		}
	}
	return strings.Join(parts, "|")
}

func normalizedDownloadTitleKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func publicDownloadTitle(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "下载任务"
	}
	if u, err := url.Parse(raw); err == nil {
		if dn := strings.TrimSpace(u.Query().Get("dn")); dn != "" {
			if decoded, err := url.QueryUnescape(dn); err == nil && strings.TrimSpace(decoded) != "" {
				return strings.TrimSpace(decoded)
			}
			return dn
		}
		if u.Host != "" {
			base := path.Base(u.Path)
			if base != "." && base != "/" && base != "" {
				base = strings.TrimSuffix(base, path.Ext(base))
				if base != "" {
					return base
				}
			}
			return u.Host
		}
	}
	if strings.HasPrefix(strings.ToLower(raw), "magnet:") {
		return "磁力下载"
	}
	return "下载任务"
}

func normalizeTorrentName(name string) string {
	name = torrentEpisodeToken.ReplaceAllString(strings.ToLower(name), "")
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
