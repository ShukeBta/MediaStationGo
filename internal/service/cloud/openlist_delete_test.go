package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestOpenListDeleteUsesExactParentAndFileName(t *testing.T) {
	var body struct {
		Dir   string   `json:"dir"`
		Names []string `json:"names"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/remove" {
			t.Fatalf("request path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "test-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "success"})
	}))
	defer server.Close()

	provider, err := New(TypeOpenList, map[string]any{
		"server": server.URL,
		"url":    server.URL + "/dav",
		"token":  "test-token",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	deletable, ok := provider.(DeletableProvider)
	if !ok {
		t.Fatal("OpenList provider should support exact file deletion")
	}
	if err := deletable.Delete(context.Background(), "/115/电影/Sintel/Sintel.mkv"); err != nil {
		t.Fatal(err)
	}
	if body.Dir != "/115/电影/Sintel" || !reflect.DeepEqual(body.Names, []string{"Sintel.mkv"}) {
		t.Fatalf("remove body = %+v", body)
	}
}

func TestOpenListDeleteRejectsRoot(t *testing.T) {
	provider, err := New(TypeOpenList, map[string]any{"server": "http://127.0.0.1:5244"}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.(DeletableProvider).Delete(context.Background(), "/"); err == nil {
		t.Fatal("root deletion must be rejected")
	}
}

func TestOpenListDeleteTreatsAlreadyMissingFileAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "message": "code: 430004, message: 文件不存在或已删除"})
	}))
	defer server.Close()
	provider, err := New(TypeOpenList, map[string]any{
		"server": server.URL,
		"url":    server.URL + "/dav",
		"token":  "test-token",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.(DeletableProvider).Delete(context.Background(), "/115/电影/Missing.mkv"); err != nil {
		t.Fatalf("already absent file should be idempotent: %v", err)
	}
}

func TestOpenListDeleteDoesNotHideStorageConfigurationErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "message": "storage does not exist"})
	}))
	defer server.Close()
	provider, err := New(TypeOpenList, map[string]any{
		"server": server.URL,
		"url":    server.URL + "/dav",
		"token":  "test-token",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.(DeletableProvider).Delete(context.Background(), "/115/电影/Missing.mkv"); err == nil {
		t.Fatal("storage configuration errors must not be treated as an already deleted file")
	}
}
