package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// 托管下载器存在时 ReloadConfig 会清空 d.qb;"先暂停加入、选文件后开始"
// 流程必须改用默认托管 qBittorrent 的凭据,而不是报"下载器未配置"。
func TestConfirmPreparedDownloadUsesManagedDefaultQBit(t *testing.T) {
	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var (
		mu      sync.Mutex
		resumed []string
		badAuth bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_ = r.ParseForm()
			if r.PostForm.Get("username") != "admin" || r.PostForm.Get("password") != "secret-pass" {
				mu.Lock()
				badAuth = true
				mu.Unlock()
				_, _ = w.Write([]byte("Fails."))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sid-test", Path: "/"})
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/resume", "/api/v2/torrents/start":
			_ = r.ParseForm()
			mu.Lock()
			resumed = append(resumed, r.PostForm.Get("hashes"))
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			_, _ = w.Write([]byte("[]"))
		}
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	client := &model.DownloadClient{Name: "qB", Type: "qbittorrent", Host: server.URL, Username: "admin", Password: "secret-pass", IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.SetDownloadManager(NewDownloadManager(zap.NewNop(), repos, nil))

	task, err := svc.ConfirmPreparedDownload(t.Context(), "user-1", strings.ToUpper(hash),
		"magnet:?xt=urn:btih:"+hash+"&dn=Movie+2026", "/downloads/movies", DownloadTaskMeta{Title: "Movie 2026"}, nil)
	if err != nil {
		t.Fatalf("confirm prepared download: %v", err)
	}
	if svc.qb.IsConfigured() {
		t.Fatal("legacy d.qb should stay blank while managed clients exist")
	}
	if task.Source != "qbittorrent" || task.DownloadClientID != client.ID || task.ExternalID != hash {
		t.Fatalf("task downloader identity = source %q client %q external %q", task.Source, task.DownloadClientID, task.ExternalID)
	}
	mu.Lock()
	defer mu.Unlock()
	if badAuth {
		t.Fatal("managed qB credentials were not passed through")
	}
	if len(resumed) != 1 || !strings.EqualFold(resumed[0], hash) {
		t.Fatalf("resumed = %#v, want the prepared hash", resumed)
	}
}

func TestPreparedDownloadRejectsNonQBitDefaultClearly(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	client := &model.DownloadClient{Name: "TR", Type: "transmission", Host: "http://127.0.0.1:1", IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.SetDownloadManager(NewDownloadManager(zap.NewNop(), repos, nil))

	err := svc.CancelPreparedDownload(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil || !strings.Contains(err.Error(), "仅支持 qBittorrent") || !strings.Contains(err.Error(), "transmission") {
		t.Fatalf("err = %v, want a clear qBittorrent-only error", err)
	}
}
