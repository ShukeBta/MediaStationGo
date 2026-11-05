package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type dvGuardTestProbe struct {
	resultFor func(string) (*ProbeResult, error)
	calls     []string
}

func (p *dvGuardTestProbe) Probe(_ context.Context, path string) (*ProbeResult, error) {
	p.calls = append(p.calls, path)
	if p.resultFor != nil {
		return p.resultFor(path)
	}
	return doviProbeResult(5, 0, true), nil
}

type dvGuardTestSubscriptionRunner struct {
	defaultExcludeWords string
	queued              int
	err                 error
	runs                int
}

func (r *dvGuardTestSubscriptionRunner) selectionDefaultExcludeWords(context.Context) string {
	return r.defaultExcludeWords
}

func (r *dvGuardTestSubscriptionRunner) RunNow(context.Context, string) (int, error) {
	r.runs++
	return r.queued, r.err
}

func doviProbeResult(profile, compatibility int, hasDOVI bool) *ProbeResult {
	return &ProbeResult{
		Document:                &ProbeDocument{SchemaVersion: ProbeDocumentSchemaVersion},
		HasDOVIConfiguration:    hasDOVI,
		DVProfile:               profile,
		HasDVProfile:            hasDOVI,
		DVBLCompatibilityID:     compatibility,
		HasDVBLCompatibilityID:  hasDOVI,
		VideoRange:              "Dolby Vision",
	}
}

func newDVGuardTestDownloadService(t *testing.T, root string, probe *dvGuardTestProbe, runner *dvGuardTestSubscriptionRunner, sub *model.Subscription) (*DownloadService, *repository.Container) {
	t.Helper()
	db := newServiceTestDB(t,
		&model.DownloadTask{}, &model.Subscription{}, &model.SubscriptionRejectedRelease{},
		&model.Media{}, &model.STRMRecord{}, &model.Setting{},
	)
	repos := repository.New(db)
	if err := repos.DB.Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.SetCompletionProbe(probe)
	svc.SetSubscriptionService(runner)
	return svc, repos
}

