package service

import (
	"context"
	"errors"
	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const doubanFullFixture = `{"id":"123","title":"豆瓣标题","original_title":"Original","intro":"完整简介","year":"2025","rating":{"value":8.6},"pubdate":["2025-06-01(中国大陆)"],"pic":{"large":"https://img1.doubanio.com/view/photo/s/public/p123.jpg?imageView2/1/w/200"},"languages":["汉语普通话"],"countries":["中国大陆"],"genres":["剧情"],"actors":[{"name":"演员"}],"durations":["125分钟"]}`

func TestDoubanEnrichmentPreservesEditsAndSavesEvidence(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.Media{}, &model.DoubanSnapshot{})
	item := model.Media{Title: "手工标题", Path: "movie.mkv", Overview: "手工简介", Rating: 7.2, PosterURL: "https://image.tmdb.org/own.jpg", DoubanID: "123"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	provider := NewDoubanProvider(nil, nil)
	provider.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/movie/123") {
			t.Fatalf("unexpected endpoint %s", req.URL)
		}
		return doubanFixtureResponse(req, 200, doubanFullFixture), nil
	})}
	scraper := &ScraperService{repo: repository.New(db), douban: provider, log: zap.NewNop()}
	scraper.repo.Media.SetSeriesKeyFunc(MediaSeriesKey)
	got, err := scraper.EnrichFromDouban(t.Context(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != item.Title || got.Overview != item.Overview || got.Rating != item.Rating || got.PosterURL != item.PosterURL {
		t.Fatalf("manual metadata overwritten: %#v", got)
	}
	if got.DoubanRating != 8.6 || got.Year != 2025 || got.ReleaseDate != "2025-06-01" || got.Actors != "演员" || got.DoubanFetchedAt == nil || got.DoubanDegraded {
		t.Fatalf("enrichment incomplete: %#v", got)
	}
	var snapshot model.DoubanSnapshot
	if err := db.First(&snapshot, "media_id = ?", item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if snapshot.Payload != doubanFullFixture || snapshot.DoubanID != "123" || snapshot.Degraded {
		t.Fatalf("bad evidence: %#v", snapshot)
	}
	// A fresh subject must be skipped by the scheduled follow-up.
	provider.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("fresh item was requested again")
		return nil, nil
	})
	if err := scraper.runDoubanEnrichment(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestDoubanEnrichmentRejectsConcurrentRebinding(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.Media{}, &model.DoubanSnapshot{})
	item := model.Media{Title: "Original", Path: "movie.mkv", DoubanID: "123"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	provider := NewDoubanProvider(nil, nil)
	provider.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := db.Model(&item).Update("douban_id", "456").Error; err != nil {
			t.Fatal(err)
		}
		return doubanFixtureResponse(req, 200, doubanFullFixture), nil
	})}
	scraper := &ScraperService{repo: repository.New(db), douban: provider, log: zap.NewNop()}
	if _, err := scraper.EnrichFromDouban(t.Context(), item.ID); !errors.Is(err, ErrDoubanBindingChanged) {
		t.Fatalf("got %v", err)
	}
	var count int64
	db.Model(&model.DoubanSnapshot{}).Count(&count)
	if count != 0 {
		t.Fatal("stale evidence saved")
	}
	var got model.Media
	db.First(&got, "id = ?", item.ID)
	if got.DoubanID != "456" || got.DoubanRating != 0 || got.Overview != "" {
		t.Fatalf("stale data saved: %#v", got)
	}
}

func TestDoubanMobileFailureAndDegradedContract(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		degraded bool
		want     error
		requests int
	}{
		{"permission denial", 403, `{}`, true, nil, 2},
		{"API permission code", 200, `{"code":1000}`, true, nil, 2},
		{"rate limit", 429, `{}`, false, ErrDoubanTemporarilyUnavailable, 1},
		{"server error", 503, `{}`, false, ErrDoubanTemporarilyUnavailable, 1},
		{"missing", 404, `{}`, false, ErrDoubanSubjectNotFound, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			provider := NewDoubanProvider(nil, nil)
			provider.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return doubanFixtureResponse(req, tc.status, tc.body), nil
				}
				if !strings.HasSuffix(req.URL.Path, "/subject/123") {
					t.Fatalf("unexpected fallback %s", req.URL)
				}
				return doubanFixtureResponse(req, 200, doubanFullFixture), nil
			})}
			got, degraded, err := provider.GetEnrichmentMatchByID(t.Context(), "123", "movie")
			if !errors.Is(err, tc.want) || degraded != tc.degraded || calls != tc.requests {
				t.Fatalf("got match=%v degraded=%v err=%v calls=%d", got, degraded, err, calls)
			}
			if got != nil && (got.PosterURL != "https://img1.doubanio.com/view/photo/l/public/p123.jpg" || got.DurationMinutes != 125) {
				t.Fatalf("bad detail: %#v", got)
			}
		})
	}
}

