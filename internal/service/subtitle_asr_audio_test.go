package service

import (
	"context"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type asrAudioResolver struct {
	typ string
	ref string
	ua  string
}

func (r *asrAudioResolver) CloudResolve(_ context.Context, typ, ref, clientUA string) (*cloud.DirectLink, error) {
	r.typ = typ
	r.ref = ref
	r.ua = clientUA
	return &cloud.DirectLink{
		URL:     "https://cdn.example/movie.mkv",
		Headers: map[string]string{"Referer": "https://example.invalid/"},
	}, nil
}

func TestResolveASRAudioSourceReusesCloudPlaybackResolution(t *testing.T) {
	resolver := &asrAudioResolver{}
	stream := &StreamService{storage: resolver}
	media := &model.Media{STRMURL: "/api/cloud/play/openlist?ref=%2FMovies%2Fmovie.mkv"}

	source, err := stream.resolveASRAudioSource(t.Context(), media)
	if err != nil {
		t.Fatal(err)
	}
	if resolver.typ != "openlist" || resolver.ref != "/Movies/movie.mkv" || resolver.ua != cloudMediaInternalUserAgent {
		t.Fatalf("unexpected cloud resolve request: %#v", resolver)
	}
	if source.input != "https://cdn.example/movie.mkv" {
		t.Fatalf("input = %q", source.input)
	}
	if source.headers["User-Agent"] != cloudMediaInternalUserAgent || source.headers["Referer"] == "" {
		t.Fatalf("headers = %#v", source.headers)
	}
}

func TestResolveASRAudioSourceRejectsUnresolvableCloudPath(t *testing.T) {
	stream := &StreamService{}
	_, err := stream.resolveASRAudioSource(t.Context(), &model.Media{Path: "cloud://openlist/missing"})
	if err == nil {
		t.Fatal("expected explicit unresolved cloud media error")
	}
}