func addDVGuardTask(t *testing.T, repos *repository.Container, subID, title, hash, contentPath string) *model.DownloadTask {
	t.Helper()
	task := &model.DownloadTask{
		UserID: "user-1", SubscriptionID: subID, ExternalID: hash,
		Source: "qbittorrent", URL: "https://tracker.example/torrent", Title: title,
		IdentityKey: downloadTaskIdentityKey(title), SavePath: filepath.Dir(contentPath),
		MediaType: "tv", Status: "completed", Progress: 1,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	return task
}

func runDVGuardCompletion(t *testing.T, svc *DownloadService, task *model.DownloadTask, contentPath string) {
	t.Helper()
	svc.onTorrentComplete(t.Context(), QBitTorrent{
		Hash: task.ExternalID, Name: task.Title, Progress: 1, State: "completed",
		SavePath: filepath.Dir(contentPath), ContentPath: contentPath,
	})
}

func writeDVGuardFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("video fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSubscriptionDVGuardProfile5CompatibilityZeroRejectsAndRequeues(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Guard Show S01E01 1080p DV.mkv")
	writeDVGuardFile(t, path)
	probe := &dvGuardTestProbe{}
	runner := &dvGuardTestSubscriptionRunner{queued: 1}
	sub := &model.Subscription{Name: "Guard Show", FeedURL: "https://feed.example/rss", Filter: "Guard Show", MediaType: "tv", ExcludeWords: "h265", Enabled: true}
	svc, repos := newDVGuardTestDownloadService(t, root, probe, runner, sub)
	task := addDVGuardTask(t, repos, sub.ID, "Guard Show S01E01 1080p DV", "hash-p5", path)

	runDVGuardCompletion(t, svc, task, path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("P5 file still exists, stat err=%v", err)
	}
	if runner.runs != 1 {
		t.Fatalf("RunNow calls = %d, want 1", runner.runs)
	}
	var rejected []model.SubscriptionRejectedRelease
	if err := repos.DB.Where("subscription_id = ?", sub.ID).Find(&rejected).Error; err != nil {
		t.Fatal(err)
	}
	if len(rejected) != 1 || rejected[0].EpisodeNumber != 1 || rejected[0].SeasonNumber != 1 {
		t.Fatalf("persisted rejected releases = %+v, want the release rejected for S01E01", rejected)
	}
	var stored model.DownloadTask
	if err := repos.DB.First(&stored, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "deleted" || !stored.CompletionGuardChecked {
		t.Fatalf("task completion state = status:%q guard_checked:%v", stored.Status, stored.CompletionGuardChecked)
	}
}

func TestSubscriptionDVGuardDoesNotRejectNonP5Profiles(t *testing.T) {
	tests := []struct {
		name          string
		profile       int
		compatibility int
		hasDOVI       bool
	}{
		{name: "profile 8 compatibility 1", profile: 8, compatibility: 1, hasDOVI: true},
		{name: "profile 8 compatibility 2", profile: 8, compatibility: 2, hasDOVI: true},
		{name: "profile 5 compatibility 1", profile: 5, compatibility: 1, hasDOVI: true},
		{name: "no DOVI configuration", profile: 0, compatibility: 0, hasDOVI: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "Guard Show S01E01 1080p DV.mkv")
			writeDVGuardFile(t, path)
			probe := &dvGuardTestProbe{resultFor: func(string) (*ProbeResult, error) {
				return doviProbeResult(test.profile, test.compatibility, test.hasDOVI), nil
			}}
			runner := &dvGuardTestSubscriptionRunner{queued: 1}
			sub := &model.Subscription{Name: "Guard Show", FeedURL: "https://feed.example/rss", Filter: "Guard Show", MediaType: "tv", ExcludeWords: "h265", Enabled: true}
			svc, repos := newDVGuardTestDownloadService(t, root, probe, runner, sub)
			task := addDVGuardTask(t, repos, sub.ID, "Guard Show S01E01 1080p DV", "hash-non-p5", path)

			runDVGuardCompletion(t, svc, task, path)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("non-P5 file was removed: %v", err)
			}
			if runner.runs != 0 {
				t.Fatalf("RunNow calls = %d, want 0", runner.runs)
			}
			var rejected int64
			if err := repos.DB.Model(&model.SubscriptionRejectedRelease{}).Count(&rejected).Error; err != nil {
				t.Fatal(err)
			}
			if rejected != 0 {
				t.Fatalf("rejected rows = %d, want 0", rejected)
			}
		})
	}
}

func TestSubscriptionDVGuardProbeErrorPreservesFileAndDoesNotReject(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Guard Show S01E01 1080p DV.mkv")
	writeDVGuardFile(t, path)
	probe := &dvGuardTestProbe{resultFor: func(string) (*ProbeResult, error) { return nil, errors.New("ffprobe unavailable") }}
	runner := &dvGuardTestSubscriptionRunner{queued: 1}
	sub := &model.Subscription{Name: "Guard Show", FeedURL: "https://feed.example/rss", Filter: "Guard Show", MediaType: "tv", ExcludeWords: "h265", Enabled: true}
	svc, repos := newDVGuardTestDownloadService(t, root, probe, runner, sub)
	task := addDVGuardTask(t, repos, sub.ID, "Guard Show S01E01 1080p DV", "hash-probe-error", path)

	runDVGuardCompletion(t, svc, task, path)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("probe error removed the file: %v", err)
	}
	if runner.runs != 0 {
		t.Fatalf("RunNow calls = %d, want 0", runner.runs)
	}
	var rejected int64
	if err := repos.DB.Model(&model.SubscriptionRejectedRelease{}).Count(&rejected).Error; err != nil {
		t.Fatal(err)
	}
	if rejected != 0 {
		t.Fatalf("rejected rows = %d, want 0", rejected)
	}
}

