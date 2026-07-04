package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOrganizeDirectoryClassifiesScraperMatchBeforeRename(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/search/movie":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                45745,
					"title":             "寻龙记",
					"original_title":    "Sintel",
					"original_language": "en",
					"genre_ids":         []int{16, 14},
					"release_date":      "2010-09-27",
					"vote_average":      7.4,
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Sintel.2010.1080p.CodexVerify.mp4")
	writeOrgFile(t, sourceFile, "movie")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "movie",
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电影", "动画电影", "寻龙记 (2010)", "寻龙记 (2010).mp4")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized movie should use metadata category path %q: %v; items=%#v", want, err, res.Items)
	}
	if len(res.Items) != 1 || res.Items[0].Category != "动画电影" {
		t.Fatalf("organize category = %#v, want 动画电影", res.Items)
	}
}

func TestOrganizeDirectoryMetadataCategoryOverridesDownloadFolder(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                12345,
					"name":              "间谍过家家",
					"original_name":     "SPY×FAMILY",
					"original_language": "ja",
					"origin_country":    []string{"JP"},
					"genre_ids":         []int{16, 35},
					"first_air_date":    "2022-04-09",
					"vote_average":      8.6,
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(srcRoot, "国产剧", "Spy.x.Family.S01E01.2022.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   srcRoot,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "动漫", "日番", "间谍过家家", "Season 01", "间谍过家家 - S01E01.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("metadata category should override wrong source folder at %q: %v; items=%#v", want, err, res.Items)
	}
	if len(res.Items) != 1 || res.Items[0].Category != "日番" || res.Items[0].MediaType != "anime" {
		t.Fatalf("organize metadata category/type = %#v, want 日番/anime", res.Items)
	}
}

func TestOrganizeDirectoryMovieMetadataOverridesWrongTVFolder(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		paths = append(paths, r.URL.Path)
		switch {
		case r.URL.Path == "/search/movie":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                1292695,
					"title":             "杀的就是你",
					"original_title":    "They Will Kill You",
					"original_language": "en",
					"genre_ids":         []int{27, 53},
					"release_date":      "2026-01-16",
					"vote_average":      6.3,
				}},
			})
		case r.URL.Path == "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                1198994,
					"name":              "请求救援",
					"original_name":     "They Will Kill You",
					"original_language": "en",
					"origin_country":    []string{"US"},
					"genre_ids":         []int{18},
					"first_air_date":    "2026-01-01",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(srcRoot, "欧美剧", "They.Will.Kill.You.2026.1080p.HDTV.x264-HiDt.mkv")
	writeOrgFile(t, sourceFile, "movie")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   srcRoot,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电影", "欧美电影", "杀的就是你 (2026)", "杀的就是你 (2026).mkv")
	if res.Organized != 1 || res.Reclassified != 0 {
		t.Fatalf("result = %+v, want organized movie only; paths=%v", res, paths)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("movie in wrong TV folder should organize as movie at %q: %v; items=%#v paths=%v", want, err, res.Items, paths)
	}
	if len(paths) == 0 || paths[0] != "/search/movie" {
		t.Fatalf("first metadata search path = %q, want /search/movie; all=%v", firstQuery(paths), paths)
	}
	if len(res.Items) != 1 || res.Items[0].MediaType != "movie" || res.Items[0].Category != "欧美电影" {
		t.Fatalf("organize item = %#v, want movie/欧美电影", res.Items)
	}
}

