package helper

import (
	"io"
	"net/http"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

// GetPageSource fetches a page with browser-like headers.
// Returns (pageSource, cookies, error).
func GetPageSource(url string, site *model.Site, timeout int, log *zap.Logger) (string, string, error) {
	client := NewSiteHTTPClient(timeout, site.UseProxy)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", err
	}

	headers := HTTPHeaderPresets()
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	ApplySiteAuthHeaders(req, site)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	cookies := ""
	for _, c := range resp.Cookies() {
		if cookies != "" {
			cookies += "; "
		}
		cookies += c.Name + "=" + c.Value
	}

	return string(body), cookies, nil
}
