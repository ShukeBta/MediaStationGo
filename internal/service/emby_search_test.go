package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbySearchTermMatchesCaseInsensitiveTitleAndPath(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Adult", Path: `/media/adult`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "mide-949"}, LibraryID: lib.ID, Title: "MIDE-949", Path: `/media/adult/MIDE-949/MIDE-949.mp4`},
		{Base: model.Base{ID: "fc2"}, LibraryID: lib.ID, Title: "Unknown", Path: `/media/adult/FC2-PPV-926114/video.mp4`},
		{Base: model.Base{ID: "other"}, LibraryID: lib.ID, Title: "ABF-363", Path: `/media/adult/ABF-363/ABF-363.mp4`},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, SearchTerm: "mide-949", Limit: 50})
	if err != nil {
		t.Fatalf("items by title: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "mide-949" {
		t.Fatalf("SearchTerm should match title ignoring case, got %#v", items)
	}

	out, err = svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, SearchTerm: "fc2-ppv-926114", Limit: 50})
	if err != nil {
		t.Fatalf("items by path: %v", err)
	}
	items = out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "fc2" {
		t.Fatalf("SearchTerm should match path ignoring case, got %#v", items)
	}
}

func TestEmbyNameStartsWithMatchesTitlePrefixOnly(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Adult", Path: `/media/adult`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "mide-949"}, LibraryID: lib.ID, Title: "MIDE-949", Path: `/media/adult/MIDE-949/MIDE-949.mp4`},
		{Base: model.Base{ID: "amide"}, LibraryID: lib.ID, Title: "AMIDE", Path: `/media/adult/AMIDE/AMIDE.mp4`},
		{Base: model.Base{ID: "path-only"}, LibraryID: lib.ID, Title: "Unknown", Path: `/media/adult/MID-PATH/video.mp4`},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, NameStartsWith: "mid", Limit: 50})
	if err != nil {
		t.Fatalf("items by prefix: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "mide-949" {
		t.Fatalf("NameStartsWith should match title prefix ignoring case, got %#v", items)
	}

	out, err = svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, NameStartsWith: "ide", Limit: 50})
	if err != nil {
		t.Fatalf("items by non-prefix: %v", err)
	}
	items = out["Items"].([]map[string]any)
	if len(items) != 0 {
		t.Fatalf("NameStartsWith should not behave like contains search, got %#v", items)
	}
}
