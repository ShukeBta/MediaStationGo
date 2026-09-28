package service

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestExpandPersistedSeriesGroupsLeavesStableEpisodesUnresolved(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Base: model.Base{ID: "stable-tv"}, Name: "TV", Path: "/vault/tv", Type: "tv", Enabled: true}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	rows := []model.Media{
		{Base: model.Base{ID: "stable-1"}, LibraryID: lib.ID, Title: "Stable Show", Path: lib.Path + "/Stable Show/Season 01/episode-1.mkv", SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "stable-2"}, LibraryID: lib.ID, Title: "Stable Show", Path: lib.Path + "/Stable Show/Season 01/episode-2.mkv", SeasonNum: 1, EpisodeNum: 2},
	}
	for i := range rows {
		repos.Media.PrepareSeriesKey(&rows[i])
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	filter := repository.MediaQueryFilter{IncludeNSFW: true}
	candidates, complete, err := repos.Media.ListPersistedSeriesCardGroups(t.Context(), []string{lib.ID}, filter)
	if err != nil || !complete || len(candidates) != 1 || candidates[0].SeriesCount != 2 {
		t.Fatalf("stable candidates=%#v complete=%v err=%v", candidates, complete, err)
	}
	want := append([]repository.SeriesCardGroupCandidate(nil), candidates...)
	mediaQueries := 0
	const callback = "test:stable-series-expansion-queries"
	if err := db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "media" {
			mediaQueries++
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })
	got, err := svc.expandPersistedSeriesGroups(t.Context(), candidates, filter)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || got[0].ProjectionResolved {
		t.Fatalf("stable group should keep its aggregate sample unresolved: got=%#v want=%#v", got, want)
	}
	if mediaQueries != 0 {
		t.Fatalf("stable group fetched media rows during expansion: %d queries", mediaQueries)
	}
}

