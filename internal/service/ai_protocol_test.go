package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestAIProtocolsSupportSearchRecommendationAndChat(t *testing.T) {
	for _, protocol := range []string{"openai", "responses", "anthropic", "gemini", "ollama"} {
		t.Run(protocol, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				wantPath := map[string]string{"openai": "/v1/chat/completions", "responses": "/v1/responses", "anthropic": "/v1/messages", "gemini": "/v1beta/models/test-model:generateContent", "ollama": "/api/chat"}[protocol]
				if r.Method != http.MethodPost || r.URL.Path != wantPath || r.URL.Query().Get("gateway") != "test" {
					t.Errorf("request = %s %s, want POST %s with proxy query", r.Method, r.URL, wantPath)
				}
				header, wantKey := "Authorization", "Bearer test-key"
				switch protocol {
				case "anthropic":
					header, wantKey = "x-api-key", "test-key"
					if r.Header.Get("anthropic-version") != "2023-06-01" {
						t.Error("missing Anthropic version")
					}
				case "gemini":
					header, wantKey = "x-goog-api-key", "test-key"
					if strings.Contains(r.URL.RawQuery, "test-key") {
						t.Error("Gemini key must not be in the URL")
					}
				case "ollama":
					wantKey = ""
				}
				if r.Header.Get(header) != wantKey {
					t.Errorf("unexpected %s header", header)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					return
				}
				messageKey, count := "messages", 1
				if call == 3 {
					count = 3
				}
				switch protocol {
				case "openai", "ollama":
					count++ // system message
					if protocol == "ollama" && payload["stream"] != false {
						t.Error("native Ollama must disable streaming")
					}
				case "responses":
					messageKey = "input"
					if payload["instructions"] == "" || payload["store"] != false || payload["temperature"] != nil {
						t.Errorf("invalid Responses options: %#v", payload)
					}
				case "anthropic":
					if payload["system"] == "" || payload["max_tokens"] != float64(1024) {
						t.Errorf("invalid Anthropic options: %#v", payload)
					}
				case "gemini":
					messageKey = "contents"
					if payload["systemInstruction"] == nil {
						t.Error("missing Gemini system instruction")
					}
				}
				messages, ok := payload[messageKey].([]any)
				if !ok || len(messages) != count {
					t.Errorf("%s = %#v, want %d messages", messageKey, payload[messageKey], count)
				} else if protocol == "gemini" && call == 3 && messages[1].(map[string]any)["role"] != "model" {
					t.Error("Gemini assistant history must use the model role")
				}
				answer := `{"query":"Arrival","year":2016}`
				if call == 2 {
					answer = "Arrival, Contact"
				} else if call == 3 {
					answer = "Hello"
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(aiProtocolTestResponse(protocol, answer))
			}))
			defer server.Close()
			key := "test-key"
			if protocol == "ollama" {
				key = ""
			}
			svc := NewAIService(&config.Config{AI: config.AIConfig{Enabled: true, Provider: protocol, Protocol: protocol, APIBase: server.URL + "?gateway=test", APIKey: key, Model: "test-model"}}, zap.NewNop(), nil)
			svc.client = server.Client()
			intent, err := svc.SmartSearch(t.Context(), "a science fiction movie")
			if err != nil || intent.Query != "Arrival" || intent.Year != 2016 {
				t.Fatalf("search = %#v, %v", intent, err)
			}
			titles, err := svc.Recommend(t.Context(), []string{"Interstellar"}, 2)
			if err != nil || !reflect.DeepEqual(titles, []string{"Arrival", "Contact"}) {
				t.Fatalf("recommendations = %#v, %v", titles, err)
			}
			reply, err := svc.Chat(t.Context(), []ChatTurn{{Role: "user", Content: "first"}, {Role: "assistant", Content: "previous"}, {Role: "user", Content: "now"}})
			if err != nil || reply != "Hello" || calls.Load() != 3 {
				t.Fatalf("chat = %q, %v; calls=%d", reply, err, calls.Load())
			}
		})
	}
}

func aiProtocolTestResponse(protocol, answer string) any {
	switch protocol {
	case "responses":
		return map[string]any{"status": "completed", "output": []any{
			map[string]any{"type": "reasoning"},
			map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": answer}}},
		}}
	case "anthropic":
		return map[string]any{"content": []any{map[string]string{"type": "thinking", "thinking": "not the reply"}, map[string]string{"type": "text", "text": answer}}}
	case "gemini":
		return map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"thought": true, "text": "not the reply"}, map[string]string{"text": answer}}}}}}
	case "ollama":
		return map[string]any{"message": ChatTurn{Role: "assistant", Content: answer}, "done": true}
	default:
		return map[string]any{"choices": []any{map[string]any{"message": ChatTurn{Role: "assistant", Content: answer}}}}
	}
}

