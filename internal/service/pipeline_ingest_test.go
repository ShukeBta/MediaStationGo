package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestPipelineIngestFindsMediaByTargetPath(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	maintenance := NewPipelineMaintenanceService(zap.NewNop(), repos)
	svc := NewPipelineIngestService(zap.NewNop(), repos, nil, maintenance, nil)

	lib, root := createPipelineMaintenanceRoot(t, db, "adult", "/115/adult")
	rows := []model.Media{
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "PV", Path: "cloud://openlist/115/adult/ABF-363/特典/PV.mp4", SizeBytes: 10},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "ABF-363", Path: "cloud://openlist/115/adult/ABF-363/ABF-363.mp4", SizeBytes: 3000},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	main := rows[1]

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{
			Category:         "adult",
			LibraryID:        lib.ID,
			RootID:           root.ID,
			RootOpenListPath: "/115/adult",
		},
		Title:               "ABF-363",
		TargetOpenListPaths: []string{"/115/adult/ABF-363"},
		RequireTargetPath:   true,
		Scan:                false,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted {
		t.Fatalf("job status=%s error=%s", job.Status, job.Error)
	}
	if job.Result.Media == nil || job.Result.Media.ID != main.ID || job.Result.Media.MatchMode != "path" {
		t.Fatalf("unexpected media result: %#v", job.Result.Media)
	}
	if len(job.Result.MediaItems) != 2 {
		t.Fatalf("media items = %#v, want both target-path records", job.Result.MediaItems)
	}
}

func TestChoosePipelineIngestMediaPrefersEpisodeForTVPack(t *testing.T) {
	rows := []model.Media{
		{Base: model.Base{ID: "movie"}, Path: "cloud://openlist/115/tv/Pack/Related Movies/Stargate/Stargate.1994.mkv", SizeBytes: 36_000},
		{Base: model.Base{ID: "episode"}, Path: "cloud://openlist/115/tv/Pack/SG-1/Season 01/Stargate.SG-1.S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, SizeBytes: 16_000},
	}

	selected := choosePipelineIngestMedia(rows, "/115/tv", "tv")
	if selected.ID != "episode" {
		t.Fatalf("selected=%#v, want episodic row", selected)
	}
	selected = choosePipelineIngestMedia(rows, "/115/tv", "movie")
	if selected.ID != "movie" {
		t.Fatalf("selected=%#v, want largest movie row without episodic preference", selected)
	}
}

func TestPipelineIngestCandidateFilterPreservesEpisodesAndReportsHidePatterns(t *testing.T) {
	filter := pipelineIngestCloudCandidateFilter(PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "movie"},
		FilterSmallVideoMaxBytes:  100 * 1024 * 1024,
	})
	candidates := []cloudCandidate{
		{name: "Movie.mkv", path: "cloud://openlist/115/movie/Movie/Movie.mkv", size: 800 * 1024 * 1024},
		{name: "E01.mkv", path: "cloud://openlist/115/movie/Movie/E01.mkv", size: 10 * 1024 * 1024},
		{name: "[GM-Team][国漫][遮天][2023][176][AVC][GB][1080P].mp4", path: "cloud://openlist/115/movie/Movie/[GM-Team][国漫][遮天][2023][176][AVC][GB][1080P].mp4", size: 10 * 1024 * 1024},
		{name: "trailer.mp4", path: "cloud://openlist/115/movie/Movie/trailer.mp4", size: 20 * 1024 * 1024},
		{name: "sample.mp4", path: "cloud://openlist/115/movie/Movie/Extras/sample.mp4", size: 10 * 1024 * 1024},
	}

	accepted, ignored := filter(candidates)
	if len(accepted) != 3 || accepted[0].name != "Movie.mkv" || accepted[1].name != "E01.mkv" || accepted[2].name != "[GM-Team][国漫][遮天][2023][176][AVC][GB][1080P].mp4" {
		t.Fatalf("accepted = %#v", accepted)
	}
	items := pipelineIngestIgnoredMediaResults(ignored)
	if len(items) != 2 {
		t.Fatalf("ignored = %#v", items)
	}
	if items[0].OpenListPath != "/115/movie/Movie/Extras" || items[0].HidePath != "/115/movie/Movie" || items[0].HidePattern != "^Extras$" {
		t.Fatalf("extra directory hide = %#v", items[0])
	}
	if items[1].OpenListPath != "/115/movie/Movie/trailer.mp4" || items[1].Reason != "known_junk_name" {
		t.Fatalf("trailer hide = %#v", items[1])
	}
}

