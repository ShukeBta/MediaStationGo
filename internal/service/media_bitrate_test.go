package service

import (
	"math"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestEffectiveMediaBitRate(t *testing.T) {
	tests := []struct {
		name          string
		storedBitRate int64
		sizeBytes     int64
		durationSec   int
		want          int64
	}{
		{name: "stored value wins", storedBitRate: 24_000_000, sizeBytes: 1, durationSec: 1, want: 24_000_000},
		{name: "derive average", sizeBytes: 900_000_000, durationSec: 600, want: 12_000_000},
		{name: "missing size", durationSec: 600, want: 0},
		{name: "missing duration", sizeBytes: 900_000_000, want: 0},
		{name: "overflow", sizeBytes: math.MaxInt64, durationSec: 1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveMediaBitRate(tt.storedBitRate, tt.sizeBytes, tt.durationSec); got != tt.want {
				t.Fatalf("effectiveMediaBitRate() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBaseMediaSourceDerivesAverageBitRate(t *testing.T) {
	media := &model.Media{SizeBytes: 900_000_000, DurationSec: 600}
	source := (&EmbyService{}).baseMediaSource(media, "mkv", false, "", true)
	if got := source["Bitrate"]; got != int64(12_000_000) {
		t.Fatalf("Bitrate = %#v, want 12000000", got)
	}
}

func TestPersistMediaProbeResultDerivesAverageBitRate(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	media := model.Media{
		Base:      model.Base{ID: "derived-bitrate"},
		SizeBytes: 900_000_000,
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	probe := &ProbeResult{DurationSec: 600, VideoCodec: "h264", AudioCodec: "aac"}
	if err := persistMediaProbeResult(t.Context(), repos, nil, nil, nil, &media, probe); err != nil {
		t.Fatal(err)
	}

	var persisted model.Media
	if err := repos.DB.First(&persisted, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.BitRate != 12_000_000 || media.BitRate != 12_000_000 {
		t.Fatalf("persisted bitrate=%d in-memory bitrate=%d, want 12000000", persisted.BitRate, media.BitRate)
	}
	if persisted.VideoBitRate != 0 || persisted.AudioBitRate != 0 {
		t.Fatalf("derived total bitrate must not invent stream bitrates: video=%d audio=%d", persisted.VideoBitRate, persisted.AudioBitRate)
	}
}
