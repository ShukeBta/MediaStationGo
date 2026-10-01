package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestSiteSearchKeepsTorrentIDForAuthenticatedDownload(t *testing.T) {
	// Several trackers need the opaque torrent ID to issue a download token;
	// the display URL is not itself downloadable and need not contain ?id=.
	site := model.Site{Base: model.Base{ID: "tracker"}, Name: "PT"}
	items := siteSearchResultsFromItems(site, &SiteSearchResult{Items: []TorrentItem{{
		ID: "resource-42", Title: "Show S01E01", DetailURL: "https://tracker.example/torrents/resource-42",
		PosterURL: "https://tracker.example/cover.jpg", Labels: "国语", Seeders: 10,
	}}}, "Show")
	if len(items) != 1 || items[0].ID != "resource-42" || items[0].SiteID != "tracker" || items[0].SearchKeyword != "Show" || items[0].PosterURL == "" {
		t.Fatalf("search lost the identity required for download/subscribe: %+v", items)
	}
}
