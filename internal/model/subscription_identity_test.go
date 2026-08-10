package model

import "testing"

func TestSubscriptionIdentityKeyNormalizesEquivalentRules(t *testing.T) {
	base := Subscription{
		Name:          " Example Show ",
		FeedURL:       "site-search://search?keyword=Example",
		Filter:        " Example.*Show ",
		MediaType:     "TV",
		MediaCategory: " 欧美剧 ",
		SearchMode:    "KEYWORD",
		Resolution:    "1080P",
		Effects:       " HDR, Atmos ",
		ExcludeWords:  " CAM, TS ",
		WashPriority:  "Balanced",
		Priority:      50,
	}
	equivalent := base
	equivalent.Name = "example show"
	equivalent.MediaType = "tv"
	equivalent.MediaCategory = "欧美剧"
	equivalent.SearchMode = "keyword"
	equivalent.Resolution = "1080p"
	equivalent.Effects = "hdr,atmos"
	equivalent.ExcludeWords = "cam,ts"
	equivalent.WashPriority = "balanced"
	if got, want := SubscriptionIdentityKey(&equivalent), SubscriptionIdentityKey(&base); got != want {
		t.Fatalf("equivalent keys differ: %s != %s", got, want)
	}

	differentRule := base
	differentRule.Resolution = "2160p"
	if SubscriptionIdentityKey(&differentRule) == SubscriptionIdentityKey(&base) {
		t.Fatal("different resolution should produce a different identity")
	}

	displayOnly := base
	displayOnly.PosterURL = "https://example.test/poster.jpg"
	displayOnly.Overview = "new overview"
	displayOnly.TotalEpisodes = 24
	if SubscriptionIdentityKey(&displayOnly) != SubscriptionIdentityKey(&base) {
		t.Fatal("display/runtime metadata should not change identity")
	}
}
