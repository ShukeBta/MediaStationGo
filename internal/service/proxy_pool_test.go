package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestNormalizeProxyPoolURL(t *testing.T) {
	for _, raw := range []string{
		"proxy.example:8080",
		"http://proxy.example:8080",
		"https://proxy.example:8443",
		"socks5://proxy.example:1080",
		"socks5h://proxy.example:1080",
	} {
		if _, err := normalizeProxyPoolURL(raw); err != nil {
			t.Fatalf("normalize %q: %v", raw, err)
		}
	}

	for _, raw := range []string{
		"ftp://proxy.example:21",
		"http:///missing-host",
		"http://proxy.example/path",
		"http://proxy.example?token=secret",
		"http://proxy.example#fragment",
	} {
		if _, err := normalizeProxyPoolURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}

	const sensitive = "ftp://alice:top-secret@proxy.example:21"
	_, err := normalizeProxyPoolURL(sensitive)
	if err == nil || strings.Contains(err.Error(), "alice") || strings.Contains(err.Error(), "top-secret") {
		t.Fatalf("unsafe validation error: %v", err)
	}
}

func TestProxyPoolServiceReplaceEncryptsAndProjectsCredentials(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.ProxyPoolEntry{})
	crypto := NewCryptoService("proxy-pool-test-secret", zap.NewNop())
	svc := NewProxyPoolService(&repository.Container{DB: db}, crypto)
	authURL := "http://alice:top-secret@proxy-a.example:8080"
	plainURL := "socks5://proxy-b.example:1080"

	items, err := svc.Replace(t.Context(), []ProxyPoolInput{{URL: &authURL}, {URL: &plainURL}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].DisplayURL != "http://proxy-a.example:8080" || !items[0].HasAuth {
		t.Fatalf("unsafe or incomplete projection: %#v", items)
	}
	response, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"alice", "top-secret", "enc:v1:"} {
		if strings.Contains(string(response), forbidden) {
			t.Fatalf("response exposed %q: %s", forbidden, response)
		}
	}

	var rows []model.ProxyPoolEntry
	if err := db.Order("position asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !crypto.IsEncrypted(rows[0].URL) || !crypto.IsEncrypted(rows[1].URL) {
		t.Fatalf("proxy URLs were not encrypted: %#v", rows)
	}
	for _, row := range rows {
		if strings.Contains(row.URL, "alice") || strings.Contains(row.URL, "top-secret") || strings.Contains(row.URL, "proxy-") {
			t.Fatal("proxy plaintext reached the database")
		}
	}

	firstID, secondID := items[0].ID, items[1].ID
	generation := svc.generation
	items, err = svc.Replace(t.Context(), []ProxyPoolInput{{ID: secondID}, {ID: firstID}})
	if err != nil {
		t.Fatal(err)
	}
	if items[0].ID != secondID || items[1].ID != firstID || svc.generation != generation+1 {
		t.Fatalf("reorder result = %#v, generation=%d", items, svc.generation)
	}

	replacement := "socks5h://bob:new-secret@proxy-c.example:1080"
	items, err = svc.Replace(t.Context(), []ProxyPoolInput{{ID: firstID, URL: &replacement}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != firstID || items[0].DisplayURL != "socks5h://proxy-c.example:1080" || !items[0].HasAuth {
		t.Fatalf("replacement result = %#v", items)
	}
	var deleted model.ProxyPoolEntry
	if err := db.First(&deleted, "id = ?", secondID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("removed proxy was not physically deleted: %v", err)
	}

	if _, err := svc.Replace(t.Context(), []ProxyPoolInput{{ID: "missing"}}); err == nil {
		t.Fatal("expected unknown proxy id to fail")
	}
	if _, err := svc.Replace(t.Context(), []ProxyPoolInput{{ID: firstID}, {ID: firstID}}); err == nil {
		t.Fatal("expected duplicate proxy id to fail")
	}
	badReplacement := "ftp://bob:do-not-leak@proxy.example:21"
	if _, err := svc.Replace(t.Context(), []ProxyPoolInput{{ID: firstID, URL: &badReplacement}}); err == nil || strings.Contains(err.Error(), "bob") || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("unsafe replacement error: %v", err)
	}
	items, err = svc.List(t.Context())
	if err != nil || len(items) != 1 || items[0].DisplayURL != "socks5h://proxy-c.example:1080" {
		t.Fatalf("failed update changed the proxy pool: %#v, %v", items, err)
	}
}

