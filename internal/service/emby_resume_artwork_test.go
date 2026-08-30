package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

func TestEmbyResumeItemsInheritSeriesArtworkPerUniqueSeries(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "resume-art-user"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	lib := model.Library{Name: "TV", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	setTestUserLibraries(t, svc, viewer, lib.ID)

	base := time.Date(2026, 8, 7, 8, 0, 0, 0, time.UTC)
	const (
		seriesAPoster   = "https://img.example/series-a-poster.jpg"
		seriesABackdrop = "https://img.example/series-a-backdrop.jpg"
		seriesBPoster   = "https://img.example/series-b-poster.jpg"
		seriesBBackdrop = "https://img.example/series-b-backdrop.jpg"
		ownEpisodeStill = "https://img.example/episode-own-still.jpg"
	)
	rows := []model.Media{
		{Base: model.Base{ID: "series-a-art", CreatedAt: base}, LibraryID: lib.ID, Title: "Series A", Path: `/media/tv/Series A/S01E00.mkv`, PosterURL: seriesAPoster, BackdropURL: seriesABackdrop, SeasonNum: 1},
		{Base: model.Base{ID: "series-a-ep1", CreatedAt: base.Add(time.Minute)}, LibraryID: lib.ID, Title: "Series A", Path: `/media/tv/Series A/S01E01.mkv`, SeasonNum: 1, EpisodeNum: 1, DurationSec: 120},
		{Base: model.Base{ID: "series-a-ep2", CreatedAt: base.Add(2 * time.Minute)}, LibraryID: lib.ID, Title: "Series A", Path: `/media/tv/Series A/S01E02.mkv`, SeasonNum: 1, EpisodeNum: 2, DurationSec: 120},
		{Base: model.Base{ID: "series-a-own", CreatedAt: base.Add(3 * time.Minute)}, LibraryID: lib.ID, Title: "Series A", Path: `/media/tv/Series A/S01E03.mkv`, BackdropURL: ownEpisodeStill, SeasonNum: 1, EpisodeNum: 3, DurationSec: 120},
		{Base: model.Base{ID: "series-b-art", CreatedAt: base.Add(4 * time.Minute)}, LibraryID: lib.ID, Title: "Series B", Path: `/media/tv/Series B/S01E00.mkv`, PosterURL: seriesBPoster, BackdropURL: seriesBBackdrop, SeasonNum: 1},
		{Base: model.Base{ID: "series-b-ep1", CreatedAt: base.Add(5 * time.Minute)}, LibraryID: lib.ID, Title: "Series B", Path: `/media/tv/Series B/S01E01.mkv`, SeasonNum: 1, EpisodeNum: 1, DurationSec: 120},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media %s: %v", rows[i].ID, err)
		}
	}
	for i, mediaID := range []string{"series-a-ep1", "series-a-ep2", "series-a-own", "series-b-ep1"} {
		if err := svc.repo.DB.Create(&model.PlaybackHistory{
			UserID: viewer.ID, MediaID: mediaID, PositionMs: 30_000, DurationMs: 120_000,
			WatchedAt: base.Add(time.Duration(10+i) * time.Minute), Completed: false,
		}).Error; err != nil {
			t.Fatalf("create history %s: %v", mediaID, err)
		}
	}

	seriesLookupQueries := 0
	const callbackName = "test:count-resume-series-artwork-queries"
	if err := svc.repo.DB.Callback().Query().After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		query := strings.ToLower(db.Statement.SQL.String())
		if strings.Contains(query, "season_num > 0 or episode_num > 0") {
			seriesLookupQueries++
		}
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.repo.DB.Callback().Query().Remove(callbackName)
	})

	out, err := svc.ResumeItems(t.Context(), viewer.ID)
	if err != nil {
		t.Fatalf("resume items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 4 {
		t.Fatalf("resume items len = %d, want 4: %#v", len(items), items)
	}
	byID := make(map[string]map[string]any, len(items))
	for _, item := range items {
		byID[item["Id"].(string)] = item
	}

	seriesAID := svc.seriesIDForMedia(&rows[1])
	seriesBID := svc.seriesIDForMedia(&rows[5])
	wantAPrimary := embyImageTag(seriesAID, "primary", seriesAPoster, rows[3].CreatedAt)
	wantABackdrop := embyImageTag(seriesAID, "backdrop", seriesABackdrop, rows[3].CreatedAt)
	wantBPrimary := embyImageTag(seriesBID, "primary", seriesBPoster, rows[5].CreatedAt)
	wantBBackdrop := embyImageTag(seriesBID, "backdrop", seriesBBackdrop, rows[5].CreatedAt)

	for _, mediaID := range []string{"series-a-ep1", "series-a-ep2"} {
		assertResumeArtwork(t, byID[mediaID], seriesAID, wantAPrimary, wantABackdrop)
	}
	assertResumeArtwork(t, byID["series-b-ep1"], seriesBID, wantBPrimary, wantBBackdrop)

	own := byID["series-a-own"]
	wantOwnPrimary := embyImageTag(rows[3].ID, "primary", ownEpisodeStill, rows[3].UpdatedAt)
	if tags := own["ImageTags"].(map[string]string); tags["Primary"] != wantOwnPrimary {
		t.Fatalf("episode primary was overwritten: got %#v want %q", tags, wantOwnPrimary)
	}
	if own["PrimaryImageItemId"] != rows[3].ID || own["PrimaryImageTag"] != wantOwnPrimary {
		t.Fatalf("episode primary owner was overwritten: %#v", own)
	}
	wantOwnBackdrop := embyImageTag(rows[3].ID, "backdrop", ownEpisodeStill, rows[3].UpdatedAt)
	if tags := own["BackdropImageTags"].([]string); len(tags) != 1 || tags[0] != wantOwnBackdrop {
		t.Fatalf("episode backdrop must stay its own still: %#v", own)
	}
	for _, key := range []string{"BackdropImageItemId", "ParentBackdropItemId", "ParentBackdropImageTags"} {
		if _, exists := own[key]; exists {
			t.Fatalf("episode own backdrop must omit %s: %#v", key, own)
		}
	}

	if seriesLookupQueries != 2 {
		t.Fatalf("series artwork query count = %d, want one per unique SeriesId (2)", seriesLookupQueries)
	}
}

