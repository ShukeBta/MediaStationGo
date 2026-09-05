package service

import (
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// TestAttachLibraryMetadataAutoCategoryKeepsOwnDisplayLibrary reproduces issue #61.
//
// A movie scanned from a source cloud library ("115 云下载") is auto-categorized into a
// separate auto-category library ("成人"): its library_id points to the auto-category
// library, but its physical cloud path still lives under the source scan directory
// (cloud://cloud115/云下载/...). Display resolution must attribute the media to the
// library it is actually browsed under (the auto-category "成人" library), not path-match
// it back to the source cloud library — otherwise the detail page "返回媒体库" button
// jumps to the wrong library.
func TestAttachLibraryMetadataAutoCategoryKeepsOwnDisplayLibrary(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)

	source := model.Library{Name: "115 云下载", Path: "cloud://cloud115/云下载", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &source); err != nil {
		t.Fatal(err)
	}

	autoPath := BuildCloudAutoCategoryLibraryPathWithScanDir("cloud115", "成人/成人", "成人")
	adult := model.Library{Name: "成人", Path: autoPath, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &adult); err != nil {
		t.Fatal(err)
	}
	if !CloudLibraryAutoCategory(adult) {
		t.Fatalf("adult library should be auto-category, got path %q", adult.Path)
	}

	mediaPath := "cloud://cloud115/云下载/Some.Movie.2024/Some.Movie.2024.mp4"
	if err := repos.Media.Upsert(t.Context(), &model.Media{
		LibraryID: adult.ID,
		Title:     "Some Movie",
		Path:      mediaPath,
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	items := []model.Media{{LibraryID: adult.ID, Title: "Some Movie", Path: mediaPath}}
	svc.attachLibraryMetadata(t.Context(), items)
	displayOnly := []model.Media{{LibraryID: adult.ID, Title: "Some Movie", Path: mediaPath}}
	svc.attachLibraryDisplayMetadata(t.Context(), displayOnly)
	for _, field := range []struct {
		name string
		full string
		only string
	}{
		{"library_name", items[0].LibraryName, displayOnly[0].LibraryName},
		{"library_path", items[0].LibraryPath, displayOnly[0].LibraryPath},
		{"display_library_id", items[0].DisplayLibraryID, displayOnly[0].DisplayLibraryID},
		{"display_library_name", items[0].DisplayLibraryName, displayOnly[0].DisplayLibraryName},
		{"display_library_path", items[0].DisplayLibraryPath, displayOnly[0].DisplayLibraryPath},
	} {
		if field.full != field.only {
			t.Fatalf("display-only %s = %q, full = %q", field.name, field.only, field.full)
		}
	}

	got := items[0]
	if got.DisplayLibraryID != adult.ID {
		t.Fatalf("display_library_id = %s (%s), want auto-category library %s (成人)",
			got.DisplayLibraryID, got.DisplayLibraryName, adult.ID)
	}
	if got.DisplayLibraryName != "成人" {
		t.Fatalf("display_library_name = %q, want 成人", got.DisplayLibraryName)
	}
	if got.LibraryName != "成人" {
		t.Fatalf("library_name = %q, want 成人 (not the source cloud library)", got.LibraryName)
	}
}

func TestAttachLibraryMetadataUsesEmbyAdultDisplayTitle(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	adult := model.Library{Name: "成人", Path: "cloud://openlist/115/成人", Type: "adult", Enabled: true}
	if err := repos.Library.Create(t.Context(), &adult); err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	items := []model.Media{{
		LibraryID:    adult.ID,
		Title:        "测试标题",
		OriginalName: "SSIS-218",
		Path:         "cloud://openlist/115/成人/SSIS-218/SSIS-218.mp4",
	}}

	svc.attachLibraryMetadata(t.Context(), items)

	if items[0].DisplayTitle != "SSIS-218 测试标题" {
		t.Fatalf("display_title = %q", items[0].DisplayTitle)
	}
	if items[0].AutoCategory != "成人" {
		t.Fatalf("auto_category = %q, want 成人", items[0].AutoCategory)
	}
	if items[0].AdultType != adultMediaTypeAV {
		t.Fatalf("adult_type = %q, want AV", items[0].AdultType)
	}
}

func TestAttachLibraryMetadataMarksFC2AdultType(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	adult := model.Library{Name: "成人", Path: "cloud://openlist/115/成人", Type: "adult", Enabled: true}
	if err := repos.Library.Create(t.Context(), &adult); err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	items := []model.Media{{
		LibraryID:    adult.ID,
		Title:        "FC2 作品",
		OriginalName: "FC2-PPV-3780016",
		Path:         "cloud://openlist/115/成人/FC2-PPV-3780016/main.mp4",
	}}

	svc.attachLibraryMetadata(t.Context(), items)

	if items[0].AdultType != adultMediaTypeFC2 {
		t.Fatalf("adult_type = %q, want FC2", items[0].AdultType)
	}
}
