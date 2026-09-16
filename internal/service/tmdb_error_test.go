package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type tmdbErrorRoundTripFunc func(*http.Request) (*http.Response, error)

func (f tmdbErrorRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTMDbGetJSONRedactsAPIKeyFromHTTPError(t *testing.T) {
	const apiKey = "tmdb-secret-key-must-not-escape"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	provider := &TMDbProvider{client: server.Client()}
	err := provider.getJSON(
		t.Context(),
		server.URL+"/tv/55925/season/3?api_key="+apiKey+"&language=zh-CN",
		&struct{}{},
	)
	assertSafeTMDbError(t, err, apiKey, "/tv/55925/season/3", "HTTP 404")
}

func TestTMDbGetJSONRedactsAPIKeyFromTransportError(t *testing.T) {
	const apiKey = "tmdb-secret-key-must-not-escape"
	provider := &TMDbProvider{client: &http.Client{Transport: tmdbErrorRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed for " + req.URL.String())
	})}}

	err := provider.getJSON(
		context.Background(),
		"https://api.themoviedb.org/3/tv/55925/season/3?api_key="+apiKey+"&language=zh-CN",
		&struct{}{},
	)
	assertSafeTMDbError(t, err, apiKey, "/3/tv/55925/season/3", "request failed")
}

func assertSafeTMDbError(t *testing.T, err error, apiKey string, expected ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected TMDB request error")
	}
	message := err.Error()
	for _, secret := range []string{apiKey, "api_key=", "language=zh-CN"} {
		if strings.Contains(message, secret) {
			t.Fatalf("TMDB error exposed query data %q: %s", secret, message)
		}
	}
	for _, value := range expected {
		if !strings.Contains(message, value) {
			t.Fatalf("TMDB error %q does not contain %q", message, value)
		}
	}
}
