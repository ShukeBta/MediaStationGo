package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestAddDownloadWithMetaAutoClassifiesSavePathAndQBitCategory(t *testing.T) {
	var addCalls int32
	var gotSavePath string
	var gotCategory string
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"auto123","name":"声生不息 S01E01","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			gotSavePath = r.FormValue("savepath")
			gotCategory = r.FormValue("category")
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.savepath", "/downloads"); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee&dn=%E5%A3%B0%E7%94%9F%E4%B8%8D%E6%81%AF+S01E01", "", DownloadTaskMeta{
		Title:          "声生不息 S01E01",
		SourceCategory: "综艺",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join("/downloads", "综艺")
	if task.SavePath != wantPath {
		t.Fatalf("task save path = %q, want %q", task.SavePath, wantPath)
	}
	if gotSavePath != wantPath {
		t.Fatalf("qb savepath = %q, want %q", gotSavePath, wantPath)
	}
	if gotCategory != "综艺" {
		t.Fatalf("qb category = %q, want 综艺", gotCategory)
	}
}

func TestDownloadSavePathCategoryRootKeepsWindowsClientSeparators(t *testing.T) {
	if got := downloadSavePathCategoryRoot(`F:\downloads`, "国产剧"); got != `F:\downloads\国产剧` {
		t.Fatalf("downloadSavePathCategoryRoot() = %q, want Windows qB path", got)
	}
	if got := downloadSavePathCategoryRoot(`F:\downloads\国产剧`, "国产剧"); got != `F:\downloads\国产剧` {
		t.Fatalf("downloadSavePathCategoryRoot() duplicated category: %q", got)
	}
	if got := downloadSavePathCategoryRoot(`/downloads`, "国产剧"); got != filepath.Join(`/downloads`, "国产剧") {
		t.Fatalf("downloadSavePathCategoryRoot() = %q, want local path", got)
	}
}

func TestTranslateClientPathMapsWindowsQBitPathToContainerDownloadPath(t *testing.T) {
	root := t.TempDir()
	containerDownloads := filepath.Join(root, "downloads")
	want := filepath.Join(containerDownloads, "国产剧", "Show.S01E01.mkv")
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := translateClientPath(`F:\downloads\国产剧\Show.S01E01.mkv`, map[string]string{
		`F:\downloads`: containerDownloads,
	})
	if got != want {
		t.Fatalf("translateClientPath() = %q, want %q", got, want)
	}
}

func TestAddDownloadWithMetaCanDisableAutoClassifiedSavePath(t *testing.T) {
	var addCalls int32
	var gotSavePath string
	var gotCategory string
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"auto456","name":"声生不息 S01E01","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			gotSavePath = r.FormValue("savepath")
			gotCategory = r.FormValue("category")
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.savepath", "/downloads"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), DownloadSmartClassifySettingKey, "false"); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff&dn=%E5%A3%B0%E7%94%9F%E4%B8%8D%E6%81%AF+S01E01", "", DownloadTaskMeta{
		Title:          "声生不息 S01E01",
		SourceCategory: "综艺",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.SavePath != "/downloads" {
		t.Fatalf("task save path = %q, want /downloads", task.SavePath)
	}
	if gotSavePath != "/downloads" {
		t.Fatalf("qb savepath = %q, want /downloads", gotSavePath)
	}
	if gotCategory != "" {
		t.Fatalf("qb category = %q, want empty", gotCategory)
	}
}
