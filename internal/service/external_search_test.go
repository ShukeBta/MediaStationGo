package service

import "testing"

func TestBuildSubscribeKeyword(t *testing.T) {
	if got := buildSubscribeKeyword("沙丘", 2024); got != "沙丘 2024" {
		t.Fatalf("keyword = %q", got)
	}
	if got := buildSubscribeKeyword("沙丘", 0); got != "沙丘" {
		t.Fatalf("keyword without year = %q", got)
	}
	if got := buildSubscribeKeyword("", 2024); got != "" {
		t.Fatalf("empty keyword = %q, want empty", got)
	}
}

func TestBuildSubscribeAliasesIncludesOriginalTitleWithYear(t *testing.T) {
	got := buildSubscribeAliases("玩具总动员 5", "Toy Story 5", 2026)
	for _, want := range []string{"玩具总动员 5", "Toy Story 5", "玩具总动员 5 2026", "Toy Story 5 2026"} {
		if !containsString(got, want) {
			t.Fatalf("aliases = %#v, missing %q", got, want)
		}
	}
	if containsString(got, "2026") {
		t.Fatalf("aliases = %#v, must not include bare year", got)
	}
}

func TestDedupeExternalMedia(t *testing.T) {
	in := []ExternalMediaResult{
		{Source: "tmdb", MediaType: "movie", TMDbID: 1, Title: "A"},
		{Source: "tmdb", MediaType: "movie", TMDbID: 1, Title: "A duplicate"},
		{Source: "douban", DoubanID: "2", Title: "B"},
	}
	got := dedupeExternalMedia(in)
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
}
