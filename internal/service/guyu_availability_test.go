package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

const guyuTMDbID = 279155

func guyuAvailabilitySubscription(savePath string) *model.Subscription {
	return &model.Subscription{
		Base:          model.Base{ID: "sub-guyu-availability"},
		Name:          "谷雨",
		Filter:        "谷雨",
		OriginalName:  "The Silent Storm",
		TMDbID:        guyuTMDbID,
		MediaType:     "tv",
		TotalEpisodes: 30,
		SavePath:      savePath,
	}
}

func guyuEpisodeRows(title string, tmdbID int) []model.Media {
	rows := make([]model.Media, 0, 8)
	for episode := 1; episode <= 8; episode++ {
		rows = append(rows, model.Media{
			Title:      title + " S01E" + twoDigits(episode),
			Path:       "/library/" + title + "/" + title + ".S01E" + twoDigits(episode) + ".mkv",
			TMDbID:     tmdbID,
			SeasonNum:  1,
			EpisodeNum: episode,
		})
	}
	return rows
}

func twoDigits(value int) string {
	if value < 10 {
		return "0" + string(rune('0'+value))
	}
	return string(rune('0'+value/10)) + string(rune('0'+value%10))
}

func TestGuyuLibraryAvailabilityRejectsDifferentTMDbIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		title  string
		tmdbID int
	}{
		{name: "different_nonzero_tmdb", title: "倾世锦鳞谷雨来", tmdbID: 991001},
		{name: "zero_tmdb_full_title", title: "倾世锦鳞谷雨来", tmdbID: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := newServiceTestDB(t, &model.Media{})
			repos := repository.New(db)
			rows := guyuEpisodeRows(test.title, test.tmdbID)
			if err := db.Create(&rows).Error; err != nil {
				t.Fatal(err)
			}

			availability := SubscriptionLocalAvailability(t.Context(), repos, guyuAvailabilitySubscription(""))
			if availability.DownloadedEpisodes != 0 || availability.LocalMediaCount != 0 || availability.InLibrary {
				t.Fatalf("polluted library counted: %+v", availability)
			}
			missingEpisodes := missingEpisodeSet(availability)
			for episode := 1; episode <= 8; episode++ {
				if _, missing := missingEpisodes[episode]; !missing {
					t.Fatalf("E%02d missing marker absent: %+v", episode, availability.MissingEpisodeKeys)
				}
			}
		})
	}
}

func TestGuyuLibraryAvailabilityAcceptsMatchingTMDbIdentity(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	rows := guyuEpisodeRows("The Silent Storm", guyuTMDbID)
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	availability := SubscriptionLocalAvailability(t.Context(), repos, guyuAvailabilitySubscription(""))
	if availability.DownloadedEpisodes != 8 || availability.LocalMediaCount != 8 {
		t.Fatalf("matching library rows not counted: %+v", availability)
	}
	for episode := 1; episode <= 8; episode++ {
		if _, missing := availability.MissingEpisodeKeys[episodeKey(1, episode)]; missing {
			t.Fatalf("matching E%02d was marked missing: %+v", episode, availability.MissingEpisodeKeys)
		}
	}
}

func TestGuyuPendingAvailabilityRequiresStrictReleaseTitle(t *testing.T) {
	root := t.TempDir()
	wrongDir := filepath.Join(root, "倾世锦鳞谷雨来", "Season 01")
	rightDir := filepath.Join(root, "The Silent Storm", "Season 01")
	if err := os.MkdirAll(wrongDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rightDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for dir, name := range map[string]string{
		wrongDir: "倾世锦鳞谷雨来 - S01E05.mkv",
		rightDir: "The Silent Storm - S01E05.mkv",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:guyu-wrong",
		Title:    "倾世锦鳞谷雨来 S01E05 1080p",
		SavePath: root,
		Status:   "queued",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:guyu-right",
		Title:    "The Silent Storm S01E05 1080p",
		SavePath: root,
		Status:   "queued",
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	availability := svc.pendingDownloadAvailability(t.Context(), guyuAvailabilitySubscription(root))
	if availability.DownloadedEpisodes != 1 {
		t.Fatalf("pending availability = %+v, want only one strict E05 match", availability)
	}
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 5)]; !ok {
		t.Fatalf("strict E05 was not counted: %+v", availability.ExistingEpisodeKeys)
	}
	if !svc.downloadPathHasCandidate(t.Context(), guyuAvailabilitySubscription(root), "The Silent Storm S01E05 1080p", root) {
		t.Fatal("strict matching directory file should satisfy E05")
	}
	if svc.downloadPathHasCandidate(t.Context(), guyuAvailabilitySubscription(root), "倾世锦鳞谷雨来 S01E05 1080p", root) {
		t.Fatal("substring-only directory file must not satisfy E05")
	}
}

func TestGuyuLiveTorrentAvailabilityRequiresStrictReleaseTitle(t *testing.T) {
	adapter := &guyuAvailabilityAdapter{torrents: []TorrentInfo{
		{Name: "倾世锦鳞谷雨来 S01E05 1080p", State: "downloading"},
		{Name: "The Silent Storm S01E05 1080p", State: "downloading"},
	}}
	manager := NewDownloadManager(zap.NewNop(), nil, nil)
	manager.clients = map[string]DownloadAdapter{"guyu-test": adapter}
	manager.models = map[string]model.DownloadClient{"guyu-test": {Base: model.Base{ID: "guyu-test"}, Name: "guyu-test", Type: "fake", IsDefault: true}}
	manager.order = []string{"guyu-test"}
	downloads := &DownloadService{manager: manager}
	svc := NewSubscriptionService(nil, nil, nil, downloads, nil, nil)
	availability := svc.pendingDownloadAvailability(t.Context(), guyuAvailabilitySubscription(""))
	if availability.DownloadedEpisodes != 1 {
		t.Fatalf("live availability = %+v, want only one strict E05 match", availability)
	}
}

type guyuAvailabilityAdapter struct {
	torrents []TorrentInfo
}

func (a *guyuAvailabilityAdapter) Initialize(context.Context, DownloadClientConfig) error { return nil }
func (a *guyuAvailabilityAdapter) Ping(context.Context) error                             { return nil }
func (a *guyuAvailabilityAdapter) AddTorrent(context.Context, string, string) (string, error) {
	return "", nil
}
func (a *guyuAvailabilityAdapter) AddMagnet(context.Context, string, string) (string, error) {
	return "", nil
}
func (a *guyuAvailabilityAdapter) Pause(context.Context, string) error  { return nil }
func (a *guyuAvailabilityAdapter) Resume(context.Context, string) error { return nil }
func (a *guyuAvailabilityAdapter) Remove(context.Context, string, bool) error {
	return nil
}
func (a *guyuAvailabilityAdapter) List(context.Context, string) ([]TorrentInfo, error) {
	return a.torrents, nil
}
func (a *guyuAvailabilityAdapter) GetInfo(context.Context, string) (*TorrentInfo, error) {
	return nil, nil
}
