package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestScannedMediaEpisodeIdentityRejectsMovieOrdinal(t *testing.T) {
	movie := &model.Library{Type: "movie"}
	path := `cloud://openlist/电影/哆啦A梦剧场版/【ドラえもん 劇場版】【01】【のび太の恐竜】【1080P】.mkv`
	season, episode, trusted := scannedMediaEpisodeIdentity(movie, path)
	if trusted || season != 0 || episode != 0 {
		t.Fatalf("movie ordinal became episode identity: season=%d episode=%d trusted=%v", season, episode, trusted)
	}
}

func TestScannedMediaEpisodeIdentityKeepsExplicitAndSeriesEpisodes(t *testing.T) {
	movie := &model.Library{Type: "movie"}
	season, episode, trusted := scannedMediaEpisodeIdentity(movie, `/media/mixed/Show.S01E02.mkv`)
	if !trusted || season != 1 || episode != 2 {
		t.Fatalf("explicit movie-library episode identity = %d/%d trusted=%v", season, episode, trusted)
	}

	tv := &model.Library{Type: "tv"}
	season, episode, trusted = scannedMediaEpisodeIdentity(tv, `/media/tv/Show/【02】.mkv`)
	if !trusted || season != 1 || episode != 2 {
		t.Fatalf("series-library ordinal identity = %d/%d trusted=%v", season, episode, trusted)
	}
}
