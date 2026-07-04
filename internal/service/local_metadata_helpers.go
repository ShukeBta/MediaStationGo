package service

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func firstRemoteURL(baseDir string, values ...string) string {
	for _, value := range values {
		value = cleanXMLText(value)
		if value == "" {
			continue
		}
		if isHTTPURL(value) {
			return value
		}
		if filepath.IsAbs(value) && fileExists(value) {
			return filepath.Clean(value)
		}
		if baseDir != "" {
			local := filepath.Join(baseDir, filepath.FromSlash(value))
			if fileExists(local) {
				return filepath.Clean(local)
			}
		}
	}
	return ""
}

func firstText(values ...string) string {
	for _, value := range values {
		if text := cleanXMLText(value); text != "" {
			return text
		}
	}
	return ""
}

func cleanXMLText(value string) string {
	return strings.TrimSpace(value)
}

func joinNFOValues(values []string) string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = cleanXMLText(part)
			if part == "" {
				continue
			}
			key := strings.ToLower(part)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, part)
		}
	}
	return strings.Join(out, ",")
}

func yearFromDate(value string) int {
	if len(value) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(value[:4])
	return year
}

func normalizeReleaseDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < len("2006-01-02") {
		return ""
	}
	value = value[:len("2006-01-02")]
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.Format("2006-01-02")
	}
	return ""
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
