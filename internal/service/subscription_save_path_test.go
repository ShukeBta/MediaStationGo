package service

import (
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubscriptionSaveRootPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, explicit, setting, containerEnv, hostEnv, want string
	}{
		{name: "subscription override", explicit: "/qb/custom", setting: "/qb/default", containerEnv: "/container/downloads", hostEnv: "/host/downloads", want: "/qb/custom"},
		{name: "configured root", setting: "/qb/default", containerEnv: "/container/downloads", hostEnv: "/host/downloads", want: "/qb/default"},
		{name: "container environment", containerEnv: "/container/downloads", hostEnv: "/host/downloads", want: "/container/downloads"},
		{name: "download environment", hostEnv: "/host/downloads", want: "/host/downloads"},
		{name: "downloader default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", tc.containerEnv)
			t.Setenv("MEDIASTATION_DOWNLOAD_DIR", tc.hostEnv)
			db := newServiceTestDB(t, &model.Setting{})
			repos := repository.New(db)
			if err := repos.Setting.Set(t.Context(), "qbittorrent.savepath", tc.setting); err != nil {
				t.Fatal(err)
			}
			svc := &SubscriptionService{repo: repos}
			sub := &model.Subscription{SavePath: tc.explicit}
			if got := svc.subscriptionBaseSavePath(t.Context(), sub); got != tc.want {
				t.Fatalf("availability root = %q, want %q", got, tc.want)
			}
			want := tc.want
			if want != "" {
				want = filepath.Join(want, "国产剧")
			}
			if got := svc.resolveSubscriptionSavePath(t.Context(), sub, "tv", "国产剧"); got != want {
				t.Fatalf("classified destination = %q, want %q", got, want)
			}
			if err := repos.Setting.Set(t.Context(), DownloadSmartClassifySettingKey, "false"); err != nil {
				t.Fatal(err)
			}
			if got := svc.resolveSubscriptionSavePath(t.Context(), sub, "tv", "国产剧"); got != tc.want {
				t.Fatalf("unclassified destination = %q, want %q", got, tc.want)
			}
		})
	}
}
