package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func embyProjectionFixture(t testing.TB, works, episodes int) (*EmbyService, model.Library) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	// Use the production migration so benchmarks include the composite and
	// partial indexes used by the actual browse queries, not just model indexes.
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	e := NewEmbyService(&config.Config{}, zap.NewNop(), repository.New(db))
	lib := model.Library{Name: "Shows", Path: "/media/shows", Type: "tv", Enabled: true}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	for i := 0; i < works; i++ {
		rows := make([]model.Media, episodes)
		for j := range rows {
			rows[j] = model.Media{Base: model.Base{ID: fmt.Sprintf("work-%04d-episode-%03d", i, j), CreatedAt: stamp.Add(time.Duration(i%3) * time.Hour), UpdatedAt: stamp.Add(time.Duration(j) * time.Minute)},
				LibraryID: lib.ID, Title: fmt.Sprintf("Show %04d S01E%03d", i, j), Path: fmt.Sprintf("/media/shows/Show %04d/Season 1/E%03d.mkv", i, j),
				SeasonNum: 1, EpisodeNum: j + 1, ReleaseDate: fmt.Sprintf("%04d-01-01", 2000+i%20), Year: 2000 + i%20,
				TMDbID: 1000 + i, Genres: "Drama,Comedy", Countries: "US", Languages: "en", PosterURL: fmt.Sprintf("https://img.example/%d-%d/poster.jpg", i, j),
				Overview: strings.Repeat("description", 400), SearchPinyin: strings.Repeat("search alias ", 400), SearchInitials: strings.Repeat("alias", 400),
				VideoCodec: "hevc", AudioCodec: "aac", DurationSec: 2400, Width: 1920, Height: 1080, FileID: fmt.Sprintf("file-%d-%d", i, j), Rating: float32(i % 10),
			}
		}
		if err := db.CreateInBatches(&rows, 30).Error; err != nil {
			t.Fatal(err)
		}
	}
	return e, lib
}

// Test-only reference: the previous full-row series browse path. It is never
// compiled into the server or used as a runtime fallback.
func embyProjectionReference(ctx context.Context, e *EmbyService, lib string, p ItemsParams) (map[string]any, error) {
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("season_num > 0 OR episode_num > 0").Where("COALESCE(part_group_key, '') = ''").Where("library_id = ?", lib)
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	q = applyEmbyMediaSearch(q, p)
	var rows []model.Media
	if err := q.Order(mediaReleaseOrderSQL(true)).Limit(embySeriesGroupingLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	rows = e.filterMediaRowsByEmbyGenres(rows, p)
	groups, err := e.seriesGroupsFromMedia(ctx, rows)
	if err != nil {
		return nil, err
	}
	var parts []model.Media
	q = e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("library_id = ? AND COALESCE(part_group_key, '') <> ''", lib)
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	q = applyEmbyMediaSearch(q, p)
	if err := q.Order("media.part_group_key ASC, media.part_index ASC, media.created_at ASC").Find(&parts).Error; err != nil {
		return nil, err
	}
	parts = e.filterMediaRowsByEmbyGenres(parts, p)
	groups = append(groups, e.multipartSeriesGroupsFromMedia(parts)...)
	sortSeriesGroups(groups, p)
	items := []map[string]any{}
	for _, g := range pageSlice(groups, p.StartIndex, p.Limit) {
		items = append(items, e.seriesPayload(g))
	}
	return map[string]any{"Items": items, "TotalRecordCount": len(groups), "StartIndex": p.StartIndex}, nil
}

func TestEmbyBrowseProjectionMatchesFullRowsAndKeepsDetails(t *testing.T) {
	e, lib := embyProjectionFixture(t, 17, 3)
	// Persisted Series metadata must continue to own artwork/title and ordering.
	s := model.Series{Base: model.Base{ID: "canonical"}, LibraryID: lib.ID, Title: "Canonical", TMDbID: 1000, PosterURL: "https://img.example/canonical.jpg", BackdropURL: "https://img.example/canonical-bd.jpg", Year: 2025}
	if err := e.repo.DB.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	for _, sortBy := range []string{"DateCreated", "SortName", "PremiereDate", "ProductionYear", "CommunityRating", "DateLastContentAdded,SortName"} {
		for _, direction := range []string{"Ascending", "Descending"} {
			for _, offset := range []int{0, 12, 100} {
				p := ItemsParams{ParentID: lib.ID, Limit: 12, StartIndex: offset, SortBy: sortBy, SortOrder: direction}
				want, err := embyProjectionReference(t.Context(), e, lib.ID, p)
				if err != nil {
					t.Fatal(err)
				}
				e.invalidateVirtualSeriesCache()
				got, err := e.seriesItemsForLibrary(t.Context(), lib.ID, p)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					for i, item := range got["Items"].([]map[string]any) {
						for key, value := range item {
							old := want["Items"].([]map[string]any)[i][key]
							if !reflect.DeepEqual(value, old) {
								t.Logf("card %d field %s got=%#v want=%#v", i, key, value, old)
							}
						}
						break
					}
					t.Fatalf("changed payload: sort=%s direction=%s offset=%d", sortBy, direction, offset)
				}
				for _, item := range got["Items"].([]map[string]any) {
					if _, ok := e.cachedSeriesGroup(item["Id"].(string)); ok {
						t.Fatal("list populated detail cache")
					}
				}
			}
		}
	}
	for _, p := range []ItemsParams{{Limit: 12, Genres: []string{"Drama"}}, {Limit: 12, Genres: []string{"missing"}}, {Limit: 12, SearchTerm: "Show 0001"}} {
		want, err := embyProjectionReference(t.Context(), e, lib.ID, p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := e.seriesItemsForLibrary(t.Context(), lib.ID, p)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("filter payload changed: %#v", p)
		}
	}
}

