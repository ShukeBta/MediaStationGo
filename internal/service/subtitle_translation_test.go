package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubtitleTranslationUsesPlainTextWithContextAndGlossary(t *testing.T) {
	var authorization string
	var requestedModel string
	var userPrompt string
	var responseFormat any
	var temperature float64
	var topP float64
	var maxTokens int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		var payload struct {
			Model          string  `json:"model"`
			ResponseFormat any     `json:"response_format"`
			Temperature    float64 `json:"temperature"`
			TopP           float64 `json:"top_p"`
			MaxTokens      int     `json:"max_tokens"`
			Messages       []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		requestedModel = payload.Model
		responseFormat = payload.ResponseFormat
		temperature = payload.Temperature
		topP = payload.TopP
		maxTokens = payload.MaxTokens
		if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
			t.Fatalf("messages = %#v", payload.Messages)
		}
		userPrompt = payload.Messages[0].Content
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"今天来晚了。"}}]}`))
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.APIConfig{})
	apiConfig := NewAPIConfigService(
		zap.NewNop(), repository.New(db), NewCryptoService("test-secret", zap.NewNop()),
	)
	key := "cloud-secret"
	baseURL := server.URL + "/v1"
	modelID := "deepseek-chat"
	enabled := true
	if _, err := apiConfig.Update(t.Context(), "deepseek", APIConfigPatch{
		APIKey: &key, BaseURL: &baseURL, Model: &modelID, Enabled: &enabled,
	}); err != nil {
		t.Fatal(err)
	}

	service := NewSubtitleService(zap.NewNop(), repository.New(db)).SetAPIConfig(apiConfig)
	result, err := service.TranslateText(
		context.Background(),
		"deepseek",
		modelID,
		SubtitleTranslationInput{
			Text: "今日は遅かった。", Context: []string{"昨日も遅かった。"}, Glossary: "東京 -> 东京",
			RetryInstruction: "上次译文仍含日文假名，请全部翻译。",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer "+key || requestedModel != modelID {
		t.Fatalf("authorization=%q model=%q", authorization, requestedModel)
	}
	if responseFormat != nil {
		t.Fatalf("response_format must not be sent: %#v", responseFormat)
	}
	if temperature != 0.1 || topP != 0.9 || maxTokens != 1024 {
		t.Fatalf("parameters temperature=%v top_p=%v max_tokens=%d", temperature, topP, maxTokens)
	}
	if !strings.Contains(userPrompt, "昨日も遅かった。") || !strings.Contains(userPrompt, "東京 -> 东京") ||
		!strings.Contains(userPrompt, "上次译文仍含日文假名") || !strings.HasSuffix(userPrompt, "今日は遅かった。") {
		t.Fatalf("prompt does not contain context, glossary, and target: %q", userPrompt)
	}
	if result.Translation != "今天来晚了。" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSubtitleTranslationRejectsUnconfiguredModel(t *testing.T) {
	db := newServiceTestDB(t, &model.APIConfig{})
	apiConfig := NewAPIConfigService(
		zap.NewNop(), repository.New(db), NewCryptoService("test-secret", zap.NewNop()),
	)
	key := "cloud-secret"
	baseURL := "https://example.invalid/v1"
	modelID := "configured-model"
	if _, err := apiConfig.Update(t.Context(), "siliconflow", APIConfigPatch{
		APIKey: &key, BaseURL: &baseURL, Model: &modelID,
	}); err != nil {
		t.Fatal(err)
	}
	service := NewSubtitleService(zap.NewNop(), repository.New(db)).SetAPIConfig(apiConfig)
	_, err := service.TranslateText(
		t.Context(), "siliconflow", "client-supplied-model",
		SubtitleTranslationInput{Text: "こんにちは"},
	)
	if err == nil {
		t.Fatal("expected mismatched model to be rejected")
	}
}

func TestSubtitleTranslationRejectsClearlyUntranslatedOutput(t *testing.T) {
	for _, output := range []string{"こんにちは", "```text\n你好\n```", `{"translation":"你好"}`, "<think>分析</think>你好"} {
		if _, err := validateSubtitleTranslationOutput("こんにちは", output); err == nil {
			t.Fatalf("expected output to be rejected: %q", output)
		}
	}
	if translated, err := validateSubtitleTranslationOutput("こんにちは", "你好"); err != nil || translated != "你好" {
		t.Fatalf("valid translation rejected: %q %v", translated, err)
	}
}
