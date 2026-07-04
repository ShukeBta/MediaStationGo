package helper

import "strings"

// IsCloudflareChallenge checks if the HTML content is a Cloudflare challenge page.
func IsCloudflareChallenge(html string) bool {
	challengeTitles := []string{
		"Just a moment...",
		"请稍候…",
		"DDOS-GUARD",
	}
	challengeSelectors := []string{
		"#cf-challenge-running",
		".ray_id",
		".attack-box",
		"#cf-please-wait",
		"#challenge-spinner",
		"#trk_jschal_js",
	}

	lowerHTML := strings.ToLower(html)
	for _, title := range challengeTitles {
		titleLower := strings.ToLower(title)
		if strings.Contains(lowerHTML, strings.ToLower("<title>"+title)) ||
			strings.Contains(lowerHTML, titleLower) {
			return true
		}
	}

	for _, selector := range challengeSelectors {
		if strings.Contains(lowerHTML, strings.ToLower(selector)) {
			return true
		}
	}

	return false
}
