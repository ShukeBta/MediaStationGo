package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// seedEpisodicLibrary creates a TV library with one series of episodeCount
// episodes (season 1) plus optional extra series; episodes are named
// "<prefix>-<num>".
func seedEpisodicLibrary(t *testing.T, svc *EmbyService, viewer *model.User, prefix string, episodeCount int, extraSeries map[string]int) {
	t.Helper()
	lib := model.Library{Name: "动漫", Path: `/media/anime`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	setTestUserLibraries(t, svc, viewer, lib.ID)
	seriesSpecs := map[string]int{prefix: episodeCount}
	for name, count := range extraSeries {
		seriesSpecs[name] = count
	}
	for seriesName, count := range seriesSpecs {
		for ep := 1; ep <= count; ep++ {
			media := model.Media{
				Base:        model.Base{ID: fmt.Sprintf("%s-%03d", seriesName, ep)},
				LibraryID:   lib.ID,
				Title:       seriesName,
				Path:        fmt.Sprintf(`/media/anime/%s/第%d集.mkv`, seriesName, ep),
				SeasonNum:   1,
				EpisodeNum:  ep,
				DurationSec: 1200,
			}
			if err := svc.repo.DB.Create(&media).Error; err != nil {
				t.Fatalf("create episode %s ep %d: %v", seriesName, ep, err)
			}
		}
	}
}

func TestEmbyNextUpReturnsInProgressEpisodeForSeries(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	seedEpisodicLibrary(t, svc, viewer, "遮天", 177, nil)
	base := time.Now().UTC()
	histories := []model.PlaybackHistory{
		{UserID: viewer.ID, MediaID: "遮天-007", PositionMs: 1_200_000, DurationMs: 1_200_000, WatchedAt: base.Add(-48 * time.Hour), Completed: true},
		{UserID: viewer.ID, MediaID: "遮天-008", PositionMs: 300_000, DurationMs: 1_260_000, WatchedAt: base.Add(-2 * time.Hour), Completed: false},
		{UserID: viewer.ID, MediaID: "遮天-009", PositionMs: 40_000, DurationMs: 1_320_000, WatchedAt: base.Add(-1 * time.Hour), Completed: false},
	}
	for _, row := range histories {
		if err := svc.repo.DB.Create(&row).Error; err != nil {
			t.Fatalf("create history: %v", err)
		}
	}

	seriesID := virtualSeriesID(t, svc, viewer.ID, "遮天")
	out, err := svc.NextUpItems(t.Context(), NextUpParams{UserID: viewer.ID, SeriesIDs: []string{seriesID}})
	if err != nil {
		t.Fatalf("next up: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("expected the in-progress episode as next up, got %#v", items)
	}
	if items[0]["Id"] != "遮天-009" {
		t.Fatalf("next up should be the most recently touched in-progress episode, got %#v", items[0]["Id"])
	}
	userData := items[0]["UserData"].(map[string]any)
	if userData["PlaybackPositionTicks"] != int64(40_000*10_000) {
		t.Fatalf("next up should carry the resume position, got %#v", userData["PlaybackPositionTicks"])
	}
	if userData["LastPlayedDate"] == "" {
		t.Fatalf("next up should expose LastPlayedDate: %#v", userData)
	}
}

func TestEmbyNextUpSkipsToFirstUnplayedAfterCompleted(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	seedEpisodicLibrary(t, svc, viewer, "剧情", 10, nil)
	base := time.Now().UTC()
	for ep := 1; ep <= 3; ep++ {
		row := model.PlaybackHistory{
			UserID:     viewer.ID,
			MediaID:    fmt.Sprintf("剧情-%03d", ep),
			PositionMs: 1_200_000,
			DurationMs: 1_200_000,
			WatchedAt:  base.Add(time.Duration(ep-3) * time.Hour),
			Completed:  true,
		}
		if err := svc.repo.DB.Create(&row).Error; err != nil {
			t.Fatalf("create history: %v", err)
		}
	}

	seriesID := virtualSeriesID(t, svc, viewer.ID, "剧情")
	out, err := svc.NextUpItems(t.Context(), NextUpParams{UserID: viewer.ID, SeriesIDs: []string{seriesID}})
	if err != nil {
		t.Fatalf("next up: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "剧情-004" {
		t.Fatalf("next up should be the first unplayed episode after the last completed one, got %#v", items)
	}
	userData := items[0]["UserData"].(map[string]any)
	if userData["PlaybackPositionTicks"] != int64(0) {
		t.Fatalf("an unwatched next-up episode should not carry a resume position, got %#v", userData["PlaybackPositionTicks"])
	}
}

func TestEmbyNextUpAllWatchedReturnsEmpty(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	seedEpisodicLibrary(t, svc, viewer, "全看完", 3, nil)
	base := time.Now().UTC()
	for ep := 1; ep <= 3; ep++ {
		row := model.PlaybackHistory{
			UserID:     viewer.ID,
			MediaID:    fmt.Sprintf("全看完-%03d", ep),
			PositionMs: 1_200_000,
			DurationMs: 1_200_000,
			WatchedAt:  base.Add(time.Duration(ep) * time.Hour),
			Completed:  true,
		}
		if err := svc.repo.DB.Create(&row).Error; err != nil {
			t.Fatalf("create history: %v", err)
		}
	}

	seriesID := virtualSeriesID(t, svc, viewer.ID, "全看完")
	out, err := svc.NextUpItems(t.Context(), NextUpParams{UserID: viewer.ID, SeriesIDs: []string{seriesID}})
	if err != nil {
		t.Fatalf("next up: %v", err)
	}
	if items := out["Items"].([]map[string]any); len(items) != 0 {
		t.Fatalf("a fully watched series should have no next up, got %#v", items)
	}
}

func TestEmbyNextUpHomeOrdersSeriesByRecentActivity(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	seedEpisodicLibrary(t, svc, viewer, "新剧", 12, map[string]int{"旧剧": 8, "没看过": 5})
	base := time.Now().UTC()
	histories := []model.PlaybackHistory{
		{UserID: viewer.ID, MediaID: "旧剧-002", PositionMs: 240_000, DurationMs: 1_200_000, WatchedAt: base.Add(-3 * time.Hour), Completed: false},
		{UserID: viewer.ID, MediaID: "新剧-005", PositionMs: 100_000, DurationMs: 1_200_000, WatchedAt: base.Add(-1 * time.Hour), Completed: false},
	}
	for _, row := range histories {
		if err := svc.repo.DB.Create(&row).Error; err != nil {
			t.Fatalf("create history: %v", err)
		}
	}

	out, err := svc.NextUpItems(t.Context(), NextUpParams{UserID: viewer.ID})
	if err != nil {
		t.Fatalf("home next up: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("home next up should list only series with activity, got %#v", items)
	}
	if items[0]["Id"] != "新剧-005" || items[1]["Id"] != "旧剧-002" {
		t.Fatalf("home next up should order by latest activity, got %#v", []any{items[0]["Id"], items[1]["Id"]})
	}
	if out["TotalRecordCount"] != 2 {
		t.Fatalf("home next up total = %#v, want 2", out["TotalRecordCount"])
	}
}

func TestEmbyNextUpUnwatchedExplicitSeriesDefaultsToFirstEpisode(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	seedEpisodicLibrary(t, svc, viewer, "新番", 12, nil)

	seriesID := virtualSeriesID(t, svc, viewer.ID, "新番")
	out, err := svc.NextUpItems(t.Context(), NextUpParams{UserID: viewer.ID, SeriesIDs: []string{seriesID}})
	if err != nil {
		t.Fatalf("next up: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "新番-001" {
		t.Fatalf("an untouched series should offer its first episode, got %#v", items)
	}
}

// virtualSeriesID resolves the virtual series ID the Emby layer derives for
// the given series title inside the seeded library.
func virtualSeriesID(t *testing.T, svc *EmbyService, viewerID, seriesName string) string {
	t.Helper()
	groups, err := svc.nextUpVisibleSeriesGroups(t.Context(), viewerID)
	if err != nil {
		t.Fatalf("resolve series groups: %v", err)
	}
	for _, group := range groups {
		if group.Name == seriesName {
			return group.ID
		}
	}
	t.Fatalf("series group %q not found among %d groups", seriesName, len(groups))
	return ""
}
