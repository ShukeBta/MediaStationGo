package service

import (
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

func seedResumeRowFixtures(t *testing.T, svc *EmbyService) *model.User {
	t.Helper()
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	setTestUserLibraries(t, svc, viewer, lib.ID)
	base := time.Now().UTC()
	for i, id := range []string{"resume-a", "resume-b", "resume-c"} {
		media := model.Media{
			Base:        model.Base{ID: id},
			LibraryID:   lib.ID,
			Title:       id,
			Path:        `/media/movies/` + id + `.mkv`,
			DurationSec: 1200,
		}
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
		if err := svc.repo.DB.Create(&model.PlaybackHistory{
			UserID:     viewer.ID,
			MediaID:    id,
			PositionMs: int64(100_000 + i),
			DurationMs: 1_200_000,
			WatchedAt:  base.Add(time.Duration(i) * time.Minute),
			Completed:  false,
		}).Error; err != nil {
			t.Fatalf("create history: %v", err)
		}
	}
	return viewer
}

func TestEmbyHideFromResumeExcludesItemFromResumeAndResumableFilter(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := seedResumeRowFixtures(t, svc)

	out, err := svc.ResumeItems(t.Context(), viewer.ID)
	if err != nil {
		t.Fatalf("resume items: %v", err)
	}
	if out["TotalRecordCount"] != 3 {
		t.Fatalf("resume total = %#v, want 3", out["TotalRecordCount"])
	}

	if err := svc.SetHiddenFromResume(t.Context(), viewer.ID, "resume-b"); err != nil {
		t.Fatalf("hide from resume: %v", err)
	}

	out, err = svc.ResumeItems(t.Context(), viewer.ID)
	if err != nil {
		t.Fatalf("resume items after hide: %v", err)
	}
	if out["TotalRecordCount"] != 2 {
		t.Fatalf("resume total after hide = %#v, want 2", out["TotalRecordCount"])
	}
	for _, item := range out["Items"].([]map[string]any) {
		if item["Id"] == "resume-b" {
			t.Fatalf("hidden item must not appear in resume list: %#v", item["Id"])
		}
	}

	resumable, err := svc.Items(t.Context(), ItemsParams{
		UserID:  viewer.ID,
		Filters: []string{"IsResumable"},
		Limit:   50,
	})
	if err != nil {
		t.Fatalf("resumable items: %v", err)
	}
	for _, item := range resumable["Items"].([]map[string]any) {
		if item["Id"] == "resume-b" {
			t.Fatalf("hidden item must not appear in IsResumable query: %#v", item["Id"])
		}
	}

	// 重复隐藏与已有音轨偏好共存：先建立 SubtitleEnabled=false 的偏好行，
	// 隐藏后不得覆盖它。
	pref := &model.UserMediaPlaybackPreference{
		UserID:          viewer.ID,
		MediaID:         "resume-a",
		SubtitleEnabled: false,
	}
	if err := svc.repo.DB.Create(pref).Error; err != nil {
		t.Fatalf("create preference: %v", err)
	}
	if err := svc.SetHiddenFromResume(t.Context(), viewer.ID, "resume-a"); err != nil {
		t.Fatalf("hide again: %v", err)
	}
	stored, err := svc.repo.MediaPlaybackPreference.FindByUserAndMedia(t.Context(), viewer.ID, "resume-a")
	if err != nil || stored == nil {
		t.Fatalf("load preference: %v %#v", err, stored)
	}
	if !stored.HiddenFromResume {
		t.Fatalf("hidden flag not persisted: %#v", stored)
	}
	if stored.SubtitleEnabled {
		t.Fatalf("hide must not touch subtitle preference: %#v", stored)
	}
}

func TestEmbyEpisodeRowPayloadExposesThumbStill(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "剧集", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	episode := model.Media{
		Base:        model.Base{ID: "ep-thumb"},
		LibraryID:   lib.ID,
		Title:       "遮天",
		Path:        `/media/tv/遮天/Season 1/遮天 - S01E01.mkv`,
		PosterURL:   `https://image.example/series-poster.jpg`,
		BackdropURL: `https://image.example/episode-still.jpg`,
		SeasonNum:   1,
		EpisodeNum:  151,
	}
	if err := svc.repo.DB.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}

	item := svc.itemPayload(t.Context(), &episode, false, 0)
	tags, ok := item["ImageTags"].(map[string]string)
	if !ok {
		t.Fatalf("ImageTags type: %#v", item["ImageTags"])
	}
	wantThumb := embyImageTag(episode.ID, "thumb", episode.BackdropURL, episode.UpdatedAt)
	if tags["Thumb"] != wantThumb {
		t.Fatalf("episode Thumb tag = %#v, want still thumb %q", tags["Thumb"], wantThumb)
	}
	wantPrimary := embyImageTag(episode.ID, "primary", episode.BackdropURL, episode.UpdatedAt)
	if tags["Primary"] != wantPrimary {
		t.Fatalf("episode Primary must stay the still: %#v", tags["Primary"])
	}
	if backdropTags, ok := item["BackdropImageTags"].([]string); !ok || len(backdropTags) != 0 {
		t.Fatalf("episode Backdrop contract must stay empty: %#v", item["BackdropImageTags"])
	}
	thumbURL, err := svc.ImageURL(t.Context(), episode.ID, "Thumb")
	if err != nil {
		t.Fatalf("thumb image url: %v", err)
	}
	if thumbURL != episode.BackdropURL {
		t.Fatalf("episode Thumb image = %q, want still %q", thumbURL, episode.BackdropURL)
	}

	bare := model.Media{
		Base:       model.Base{ID: "ep-no-still"},
		LibraryID:  lib.ID,
		Title:      "遮天",
		Path:       `/media/tv/遮天/Season 1/遮天 - S01E02.mkv`,
		PosterURL:  `https://image.example/series-poster.jpg`,
		SeasonNum:  1,
		EpisodeNum: 152,
	}
	if err := svc.repo.DB.Create(&bare).Error; err != nil {
		t.Fatalf("create episode without still: %v", err)
	}
	bareItem := svc.itemPayload(t.Context(), &bare, false, 0)
	bareTags := bareItem["ImageTags"].(map[string]string)
	if _, hasThumb := bareTags["Thumb"]; hasThumb {
		t.Fatalf("episode without still must not advertise Thumb: %#v", bareTags)
	}
}

