package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestSubtitlePipelineHTTPClientUsesAuthenticatedCandidateSessions(t *testing.T) {
	canApply := true
	canPreview := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
		}
		switch r.URL.Path {
		case "/v1/subtitles/search":
			if body["owner_id"] != "admin-id" || body["media_id"] != "media-1" {
				t.Fatalf("unexpected request body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(SubtitleSearchResponse{
				SessionID: "session-1", MediaID: "media-1",
				Items: []SubtitleSearchCandidate{{
					CandidateID: "candidate-1", Provider: "subtitlecat",
					CanApply: &canApply, CanPreview: &canPreview,
					PreviewUnavailableReason: "SUP 是图形字幕，当前没有可展示的文本预览",
				}},
			})
		case "/v1/subtitles/preview":
			if body["search_session_id"] != "session-1" || body["candidate_id"] != "candidate-1" {
				t.Fatalf("unexpected selection: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(SubtitleCandidatePreview{
				SubtitleSearchCandidate: SubtitleSearchCandidate{CandidateID: "candidate-1", Provider: "subtitlecat"},
				MediaID:                 "media-1", ContentSample: "字幕预览",
			})
		case "/v1/subtitles/apply":
			_ = json.NewEncoder(w).Encode(SubtitleApplyResult{
				MediaID: "media-1", Status: "success", Source: "subtitlecat", Filename: "subtitle.srt", Count: 1,
			})
		case "/v1/subtitles/season/tasks/season-task-1/retry":
			mediaIDs, ok := body["media_ids"].([]any)
			if body["owner_id"] != "admin-id" || !ok || len(mediaIDs) != 1 || mediaIDs[0] != "episode-5" {
				t.Fatalf("unexpected season retry body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(SubtitleSeasonTask{
				ID: "season-task-2", MediaID: "media-1", Season: 1, Status: "queued", ProgressTotal: 1,
			})
		case "/v1/subtitles/asr":
			if r.Method == http.MethodGet {
				if r.URL.Query().Get("limit") != "50" {
					t.Fatalf("unexpected ASR list query: %s", r.URL.RawQuery)
				}
				_ = json.NewEncoder(w).Encode(SubtitleASRTaskList{Items: []SubtitleASRTask{{
					ID: "asr-1", OwnerID: "admin-id", MediaID: "media-1", SourceLanguage: "ja",
					Status: "completed", Stage: "completed",
				}}})
				return
			}
			if body["asr_model"] != "faster-whisper/large-v3" {
				t.Fatalf("unexpected ASR model: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(SubtitleASRTask{
				ID: "asr-1", OwnerID: "admin-id", MediaID: "media-1", SourceLanguage: "ja",
				Status: "queued", Stage: "queued",
			})
		case "/v1/subtitles/asr/asr-1":
			if r.URL.Query().Get("owner_id") != "admin-id" {
				t.Fatalf("unexpected owner query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(SubtitleASRTask{
				ID: "asr-1", OwnerID: "admin-id", MediaID: "media-1", SourceLanguage: "ja",
				Status: "completed", Stage: "completed",
				Result: &SubtitleASRResult{Filename: "sensevoice-deepseek-zh-cn.srt", Source: "sensevoice-deepseek"},
			})
		case "/v1/subtitles/asr/asr-1/model":
			if body["owner_id"] != "admin-id" || body["asr_model"] != "faster-whisper/large-v3" || body["translation_model"] != "qwen-next" {
				t.Fatalf("unexpected model update body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(SubtitleASRTask{ID: "asr-1", Status: "queued", TranslationModel: "qwen-next"})
		case "/v1/subtitles/asr/asr-models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []string{"FunAudioLLM/SenseVoiceSmall", "faster-whisper/large-v3"}})
		case "/v1/subtitles/asr/asr-1/cancel":
			if body["owner_id"] != "admin-id" {
				t.Fatalf("unexpected cancel body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(SubtitleASRTask{ID: "asr-1", Status: "canceled", Stage: "canceled"})
		case "/v1/subtitles/asr/asr-1/retranslate":
			if body["owner_id"] != "admin-id" || body["translation_model"] != "qwen-next" {
				t.Fatalf("unexpected retranslate body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(SubtitleASRTask{
				ID: "asr-1", Status: "queued", TranslationModel: "qwen-next",
				CachedAudio: true, CachedTranscript: true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := newResourcePipelineHTTPClient(config.ResourceImportConfig{
		PipelineURL: server.URL, PipelineToken: "secret", SearchTimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	search, err := client.SearchSubtitles(t.Context(), "admin-id", "media-1", 20)
	if err != nil {
		t.Fatal(err)
	}
	if search.SessionID != "session-1" || len(search.Items) != 1 || search.Items[0].CanApply == nil || !*search.Items[0].CanApply || search.Items[0].CanPreview == nil || *search.Items[0].CanPreview {
		t.Fatalf("unexpected search response: %#v", search)
	}
	preview, err := client.PreviewSubtitle(t.Context(), "admin-id", "media-1", "session-1", "candidate-1")
	if err != nil {
		t.Fatal(err)
	}
	if preview.ContentSample != "字幕预览" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	applied, err := client.ApplySubtitle(t.Context(), "admin-id", "media-1", "session-1", "candidate-1")
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != "success" || applied.Filename != "subtitle.srt" {
		t.Fatalf("unexpected apply response: %#v", applied)
	}
	retried, err := client.RetrySeasonSubtitles(t.Context(), "admin-id", "season-task-1", []string{"episode-5"})
	if err != nil || retried.ID != "season-task-2" || retried.ProgressTotal != 1 {
		t.Fatalf("unexpected season retry response: %#v err=%v", retried, err)
	}
	asr, err := client.CreateSubtitleASR(t.Context(), "admin-id", "media-1", "ja", "faster-whisper/large-v3", "local", "qwen-test")
	if err != nil || asr.ID != "asr-1" || asr.SourceLanguage != "ja" {
		t.Fatalf("unexpected ASR create response: %#v err=%v", asr, err)
	}
	asr, err = client.GetSubtitleASR(t.Context(), "admin-id", "asr-1")
	if err != nil || asr.Status != "completed" || asr.Result == nil || asr.Result.Source != "sensevoice-deepseek" {
		t.Fatalf("unexpected ASR get response: %#v err=%v", asr, err)
	}
	listed, err := client.ListSubtitleASR(t.Context(), 50)
	if err != nil || len(listed) != 1 || listed[0].ID != "asr-1" {
		t.Fatalf("unexpected ASR list response: %#v err=%v", listed, err)
	}
	models, err := client.ListSubtitleASREngines(t.Context())
	if err != nil || len(models) != 2 || models[1] != "faster-whisper/large-v3" {
		t.Fatalf("unexpected ASR models: %#v err=%v", models, err)
	}
	updated, err := client.UpdateSubtitleASRModel(t.Context(), "admin-id", "asr-1", "faster-whisper/large-v3", "local", "qwen-next")
	if err != nil || updated.Status != "queued" || updated.TranslationModel != "qwen-next" {
		t.Fatalf("unexpected ASR model update response: %#v err=%v", updated, err)
	}
	canceled, err := client.CancelSubtitleASR(t.Context(), "admin-id", "asr-1")
	if err != nil || canceled.Status != "canceled" {
		t.Fatalf("unexpected ASR cancel response: %#v err=%v", canceled, err)
	}
	retranslated, err := client.RetranslateSubtitleASR(t.Context(), "admin-id", "asr-1", "local", "qwen-next")
	if err != nil || retranslated.Status != "queued" || !retranslated.CachedTranscript {
		t.Fatalf("unexpected ASR retranslate response: %#v err=%v", retranslated, err)
	}
}
