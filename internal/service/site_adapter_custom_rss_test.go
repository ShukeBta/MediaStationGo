package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestCustomRSSSearchPreservesDownloadIdentity(t *testing.T) {
	const download = "https://tracker.example/download?id=42&passkey=test"
	feed := `<rss version="2.0"><channel>
	<item>
	 <title><![CDATA[Show S01E01 & Friends]]></title>
	 <link>https://tracker.example/details?id=42</link>
	 <enclosure url="https://tracker.example/download?id=42&amp;passkey=test" length="12345" type="application/x-bittorrent" />
	 <description><![CDATA[<p>国语 1.5 GB</p>]]></description>
	 <category>TV</category>
	 <pubDate>Thu, 01 Oct 2026 10:00:00 +0800</pubDate>
	</item>
	</channel></rss>`
	result, err := parseRSSXML([]byte(feed), "RSS", "show")
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("multiline feed was not parsed: %+v %v", result, err)
	}
	item := result.Items[0]
	if item.ID != download || item.DownloadURL != download || item.DetailURL == download || item.Title != "Show S01E01 & Friends" || item.Size != 12345 || item.UploadTime.IsZero() {
		t.Fatalf("download identity or metadata lost: %+v", item)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, feed)
	}))
	t.Cleanup(server.Close)
	db := newServiceTestDB(t, &model.Site{})
	sites := NewSiteService(zap.NewNop(), repository.New(db), "")
	site := model.Site{Name: "RSS", Type: "custom_rss", URL: server.URL, Enabled: true}
	if err := sites.Create(t.Context(), &site); err != nil {
		t.Fatal(err)
	}
	results, err := sites.Search(t.Context(), "Show")
	if err != nil || len(results) != 1 {
		t.Fatalf("RSS search failed: %+v %v", results, err)
	}
	// This is the strict resolution path used by automatic PT subscriptions.
	resolved, err := sites.DownloadURL(t.Context(), results[0].SiteID, results[0].ID, "")
	if err != nil || resolved != download {
		t.Fatalf("RSS subscription download = %q, %v", resolved, err)
	}
	// Older interactive caches carried ordinal IDs. Their supplied URL still
	// works, while an ordinal without any URL must not reach the downloader.
	resolved, err = sites.DownloadURL(t.Context(), site.ID, "0", download)
	if err != nil || resolved != download {
		t.Fatalf("legacy RSS cache fallback = %q, %v", resolved, err)
	}
	if _, err := sites.DownloadURL(t.Context(), site.ID, "0", ""); err == nil {
		t.Fatal("ordinal RSS identity must be rejected")
	}
}

func TestCustomRSSLinkOnlyAndInvalidXML(t *testing.T) {
	result, err := parseRSSXML([]byte(`<rss><channel><item><title>Show</title><link>https://tracker.example/file.torrent</link><description>2 GB</description></item></channel></rss>`), "RSS", "")
	if err != nil || len(result.Items) != 1 || result.Items[0].ID != "https://tracker.example/file.torrent" || result.Items[0].Size != 2*1024*1024*1024 {
		t.Fatalf("link-only RSS failed: %+v %v", result, err)
	}
	if _, err := parseRSSXML([]byte(`<html>verification required</html>`), "RSS", ""); err == nil {
		t.Fatal("upstream verification page must not look like an empty feed")
	}
}