func TestEmbyResumeItemsReturnSeriesArtworkQueryError(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "resume-art-error-user"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	lib := model.Library{Name: "TV", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	setTestUserLibraries(t, svc, viewer, lib.ID)
	media := model.Media{Base: model.Base{ID: "resume-art-error-ep"}, LibraryID: lib.ID, Title: "Series Error", Path: `/media/tv/Series Error/S01E01.mkv`, SeasonNum: 1, EpisodeNum: 1, DurationSec: 120}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	if err := svc.repo.DB.Create(&model.PlaybackHistory{UserID: viewer.ID, MediaID: media.ID, PositionMs: 30_000, DurationMs: 120_000, WatchedAt: time.Now(), Completed: false}).Error; err != nil {
		t.Fatalf("create history: %v", err)
	}

	forcedErr := errors.New("forced resume series artwork query failure")
	const callbackName = "test:fail-resume-series-artwork-query"
	if err := svc.repo.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db.Statement.Table == "media" {
			if _, ordered := db.Statement.Clauses["ORDER BY"]; ordered {
				db.AddError(forcedErr)
			}
		}
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.repo.DB.Callback().Query().Remove(callbackName)
	})

	if _, err := svc.ResumeItems(t.Context(), viewer.ID); !errors.Is(err, forcedErr) {
		t.Fatalf("resume artwork error = %v, want %v", err, forcedErr)
	}
}

func assertResumeArtwork(t *testing.T, item map[string]any, ownerID, primaryTag, backdropTag string) {
	t.Helper()
	if item == nil {
		t.Fatal("resume item is missing")
	}
	if tags := item["ImageTags"].(map[string]string); tags["Primary"] != primaryTag {
		t.Fatalf("resume primary tags = %#v, want %q", tags, primaryTag)
	}
	if item["PrimaryImageItemId"] != ownerID || item["PrimaryImageTag"] != primaryTag {
		t.Fatalf("resume primary owner fields = %#v, want owner %q tag %q", item, ownerID, primaryTag)
	}
	if tags := item["BackdropImageTags"].([]string); len(tags) != 0 {
		t.Fatalf("resume item must not expose parent artwork as its own backdrop: %#v", tags)
	}
	if tags := item["ParentBackdropImageTags"].([]string); len(tags) != 1 || tags[0] != backdropTag {
		t.Fatalf("resume parent backdrop tags = %#v, want %q", tags, backdropTag)
	}
	if item["ParentBackdropItemId"] != ownerID {
		t.Fatalf("resume parent backdrop owner = %#v, want %q", item, ownerID)
	}
	if _, exists := item["BackdropImageItemId"]; exists {
		t.Fatalf("resume parent backdrop must omit BackdropImageItemId: %#v", item)
	}
}