func TestEmbyBrowseMultipartRatingUsesAnchor(t *testing.T) {
	e, lib := embyProjectionFixture(t, 5, 3)
	if err := e.repo.DB.Model(&model.Media{}).Where("tm_db_id = ?", 1000).Updates(map[string]any{"part_group_key": "part", "part_group_title": "Multipart", "part_index": 2, "rating": 9}).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.repo.DB.Model(&model.Media{}).Where("id = ?", "work-0000-episode-000").Updates(map[string]any{"part_index": 1, "rating": 0}).Error; err != nil {
		t.Fatal(err)
	}
	for _, direction := range []string{"Ascending", "Descending"} {
		p := ItemsParams{Limit: 12, SortBy: "CommunityRating", SortOrder: direction}
		want, err := embyProjectionReference(t.Context(), e, lib.ID, p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := e.seriesItemsForLibrary(t.Context(), lib.ID, p)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("multipart rating changed: %s", direction)
		}
	}
}

func TestEmbyBrowseProjectionLatestAndMultipart(t *testing.T) {
	e, lib := embyProjectionFixture(t, 5, 3)
	if err := e.repo.DB.Model(&model.Media{}).Where("tm_db_id = ?", 1000).Updates(map[string]any{"part_group_key": "part", "part_group_title": "Multipart"}).Error; err != nil {
		t.Fatal(err)
	}
	p := ItemsParams{Limit: 12, SortBy: "DateCreated", SortOrder: "Descending"}
	want, err := embyProjectionReference(t.Context(), e, lib.ID, p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.seriesItemsForLibrary(t.Context(), lib.ID, p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("multipart changed")
	}
	var rows []model.Media
	if err := e.repo.DB.Where("library_id = ? AND (season_num > 0 OR episode_num > 0)", lib.ID).Order("media.created_at DESC, media.id DESC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	groups, err := e.seriesGroupsFromMedia(t.Context(), rows)
	if err != nil {
		t.Fatal(err)
	}
	sortSeriesGroups(groups, p)
	latestWant := []map[string]any{}
	for _, g := range pageSlice(groups, 0, 3) {
		latestWant = append(latestWant, e.seriesPayload(g))
	}
	latest, err := e.latestSeriesItemsForLibrary(t.Context(), "", lib.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(latest, latestWant) {
		t.Fatal("latest multipart semantics changed")
	}
}

func TestEmbyBrowseProjectionSelectsNoWideInventoryColumns(t *testing.T) {
	e, lib := embyProjectionFixture(t, 17, 2)
	var fullRows int64
	if err := e.repo.DB.Callback().Query().After("gorm:query").Register("projection_inventory", func(db *gorm.DB) {
		if db.Statement.Table == "media" && len(db.Statement.Selects) == 0 {
			fullRows += db.RowsAffected
		}
	}); err != nil {
		t.Fatal(err)
	}
	got, err := e.seriesItemsForLibrary(t.Context(), lib.ID, ItemsParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got["TotalRecordCount"] != 17 || fullRows != 0 {
		t.Fatalf("full rows=%d; want 0 for cards, total=%v", fullRows, got["TotalRecordCount"])
	}
	fullRows = 0
	if _, err := e.FolderCoverArtwork(t.Context(), lib.ID, "Primary", 4); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Genres(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50}); err != nil {
		t.Fatal(err)
	}
	if fullRows != 0 {
		t.Fatalf("folder/genres loaded %d full media rows", fullRows)
	}
}

func TestEmbyBrowseProjectionReflectsInventoryChanges(t *testing.T) {
	e, lib := embyProjectionFixture(t, 1, 2)
	p := ItemsParams{Limit: 12}
	check := func(total int) {
		t.Helper()
		got, err := e.seriesItemsForLibrary(t.Context(), lib.ID, p)
		if err != nil {
			t.Fatal(err)
		}
		want, err := embyProjectionReference(t.Context(), e, lib.ID, p)
		if err != nil {
			t.Fatal(err)
		}
		gotJSON, gotErr := json.Marshal(got)
		wantJSON, wantErr := json.Marshal(want)
		if gotErr != nil || wantErr != nil {
			t.Fatalf("encode payload: %v / %v", gotErr, wantErr)
		}
		if got["TotalRecordCount"] != total || string(gotJSON) != string(wantJSON) {
			t.Fatalf("inventory change not reflected: want %d groups", total)
		}
	}
	check(1)
	row := model.Media{LibraryID: lib.ID, Title: "New Show", Path: "/media/new/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1}
	if err := e.repo.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	check(2)
	if err := e.repo.DB.Model(&row).Updates(map[string]any{"poster_url": "https://img.example/new.jpg", "overview": "New metadata"}).Error; err != nil {
		t.Fatal(err)
	}
	check(2)
	if err := e.repo.DB.Model(&row).Update("library_id", "moved-library").Error; err != nil {
		t.Fatal(err)
	}
	check(1)
}

func BenchmarkEmbyBrowseProjection(b *testing.B) {
	for _, works := range []int{18, 177} {
		b.Run(fmt.Sprintf("works_%d", works), func(b *testing.B) {
			e, lib := embyProjectionFixture(b, works, 55)
			for {
				n, err := e.repo.Media.BackfillEmbyKeys(b.Context(), 1000)
				if err != nil {
					b.Fatal(err)
				}
				if n == 0 {
					break
				}
			}
			p := ItemsParams{Limit: 12, SortBy: "DateLastContentAdded,SortName", SortOrder: "Descending"}
			b.Run("before_full_rows", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := embyProjectionReference(b.Context(), e, lib.ID, p); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("after_sql_page", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := e.seriesItemsForLibrary(b.Context(), lib.ID, p); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
