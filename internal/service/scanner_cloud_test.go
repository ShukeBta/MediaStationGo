package service

import (
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestCloudLibraryPathParsing(t *testing.T) {
	typ, dir, ok := parseCloudLibraryPath("cloud://cloud115/abc%20123?ignored=1")
	if !ok || typ != "cloud115" || dir != "abc 123" {
		t.Fatalf("parse path got typ=%q dir=%q ok=%v", typ, dir, ok)
	}
	typ, dir, ok = parseCloudLibraryPath("cloud://openlist/Movies?dir=%2FMovies")
	if !ok || typ != "openlist" || dir != "Movies" {
		t.Fatalf("parse query got typ=%q dir=%q ok=%v", typ, dir, ok)
	}
	if ref := cloudEntryRef("cloud115", "fid", "pick"); ref != "pick" {
		t.Fatalf("115 ref = %q, want pick", ref)
	}
}

func TestCloudMountConflictDetectsNestedMounts(t *testing.T) {
	root := model.Library{Base: model.Base{ID: "root"}, Name: "115", Path: "cloud://cloud115", Enabled: true}
	childPath := BuildCloudLibraryPath("cloud115", "child-id", "parent-id/child-id")
	info, ok := ParseCloudLibraryMount(childPath)
	if !ok || info.ScanDir != "child-id" || info.DisplayDir != "parent-id/child-id" {
		t.Fatalf("parse child mount = %#v ok=%v", info, ok)
	}

	conflict := FindCloudMountConflict([]model.Library{root}, "cloud115", "child-id", "parent-id/child-id")
	if conflict != nil {
		t.Fatalf("child mount under existing root should be allowed, got conflict %#v", conflict)
	}

	sibling := model.Library{Base: model.Base{ID: "sibling"}, Name: "Sibling", Path: BuildCloudLibraryPath("cloud115", "sibling-id", "parent-id/sibling-id"), Enabled: true}
	conflict = FindCloudMountConflict([]model.Library{sibling}, "cloud115", "child-id", "parent-id/child-id")
	if conflict != nil {
		t.Fatalf("sibling conflict = %#v, want nil", conflict)
	}

	conflict = FindCloudMountConflict([]model.Library{sibling}, "cloud115", "parent-id", "parent-id")
	if conflict == nil || !conflict.Nested {
		t.Fatalf("parent mount over existing child = %#v, want nested conflict", conflict)
	}
	oldIDPath := BuildCloudLibraryPath("cloud115", "child-id", "old-parent-id/child-id")
	conflict = FindCloudMountConflict([]model.Library{{Base: model.Base{ID: "old"}, Name: "Old", Path: oldIDPath, Enabled: true}}, "cloud115", "child-id", "父目录/子目录")
	if conflict == nil || !conflict.Exact {
		t.Fatalf("same scan dir with renamed display path = %#v, want exact conflict", conflict)
	}

	root.CreatedAt = root.CreatedAt.Add(-1)
	child := model.Library{Base: model.Base{ID: "child"}, Name: "Child", Path: childPath, Enabled: true}
	if shadow := CloudLibraryShadowed([]model.Library{root, child}, child); shadow != nil {
		t.Fatalf("child should not be shadowed by root: %#v", shadow)
	}
	if shadow := CloudLibraryShadowed([]model.Library{root, child}, root); shadow == nil || !shadow.Nested {
		t.Fatalf("root should be shadowed by child, got %#v", shadow)
	}
}

func TestCancelCloudScansForProviderSignalsRunningScan(t *testing.T) {
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repository.New(nil), NewHub(zap.NewNop()), nil, nil)
	cancelled := false
	scanner.cloudScans["lib-1"] = &cloudScanEntry{
		status: CloudScanStatus{LibraryID: "lib-1", Provider: "openlist", State: "running"},
		cancel: func() {
			cancelled = true
		},
	}

	if got := scanner.CancelCloudScansForProvider("openlist"); got != 1 {
		t.Fatalf("cancelled = %d, want 1", got)
	}
	if !cancelled {
		t.Fatal("cancel func was not called")
	}
	if state := scanner.cloudScans["lib-1"].status.State; state != "canceling" {
		t.Fatalf("state = %q, want canceling", state)
	}
}

