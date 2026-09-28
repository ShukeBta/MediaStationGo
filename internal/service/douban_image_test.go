package service

import (
	"bytes"
	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestDoubanImageMirrorFailureRetriesAndCachesOriginal(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.APIConfig{})
	if err := db.Create(&model.APIConfig{Provider: "douban", Enabled: true, BaseURL: "https://images.example.com"}).Error; err != nil {
		t.Fatal(err)
	}
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	proxy.apiConfig = NewAPIConfigService(zap.NewNop(), repository.New(db), NewCryptoService("test-secret", zap.NewNop()))
	var hosts []string
	proxy.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		hosts = append(hosts, req.URL.Host)
		if req.URL.Host == "images.example.com" {
			return doubanFixtureResponse(req, 404, `missing`), nil
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(testJPEG)), Request: req}, nil
	})}
	raw := "https://img1.doubanio.com/view/photo/l/public/p123.jpg"
	for range 2 {
		if got, _, err := proxy.Fetch(t.Context(), raw); err != nil || !bytes.Equal(got, testJPEG) {
			t.Fatalf("fetch err=%v", err)
		}
	}
	if strings.Join(hosts, ",") != "images.example.com,img1.doubanio.com" {
		t.Fatalf("unexpected retries/cache behavior: %v", hosts)
	}
}

func TestDoubanImageDirectAndRepairCacheIsolation(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.APIConfig{})
	if err := db.Create(&model.APIConfig{Provider: "douban", Enabled: true, ImageDirect: true, BaseURL: "https://images.example.com"}).Error; err != nil {
		t.Fatal(err)
	}
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	proxy.apiConfig = NewAPIConfigService(zap.NewNop(), repository.New(db), NewCryptoService("test-secret", zap.NewNop()))
	clients := proxy.imageClientsForHost(t.Context(), "img1.doubanio.com")
	if len(clients) != 1 || clients[0].name != "direct" {
		t.Fatalf("clients=%#v", clients)
	}
	transport, ok := clients[0].client.Transport.(*imageSafeTransport)
	if !ok {
		t.Fatalf("direct image client has no SSRF guard: %T", clients[0].client.Transport)
	}
	if transport.base.Proxy != nil {
		t.Fatal("direct image transport uses a proxy")
	}
	if proxy.useDoubanImageDirect(t.Context(), "doubanio.com.attacker.test") {
		t.Fatal("unrelated hostname inherited Douban settings")
	}
	raw := "https://img1.doubanio.com/view/photo/l/public/p123.jpg"
	_, _, failed, err := proxy.remoteImageCachePaths(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(proxy.cacheDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{failed, failed + ".direct"} {
		if err := os.WriteFile(name, []byte("failed"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := proxy.RemoveFailed(raw); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{failed, failed + ".direct"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("failure marker retained: %s", name)
		}
	}
}

func TestDoubanArtworkRepairRetainsPosterIdentity(t *testing.T) {
	live := model.Media{PosterURL: "https://img1.doubanio.com/view/photo/s_ratio_poster/public/p123.jpg?imageView2/1/w/100"}
	updates := missingDoubanMetadata(live, &Match{PosterURL: "https://img1.doubanio.com/view/photo/l/public/different.jpg"})
	if updates["poster_url"] != "https://img1.doubanio.com/view/photo/l/public/p123.jpg" {
		t.Fatalf("repair replaced image identity: %#v", updates)
	}
}
