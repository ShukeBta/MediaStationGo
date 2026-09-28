package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestScanAdmissionConflictsAcrossLibrariesAndQueuesEveryNewRoot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	svc := &service.Container{Repo: repos, Tasks: service.NewTaskTrackerService(zap.NewNop(), nil)}
	svc.Scan = service.NewScannerService(&config.Config{}, zap.NewNop(), repos, service.NewHub(zap.NewNop()), nil, nil)
	var libs []*model.Library
	for _, name := range []string{"first", "second"} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, name+".mkv"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		lib := &model.Library{Name: name, Path: root, Enabled: true, Type: "movie"}
		if err := repos.Library.CreateWithRoots(t.Context(), lib, []model.LibraryRoot{{Path: root, Enabled: true}}); err != nil {
			t.Fatal(err)
		}
		lib, err = repos.Library.FindByID(t.Context(), lib.ID)
		if err != nil {
			t.Fatal(err)
		}
		libs = append(libs, lib)
	}
	_, release, ok := svc.Scan.TryReserveLocalScan(t.Context())
	if !ok {
		t.Fatal("reserve")
	}
	defer release()
	r := gin.New()
	r.POST("/libraries/:id/scan", scanLibraryHandler(svc))
	r.POST("/libraries/:id/roots/:root_id/scan", scanLibraryRootHandler(svc))
	for _, path := range []string{"/libraries/" + libs[0].ID + "/scan", "/libraries/" + libs[1].ID + "/roots/" + libs[1].Roots[0].ID + "/scan"} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusConflict {
			t.Fatalf("busy scan accepted: %d %s", response.Code, response.Body.String())
		}
	}
	if len(svc.Tasks.Snapshot().Active) != 0 {
		t.Fatal("rejected requests created tasks")
	}
	for _, lib := range libs {
		queueLibraryRootScan(svc, lib.ID, lib.Roots[0].ID)
	}
	release()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(svc.Tasks.Snapshot().Recent) == 2 {
			var count int64
			db.Model(&model.Media{}).Count(&count)
			if count != 2 {
				t.Fatalf("queued roots dropped: %d", count)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("queued root scans did not complete")
}
