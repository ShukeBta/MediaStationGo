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

func TestApplyLocalMetadataAddsTechnicalMetadataWithoutMarkingProbeComplete(t *testing.T) {
	media := &model.Media{}
	applyLocalMetadata(media, &LocalMetadata{Technical: LocalTechnicalMetadata{
		DurationSec:        7185,
		Width:              3840,
		Height:             2160,
		VideoCodec:         "hevc",
		AudioCodec:         "truehd",
		VideoBitRate:       25000000,
		FrameRate:          23.976,
		VideoProfile:       "Main 10",
		VideoRange:         "HDR10",
		VideoBitDepth:      10,
		AudioBitRate:       768000,
		AudioChannels:      8,
		AudioChannelLayout: "7.1",
		AudioSampleRate:    48000,
	}})

	if media.DurationSec != 7185 || media.Width != 3840 || media.Height != 2160 || media.VideoCodec != "hevc" || media.AudioCodec != "truehd" || media.AudioChannels != 8 {
		t.Fatalf("technical metadata was not applied: %+v", media)
	}
	if media.MediaProbeVersion != 0 {
		t.Fatalf("NFO technical metadata must not mark ffprobe complete, got version %d", media.MediaProbeVersion)
	}
}
