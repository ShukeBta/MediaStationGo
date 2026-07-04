package helper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// FlareSolverrRequest represents a request to FlareSolverr.
type FlareSolverrRequest struct {
	Cmd        string               `json:"cmd"`
	URL        string               `json:"url"`
	Session    string               `json:"session,omitempty"`
	MaxTimeout int                  `json:"maxTimeout,omitempty"`
	Proxy      *FlareSolverrProxy   `json:"proxy,omitempty"`
	Cookies    []FlareSolverrCookie `json:"cookies,omitempty"`
}

// FlareSolverrProxy represents proxy config for FlareSolverr.
type FlareSolverrProxy struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// FlareSolverrCookie represents a cookie for FlareSolverr.
type FlareSolverrCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain,omitempty"`
	Path   string `json:"path,omitempty"`
}

// FlareSolverrResponse represents FlareSolverr's response.
type FlareSolverrResponse struct {
	Status   string                `json:"status"`
	Message  string                `json:"message"`
	Solution *FlareSolverrSolution `json:"solution,omitempty"`
}

// FlareSolverrSolution contains the solved challenge result.
type FlareSolverrSolution struct {
	URL       string               `json:"url"`
	Status    int                  `json:"status"`
	Headers   map[string]string    `json:"headers"`
	Cookies   []FlareSolverrCookie `json:"cookies"`
	UserAgent string               `json:"userAgent"`
	Response  string               `json:"response"`
}

// FetchURLWithFlareSolverr uses FlareSolverr to fetch a URL,
// bypassing Cloudflare/WAF challenges.
func FetchURLWithFlareSolverr(flareSolverrURL string, targetURL string, cookieStr string, timeout int, proxyURL string, log *zap.Logger) (string, error) {
	if flareSolverrURL == "" {
		return "", fmt.Errorf("FlareSolverr URL not configured")
	}
	if timeout <= 0 {
		timeout = 60
	}

	var cookies []FlareSolverrCookie
	if cookieStr != "" {
		cookies = parseCookiesForFlareSolverr(cookieStr)
	}

	reqBody := FlareSolverrRequest{
		Cmd:        "request.get",
		URL:        targetURL,
		MaxTimeout: timeout * 1000,
		Cookies:    cookies,
	}
	if proxyURL != "" {
		reqBody.Proxy = &FlareSolverrProxy{URL: proxyURL}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal FlareSolverr request: %w", err)
	}

	client := &http.Client{Timeout: time.Duration(timeout+10) * time.Second}
	resp, err := client.Post(flareSolverrURL, "application/json", strings.NewReader(string(jsonBody)))
	if err != nil {
		return "", fmt.Errorf("FlareSolverr request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read FlareSolverr response: %w", err)
	}

	var fsResp FlareSolverrResponse
	if err := json.Unmarshal(body, &fsResp); err != nil {
		return "", fmt.Errorf("failed to parse FlareSolverr response: %w", err)
	}

	if fsResp.Status != "ok" {
		return "", fmt.Errorf("FlareSolverr error: %s", fsResp.Message)
	}

	if fsResp.Solution != nil {
		return fsResp.Solution.Response, nil
	}
	return "", fmt.Errorf("FlareSolverr returned no solution")
}

// parseCookiesForFlareSolverr converts a cookie header string to FlareSolverr format.
func parseCookiesForFlareSolverr(cookieStr string) []FlareSolverrCookie {
	var cookies []FlareSolverrCookie
	parts := strings.Split(cookieStr, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			cookies = append(cookies, FlareSolverrCookie{
				Name:  kv[0],
				Value: kv[1],
			})
		}
	}
	return cookies
}