func TestPipelineIngestScansOnlyTargetOpenListPath(t *testing.T) {
	requested := []openListListTestRequest{}
	upstream := newOpenListAPIServerWithRequests(t, func(req openListListTestRequest) ([]openListTestEntry, int) {
		requested = append(requested, req)
		switch req.Path {
		case "/115/adult":
			return []openListTestEntry{{Name: "EBWH-285", IsDir: true}, {Name: "OTHER-001", IsDir: true}}, 2
		case "/115/adult/EBWH-285":
			return []openListTestEntry{{Name: "EBWH-285.mp4", Size: 5000}}, 1
		case "/115/adult/OTHER-001":
			t.Fatalf("targeted ingest should not recursively scan sibling directory %q", req.Path)
			return nil, 0
		default:
			t.Fatalf("unexpected openlist path %q", req.Path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "openlist-token"}}); err != nil {
		t.Fatal(err)
	}
	libPath := BuildCloudLibraryPath("openlist", "/115/adult", "/115/adult")
	lib := model.Library{Name: "Adult", Path: libPath, Type: "adult", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "Adult", Path: libPath, Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	maintenance := NewPipelineMaintenanceService(log, repos)
	svc := NewPipelineIngestService(log, repos, scanner, maintenance, nil)

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "adult", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/adult"},
		Title:                     "EBWH-285",
		TargetOpenListPaths:       []string{"/115/adult/EBWH-285"},
		RequireTargetPath:         true,
		Scan:                      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted {
		t.Fatalf("job status=%s error=%s", job.Status, job.Error)
	}
	if job.Result.Scan == nil || job.Result.Scan.Visited != 1 || job.Result.Scan.Added != 1 {
		t.Fatalf("scan result = %#v, want visited=1 added=1", job.Result.Scan)
	}
	if job.Result.Media == nil || job.Result.Media.MatchMode != "path" || job.Result.Media.Path != "cloud://openlist/115/adult/EBWH-285/EBWH-285.mp4" {
		t.Fatalf("unexpected media result: %#v", job.Result.Media)
	}
	if len(requested) != 2 || requested[0].Path != "/115/adult" || requested[1].Path != "/115/adult/EBWH-285" {
		t.Fatalf("openlist paths requested = %#v, want parent then target only", requested)
	}
	if !requested[0].Refresh || !requested[1].Refresh {
		t.Fatalf("openlist refresh flags = %#v, want parent and target refreshed", requested)
	}
}

func TestPipelineIngestStableTreeWaitsThenScansTargetExactlyOnce(t *testing.T) {
	var packRefreshes atomic.Int32
	var parentRefreshed atomic.Bool
	var deepRefresh atomic.Bool
	var visibilityReady atomic.Bool
	upstream := newOpenListAPIServerWithRequests(t, func(req openListListTestRequest) ([]openListTestEntry, int) {
		if strings.Contains(req.Path, "/Pack/Movie") && req.Refresh {
			deepRefresh.Store(true)
		}
		switch req.Path {
		case "/115/movie":
			if req.Refresh {
				parentRefreshed.Store(true)
			}
			return []openListTestEntry{{Name: "Pack", IsDir: true}}, 1
		case "/115/movie/Pack":
			if req.Refresh {
				packRefreshes.Add(1)
			}
			entries := []openListTestEntry{{Name: "Movie1", IsDir: true}}
			if visibilityReady.Load() {
				entries = append(entries, openListTestEntry{Name: "Movie2", IsDir: true})
			}
			return entries, len(entries)
		case "/115/movie/Pack/Movie1":
			return []openListTestEntry{{Name: "Movie1.mkv", Size: 5000}}, 1
		case "/115/movie/Pack/Movie2":
			return []openListTestEntry{{Name: "Movie2.mkv", Size: 6000}}, 1
		default:
			t.Fatalf("unexpected OpenList path %q", req.Path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "openlist-token"}}); err != nil {
		t.Fatal(err)
	}
	libPath := BuildCloudLibraryPath("openlist", "/115/movie", "/115/movie")
	lib := model.Library{Name: "Movie", Path: libPath, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "Movie", Path: libPath, Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	svc := NewPipelineIngestService(log, repos, scanner, NewPipelineMaintenanceService(log, repos), nil)
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	svc.now = func() time.Time { return time.Unix(0, clock.Load()) }
	svc.wait = func(_ context.Context, duration time.Duration) error {
		clock.Add(int64(duration))
		visibilityReady.Store(true)
		return nil
	}

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "movie", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/movie"},
		Title:                     "Pack", TargetOpenListPaths: []string{"/115/movie/Pack"}, RequireTargetPath: true, Scan: true, RequireStableTree: true, TargetParentsVerified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted || job.Result.Convergence == nil {
		t.Fatalf("job=%#v", job)
	}
	if job.Result.Convergence.Attempt != 1 || job.Result.Convergence.StableForSeconds != 30 || job.Result.Convergence.Manifest.FileCount != 2 || job.Result.Convergence.Manifest.DirectoryCount != 2 {
		t.Fatalf("convergence=%#v", job.Result.Convergence)
	}
	if len(job.Result.MediaItems) != 2 {
		t.Fatalf("media items=%#v, want 2", job.Result.MediaItems)
	}
	if deepRefresh.Load() {
		t.Fatal("deep branch was force-refreshed")
	}
	if parentRefreshed.Load() {
		t.Fatal("verified target parent was force-refreshed")
	}
	if packRefreshes.Load() != 1 {
		t.Fatalf("target refreshes=%d, want 1", packRefreshes.Load())
	}
}

func TestPipelineIngestStableTreeEmptyTargetNeedsAttentionWithoutRetry(t *testing.T) {
	var packRefreshes atomic.Int32
	upstream := newOpenListAPIServerWithRequests(t, func(req openListListTestRequest) ([]openListTestEntry, int) {
		switch req.Path {
		case "/115/movie":
			return []openListTestEntry{{Name: "Pack", IsDir: true}}, 1
		case "/115/movie/Pack":
			packRefreshes.Add(1)
			return nil, 0
		default:
			t.Fatalf("unexpected OpenList path %q", req.Path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	_, _ = storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "openlist-token"}})
	libPath := BuildCloudLibraryPath("openlist", "/115/movie", "/115/movie")
	lib := model.Library{Name: "Movie", Path: libPath, Type: "movie", Enabled: true}
	_ = repos.Library.Create(t.Context(), &lib)
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "Movie", Path: libPath, Enabled: true}
	_ = db.Create(&root).Error
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	svc := NewPipelineIngestService(log, repos, scanner, NewPipelineMaintenanceService(log, repos), nil)
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	svc.now = func() time.Time { return time.Unix(0, clock.Load()) }
	svc.wait = func(_ context.Context, duration time.Duration) error { clock.Add(int64(duration)); return nil }
	svc.maxConvergence = time.Minute

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "movie", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/movie"},
		Title:                     "Pack", TargetOpenListPaths: []string{"/115/movie/Pack"}, RequireTargetPath: true, Scan: true, RequireStableTree: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusNeedsAttention || job.Result.Convergence == nil || job.Result.Convergence.Status != PipelineIngestStatusNeedsAttention {
		t.Fatalf("job=%#v", job)
	}
	if job.Result.Media != nil || job.Error != "" {
		t.Fatalf("needs-attention job must not claim media or failure: %#v", job)
	}
	if packRefreshes.Load() != 1 || job.Result.Convergence.Attempt != 1 {
		t.Fatalf("target refreshes=%d convergence=%#v, want one attempt", packRefreshes.Load(), job.Result.Convergence)
	}
}

func TestPipelineIngestStableTreeCancelsActiveWalkAtDeadline(t *testing.T) {
	var activeWalkCanceled atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/list" {
			t.Fatalf("unexpected openlist api request %s", r.URL.Path)
		}
		var req openListListTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode openlist list request: %v", err)
		}
		if req.Path == "/115/movie/Pack" {
			<-r.Context().Done()
			activeWalkCanceled.Store(true)
			return
		}
		if req.Path != "/115/movie" {
			t.Fatalf("unexpected OpenList path %q", req.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200, "message": "success",
			"data": map[string]any{"content": []map[string]any{{"name": "Pack", "is_dir": true}}, "total": 1},
		})
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	_, _ = storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "openlist-token"}})
	libPath := BuildCloudLibraryPath("openlist", "/115/movie", "/115/movie")
	lib := model.Library{Name: "Movie", Path: libPath, Type: "movie", Enabled: true}
	_ = repos.Library.Create(t.Context(), &lib)
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "Movie", Path: libPath, Enabled: true}
	_ = db.Create(&root).Error
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	svc := NewPipelineIngestService(log, repos, scanner, NewPipelineMaintenanceService(log, repos), nil)
	svc.stableWindow = 10 * time.Millisecond
	svc.maxConvergence = 100 * time.Millisecond

	startedAt := time.Now()
	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "movie", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/movie"},
		Title:                     "Pack", TargetOpenListPaths: []string{"/115/movie/Pack"}, RequireTargetPath: true, Scan: true, RequireStableTree: true, TargetParentsVerified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if elapsed := time.Since(startedAt); elapsed > 2*time.Second {
		t.Fatalf("active walk exceeded convergence deadline: %s", elapsed)
	}
	if job.Status != PipelineIngestStatusNeedsAttention || job.Result.Convergence == nil || job.Result.Convergence.Status != PipelineIngestStatusNeedsAttention {
		t.Fatalf("job=%#v", job)
	}
	if !activeWalkCanceled.Load() {
		t.Fatal("active OpenList walk was not canceled at the convergence deadline")
	}
}

