package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestApplyLocalMetadataPreservesPathHintAsPending(t *testing.T) {
	media := &model.Media{Title: "原始标题", ScrapeStatus: "pending"}
	applyLocalMetadata(media, &LocalMetadata{
		Title:    "路径标题",
		Year:     2026,
		TMDbID:   12345,
		PathHint: true,
	})

	if media.Title != "路径标题" || media.Year != 2026 || media.TMDbID != 12345 {
		t.Fatalf("path hint metadata was not applied: %+v", media)
	}
	if media.ScrapeStatus != "pending" {
		t.Fatalf("path hints alone must stay enrichable, got scrape_status=%q", media.ScrapeStatus)
	}
}

func TestApplyLocalMetadataMarksNFOAndDescriptiveMetadataMatched(t *testing.T) {
	nfoMedia := &model.Media{ScrapeStatus: "pending"}
	applyLocalMetadata(nfoMedia, &LocalMetadata{HasNFO: true})
	if nfoMedia.ScrapeStatus != "matched" {
		t.Fatalf("NFO metadata should mark matched, got %q", nfoMedia.ScrapeStatus)
	}

	descriptiveMedia := &model.Media{ScrapeStatus: "pending"}
	applyLocalMetadata(descriptiveMedia, &LocalMetadata{Overview: "剧情简介"})
	if descriptiveMedia.ScrapeStatus != "matched" {
		t.Fatalf("descriptive metadata should mark matched, got %q", descriptiveMedia.ScrapeStatus)
	}
}
