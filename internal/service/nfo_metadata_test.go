package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func nfoEditingFixture(t *testing.T, episode bool) (*MediaService, *model.Media) {
	t.Helper()
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "NFO", Path: t.TempDir(), Type: "movie"}
	if episode {
		lib.Type = "tv"
	}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(lib.Path, "Example")
	if episode {
		dir = filepath.Join(dir, "Season 01")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	m := &model.Media{LibraryID: lib.ID, Path: filepath.Join(dir, "video.mkv"), Title: "Old", Year: 2025, Rating: 8, Actors: "Alice", TMDbID: 20}
	if episode {
		m.SeasonNum = 1
		m.EpisodeNum = 2
		m.EpisodeTitle = "Old episode"
	}
	if err := repos.DB.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	return NewMediaService(&config.Config{}, zap.NewNop(), repos), m
}

func TestNFOMetadataPreservesUneditedXMLAndClearsFields(t *testing.T) {
	s, m := nfoEditingFixture(t, false)
	path := filepath.Join(filepath.Dir(m.Path), "movie.nfo")
	input := `<movie custom="yes"><title>Old</title><year>2025</year><rating>8</rating><ratings><rating><value>8</value></rating></ratings><genre>Old</genre><genre>Duplicate</genre><actor><name>Alice</name><role>Hero</role></actor><fileinfo><streamdetails><audio><language>ja</language></audio></streamdetails></fileinfo><!--keep--><uniqueid type="imdb">tt123</uniqueid><custom><data>yes</data></custom></movie>`
	if err := os.WriteFile(path, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	title, genres, actors := "New & Better", "Drama,Comedy", "Alice"
	year, rating := 0, float32(0)
	updated, err := s.UpdateMetadata(t.Context(), m.ID, MediaMetadataUpdate{Title: &title, Year: &year, Rating: &rating, Genres: &genres, Actors: &actors, WriteNFO: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != title || updated.Rating != 0 || updated.Year != 0 {
		t.Fatalf("database not updated: %+v", updated)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range []string{`custom="yes"`, `<role>Hero</role>`, `<fileinfo>`, `<!--keep-->`, `<uniqueid type="imdb">tt123</uniqueid>`, `<custom><data>yes</data></custom>`, `<genre>Drama</genre><genre>Comedy</genre>`, `<year>0</year>`, `<rating>0</rating>`} {
		if !strings.Contains(string(body), retained) {
			t.Fatalf("missing %s in %s", retained, body)
		}
	}
	meta, err := ReadLocalMetadata(m.Path, filepath.Dir(filepath.Dir(m.Path)), false)
	if err != nil || meta.Title != title || meta.Rating != 0 || meta.Year != 0 {
		t.Fatalf("rescan = %+v, %v", meta, err)
	}
}

func TestNFOMetadataMalformedXMLRollsBackDatabase(t *testing.T) {
	s, m := nfoEditingFixture(t, false)
	path := nfoPath(m.Path)
	input := `<movie><title>broken`
	if err := os.WriteFile(path, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	title := "Changed"
	if _, err := s.UpdateMetadata(t.Context(), m.ID, MediaMetadataUpdate{Title: &title, WriteNFO: true}); err == nil {
		t.Fatal("expected parse failure")
	}
	stored, _ := s.repo.Media.FindByID(t.Context(), m.ID)
	if stored.Title != "Old" {
		t.Fatal("database must roll back on NFO failure")
	}
	body, _ := os.ReadFile(path)
	if string(body) != input {
		t.Fatal("malformed original was changed")
	}
}

func TestNFOMetadataEpisodeAndSeriesScopes(t *testing.T) {
	s, m := nfoEditingFixture(t, true)
	input := `<episodedetails><title>Old episode</title><showtitle>Old</showtitle><uniqueid type="tmdb">999</uniqueid><season>1</season><episode>2</episode></episodedetails>`
	if err := os.WriteFile(nfoPath(m.Path), []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	title, episode := "Show", "Episode & two"
	showID := 30
	if _, err := s.UpdateMetadata(t.Context(), m.ID, MediaMetadataUpdate{Title: &title, EpisodeTitle: &episode, TMDbID: &showID, WriteNFO: true}); err != nil {
		t.Fatal(err)
	}
	doc, _, err := readNFO(nfoPath(m.Path))
	if err != nil || doc.Title != episode || doc.ShowTitle != title || externalIDFromUniqueIDs(doc.UniqueIDs, "tmdb") != "999" {
		t.Fatalf("episode identities: %+v %v", doc, err)
	}
	if _, err := s.UpdateMetadata(t.Context(), m.ID, MediaMetadataUpdate{Title: &title, WriteNFO: true, NFOScope: "series"}); err != nil {
		t.Fatal(err)
	}
	showPath := filepath.Join(filepath.Dir(filepath.Dir(m.Path)), "tvshow.nfo")
	doc, _, err = readNFO(showPath)
	if err != nil || doc.Title != title || doc.XMLName.Local != "tvshow" {
		t.Fatalf("show NFO: %+v %v", doc, err)
	}
}

func TestNFOMetadataRejectsSymlinkAndUndoRestoresOriginal(t *testing.T) {
	s, m := nfoEditingFixture(t, false)
	input := `<movie><title>Old</title></movie>`
	if err := os.WriteFile(nfoPath(m.Path), []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	after := *m
	after.Title = "New"
	undo, err := s.writeEditedNFO(t.Context(), s.repo.DB, m, &after, "", map[string]any{"title": "New"})
	if err != nil || undo == nil {
		t.Fatalf("write: %v", err)
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(nfoPath(m.Path))
	if string(body) != input {
		t.Fatal("undo must preserve exact original bytes")
	}
	outside := filepath.Join(t.TempDir(), "outside.nfo")
	if err := os.WriteFile(outside, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(nfoPath(m.Path)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, nfoPath(m.Path)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := s.NFOEditTarget(t.Context(), m.ID, "media"); err == nil {
		t.Fatal("must reject symlink NFO")
	}
}
