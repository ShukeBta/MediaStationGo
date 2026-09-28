package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestDiscoverWindowStartMapsLogicalPagesWithoutSkipping(t *testing.T) {
	tests := []struct {
		page           int
		pageSize       int
		sourcePageSize int
		wantPage       int
		wantOffset     int
	}{
		{page: 1, pageSize: 18, sourcePageSize: 20, wantPage: 1, wantOffset: 0},
		{page: 2, pageSize: 18, sourcePageSize: 20, wantPage: 1, wantOffset: 18},
		{page: 3, pageSize: 18, sourcePageSize: 20, wantPage: 2, wantOffset: 16},
		{page: 3, pageSize: 18, sourcePageSize: 40, wantPage: 1, wantOffset: 36},
	}
	for _, test := range tests {
		page, offset := discoverWindowStart(test.page, test.pageSize, test.sourcePageSize)
		if page != test.wantPage || offset != test.wantOffset {
			t.Fatalf("page %d => source page=%d offset=%d, want %d/%d", test.page, page, offset, test.wantPage, test.wantOffset)
		}
	}
}

func TestTMDbSectionWindowContinuesAcrossSourcePages(t *testing.T) {
	provider := NewTMDbProvider(&config.Config{Secrets: config.SecretsConfig{TMDbAPIKey: "test-key"}}, zap.NewNop(), nil)
	discover := NewDiscoverService(zap.NewNop(), provider)
	requestedPages := make([]int, 0, 2)
	discover.client = &http.Client{Transport: discoverSearchRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		page, _ := strconv.Atoi(req.URL.Query().Get("page"))
		requestedPages = append(requestedPages, page)
		results := make([]map[string]any, 0, 20)
		for index := 1; index <= 20; index++ {
			id := (page-1)*20 + index
			results = append(results, map[string]any{"id": id, "title": "Item " + strconv.Itoa(id)})
		}
		body, err := json.Marshal(map[string]any{"results": results})
		if err != nil {
			return nil, err
		}
		recorder := httptest.NewRecorder()
		recorder.Header().Set("Content-Type", "application/json")
		_, _ = recorder.Write(body)
		return recorder.Result(), nil
	})}

	items, err := discover.TMDbSectionWindow(t.Context(), "tmdb_latest_movie", 2, 18)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 19 || items[0].TMDbID != 19 || items[18].TMDbID != 37 {
		t.Fatalf("items = %#v", items)
	}
	if !reflect.DeepEqual(requestedPages, []int{1, 2}) {
		t.Fatalf("requested pages = %v, want [1 2]", requestedPages)
	}
}

func TestDoubanDiscoverWindowUsesLogicalOffsetAndProbe(t *testing.T) {
	provider := NewDoubanProvider(&config.Config{}, zap.NewNop())
	provider.client = &http.Client{Transport: discoverSearchRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.Query().Get("page_start"); got != "18" {
			t.Fatalf("page_start = %q, want 18", got)
		}
		if got := req.URL.Query().Get("page_limit"); got != "19" {
			t.Fatalf("page_limit = %q, want 19", got)
		}
		recorder := httptest.NewRecorder()
		recorder.Header().Set("Content-Type", "application/json")
		_, _ = recorder.WriteString(`{"subjects":[]}`)
		return recorder.Result(), nil
	})}

	if _, err := provider.DiscoverWindow(t.Context(), "douban_hot_movie", 2, 18); err != nil {
		t.Fatal(err)
	}
}

func TestBangumiCalendarKeepsAllItemsForLogicalPaging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		items := make([]map[string]any, 0, 25)
		for index := 1; index <= 25; index++ {
			items = append(items, map[string]any{"id": index, "name": fmt.Sprintf("Item %d", index)})
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"items": items}})
	}))
	defer server.Close()

	provider := NewBangumiProvider(&config.Config{}, zap.NewNop())
	provider.base = server.URL
	items, err := provider.Calendar(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 25 {
		t.Fatalf("calendar items = %d, want 25", len(items))
	}
}

func TestJavDBPopularKeepsFullRankingForLogicalPaging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for index := 1; index <= 31; index++ {
			_, _ = fmt.Fprintf(w, `<a class="box" href="/v/%d"><strong>TEST-%03d</strong></a>`, index, index)
		}
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	items, err := provider.DiscoverJavDBPopular(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 31 {
		t.Fatalf("ranking items = %d, want 31", len(items))
	}
}

func TestFollowedAdultWorksWindowContinuesAcrossSourcePages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			page, _ = strconv.Atoi(raw)
		}
		count := 40
		if page == 2 {
			count = 20
		} else if page > 2 {
			count = 0
		}
		start := (page-1)*40 + 1
		for index := 0; index < count; index++ {
			number := start + index
			_, _ = fmt.Fprintf(w, `<a class="box" href="/v/%d"><strong>TEST-%03d</strong></a>`, number, number)
		}
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	items, err := provider.DiscoverFollowedPerformerWorksWindow(t.Context(), []model.AdultPerformerFollow{{
		Name: "Actor", Source: "javdb", SourceID: "Actor1",
	}}, 3, 18)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 19 || items[0].OriginalName != "TEST-037" || items[18].OriginalName != "TEST-055" {
		t.Fatalf("window items = %#v", items)
	}
}

func TestFollowedAdultWorksWindowHonorsShortPageNextLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			page, _ = strconv.Atoi(raw)
		}
		count := 39
		start := 1
		if page == 2 {
			count = 20
			start = 40
		} else if page > 2 {
			count = 0
		}
		for index := 0; index < count; index++ {
			number := start + index
			_, _ = fmt.Fprintf(w, `<a class="box" href="/v/%d"><strong>TEST-%03d</strong></a>`, number, number)
		}
		if page == 1 {
			_, _ = w.Write([]byte(`<a rel="next" href="/actors/Actor1?page=2">next</a>`))
		}
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	items, err := provider.DiscoverFollowedPerformerWorksWindow(t.Context(), []model.AdultPerformerFollow{{
		Name: "Actor", Source: "javdb", SourceID: "Actor1",
	}}, 3, 18)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 19 || items[0].OriginalName != "TEST-037" || items[18].OriginalName != "TEST-055" {
		t.Fatalf("window items = %#v", items)
	}
}
