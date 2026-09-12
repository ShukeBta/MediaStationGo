package service

import (
	"net/url"
	"strings"
)

const defaultMTeamAPIBase = "https://api.m-team.cc"

// M-Team serves its website and authenticated API on separate origins. Custom
// API hosts/proxies remain usable, including configurations saved in URL before
// the api_url override was available.
func mteamAPIBaseURL(cfg SiteConfig) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.Extra["api_url"]), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
		if u, err := url.Parse(base); err == nil {
			switch strings.ToLower(u.Hostname()) {
			case "m-team.cc", "www.m-team.cc", "kp.m-team.cc", "m-team.io", "www.m-team.io", "kp.m-team.io":
				return defaultMTeamAPIBase
			}
		}
	}
	if base == "" {
		return defaultMTeamAPIBase
	}
	return strings.TrimSuffix(base, "/api")
}