func TestDoubanCandidatesReturnAllAndExplicitIDDoesNotFallback(t *testing.T) {
	provider := NewDoubanProvider(nil, nil)
	calls := 0
	provider.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if strings.Contains(req.URL.Path, "subject_suggest") {
			return doubanFixtureResponse(req, 200, `[{"id":"1","title":"同名","year":"2020","img":"https://img.example/one.jpg"},{"id":"2","title":"同名","year":"2025"}]`), nil
		}
		return doubanFixtureResponse(req, 404, `{}`), nil
	})}
	scraper := &ScraperService{douban: provider}
	got := scraper.manualDoubanMatches(t.Context(), "同名")
	if len(got) != 2 || got[1].DoubanID != "2" || got[0].PosterURL != "https://img.example/one.jpg" {
		t.Fatalf("candidates=%#v", got)
	}
	if got := scraper.manualDoubanMatches(t.Context(), "1917"); len(got) != 2 {
		t.Fatalf("numeric title did not fall back: %#v", got)
	}
	before := calls
	if got := scraper.manualDoubanMatches(t.Context(), "douban:999"); len(got) != 0 {
		t.Fatalf("ID fell back to unrelated suggestion: %#v", got)
	}
	if calls-before != 2 {
		t.Fatalf("unexpected extra search requests: %d", calls-before)
	}
}

func TestDoubanCookieMigrationIsEncryptedAndDoesNotRestoreClearedKey(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.APIConfig{}, &model.Setting{})
	svc := NewAPIConfigService(zap.NewNop(), repository.New(db), NewCryptoService("test-secret", zap.NewNop()))
	if err := svc.SeedDefaults(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := svc.MigrateDoubanCookie(t.Context(), "secret-session"); err != nil {
		t.Fatal(err)
	}
	row, err := svc.findByProvider(t.Context(), "douban")
	if err != nil || row.APIKey == "secret-session" || !svc.crypto.IsEncrypted(row.APIKey) {
		t.Fatal("cookie was not encrypted")
	}
	if err := svc.Delete(t.Context(), "douban"); err != nil {
		t.Fatal(err)
	}
	if err := svc.MigrateDoubanCookie(t.Context(), "secret-session"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Resolve(t.Context(), "douban")
	if err != nil || got.APIKey != "" {
		t.Fatalf("cleared cookie restored: %#v %v", got, err)
	}
}

func doubanFixtureResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}
}

func TestDoubanEnrichmentCancellationDoesNotFallback(t *testing.T) {
	p := NewDoubanProvider(nil, nil)
	calls := 0
	p.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })}
	if _, _, err := p.GetEnrichmentMatchByID(t.Context(), "123", "movie"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if calls != 1 {
		t.Fatal("cancelled request retried")
	}
}

func TestDoubanBindingChangesClearProviderRatingAtomically(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.Media{})
	repo := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repo)
	fetched := time.Now().UTC()
	item := model.Media{Title: "Movie", Path: "movie.mkv", DoubanID: "123", DoubanRating: 8.8, DoubanFetchedAt: &fetched, DoubanDegraded: true}
	if err := repo.Media.Upsert(t.Context(), &item); err != nil {
		t.Fatal(err)
	}
	same := "123"
	unchanged, err := svc.UpdateMetadata(t.Context(), item.ID, MediaMetadataUpdate{DoubanID: &same})
	if err != nil || unchanged.DoubanRating != 8.8 || unchanged.DoubanFetchedAt == nil {
		t.Fatalf("same-ID update lost rating: %#v %v", unchanged, err)
	}
	changed := "456"
	got, err := svc.UpdateMetadata(t.Context(), item.ID, MediaMetadataUpdate{DoubanID: &changed})
	if err != nil || got.DoubanRating != 0 || got.DoubanFetchedAt != nil || got.DoubanDegraded {
		t.Fatalf("new-ID update retained old rating: %#v %v", got, err)
	}
}
