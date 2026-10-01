package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestDoubanPublicRequestsRecoverFromTransientConnectionFailure(t *testing.T) {
	for _, discover := range []bool{false, true} {
		name := "search"
		if discover {
			name = "discover"
		}
		t.Run(name, func(t *testing.T) {
			calls := 0
			provider := NewDoubanProvider(nil, nil)
			provider.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return nil, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
				}
				body := `[{"id":"123","title":"国产电影","year":"2026","type":"movie"}]`
				if discover {
					body = `{"subjects":[{"id":"123","title":"国产电影","rate":"8.0"}]}`
				}
				return doubanFixtureResponse(req, http.StatusOK, body), nil
			})
			provider.directClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("retry must not bypass the configured external transport")
				return nil, nil
			})
			if discover {
				items, err := provider.Discover(t.Context(), "douban_hot_movie")
				if err != nil || len(items) != 1 || items[0].DoubanID != "123" {
					t.Fatalf("discover = %#v, %v", items, err)
				}
			} else {
				match, err := provider.SearchMatch(t.Context(), "国产电影")
				if err != nil || match == nil || match.DoubanID != "123" {
					t.Fatalf("search = %#v, %v", match, err)
				}
			}
			if calls != 2 {
				t.Fatalf("upstream calls = %d, want 2", calls)
			}
		})
	}
}

func TestDoubanPublicRetryBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		err    error
		body   string
		calls  int
	}{
		{name: "persistent EOF", err: io.EOF, calls: 2},
		{name: "truncated body", status: http.StatusOK, err: io.ErrUnexpectedEOF, calls: 2},
		{name: "forbidden", status: http.StatusForbidden, calls: 1},
		{name: "rate limited", status: http.StatusTooManyRequests, calls: 1},
		{name: "rate limited and truncated", status: http.StatusTooManyRequests, err: io.ErrUnexpectedEOF, calls: 1},
		{name: "invalid response", status: http.StatusOK, body: "<html>login required</html>", calls: 1},
		{name: "permanent network failure", err: errors.New("permanent configuration error"), calls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			provider := NewDoubanProvider(nil, nil)
			provider.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if tt.status == 0 {
					return nil, tt.err
				}
				resp := doubanFixtureResponse(req, tt.status, tt.body)
				if tt.err != nil {
					resp.Body = &doubanErrorReadCloser{err: tt.err}
				}
				return resp, nil
			})
			_, err := provider.SearchCandidates(t.Context(), "test")
			if err == nil || calls != tt.calls {
				t.Fatalf("error = %v, calls = %d, want %d", err, calls, tt.calls)
			}
		})
	}
}

func TestDoubanPublicRetryStopsWhenRequestCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	provider := NewDoubanProvider(nil, nil)
	provider.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		cancel()
		return nil, io.EOF
	})
	_, err := provider.SearchCandidates(ctx, "test")
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("error = %v, calls = %d", err, calls)
	}
}

func TestDoubanPublicRetryRespectsParentDeadlineDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	calls := 0
	provider := NewDoubanProvider(nil, nil)
	provider.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, io.EOF
	})
	_, err := provider.SearchCandidates(ctx, "test")
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatalf("error = %v, calls = %d", err, calls)
	}
}

func TestDoubanPublicRequestsKeepProxyPoolRouting(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.APIConfig{})
	apiConfig := NewAPIConfigService(zap.NewNop(), &repository.Container{DB: db}, NewCryptoService("test-secret", zap.NewNop()))
	enabled := true
	if _, err := apiConfig.Update(t.Context(), "douban", APIConfigPatch{UseProxyPool: &enabled}); err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	provider := doubanProxyTestProvider(apiConfig,
		scriptedDoubanClient(t, "direct", &calls, doubanTestOutcome{err: io.ErrUnexpectedEOF}),
		scriptedDoubanClient(t, "proxy", &calls, doubanTestOutcome{status: http.StatusOK}),
	)
	_, err := provider.SearchCandidates(t.Context(), "test")
	if err != nil || strings.Join(calls, ",") != "direct,proxy" {
		t.Fatalf("error = %v, calls = %v", err, calls)
	}
}
