package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestRefreshAdultSectionsStoresJavDBAndAllFC2SortCaches(t *testing.T) {
	javdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for index := 1; index <= 25; index++ {
			_, _ = fmt.Fprintf(w, `<a class="box" href="/v/%d"><strong>TEST-%03d</strong></a>`, index, index)
		}
	}))
	defer javdb.Close()
	withAdultDefaultBases(t, []string{javdb.URL})

	flare := newFD2PPVFlareServer(t, func(target string) string {
		if !strings.Contains(target, "size=48") || !strings.Contains(target, "page=1") {
			t.Fatalf("unexpected FC2 target URL = %q", target)
		}
		return fd2PPVMovieCards(20)
	})
	defer flare.Close()

	adult := NewAdultProvider(zap.NewNop(), nil)
	adult.SetFlareSolverr(flare.URL, 5)
	discover := NewDiscoverService(zap.NewNop(), nil)
	if err := discover.RefreshAdultSections(t.Context(), adult); err != nil {
		t.Fatal(err)
	}

	javItems, ok := discover.CachedSection("adult_javdb_popular", 1)
	if !ok || len(javItems) != 19 {
		t.Fatalf("JavDB cache = %d, ok=%v; want 19", len(javItems), ok)
	}
	for _, sortKey := range adultDiscoverRefreshSorts {
		items, ok := discover.CachedSection("adult_fd2ppv:"+sortKey, 1)
		if !ok || len(items) != 19 {
			t.Fatalf("FC2 %s cache = %d, ok=%v; want 19", sortKey, len(items), ok)
		}
	}
}