func TestOrganizeDirectoryReclassifiesMovieFromDirtyGeneratedEpisodePath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/search/movie":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                1198994,
					"title":             "请求救援",
					"original_title":    "Request Rescue",
					"original_language": "en",
					"genre_ids":         []int{28, 53},
					"release_date":      "2026-02-01",
				}},
			})
		case r.URL.Path == "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	dest := filepath.Join(root, "media")
	euusLib := model.Library{Name: "欧美剧", Path: filepath.Join(dest, "电视剧", "欧美剧"), Type: "tv", Enabled: true}
	foreignMovieLib := model.Library{Name: "欧美电影", Path: filepath.Join(dest, "电影", "欧美电影"), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &euusLib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &foreignMovieLib); err != nil {
		t.Fatal(err)
	}

	wrongPath := filepath.Join(euusLib.Path, "请求救援 (2026)", "Season 1", "请求救援 - S01E202-1080p - 第 202 集.mkv")
	writeOrgFile(t, wrongPath, "movie")
	if err := repos.DB.Create(&model.Media{
		LibraryID:    euusLib.ID,
		Title:        "请求救援",
		OriginalName: "请求救援",
		Path:         wrongPath,
		SeasonNum:    1,
		EpisodeNum:   202,
		TMDbID:       1198994,
		Year:         2026,
		PosterURL:    "https://image.tmdb.org/t/p/w500/poster.jpg",
		BackdropURL:  "https://image.tmdb.org/t/p/w1280/backdrop.jpg",
		Overview:     "movie overview",
		ScrapeStatus: "matched",
		Container:    "mkv",
		SizeBytes:    int64(len("movie")),
		DurationSec:  7200,
		VideoCodec:   "h264",
		AudioCodec:   "aac",
	}).Error; err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   filepath.Dir(filepath.Dir(wrongPath)),
		DestPath:     euusLib.Path,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize dirty generated path: %v", err)
	}
	want := filepath.Join(foreignMovieLib.Path, "请求救援 (2026)", "请求救援 (2026).mkv")
	if res.Reclassified != 1 || res.Organized != 0 {
		t.Fatalf("result = %+v, want one reclassified movie", res)
	}
	if _, err := os.Stat(wrongPath); !os.IsNotExist(err) {
		t.Fatalf("wrong generated episode path should be moved away, stat err=%v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("movie target missing at %q: %v; items=%#v", want, err, res.Items)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", want).Error; err != nil {
		t.Fatal(err)
	}
	if got.LibraryID != foreignMovieLib.ID || got.SeasonNum != 0 || got.EpisodeNum != 0 {
		t.Fatalf("row after reclassify = library %q S%dE%d, want movie lib with cleared episode numbers", got.LibraryID, got.SeasonNum, got.EpisodeNum)
	}
}

func TestOrganizeDirectoryRejectsLooseChineseAliasMetadataForWesternSourceCategory(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/tv" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{
				"id":                107463,
				"name":              "镖人",
				"original_name":     "Biao Ren",
				"original_language": "zh",
				"origin_country":    []string{"CN"},
				"genre_ids":         []int{16, 18},
				"first_air_date":    "2023-06-01",
				"vote_average":      8.0,
			}},
		})
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(srcRoot, "欧美剧", "Blades.of.the.Guardians.S02.1080p.TX.WEB-DL", "Blades.of.the.Guardians.S02E01.1080p.TX.WEB-DL.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   srcRoot,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电视剧", "欧美剧", "Blades Of The Guardians", "Season 02", "Blades Of The Guardians - S02E01.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("loose Chinese alias metadata should be rejected and keep source category at %q: %v; items=%#v", want, err, res.Items)
	}
	wrong := filepath.Join(dest, "动漫", "国漫", "镖人")
	if _, err := os.Stat(wrong); !os.IsNotExist(err) {
		t.Fatalf("rejected loose alias should not create Chinese anime category at %q, err=%v", wrong, err)
	}
	if len(res.Items) != 1 || res.Items[0].Category != "欧美剧" || res.Items[0].MediaType != "tv" || res.Items[0].Title != "Blades Of The Guardians" {
		t.Fatalf("organize item = %#v, want Blades Of The Guardians in 欧美剧/tv", res.Items)
	}
}

