package service

import (
	"slices"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestStorageBreakdownUsesCanonicalLibraryDisplay(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	libs := []model.Library{
		{Name: "外语电影", Path: "/media/电影/外语电影", Type: "movie", Enabled: true},
		{Name: "欧美动漫", Path: "/media/动漫/欧美动漫", Type: "tv", Enabled: true},
		{Name: "9KG", Path: "/media/成人/9KG", Type: "movie", Enabled: true},
	}
	for i := range libs {
		if err := repos.Library.Create(t.Context(), &libs[i]); err != nil {
			t.Fatal(err)
		}
		if err := repos.Media.Upsert(t.Context(), &model.Media{
			LibraryID: libs[i].ID,
			Title:     libs[i].Name,
			Path:      libs[i].Path + "/item.mkv",
			SizeBytes: 1024,
		}); err != nil {
			t.Fatal(err)
		}
	}

	breakdown, err := NewStorageService(zap.NewNop(), repos).Compute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	gotNames := make([]string, 0, len(breakdown.ByLibrary))
	gotTypes := make([]string, 0, len(breakdown.ByLibrary))
	for _, row := range breakdown.ByLibrary {
		gotNames = append(gotNames, row.Name)
		gotTypes = append(gotTypes, row.Type)
	}
	if want := []string{"欧美电影", "美漫", "成人"}; !slices.Equal(gotNames, want) {
		t.Fatalf("library names = %#v, want %#v", gotNames, want)
	}
	if want := []string{"movie", "anime", "adult"}; !slices.Equal(gotTypes, want) {
		t.Fatalf("library types = %#v, want %#v", gotTypes, want)
	}
}