func TestExpandPersistedSeriesGroupsSplitsNestedLibraryBeforeAggregation(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	root := filepath.ToSlash(filepath.Join(t.TempDir(), "movies"))
	libs := []model.Library{
		{Base: model.Base{ID: "physical-movies"}, Name: "Movies", Path: root, Type: "movie", Enabled: true},
		{Base: model.Base{ID: "nested-cinema"}, Name: "Cinema", Path: root + "/Cinema", Type: "movie", Enabled: true},
	}
	if err := db.Create(&libs).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	now := time.Date(2026, 9, 29, 1, 0, 0, 123456789, time.UTC)
	rows := []model.Media{
		{Base: model.Base{ID: "alpha-poster", CreatedAt: now.Add(-time.Hour)}, LibraryID: libs[0].ID, Title: "Alpha", Path: libs[1].Path + "/alpha-old.mkv", Rating: 8, PosterURL: "https://image.test/alpha-poster.jpg"},
		{Base: model.Base{ID: "alpha-latest", CreatedAt: now}, LibraryID: libs[0].ID, Title: "Alpha", Path: libs[1].Path + "/alpha-new.mkv", Rating: 10},
		{Base: model.Base{ID: "alpha-unrated", CreatedAt: now.Add(-time.Minute)}, LibraryID: libs[0].ID, Title: "Alpha", Path: libs[1].Path + "/alpha-unrated.mkv"},
		{Base: model.Base{ID: "beta-old", CreatedAt: now.Add(-time.Hour)}, LibraryID: libs[0].ID, Title: "Beta", Path: libs[1].Path + "/beta-old.mkv", Rating: 5},
		{Base: model.Base{ID: "beta-latest", CreatedAt: now.Add(2 * time.Hour)}, LibraryID: libs[0].ID, Title: "Beta", Path: libs[1].Path + "/beta-new.mkv"},
		{Base: model.Base{ID: "middle-film", CreatedAt: now.Add(time.Hour)}, LibraryID: libs[0].ID, Title: "Middle Film", Path: libs[0].Path + "/Middle Film/movie.mkv", Rating: 7},
	}
	for i := range rows {
		repos.Media.PrepareSeriesKey(&rows[i])
	}
	sourceKey := rows[0].SeriesKey
	for i := 1; i < 5; i++ {
		if rows[i].SeriesKey != sourceKey {
			t.Fatalf("fixture must share a physical key before display projection: row=%#v", rows[i])
		}
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	filter := repository.MediaQueryFilter{IncludeNSFW: true}
	candidates, complete, err := repos.Media.ListPersistedSeriesCardGroups(t.Context(), []string{libs[0].ID}, filter)
	if err != nil || !complete || len(candidates) != 2 {
		t.Fatalf("physical candidates=%#v complete=%v err=%v", candidates, complete, err)
	}
	got, err := svc.expandPersistedSeriesGroups(t.Context(), candidates, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("nested library should split the shared directory group: %#v", got)
	}
	wants := []struct {
		id       string
		count    int64
		rating   float64
		rated    int64
		latest   time.Time
		resolved bool
	}{
		{"beta-latest", 2, 5, 1, now.Add(2 * time.Hour), true},
		{"middle-film", 1, 7, 1, now.Add(time.Hour), false},
		{"alpha-poster", 3, 18, 2, now, true},
	}
	for i, want := range wants {
		candidate := got[i]
		if candidate.ID != want.id || candidate.SeriesCount != want.count || candidate.RatingSum != want.rating || candidate.RatingCount != want.rated || !candidate.SeriesLatest.Equal(want.latest) || candidate.ProjectionResolved != want.resolved {
			t.Fatalf("candidate %d: got=%#v want=%#v", i, candidate, want)
		}
		if want.resolved && candidate.SeriesKey != sourceKey {
			t.Fatalf("split group lost physical source key: got=%q want=%q", candidate.SeriesKey, sourceKey)
		}
	}
	projected := []model.Media{got[0].Media(), got[2].Media()}
	svc.attachLibraryDisplayMetadata(t.Context(), projected)
	if projected[0].DisplayLibraryID != libs[1].ID || projected[1].DisplayLibraryID != libs[1].ID || MediaSeriesKey(projected[0]) == MediaSeriesKey(projected[1]) {
		t.Fatalf("split representatives must have distinct public identities in the nested library: %#v", projected)
	}
}

func TestExpandPersistedSeriesGroupsPreservesVisibility(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	root := filepath.ToSlash(t.TempDir())
	libs := []model.Library{
		{Base: model.Base{ID: "visible"}, Name: "Movies", Path: root + "/movies", Type: "movie", Enabled: true},
		{Base: model.Base{ID: "nested"}, Name: "Cinema", Path: root + "/movies/Cinema", Type: "movie", Enabled: true},
		{Base: model.Base{ID: "hidden"}, Name: "Secret", Path: root + "/Secret", Type: "movie", Enabled: true},
	}
	if err := db.Create(&libs).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	now := time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC)
	rows := []model.Media{
		{Base: model.Base{ID: "safe-alpha", CreatedAt: now}, LibraryID: "visible", Title: "Alpha", Path: libs[1].Path + "/alpha.mkv", Rating: 7},
		{Base: model.Base{ID: "safe-beta", CreatedAt: now}, LibraryID: "visible", Title: "Beta", Path: libs[1].Path + "/beta.mkv", Rating: 6},
		{Base: model.Base{ID: "private-alpha", CreatedAt: now.Add(time.Hour)}, LibraryID: "visible", Title: "Alpha", Path: libs[1].Path + "/alpha-private.mkv", Rating: 10, PosterURL: "https://image.test/poster.jpg", NSFW: true},
		{Base: model.Base{ID: "private-title", CreatedAt: now.Add(time.Hour)}, LibraryID: "visible", Title: "Private Film", Path: libs[1].Path + "/private.mkv", Rating: 10, NSFW: true},
		{Base: model.Base{ID: "hidden-film", CreatedAt: now.Add(2 * time.Hour)}, LibraryID: "hidden", Title: "Hidden Film", Path: libs[2].Path + "/hidden.mkv", Rating: 10},
	}
	for i := range rows {
		repos.Media.PrepareSeriesKey(&rows[i])
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for name, filter := range map[string]repository.MediaQueryFilter{
		"hidden-library":  {HiddenLibraryIDs: []string{"hidden"}},
		"allowed-library": {AllowedLibraryIDs: []string{"visible"}},
	} {
		t.Run(name, func(t *testing.T) {
			candidates, complete, err := repos.Media.ListPersistedSeriesCardGroups(t.Context(), nil, filter)
			if err != nil || !complete || len(candidates) != 1 || candidates[0].SeriesCount != 2 {
				t.Fatalf("filtered physical candidates=%#v complete=%v err=%v", candidates, complete, err)
			}
			got, err := svc.expandPersistedSeriesGroups(t.Context(), candidates, filter)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 {
				t.Fatalf("expansion exposed a hidden title: %#v", got)
			}
			seen := make(map[string]bool)
			for _, candidate := range got {
				wantRating := map[string]float64{"safe-alpha": 7, "safe-beta": 6}[candidate.ID]
				if wantRating == 0 || candidate.LibraryID != "visible" || candidate.NSFW || candidate.SeriesCount != 1 || candidate.RatingSum != wantRating || candidate.RatingCount != 1 || !candidate.SeriesLatest.Equal(now) {
					t.Fatalf("hidden media contaminated a public candidate: %#v", candidate)
				}
				seen[candidate.ID] = true
			}
			if len(seen) != 2 {
				t.Fatalf("visible works were duplicated or omitted: %#v", got)
			}
		})
	}
}
