package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestDownloadViewsDoNotExposePrivateURL(t *testing.T) {
	rows := []model.DownloadTask{{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://tracker.example/download?id=1&passkey=private-token",
		Title:    "测试影片",
		SavePath: "/downloads",
		Status:   "queued",
	}}

	tasks, torrents := DownloadViews(rows, nil)
	data, err := json.Marshal(map[string]any{
		"tasks":    tasks,
		"torrents": torrents,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if strings.Contains(body, "private-token") || strings.Contains(body, "passkey") || strings.Contains(body, "tracker.example") {
		t.Fatalf("download views leaked private URL: %s", body)
	}
	if !strings.Contains(body, "测试影片") {
		t.Fatalf("download views should keep public title: %s", body)
	}
}

func TestDownloadCompleteNotificationPayloadUsesTaskMetadata(t *testing.T) {
	body, data := downloadCompleteNotificationPayload(QBitTorrent{
		Hash:        "done123",
		Name:        "Release.Name.S01E02.1080p",
		SavePath:    "/downloads/show",
		ContentPath: "/downloads/show/Release.Name.S01E02.1080p.mkv",
	}, &model.DownloadTask{
		Title:            "正式标题",
		PosterURL:        "https://img.example/poster.jpg",
		BackdropURL:      "https://img.example/backdrop.jpg",
		MediaType:        "tv",
		MediaCategory:    "日番",
		Overview:         "简介",
		OriginalName:     "Original Title",
		OriginalLanguage: "ja",
		Year:             2026,
		Rating:           8.7,
		Genres:           "动画,剧情",
	})

	if !strings.Contains(body, "任务：正式标题") {
		t.Fatalf("body should prefer task title, got %q", body)
	}
	for _, private := range []string{"保存路径", "/downloads/show", "Hash", "done123"} {
		if strings.Contains(body, private) {
			t.Fatalf("body should not expose %q, got %q", private, body)
		}
	}
	for key, want := range map[string]interface{}{
		"resource_title":    "Release.Name.S01E02.1080p",
		"title":             "正式标题",
		"poster_url":        "https://img.example/poster.jpg",
		"backdrop_url":      "https://img.example/backdrop.jpg",
		"media_type":        "tv",
		"media_category":    "日番",
		"overview":          "简介",
		"original_title":    "Original Title",
		"original_language": "ja",
		"year":              2026,
		"rating":            float32(8.7),
		"genres":            "动画,剧情",
	} {
		if got := data[key]; got != want {
			t.Fatalf("data[%s] = %#v, want %#v", key, got, want)
		}
	}
}
