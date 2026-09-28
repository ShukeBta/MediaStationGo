package service

import "testing"

func TestCloudMetadataNeedsExternalEnrichTriggersOnMissingRating(t *testing.T) {
	// 回归：NFO 有 tmdb_id + 标题 + 海报 + 简介但评分缺失（tMM 嵌套 ratings
	// 读不出时 rating=0），也必须触发外部 enrich 以从 TMDB 补评分。
	meta := &LocalMetadata{
		Title:     "月球",
		TMDbID:    17431,
		PosterURL: "https://example.com/poster.jpg",
		BackdropURL: "https://example.com/backdrop.jpg",
		Overview:  "月球上的孤独。",
		Rating:    0,
	}
	if !cloudMetadataNeedsExternalEnrich(meta) {
		t.Fatal("expected enrich to trigger when rating is missing, got false")
	}
}

func TestCloudMetadataNeedsExternalEnrichSkipsWhenRatingPresent(t *testing.T) {
	meta := &LocalMetadata{
		Title:       "月球",
		TMDbID:      17431,
		PosterURL:   "https://example.com/poster.jpg",
		BackdropURL: "https://example.com/backdrop.jpg",
		Overview:    "月球上的孤独。",
		Rating:      7.6,
	}
	if cloudMetadataNeedsExternalEnrich(meta) {
		t.Fatal("expected enrich to skip when all fields including rating are present")
	}
}

func TestCloudMetadataNeedsExternalEnrichSkipsWithoutExternalID(t *testing.T) {
	// 没有外部 ID 时，即便评分缺失也不应触发 enrich（无 ID 无法按 ID 查 TMDB）。
	meta := &LocalMetadata{
		Title:  "某部无 ID 电影",
		Rating: 0,
	}
	if cloudMetadataNeedsExternalEnrich(meta) {
		t.Fatal("expected no enrich without external ID")
	}
}
