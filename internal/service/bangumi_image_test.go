package service

import "testing"

func TestNormalizeBangumiImageURLUsesHTTPSResizedCover(t *testing.T) {
	got := normalizeBangumiImageURL("http://lain.bgm.tv/pic/cover/l/27/ff/377130_wDU1x.jpg")
	want := "https://lain.bgm.tv/r/400/pic/cover/l/27/ff/377130_wDU1x.jpg"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestNormalizeBangumiImageURLLeavesExternalHostsAlone(t *testing.T) {
	raw := "https://image.tmdb.org/t/p/w500/poster.jpg"
	if got := normalizeBangumiImageURL(raw); got != raw {
		t.Fatalf("url = %q, want %q", got, raw)
	}
}
