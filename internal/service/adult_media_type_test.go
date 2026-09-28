package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestAdultMediaDisplayType(t *testing.T) {
	tests := []struct {
		name      string
		media     model.Media
		mediaType string
		want      string
	}{
		{name: "regular media", media: model.Media{Title: "FC2-PPV-3780016"}, mediaType: "movie", want: ""},
		{name: "ordinary av", media: model.Media{OriginalName: "MIZD-534"}, mediaType: "adult", want: adultMediaTypeAV},
		{name: "fc2 original name", media: model.Media{OriginalName: "FC2-PPV-3780016"}, mediaType: "adult", want: adultMediaTypeFC2},
		{name: "fc2 compact path", media: model.Media{Path: "/adult/FC2PPV3780016/main.mp4", NSFW: true}, want: adultMediaTypeFC2},
		{name: "fd2 genre", media: model.Media{Title: "4694056", Genres: "Adult,fd2ppv", NSFW: true}, want: adultMediaTypeFC2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adultMediaDisplayType(&tt.media, tt.mediaType); got != tt.want {
				t.Fatalf("adultMediaDisplayType() = %q, want %q", got, tt.want)
			}
		})
	}
}