func TestAIEndpointAcceptsCompleteURLs(t *testing.T) {
	for _, tt := range []struct{ protocol, base string }{
		{"openai", "https://proxy.test/v1/chat/completions"},
		{"responses", "https://proxy.test/v1/responses"},
		{"anthropic", "https://proxy.test/v1/messages"},
		{"gemini", "https://proxy.test/v1beta/models/test:generateContent"},
		{"ollama", "http://localhost:11434/api/chat"},
	} {
		got, err := aiEndpoint(aiRuntimeConfig{Protocol: tt.protocol, APIBase: tt.base + "/", Model: "test"})
		if err != nil || got != tt.base {
			t.Errorf("%s endpoint = %q, %v; want %q", tt.protocol, got, err, tt.base)
		}
	}
}

func TestAIKeepsCustomProviderCompatibleByDefault(t *testing.T) {
	svc := NewAIService(&config.Config{AI: config.AIConfig{Enabled: true, Provider: "custom-proxy", APIBase: "https://proxy.test/v1", APIKey: "test", Model: "custom-model"}}, zap.NewNop(), nil)
	if cfg := svc.resolveRuntimeConfig(t.Context()); !cfg.Enabled || cfg.Protocol != "openai" || cfg.Provider != "custom-proxy" {
		t.Fatal("custom provider labels must preserve OpenAI-compatible behavior")
	}
}

func TestAIConfigSwitchClearsOtherProviderCredentials(t *testing.T) {
	repo := &repository.Container{DB: newServiceTestDB(t, &model.APIConfig{})}
	api := NewAPIConfigService(zap.NewNop(), repo, NewCryptoService("test-secret", zap.NewNop()))
	key, extra := "old-openai-key", `{"protocol":"responses","model":"custom-model"}`
	baseURL := "https://api.openai.com/v1"
	if _, err := api.Update(t.Context(), "openai", APIConfigPatch{APIKey: &key, Extra: &extra, BaseURL: &baseURL}); err != nil {
		t.Fatal(err)
	}
	extra = `{"protocol":"anthropic","model":"claude-test"}`
	if _, err := api.Update(t.Context(), "openai", APIConfigPatch{Extra: &extra}); err != nil {
		t.Fatal(err)
	}
	ai := NewAIService(&config.Config{AI: config.AIConfig{Enabled: true, APIKey: "file-openai-key"}}, zap.NewNop(), api)
	cfg := ai.resolveRuntimeConfig(t.Context())
	if cfg.Enabled || cfg.APIKey != "" || cfg.Protocol != "anthropic" || cfg.Model != "claude-test" || cfg.APIBase != "https://api.anthropic.com/v1" {
		t.Fatalf("old credentials must not follow provider switch: enabled=%v, protocol=%s, model=%s", cfg.Enabled, cfg.Protocol, cfg.Model)
	}
	extra = `{"protocol":"ollama","model":"local-test"}`
	if _, err := api.Update(t.Context(), "openai", APIConfigPatch{Extra: &extra}); err != nil {
		t.Fatal(err)
	}
	if cfg := ai.resolveRuntimeConfig(t.Context()); !cfg.Enabled || cfg.APIKey != "" || cfg.APIBase != "http://localhost:11434" {
		t.Fatal("native Ollama must work without a key at its own default endpoint")
	}
}

func TestAIUsesSelectedProviderDatabaseConfig(t *testing.T) {
	repo := &repository.Container{DB: newServiceTestDB(t, &model.APIConfig{})}
	api := NewAPIConfigService(zap.NewNop(), repo, NewCryptoService("test-secret", zap.NewNop()))
	if err := api.SeedDefaults(t.Context()); err != nil {
		t.Fatal(err)
	}
	key, extra := "native-key", `{"model":"native-model"}`
	if _, err := api.Update(t.Context(), "anthropic", APIConfigPatch{APIKey: &key, Extra: &extra}); err != nil {
		t.Fatal(err)
	}
	svc := NewAIService(&config.Config{AI: config.AIConfig{Provider: "anthropic"}}, zap.NewNop(), api)
	if cfg := svc.resolveRuntimeConfig(t.Context()); !cfg.Enabled || cfg.Provider != "anthropic" || cfg.Protocol != "anthropic" || cfg.APIBase != "https://api.anthropic.com/v1" || cfg.Model != "native-model" {
		t.Fatal("selected provider must not be overridden by the default OpenAI row")
	}
}

func TestAIRequestCancellationAndProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid key test-key"))
	}))
	defer server.Close()
	svc := NewAIService(&config.Config{AI: config.AIConfig{Enabled: true, APIKey: "test-key", APIBase: server.URL}}, zap.NewNop(), nil)
	svc.client = server.Client()
	_, err := svc.Chat(t.Context(), []ChatTurn{{Role: "user", Content: "hello"}})
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("HTTP error must report status and redact credentials: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := svc.Chat(ctx, []ChatTurn{{Role: "user", Content: "hello"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled chat = %v", err)
	}
	for _, protocol := range []string{"openai", "responses", "anthropic", "gemini", "ollama"} {
		if _, err := parseAIResponse(protocol, []byte(`{}`)); err == nil {
			t.Errorf("%s accepted an empty completion", protocol)
		}
	}
	if _, err := parseAIResponse("ollama", []byte(`{"error":"model local-test not found"}`)); err == nil || !strings.Contains(err.Error(), "model local-test not found") {
		t.Fatalf("Ollama error should explain the failed request: %v", err)
	}
}
