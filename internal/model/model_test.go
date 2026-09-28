package model

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestMediaReferenceFieldsAllowVirtualEmbyIDs(t *testing.T) {
	virtualIDLen := len("msgo-series-c003a8f345ea99aca9bb857591afd915")
	cases := []struct {
		name      string
		model     any
		fieldName string
	}{
		{name: "media_series_id", model: &Media{}, fieldName: "SeriesID"},
		{name: "media_duplicate_of", model: &Media{}, fieldName: "DuplicateOf"},
		{name: "playback_history_media_id", model: &PlaybackHistory{}, fieldName: "MediaID"},
		{name: "playback_preference_media_id", model: &UserMediaPlaybackPreference{}, fieldName: "MediaID"},
		{name: "favorite_media_id", model: &Favorite{}, fieldName: "MediaID"},
		{name: "playlist_item_media_id", model: &PlaylistItem{}, fieldName: "MediaID"},
		{name: "strm_record_media_id", model: &STRMRecord{}, fieldName: "MediaID"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := schema.Parse(tc.model, &sync.Map{}, schema.NamingStrategy{})
			if err != nil {
				t.Fatal(err)
			}
			field := parsed.LookUpField(tc.fieldName)
			if field == nil {
				t.Fatalf("field %s not found", tc.fieldName)
			}
			if field.Size < virtualIDLen {
				t.Fatalf("%s size = %d, want at least %d", tc.fieldName, field.Size, virtualIDLen)
			}
		})
	}
}
