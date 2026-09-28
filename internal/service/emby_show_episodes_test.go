package service

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

func TestShowEpisodesLoadsOnlyRequestedSeasonAndMatchesReference(t *testing.T) {
	e, _ := embyProjectionFixture(t, 1, 180)
	// Nine seasons, including specials and legacy negative season numbers.
	var rows []model.Media
	if err := e.repo.DB.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		number := i / 20
		if number == 1 {
			number = -1
		}
		if err := e.repo.DB.Model(&rows[i]).Updates(map[string]any{"season_num": number, "episode_num": i%20 + 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.InitializeBrowseKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.DB.First(&rows[0], "id = ?", rows[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	group, ok, err := e.findSeriesGroup(t.Context(), rows[0].EmbySeriesKey, "")
	if err != nil || !ok {
		t.Fatalf("group: %v %v", ok, err)
	}
	var fullRows int64
	var tracking bool
	if err := e.repo.DB.Callback().Query().After("gorm:query").Register("target_season_rows", func(db *gorm.DB) {
		if tracking && db.Statement.Table == "media" && strings.HasPrefix(db.Statement.SQL.String(), "SELECT *") &&
			strings.Contains(db.Statement.SQL.String(), "emby_series_key =") {
			fullRows += db.RowsAffected
			if !strings.Contains(db.Statement.SQL.String(), "season_num") {
				t.Error("missing season restriction")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	for _, season := range e.seasonsForSeries(group) {
		for _, omit := range []bool{true, false} {
			p := ItemsParams{ShowID: group.ID, ParentID: season.ID, UserID: "", Limit: 7, StartIndex: 2, OmitMediaSources: omit}
			want, err := e.episodeItems(t.Context(), season.Episodes, p)
			if err != nil {
				t.Fatal(err)
			}
			fullRows, tracking = 0, true
			got, err := e.Items(t.Context(), p)
			tracking = false
			if err != nil {
				t.Fatal(err)
			}
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("season %d omit=%v response changed", season.SeasonNum, omit)
			}
			if omit && fullRows != int64(len(season.Episodes)) {
				t.Fatalf("season %d loaded %d rows; want %d, not whole show", season.SeasonNum, fullRows, len(season.Episodes))
			}
		}
	}
}