func TestPipelineIngestStableTreeInterruptedAttemptDoesNotRescanAfterRestart(t *testing.T) {
	db := newServiceTestDB(t, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	svc := NewPipelineIngestService(zap.NewNop(), repos, nil, nil, nil)
	startedAt := time.Now()
	job := PipelineIngestJob{
		ID:        "interrupted-strict-scan",
		Status:    PipelineIngestStatusRunning,
		Stage:     PipelineIngestStatusRunning,
		Message:   "running one strict target scan",
		Request:   PipelineIngestRequest{RequireStableTree: true},
		StartedAt: startedAt,
		UpdatedAt: startedAt,
		Result: PipelineIngestResult{Convergence: &PipelineIngestConvergenceResult{
			Status: PipelineIngestStatusRunning, Attempt: 1, StartedAt: startedAt, ObservedAt: startedAt,
		}},
	}
	if err := svc.storeJob(t.Context(), job); err != nil {
		t.Fatal(err)
	}

	_, _, runErr := svc.scanForPipelineIngestConverged(t.Context(), job.ID, pipelineResolvedTarget{}, job.Request, nil)
	var needsAttention *pipelineIngestNeedsAttentionError
	if !errors.As(runErr, &needsAttention) {
		t.Fatalf("error=%v, want needs_attention without touching the scanner", runErr)
	}
	got, err := svc.Get(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.Convergence == nil || got.Result.Convergence.Status != PipelineIngestStatusNeedsAttention || got.Result.Convergence.Attempt != 1 {
		t.Fatalf("convergence=%#v", got.Result.Convergence)
	}
}

func TestPipelineIngestStableTreeFirstListErrorCancelsWalkWithoutRetry(t *testing.T) {
	var badCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openListListTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		entries := []openListTestEntry{}
		switch req.Path {
		case "/115/movie":
			entries = []openListTestEntry{{Name: "Pack", IsDir: true}}
		case "/115/movie/Pack":
			entries = []openListTestEntry{{Name: "Bad", IsDir: true}}
		case "/115/movie/Pack/Bad":
			badCalls.Add(1)
			http.Error(w, "temporary list failure", http.StatusBadGateway)
			return
		default:
			t.Fatalf("unexpected OpenList path %q", req.Path)
		}
		content := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			content = append(content, map[string]any{"name": entry.Name, "size": entry.Size, "is_dir": entry.IsDir})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"content": content, "total": len(content)}})
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	_, _ = storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "openlist-token"}})
	libPath := BuildCloudLibraryPath("openlist", "/115/movie", "/115/movie")
	lib := model.Library{Name: "Movie", Path: libPath, Type: "movie", Enabled: true}
	_ = repos.Library.Create(t.Context(), &lib)
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "Movie", Path: libPath, Enabled: true}
	_ = db.Create(&root).Error
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	svc := NewPipelineIngestService(log, repos, scanner, NewPipelineMaintenanceService(log, repos), nil)
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	svc.now = func() time.Time { return time.Unix(0, clock.Load()) }
	svc.wait = func(_ context.Context, duration time.Duration) error { clock.Add(int64(duration)); return nil }

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "movie", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/movie"},
		Title:                     "Pack", TargetOpenListPaths: []string{"/115/movie/Pack"}, RequireTargetPath: true, Scan: true, RequireStableTree: true, TargetParentsVerified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusNeedsAttention || job.Result.Convergence == nil || job.Result.Convergence.Status != PipelineIngestStatusNeedsAttention {
		t.Fatalf("job=%#v", job)
	}
	if badCalls.Load() != 1 {
		t.Fatalf("bad_calls=%d, want one failed list with no retry", badCalls.Load())
	}
	if job.Result.Convergence.Attempt != 1 || job.Result.Convergence.ErrorCount != 1 || len(job.Result.MediaItems) != 0 {
		t.Fatalf("convergence=%#v media=%#v", job.Result.Convergence, job.Result.MediaItems)
	}
}