func TestProxyPoolServiceReplaceDeduplicatesURLs(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.ProxyPoolEntry{})
	crypto := NewCryptoService("proxy-pool-deduplicate-test", zap.NewNop())
	svc := NewProxyPoolService(&repository.Container{DB: db}, crypto)
	legacyURL := "http://legacy-proxy.example:8080"
	legacyCipherA, err := svc.encryptURL(legacyURL)
	if err != nil {
		t.Fatal(err)
	}
	legacyCipherB, err := svc.encryptURL(legacyURL)
	if err != nil {
		t.Fatal(err)
	}
	legacyRows := []model.ProxyPoolEntry{
		{URL: legacyCipherA, Position: 0},
		{URL: legacyCipherB, Position: 1},
	}
	if err := db.Create(&legacyRows).Error; err != nil {
		t.Fatal(err)
	}

	items, err := svc.Replace(t.Context(), []ProxyPoolInput{{ID: legacyRows[0].ID}, {ID: legacyRows[1].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != legacyRows[0].ID {
		t.Fatalf("legacy duplicates were not reduced to the first item: %#v", items)
	}

	plainURL := "proxy.example:8080"
	explicitURL := "http://proxy.example:8080/"
	authURLA := "http://account:credential-a@proxy.example:8080"
	authURLB := "http://account:credential-b@proxy.example:8080"
	items, err = svc.Replace(t.Context(), []ProxyPoolInput{
		{URL: &plainURL},
		{URL: &explicitURL},
		{URL: &authURLA},
		{URL: &authURLA},
		{URL: &authURLB},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].HasAuth || !items[1].HasAuth || !items[2].HasAuth {
		t.Fatalf("normalized duplicates or distinct credentials were handled incorrectly: %#v", items)
	}
	var stored []model.ProxyPoolEntry
	if err := db.Order("position asc").Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	want := []string{
		"http://proxy.example:8080",
		authURLA,
		authURLB,
	}
	if len(stored) != len(want) {
		t.Fatalf("stored proxy count = %d, want %d", len(stored), len(want))
	}
	for i := range stored {
		parsed, err := svc.decryptURL(stored[i].URL)
		if err != nil {
			t.Fatal(err)
		}
		if stored[i].Position != i || parsed.String() != want[i] {
			t.Fatal("stored proxies did not preserve the first normalized URL and distinct credentials")
		}
	}
}

func TestProbeProxyPoolHealthClassifiesSafeCleanupCandidates(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		err     error
		readErr error
		want    proxyPoolHealthStatus
	}{
		{name: "available", status: http.StatusOK, body: `[]`, want: proxyPoolHealthAvailable},
		{name: "invalid success body", status: http.StatusOK, body: `not-json`, want: proxyPoolHealthUnavailable},
		{name: "bad request", status: http.StatusBadRequest, body: `{}`, want: proxyPoolHealthUnavailable},
		{name: "proxy auth", status: http.StatusProxyAuthRequired, body: `{}`, want: proxyPoolHealthUnavailable},
		{name: "unexpected eof", status: http.StatusOK, readErr: io.ErrUnexpectedEOF, want: proxyPoolHealthUnavailable},
		{name: "other read error", status: http.StatusOK, readErr: errors.New("read failed"), want: proxyPoolHealthInconclusive},
		{name: "network error", err: errors.New("connection failed"), want: proxyPoolHealthUnavailable},
		{name: "forbidden", status: http.StatusForbidden, body: `{}`, want: proxyPoolHealthInconclusive},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{}`, want: proxyPoolHealthInconclusive},
		{name: "upstream error", status: http.StatusBadGateway, body: `{}`, want: proxyPoolHealthInconclusive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if tt.err != nil {
					return nil, tt.err
				}
				body := io.ReadCloser(io.NopCloser(strings.NewReader(tt.body)))
				if tt.readErr != nil {
					body = &doubanErrorReadCloser{err: tt.readErr}
				}
				return &http.Response{StatusCode: tt.status, Body: body, Request: req}, nil
			})}
			if got := probeProxyPoolHealth(t.Context(), client); got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestProxyPoolCheckSummarizesWithoutPersisting(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.ProxyPoolEntry{})
	svc := NewProxyPoolService(&repository.Container{DB: db}, NewCryptoService("proxy-health-test", zap.NewNop()))
	for _, raw := range []string{"http://proxy-a.example:8080", "http://proxy-b.example:8080", "http://proxy-c.example:8080"} {
		if _, err := svc.Replace(t.Context(), append(proxyInputsForItems(t, svc), ProxyPoolInput{URL: &raw})); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int64
	result, err := svc.check(t.Context(), func(context.Context, *http.Client) proxyPoolHealthStatus {
		switch calls.Add(1) {
		case 1, 2:
			return proxyPoolHealthAvailable
		case 3:
			return proxyPoolHealthUnavailable
		default:
			return proxyPoolHealthInconclusive
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || result.Available != 1 || result.Unavailable != 1 || result.Inconclusive != 1 || result.CleanupToken == "" {
		t.Fatalf("check result = %#v", result)
	}
	var count int64
	if err := db.Model(&model.ProxyPoolEntry{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("check changed persistence: count=%d err=%v", count, err)
	}
	if _, err := svc.check(t.Context(), func(context.Context, *http.Client) proxyPoolHealthStatus {
		return proxyPoolHealthInconclusive
	}); err == nil {
		t.Fatal("expected failed baseline to abort the check")
	}

	cancelCtx, cancel := context.WithCancel(t.Context())
	calls.Store(0)
	if _, err := svc.check(cancelCtx, func(context.Context, *http.Client) proxyPoolHealthStatus {
		if calls.Add(1) > 1 {
			cancel()
		}
		return proxyPoolHealthAvailable
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled check error = %v", err)
	}

	retained := proxyInputsForItems(t, svc)
	var once sync.Once
	var replaceErr error
	calls.Store(0)
	if _, err := svc.check(t.Context(), func(context.Context, *http.Client) proxyPoolHealthStatus {
		if calls.Add(1) > 1 {
			once.Do(func() { _, replaceErr = svc.Replace(t.Context(), retained) })
		}
		return proxyPoolHealthAvailable
	}); err == nil {
		t.Fatal("expected concurrent pool update to invalidate check")
	}
	if replaceErr != nil {
		t.Fatal(replaceErr)
	}
}

func TestProxyPoolCleanupDeletesOnlyCurrentIDs(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.ProxyPoolEntry{})
	svc := NewProxyPoolService(&repository.Container{DB: db}, NewCryptoService("proxy-cleanup-test", zap.NewNop()))
	urls := []string{"http://proxy-a.example:8080", "http://proxy-b.example:8080", "http://proxy-c.example:8080"}
	input := make([]ProxyPoolInput, 0, len(urls))
	for i := range urls {
		input = append(input, ProxyPoolInput{URL: &urls[i]})
	}
	items, err := svc.Replace(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	duplicateURL, err := svc.encryptURL(urls[0])
	if err != nil {
		t.Fatal(err)
	}
	legacyDuplicate := model.ProxyPoolEntry{URL: duplicateURL, Position: len(items)}
	if err := db.Create(&legacyDuplicate).Error; err != nil {
		t.Fatal(err)
	}
	generation := svc.generation
	token := uuid.NewString()
	svc.pending = &proxyPoolPendingCleanup{
		token: token, generation: generation, expiresAt: time.Now().Add(time.Minute),
		ids: map[string]struct{}{items[1].ID: {}},
	}
	if _, err := svc.Cleanup(t.Context(), uuid.NewString()); err == nil {
		t.Fatal("expected unrelated cleanup token to fail")
	}
	result, err := svc.Cleanup(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || len(result.Items) != 3 || result.Items[0].ID != items[0].ID || result.Items[1].ID != items[2].ID || result.Items[2].ID != legacyDuplicate.ID {
		t.Fatalf("cleanup result = %#v", result)
	}
	if svc.generation != generation+1 {
		t.Fatalf("generation = %d, want %d", svc.generation, generation+1)
	}
	svc.pending = &proxyPoolPendingCleanup{
		token: token, generation: svc.generation, expiresAt: time.Now().Add(time.Minute),
		ids: map[string]struct{}{items[0].ID: {}},
	}
	retained := []ProxyPoolInput{{ID: items[0].ID}, {ID: items[2].ID}}
	if _, err := svc.Replace(t.Context(), retained); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Cleanup(t.Context(), token); err == nil {
		t.Fatal("expected pool update to invalidate cleanup token")
	}
}

func proxyInputsForItems(t *testing.T, svc *ProxyPoolService) []ProxyPoolInput {
	t.Helper()
	items, err := svc.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	input := make([]ProxyPoolInput, 0, len(items))
	for _, item := range items {
		input = append(input, ProxyPoolInput{ID: item.ID})
	}
	return input
}

func TestAPIConfigUseProxyPoolRoundTripAndRevision(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.APIConfig{})
	if err := db.Create(&model.APIConfig{Provider: "douban", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewAPIConfigService(zap.NewNop(), &repository.Container{DB: db}, NewCryptoService("test-secret", zap.NewNop()))
	resolved, err := svc.Resolve(t.Context(), "douban")
	if err != nil || resolved.UseProxyPool || resolved.Revision != 0 {
		t.Fatalf("default resolved config = %#v, %v", resolved, err)
	}

	enabled := true
	view, err := svc.Update(t.Context(), "douban", APIConfigPatch{UseProxyPool: &enabled})
	if err != nil || !view.UseProxyPool {
		t.Fatalf("public config = %#v, %v", view, err)
	}
	publicJSON, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"proxy_pool_type", "resin_proxy_url", "resin_proxy_token", "resin_account"} {
		if strings.Contains(string(publicJSON), forbidden) {
			t.Fatalf("Douban API config still exposes %s", forbidden)
		}
	}
	resolved, err = svc.Resolve(t.Context(), "douban")
	if err != nil || !resolved.UseProxyPool || resolved.Revision != 1 {
		t.Fatalf("resolved config = %#v, %v", resolved, err)
	}
	if _, err := svc.Update(t.Context(), "douban", APIConfigPatch{UseProxyPool: &enabled}); err != nil {
		t.Fatal(err)
	}
	resolved, err = svc.Resolve(t.Context(), "douban")
	if err != nil || resolved.Revision != 2 {
		t.Fatalf("same-value save did not advance revision: %#v, %v", resolved, err)
	}
}

func TestNormalizeResinProxySettings(t *testing.T) {
	for raw, want := range map[string]string{
		"http://resin.internal:2260":  "http://resin.internal:2260",
		"https://resin.example:2260/": "https://resin.example:2260",
	} {
		got, err := normalizeResinProxyOrigin(raw)
		if err != nil || got != want {
			t.Fatalf("normalize %q = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{
		"resin.internal:2260",
		"http://user:do-not-leak@resin.internal:2260",
		"http://resin.internal:2260/path",
		"http://resin.internal:2260?token=do-not-leak",
	} {
		if _, err := normalizeResinProxyOrigin(raw); err == nil || strings.Contains(err.Error(), "do-not-leak") {
			t.Fatalf("unsafe Resin address result for %q: %v", raw, err)
		}
	}
	if got, err := normalizeProxyPoolType(""); err != nil || got != ProxyPoolTypeNormal {
		t.Fatalf("empty proxy type = %q, %v", got, err)
	}
	if _, err := normalizeProxyPoolType("other"); err == nil {
		t.Fatal("expected invalid proxy pool type to fail")
	}
}

func TestProxyPoolConfigRoundTrip(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.APIConfig{})
	if err := db.Create(&model.APIConfig{Provider: "douban", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	crypto := NewCryptoService("test-secret", zap.NewNop())
	svc := NewProxyPoolService(&repository.Container{DB: db}, crypto)
	proxyType := ProxyPoolTypeResin
	proxyURL := "http://resin.internal:2260/"
	token := "proxy-secret"
	account := " douban-main "
	if _, err := svc.UpdateConfig(t.Context(), ProxyPoolConfigPatch{
		ProxyPoolType: &proxyType,
		ResinProxyURL: &proxyURL,
	}); err == nil {
		t.Fatal("expected Resin mode without a token to fail")
	}
	view, err := svc.UpdateConfig(t.Context(), ProxyPoolConfigPatch{
		ProxyPoolType:   &proxyType,
		ResinProxyURL:   &proxyURL,
		ResinProxyToken: &token,
		ResinAccount:    &account,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.ProxyPoolType != ProxyPoolTypeResin || view.ResinProxyURL != "http://resin.internal:2260" || view.ResinAccount != "douban-main" || !view.HasResinProxyToken {
		t.Fatalf("public config = %#v", view)
	}
	publicJSON, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicJSON), token) || strings.Contains(string(publicJSON), "enc:v1:") {
		t.Fatal("public config exposed Resin credentials")
	}
	var row model.APIConfig
	if err := db.Where("provider = ?", "douban").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if !crypto.IsEncrypted(row.ResinProxyToken) || strings.Contains(row.ResinProxyToken, token) {
		t.Fatal("Resin token was not encrypted")
	}
	resolved, err := svc.resolveConfig(t.Context())
	if err != nil || resolved.ResinProxyToken != token || resolved.ResinAccount != "douban-main" {
		t.Fatalf("resolved config = %#v, %v", resolved, err)
	}
	emptyToken := ""
	if _, err := svc.UpdateConfig(t.Context(), ProxyPoolConfigPatch{ResinProxyToken: &emptyToken}); err != nil {
		t.Fatal(err)
	}
	resolved, err = svc.resolveConfig(t.Context())
	if err != nil || resolved.ResinProxyToken != token || resolved.Revision != 2 {
		t.Fatalf("empty token did not preserve config: %#v, %v", resolved, err)
	}

	invalidType := "other"
	if _, err := svc.UpdateConfig(t.Context(), ProxyPoolConfigPatch{ProxyPoolType: &invalidType}); err == nil {
		t.Fatal("expected invalid proxy pool type to fail")
	}
	invalidURL := "http://user:do-not-leak@resin.internal:2260"
	if _, err := svc.UpdateConfig(t.Context(), ProxyPoolConfigPatch{ResinProxyURL: &invalidURL}); err == nil || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("unsafe Resin URL error: %v", err)
	}
}

func TestProxyPoolConfigReadsExistingResinSettings(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.APIConfig{})
	crypto := NewCryptoService("test-secret", zap.NewNop())
	if err := db.Create(&model.APIConfig{
		Provider:        "douban",
		ProxyPoolType:   ProxyPoolTypeResin,
		ResinProxyURL:   "https://resin.example",
		ResinProxyToken: crypto.Encrypt("existing-token"),
		ResinAccount:    "existing-account",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewProxyPoolService(&repository.Container{DB: db}, crypto)
	view, err := svc.GetConfig(t.Context())
	if err != nil || view.ProxyPoolType != ProxyPoolTypeResin || view.ResinProxyURL != "https://resin.example" || view.ResinAccount != "existing-account" || !view.HasResinProxyToken {
		t.Fatalf("existing Resin config = %#v, %v", view, err)
	}
}

func TestBuildResinReverseURL(t *testing.T) {
	for _, tt := range []struct {
		name    string
		account string
		want    string
	}{
		{name: "random routing", want: "https://resin.example/proxy%2Fsecret/./https/movie.douban.com/a%2Fb?q=x%20y"},
		{name: "sticky routing", account: "douban/main", want: "https://resin.example/proxy%2Fsecret/Default.douban%2Fmain/https/movie.douban.com/a%2Fb?q=x%20y"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildResinReverseURL(resolvedProxyPoolConfig{
				ResinProxyURL:   "https://resin.example",
				ResinProxyToken: "proxy/secret",
				ResinAccount:    tt.account,
			}, "https://movie.douban.com/a%2Fb?q=x%20y")
			if err != nil || got != tt.want {
				t.Fatalf("reverse URL = %q, %v; want %q", got, err, tt.want)
			}
		})
	}

	secretURL := "https://resin.example/secret/./https/movie.douban.com/data"
	err := redactResinRequestError(&url.Error{Op: "Get", URL: secretURL, Err: io.ErrUnexpectedEOF})
	if !errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "[redacted-url]") {
		t.Fatalf("unsafe Resin error: %v", err)
	}
}

func TestShouldReselectDoubanRoute(t *testing.T) {
	tests := []struct {
		name   string
		route  int
		result doubanHTTPResult
		want   bool
	}{
		{name: "direct bad request", route: doubanDirectRoute, result: doubanHTTPResult{status: http.StatusBadRequest}, want: true},
		{name: "direct unexpected EOF", route: doubanDirectRoute, result: doubanHTTPResult{err: io.ErrUnexpectedEOF}, want: true},
		{name: "direct body unexpected EOF", route: doubanDirectRoute, result: doubanHTTPResult{status: http.StatusOK, err: errors.Join(io.ErrUnexpectedEOF)}, want: true},
		{name: "direct other network error", route: doubanDirectRoute, result: doubanHTTPResult{err: errors.New("network unavailable")}},
		{name: "direct server error", route: doubanDirectRoute, result: doubanHTTPResult{status: http.StatusServiceUnavailable}},
		{name: "proxy network error", route: 0, result: doubanHTTPResult{err: errors.New("network unavailable")}, want: true},
		{name: "proxy body error", route: 0, result: doubanHTTPResult{status: http.StatusOK, err: errors.New("read failed")}, want: true},
		{name: "proxy server error", route: 0, result: doubanHTTPResult{status: http.StatusServiceUnavailable}, want: true},
		{name: "proxy success", route: 0, result: doubanHTTPResult{status: http.StatusOK}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldReselectDoubanRoute(tt.route, tt.result); got != tt.want {
				t.Fatalf("shouldReselectDoubanRoute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoubanProxyPoolRouting(t *testing.T) {
	db := newProxyPoolTestDB(t, &model.APIConfig{})
	apiConfig := NewAPIConfigService(zap.NewNop(), &repository.Container{DB: db}, NewCryptoService("test-secret", zap.NewNop()))
	enabled := true
	if _, err := apiConfig.Update(t.Context(), "douban", APIConfigPatch{UseProxyPool: &enabled}); err != nil {
		t.Fatal(err)
	}
	resolved, err := apiConfig.Resolve(t.Context(), "douban")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("sticky proxy failure returns direct", func(t *testing.T) {
		calls := []string{}
		direct := scriptedDoubanClient(t, "direct", &calls,
			doubanTestOutcome{status: 400}, doubanTestOutcome{status: 200})
		proxy1 := scriptedDoubanClient(t, "proxy1", &calls,
			doubanTestOutcome{status: 200}, doubanTestOutcome{status: 503})
		proxy2 := scriptedDoubanClient(t, "proxy2", &calls)
		provider := doubanProxyTestProvider(apiConfig, direct, proxy1, proxy2)

		for range 2 {
			if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
				t.Fatalf("request status=%d err=%v", status, err)
			}
		}
		if got, want := strings.Join(calls, ","), "direct,proxy1,proxy1,direct"; got != want {
			t.Fatalf("calls = %s, want %s", got, want)
		}
		if route := provider.currentRoute(1, 0, resolved.Revision); route != doubanDirectRoute {
			t.Fatalf("route = %d, want direct", route)
		}
	})

	t.Run("failed proxy skips remaining pool", func(t *testing.T) {
		calls := []string{}
		provider := doubanProxyTestProvider(apiConfig,
			scriptedDoubanClient(t, "direct", &calls, doubanTestOutcome{status: 400}, doubanTestOutcome{status: 200}),
			scriptedDoubanClient(t, "proxy1", &calls, doubanTestOutcome{status: 503}),
			scriptedDoubanClient(t, "proxy2", &calls),
		)
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("status=%d err=%v", status, err)
		}
		if got, want := strings.Join(calls, ","), "direct,proxy1,direct"; got != want {
			t.Fatalf("calls = %s, want %s", got, want)
		}
	})

	t.Run("failed final direct stops routing", func(t *testing.T) {
		calls := []string{}
		provider := doubanProxyTestProvider(apiConfig,
			scriptedDoubanClient(t, "direct", &calls, doubanTestOutcome{status: 400}, doubanTestOutcome{status: 400}),
			scriptedDoubanClient(t, "proxy1", &calls, doubanTestOutcome{status: 503}),
			scriptedDoubanClient(t, "proxy2", &calls),
		)
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 400 {
			t.Fatalf("status=%d err=%v", status, err)
		}
		if got, want := strings.Join(calls, ","), "direct,proxy1,direct"; got != want {
			t.Fatalf("calls = %s, want %s", got, want)
		}
	})

	t.Run("empty pool still retries direct", func(t *testing.T) {
		calls := []string{}
		provider := doubanProxyTestProvider(apiConfig,
			scriptedDoubanClient(t, "direct", &calls, doubanTestOutcome{status: 400}, doubanTestOutcome{status: 400}),
		)
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 400 {
			t.Fatalf("status=%d err=%v", status, err)
		}
		if got := strings.Join(calls, ","); got != "direct,direct" {
			t.Fatalf("calls = %s", got)
		}
	})

	t.Run("proxy network error returns direct", func(t *testing.T) {
		calls := []string{}
		proxyErr := errors.New("proxy network failure")
		provider := doubanProxyTestProvider(apiConfig,
			scriptedDoubanClient(t, "direct", &calls, doubanTestOutcome{status: 200}),
			scriptedDoubanClient(t, "proxy1", &calls, doubanTestOutcome{err: proxyErr}),
		)
		provider.setRoute(1, 0, resolved.Revision, 0)
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("status=%d err=%v", status, err)
		}
		if route := provider.currentRoute(1, 0, resolved.Revision); route != doubanDirectRoute {
			t.Fatalf("route changed to %d", route)
		}
		if got := strings.Join(calls, ","); got != "proxy1,direct" {
			t.Fatalf("calls = %s", got)
		}
	})

	t.Run("unexpected EOF switches routes", func(t *testing.T) {
		calls := []string{}
		provider := doubanProxyTestProvider(apiConfig,
			scriptedDoubanClient(t, "direct", &calls, doubanTestOutcome{err: io.ErrUnexpectedEOF}),
			scriptedDoubanClient(t, "proxy1", &calls, doubanTestOutcome{status: 200}),
		)
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("status=%d err=%v", status, err)
		}
		if route := provider.currentRoute(1, 0, resolved.Revision); route != 0 {
			t.Fatalf("route = %d, want proxy1", route)
		}
		if got, want := strings.Join(calls, ","), "direct,proxy1"; got != want {
			t.Fatalf("calls = %s, want %s", got, want)
		}
	})

	t.Run("non-400 statuses do not switch routes", func(t *testing.T) {
		calls := []string{}
		provider := doubanProxyTestProvider(apiConfig,
			scriptedDoubanClient(t, "direct", &calls,
				doubanTestOutcome{status: 403},
				doubanTestOutcome{status: 404},
				doubanTestOutcome{status: 429},
				doubanTestOutcome{status: 500},
			),
			scriptedDoubanClient(t, "proxy1", &calls),
		)
		for _, want := range []int{403, 404, 429, 500} {
			if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != want {
				t.Fatalf("status=%d err=%v, want %d", status, err, want)
			}
		}
		if got := strings.Join(calls, ","); got != "direct,direct,direct,direct" {
			t.Fatalf("calls = %s", got)
		}
	})

	t.Run("pool generation resets to direct", func(t *testing.T) {
		calls := []string{}
		provider := doubanProxyTestProvider(apiConfig,
			scriptedDoubanClient(t, "direct", &calls, doubanTestOutcome{status: 400}, doubanTestOutcome{status: 200}),
			scriptedDoubanClient(t, "proxy1", &calls, doubanTestOutcome{status: 200}),
		)
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("select proxy status=%d err=%v", status, err)
		}
		provider.proxyPool.mu.Lock()
		provider.proxyPool.generation++
		provider.proxyPool.mu.Unlock()
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("reset request status=%d err=%v", status, err)
		}
		if got, want := strings.Join(calls, ","), "direct,proxy1,direct"; got != want {
			t.Fatalf("calls = %s, want %s", got, want)
		}
	})

	t.Run("proxy body read failure returns direct", func(t *testing.T) {
		calls := []string{}
		readErr := errors.New("read failed")
		provider := doubanProxyTestProvider(apiConfig,
			scriptedDoubanClient(t, "direct", &calls, doubanTestOutcome{status: 400}, doubanTestOutcome{status: 200}),
			scriptedDoubanClient(t, "proxy1", &calls, doubanTestOutcome{status: 200, readErr: readErr}),
		)
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("status=%d err=%v", status, err)
		}
		if got, want := strings.Join(calls, ","), "direct,proxy1,direct"; got != want {
			t.Fatalf("calls = %s, want %s", got, want)
		}
	})

	t.Run("configuration revisions reset to direct", func(t *testing.T) {
		calls := []string{}
		provider := doubanProxyTestProvider(apiConfig,
			scriptedDoubanClient(t, "direct", &calls, doubanTestOutcome{status: 400}, doubanTestOutcome{status: 200}),
			scriptedDoubanClient(t, "proxy1", &calls, doubanTestOutcome{status: 200}),
		)
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("select proxy status=%d err=%v", status, err)
		}
		disabled := false
		if _, err := apiConfig.Update(t.Context(), "douban", APIConfigPatch{UseProxyPool: &disabled}); err != nil {
			t.Fatal(err)
		}
		if _, err := apiConfig.Update(t.Context(), "douban", APIConfigPatch{UseProxyPool: &enabled}); err != nil {
			t.Fatal(err)
		}
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("reset request status=%d err=%v", status, err)
		}
		if got, want := strings.Join(calls, ","), "direct,proxy1,direct"; got != want {
			t.Fatalf("calls = %s, want %s", got, want)
		}
	})

	t.Run("resin source does not use normal pool", func(t *testing.T) {
		calls := []string{}
		provider := doubanProxyTestProvider(apiConfig,
			scriptedDoubanClient(t, "direct", &calls, doubanTestOutcome{status: 400}, doubanTestOutcome{status: 200}, doubanTestOutcome{status: 200}),
			scriptedDoubanClient(t, "normal", &calls),
		)
		resinCalls := 0
		provider.resinClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls = append(calls, "resin")
			resinCalls++
			wantURL := "http://resin.internal:2260/proxy-secret/./https/example.test/data"
			status := http.StatusOK
			if resinCalls == 2 {
				wantURL = "http://resin.internal:2260/proxy-secret/Default.douban-main/https/example.test/data"
				status = http.StatusServiceUnavailable
			}
			if got := req.URL.String(); got != wantURL {
				t.Fatalf("Resin request URL = %q, want %q", got, wantURL)
			}
			return doubanTestResponse(req, status), nil
		})}
		proxyType := ProxyPoolTypeResin
		proxyURL := "http://resin.internal:2260"
		token := "proxy-secret"
		if _, err := provider.proxyPool.UpdateConfig(t.Context(), ProxyPoolConfigPatch{
			ProxyPoolType: &proxyType, ResinProxyURL: &proxyURL, ResinProxyToken: &token,
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			normal := ProxyPoolTypeNormal
			empty, clearToken := "", "<clear>"
			if _, err := provider.proxyPool.UpdateConfig(context.Background(), ProxyPoolConfigPatch{
				ProxyPoolType: &normal, ResinProxyURL: &empty, ResinAccount: &empty, ResinProxyToken: &clearToken,
			}); err != nil {
				t.Error(err)
			}
		})
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("status=%d err=%v", status, err)
		}
		account := "douban-main"
		if _, err := provider.proxyPool.UpdateConfig(t.Context(), ProxyPoolConfigPatch{ResinAccount: &account}); err != nil {
			t.Fatal(err)
		}
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("reset status=%d err=%v", status, err)
		}
		config, err := provider.proxyPool.resolveConfig(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		provider.setRoute(0, config.Revision, provider.resolveConfig(t.Context()).Revision, 0)
		if _, status, err := provider.requestJSON(t.Context(), "https://example.test/data", ""); err != nil || status != 200 {
			t.Fatalf("Resin fallback status=%d err=%v", status, err)
		}
		if got, want := strings.Join(calls, ","), "direct,resin,direct,resin,direct"; got != want {
			t.Fatalf("calls = %s, want %s", got, want)
		}
	})

	t.Run("concurrent 400 responses keep proxy order", func(t *testing.T) {
		var directCalls atomic.Int32
		var proxy1Calls atomic.Int32
		var proxy2Calls atomic.Int32
		bothDirect := make(chan struct{})
		direct := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if directCalls.Add(1) == 2 {
				close(bothDirect)
			}
			<-bothDirect
			return doubanTestResponse(req, 400), nil
		})}
		proxy1 := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			proxy1Calls.Add(1)
			return doubanTestResponse(req, 200), nil
		})}
		proxy2 := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			proxy2Calls.Add(1)
			return doubanTestResponse(req, 200), nil
		})}
		provider := doubanProxyTestProvider(apiConfig, direct, proxy1, proxy2)

		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, status, err := provider.requestJSON(t.Context(), "https://example.test/data", "")
				if err == nil && status != 200 {
					err = errors.New("unexpected response status")
				}
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if directCalls.Load() != 2 || proxy1Calls.Load() != 2 || proxy2Calls.Load() != 0 {
			t.Fatalf("calls: direct=%d proxy1=%d proxy2=%d", directCalls.Load(), proxy1Calls.Load(), proxy2Calls.Load())
		}
	})
}

type doubanTestOutcome struct {
	status  int
	err     error
	readErr error
}

func scriptedDoubanClient(t *testing.T, name string, calls *[]string, outcomes ...doubanTestOutcome) *http.Client {
	t.Helper()
	next := 0
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		*calls = append(*calls, name)
		if next >= len(outcomes) {
			t.Fatalf("unexpected %s request", name)
		}
		outcome := outcomes[next]
		next++
		if outcome.err != nil {
			return nil, outcome.err
		}
		body := io.ReadCloser(io.NopCloser(strings.NewReader(`[]`)))
		if outcome.readErr != nil {
			body = &doubanErrorReadCloser{err: outcome.readErr}
		}
		response := doubanTestResponse(req, outcome.status)
		response.Body = body
		return response, nil
	})}
}

func doubanTestResponse(req *http.Request, status int) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`[]`)), Request: req}
}

func doubanProxyTestProvider(apiConfig *APIConfigService, direct *http.Client, proxies ...*http.Client) *DoubanProvider {
	provider := NewDoubanProvider(nil, nil)
	provider.apiConfig = apiConfig
	provider.directClient = direct
	provider.proxyPool = NewProxyPoolService(apiConfig.repo, apiConfig.crypto)
	provider.proxyPool.loaded = true
	provider.proxyPool.generation = 1
	provider.proxyPool.clients = proxies
	return provider
}

type doubanErrorReadCloser struct{ err error }

func (r *doubanErrorReadCloser) Read([]byte) (int, error) { return 0, r.err }
func (r *doubanErrorReadCloser) Close() error             { return nil }

func TestDoubanSearchAndDiscoverPreferHTTPStatusErrors(t *testing.T) {
	provider := NewDoubanProvider(nil, nil)
	provider.client = scriptedDoubanClient(t, "legacy", &[]string{},
		doubanTestOutcome{status: 400, readErr: errors.New("read failed")},
		doubanTestOutcome{status: 400, readErr: errors.New("read failed")},
	)
	if _, err := provider.Search(t.Context(), "test"); err == nil || err.Error() != "douban search: 400" {
		t.Fatalf("search error = %v", err)
	}
	if _, err := provider.Discover(t.Context(), "douban_hot_movie"); err == nil || err.Error() != "douban discover: 400" {
		t.Fatalf("discover error = %v", err)
	}
}

func newProxyPoolTestDB(t *testing.T, models ...any) *gorm.DB {
	db := newServiceTestDB(t, models...)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db
}
