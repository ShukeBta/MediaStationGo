package helper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	solution, err := FetchURLWithFlareSolverrResult(
		flareSolverrURL,
		targetURL,
		parseCookiesForFlareSolverr(cookieStr),
		timeout,
		proxyURL,
		log,
	)
	if err != nil {
		return "", err
	}
	return solution.Response, nil
}

// FetchURLWithFlareSolverrResult returns the complete FlareSolverr solution so
// callers that maintain an authenticated browser identity can reuse the
// returned cookies and user agent. Cookie values are never logged here.
func FetchURLWithFlareSolverrResult(
	flareSolverrURL string,
	targetURL string,
	cookies []FlareSolverrCookie,
	timeout int,
	proxyURL string,
	log *zap.Logger,
) (*FlareSolverrSolution, error) {
	return FetchURLWithFlareSolverrResultContext(
		context.Background(),
		flareSolverrURL,
		targetURL,
		cookies,
		timeout,
		proxyURL,
		log,
	)
}

// FetchURLWithFlareSolverrResultContext is the cancellable variant used by
// request-scoped callers that must stop waiting at their own deadline.
func FetchURLWithFlareSolverrResultContext(
	ctx context.Context,
	flareSolverrURL string,
	targetURL string,
	cookies []FlareSolverrCookie,
	timeout int,
	proxyURL string,
	log *zap.Logger,
) (*FlareSolverrSolution, error) {
	flareSolverrURL = normalizeFlareSolverrEndpoint(flareSolverrURL)
	if flareSolverrURL == "" {
		return nil, fmt.Errorf("FlareSolverr URL not configured")
	}
	if timeout <= 0 {
		timeout = 60
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
		return nil, fmt.Errorf("failed to marshal FlareSolverr request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, flareSolverrURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create FlareSolverr request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: time.Duration(timeout+10) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FlareSolverr request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read FlareSolverr response: %w", err)
	}

	var fsResp FlareSolverrResponse
	if err := json.Unmarshal(body, &fsResp); err != nil {
		return nil, fmt.Errorf("failed to parse FlareSolverr response: %w", err)
	}

	if fsResp.Status != "ok" {
		return nil, fmt.Errorf("FlareSolverr error: %s", fsResp.Message)
	}

	if fsResp.Solution != nil {
		if fsResp.Solution.Status >= http.StatusBadRequest {
			return nil, fmt.Errorf("FlareSolverr target returned %d", fsResp.Solution.Status)
		}
		return fsResp.Solution, nil
	}
	return nil, fmt.Errorf("FlareSolverr returned no solution")
}

func normalizeFlareSolverrEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	if strings.TrimSpace(u.Path) == "" || u.Path == "/" {
		u.Path = "/v1"
	}
	return u.String()
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
