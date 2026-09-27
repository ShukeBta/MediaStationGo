package service

import (
	"errors"
	"fmt"
	"testing"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestMediaVersionOwnerCanDeleteOnlyOwnedVersion(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Media{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	user := model.User{Username: "owner", PasswordHash: "x", Role: "user", IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	owned := model.Media{LibraryID: "library-1", Title: "Sintel", TMDbID: 123, Path: "cloud://openlist/Movies/Sintel.1080p.mkv"}
	other := model.Media{LibraryID: "library-1", Title: "Sintel", TMDbID: 123, Path: "cloud://openlist/Movies/Sintel.2160p.mkv"}
	if err := repos.Media.Upsert(t.Context(), &owned); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &other); err != nil {
		t.Fatal(err)
	}
	job := model.ResourceImportJob{
		UserID: user.ID, LibraryID: owned.LibraryID, SearchSessionID: "session",
		CandidateJSON: "{}", CandidateTitle: "Sintel", IdempotencyKey: "owned-version",
		Status: ResourceImportStatusCompleted, Stage: "completed", MediaID: owned.ID, Attempt: 1,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	versions, err := svc.ListMediaVersions(t.Context(), other.ID, user.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions.Items) != 2 || !versions.CanManageVersions {
		t.Fatalf("versions = %+v", versions)
	}
	permissions := map[string]bool{}
	for _, item := range versions.Items {
		permissions[item.ID] = item.CanManage
	}
	if !permissions[owned.ID] || permissions[other.ID] {
		t.Fatalf("version permissions = %#v", permissions)
	}
	if _, err := svc.DeleteMediaVersion(t.Context(), other.ID, other.ID, user.ID, false); !errors.Is(err, ErrMediaVersionForbidden) {
		t.Fatalf("delete unowned version err = %v", err)
	}
	result, err := svc.DeleteMediaVersion(t.Context(), other.ID, owned.ID, user.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedID != owned.ID || result.NextMediaID != other.ID {
		t.Fatalf("delete result = %+v", result)
	}
	var deleted model.Media
	if err := db.Unscoped().First(&deleted, "id = ?", owned.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !deleted.DeletedAt.Valid || deleted.DeletionKind != "version" || deleted.DeletedByUserID != user.ID {
		t.Fatalf("deleted version = %+v", deleted)
	}
}

func TestAdminCanManageEveryMediaVersion(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	first := model.Media{LibraryID: "library-1", Title: "Sintel", TMDbID: 123, Path: "cloud://openlist/Movies/Sintel.A.mkv"}
	second := model.Media{LibraryID: "library-1", Title: "Sintel", TMDbID: 123, Path: "cloud://openlist/Movies/Sintel.B.mkv"}
	if err := repos.Media.Upsert(t.Context(), &first); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &second); err != nil {
		t.Fatal(err)
	}
	versions, err := svc.ListMediaVersions(t.Context(), first.ID, "admin", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range versions.Items {
		if !item.CanManage {
			t.Fatalf("admin cannot manage version %+v", item)
		}
	}
}

func TestListMediaVersionsDoesNotMergeDomainPrefixedTitles(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	rows := []model.Media{
		{
			LibraryID: "other-library",
			Title:     "mtcang.com v",
			Path:      "cloud://openlist/115/其他/作品二十/mtcang.com v.mp4",
		},
		{
			LibraryID: "other-library",
			Title:     "mtcang.com 跳蛋",
			Path:      "cloud://openlist/115/其他/作品十/mtcang.com 跳蛋.mp4",
		},
		{
			LibraryID: "other-library",
			Title:     "mtcang.com spa",
			Path:      "cloud://openlist/115/其他/作品十一/mtcang.com spa.mp4",
		},
	}
	for i := range rows {
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}

	versions, err := svc.ListMediaVersions(t.Context(), rows[0].ID, "admin", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions.Items) != 1 || versions.Items[0].ID != rows[0].ID {
		t.Fatalf("domain-prefixed titles were merged as versions: %#v", versions.Items)
	}
}

func TestListMediaVersionsDoesNotLoadUnrelatedLibraryRows(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	versions := []model.Media{
		{Base: model.Base{ID: "target-1080"}, LibraryID: "large-library", Title: "Sintel", TMDbID: 123, Path: "/media/Sintel.1080p.mkv"},
		{Base: model.Base{ID: "target-2160"}, LibraryID: "large-library", Title: "Sintel", TMDbID: 123, Path: "/media/Sintel.2160p.mkv"},
	}
	for i := range versions {
		if err := repos.Media.Upsert(t.Context(), &versions[i]); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := make([]model.Media, 200)
	for i := range unrelated {
		unrelated[i] = model.Media{
			LibraryID: "large-library",
			Title:     "Unrelated",
			Path:      fmt.Sprintf("/media/unrelated/%03d.mkv", i),
		}
	}
	if err := db.Create(&unrelated).Error; err != nil {
		t.Fatal(err)
	}

	var maxMediaRows int64
	const callbackName = "test:list_media_versions_row_count"
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "media" && tx.RowsAffected > maxMediaRows {
			maxMediaRows = tx.RowsAffected
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	result, err := svc.ListMediaVersions(t.Context(), versions[0].ID, "admin", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("versions = %#v", result.Items)
	}
	if maxMediaRows > int64(len(versions)) {
		t.Fatalf("a media query loaded %d rows for a %d-row version group", maxMediaRows, len(versions))
	}
}

func TestListMediaVersionsKeepsMultipartItemsSeparate(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	parts := []model.Media{
		{Base: model.Base{ID: "part-1"}, LibraryID: "movies", Title: "Work Part 1", Path: "/media/work/part-1.mkv", PartGroupKey: "work", PartIndex: 1},
		{Base: model.Base{ID: "part-2"}, LibraryID: "movies", Title: "Work Part 2", Path: "/media/work/part-2.mkv", PartGroupKey: "work", PartIndex: 2},
	}
	for i := range parts {
		if err := repos.Media.Upsert(t.Context(), &parts[i]); err != nil {
			t.Fatal(err)
		}
	}

	result, err := svc.ListMediaVersions(t.Context(), parts[0].ID, "admin", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != parts[0].ID {
		t.Fatalf("multipart siblings leaked into versions: %#v", result.Items)
	}
}

func TestListMediaVersionsRejectsStaleProjectionWithoutFullScan(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	media := model.Media{Base: model.Base{ID: "stale"}, LibraryID: "movies", Title: "Sintel", TMDbID: 123, Path: "/media/Sintel.mkv"}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Media{}).Where("id = ?", media.ID).UpdateColumns(map[string]any{
		"media_version_key":         "",
		"media_version_key_version": 0,
	}).Error; err != nil {
		t.Fatal(err)
	}

	_, err := svc.ListMediaVersions(t.Context(), media.ID, "admin", true)
	if !errors.Is(err, ErrMediaVersionKeyStale) {
		t.Fatalf("stale projection error = %v", err)
	}
}
