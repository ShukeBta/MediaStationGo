package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

func setTestUserLibraries(t *testing.T, svc *EmbyService, user *model.User, libraryIDs ...string) {
	t.Helper()
	user.AllowedLibraryIDs = append([]string(nil), libraryIDs...)
	if err := svc.repo.DB.Save(user).Error; err != nil {
		t.Fatalf("assign test user libraries: %v", err)
	}
}

func TestEmbyRootItemsExposeLibraries(t *testing.T) {
	svc := newTestEmbyService(t)
	for _, lib := range []model.Library{
		{Name: "电影", Path: `F:\downloads\电影`, Type: "movie", Enabled: true},
		{Name: "综艺", Path: `F:\downloads\综艺`, Type: "variety", Enabled: true},
	} {
		if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
			t.Fatalf("create library: %v", err)
		}
	}
	disabled := model.Library{Name: "停用库", Path: `F:\downloads\停用`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &disabled); err != nil {
		t.Fatalf("create disabled library: %v", err)
	}
	if err := svc.repo.Library.UpdateEnabled(t.Context(), disabled.ID, false); err != nil {
		t.Fatalf("disable library: %v", err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{Limit: 50})
	if err != nil {
		t.Fatalf("root items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("expected root items to expose only enabled libraries, got %#v", items)
	}
	if items[0]["Type"] != "CollectionFolder" || items[1]["Type"] != "CollectionFolder" {
		t.Fatalf("root should return collection folders: %#v", items)
	}
	if items[1]["CollectionType"] != "tvshows" {
		t.Fatalf("variety libraries should use tvshows collection type: %#v", items[1])
	}
}

func TestEmbyFolderItemQueryExposesLibrariesForHome(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{Base: model.Base{ID: "movie-1"}, LibraryID: lib.ID, Title: "不应出现在文件夹查询", Path: `/media/movies/a.mkv`}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		IncludeItemTypes: []string{"Folder", "CollectionFolder"},
		Limit:            50,
	})
	if err != nil {
		t.Fatalf("folder items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("expected one library folder, got %#v", items)
	}
	if items[0]["Type"] != "CollectionFolder" || items[0]["IsFolder"] != true {
		t.Fatalf("folder query should return collection folders, got %#v", items[0])
	}
}

