package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestDownloadClientCreateNormalizesHostAndClearsDefault(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewDownloadClientService(zap.NewNop(), repos)

	first, err := svc.Create(t.Context(), DownloadClientInput{
		Name:      "qB old",
		Type:      "qbittorrent",
		Host:      "http://127.0.0.1:8080/",
		IsDefault: true,
		Enabled:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(t.Context(), DownloadClientInput{
		Name:      "qB NAS",
		Type:      "qbittorrent",
		Host:      "172.17.0.1:8085",
		IsDefault: true,
		Enabled:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Host != "http://172.17.0.1:8085" {
		t.Fatalf("host = %q, want normalized http URL", second.Host)
	}
	refreshedFirst, err := repos.DownloadClient.FindByID(t.Context(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedFirst == nil || refreshedFirst.IsDefault {
		t.Fatalf("old default should be cleared, got %#v", refreshedFirst)
	}
	refreshedSecond, err := repos.DownloadClient.FindByID(t.Context(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedSecond == nil || !refreshedSecond.IsDefault {
		t.Fatalf("new default should be active, got %#v", refreshedSecond)
	}
}

func TestDownloadClientCreateMakesFirstEnabledClientDefault(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewDownloadClientService(zap.NewNop(), repos)

	client, err := svc.Create(t.Context(), DownloadClientInput{
		Name:    "qB",
		Type:    "qbittorrent",
		Host:    "127.0.0.1:8080",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !client.IsDefault {
		t.Fatalf("first enabled client should become default: %#v", client)
	}
}

func TestDownloadClientRejectsUnsupportedHostScheme(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadClient{}, &model.Setting{})
	svc := NewDownloadClientService(zap.NewNop(), repository.New(db))

	if _, err := svc.Create(t.Context(), DownloadClientInput{
		Name:    "bad",
		Type:    "qbittorrent",
		Host:    "ftp://127.0.0.1:8080",
		Enabled: true,
	}); err == nil {
		t.Fatal("expected unsupported scheme error")
	}
}

func TestDownloadClientRejectsUnsafeEndpointParts(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadClient{}, &model.Setting{})
	svc := NewDownloadClientService(zap.NewNop(), repository.New(db))

	for _, host := range []string{
		"http://user:pass@127.0.0.1:6800",
		"http://127.0.0.1:6800/jsonrpc?target=http://169.254.169.254",
		"http://127.0.0.1:6800/jsonrpc#fragment",
		"http://127.0.0.1:70000",
		"file:///etc/passwd",
	} {
		if _, err := svc.Create(t.Context(), DownloadClientInput{
			Name:    "bad",
			Type:    "aria2",
			Host:    host,
			Enabled: true,
		}); err == nil {
			t.Fatalf("Create allowed unsafe host %q", host)
		}
	}
}

func TestDownloadClientRPCURLAppendsExpectedPath(t *testing.T) {
	cases := []struct {
		clientType string
		host       string
		want       string
	}{
		{"aria2", "127.0.0.1:6800", "http://127.0.0.1:6800/jsonrpc"},
		{"aria2", "http://nas.local:6800/rpc", "http://nas.local:6800/rpc/jsonrpc"},
		{"transmission", "http://nas.local:9091", "http://nas.local:9091/transmission/rpc"},
		{"transmission", "http://nas.local:9091/transmission/rpc", "http://nas.local:9091/transmission/rpc"},
	}
	for _, tc := range cases {
		got, err := downloadClientRPCURL(tc.clientType, tc.host)
		if err != nil {
			t.Fatalf("downloadClientRPCURL(%q, %q) error: %v", tc.clientType, tc.host, err)
		}
		if got != tc.want {
			t.Fatalf("downloadClientRPCURL(%q, %q) = %q, want %q", tc.clientType, tc.host, got, tc.want)
		}
	}
}

func TestDownloadClientTestAria2RejectsJSONRPCError(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var request aria2Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error": map[string]any{
				"code":    1,
				"message": "unauthorized",
			},
		})
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewDownloadClientService(zap.NewNop(), repos)
	client, err := svc.Create(t.Context(), DownloadClientInput{
		Name:     "aria2",
		Type:     "aria2",
		Host:     server.URL,
		Password: "wrong-secret",
		Enabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Test(t.Context(), client.ID); err == nil {
		t.Fatal("expected aria2 JSON-RPC error to fail the connection test")
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("aria2 connection test method = %q, want POST", gotMethod)
	}
}

func TestDownloadClientTestTransmissionRejectsUnauthorizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewDownloadClientService(zap.NewNop(), repos)
	client, err := svc.Create(t.Context(), DownloadClientInput{
		Name:     "transmission",
		Type:     "transmission",
		Host:     server.URL,
		Username: "user",
		Password: "wrong-password",
		Enabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Test(t.Context(), client.ID); err == nil {
		t.Fatal("expected Transmission 401 response to fail the connection test")
	}
}

func TestDownloadClientEncryptsPasswordAndDecryptsItForConnectionTest(t *testing.T) {
	var gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request aria2Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(request.Params) > 0 {
			gotToken, _ = request.Params[0].(string)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  map[string]any{"version": "1.37"},
		})
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	crypto := NewCryptoService("download-client-test-secret", zap.NewNop())
	svc := NewDownloadClientService(zap.NewNop(), repos, crypto)
	client, err := svc.Create(t.Context(), DownloadClientInput{
		Name:     "aria2",
		Type:     "aria2",
		Host:     server.URL,
		Password: "rpc-secret",
		Enabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	stored, err := repos.DownloadClient.FindByID(t.Context(), client.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.Password == "rpc-secret" || !crypto.IsEncrypted(stored.Password) {
		t.Fatalf("password was not encrypted at rest: %#v", stored)
	}
	if err := svc.Test(t.Context(), client.ID); err != nil {
		t.Fatal(err)
	}
	if gotToken != "token:rpc-secret" {
		t.Fatalf("aria2 token = %q, want decrypted password", gotToken)
	}

	if _, err := svc.Update(t.Context(), client.ID, DownloadClientInput{
		Name:    "aria2 renamed",
		Type:    "aria2",
		Host:    server.URL,
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := repos.DownloadClient.FindByID(t.Context(), client.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil || updated.Password != stored.Password {
		t.Fatalf("blank password update should preserve ciphertext: before=%q after=%#v", stored.Password, updated)
	}
}

func TestAria2AdapterRejectsUnsafeHostBeforeHTTPRequest(t *testing.T) {
	adapter := NewAria2Adapter()
	called := false
	adapter.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}

	if err := adapter.Initialize(t.Context(), DownloadClientConfig{
		Host:     "http://user:pass@127.0.0.1:6800",
		Password: "secret",
	}); err == nil {
		t.Fatal("expected unsafe host error")
	}
	if called {
		t.Fatal("unsafe aria2 host should be rejected before any HTTP request")
	}
}

func TestAria2AdapterUsesNormalizedRPCURL(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var request aria2Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  map[string]any{"version": "1.37"},
		})
	}))
	defer server.Close()

	adapter := NewAria2Adapter()
	if err := adapter.Initialize(t.Context(), DownloadClientConfig{
		Host:     server.URL,
		Password: "secret",
	}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/jsonrpc" {
		t.Fatalf("aria2 request path = %q, want /jsonrpc", gotPath)
	}
}

func TestDownloadClientDeleteClearsLegacyQBitConnectionWhenNoDefault(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	for key, value := range map[string]string{
		"qbittorrent.url":      "http://127.0.0.1:8080",
		"qbittorrent.username": "admin",
		"qbittorrent.password": "admin",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewDownloadClientService(zap.NewNop(), repos)
	row, err := svc.Create(t.Context(), DownloadClientInput{
		Name:      "qB",
		Type:      "qbittorrent",
		Host:      "http://127.0.0.1:8080",
		IsDefault: true,
		Enabled:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(t.Context(), row.ID); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"qbittorrent.url", "qbittorrent.username", "qbittorrent.password"} {
		value, err := repos.Setting.Get(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}
		if value != "" {
			t.Fatalf("%s = %q, want cleared", key, value)
		}
	}
}

func TestDownloadClientUpdateClearsLegacyQBitConnectionWhenDefaultDisabled(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.url", "http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	svc := NewDownloadClientService(zap.NewNop(), repos)
	row, err := svc.Create(t.Context(), DownloadClientInput{
		Name:      "qB",
		Type:      "qbittorrent",
		Host:      "http://127.0.0.1:8080",
		IsDefault: true,
		Enabled:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Update(t.Context(), row.ID, DownloadClientInput{
		Name:      "qB",
		Type:      "qbittorrent",
		Host:      "http://127.0.0.1:8080",
		IsDefault: false,
		Enabled:   false,
	}); err != nil {
		t.Fatal(err)
	}

	value, err := repos.Setting.Get(t.Context(), "qbittorrent.url")
	if err != nil {
		t.Fatal(err)
	}
	if value != "" {
		t.Fatalf("qbittorrent.url = %q, want cleared", value)
	}
}
