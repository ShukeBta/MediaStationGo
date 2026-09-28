package helper

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizeFlareSolverrEndpoint(t *testing.T) {
	cases := map[string]string{
		"":                          "",
		"http://localhost:8191":     "http://localhost:8191/v1",
		"http://flaresolverr:8191/": "http://flaresolverr:8191/v1",
		"http://localhost:8191/v1":  "http://localhost:8191/v1",
	}
	for input, want := range cases {
		if got := normalizeFlareSolverrEndpoint(input); got != want {
			t.Fatalf("normalizeFlareSolverrEndpoint(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFetchURLWithFlareSolverrResultReturnsBrowserIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request FlareSolverrRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Cookies) != 1 || request.Cookies[0].Name != "member" || request.Cookies[0].Value != "secret" {
			t.Fatalf("cookies = %#v", request.Cookies)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"status":    200,
				"response":  "<html>ok</html>",
				"userAgent": "test-browser",
				"cookies": []map[string]string{
					{"name": "cf_clearance", "value": "fresh", "domain": ".example.test", "path": "/"},
				},
			},
		})
	}))
	defer server.Close()

	solution, err := FetchURLWithFlareSolverrResult(
		server.URL,
		"https://example.test/page",
		[]FlareSolverrCookie{{Name: "member", Value: "secret"}},
		5,
		"",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if solution.Response != "<html>ok</html>" || solution.UserAgent != "test-browser" {
		t.Fatalf("solution = %#v", solution)
	}
	if len(solution.Cookies) != 1 || solution.Cookies[0].Name != "cf_clearance" {
		t.Fatalf("solution cookies = %#v", solution.Cookies)
	}
}

func TestFetchURLWithFlareSolverrResultContextStopsAtCallerDeadline(t *testing.T) {
	releaseServer := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-releaseServer:
		}
	}))
	defer server.Close()
	defer close(releaseServer)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := FetchURLWithFlareSolverrResultContext(
		ctx,
		server.URL,
		"https://example.test/page",
		nil,
		60,
		"",
		nil,
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancellable FlareSolverr request took %s", elapsed)
	}
}
