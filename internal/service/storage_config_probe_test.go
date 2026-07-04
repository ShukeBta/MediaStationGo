package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStorageConfigOpenListHTTPSAgainstHTTPHint(t *testing.T) {
	openlist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	defer openlist.Close()

	_, storage := newStorageUploadTestService(t)
	badHTTPS := "https://" + strings.TrimPrefix(openlist.URL, "http://")
	err := storage.Test(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": badHTTPS,
		},
	})
	if err == nil {
		t.Fatal("want protocol mismatch error")
	}
	if !strings.Contains(err.Error(), "请改用 http://") || !strings.Contains(err.Error(), "server gave HTTP response to HTTPS client") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStorageConfigOpenListTestRejectsUnauthorizedList(t *testing.T) {
	openlist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dav" {
			t.Fatalf("unexpected openlist path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
	}))
	defer openlist.Close()

	_, storage := newStorageUploadTestService(t)
	err := storage.Test(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": openlist.URL,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "http 401") {
		t.Fatalf("openlist unauthorized probe error = %v, want http 401", err)
	}
}

func TestStorageConfigOpenListTestUsesAPIListWithToken(t *testing.T) {
	var listed bool
	openlist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/list" {
			t.Fatalf("unexpected openlist path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "openlist-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		listed = true
		_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
	}))
	defer openlist.Close()

	_, storage := newStorageUploadTestService(t)
	if err := storage.Test(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": openlist.URL,
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatalf("openlist API probe: %v", err)
	}
	if !listed {
		t.Fatal("openlist test should probe /api/fs/list")
	}
}

func TestStorageConfigAlistTestRejectsUnauthorized(t *testing.T) {
	alist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me" {
			t.Fatalf("unexpected alist path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer alist.Close()

	_, storage := newStorageUploadTestService(t)
	err := storage.Test(t.Context(), StorageInput{
		Type: "alist",
		Config: map[string]any{
			"server": alist.URL,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("alist unauthorized probe error = %v, want authentication failed", err)
	}
}

func TestStorageConfigCloudProviderRejectsDisabledConfig(t *testing.T) {
	_, storage := newStorageUploadTestService(t)
	enabled := false
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"url": "http://127.0.0.1:5244/dav",
		},
		Enabled: &enabled,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := storage.CloudProvider(t.Context(), "openlist")
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled provider error = %v, want disabled", err)
	}
}

func TestStorageConfigSavePreservesExistingSecretWhenFormLeavesItBlank(t *testing.T) {
	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": "http://openlist.test",
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server":          "http://openlist.test",
			"token":           "",
			"timeout_seconds": "180",
		},
	}); err != nil {
		t.Fatal(err)
	}
	view, err := storage.Get(t.Context(), "openlist")
	if err != nil {
		t.Fatal(err)
	}
	if view.Config["token"] != "openlist-token" {
		t.Fatalf("token = %#v, want preserved token", view.Config["token"])
	}
	if view.Config["timeout_seconds"] != "180" {
		t.Fatalf("timeout_seconds = %#v, want updated timeout", view.Config["timeout_seconds"])
	}
}