func TestPipelineIngestForcesTargetSeasonForAnimeAbsoluteEpisode(t *testing.T) {
	upstream := newOpenListAPIServerWithRequests(t, func(req openListListTestRequest) ([]openListTestEntry, int) {
		switch req.Path {
		case "/115/anime":
			return []openListTestEntry{{Name: "Swallowed Star", IsDir: true}}, 1
		case "/115/anime/Swallowed Star":
			return []openListTestEntry{{Name: "Swallowed.Star.S05E139.mkv", Size: 5000}}, 1
		default:
			t.Fatalf("unexpected openlist path %q", req.Path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "openlist-token"}}); err != nil {
		t.Fatal(err)
	}
	libPath := BuildCloudLibraryPath("openlist", "/115/anime", "/115/anime")
	lib := model.Library{Name: "Anime", Path: libPath, Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "Anime", Path: libPath, Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	const mediaRef = "/115/anime/Swallowed Star/Swallowed.Star.S05E139.mkv"
	mediaPath := cloudMediaPath("openlist", mediaRef)
	wrongSeason := model.Media{
		Base:          model.Base{ID: "wrong-season-episode"},
		LibraryID:     lib.ID,
		LibraryRootID: root.ID,
		Title:         "Swallowed Star",
		Path:          mediaPath,
		SizeBytes:     5000,
		Container:     "mkv",
		STRMURL:       BuildRelativeCloudPlayURL("openlist", mediaRef),
		ScrapeStatus:  "pending",
		SeasonNum:     5,
		EpisodeNum:    139,
	}
	if err := db.Create(&wrongSeason).Error; err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	maintenance := NewPipelineMaintenanceService(log, repos)
	svc := NewPipelineIngestService(log, repos, scanner, maintenance, nil)

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "anime", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/anime"},
		Title:                     "Swallowed Star",
		TargetOpenListPaths:       []string{"/115/anime/Swallowed Star"},
		RequireTargetPath:         true,
		Scan:                      true,
		ForceSeasonNumber:         1,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted {
		t.Fatalf("job status=%s error=%s", job.Status, job.Error)
	}
	if job.Result.Scan == nil || job.Result.Scan.Added != 0 || job.Result.Scan.Updated != 1 {
		t.Fatalf("scan result=%+v, want added=0 updated=1", job.Result.Scan)
	}
	var media model.Media
	if err := db.First(&media, "path = ?", mediaPath).Error; err != nil {
		t.Fatal(err)
	}
	if media.ID != wrongSeason.ID {
		t.Fatalf("media id=%s, want existing row %s", media.ID, wrongSeason.ID)
	}
	if media.SeasonNum != 1 || media.EpisodeNum != 139 {
		t.Fatalf("season/episode = %d/%d, want 1/139", media.SeasonNum, media.EpisodeNum)
	}
}

func TestPipelineIngestScansDirectoryNamedLikeVideoFile(t *testing.T) {
	requested := []openListListTestRequest{}
	upstream := newOpenListAPIServerWithRequests(t, func(req openListListTestRequest) ([]openListTestEntry, int) {
		requested = append(requested, req)
		switch req.Path {
		case "/115/movie":
			return []openListTestEntry{{Name: "Movie.mp4", IsDir: true}}, 1
		case "/115/movie/Movie.mp4":
			return []openListTestEntry{{Name: "Movie.mp4", Size: 5000}}, 1
		default:
			t.Fatalf("unexpected openlist path %q", req.Path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "openlist-token"}}); err != nil {
		t.Fatal(err)
	}
	libPath := BuildCloudLibraryPath("openlist", "/115/movie", "/115/movie")
	lib := model.Library{Name: "Movie", Path: libPath, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "Movie", Path: libPath, Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	maintenance := NewPipelineMaintenanceService(log, repos)
	svc := NewPipelineIngestService(log, repos, scanner, maintenance, nil)

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "movie", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/movie"},
		Title:                     "Movie",
		TargetOpenListPaths:       []string{"/115/movie/Movie.mp4"},
		RequireTargetPath:         true,
		Scan:                      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted {
		t.Fatalf("job status=%s error=%s", job.Status, job.Error)
	}
	if job.Result.Scan == nil || job.Result.Scan.Visited != 1 || job.Result.Scan.Added != 1 {
		t.Fatalf("scan result = %#v, want visited=1 added=1", job.Result.Scan)
	}
	if job.Result.Media == nil || job.Result.Media.MatchMode != "path" || job.Result.Media.Path != "cloud://openlist/115/movie/Movie.mp4/Movie.mp4" {
		t.Fatalf("unexpected media result: %#v", job.Result.Media)
	}
	if len(requested) != 2 || requested[0].Path != "/115/movie" || requested[1].Path != "/115/movie/Movie.mp4" {
		t.Fatalf("openlist paths requested = %#v, want parent then target only", requested)
	}
	if !requested[0].Refresh || !requested[1].Refresh {
		t.Fatalf("openlist refresh flags = %#v, want parent and target refreshed", requested)
	}
}

func TestPipelineIngestScansExactVideoFileAtLibraryRoot(t *testing.T) {
	requested := []openListListTestRequest{}
	upstream := newOpenListAPIServerWithRequests(t, func(req openListListTestRequest) ([]openListTestEntry, int) {
		requested = append(requested, req)
		if req.Path != "/115/movie" {
			t.Fatalf("unexpected openlist path %q", req.Path)
		}
		return []openListTestEntry{{Name: "Movie.mp4", Size: 5000}, {Name: "Other.mp4", Size: 6000}}, 2
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "openlist-token"}}); err != nil {
		t.Fatal(err)
	}
	libPath := BuildCloudLibraryPath("openlist", "/115/movie", "/115/movie")
	lib := model.Library{Name: "Movie", Path: libPath, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "Movie", Path: libPath, Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	maintenance := NewPipelineMaintenanceService(log, repos)
	svc := NewPipelineIngestService(log, repos, scanner, maintenance, nil)

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "movie", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/movie"},
		Title:                     "Movie",
		TargetOpenListPaths:       []string{"/115/movie/Movie.mp4"},
		RequireTargetPath:         true,
		Scan:                      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted {
		t.Fatalf("job status=%s error=%s", job.Status, job.Error)
	}
	if job.Result.Scan == nil || job.Result.Scan.Visited != 1 || job.Result.Scan.Added != 1 {
		t.Fatalf("scan result = %#v, want visited=1 added=1", job.Result.Scan)
	}
	if job.Result.Media == nil || job.Result.Media.MatchMode != "path" || job.Result.Media.Path != "cloud://openlist/115/movie/Movie.mp4" {
		t.Fatalf("unexpected media result: %#v", job.Result.Media)
	}
	if len(requested) != 2 || requested[0].Path != "/115/movie" || requested[1].Path != "/115/movie" {
		t.Fatalf("openlist paths requested = %#v, want parent only", requested)
	}
	if !requested[0].Refresh || requested[1].Refresh {
		t.Fatalf("openlist refresh flags = %#v, want refresh then cached exact-file list", requested)
	}
}

func TestPipelineIngestUsesExistingTargetWhenHistoricalCandidateIsMissing(t *testing.T) {
	upstream := newOpenListAPIServer(t, func(path string, _, _ int) ([]openListTestEntry, int) {
		switch path {
		case "/115/adult":
			return []openListTestEntry{{Name: "NEW-001", IsDir: true}}, 1
		case "/115/adult/NEW-001":
			return []openListTestEntry{{Name: "NEW-001.mp4", Size: 5000}}, 1
		default:
			t.Fatalf("unexpected openlist path %q", path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "openlist-token"}}); err != nil {
		t.Fatal(err)
	}
	libPath := BuildCloudLibraryPath("openlist", "/115/adult", "/115/adult")
	lib := model.Library{Name: "Adult", Path: libPath, Type: "adult", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "Adult", Path: libPath, Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	maintenance := NewPipelineMaintenanceService(log, repos)
	svc := NewPipelineIngestService(log, repos, scanner, maintenance, nil)

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "adult", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/adult"},
		Title:                     "NEW-001",
		TargetOpenListPaths:       []string{"/115/adult/NEW-001", "/115/adult/OLD-001"},
		RequireTargetPath:         true,
		Scan:                      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted {
		t.Fatalf("job status=%s error=%s", job.Status, job.Error)
	}
	if job.Result.Media == nil || job.Result.Media.Path != "cloud://openlist/115/adult/NEW-001/NEW-001.mp4" {
		t.Fatalf("unexpected media result: %#v", job.Result.Media)
	}
}

func TestPipelineIngestRejectsUnhandledTargetOpenListPaths(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	lib := model.Library{Name: "Local", Path: "/media/local", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "Local", Path: "/media/local", Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	maintenance := NewPipelineMaintenanceService(log, repos)
	svc := NewPipelineIngestService(log, repos, scanner, maintenance, nil)

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{
			Category:         "movie",
			LibraryID:        lib.ID,
			RootID:           root.ID,
			RootOpenListPath: "/115/movie",
		},
		Title:               "Movie",
		TargetOpenListPaths: []string{"/115/movie/Movie"},
		RequireTargetPath:   true,
		Scan:                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusFailed {
		t.Fatalf("job status=%s error=%s, want failed", job.Status, job.Error)
	}
	if !strings.Contains(job.Error, "target_openlist_paths") {
		t.Fatalf("job error = %q, want target_openlist_paths error", job.Error)
	}
	if job.Result.Scan != nil {
		t.Fatalf("scan result = %#v, want no fallback root scan", job.Result.Scan)
	}
}

func TestPipelineIngestRunsMovieExtrasRepair(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	maintenance := NewPipelineMaintenanceService(zap.NewNop(), repos)
	svc := NewPipelineIngestService(zap.NewNop(), repos, nil, maintenance, nil)

	lib, root := createPipelineMaintenanceRoot(t, db, "movie", "/115/movie")
	rows := []model.Media{
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Movie", Path: "cloud://openlist/115/movie/Movie/Movie.mkv", SizeBytes: 5000},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "PV", Path: "cloud://openlist/115/movie/Movie/Extras/PV.mp4", SizeBytes: 10},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{
			Category:         "movie",
			LibraryID:        lib.ID,
			RootID:           root.ID,
			RootOpenListPath: "/115/movie",
		},
		TargetOpenListPaths: []string{"/115/movie/Movie"},
		RequireTargetPath:   true,
		RepairMovieExtras:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted {
		t.Fatalf("job status=%s error=%s", job.Status, job.Error)
	}
	if job.Result.MovieExtras == nil || job.Result.MovieExtras.Updated != 1 || job.Result.MovieExtras.OpenListHidePath != "/115/movie/Movie" {
		t.Fatalf("unexpected extras result: %#v", job.Result.MovieExtras)
	}
}

func TestPipelineIngestMaterializesCloudSubtitles(t *testing.T) {
	fixture := newCloudSubtitleOpenListFixture(t, false)
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	lib, root := createPipelineMaintenanceRoot(t, db, "movie", "/115/movie")
	media := model.Media{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Movie", Path: "cloud://openlist/115/movie/Movie/Movie.mkv", SizeBytes: 5000}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": fixture.server.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	subtitle := NewSubtitleService(log, repos, storage)
	subtitle.SetMaterializedCacheDir(t.TempDir())
	svc := NewPipelineIngestService(log, repos, nil, NewPipelineMaintenanceService(log, repos), nil)
	svc.SetSubtitleService(subtitle)

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "movie", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/movie"},
		Title:                     "Movie", TargetOpenListPaths: []string{"/115/movie/Movie"}, RequireTargetPath: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted || job.Result.CloudSubtitleStatus == "failed" {
		t.Fatalf("core job status=%s error=%s subtitle_status=%s", job.Status, job.Error, job.Result.CloudSubtitleStatus)
	}
	job = waitPipelineIngestCloudSubtitle(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted || job.Result.CloudSubtitles == nil || job.Result.CloudSubtitles.Cached != 2 {
		t.Fatalf("job status=%s error=%s subtitles=%#v", job.Status, job.Error, job.Result.CloudSubtitles)
	}
}

func TestPipelineIngestCompletesWhenDeferredCloudSubtitleDownloadFails(t *testing.T) {
	fixture := newCloudSubtitleOpenListFixture(t, true)
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.StorageConfig{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	log := zap.NewNop()
	lib, root := createPipelineMaintenanceRoot(t, db, "movie", "/115/movie")
	media := model.Media{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Movie", Path: "cloud://openlist/115/movie/Movie/Movie.mkv", SizeBytes: 5000}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": fixture.server.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	subtitle := NewSubtitleService(log, repos, storage)
	subtitle.SetMaterializedCacheDir(t.TempDir())
	svc := NewPipelineIngestService(log, repos, nil, NewPipelineMaintenanceService(log, repos), nil)
	svc.SetSubtitleService(subtitle)

	job, err := svc.Start(t.Context(), PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "movie", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/movie"},
		Title:                     "Movie", TargetOpenListPaths: []string{"/115/movie/Movie"}, RequireTargetPath: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted || job.Error != "" {
		t.Fatalf("core job status=%s error=%s", job.Status, job.Error)
	}
	job = waitPipelineIngestCloudSubtitle(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted || job.Result.CloudSubtitleStatus != "failed" || job.Result.CloudSubtitles == nil || job.Result.CloudSubtitles.Status != "failed" {
		t.Fatalf("job status=%s error=%s subtitles=%#v", job.Status, job.Error, job.Result.CloudSubtitles)
	}
	if !strings.Contains(job.Result.CloudSubtitleError, "http 502") || !strings.Contains(job.Result.CloudSubtitles.Error, "http 502") {
		t.Fatalf("job error=%q subtitles=%#v", job.Error, job.Result.CloudSubtitles)
	}
}

func TestPipelineIngestPersistsAndReusesIdempotentJob(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	maintenance := NewPipelineMaintenanceService(zap.NewNop(), repos)
	svc := NewPipelineIngestService(zap.NewNop(), repos, nil, maintenance, nil)
	lib, root := createPipelineMaintenanceRoot(t, db, "adult", "/115/adult")
	media := model.Media{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "ABF-363", Path: "cloud://openlist/115/adult/ABF-363/ABF-363.mp4"}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	request := PipelineIngestRequest{
		PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "adult", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/adult"},
		IdempotencyKey:            "task:ABF-363",
		Title:                     "ABF-363",
		TargetOpenListPaths:       []string{"/115/adult/ABF-363"},
		RequireTargetPath:         true,
	}

	job, err := svc.Start(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	job = waitPipelineIngestJob(t, svc, job.ID)
	if job.Status != PipelineIngestStatusCompleted {
		t.Fatalf("job=%#v", job)
	}

	restarted := NewPipelineIngestService(zap.NewNop(), repos, nil, maintenance, nil)
	persisted, err := restarted.Get(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != PipelineIngestStatusCompleted || persisted.Result.Media == nil || persisted.Result.Media.ID != media.ID {
		t.Fatalf("persisted=%#v", persisted)
	}
	reused, err := restarted.Start(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != job.ID || reused.Status != PipelineIngestStatusCompleted {
		t.Fatalf("reused=%#v", reused)
	}
	conflicting := request
	conflicting.Title = "Different"
	if _, err := restarted.Start(t.Context(), conflicting); err == nil || !strings.Contains(err.Error(), "different request") {
		t.Fatalf("conflict err=%v", err)
	}
	var count int64
	if err := db.Model(&model.PipelineIngestJobRecord{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("job records=%d", count)
	}
}

func TestPipelineIngestRecoversPersistedRunningJob(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.PipelineIngestJobRecord{})
	repos := repository.New(db)
	maintenance := NewPipelineMaintenanceService(zap.NewNop(), repos)
	lib, root := createPipelineMaintenanceRoot(t, db, "adult", "/115/adult")
	media := model.Media{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "ABF-363", Path: "cloud://openlist/115/adult/ABF-363/ABF-363.mp4"}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	seed := NewPipelineIngestService(zap.NewNop(), repos, nil, maintenance, nil)
	seedJob := PipelineIngestJob{
		ID:      "recover-job",
		Status:  PipelineIngestStatusRunning,
		Stage:   "find_media",
		Message: "finding ingested media",
		Request: PipelineIngestRequest{
			PipelineMaintenanceTarget: PipelineMaintenanceTarget{Category: "adult", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/adult"},
			IdempotencyKey:            "task:recover-ABF-363",
			Title:                     "ABF-363",
			TargetOpenListPaths:       []string{"/115/adult/ABF-363"},
			RequireTargetPath:         true,
		},
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := seed.storeJob(t.Context(), seedJob); err != nil {
		t.Fatal(err)
	}

	restarted := NewPipelineIngestService(zap.NewNop(), repos, nil, maintenance, nil)
	count, err := restarted.Recover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("recovered=%d", count)
	}
	job := waitPipelineIngestJob(t, restarted, seedJob.ID)
	if job.Status != PipelineIngestStatusCompleted || job.Result.Media == nil || job.Result.Media.ID != media.ID {
		t.Fatalf("job=%#v", job)
	}
}

func waitPipelineIngestJob(t *testing.T, svc *PipelineIngestService, id string) PipelineIngestJob {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		job, err := svc.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if !pipelineIngestStatusActive(job.Status) {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("pipeline ingest job timed out: %#v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitPipelineIngestCloudSubtitle(t *testing.T, svc *PipelineIngestService, id string) PipelineIngestJob {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		job, err := svc.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Result.CloudSubtitleStatus != "pending" && job.Result.CloudSubtitleStatus != "running" {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("pipeline cloud subtitle enhancement timed out: %#v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