func TestSubscriptionDVGuardStopsReenqueueAfterThreeRejectedReleases(t *testing.T) {
	root := t.TempDir()
	probe := &dvGuardTestProbe{}
	runner := &dvGuardTestSubscriptionRunner{queued: 1}
	sub := &model.Subscription{Name: "Guard Show", FeedURL: "https://feed.example/rss", Filter: "Guard Show", MediaType: "tv", ExcludeWords: "h265", Enabled: true}
	svc, repos := newDVGuardTestDownloadService(t, root, probe, runner, sub)
	for i := 1; i <= 4; i++ {
		title := "Guard Show S01E01 1080p DV Release " + strings.Repeat("X", i)
		path := filepath.Join(root, "Guard Show S01E01 Release "+strings.Repeat("X", i)+".mkv")
		writeDVGuardFile(t, path)
		task := addDVGuardTask(t, repos, sub.ID, title, "hash-cap-"+strings.Repeat("x", i), path)
		runDVGuardCompletion(t, svc, task, path)
	}
	if runner.runs != subscriptionDVReenqueueCap {
		t.Fatalf("RunNow calls = %d, want %d", runner.runs, subscriptionDVReenqueueCap)
	}
	var count int64
	if err := repos.DB.Model(&model.SubscriptionRejectedRelease{}).
		Where("subscription_id = ? AND season_number = 1 AND episode_number = 1", sub.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("persisted episode rejection count = %d, want 4", count)
	}
}

func TestManualDownloadDoesNotRunSubscriptionDVGuard(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Guard Show S01E01 1080p DV.mkv")
	writeDVGuardFile(t, path)
	probe := &dvGuardTestProbe{}
	runner := &dvGuardTestSubscriptionRunner{queued: 1}
	sub := &model.Subscription{Name: "Guard Show", FeedURL: "https://feed.example/rss", Filter: "Guard Show", MediaType: "tv", ExcludeWords: "h265", Enabled: true}
	svc, repos := newDVGuardTestDownloadService(t, root, probe, runner, sub)
	task := addDVGuardTask(t, repos, "", "Guard Show S01E01 1080p DV", "hash-manual", path)
	// Manual tasks are not owned by this subscription, even if a matching
	// subscription exists in the database.
	runDVGuardCompletion(t, svc, task, path)
	if len(probe.calls) != 0 || runner.runs != 0 {
		t.Fatalf("manual task ran guard: probe calls=%d RunNow calls=%d", len(probe.calls), runner.runs)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("manual download file was removed: %v", err)
	}
}

func TestSubscriptionDVGuardOnlyDeletesMatchingFileFromMultiEpisodeTorrent(t *testing.T) {
	root := t.TempDir()
	episodeOne := filepath.Join(root, "Guard Show S01E01.mkv")
	episodeTwo := filepath.Join(root, "Guard Show S01E02.mkv")
	writeDVGuardFile(t, episodeOne)
	writeDVGuardFile(t, episodeTwo)
	probe := &dvGuardTestProbe{resultFor: func(path string) (*ProbeResult, error) {
		if strings.Contains(filepath.Base(path), "E01") {
			return doviProbeResult(5, 0, true), nil
		}
		return doviProbeResult(8, 1, true), nil
	}}
	runner := &dvGuardTestSubscriptionRunner{queued: 1}
	sub := &model.Subscription{Name: "Guard Show", FeedURL: "https://feed.example/rss", Filter: "Guard Show", MediaType: "tv", ExcludeWords: "h265", Enabled: true}
	svc, repos := newDVGuardTestDownloadService(t, root, probe, runner, sub)
	task := addDVGuardTask(t, repos, sub.ID, "Guard Show S01 Complete DV Pack", "hash-pack", episodeOne)
	task.SavePath = root
	if err := repos.DB.Model(&model.DownloadTask{}).Where("id = ?", task.ID).Update("save_path", root).Error; err != nil {
		t.Fatal(err)
	}
	runDVGuardCompletion(t, svc, task, root)
	if _, err := os.Stat(episodeOne); !os.IsNotExist(err) {
		t.Fatalf("P5 episode file remains, stat err=%v", err)
	}
	if _, err := os.Stat(episodeTwo); err != nil {
		t.Fatalf("non-P5 episode file was removed: %v", err)
	}
	if runner.runs != 1 {
		t.Fatalf("RunNow calls = %d, want 1", runner.runs)
	}
}

