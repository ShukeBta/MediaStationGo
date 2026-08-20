package service

import (
	"context"
	"net/url"
	"strings"
)

func adultConfiguredBases(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\r', '\t', ' ':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.HasPrefix(part, "http://") && !strings.HasPrefix(part, "https://") {
			part = "https://" + part
		}
		out = append(out, strings.TrimRight(part, "/"))
	}
	return out
}

func adultSourceKind(base string) string {
	u, err := url.Parse(strings.TrimSpace(base))
	host := ""
	if err == nil {
		host = strings.ToLower(u.Hostname())
	}
	if host == "" {
		host = strings.ToLower(base)
	}
	if strings.Contains(host, "javdb") {
		return "javdb"
	}
	for _, needle := range []string{"javbus", "cdnbus", "javsee", "busjav"} {
		if strings.Contains(host, needle) {
			return "javbus"
		}
	}
	return "javdb"
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (p *AdultProvider) getSetting(ctx context.Context, key, fallback string) string {
	if p != nil && p.repo != nil && p.repo.Setting != nil {
		if val, err := p.repo.Setting.Get(ctx, key); err == nil && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return fallback
}

func (p *AdultProvider) getSettingBool(ctx context.Context, key string, fallback bool) bool {
	val := p.getSetting(ctx, key, "")
	if val == "" {
		return fallback
	}
	val = strings.ToLower(val)
	return val == "true" || val == "1" || val == "yes" || val == "on"
}
