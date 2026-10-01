package service

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
)

func TestQBitDestinationOverridesAutomaticManagementOnlyWhenConfigured(t *testing.T) {
	for _, clientKind := range []string{"adapter", "legacy"} {
		for _, savePath := range []string{"/downloads/custom", ""} {
			t.Run(clientKind+"/"+savePath, func(t *testing.T) {
				var posted atomic.Bool
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/auth/login":
						_, _ = w.Write([]byte("Ok."))
					case "/api/v2/torrents/add":
						posted.Store(true)
						if err := r.ParseMultipartForm(1 << 20); err != nil {
							t.Error(err)
							http.Error(w, "invalid form", http.StatusBadRequest)
							return
						}
						defer r.MultipartForm.RemoveAll()
						if got := r.FormValue("savepath"); got != savePath {
							t.Errorf("qB savepath = %q, want %q", got, savePath)
						}
						if savePath == "" {
							if _, exists := r.MultipartForm.Value["autoTMM"]; exists {
								t.Error("an unspecified destination must preserve qB's default management policy")
							}
						} else if got := r.FormValue("autoTMM"); got != "false" {
							t.Errorf("qB autoTMM = %q; configured destination must disable automatic movement", got)
						}
						_, _ = w.Write([]byte("Ok."))
					default:
						http.NotFound(w, r)
					}
				}))
				t.Cleanup(server.Close)
				payload := []byte("d4:infod4:name5:movieee")
				var err error
				if clientKind == "legacy" {
					client := NewQBitClient(zap.NewNop(), QBitConfig{BaseURL: server.URL})
					err = client.AddTorrentFileWithCategory(t.Context(), payload, "movie.torrent", savePath, "Movies")
				} else {
					client := NewQBitAdapter()
					if err = client.Initialize(t.Context(), DownloadClientConfig{Host: server.URL}); err != nil {
						t.Fatal(err)
					}
					_, err = client.AddTorrentFileWithCategory(t.Context(), payload, "movie.torrent", savePath, "Movies")
				}
				if err != nil {
					t.Fatal(err)
				}
				if !posted.Load() {
					t.Fatal("torrent was not sent to qB")
				}
			})
		}
	}
}