func TestOrganizeDirectoryDoesNotScrapeByDownloadCategoryFolder(t *testing.T) {
	var queries []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/tv" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query().Get("query")
		queries = append(queries, query)
		switch {
		case query == "国产剧":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":             843248,
					"name":           "高达 G之复国运动 剧场版III 来自宇宙的遗产",
					"first_air_date": "2021-07-22",
				}},
			})
		case strings.EqualFold(query, "ashes to crown"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                289271,
					"name":              "翘楚",
					"original_name":     "Ashes to Crown",
					"original_language": "zh",
					"origin_country":    []string{"CN"},
					"genre_ids":         []int{18},
					"first_air_date":    "2026-06-01",
				}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(root, "downloads", "国产剧", "Ashes.to.Crown.S01.1080p.YOUKU.WEB-DL.AAC2.0.H.264-MWeb", "Ashes.to.Crown.S01E06.1080p.YOUKU.WEB-DL.AAC2.0.H.264-MWeb.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:           sourceFile,
		DestPath:             dest,
		TransferMode:         TransferCopy,
		MediaType:            "tv",
		MediaCategory:        "国产剧",
		AllowReplaceExisting: false,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电视剧", "国产剧", "翘楚", "Season 01", "翘楚 - S01E06.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v queries=%#v", res.Organized, res.Items, res.Errors, queries)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file should use release title metadata, not category query, at %q: %v; items=%#v queries=%#v", want, err, res.Items, queries)
	}
	if len(queries) == 0 || queries[0] == "国产剧" {
		t.Fatalf("first scrape query = %#v, want release title before category folder", queries)
	}
	if len(res.Items) != 1 || res.Items[0].Title != "翘楚" || res.Items[0].Category != "国产剧" {
		t.Fatalf("organize item = %#v, want title 翘楚 in category 国产剧", res.Items)
	}
}

func TestOrganizeDirectoryUsesPathTMDbIDBeforeTitleSearch(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/tv/156568":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                156568,
				"name":              "人世间",
				"original_name":     "A Lifelong Journey",
				"overview":          "正确的剧集条目",
				"poster_path":       "/lifelong.jpg",
				"first_air_date":    "2022-01-28",
				"origin_country":    []string{"CN"},
				"genre_ids":         []int{18},
				"vote_average":      8.1,
				"original_language": "zh",
			})
		case "/search/tv", "/search/movie":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":             999999,
					"name":           "错误候选",
					"first_air_date": "2022-01-01",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(srcRoot, "东南亚电影", "人世间 (2022) [tmdbid-156568]", "人世间.S01E01.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   srcRoot,
		DestPath:     filepath.Join(dest, "电视剧", "国产电视剧"),
		MediaType:    "tv",
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电视剧", "国产剧", "人世间", "Season 01", "人世间 - S01E01.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v paths=%v", res.Organized, res.Items, res.Errors, paths)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("tmdb-id organize target missing at %q: %v; items=%#v paths=%v", want, err, res.Items, paths)
	}
	if len(paths) == 0 || paths[0] != "/tv/156568" {
		t.Fatalf("first metadata lookup path=%q, want /tv/156568; all=%v", firstQuery(paths), paths)
	}
	for _, path := range paths {
		if strings.HasPrefix(path, "/search/") {
			t.Fatalf("path tmdb id should avoid fuzzy search; paths=%v", paths)
		}
	}
	if len(res.Items) != 1 || res.Items[0].Category != "国产剧" || res.Items[0].MediaType != "tv" {
		t.Fatalf("organize item = %#v, want 国产剧/tv", res.Items)
	}
}

func TestOrganizeDirectoryEpisodeMarkerOverridesMovieSourceFolder(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                100088,
					"name":              "The Last of Us",
					"original_language": "en",
					"origin_country":    []string{"US"},
					"genre_ids":         []int{18},
					"first_air_date":    "2023-01-15",
					"vote_average":      8.7,
				}},
			})
		case r.URL.Path == "/search/movie":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":           999,
					"title":        "Wrong Movie",
					"release_date": "2023-01-01",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(srcRoot, "欧美电影", "The.Last.of.Us.S01E01.2023.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   srcRoot,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电视剧", "欧美剧", "The Last of Us", "Season 01", "The Last of Us - S01E01.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("episode marker should force TV organize path %q: %v; items=%#v", want, err, res.Items)
	}
	if len(res.Items) != 1 || res.Items[0].Category != "欧美剧" || res.Items[0].MediaType != "tv" {
		t.Fatalf("organize episode category/type = %#v, want 欧美剧/tv", res.Items)
	}
}
