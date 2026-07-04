package helper

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// ApplySiteAuthHeaders applies authentication headers based on site config.
func ApplySiteAuthHeaders(req *http.Request, site *model.Site) {
	switch site.AuthType {
	case "cookie":
		if site.Cookie != "" {
			req.Header.Set("Cookie", site.Cookie)
		}
	case "api_key":
		if site.APIKey != "" {
			if isYemaPTSite(site) {
				req.Header.Set("Authorization", site.APIKey)
			} else {
				req.Header.Set("x-api-key", site.APIKey)
			}
		}
	case "auth_header":
		if site.AuthHeader != "" {
			req.Header.Set("Authorization", site.AuthHeader)
		}
	}

	if site.UserAgent != "" {
		req.Header.Set("User-Agent", site.UserAgent)
	}
}

func isYemaPTSite(site *model.Site) bool {
	if site == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(site.Type), "yemapt") {
		return true
	}
	u, err := url.Parse(strings.TrimSpace(site.URL))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "yemapt.org" || strings.HasSuffix(host, ".yemapt.org")
}
