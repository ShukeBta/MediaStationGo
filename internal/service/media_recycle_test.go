package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type fakeCloudMediaDeleter struct {
	provider      string
	ref           string
	err           error
	pruneProvider string
	pruneRef      string
	pruneRoot     string
	pruneErr      error
}

func (f *fakeCloudMediaDeleter) DeleteCloudFile(_ context.Context, provider, ref string) error {
	f.provider = provider
	f.ref = ref
	return f.err
}

func (f *fakeCloudMediaDeleter) PruneEmptyCloudParents(_ context.Context, provider, ref, rootRef string) error {
	f.pruneProvider = provider
	f.pruneRef = ref
	f.pruneRoot = rootRef
	return f.pruneErr
}

func TestListRecycleBinPrunesOldRowsOverLimit(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	now := time.Now()
	for i := 0; i < maxRecycleBinRecords+5; i++ {
		deletedAt := now.Add(time.Duration(i) * time.Second)
		media := model.Media{
			Base: model.Base{
				ID:        fmt.Sprintf("media-%03d", i),
				DeletedAt: gorm.DeletedAt{Time: deletedAt, Valid: true},
			},
			Title: fmt.Sprintf("Movie %03d", i),
			Path:  filepath.Join(t.TempDir(), fmt.Sprintf("Movie %03d.mkv", i)),
		}
		if err := db.Unscoped().Create(&media).Error; err != nil {
			t.Fatal(err)
		}
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	rows, err := svc.ListRecycleBin(t.Context(), 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != maxRecycleBinRecords {
		t.Fatalf("recycle rows = %d, want %d", len(rows), maxRecycleBinRecords)
	}
	var count int64
	if err := db.Unscoped().Model(&model.Media{}).Where("deleted_at IS NOT NULL").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != maxRecycleBinRecords {
		t.Fatalf("stored recycle rows = %d, want %d", count, maxRecycleBinRecords)
	}
	var oldCount int64
	if err := db.Unscoped().Model(&model.Media{}).Where("id IN ?", []string{"media-000", "media-001", "media-002", "media-003", "media-004"}).Count(&oldCount).Error; err != nil {
		t.Fatal(err)
	}
	if oldCount != 0 {
		t.Fatalf("oldest recycle rows were not pruned, count=%d", oldCount)
	}
}

func TestListRecycleBinKeepsCloudTombstonesWhenPruning(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	oldCloud := model.Media{
		Base: model.Base{
			ID:        "cloud-hidden",
			DeletedAt: gorm.DeletedAt{Time: time.Now().Add(-24 * time.Hour), Valid: true},
		},
		Title: "Hidden Cloud",
		Path:  "cloud://openlist/Movies/Hidden.mkv",
	}
	if err := db.Unscoped().Create(&oldCloud).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < maxRecycleBinRecords+5; i++ {
		deletedAt := now.Add(time.Duration(i) * time.Second)
		media := model.Media{
			Base: model.Base{
				ID:        fmt.Sprintf("local-%03d", i),
				DeletedAt: gorm.DeletedAt{Time: deletedAt, Valid: true},
			},
			Title: fmt.Sprintf("Local %03d", i),
			Path:  filepath.Join(t.TempDir(), fmt.Sprintf("Local %03d.mkv", i)),
		}
		if err := db.Unscoped().Create(&media).Error; err != nil {
			t.Fatal(err)
		}
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	if _, err := svc.ListRecycleBin(t.Context(), 500); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Unscoped().Model(&model.Media{}).Where("id = ?", oldCloud.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("cloud tombstone should not be pruned from recycle storage")
	}
}

func TestSoftDeleteInvalidatesMediaAndStatsCache(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	media := model.Media{
		Base:  model.Base{ID: "local-media"},
		Title: "Cached Movie",
		Path:  filepath.Join(t.TempDir(), "Cached Movie.mkv"),
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	cache := NewRuntimeCacheService(&config.Config{}, zap.NewNop())
	cache.SetJSON(t.Context(), "media:list:test", map[string]string{"state": "stale"}, time.Minute)
	cache.SetJSON(t.Context(), "stats:snapshot:base", map[string]int{"media": 1}, time.Minute)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos).SetRuntimeCache(cache)
	if err := svc.SoftDelete(t.Context(), media.ID); err != nil {
		t.Fatal(err)
	}

	var mediaCache map[string]string
	if cache.GetJSON(t.Context(), "media:list:test", &mediaCache) {
		t.Fatal("soft delete should invalidate media cache")
	}
	var statsCache map[string]int
	if cache.GetJSON(t.Context(), "stats:snapshot:base", &statsCache) {
		t.Fatal("soft delete should invalidate stats cache")
	}
}

func TestSoftDeleteManyByMovesAllRowsInOneBatch(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	for _, id := range []string{"episode-1", "episode-2"} {
		media := model.Media{Base: model.Base{ID: id}, Title: id, Path: filepath.Join(t.TempDir(), id+".mkv")}
		if err := repos.DB.Create(&media).Error; err != nil {
			t.Fatal(err)
		}
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	applied, err := svc.SoftDeleteManyBy(t.Context(), []string{" episode-1 ", "episode-2", "episode-2"}, "user-1", "media")
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("applied = %d, want 2", applied)
	}
	var activeCount int64
	if err := db.Model(&model.Media{}).Count(&activeCount).Error; err != nil {
		t.Fatal(err)
	}
	if activeCount != 0 {
		t.Fatalf("active media rows = %d, want 0", activeCount)
	}
	var deletedCount int64
	if err := db.Unscoped().Model(&model.Media{}).Where("deleted_at IS NOT NULL AND deletion_kind = ?", "media").Count(&deletedCount).Error; err != nil {
		t.Fatal(err)
	}
	if deletedCount != 2 {
		t.Fatalf("deleted media rows = %d, want 2", deletedCount)
	}
}

func TestPurgeDeletedCloudMediaDeletesProviderFileBeforeDatabaseRow(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	media := model.Media{
		Base:  model.Base{ID: "cloud-media", DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}},
		Title: "Sintel", Path: "cloud://openlist/115/电影/Sintel/Sintel.mkv",
	}
	if err := db.Unscoped().Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	deleter := &fakeCloudMediaDeleter{}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos).SetCloudMediaDeleter(deleter)
	if err := svc.PurgeDeleted(t.Context(), media.ID); err != nil {
		t.Fatal(err)
	}
	if deleter.provider != "openlist" || deleter.ref != "115/电影/Sintel/Sintel.mkv" {
		t.Fatalf("cloud delete = provider %q ref %q", deleter.provider, deleter.ref)
	}
	var count int64
	if err := db.Unscoped().Model(&model.Media{}).Where("id = ?", media.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("database row should be purged after cloud delete succeeds")
	}
}

func TestPurgeDeletedCloudMediaPrunesEmptyParentsWithinLibraryRoot(t *testing.T) {
	db := newServiceTestDB(t, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	root := model.LibraryRoot{Base: model.Base{ID: "root-1"}, LibraryID: "library-1", Path: "cloud://openlist/115/剧集"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		Base:          model.Base{ID: "cloud-media", DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}},
		LibraryID:     root.LibraryID,
		LibraryRootID: root.ID,
		Title:         "My Series",
		Path:          "cloud://openlist/115/剧集/My Series/Season 1/My Series.S01E01.mkv",
	}
	if err := db.Unscoped().Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	deleter := &fakeCloudMediaDeleter{}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos).SetCloudMediaDeleter(deleter)
	if err := svc.PurgeDeleted(t.Context(), media.ID); err != nil {
		t.Fatal(err)
	}
	if deleter.pruneProvider != "openlist" || deleter.pruneRef != "115/剧集/My Series/Season 1/My Series.S01E01.mkv" || deleter.pruneRoot != "115/剧集" {
		t.Fatalf("prune request = provider %q ref %q root %q", deleter.pruneProvider, deleter.pruneRef, deleter.pruneRoot)
	}
}

