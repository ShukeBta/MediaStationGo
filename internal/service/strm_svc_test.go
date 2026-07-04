package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestGenerateSTRMForLibraryWritesFilesAndRecords(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.STRMRecord{}, &model.Setting{})
	repos := repository.New(db)
	lib := model.Library{Name: "电影", Path: "cloud://openlist/电影", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "cloud-media"}, LibraryID: lib.ID, Title: "云盘电影", Year: 2026, Path: "cloud://openlist/电影/云盘电影.mkv", STRMURL: "/api/cloud/play/openlist?ref=movie"},
		{Base: model.Base{ID: "local-media"}, LibraryID: lib.ID, Title: "本地电影", Year: 2025, Path: filepath.Join(t.TempDir(), "本地电影.mkv")},
	}
	for i := range rows {
		if err := repos.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), repos, &config.Config{})

	res, err := svc.GenerateForLibrary(t.Context(), GenerateSTRMOptions{
		LibraryID:     lib.ID,
		OutputDir:     outDir,
		BaseURL:       "http://nas.example:18080",
		IncludeLocal:  true,
		PlaybackToken: "strm-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 2 || res.Skipped != 0 {
		t.Fatalf("result = %#v, want generated=2 skipped=0", res)
	}
	libraryOutDir := filepath.Join(outDir, "电影")
	if res.OutputDir != libraryOutDir {
		t.Fatalf("output dir = %q, want %q", res.OutputDir, libraryOutDir)
	}
	cloudSTRM := filepath.Join(libraryOutDir, "云盘电影 (2026)", "云盘电影 (2026).strm")
	localSTRM := filepath.Join(libraryOutDir, "本地电影 (2025)", "本地电影 (2025).strm")
	assertFileContains(t, cloudSTRM, "http://nas.example:18080/api/stream/cloud-media?token=strm-token")
	assertFileContains(t, localSTRM, "http://nas.example:18080/api/stream/local-media?token=strm-token")
	if got, err := repos.Setting.Get(t.Context(), "app.server_url"); err != nil || got != "http://nas.example:18080" {
		t.Fatalf("app.server_url = %q, %v; want generated base url", got, err)
	}
	if got, err := repos.Setting.Get(t.Context(), "strm.base_url"); err != nil || got != "http://nas.example:18080" {
		t.Fatalf("strm.base_url = %q, %v; want generated base url", got, err)
	}

	var count int64
	if err := repos.DB.Model(&model.STRMRecord{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("strm record count = %d, want 2", count)
	}

	res, err = svc.GenerateForLibrary(t.Context(), GenerateSTRMOptions{
		LibraryID:     lib.ID,
		OutputDir:     outDir,
		BaseURL:       "http://nas.example:18080",
		IncludeLocal:  true,
		PlaybackToken: "strm-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 2 {
		t.Fatalf("second run skipped = %d, want 2", res.Skipped)
	}
}

func TestGenerateSTRMForLibrarySignsDefaultPlaybackToken(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.STRMRecord{}, &model.Setting{}, &model.User{})
	repos := repository.New(db)
	admin := model.User{Username: "admin", PasswordHash: "x", Role: "admin", Tier: "plus", IsActive: true}
	if err := repos.User.Create(t.Context(), &admin); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "电影", Path: "cloud://openlist/电影", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{Base: model.Base{ID: "cloud-media"}, LibraryID: lib.ID, Title: "云盘电影", Year: 2026, Path: "cloud://openlist/电影/云盘电影.mkv", STRMURL: "/api/cloud/play/openlist?ref=movie"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(t.TempDir(), "strm")
	const secret = "test-secret"
	svc := NewSTRMService(zap.NewNop(), repos, &config.Config{Secrets: config.SecretsConfig{JWTSecret: secret}})
	res, err := svc.GenerateForLibrary(t.Context(), GenerateSTRMOptions{
		LibraryID:    lib.ID,
		OutputDir:    outDir,
		BaseURL:      "http://nas.example:18080",
		IncludeLocal: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want generated=1 with no errors", res)
	}
	cloudSTRM := filepath.Join(outDir, "电影", "云盘电影 (2026)", "云盘电影 (2026).strm")
	got := readSTRM(t, cloudSTRM)
	if !strings.HasPrefix(got, "http://nas.example:18080/api/stream/cloud-media?token=") {
		t.Fatalf("generated url = %q, want tokenized /api/stream url", got)
	}
	token := strings.TrimPrefix(got, "http://nas.example:18080/api/stream/cloud-media?token=")
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("generated token did not validate: %v", err)
	}
	if claims.UserID != admin.ID || claims.Role != "admin" || claims.Tier != "plus" {
		t.Fatalf("claims = %#v, want admin identity", claims)
	}
	if ttl := time.Until(claims.ExpiresAt.Time); ttl < EmbyTokenDuration-time.Minute {
		t.Fatalf("token ttl = %v, want close to %v", ttl, EmbyTokenDuration)
	}
}

func TestGenerateSTRMForLibraryCleanupStaleFilesAndRecords(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.STRMRecord{}, &model.Setting{})
	repos := repository.New(db)
	lib := model.Library{Name: "电影", Path: "cloud://openlist/电影", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{Base: model.Base{ID: "cloud-media"}, LibraryID: lib.ID, Title: "云盘电影", Year: 2026, Path: "cloud://openlist/电影/云盘电影.mkv", STRMURL: "/api/cloud/play/openlist?ref=movie"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "strm")
	libraryOutDir := filepath.Join(outDir, "电影")
	stalePath := filepath.Join(libraryOutDir, "旧电影", "旧电影.strm")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte("http://old.example/stream\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleRecord := model.STRMRecord{Title: "旧电影", URL: "http://old.example/stream", FilePath: stalePath, Protocol: "http", MediaID: "missing-media"}
	if err := repos.DB.Create(&staleRecord).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewSTRMService(zap.NewNop(), repos, &config.Config{})
	res, err := svc.GenerateForLibrary(t.Context(), GenerateSTRMOptions{
		LibraryID:     lib.ID,
		OutputDir:     outDir,
		BaseURL:       "http://nas.example:18080",
		IncludeLocal:  true,
		Overwrite:     true,
		PlaybackToken: "strm-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cleaned == 0 {
		t.Fatalf("cleaned = %d, want stale file/record cleaned", res.Cleaned)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale strm file should be removed, stat err=%v", err)
	}
	var count int64
	if err := repos.DB.Model(&model.STRMRecord{}).Where("media_id = ?", "missing-media").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale strm record count = %d, want 0", count)
	}
	if err := repos.DB.Model(&model.STRMRecord{}).Where("media_id = ?", media.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("current strm record count = %d, want 1", count)
	}
	assertFileContains(t, filepath.Join(libraryOutDir, "云盘电影 (2026)", "云盘电影 (2026).strm"), "http://nas.example:18080/api/stream/cloud-media?token=strm-token")
}

func TestSTRMLibraryOutputSubdirUsesLibraryCategoryPath(t *testing.T) {
	tests := []struct {
		name string
		lib  model.Library
		want string
	}{
		{
			name: "cloud nested tv category",
			lib:  model.Library{Name: "OpenList · 欧美剧", Path: BuildCloudLibraryPath("openlist", "/电视剧/欧美剧", "/电视剧/欧美剧"), Type: "tv"},
			want: filepath.Join("电视剧", "欧美剧"),
		},
		{
			name: "cloud second-level category without root",
			lib:  model.Library{Name: "OpenList · 国产剧", Path: BuildCloudLibraryPath("openlist", "/国产剧", "/国产剧"), Type: "tv"},
			want: filepath.Join("电视剧", "国产剧"),
		},
		{
			name: "local nested tv category",
			lib:  model.Library{Name: "欧美剧", Path: `F:\media\电视剧\欧美剧`, Type: "tv"},
			want: filepath.Join("电视剧", "欧美剧"),
		},
		{
			name: "uncategorized tv category stays uncategorized",
			lib:  model.Library{Name: "OpenList · 未分类", Path: BuildCloudLibraryPath("openlist", "/电视剧/未分类", "/电视剧/未分类"), Type: "tv"},
			want: filepath.Join("电视剧", "未分类"),
		},
		{
			name: "fallback to type root",
			lib:  model.Library{Name: "Archive", Path: `F:\archive`, Type: "movie"},
			want: "电影",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strmLibraryOutputSubdir(tt.lib); got != tt.want {
				t.Fatalf("strmLibraryOutputSubdir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSTRMLibrarySpecificOutputDirPreservesExplicitCategoryRoot(t *testing.T) {
	base := filepath.Join(t.TempDir(), "strm", "电视剧")
	lib := model.Library{
		Name: "国产剧",
		Path: BuildCloudLibraryPath("openlist", "/电视剧/国产剧", "/电视剧/国产剧"),
		Type: "tv",
	}

	got := strmLibrarySpecificOutputDir(base, &lib)
	want := filepath.Join(base, "国产剧")
	if got != want {
		t.Fatalf("strmLibrarySpecificOutputDir() = %q, want %q", got, want)
	}
}

func TestGenerateSTRMForLibraryUsesCategoryDefaultOutputDir(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.STRMRecord{}, &model.Setting{})
	repos := repository.New(db)
	dataDir := t.TempDir()
	lib := model.Library{Name: "OpenList · 欧美剧", Path: BuildCloudLibraryPath("openlist", "/电视剧/欧美剧", "/电视剧/欧美剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{Base: model.Base{ID: "show-1"}, LibraryID: lib.ID, Title: "第一集", Path: "cloud://openlist/电视剧/欧美剧/Show/S01E01.mkv", STRMURL: "/api/cloud/play/openlist?ref=show", SeasonNum: 1, EpisodeNum: 1}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewSTRMService(zap.NewNop(), repos, &config.Config{App: config.AppConfig{DataDir: dataDir}})

	res, err := svc.GenerateForLibrary(t.Context(), GenerateSTRMOptions{
		LibraryID:     lib.ID,
		BaseURL:       "http://nas.example:18080",
		IncludeLocal:  true,
		PlaybackToken: "strm-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(dataDir, "strm", "电视剧", "欧美剧")
	if res.OutputDir != wantDir {
		t.Fatalf("output dir = %q, want %q", res.OutputDir, wantDir)
	}
	assertFileContains(t, filepath.Join(wantDir, "Show", "Season 01", "Show - S01E01.strm"), "http://nas.example:18080/api/stream/show-1?token=strm-token")
}

func TestGenerateSTRMRemapsLegacyAppDataOutputDir(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.STRMRecord{}, &model.Setting{})
	repos := repository.New(db)
	dataDir := t.TempDir()
	lib := model.Library{Name: "电影", Path: "cloud://openlist/电影", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		Base:      model.Base{ID: "cloud-media"},
		LibraryID: lib.ID,
		Title:     "云盘电影",
		Year:      2026,
		Path:      "cloud://openlist/电影/云盘电影.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=movie",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "strm.output_dir", "/app/data/strm"); err != nil {
		t.Fatal(err)
	}
	svc := NewSTRMService(zap.NewNop(), repos, &config.Config{App: config.AppConfig{DataDir: dataDir}})

	res, err := svc.GenerateForLibrary(t.Context(), GenerateSTRMOptions{
		LibraryID:     lib.ID,
		BaseURL:       "http://nas.example:18080",
		PlaybackToken: "strm-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(dataDir, "strm", "电影")
	if res.OutputDir != wantDir {
		t.Fatalf("output dir = %q, want %q", res.OutputDir, wantDir)
	}
	if got, err := repos.Setting.Get(t.Context(), "strm.output_dir"); err != nil || got != wantDir {
		t.Fatalf("saved strm.output_dir = %q, %v; want %q", got, err, wantDir)
	}
	assertFileContains(t, filepath.Join(wantDir, "云盘电影 (2026)", "云盘电影 (2026).strm"), "http://nas.example:18080/api/stream/cloud-media?token=strm-token")
}

func TestGenerateSTRMForLibraryUsesPathEpisodeFallback(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.STRMRecord{}, &model.Setting{})
	repos := repository.New(db)
	lib := model.Library{Name: "OpenList · 国产剧", Path: BuildCloudLibraryPath("openlist", "/电视剧/国产剧", "/电视剧/国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "ep-1"}, LibraryID: lib.ID, Title: "南部档案", Path: "cloud://openlist/电视剧/国产剧/南部档案/Season 01/Archives.The.Nanyang.Mystery.S01E01.mkv", STRMURL: "/api/cloud/play/openlist?ref=ep1"},
		{Base: model.Base{ID: "ep-2"}, LibraryID: lib.ID, Title: "南部档案", Path: "cloud://openlist/电视剧/国产剧/南部档案/Season 01/Archives.The.Nanyang.Mystery.S01E02.mkv", STRMURL: "/api/cloud/play/openlist?ref=ep2"},
	}
	for i := range rows {
		if err := repos.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), repos, &config.Config{})
	res, err := svc.GenerateForLibrary(t.Context(), GenerateSTRMOptions{
		LibraryID:     lib.ID,
		OutputDir:     outDir,
		BaseURL:       "http://nas.example:18080",
		IncludeLocal:  true,
		PlaybackToken: "strm-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 2 || res.Skipped != 0 {
		t.Fatalf("result = %#v, want generated=2 skipped=0", res)
	}
	libraryOutDir := filepath.Join(outDir, "电视剧", "国产剧")
	assertFileContains(t, filepath.Join(libraryOutDir, "南部档案", "Season 01", "南部档案 - S01E01.strm"), "http://nas.example:18080/api/stream/ep-1?token=strm-token")
	assertFileContains(t, filepath.Join(libraryOutDir, "南部档案", "Season 01", "南部档案 - S01E02.strm"), "http://nas.example:18080/api/stream/ep-2?token=strm-token")
}

func TestGenerateSTRMForLibraryPreservesSourceTree(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.STRMRecord{}, &model.Setting{})
	repos := repository.New(db)
	lib := model.Library{Name: "OpenList · 国产剧", Path: BuildCloudLibraryPath("openlist", "/电视剧/国产剧", "/电视剧/国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		Base:      model.Base{ID: "ep-1"},
		LibraryID: lib.ID,
		Title:     "南部档案",
		Path:      "cloud://openlist/电视剧/国产剧/南部档案/Season 01/Archives.The.Nanyang.Mystery.S01E01.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=ep1",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), repos, &config.Config{})
	res, err := svc.GenerateForLibrary(t.Context(), GenerateSTRMOptions{
		LibraryID:     lib.ID,
		OutputDir:     outDir,
		BaseURL:       "http://nas.example:18080",
		IncludeLocal:  true,
		PreserveTree:  true,
		PlaybackToken: "strm-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || res.Skipped != 0 {
		t.Fatalf("result = %#v, want generated=1 skipped=0", res)
	}
	wantPath := filepath.Join(outDir, "电视剧", "国产剧", "南部档案", "Season 01", "Archives.The.Nanyang.Mystery.S01E01.strm")
	assertFileContains(t, wantPath, "http://nas.example:18080/api/stream/ep-1?token=strm-token")
	if got, err := repos.Setting.Get(t.Context(), "strm.preserve_tree"); err != nil || got != "true" {
		t.Fatalf("strm.preserve_tree = %q, %v; want true", got, err)
	}
}

func TestGenerateSTRMForLibraryCanSkipLocalMedia(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.STRMRecord{}, &model.Setting{})
	repos := repository.New(db)
	lib := model.Library{Name: "电影", Path: "cloud://openlist/电影", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "cloud-media"}, LibraryID: lib.ID, Title: "云盘电影", Year: 2026, Path: "cloud://openlist/电影/云盘电影.mkv", STRMURL: "/api/cloud/play/openlist?ref=movie"},
		{Base: model.Base{ID: "local-media"}, LibraryID: lib.ID, Title: "本地电影", Year: 2025, Path: filepath.Join(t.TempDir(), "本地电影.mkv")},
	}
	for i := range rows {
		if err := repos.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), repos, &config.Config{})
	res, err := svc.GenerateForLibrary(t.Context(), GenerateSTRMOptions{
		LibraryID:     lib.ID,
		OutputDir:     outDir,
		BaseURL:       "http://nas.example:18080",
		IncludeLocal:  false,
		PlaybackToken: "strm-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || res.Skipped != 1 {
		t.Fatalf("result = %#v, want generated=1 skipped=1", res)
	}
	assertFileContains(t, filepath.Join(outDir, "电影", "云盘电影 (2026)", "云盘电影 (2026).strm"), "http://nas.example:18080/api/stream/cloud-media?token=strm-token")
	if _, err := os.Stat(filepath.Join(outDir, "电影", "本地电影 (2025)", "本地电影 (2025).strm")); !os.IsNotExist(err) {
		t.Fatalf("local media strm should not exist, stat err=%v", err)
	}
}

func TestGenerateSTRMForAllLibrariesWritesPerLibraryFolders(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.STRMRecord{}, &model.Setting{})
	repos := repository.New(db)
	movieLib := model.Library{Name: "电影", Path: "cloud://openlist/电影", Type: "movie", Enabled: true}
	tvLib := model.Library{Name: "欧美剧", Path: BuildCloudLibraryPath("openlist", "/电视剧/欧美剧", "/电视剧/欧美剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &movieLib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &tvLib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "movie-1"}, LibraryID: movieLib.ID, Title: "云盘电影", Year: 2026, Path: "cloud://openlist/电影/云盘电影.mkv", STRMURL: "/api/cloud/play/openlist?ref=movie"},
		{Base: model.Base{ID: "show-1"}, LibraryID: tvLib.ID, Title: "第一集", Path: "cloud://openlist/电视剧/欧美剧/Show/S01E01.mkv", STRMURL: "/api/cloud/play/openlist?ref=show", SeasonNum: 1, EpisodeNum: 1},
	}
	for i := range rows {
		if err := repos.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	outDir := filepath.Join(t.TempDir(), "strm-all")
	svc := NewSTRMService(zap.NewNop(), repos, &config.Config{})
	res, err := svc.GenerateForAllLibraries(t.Context(), GenerateSTRMOptions{
		OutputDir:     outDir,
		BaseURL:       "http://nas.example:18080",
		IncludeLocal:  true,
		Overwrite:     true,
		PlaybackToken: "strm-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 2 {
		t.Fatalf("generated = %d, want 2", res.Generated)
	}
	assertFileContains(t, filepath.Join(outDir, "电影", "云盘电影 (2026)", "云盘电影 (2026).strm"), "http://nas.example:18080/api/stream/movie-1?token=strm-token")
	assertFileContains(t, filepath.Join(outDir, "电视剧", "欧美剧", "Show", "Season 01", "Show - S01E01.strm"), "http://nas.example:18080/api/stream/show-1?token=strm-token")
	var count int64
	if err := repos.DB.Model(&model.STRMRecord{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("active strm record count = %d, want 2", count)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	if got := readSTRM(t, path); got != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func readSTRM(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}
