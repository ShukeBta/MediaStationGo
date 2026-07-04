// Package service — AI integration (OpenAI-compatible chat completions).
//
// AIService is a thin wrapper around any OpenAI-compatible REST endpoint
// (OpenAI, DeepSeek, Qwen, Ollama, …). Today we expose two operations:
//
//   - SmartSearch:    interpret a free-form Chinese / English query and
//     return a normalised JSON intent the React UI can
//     translate into filter params.
//   - Recommend:      given a list of recently-watched titles, generate
//     a short list of "you might like…" recommendations.
//
// The service is disabled (every method returns nil) when ai.enabled is
// false or ai.api_key is empty.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// AIService talks to an OpenAI-compatible chat-completions endpoint.
type AIService struct {
	cfg       *config.Config
	log       *zap.Logger
	client    *http.Client
	apiConfig *APIConfigService
}

// NewAIService is the constructor.
func NewAIService(cfg *config.Config, log *zap.Logger, apiConfig *APIConfigService) *AIService {
	timeout := time.Duration(cfg.AI.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &AIService{
		cfg:       cfg,
		log:       log,
		apiConfig: apiConfig,
		client:    NewExternalHTTPClient(timeout),
	}
}

// Enabled reports whether the AI integration is configured.
func (a *AIService) Enabled() bool {
	return a.cfg.AI.Enabled && strings.TrimSpace(a.cfg.AI.APIKey) != ""
}

// EnabledFor reports whether the AI integration is configured for a request.
func (a *AIService) EnabledFor(ctx context.Context) bool {
	return a.resolveRuntimeConfig(ctx).Enabled
}

// AIStatus is returned to the UI for connection-state display.
type AIStatus struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// Status resolves live database-backed AI config for the UI.
func (a *AIService) Status(ctx context.Context) AIStatus {
	cfg := a.resolveRuntimeConfig(ctx)
	return AIStatus{Enabled: cfg.Enabled, Provider: cfg.Provider, Model: cfg.Model}
}

// SearchIntent is the structured output the smart search endpoint returns.
type SearchIntent struct {
	Query    string `json:"query"`
	Year     int    `json:"year,omitempty"`
	Genre    string `json:"genre,omitempty"`
	Type     string `json:"type,omitempty"` // movie / tv / anime / music
	Sort     string `json:"sort,omitempty"` // recent / rating / random
	Language string `json:"language,omitempty"`
}

// SmartSearch turns a natural-language query into a structured intent.
// Returns a best-effort intent on parse failure (raw query passes through).
func (a *AIService) SmartSearch(ctx context.Context, raw string) (*SearchIntent, error) {
	runtime := a.resolveRuntimeConfig(ctx)
	if !runtime.Enabled {
		return &SearchIntent{Query: raw}, nil
	}
	const sys = "You are a media-library search assistant. Read the user's query and " +
		"output a JSON object with the keys: query (string), year (int, optional), " +
		"genre (string, optional), type (movie|tv|anime|music, optional), sort " +
		"(recent|rating|random, optional), language (zh|en, optional). Respond with " +
		"JSON only, no commentary."
	out, err := a.complete(ctx, runtime, sys, raw)
	if err != nil {
		return &SearchIntent{Query: raw}, err
	}
	var intent SearchIntent
	if err := json.Unmarshal([]byte(out), &intent); err != nil {
		// Fallback: tolerate non-JSON output by treating the raw text as
		// the cleaned query.
		intent.Query = strings.TrimSpace(out)
	}
	if intent.Query == "" {
		intent.Query = raw
	}
	return &intent, nil
}

// Recommend builds a short comma-separated list of titles given the user's
// history. The first call is intentionally best-effort: a future iteration
// may chain media DB lookups onto each suggestion.
func (a *AIService) Recommend(ctx context.Context, history []string, max int) ([]string, error) {
	runtime := a.resolveRuntimeConfig(ctx)
	if !runtime.Enabled || len(history) == 0 {
		return nil, nil
	}
	if max <= 0 || max > 20 {
		max = 8
	}
	sys := fmt.Sprintf("You are a film / TV recommendation assistant. Reply with %d "+
		"comma-separated titles only, no commentary, in the same language as the input.", max)
	usr := "I recently watched: " + strings.Join(history, "; ")
	out, err := a.complete(ctx, runtime, sys, usr)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(out, ",")
	titles := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "\"'`")
		if p != "" {
			titles = append(titles, p)
		}
	}
	return titles, nil
}

// complete is the shared helper — POST /v1/chat/completions.
func (a *AIService) complete(ctx context.Context, runtime aiRuntimeConfig, system, user string) (string, error) {
	payload := map[string]any{
		"model":       runtime.Model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	body, _ := json.Marshal(payload)
	endpoint := strings.TrimRight(runtime.APIBase, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+runtime.APIKey)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	type choice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	var out struct {
		Choices []choice `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("ai: empty completion")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// ChatTurn is one message in a multi-turn assistant transcript.
type ChatTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Chat sends an entire transcript to the LLM. When the AI is disabled
// we return a deterministic offline reply so the assistant UI still
// has something to render.
func (a *AIService) Chat(ctx context.Context, history []ChatTurn) (string, error) {
	runtime := a.resolveRuntimeConfig(ctx)
	if !runtime.Enabled || len(history) == 0 {
		return offlineReply(history), nil
	}
	// Build a chat/completions payload preserving the history order.
	msgs := make([]map[string]string, 0, len(history)+1)
	msgs = append(msgs, map[string]string{
		"role": "system",
		"content": "You are MediaStationGo's helpful media-library assistant. " +
			"Respond concisely in the user's language. " +
			"Never invent file paths or media that don't exist.",
	})
	for _, t := range history {
		msgs = append(msgs, map[string]string{"role": t.Role, "content": t.Content})
	}
	payload := map[string]any{
		"model":       runtime.Model,
		"temperature": 0.4,
		"messages":    msgs,
	}
	body, _ := json.Marshal(payload)
	endpoint := strings.TrimRight(runtime.APIBase, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+runtime.APIKey)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	type choice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	var out struct {
		Choices []choice `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("ai: empty completion")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// offlineReply returns a deterministic stand-in response so the UI's
// chat view stays functional when the AI provider is not configured.
func offlineReply(history []ChatTurn) string {
	if len(history) == 0 {
		return "Hi — AI provider is not configured. Set up OpenAI/DeepSeek in API Configs to chat with me."
	}
	last := history[len(history)-1].Content
	if len(last) > 80 {
		last = last[:80] + "…"
	}
	return "(offline) Heard: " + last + "\n请在 API 配置中接入 LLM 后重试。"
}
