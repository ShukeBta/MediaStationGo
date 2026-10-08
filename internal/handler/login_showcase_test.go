package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func TestLoginShowcasePublicMetadataAndArtwork(t *testing.T) {
	cfg, svc, _ := currentRoleTestServices(t)
	if err := svc.Repo.DB.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	art := filepath.Join(dir, "private-artwork.png")
	var picture bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&picture, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(art, picture.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.Cache.CacheDir = filepath.Join(dir, "cache")
	svc.ImageProxy = service.NewImageProxy(cfg, svc.Log)
	svc.ImageProxy.SetLibraryRootsProvider(func() []string { return []string{dir} })
	for _, id := range []string{"enabled", "disabled", "deleted"} {
		if err := svc.Repo.DB.Create(&model.Library{Base: model.Base{ID: id}, Name: id, Path: dir, Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Repo.DB.Model(&model.Library{}).Where("id = ?", "disabled").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.DB.Delete(&model.Library{}, "id = ?", "deleted").Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "eligible"}, LibraryID: "enabled", Title: "Visible movie", Overview: "A movie synopsis", Year: 2025, PosterURL: art, BackdropURL: filepath.Join(dir, "scene-must-not-be-used.png"), STRMURL: "https://secret:password@example.com/movie"},
		{Base: model.Base{ID: "adult"}, LibraryID: "enabled", Title: "Adult", NSFW: true, PosterURL: art},
		{Base: model.Base{ID: "disabled"}, LibraryID: "disabled", Title: "Disabled", PosterURL: art},
		{Base: model.Base{ID: "deleted-library"}, LibraryID: "deleted", Title: "Deleted library", PosterURL: art},
		{Base: model.Base{ID: "deleted-media"}, LibraryID: "enabled", Title: "Deleted media", PosterURL: art},
		{Base: model.Base{ID: "no-art"}, LibraryID: "enabled", Title: "No artwork"},
		{Base: model.Base{ID: "backdrop-only"}, LibraryID: "enabled", Title: "Only a scene", BackdropURL: art},
		{Base: model.Base{ID: "generated-backdrop-only"}, LibraryID: "enabled", Title: "Only a generated scene", PosterURL: "  ", GeneratedBackdropURL: art},
		{Base: model.Base{ID: "duplicate"}, LibraryID: "enabled", Title: "Duplicate", IsDuplicate: true, PosterURL: art},
	}
	for i := range rows {
		rows[i].Path = filepath.Join(dir, rows[i].ID+".mkv")
		if err := svc.Repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Repo.DB.Delete(&model.Media{}, "id = ?", "deleted-media").Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	registerPublicAuthRoutes(router.Group("/api"), svc, svc.Log)
	request := func(path string, status int) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatalf("%s: status=%d body=%s", path, w.Code, w.Body.String())
		}
		return w
	}
	w := request("/api/auth/showcase", 200)
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("unexpected items: %s", w.Body.String())
	}
	item := payload.Items[0]
	if len(item) != 4 || item["title"] != "Visible movie" || item["overview"] != "A movie synopsis" || item["year"] != float64(2025) {
		t.Fatalf("unexpected DTO: %#v", item)
	}
	if bytes.Contains(w.Body.Bytes(), []byte("private-artwork")) || bytes.Contains(w.Body.Bytes(), []byte("secret")) {
		t.Fatal("response leaked private source metadata")
	}
	w = request(item["artwork_url"].(string)+"?url=https://attacker.invalid/other&refresh=1&width=99999", 200)
	if !bytes.Equal(w.Body.Bytes(), picture.Bytes()) {
		t.Fatal("did not serve persisted local artwork")
	}
	for _, id := range []string{"adult", "disabled", "deleted-library", "deleted-media", "no-art", "backdrop-only", "generated-backdrop-only", "duplicate", "missing"} {
		request("/api/auth/showcase/artwork/"+id, 404)
	}
	for i := 0; i < 12; i++ {
		row := model.Media{LibraryID: "enabled", Title: fmt.Sprintf("Movie %d", i), Path: filepath.Join(dir, fmt.Sprintf("%d.mkv", i)), PosterURL: "  ", GeneratedPosterURL: art}
		if err := svc.Repo.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	w = request("/api/auth/showcase?limit=1000", 200)
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 8 {
		t.Fatalf("selection must be bounded at eight: %s", w.Body.String())
	}
	if err := svc.Repo.DB.Model(&model.Library{}).Where("id = ?", "enabled").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	request("/api/auth/showcase/artwork/eligible", 404)
	w = request("/api/auth/showcase", 200)
	if w.Body.String() != `{"items":[]}` {
		t.Fatalf("empty result: %s", w.Body.String())
	}
}
