package service

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestSchedulerCloudUploadUsesConfiguredLocalSource(t *testing.T) {
	var uploaded []string
	alist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/mkdir":
			_, _ = w.Write([]byte(`{"code":200}`))
		case "/api/fs/get":
			w.WriteHeader(http.StatusNotFound)
		case "/api/fs/put":
			decoded, _ := url.PathUnescape(r.Header.Get("File-Path"))
			uploaded = append(uploaded, decoded)
			_, _ = w.Write([]byte(`{"code":200}`))
		default:
			t.Fatalf("unexpected alist path %s", r.URL.Path)
		}
	}))
	defer alist.Close()

	repos, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "alist",
		Config: map[string]any{
			"server":           alist.URL,
			"token":            "token",
			"transfer_enabled": "true",
		},
	}); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "Show.S01E01.mkv"), []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		CloudUploadAutoEnabledKey: "true",
		CloudUploadProviderKey:    "alist",
		CloudUploadSourceDirKey:   source,
		CloudUploadDestPathKey:    "/cloud-media",
		CloudUploadRecursiveKey:   "true",
		CloudUploadSidecarsKey:    "false",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	scheduler := NewSchedulerService(zap.NewNop(), repos, nil, nil, nil, storage, NewHub(zap.NewNop()), "")
	if err := scheduler.jobUploadLocalToCloud(t.Context()); err != nil {
		t.Fatalf("cloud upload job: %v", err)
	}
	if len(uploaded) != 1 || uploaded[0] != "/cloud-media/Show.S01E01.mkv" {
		t.Fatalf("uploaded = %#v", uploaded)
	}
}

func TestStorageConfigUploadLocalToCloudDrive2(t *testing.T) {
	var uploaded []string
	dav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut:
			uploaded = append(uploaded, r.URL.Path)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected method %s %s", r.Method, r.URL.Path)
		}
	}))
	defer dav.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "clouddrive2",
		Config: map[string]any{
			"url":              dav.URL + "/dav",
			"username":         "user",
			"password":         "pass",
			"transfer_enabled": "true",
		},
	}); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "Movie.mkv"), []byte("movie"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := storage.UploadLocal(t.Context(), CloudUploadInput{
		Type:       "clouddrive2",
		SourcePath: source,
		DestPath:   "/MediaStationGo",
		Recursive:  true,
	})
	if err != nil {
		t.Fatalf("upload local: %v", err)
	}
	if res.Uploaded != 1 || len(uploaded) != 1 || uploaded[0] != "/dav/MediaStationGo/Movie.mkv" {
		t.Fatalf("result = %+v uploaded=%#v", res, uploaded)
	}
}

func TestStorageConfigUploadLocalRequiresTransferEnabled(t *testing.T) {
	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "alist",
		Config: map[string]any{
			"server": "http://alist.test",
			"token":  "token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "Movie.mkv"), []byte("movie"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := storage.UploadLocal(t.Context(), CloudUploadInput{
		Type:       "alist",
		SourcePath: source,
		DestPath:   "/MediaStationGo",
		Recursive:  true,
	})
	if err == nil || !strings.Contains(err.Error(), "transfer is disabled") {
		t.Fatalf("upload error = %v, want transfer disabled", err)
	}
}

func TestStorageConfigUploadLocalMoveDeletesSourceAfterUpload(t *testing.T) {
	var uploaded []string
	alist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/mkdir":
			_, _ = w.Write([]byte(`{"code":200}`))
		case "/api/fs/get":
			w.WriteHeader(http.StatusNotFound)
		case "/api/fs/put":
			decoded, _ := url.PathUnescape(r.Header.Get("File-Path"))
			uploaded = append(uploaded, decoded)
			_, _ = w.Write([]byte(`{"code":200}`))
		default:
			t.Fatalf("unexpected alist path %s", r.URL.Path)
		}
	}))
	defer alist.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "alist",
		Config: map[string]any{
			"server":           alist.URL,
			"token":            "token",
			"transfer_enabled": "true",
			"transfer_mode":    "move",
		},
	}); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	file := filepath.Join(source, "Movie.mkv")
	if err := os.WriteFile(file, []byte("movie"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := storage.UploadLocal(t.Context(), CloudUploadInput{
		Type:       "alist",
		SourcePath: source,
		DestPath:   "/MediaStationGo",
		Recursive:  true,
	})
	if err != nil {
		t.Fatalf("upload local move: %v", err)
	}
	if res.Uploaded != 1 || res.Moved != 1 || len(uploaded) != 1 {
		t.Fatalf("result = %+v uploaded=%#v", res, uploaded)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("source should be removed after move upload, stat err=%v", err)
	}
}