// TestEmbyEpisodeRowTitleKeepsStandardEmbyName 固定“最近观看/NextUp”卡片
// 的标题契约：Name 保持标准 Emby 行为（单集标题本身，不拼集数前缀）。
func TestEmbyEpisodeRowTitleKeepsStandardEmbyName(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "剧集", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	episode := model.Media{
		Base:         model.Base{ID: "ep-title"},
		LibraryID:    lib.ID,
		Title:        "遮天",
		EpisodeTitle: "天骄聚秦岭",
		Path:         `/media/tv/遮天/Season 1/遮天 - S01E151.mkv`,
		SeasonNum:    1,
		EpisodeNum:   151,
	}
	if err := svc.repo.DB.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}

	item := svc.itemPayload(t.Context(), &episode, false, 0)
	if item["Name"] != "天骄聚秦岭" {
		t.Fatalf("episode Name = %#v, want standard Emby title without number prefix", item["Name"])
	}
}

func TestEmbyRecordProgressRestoresHiddenItemToResume(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := seedResumeRowFixtures(t, svc)

	if err := svc.SetHiddenFromResume(t.Context(), viewer.ID, "resume-b"); err != nil {
		t.Fatalf("hide from resume: %v", err)
	}
	out, err := svc.ResumeItems(t.Context(), viewer.ID)
	if err != nil {
		t.Fatalf("resume items after hide: %v", err)
	}
	if out["TotalRecordCount"] != 2 {
		t.Fatalf("resume total after hide = %#v, want 2", out["TotalRecordCount"])
	}

	// 再次播放（Emby /Sessions/Playing/Progress 路径）：条目应恢复到继续观看。
	if err := svc.RecordProgress(t.Context(), viewer.ID, "resume-b", 400_000, 12_000_000_000); err != nil {
		t.Fatalf("record progress: %v", err)
	}
	out, err = svc.ResumeItems(t.Context(), viewer.ID)
	if err != nil {
		t.Fatalf("resume items after replay: %v", err)
	}
	if out["TotalRecordCount"] != 3 {
		t.Fatalf("resume total after replay = %#v, want 3", out["TotalRecordCount"])
	}
	stored, err := svc.repo.MediaPlaybackPreference.FindByUserAndMedia(t.Context(), viewer.ID, "resume-b")
	if err != nil || stored == nil {
		t.Fatalf("load preference: %v %#v", err, stored)
	}
	if stored.HiddenFromResume {
		t.Fatalf("hidden flag should be cleared after replay: %#v", stored)
	}

	// 内部播放 API 路径同样恢复。
	if err := svc.SetHiddenFromResume(t.Context(), viewer.ID, "resume-c"); err != nil {
		t.Fatalf("hide again: %v", err)
	}
	playback := NewPlaybackService(zap.NewNop(), svc.repo)
	if err := playback.RecordProgress(t.Context(), viewer.ID, "resume-c", 200_000, 1_200_000); err != nil {
		t.Fatalf("record progress via playback service: %v", err)
	}
	stored, err = svc.repo.MediaPlaybackPreference.FindByUserAndMedia(t.Context(), viewer.ID, "resume-c")
	if err != nil || stored == nil {
		t.Fatalf("load preference: %v %#v", err, stored)
	}
	if stored.HiddenFromResume {
		t.Fatalf("playback service should clear hidden flag: %#v", stored)
	}
}