func TestPurgeDeletedCloudMediaContinuesWhenParentPruneFails(t *testing.T) {
	db := newServiceTestDB(t, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	root := model.LibraryRoot{Base: model.Base{ID: "root-1"}, LibraryID: "library-1", Path: "cloud://openlist/115/剧集"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		Base:          model.Base{ID: "cloud-media", DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}},
		LibraryID:     root.LibraryID,
		LibraryRootID: root.ID,
		Title:         "My Series",
		Path:          "cloud://openlist/115/剧集/My Series/Season 1/My Series.S01E01.mkv",
	}
	if err := db.Unscoped().Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	deleter := &fakeCloudMediaDeleter{pruneErr: errors.New("password is incorrect or you have no permission")}
	core, logs := observer.New(zap.WarnLevel)
	svc := NewMediaService(&config.Config{}, zap.New(core), repos).SetCloudMediaDeleter(deleter)
	if err := svc.PurgeDeleted(t.Context(), media.ID); err != nil {
		t.Fatalf("purge should continue after parent prune failure: %v", err)
	}
	if logs.Len() != 1 || logs.All()[0].Message != "purge deleted media: prune empty cloud parents failed; continuing" {
		t.Fatalf("parent prune warning logs = %+v", logs.All())
	}
	var count int64
	if err := db.Unscoped().Model(&model.Media{}).Where("id = ?", media.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("database row should be purged after provider delete succeeds")
	}
}

