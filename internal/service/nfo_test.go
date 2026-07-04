package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestWriteMediaNFOUsesMappedDestinationPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MEDIASTATION_MEDIA_DIR", root)
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", "/media")
	mediaPath := filepath.Join(root, "电影", "测试电影.mkv")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := WriteMediaNFO(&model.Media{
		Title: "测试电影",
		Path:  "/media/电影/测试电影.mkv",
		Year:  2026,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "电影", "测试电影.nfo")
	if got != want {
		t.Fatalf("nfo path = %q, want %q", got, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
}

func TestWriteMediaNFOUsesEpisodeTitleForEpisodeDetails(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "剧集", "间谍过家家", "Season 02", "间谍过家家 - S02E01.mkv")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := WriteMediaNFO(&model.Media{
		Title:        "间谍过家家",
		OriginalName: "SPY×FAMILY",
		EpisodeTitle: "任务代号: 猫",
		Path:         mediaPath,
		SeasonNum:    2,
		EpisodeNum:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "<title>任务代号: 猫</title>") || !strings.Contains(text, "<showtitle>间谍过家家</showtitle>") {
		t.Fatalf("episode nfo did not keep episode/show titles separate:\n%s", text)
	}
}