func TestInferCloudMountMediaType(t *testing.T) {
	cases := map[string]string{
		"/日漫":      "anime",
		"/国漫":      "anime",
		"/美漫":      "anime",
		"/电视剧/国产剧": "tv",
		"/电视剧/欧美剧": "tv",
		"/电视剧/日韩剧": "tv",
		"/电影/动画电影": "movie",
		"/电影/华语电影": "movie",
		"/电影/欧美电影": "movie",
		"/综艺":      "variety",
	}
	for dir, want := range cases {
		if got := InferCloudMountMediaType(dir, "OpenList · "+dir); got != want {
			t.Fatalf("%s type = %s, want %s", dir, got, want)
		}
	}
}

func TestCloudSeriesTitlePrefersShowFolder(t *testing.T) {
	title, year := cloudSeriesTitleFromMediaPath("cloud://openlist/国产剧/紫川 (2024) {tmdb-247590}/Season 2/紫川.2024.S02E24.第24集.2160p.WEB-DL.H.265-ColorTV.mkv")
	if title != "紫川" || year != 2024 {
		t.Fatalf("cloud series title = %q/%d, want 紫川/2024", title, year)
	}
	title, year = cloudSeriesTitleFromMediaPath("cloud://openlist/国产剧/紫川.2024.S02E24.mkv")
	if title != "" || year != 0 {
		t.Fatalf("single category folder should not override title, got %q/%d", title, year)
	}
}

func TestCloudMetadataNeedsRefreshWhenPathHintConflicts(t *testing.T) {
	existing := existingCloudMedia{
		Year:   2025,
		TMDbID: 220269,
	}
	local := &LocalMetadata{
		Year:     2025,
		TMDbID:   296753,
		PathHint: true,
	}
	if !cloudMetadataNeedsRefresh(existing, local) {
		t.Fatal("conflicting explicit cloud path hint should refresh existing media")
	}
}

func TestParseCloudArtworkURL(t *testing.T) {
	typ, ref, ok := ParseCloudArtworkURL("http://nas.local/api/cloud/play/openlist?ref=%2FAnime%2FJianLai%2Fposter.jpg")
	if !ok || typ != "openlist" || ref != "/Anime/JianLai/poster.jpg" {
		t.Fatalf("parse cloud image url = typ=%q ref=%q ok=%v", typ, ref, ok)
	}
	typ, ref, ok = ParseCloudArtworkURL("/api/img/cloud/openlist?ref=%2FAnime%2FJianLai%2Fposter.jpg")
	if !ok || typ != "openlist" || ref != "/Anime/JianLai/poster.jpg" {
		t.Fatalf("parse cached cloud artwork url = typ=%q ref=%q ok=%v", typ, ref, ok)
	}
	typ, ref, ok = ParseCloudArtworkURL("/api/img/cloud/openlist?ref=%2FMovies%2FMovie.tbn")
	if !ok || typ != "openlist" || ref != "/Movies/Movie.tbn" {
		t.Fatalf("parse tbn cloud artwork url = typ=%q ref=%q ok=%v", typ, ref, ok)
	}
	if _, _, ok := ParseCloudArtworkURL("/api/cloud/play/openlist?ref=%2FAnime%2FJianLai%2Fmovie.mkv"); ok {
		t.Fatal("video cloud url should not be treated as artwork")
	}
	if _, _, ok := ParseCloudArtworkURL("https://image.tmdb.org/t/p/w500/poster.jpg"); ok {
		t.Fatal("remote HTTP poster should not be treated as cloud artwork")
	}
}
