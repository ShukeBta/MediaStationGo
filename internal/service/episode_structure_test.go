package service

import (
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"testing"
)

func TestEpisodeCollectionDoesNotImposeSeasonOne(t *testing.T) {
	for _, collection := range []string{"Marple.s01-s06", "Marple.Season 1-6", "马普尔第一至六季", "马普尔全六季"} {
		m := &model.Media{Path: "/tv/" + collection + "/Marple.S06E02.mkv"}
		if err := episodeIdentityFromPath(m); err != nil || m.SeasonNum != 6 || m.EpisodeNum != 2 {
			t.Fatalf("%s: %+v %v", collection, m, err)
		}
	}
	m := &model.Media{Path: "/tv/Marple/S01/Marple.S06E02.mkv"}
	if episodeIdentityFromPath(m) == nil {
		t.Fatal("real season conflict accepted")
	}
}

func TestEpisodeSpecialStructuresRequireConfirmation(t *testing.T) {
	for _, name := range []string{"Friends.SE09.23.24.mkv", "Lost_第5季_13.5.rmvb", "Lost_第5季_15.5.rmvb", "Lost_第6季_17_01.rmvb", "Lost_第6季_17_02_end.rmvb", "Greys.S05E01E02.mkv", "Greys.S06E01-E02.mkv"} {
		m := &model.Media{Path: "/tv/Show/S01/" + name, SeasonNum: 1, EpisodeNum: 1}
		if ParseEpisodeEvidence(m.Path).Issue == "" {
			t.Errorf("no structural evidence: %s", name)
		}
		if episodeIdentityFromPath(m) == nil {
			t.Errorf("accepted: %s", name)
		}
	}
	for _, path := range []string{"/tv/Show/E03.mkv", "/tv/Show/no-number.mkv"} {
		m := &model.Media{Path: path, SeasonNum: 1, EpisodeNum: 1}
		if episodeIdentityFromPath(m) == nil {
			t.Errorf("trusted stale numbers: %s", path)
		}
	}
	for _, path := range []string{"/tv/Gods/S02/Gods.S02E07.720p.mkv", "/tv/Gods/S02/Gods.S02E07.1080p.mkv", "/tv/Show/S02/03.mkv"} {
		m := &model.Media{Path: path}
		if err := episodeIdentityFromPath(m); err != nil {
			t.Errorf("valid version rejected: %s: %v", path, err)
		}
	}
}

func TestBonusDirectoryRequiresExplicitMapping(t *testing.T) {
	for _, path := range []string{"/tv/Show/Extras/Show.S00E01.mkv", "/tv/Show/Bonus/Show.S01E01.mkv"} {
		if err := episodeIdentityFromPath(&model.Media{Path: path}); err == nil {
			t.Fatalf("bonus accepted: %s", path)
		}
	}
}

func TestEpisodeTitleAcceptsProviderAliasNotSearchKeyword(t *testing.T) {
	m := &Match{Title: "Primary", OriginalName: "Original", Aliases: []string{"Verified Alias"}}
	if !episodePathTitleTrusted("/tv/Verified.Alias.S01E01.mkv", m) {
		t.Fatal("provider alias rejected")
	}
	m.Aliases = nil
	m.SearchKeyword = "Verified Alias"
	if episodePathTitleTrusted("/tv/Verified.Alias.S01E01.mkv", m) {
		t.Fatal("query treated as evidence")
	}
}
