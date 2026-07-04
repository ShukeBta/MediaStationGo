package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestSelectRSSSubscriptionCandidatesDefaultKeepsOnlyBestWebDLPerEpisode(t *testing.T) {
	sub := &model.Subscription{Name: "House of the Dragon 自动订阅", Filter: "House of the Dragon", MediaType: "tv"}
	items := []rssItem{
		rssSubscriptionItem("House of the Dragon S03E01 1080p WEB-DL H264 AAC", "https://pt/download/e01-1080"),
		rssSubscriptionItem("House of the Dragon S03E01 2160p WEB-DL H264 AAC", "https://pt/download/e01-2160"),
		rssSubscriptionItem("House of the Dragon S03E01 720p WEBRip H264 AAC", "https://pt/download/e01-720"),
		rssSubscriptionItem("House of the Dragon S03E02 1080p HDTV H264 AAC", "https://pt/download/e02-hdtv"),
		rssSubscriptionItem("House of the Dragon S03E02 1080p WEB-DL H264 AAC", "https://pt/download/e02-webdl"),
	}

	got := selectRSSSubscriptionCandidates(items, sub, compileFilter(sub.Filter), nil, LocalAvailability{})
	if len(got) != 2 {
		t.Fatalf("selected %d candidates, want one best release per episode", len(got))
	}
	if got[0].Download != "https://pt/download/e01-2160" {
		t.Fatalf("episode 1 selected %q, want 2160p WEB-DL", got[0].Download)
	}
	if got[1].Download != "https://pt/download/e02-webdl" {
		t.Fatalf("episode 2 selected %q, want WEB-DL over HDTV", got[1].Download)
	}
}

func rssSubscriptionItem(title, download string) rssItem {
	item := rssItem{Title: title, Link: download, GUID: download}
	item.Enclosure.URL = download
	return item
}
