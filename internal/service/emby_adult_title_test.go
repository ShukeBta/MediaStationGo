package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyAdultDisplayNameNormalizesAdultCodeTitle(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Adult", Path: `/media/adult`, Type: "adult", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "mide-949"},
		LibraryID: lib.ID,
		Title:     "mide 949",
		Path:      `/media/adult/MIDE-949/mide 949.mp4`,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	item, err := svc.Item(t.Context(), media.ID, "user-1")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if item["Name"] != "MIDE-949" {
		t.Fatalf("Name = %#v, want MIDE-949", item["Name"])
	}
}

func TestEmbyAdultDisplayNameStripsCHNoiseSuffix(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Adult", Path: `/media/adult`, Type: "adult", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "ssis-218"},
		LibraryID: lib.ID,
		Title:     "SSIS-218CH bonus",
		Path:      `/media/adult/SSIS-218/main.mp4`,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	item, err := svc.Item(t.Context(), media.ID, "user-1")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if item["Name"] != "SSIS-218 bonus" {
		t.Fatalf("Name = %#v, want SSIS-218 bonus", item["Name"])
	}
}

func TestEmbyAdultDisplayNamePrefixesNSFWItemOutsideAdultLibrary(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Movies", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "fc2-926114"},
		LibraryID: lib.ID,
		Title:     "downloaded title",
		Path:      `/media/movies/FC2PPV926114/main.mp4`,
		NSFW:      true,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	item, err := svc.Item(t.Context(), media.ID, "user-1")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if item["Name"] != "FC2-PPV-926114 downloaded title" {
		t.Fatalf("Name = %#v, want FC2-PPV-926114 downloaded title", item["Name"])
	}
}

func TestEmbyAdultDisplayNameDoesNotPrefixNormalMovie(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Movies", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "bdmv-001"},
		LibraryID: lib.ID,
		Title:     "BDMV-001",
		Path:      `/media/movies/BDMV-001/main.mkv`,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	item, err := svc.Item(t.Context(), media.ID, "user-1")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if item["Name"] != "BDMV-001" {
		t.Fatalf("Name = %#v, want unchanged title", item["Name"])
	}
}
