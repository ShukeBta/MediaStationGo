package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

func TestChannelSubscribesCanDisableAllEvents(t *testing.T) {
	channel := model.NotifyChannel{Events: `["` + NotifyEventNone + `"]`}

	if channelSubscribes(channel, EventDownloadComplete) {
		t.Fatal("explicit none sentinel should disable event pushes")
	}
}

func TestChannelSubscribesKeepsLegacyEmptyAsAllEvents(t *testing.T) {
	for _, raw := range []string{"", "[]"} {
		channel := model.NotifyChannel{Events: raw}
		if !channelSubscribes(channel, EventDownloadComplete) {
			t.Fatalf("legacy events %q should still subscribe to all events", raw)
		}
	}
}

func TestChannelSubscribesSupportsExplicitAllAndSpecificEvents(t *testing.T) {
	all := model.NotifyChannel{Events: `["` + NotifyEventAll + `"]`}
	if !channelSubscribes(all, EventScrapeFailed) {
		t.Fatal("explicit all sentinel should subscribe to every event")
	}

	specific := model.NotifyChannel{Events: `["` + EventDownloadComplete + `"]`}
	if !channelSubscribes(specific, EventDownloadComplete) {
		t.Fatal("specific event should be subscribed")
	}
	if channelSubscribes(specific, EventScrapeFailed) {
		t.Fatal("unlisted event should not be subscribed")
	}
}

func TestTelegramDispatchUsesPhotoAndFormattedCaption(t *testing.T) {
	var gotPath string
	var gotForm map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = map[string]string{}
		for key := range r.Form {
			gotForm[key] = r.Form.Get(key)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	svc := NewNotifyChannelService(zap.NewNop(), nil)
	channel := model.NotifyChannel{
		Type: "telegram",
		Config: `{
			"bot_token":"123456:ABC",
			"group_chat_id":"-10001",
			"api_base_url":"` + server.URL + `"
		}`,
	}
	err := svc.dispatchOneEvent(t.Context(), channel, NotifyEvent{
		Type:    EventDownloadComplete,
		Title:   "MediaStationGo 下载完成",
		Message: "任务：示例影片\n保存路径：/downloads/movie\nHash：abcdef",
		Data: map[string]interface{}{
			"poster_url":        server.URL + "/poster.jpg",
			"media_type":        "tv",
			"media_category":    "纪录片剧集",
			"title":             "美国甜心：达拉斯牛仔啦啦队",
			"original_title":    "AMERICA'S SWEETHEARTS: Dallas Cowboys Cheerleaders",
			"original_language": "en",
			"year":              2024,
			"season_episode":    "S03E07",
			"size":              "3.0GB / 5.7Mbps",
			"version":           "H264.NF.FHD-HHWEB",
			"rating":            8.2,
			"genres":            "纪录",
			"overview":          "从试镜到训练营再到 NFL 赛季，一路跟随达拉斯牛仔队啦啦队队员们追逐梦想。",
			"tmdb_url":          server.URL + "/tmdb",
			"imdb_url":          server.URL + "/imdb",
			"douban_url":        server.URL + "/douban",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/bot123456:ABC/sendPhoto" {
		t.Fatalf("path = %q, want sendPhoto", gotPath)
	}
	if gotForm["chat_id"] != "-10001" || gotForm["photo"] == "" {
		t.Fatalf("telegram form = %#v", gotForm)
	}
	caption := gotForm["caption"]
	for _, want := range []string{
		"#下载完成",
		"📺 任务：示例影片",
	} {
		if !strings.Contains(caption, want) {
			t.Fatalf("caption missing %q: %s", want, caption)
		}
	}
	for _, unwanted := range []string{
		"✅ <b>下载完成</b>",
		"🎯 订阅命中新资源",
		"保存路径",
		"abcdef",
		"美国甜心：达拉斯牛仔啦啦队",
		"AMERICA'S SWEETHEARTS",
		"2024",
		"3.0GB",
		"简介",
		server.URL + "/tmdb",
	} {
		if strings.Contains(caption, unwanted) {
			t.Fatalf("caption should not include %q: %s", unwanted, caption)
		}
	}
}

func TestTelegramDownloadCompleteNeverFallsBackToSensitiveFields(t *testing.T) {
	caption := formatTelegramNotification(NotifyEvent{
		Type: EventDownloadComplete,
		Data: map[string]interface{}{
			"save_path": "/downloads/private/movie.mkv",
			"hash":      "abcdef",
		},
	})
	for _, want := range []string{"#下载完成", "📺 任务：下载任务"} {
		if !strings.Contains(caption, want) {
			t.Fatalf("caption missing %q: %s", want, caption)
		}
	}
	for _, unwanted := range []string{"保存路径", "/downloads/private", "Hash", "abcdef"} {
		if strings.Contains(caption, unwanted) {
			t.Fatalf("caption should not include %q: %s", unwanted, caption)
		}
	}
}

// TestTelegramMediaTemplateRendersEnrichedFields 验证补齐的媒体字段
// (年份/评分/类型/原名/语言)确实透传进 Telegram 富模板 caption。
// 这是「模型/刮削链路补齐字段」与「作者富模板」对接的端到端断言。
func TestTelegramMediaTemplateRendersEnrichedFields(t *testing.T) {
	caption := formatTelegramNotification(NotifyEvent{
		Type:    EventSubscriptionHit,
		Title:   "MediaStationGo 订阅命中新资源",
		Message: "订阅：遮天\n新增资源：1",
		Data: map[string]interface{}{
			"media_category":    "国漫",
			"title":             "遮天",
			"original_title":    "Shrouding the Heavens",
			"original_language": "zh",
			"year":              2023,
			"rating":            8.6,
			"genres":            "动画,奇幻",
			"overview":          "荒古禁地中走出的少年。",
			"tmdb_url":          "https://www.themoviedb.org/tv/223911",
			"imdb_url":          "https://www.imdb.com/title/tt12345678/",
		},
	})
	for _, want := range []string{
		"#国漫",
		"📺 中文片名：遮天",
		"🧿 原始片名：Shrouding the Heavens",
		"🌐 原始语言：中文",
		"📅 发行年份：2023",
		"⭐️ 评分：8.6",
		"💎 类型：动画、奇幻",
		"🪬 简介：",
		`<a href="https://www.themoviedb.org/tv/223911">TMDB</a>`,
		`<a href="https://www.imdb.com/title/tt12345678/">IMDB</a>`,
	} {
		if !strings.Contains(caption, want) {
			t.Fatalf("caption missing %q:\n%s", want, caption)
		}
	}
}
