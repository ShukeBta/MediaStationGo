package repository

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestMediaSearchMatchesActors(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	repos := New(db)
	media := model.Media{Title: "影片", Path: "/media/movie.mkv", Actors: "演员甲,演员乙"}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}
	items, err := repos.Media.Search(t.Context(), "演员乙", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != media.ID {
		t.Fatalf("items = %#v", items)
	}
}

func TestMediaSearchMatchesGeneratedPinyinAliases(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	repos := New(db)
	rows := []model.Media{
		{Title: "黑衣人", OriginalName: "Men in Black", Path: "/media/movies/men-in-black.mkv", Actors: "刘德华", Genres: "科幻,喜剧"},
		{Title: "MIZD-534", Path: "/media/adult/MIZD-534.mp4", Actors: "七沢みあ"},
	}
	for i := range rows {
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}

	queries := map[string]string{
		"heiyiren": "黑衣人",
		"hyr":      "黑衣人",
		"kehuan":   "黑衣人",
		"kh":       "黑衣人",
		"liudehua": "黑衣人",
		"ldh":      "黑衣人",
		"mizd534":  "MIZD-534",
	}
	for query, wantTitle := range queries {
		t.Run(query, func(t *testing.T) {
			items, err := repos.Media.Search(t.Context(), query, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 || items[0].Title != wantTitle {
				t.Fatalf("search %q = %#v, want %q", query, items, wantTitle)
			}
		})
	}
}

func TestMediaSearchPrioritizesExactTitle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	repos := New(db)
	for _, media := range []model.Media{
		{Title: "黑衣人归来", Path: "/media/movies/men-in-black-returns.mkv"},
		{Title: "黑衣人", Path: "/media/movies/men-in-black.mkv"},
	} {
		row := media
		if err := repos.Media.Upsert(t.Context(), &row); err != nil {
			t.Fatal(err)
		}
	}
	items, err := repos.Media.Search(t.Context(), "黑衣人", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Title != "黑衣人" {
		t.Fatalf("exact title should rank first, got %#v", items)
	}
}