func TestPurgeDeletedCloudMediaKeepsTombstoneWhenProviderDeleteFails(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	media := model.Media{
		Base:  model.Base{ID: "cloud-media", DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}},
		Title: "Sintel", Path: "cloud://openlist/115/电影/Sintel/Sintel.mkv",
	}
	if err := db.Unscoped().Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	deleter := &fakeCloudMediaDeleter{err: errors.New("upstream rejected delete")}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos).SetCloudMediaDeleter(deleter)
	if err := svc.PurgeDeleted(t.Context(), media.ID); err == nil {
		t.Fatal("purge should fail when cloud delete fails")
	}
	var count int64
	if err := db.Unscoped().Model(&model.Media{}).Where("id = ? AND deleted_at IS NOT NULL", media.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("cloud tombstone must remain when provider delete fails")
	}
}

func TestRecycleBinForUserOnlyListsOwnedImports(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Media{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	user := model.User{Username: "owner", PasswordHash: "x", Role: "user", IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	owned := model.Media{Base: model.Base{DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}}, Title: "Owned", Path: "cloud://openlist/Movies/Owned.mkv", DeletionKind: "version"}
	other := model.Media{Base: model.Base{DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}}, Title: "Other", Path: "cloud://openlist/Movies/Other.mkv", DeletionKind: "version"}
	if err := db.Unscoped().Create(&owned).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Unscoped().Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	job := model.ResourceImportJob{
		UserID: user.ID, SearchSessionID: "session", CandidateJSON: "{}", CandidateTitle: "Owned",
		IdempotencyKey: "owned-recycle", Status: ResourceImportStatusCompleted, Stage: "completed", MediaID: owned.ID, Attempt: 1,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	items, err := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		ListRecycleBinForUser(t.Context(), user.ID, false, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != owned.ID || items[0].DeletionKind != "version" || !items[0].CanManage {
		t.Fatalf("user recycle items = %+v", items)
	}
}

func TestCloudMediaFileTargetRejectsDirectoryLikePath(t *testing.T) {
	_, _, isCloud, err := cloudMediaFileTarget("cloud://openlist/115/电影/Sintel")
	if !isCloud || err == nil {
		t.Fatalf("directory-like cloud path should be rejected: is_cloud=%v err=%v", isCloud, err)
	}
}
