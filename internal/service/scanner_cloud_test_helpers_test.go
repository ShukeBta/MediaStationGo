package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type openListTestEntry struct {
	Name  string
	Size  int64
	IsDir bool
}

func newOpenListAPIServer(t *testing.T, list func(path string, page, perPage int) ([]openListTestEntry, int)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/list" {
			t.Fatalf("unexpected openlist api request %s", r.URL.Path)
		}
		var in struct {
			Path    string `json:"path"`
			Page    int    `json:"page"`
			PerPage int    `json:"per_page"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("decode openlist list request: %v", err)
		}
		if in.Path == "" {
			in.Path = "/"
		}
		if in.Page <= 0 {
			in.Page = 1
		}
		if in.PerPage <= 0 {
			in.PerPage = 500
		}
		entries, total := list(in.Path, in.Page, in.PerPage)
		content := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			content = append(content, map[string]any{
				"name":   entry.Name,
				"size":   entry.Size,
				"is_dir": entry.IsDir,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "success",
			"data": map[string]any{
				"content": content,
				"total":   total,
			},
		})
	}))
}
