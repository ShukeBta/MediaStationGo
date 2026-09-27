package service

import (
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestUpdateMediaAggregationAttachesWholeTreeInRequestedOrder(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: "/media/other", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	const sourceGroup = "source-group"
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "目标作品", Path: "/media/target.mp4"},
		{LibraryID: lib.ID, Title: "来源上", Path: "/media/source-1.mp4", PartGroupKey: sourceGroup, PartGroupTitle: "来源作品", PartIndex: 1},
		{LibraryID: lib.ID, Title: "来源下", Path: "/media/source-2.mp4", PartGroupKey: sourceGroup, PartGroupTitle: "来源作品", PartIndex: 2},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)

	result, err := svc.UpdateMediaAggregation(t.Context(), lib.ID, MediaAggregationRequest{
		Action: MediaAggregationActionGroup, Title: "目标作品",
		MediaIDs: []string{rows[0].ID, rows[1].ID, rows[2].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 3 || result.GroupKey == "" || result.GroupKey == sourceGroup {
		t.Fatalf("result=%#v", result)
	}
	for index, id := range []string{rows[0].ID, rows[1].ID, rows[2].ID} {
		var stored model.Media
		if err := repos.DB.First(&stored, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if stored.PartGroupKey != result.GroupKey || stored.PartGroupTitle != "目标作品" || stored.PartIndex != index+1 {
			t.Fatalf("stored[%d]=%#v", index, stored)
		}
	}
}

func TestUpdateMediaAggregationDetachesAndReindexesRemainingChildren(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: "/media/other", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	const groupKey = "part-group"
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "第一项", Path: "/media/1.mp4", PartGroupKey: groupKey, PartGroupTitle: "作品", PartIndex: 1},
		{LibraryID: lib.ID, Title: "第二项", Path: "/media/2.mp4", PartGroupKey: groupKey, PartGroupTitle: "作品", PartIndex: 2},
		{LibraryID: lib.ID, Title: "第三项", Path: "/media/3.mp4", PartGroupKey: groupKey, PartGroupTitle: "作品", PartIndex: 3},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)

	if _, err := svc.UpdateMediaAggregation(t.Context(), lib.ID, MediaAggregationRequest{
		Action: MediaAggregationActionDetach, MediaIDs: []string{rows[1].ID},
	}); err != nil {
		t.Fatal(err)
	}
	var stored []model.Media
	if err := repos.DB.Where("id IN ?", []string{rows[0].ID, rows[1].ID, rows[2].ID}).Order("path ASC").Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored[0].PartIndex != 1 || stored[0].PartGroupKey != groupKey ||
		stored[1].PartIndex != 0 || stored[1].PartGroupKey != "" ||
		stored[2].PartIndex != 2 || stored[2].PartGroupKey != groupKey {
		t.Fatalf("stored=%#v", stored)
	}
}

func TestUpdateMediaAggregationRejectsPartialExistingTree(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: "/media/other", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "目标", Path: "/media/target.mp4"},
		{LibraryID: lib.ID, Title: "来源一", Path: "/media/source-1.mp4", PartGroupKey: "source", PartGroupTitle: "来源", PartIndex: 1},
		{LibraryID: lib.ID, Title: "来源二", Path: "/media/source-2.mp4", PartGroupKey: "source", PartGroupTitle: "来源", PartIndex: 2},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)

	_, err := svc.UpdateMediaAggregation(t.Context(), lib.ID, MediaAggregationRequest{
		Action: MediaAggregationActionGroup, Title: "目标", MediaIDs: []string{rows[0].ID, rows[1].ID},
	})
	if err == nil || !strings.Contains(err.Error(), "完整选择现有聚合作品") {
		t.Fatalf("expected partial-tree error, got %v", err)
	}
}

func TestUpdateMediaAggregationConvertsWholeVersionTreeToParts(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: "/media/other", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "目标", Path: "/media/target.mp4"},
		{LibraryID: lib.ID, Title: "多版本一", Path: "/media/version-1.mp4", VersionGroupKey: "version"},
		{LibraryID: lib.ID, Title: "多版本二", Path: "/media/version-2.mp4", VersionGroupKey: "version"},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)

	result, err := svc.UpdateMediaAggregation(t.Context(), lib.ID, MediaAggregationRequest{
		Action: MediaAggregationActionGroup, Title: "目标", MediaIDs: []string{rows[0].ID, rows[1].ID, rows[2].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, row := range rows {
		var stored model.Media
		if err := repos.DB.First(&stored, "id = ?", row.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.PartGroupKey != result.GroupKey || stored.PartIndex != index+1 || stored.VersionGroupKey != "" {
			t.Fatalf("stored[%d]=%#v", index, stored)
		}
	}
}

func TestUpdateMediaAggregationConvertsVersionTreeInPlace(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: "/media/other", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "作品", Path: "/media/movie-part-1.mp4", VersionGroupKey: "version"},
		{LibraryID: lib.ID, Title: "作品", Path: "/media/movie-part-2.mp4", VersionGroupKey: "version"},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)

	result, err := svc.UpdateMediaAggregation(t.Context(), lib.ID, MediaAggregationRequest{
		Action: MediaAggregationActionGroup,
		Title:  "作品",
		MediaIDs: []string{
			rows[0].ID,
			rows[1].ID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != len(rows) || result.GroupKey == "" {
		t.Fatalf("result=%#v", result)
	}
	for index, row := range rows {
		var stored model.Media
		if err := repos.DB.First(&stored, "id = ?", row.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.PartGroupKey != result.GroupKey || stored.PartGroupTitle != "作品" || stored.PartIndex != index+1 || stored.VersionGroupKey != "" {
			t.Fatalf("stored[%d]=%#v", index, stored)
		}
	}
}

func TestUpdateMediaAggregationRejectsPartialVersionTree(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: "/media/other", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "目标", Path: "/media/target.mp4"},
		{LibraryID: lib.ID, Title: "多版本一", Path: "/media/version-1.mp4", VersionGroupKey: "version"},
		{LibraryID: lib.ID, Title: "多版本二", Path: "/media/version-2.mp4", VersionGroupKey: "version"},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)

	_, err := svc.UpdateMediaAggregation(t.Context(), lib.ID, MediaAggregationRequest{
		Action: MediaAggregationActionGroup, Title: "目标", MediaIDs: []string{rows[0].ID, rows[1].ID},
	})
	if err == nil || !strings.Contains(err.Error(), "完整选择多版本作品") {
		t.Fatalf("expected partial version-tree error, got %v", err)
	}
}