func TestEmbyPlaybackInfoExposesExternalSubtitles(t *testing.T) {
	svc := newTestEmbyService(t)
	svc.SetSubtitleService(NewSubtitleService(zap.NewNop(), svc.repo))
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "Movie.mkv")
	if err := os.WriteFile(filepath.Join(dir, "Movie.zh.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\n你好\n"), 0o644); err != nil {
		t.Fatalf("write subtitle: %v", err)
	}
	lib := model.Library{Name: "电影", Path: dir, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{
		Base:       model.Base{ID: "media-sub"},
		LibraryID:  lib.ID,
		Title:      "Movie",
		Path:       mediaPath,
		Container:  "mkv",
		VideoCodec: "h264",
		AudioCodec: "aac",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	playback, err := svc.PlaybackInfo(t.Context(), "media-sub", "user-1")
	if err != nil {
		t.Fatalf("playback info: %v", err)
	}
	source := playback["MediaSources"].([]map[string]any)[0]
	streams := source["MediaStreams"].([]map[string]any)
	var subtitle map[string]any
	for _, stream := range streams {
		if stream["Type"] == "Subtitle" {
			subtitle = stream
			break
		}
	}
	if subtitle == nil {
		t.Fatalf("expected subtitle stream, got %#v", streams)
	}
	if subtitle["Codec"] != "srt" || subtitle["DeliveryUrl"] != "/emby/Videos/media-sub/media-sub/Subtitles/2/Stream.srt" {
		t.Fatalf("unexpected subtitle stream: %#v", subtitle)
	}
	if path, _ := subtitle["Path"].(string); path != "/subtitles/media-sub/Movie.zh.srt" {
		t.Fatalf("external subtitle Path should be a standard virtual file path: %#v", subtitle)
	}
	if subtitle["TimeBase"] != "1/1000" || subtitle["SupportsExternalStream"] != true {
		t.Fatalf("external subtitle should expose standard Emby fields: %#v", subtitle)
	}
	if subtitle["DisplayTitle"] != "zh - SRT - External" {
		t.Fatalf("unexpected subtitle display title: %#v", subtitle)
	}
	if source["DefaultSubtitleStreamIndex"] != subtitle["Index"] {
		t.Fatalf("DefaultSubtitleStreamIndex should point to first external subtitle: %#v", source)
	}

	item, err := svc.Item(t.Context(), "media-sub", "user-1")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	embeddedStreams := item["MediaSources"].([]map[string]any)[0]["MediaStreams"].([]map[string]any)
	var embeddedSubtitle map[string]any
	for _, stream := range embeddedStreams {
		if stream["Type"] == "Subtitle" {
			embeddedSubtitle = stream
			break
		}
	}
	if embeddedSubtitle == nil {
		t.Fatalf("item detail should expose external subtitles before playback: %#v", embeddedStreams)
	}
	if embeddedSubtitle["Codec"] != "srt" || embeddedSubtitle["DeliveryUrl"] != subtitle["DeliveryUrl"] {
		t.Fatalf("item detail subtitle should match playback info: item=%#v playback=%#v", embeddedSubtitle, subtitle)
	}
}

func TestEmbyExternalSubtitleNormalizesCachedChineseTrack(t *testing.T) {
	stream := embyExternalSubtitleStream("media-sub", 2, 0, SubtitleTrack{
		Lang:  "zh-Hans",
		Label: "简体中文",
		Path:  "local-subtitle://media-sub/subtitlecat-example.srt",
		Codec: "srt",
	})
	if stream["Language"] != "zh-CN" || stream["DisplayLanguage"] != "Chinese Simplified" {
		t.Fatalf("unexpected normalized language fields: %#v", stream)
	}
	if stream["Path"] != "/subtitles/media-sub/subtitlecat-example.srt" {
		t.Fatalf("unexpected virtual subtitle path: %#v", stream)
	}
	if delivery, _ := stream["DeliveryUrl"].(string); strings.Contains(delivery, "mp_track") {
		t.Fatalf("standard Emby subtitle URL must not require mp_track: %#v", stream)
	}
}

func TestEmbyUnsupportedItemTypesDoNotLeakAllMedia(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{Base: model.Base{ID: "movie-1"}, LibraryID: lib.ID, Title: "普通电影", Path: `/media/movies/a.mkv`}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	for _, includeType := range []string{"BoxSet", "Game", "Book", "Audio", "MusicAlbum", "Playlist", "TvChannel"} {
		out, err := svc.Items(t.Context(), ItemsParams{
			IncludeItemTypes: []string{includeType},
			Recursive:        true,
			Limit:            50,
		})
		if err != nil {
			t.Fatalf("%s items: %v", includeType, err)
		}
		if out["TotalRecordCount"] != int64(0) {
			t.Fatalf("%s should not return media rows, got %#v", includeType, out)
		}
		items := out["Items"].([]map[string]any)
		if len(items) != 0 {
			t.Fatalf("%s should return an empty list, got %#v", includeType, items)
		}
	}
}

func TestEmbyItemsFiltersFavorites(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	setTestUserLibraries(t, svc, viewer, lib.ID)
	favorite := model.Media{Base: model.Base{ID: "fav-1"}, LibraryID: lib.ID, Title: "收藏电影", Path: `/media/movies/fav.mkv`}
	normal := model.Media{Base: model.Base{ID: "normal-1"}, LibraryID: lib.ID, Title: "普通电影", Path: `/media/movies/normal.mkv`}
	if err := svc.repo.DB.Create(&favorite).Error; err != nil {
		t.Fatalf("create favorite media: %v", err)
	}
	if err := svc.repo.DB.Create(&normal).Error; err != nil {
		t.Fatalf("create normal media: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Favorite{UserID: viewer.ID, MediaID: favorite.ID}).Error; err != nil {
		t.Fatalf("create favorite: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		UserID:    viewer.ID,
		Filters:   []string{"IsFavorite"},
		Recursive: true,
		Limit:     50,
	})
	if err != nil {
		t.Fatalf("favorite items: %v", err)
	}
	if out["TotalRecordCount"] != int64(1) {
		t.Fatalf("expected one favorite, got %#v", out)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != favorite.ID {
		t.Fatalf("favorite filter returned wrong items: %#v", items)
	}
	userData := items[0]["UserData"].(map[string]any)
	if userData["IsFavorite"] != true {
		t.Fatalf("favorite payload should carry IsFavorite=true: %#v", userData)
	}
}

func TestEmbyItemsFiltersResumableForHome(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	setTestUserLibraries(t, svc, viewer, lib.ID)
	resumable := model.Media{Base: model.Base{ID: "resume-1"}, LibraryID: lib.ID, Title: "继续观看", Path: `/media/movies/resume.mkv`, DurationSec: 120}
	normal := model.Media{Base: model.Base{ID: "normal-1"}, LibraryID: lib.ID, Title: "普通电影", Path: `/media/movies/normal.mkv`, DurationSec: 120}
	if err := svc.repo.DB.Create(&resumable).Error; err != nil {
		t.Fatalf("create resumable media: %v", err)
	}
	if err := svc.repo.DB.Create(&normal).Error; err != nil {
		t.Fatalf("create normal media: %v", err)
	}
	if err := svc.repo.DB.Create(&model.PlaybackHistory{
		UserID:     viewer.ID,
		MediaID:    resumable.ID,
		PositionMs: 30_000,
		DurationMs: 120_000,
		WatchedAt:  time.Now(),
		Completed:  false,
	}).Error; err != nil {
		t.Fatalf("create playback history: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		UserID:     viewer.ID,
		Filters:    []string{"IsResumable"},
		Recursive:  true,
		SortBy:     "DatePlayed",
		SortOrder:  "Descending",
		Limit:      50,
		StartIndex: 0,
	})
	if err != nil {
		t.Fatalf("resumable items: %v", err)
	}
	if out["TotalRecordCount"] != int64(1) {
		t.Fatalf("expected one resumable item, got %#v", out)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != resumable.ID {
		t.Fatalf("resumable filter returned wrong items: %#v", items)
	}
}

func TestEmbyResumableItemsDefaultToDatePlayed(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	lib := model.Library{Name: "Movies", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	setTestUserLibraries(t, svc, viewer, lib.ID)
	oldWatchNewRelease := model.Media{
		Base:        model.Base{ID: "old-watch-new-release"},
		LibraryID:   lib.ID,
		Title:       "Newer Release",
		Path:        `/media/movies/newer-release.mkv`,
		DurationSec: 120,
		ReleaseDate: "2026-07-01",
	}
	newWatchOldRelease := model.Media{
		Base:        model.Base{ID: "new-watch-old-release"},
		LibraryID:   lib.ID,
		Title:       "Older Release",
		Path:        `/media/movies/older-release.mkv`,
		DurationSec: 120,
		ReleaseDate: "2025-01-01",
	}
	if err := svc.repo.DB.Create(&oldWatchNewRelease).Error; err != nil {
		t.Fatalf("create newer release media: %v", err)
	}
	if err := svc.repo.DB.Create(&newWatchOldRelease).Error; err != nil {
		t.Fatalf("create older release media: %v", err)
	}
	base := time.Now()
	for _, row := range []model.PlaybackHistory{
		{
			UserID:     viewer.ID,
			MediaID:    oldWatchNewRelease.ID,
			PositionMs: 30_000,
			DurationMs: 120_000,
			WatchedAt:  base.Add(-time.Hour),
			Completed:  false,
		},
		{
			UserID:     viewer.ID,
			MediaID:    newWatchOldRelease.ID,
			PositionMs: 30_000,
			DurationMs: 120_000,
			WatchedAt:  base,
			Completed:  false,
		},
	} {
		if err := svc.repo.DB.Create(&row).Error; err != nil {
			t.Fatalf("create playback history: %v", err)
		}
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		UserID:    viewer.ID,
		Filters:   []string{"IsResumable"},
		Recursive: true,
		Limit:     50,
	})
	if err != nil {
		t.Fatalf("resumable items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("resumable items len = %d, payload=%#v", len(items), out)
	}
	if items[0]["Id"] != newWatchOldRelease.ID {
		t.Fatalf("resumable items should default to DatePlayed order, got %#v", items)
	}
	userData := items[0]["UserData"].(map[string]any)
	if userData["LastPlayedDate"] == "" {
		t.Fatalf("resumable item should expose LastPlayedDate: %#v", items[0])
	}
}

func TestEmbyResumeItemsDefaultsToTenAndIncludesLastPlayedDate(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	lib := model.Library{Name: "Movies", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	setTestUserLibraries(t, svc, viewer, lib.ID)
	base := time.Now().UTC()
	for i := 0; i < 12; i++ {
		media := model.Media{
			Base:        model.Base{ID: fmt.Sprintf("resume-%02d", i)},
			LibraryID:   lib.ID,
			Title:       fmt.Sprintf("Resume %02d", i),
			Path:        fmt.Sprintf(`/media/movies/resume-%02d.mkv`, i),
			DurationSec: 120,
		}
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
		if err := svc.repo.DB.Create(&model.PlaybackHistory{
			UserID:     viewer.ID,
			MediaID:    media.ID,
			PositionMs: 30_000,
			DurationMs: 120_000,
			WatchedAt:  base.Add(time.Duration(i) * time.Minute),
			Completed:  false,
		}).Error; err != nil {
			t.Fatalf("create history: %v", err)
		}
	}

	out, err := svc.ResumeItems(t.Context(), viewer.ID)
	if err != nil {
		t.Fatalf("resume items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 10 {
		t.Fatalf("default resume item count = %d, want 10", len(items))
	}
	if out["TotalRecordCount"] != 10 {
		t.Fatalf("resume total = %#v, want fixed 10", out["TotalRecordCount"])
	}
	if out["StartIndex"] != 0 {
		t.Fatalf("resume start index = %#v, want 0", out["StartIndex"])
	}
	if items[0]["Id"] != "resume-11" {
		t.Fatalf("resume items should be newest first, got %#v", items[0])
	}
	userData := items[0]["UserData"].(map[string]any)
	if userData["LastPlayedDate"] == "" {
		t.Fatalf("resume item should expose LastPlayedDate: %#v", items[0])
	}
}

func TestEmbyResumeItemsSkipsInvalidHistoryBeforePaging(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	lib := model.Library{Name: "Movies", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	setTestUserLibraries(t, svc, viewer, lib.ID)
	base := time.Now().UTC()
	validRows := []model.Media{
		{Base: model.Base{ID: "valid-new"}, LibraryID: lib.ID, Title: "Valid New", Path: `/media/movies/valid-new.mkv`, DurationSec: 120},
		{Base: model.Base{ID: "valid-old"}, LibraryID: lib.ID, Title: "Valid Old", Path: `/media/movies/valid-old.mkv`, DurationSec: 120},
	}
	for i := range validRows {
		if err := svc.repo.DB.Create(&validRows[i]).Error; err != nil {
			t.Fatalf("create valid media: %v", err)
		}
	}
	deleted := model.Media{Base: model.Base{ID: "deleted-media"}, LibraryID: lib.ID, Title: "Deleted", Path: `/media/movies/deleted.mkv`, DurationSec: 120}
	if err := svc.repo.DB.Create(&deleted).Error; err != nil {
		t.Fatalf("create deleted media: %v", err)
	}
	if err := svc.repo.DB.Delete(&deleted).Error; err != nil {
		t.Fatalf("soft delete media: %v", err)
	}
	histories := []model.PlaybackHistory{
		{UserID: viewer.ID, MediaID: "missing-media", PositionMs: 50_000, DurationMs: 120_000, WatchedAt: base.Add(5 * time.Minute), Completed: false},
		{UserID: viewer.ID, MediaID: deleted.ID, PositionMs: 40_000, DurationMs: 120_000, WatchedAt: base.Add(4 * time.Minute), Completed: false},
		{UserID: viewer.ID, MediaID: "valid-new", PositionMs: 10_000, DurationMs: 120_000, WatchedAt: base.Add(3 * time.Minute), Completed: false},
		{UserID: viewer.ID, MediaID: "valid-new", PositionMs: 30_000, DurationMs: 120_000, WatchedAt: base.Add(2 * time.Minute), Completed: false},
		{UserID: viewer.ID, MediaID: "valid-old", PositionMs: 20_000, DurationMs: 120_000, WatchedAt: base.Add(time.Minute), Completed: false},
	}
	for i := range histories {
		if err := svc.repo.DB.Create(&histories[i]).Error; err != nil {
			t.Fatalf("create history: %v", err)
		}
	}

	out, err := svc.ResumeItems(t.Context(), viewer.ID)
	if err != nil {
		t.Fatalf("resume items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("resume item count = %d, want 2: %#v", len(items), items)
	}
	if out["TotalRecordCount"] != 2 {
		t.Fatalf("resume total = %#v, want 2", out["TotalRecordCount"])
	}
	if items[0]["Id"] != "valid-new" || items[1]["Id"] != "valid-old" {
		t.Fatalf("resume items should skip invalid history before paging and keep order, got %#v", items)
	}
	userData := items[0]["UserData"].(map[string]any)
	if got := userData["PlaybackPositionTicks"]; got != int64(10_000*10_000) {
		t.Fatalf("resume duplicate media should use newest history ticks = %#v", got)
	}

}

func TestEmbyUserPolicyDisablesDownloadsForViewers(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	admin := &model.User{Username: "admin", Role: "admin", Tier: "plus", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := svc.repo.User.Create(t.Context(), admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	viewerPayload, err := svc.FindUser(t.Context(), viewer.ID)
	if err != nil {
		t.Fatalf("viewer payload: %v", err)
	}
	adminPayload, err := svc.FindUser(t.Context(), admin.ID)
	if err != nil {
		t.Fatalf("admin payload: %v", err)
	}
	viewerPolicy := viewerPayload["Policy"].(map[string]any)
	adminPolicy := adminPayload["Policy"].(map[string]any)
	if viewerPolicy["EnableMediaPlayback"] != true {
		t.Fatalf("viewer must keep playback enabled: %#v", viewerPolicy)
	}
	if viewerPolicy["EnableContentDownloading"] != false ||
		viewerPolicy["EnableSyncTranscoding"] != false ||
		viewerPolicy["EnableMediaConversion"] != false {
		t.Fatalf("viewer must not be allowed to download/sync media: %#v", viewerPolicy)
	}
	if adminPolicy["EnableContentDownloading"] != true {
		t.Fatalf("admin should keep downloading capability: %#v", adminPolicy)
	}
}

func TestEmbyHidesAdultLibrariesForUserLock(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Username: "viewer", Role: "user", Tier: "free", IsActive: true, HideAdult: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	safe := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	adult := model.Library{Name: "9KG 成人", Path: `/media/9KG`, Type: "adult", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &safe); err != nil {
		t.Fatalf("create safe library: %v", err)
	}
	if err := svc.repo.Library.Create(t.Context(), &adult); err != nil {
		t.Fatalf("create adult library: %v", err)
	}
	setTestUserLibraries(t, svc, viewer, safe.ID, adult.ID)
	if err := svc.repo.DB.Create(&model.Media{LibraryID: safe.ID, Title: "安全电影", Path: `/media/movies/a.mkv`}).Error; err != nil {
		t.Fatalf("create safe media: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{LibraryID: adult.ID, Title: "成人电影", Path: `/media/9KG/a.mkv`, NSFW: true}).Error; err != nil {
		t.Fatalf("create adult media: %v", err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{UserID: viewer.ID, Limit: 50})
	if err != nil {
		t.Fatalf("root items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Name"] != "电影" {
		t.Fatalf("adult library should be hidden: %#v", items)
	}
	adultItems, err := svc.Items(t.Context(), ItemsParams{UserID: viewer.ID, ParentID: adult.ID, Limit: 50})
	if err != nil {
		t.Fatalf("adult items: %v", err)
	}
	if got := adultItems["TotalRecordCount"]; got != int64(0) {
		t.Fatalf("adult media should be hidden, total=%#v payload=%#v", got, adultItems)
	}
}

func TestEmbyHonorsAdministratorLibraryAccess(t *testing.T) {
	svc := newTestEmbyService(t)
	allowed := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	blocked := model.Library{Name: "私有库", Path: `/media/private`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &allowed); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.Library.Create(t.Context(), &blocked); err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{
		Username:          "limited-viewer",
		Role:              "user",
		Tier:              "free",
		IsActive:          true,
		AllowedLibraryIDs: []string{allowed.ID},
	}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.DB.Create(&[]model.Media{
		{LibraryID: allowed.ID, Title: "可见电影", Path: `/media/movies/a.mkv`},
		{LibraryID: blocked.ID, Title: "不可见电影", Path: `/media/private/b.mkv`},
	}).Error; err != nil {
		t.Fatal(err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{UserID: viewer.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != allowed.ID {
		t.Fatalf("root libraries = %#v, want only %s", items, allowed.ID)
	}
	blockedItems, err := svc.Items(t.Context(), ItemsParams{UserID: viewer.ID, ParentID: blocked.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if got := blockedItems["TotalRecordCount"]; got != int64(0) {
		t.Fatalf("blocked library media should be hidden, total=%#v", got)
	}
}

func TestEmbyPlaybackInfoRespectsDirectPlayOnly(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{Base: model.Base{ID: "m-1"}, LibraryID: lib.ID, Title: "Inception", Path: `/media/movies/inception.mkv`}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	pb, err := svc.PlaybackInfo(t.Context(), "m-1", "user-1")
	if err != nil {
		t.Fatalf("playback info: %v", err)
	}
	src := pb["MediaSources"].([]map[string]any)[0]
	if src["SupportsTranscoding"] != true {
		t.Fatalf("expected SupportsTranscoding=true by default, got %#v", src["SupportsTranscoding"])
	}
	if _, ok := src["TranscodingUrl"]; !ok {
		t.Fatalf("expected TranscodingUrl present by default: %#v", src)
	}
	if src["TranscodingUrl"] != "/Videos/m-1/master.m3u8" {
		t.Fatalf("expected HLS TranscodingUrl by default, got %#v", src["TranscodingUrl"])
	}

	if err := svc.repo.Setting.Set(t.Context(), PlaybackDirectOnlySettingKey, "true"); err != nil {
		t.Fatalf("enable direct-only: %v", err)
	}
	pb, err = svc.PlaybackInfo(t.Context(), "m-1", "user-1")
	if err != nil {
		t.Fatalf("playback info (direct-only): %v", err)
	}
	src = pb["MediaSources"].([]map[string]any)[0]
	if src["SupportsTranscoding"] != false {
		t.Fatalf("expected SupportsTranscoding=false in direct-only mode, got %#v", src["SupportsTranscoding"])
	}
	if _, ok := src["TranscodingUrl"]; ok {
		t.Fatalf("expected no TranscodingUrl in direct-only mode: %#v", src)
	}
	if src["SupportsDirectPlay"] != true || src["DirectStreamUrl"] != "/Videos/m-1/stream.mkv" {
		t.Fatalf("direct-only must still allow direct play: %#v", src)
	}
}

func TestEmbyPlaybackInfoUsesStandardVideoStreamInSTRMMode(t *testing.T) {
	svc := newTestEmbyService(t)
	if err := svc.repo.Setting.Set(t.Context(), CloudPlaybackModeSettingKey, CloudPlaybackModeSTRM); err != nil {
		t.Fatalf("set cloud playback mode: %v", err)
	}
	lib := model.Library{Name: "OpenList", Path: `cloud://openlist/Movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "cloud-1"},
		LibraryID: lib.ID,
		Title:     "Cloud Movie",
		Path:      `cloud://openlist/Movies/f1.mkv`,
		STRMURL:   `/api/cloud/play/openlist?ref=%2FMovies%2Ff1.mkv`,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	pb, err := svc.PlaybackInfo(t.Context(), "cloud-1", "user-1")
	if err != nil {
		t.Fatalf("playback info: %v", err)
	}
	src := pb["MediaSources"].([]map[string]any)[0]
	if src["IsRemote"] != true {
		t.Fatalf("strm media should be marked remote: %#v", src)
	}
	if src["DirectStreamUrl"] != "/Videos/cloud-1/stream.mkv" {
		t.Fatalf("strm playback should use the standard Emby stream URL: %#v", src)
	}
	if src["Path"] != "/Videos/cloud-1/stream.mkv" {
		t.Fatalf("path should use the standard Emby stream URL: %#v", src)
	}
	streams := src["MediaStreams"].([]map[string]any)
	if len(streams) == 0 || streams[0]["Type"] != "Video" {
		t.Fatalf("strm media should expose a fallback video stream for Android clients: %#v", src)
	}
}

func TestEmbyPlaybackInfoUsesVideoStreamWhenSTRMDisabled(t *testing.T) {
	svc := newTestEmbyService(t)
	if err := svc.repo.Setting.Set(t.Context(), CloudPlaybackModeSettingKey, CloudPlaybackModeRedirectProxy); err != nil {
		t.Fatalf("set cloud playback mode: %v", err)
	}
	lib := model.Library{Name: "OpenList", Path: `cloud://openlist/Movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "cloud-302"},
		LibraryID: lib.ID,
		Title:     "Cloud 302 Movie",
		Path:      `cloud://openlist/Movies/Movie.mkv`,
		STRMURL:   `/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv`,
		Container: "mkv",
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	pb, err := svc.PlaybackInfo(t.Context(), "cloud-302", "user-1")
	if err != nil {
		t.Fatalf("playback info: %v", err)
	}
	src := pb["MediaSources"].([]map[string]any)[0]
	if src["DirectStreamUrl"] != "/Videos/cloud-302/stream.mkv" {
		t.Fatalf("302/proxy mode should use Emby video stream URL: %#v", src)
	}
	if src["Path"] != "/Videos/cloud-302/stream.mkv" {
		t.Fatalf("302/proxy mode path should use Emby video stream URL: %#v", src)
	}
}

func TestEmbyPlaybackInfoDoesNotProbeMissingCloudTrackMetadata(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "OpenList", Path: `cloud://openlist/Movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "cloud-probe-1"},
		LibraryID: lib.ID,
		Title:     "云盘电影",
		Path:      `cloud://openlist/Movies/Movie.mkv`,
		STRMURL:   `http://nas.local/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv`,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	pb, err := svc.PlaybackInfo(t.Context(), "cloud-probe-1", "user-1")
	if err != nil {
		t.Fatalf("playback info: %v", err)
	}
	var persisted model.Media
	if err := svc.repo.DB.First(&persisted, "id = ?", "cloud-probe-1").Error; err != nil {
		t.Fatalf("reload media: %v", err)
	}
	if persisted.MediaProbeVersion != 0 || persisted.DurationSec != 0 || persisted.VideoCodec != "" || persisted.AudioCodec != "" {
		t.Fatalf("PlaybackInfo must not mutate missing track metadata: %#v", persisted)
	}
	src := pb["MediaSources"].([]map[string]any)[0]
	if src["RunTimeTicks"] != int64(0) {
		t.Fatalf("PlaybackInfo should expose current persisted runtime without probing: %#v", src)
	}
	streams := src["MediaStreams"].([]map[string]any)
	if len(streams) != 1 || streams[0]["Codec"] != "unknown" {
		t.Fatalf("PlaybackInfo should keep the existing unknown stream placeholder: %#v", streams)
	}
}
