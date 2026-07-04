package service

import (
	"path/filepath"
	"strings"
)

func AdultCodeFromMediaPath(path string) string {
	if code := normalizeAdultCode(filepath.Base(path)); code != "" {
		return code
	}
	return normalizeAdultCode(path)
}

func normalizeAdultCode(input string) string {
	input = strings.ToUpper(strings.TrimSpace(input))
	if input == "" {
		return ""
	}
	input = strings.ReplaceAll(input, "_", "-")
	if m := adultFC2Pattern.FindStringSubmatch(input); len(m) > 1 {
		return "FC2-PPV-" + m[1]
	}
	if m := adultHEYZOPattern.FindStringSubmatch(input); len(m) > 1 {
		return "HEYZO-" + m[1]
	}
	if m := adultUncensoredPattern.FindStringSubmatch(input); len(m) > 2 {
		return m[1] + "-" + m[2]
	}
	for _, m := range adultStandardPattern.FindAllStringSubmatch(input, -1) {
		if len(m) < 3 {
			continue
		}
		prefix := strings.TrimSpace(m[1])
		if _, excluded := adultExcludedPrefixes[prefix]; excluded {
			continue
		}
		return prefix + "-" + m[2]
	}
	return ""
}
