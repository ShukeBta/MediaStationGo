package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type OpenSearchMediaBackend struct {
	baseURL  string
	index    string
	username string
	password string
	client   *http.Client
}

func NewOpenSearchMediaBackend(cfg config.SearchConfig) *OpenSearchMediaBackend {
	if strings.TrimSpace(cfg.Backend) != "opensearch" || strings.TrimSpace(cfg.OpenSearchURL) == "" {
		return nil
	}
	index := strings.TrimSpace(cfg.Index)
	if index == "" {
		index = "mediastation_media"
	}
	return &OpenSearchMediaBackend{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.OpenSearchURL), "/"),
		index:    index,
		username: strings.TrimSpace(cfg.Username),
		password: cfg.Password,
		client:   &http.Client{Timeout: 4 * time.Second},
	}
}

func (b *OpenSearchMediaBackend) SearchMediaIDs(ctx context.Context, query string, offset, limit int, filter MediaQueryFilter) ([]string, int64, error) {
	if b == nil || b.client == nil || b.baseURL == "" || b.index == "" {
		return nil, 0, fmt.Errorf("opensearch backend not configured")
	}
	if limit <= 0 {
		limit = 50
	}
	must := []any{
		map[string]any{
			"multi_match": map[string]any{
				"query":     query,
				"fields":    []string{"title^6", "original_name^4", "actors^3", "genres^2", "search_pinyin^2", "search_initials^2", "path"},
				"type":      "best_fields",
				"operator":  "and",
				"fuzziness": "AUTO",
			},
		},
	}
	filters := []any{
		map[string]any{"term": map[string]any{"deleted": false}},
	}
	if !filter.IncludeNSFW {
		filters = append(filters, map[string]any{"term": map[string]any{"nsfw": false}})
	}
	if len(filter.AllowedLibraryIDs) > 0 {
		filters = append(filters, map[string]any{"terms": map[string]any{"library_id": filter.AllowedLibraryIDs}})
	}
	if len(filter.HiddenLibraryIDs) > 0 {
		filters = append(filters, map[string]any{"bool": map[string]any{
			"must_not": []any{map[string]any{"terms": map[string]any{"library_id": filter.HiddenLibraryIDs}}},
		}})
	}
	body := map[string]any{
		"from": offset,
		"size": limit,
		"_source": []string{
			"id",
		},
		"query": map[string]any{
			"bool": map[string]any{
				"must":   must,
				"filter": filters,
			},
		},
	}
	var resp struct {
		Hits struct {
			Total any `json:"total"`
			Hits  []struct {
				ID     string `json:"_id"`
				Source struct {
					ID string `json:"id"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := b.doJSON(ctx, http.MethodPost, "/"+url.PathEscape(b.index)+"/_search", body, &resp); err != nil {
		return nil, 0, err
	}
	ids := make([]string, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		id := strings.TrimSpace(hit.Source.ID)
		if id == "" {
			id = strings.TrimSpace(hit.ID)
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, openSearchTotal(resp.Hits.Total), nil
}

func (b *OpenSearchMediaBackend) EnsureIndex(ctx context.Context) error {
	if err := b.do(ctx, http.MethodHead, "/"+url.PathEscape(b.index), nil, "", nil); err == nil {
		return nil
	}
	mapping := map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"id":            map[string]any{"type": "keyword"},
				"library_id":    map[string]any{"type": "keyword"},
				"title":         map[string]any{"type": "text"},
				"original_name": map[string]any{"type": "text"},
				"path":          map[string]any{"type": "text"},
				"genres":        map[string]any{"type": "text"},
				"actors":        map[string]any{"type": "text"},
				"search_pinyin": map[string]any{"type": "text"},
				"search_initials": map[string]any{"type": "text"},
				"nsfw":          map[string]any{"type": "boolean"},
				"deleted":       map[string]any{"type": "boolean"},
				"created_at":    map[string]any{"type": "date"},
			},
		},
	}
	return b.doJSON(ctx, http.MethodPut, "/"+url.PathEscape(b.index), mapping, nil)
}

func (b *OpenSearchMediaBackend) IndexMedia(ctx context.Context, rows []model.Media) error {
	if len(rows) == 0 {
		return nil
	}
	var bulk bytes.Buffer
	enc := json.NewEncoder(&bulk)
	for _, row := range rows {
		if err := enc.Encode(map[string]any{"index": map[string]any{"_index": b.index, "_id": row.ID}}); err != nil {
			return err
		}
		if err := enc.Encode(map[string]any{
			"id":            row.ID,
			"library_id":    row.LibraryID,
			"title":         row.Title,
			"original_name": row.OriginalName,
			"path":          row.Path,
			"genres":        row.Genres,
			"actors":        row.Actors,
			"search_pinyin": row.SearchPinyin,
			"search_initials": row.SearchInitials,
			"nsfw":          row.NSFW,
			"deleted":       row.DeletedAt.Valid,
			"created_at":    row.CreatedAt,
		}); err != nil {
			return err
		}
	}
	return b.do(ctx, http.MethodPost, "/_bulk", &bulk, "application/x-ndjson", nil)
}

func (b *OpenSearchMediaBackend) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	return b.do(ctx, method, path, reader, "application/json", out)
}

func (b *OpenSearchMediaBackend) do(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, b.baseURL+path, body)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if b.username != "" {
		req.SetBasicAuth(b.username, b.password)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("opensearch %s %s returned %d", method, path, resp.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func openSearchTotal(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case map[string]any:
		if n, ok := v["value"].(float64); ok {
			return int64(n)
		}
	}
	return 0
}