func TestSubscriptionDVGuardRequiresDVWordInEffectiveExclusions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Guard Show S01E01 1080p DV.mkv")
	writeDVGuardFile(t, path)
	probe := &dvGuardTestProbe{}
	runner := &dvGuardTestSubscriptionRunner{}
	sub := &model.Subscription{
		Name: "Guard Show", FeedURL: "site-search://tracker?keyword=Guard+Show", Filter: "Guard Show",
		MediaType: "tv", Effects: "dolby-vision", ExcludeWords: "h265", Enabled: true,
	}
	svc, repos := newDVGuardTestDownloadService(t, root, probe, runner, sub)
	task := addDVGuardTask(t, repos, sub.ID, "Guard Show S01E01 1080p DV", "hash-no-dv-filter", path)

	runDVGuardCompletion(t, svc, task, path)
	if len(probe.calls) != 0 || runner.runs != 0 {
		t.Fatalf("guard ran without a DV exclusion: probe calls=%d RunNow calls=%d", len(probe.calls), runner.runs)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file was removed while guard was disabled: %v", err)
	}
}

func TestRejectedSubscriptionReleasesAreFilteredFromBothCandidatePaths(t *testing.T) {
	title := "Guard Show S01E01 2160p WEB-DL Release"
	rejected := map[string]struct{}{rejectedReleaseIdentity(title, ""): {}}
	sub := &model.Subscription{Name: "Guard Show", Filter: "Guard Show", MediaType: "tv"}
	search := SearchResult{Title: title, DownloadURL: "https://tracker.example/download"}
	selected, stats := selectSiteSearchCandidatesWithStatsAndRejected([]SearchResult{search}, sub, nil, LocalAvailability{}, rejected)
	if len(selected) != 0 || stats.RejectedReleaseSkipped != 1 {
		t.Fatalf("site-search candidates=%+v stats=%+v, want refused release filtered", selected, stats)
	}
	rss := rssSubscriptionItem(title, "https://tracker.example/download")
	selected = selectRSSSubscriptionCandidatesExcluding([]rssItem{rss}, sub, nil, nil, LocalAvailability{}, rejected)
	if len(selected) != 0 {
		t.Fatalf("RSS candidates=%+v, want refused release filtered", selected)
	}
}

func TestRejectedReleaseIdentityKeepsAlternativeReleaseInBothCandidatePaths(t *testing.T) {
	rejectedTitle := "Guard Show S01E01 2160p WEB-DL OldGroup"
	alternativeTitle := "Guard Show S01E01 1080p BluRay NewGroup"
	rejected := map[string]struct{}{rejectedReleaseIdentity(rejectedTitle, ""): {}}
	sub := &model.Subscription{Name: "Guard Show", Filter: "Guard Show", MediaType: "tv"}

	searchResults := []SearchResult{
		{Title: rejectedTitle, DownloadURL: "https://tracker.example/old-release"},
		{Title: alternativeTitle, DownloadURL: "https://tracker.example/alternative-release"},
	}
	selected, stats := selectSiteSearchCandidatesWithStatsAndRejected(searchResults, sub, nil, LocalAvailability{}, rejected)
	if len(selected) != 1 || selected[0].Item.Title != alternativeTitle || stats.RejectedReleaseSkipped != 1 {
		t.Fatalf("site-search candidates=%+v stats=%+v, want only the alternative release selected", selected, stats)
	}

	rssItems := []rssItem{
		rssSubscriptionItem(rejectedTitle, "https://tracker.example/old-release"),
		rssSubscriptionItem(alternativeTitle, "https://tracker.example/alternative-release"),
	}
	selected = selectRSSSubscriptionCandidatesExcluding(rssItems, sub, nil, nil, LocalAvailability{}, rejected)
	if len(selected) != 1 || selected[0].Item.Title != alternativeTitle {
		t.Fatalf("RSS candidates=%+v, want only the alternative release selected", selected)
	}
}

func TestSubscriptionDVGuardEnqueueFilteringChecksPersistedRelease(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.SubscriptionRejectedRelease{}, &model.DownloadTask{})
	repos := repository.New(db)
	sub := &model.Subscription{Name: "Guard Show", FeedURL: "https://feed.example/rss", Enabled: true}
	if err := db.Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	row := model.SubscriptionRejectedRelease{
		SubscriptionID: sub.ID, SeasonNumber: 1, EpisodeNumber: 1,
		ReleaseIdentity: rejectedReleaseIdentity("Guard Show S01E01 Release", ""), ReleaseTitle: "Guard Show S01E01 Release",
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, nil, nil)
	if !svc.isRejectedSubscriptionRelease(t.Context(), sub.ID, rejectedReleaseIdentity(row.ReleaseTitle, "")) {
		t.Fatal("persisted release was not found by enqueue filter")
	}
}
