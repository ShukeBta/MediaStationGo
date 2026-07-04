package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestAddDownloadWithMetaSkipsExistingLocalMovieBeforeQBAdd(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := db.Create(&model.Media{
		Title: "Inception",
		Path:  "/media/movies/Inception (2010)/Inception (2010).mkv",
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&dn=Inception+2010+1080p", "/downloads", DownloadTaskMeta{
		Title: "Inception 2010 1080p WEB-DL",
	})
	if !errors.Is(err, ErrMediaAlreadyInLibrary) {
		t.Fatalf("err = %v, want ErrMediaAlreadyInLibrary", err)
	}
	if task != nil {
		t.Fatalf("task = %#v, want nil because local media already exists", task)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("download rows = %d, want 0", len(rows))
	}
}

func TestAddDownloadWithMetaSkipsExistingLocalEpisodeBeforeQBAdd(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := db.Create(&model.Media{
		Title:      "Some Show",
		Path:       "/media/tv/Some Show/Season 01/Some Show - S01E01.mkv",
		SeasonNum:  1,
		EpisodeNum: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd&dn=Some+Show+S01E01", "/downloads", DownloadTaskMeta{
		Title: "Some Show S01E01 2160p WEB-DL",
	})
	if !errors.Is(err, ErrMediaAlreadyInLibrary) {
		t.Fatalf("err = %v, want ErrMediaAlreadyInLibrary", err)
	}
	if task != nil {
		t.Fatalf("task = %#v, want nil because local episode already exists", task)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("download rows = %d, want 0", len(rows))
	}
}

func TestAddDownloadWithMetaQueuesEpisodeRangeWhenOnlyPartlyInLibrary(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			http.Error(w, "temporary list unavailable", http.StatusInternalServerError)
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Media{}, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	if err := db.Create(&model.Media{
		Title:      "Archives The Nanyang Mystery",
		Path:       "/media/tv/Archives The Nanyang Mystery/Season 01/Archives The Nanyang Mystery - S01E07.mkv",
		SeasonNum:  1,
		EpisodeNum: 7,
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:abababababababababababababababababababab&dn=Archives+The+Nanyang+Mystery+2026+S01E07-S01E08", "/downloads", DownloadTaskMeta{
		Title: "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
	})
	if err != nil {
		t.Fatalf("AddDownloadWithMeta returned %v, want queued because E08 is missing", err)
	}
	if task == nil {
		t.Fatal("task = nil, want queued task")
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
}

func TestAddDownloadWithMetaSkipsEpisodeRangeWhenFullyInLibrary(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	for _, episode := range []int{7, 8} {
		if err := db.Create(&model.Media{
			Title:      "Archives The Nanyang Mystery",
			Path:       filepath.Join("/media/tv/Archives The Nanyang Mystery/Season 01", fmt.Sprintf("Archives The Nanyang Mystery - S01E%02d.mkv", episode)),
			SeasonNum:  1,
			EpisodeNum: episode,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:babababababababababababababababababababa&dn=Archives+The+Nanyang+Mystery+2026+S01E07-S01E08", "/downloads", DownloadTaskMeta{
		Title: "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
	})
	if !errors.Is(err, ErrMediaAlreadyInLibrary) {
		t.Fatalf("err = %v, want ErrMediaAlreadyInLibrary", err)
	}
	if task != nil {
		t.Fatalf("task = %#v, want nil", task)
	}
}

func TestAddDownloadWithMetaQueuesExplicitEpisodeWhenOnlySeriesPackInLibrary(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			http.Error(w, "temporary list unavailable", http.StatusInternalServerError)
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Media{}, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	if err := db.Create(&model.Media{
		Title: "Archives The Nanyang Mystery S01 Complete",
		Path:  "/media/tv/Archives The Nanyang Mystery/Season 01/Archives The Nanyang Mystery S01 Complete.mkv",
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:fefefefefefefefefefefefefefefefefefefefe&dn=Archives+The+Nanyang+Mystery+2026+S01E29-E33", "/downloads/tv", DownloadTaskMeta{
		SubscriptionID: "sub-nanyang",
		Title:          "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL",
	})
	if err != nil {
		t.Fatalf("AddDownloadWithMeta returned %v, want queued because explicit missing episodes are not proven by a pack row", err)
	}
	if task == nil {
		t.Fatal("task = nil, want queued task")
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
}

func TestAddDownloadWithMetaSkipsExistingLocalEpisodeWithReleaseGroup(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.DownloadTask{}, &model.Setting{}, &model.DownloadClient{})
	repos := repository.New(db)
	if err := db.Create(&model.Media{
		Title:      "凡人修仙传",
		Path:       "/media/动漫/国漫/凡人修仙传/Season 01/凡人修仙传 - S01E146.mkv",
		SeasonNum:  1,
		EpisodeNum: 146,
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee&dn=%5BMagicStar%5D+%E5%87%A1%E4%BA%BA%E4%BF%AE%E4%BB%99%E4%BC%A0+%E5%B9%B4%E7%95%AA+-+146+%5B1080p%5D", "/downloads", DownloadTaskMeta{
		Title: "[MagicStar] 凡人修仙传 年番 - 146 [1080p][WEB-DL]",
	})
	if !errors.Is(err, ErrMediaAlreadyInLibrary) {
		t.Fatalf("err = %v, want ErrMediaAlreadyInLibrary", err)
	}
	if task != nil {
		t.Fatalf("task = %#v, want nil", task)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("download rows = %d, want 0", len(rows))
	}
}
