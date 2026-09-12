// Package service — AI integration with configurable provider protocols.
//
// AIService supports OpenAI-compatible chat, Responses, Anthropic, Gemini and
// native Ollama endpoints. It exposes smart search, recommendations and chat:
//
//   - SmartSearch:    interpret a free-form Chinese / English query and
//     return a normalised JSON intent the React UI can
//     translate into filter params.
//   - Recommend:      given a list of recently-watched titles, generate
//     a short list of "you might like…" recommendations.
//
// Native Ollama can run without an API key. Other protocols require a key.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// AIService talks to the configured model provider.
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
	return a.EnabledFor(context.Background())
}

// EnabledFor reports whether the AI integration is configured for a request.
func (a *AIService) EnabledFor(ctx context.Context) bool {
	return a.resolveRuntimeConfig(ctx).Enabled
}

// AIStatus is returned to the UI for connection-state display.
type AIStatus struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	Protocol string `json:"protocol"`
	Model    string `json:"model"`
}

// Status resolves live database-backed AI config for the UI.
func (a *AIService) Status(ctx context.Context) AIStatus {
	cfg := a.resolveRuntimeConfig(ctx)
	return AIStatus{Enabled: cfg.Enabled, Provider: cfg.Provider, Protocol: cfg.Protocol, Model: cfg.Model}
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

// complete uses the same protocol handling as conversational chat.
func (a *AIService) complete(ctx context.Context, runtime aiRuntimeConfig, system, user string) (string, error) {
	return a.completeMessages(ctx, runtime, system, []ChatTurn{{Role: "user", Content: user}}, 0.2)
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
	const system = "You are MediaStationGo's helpful media-library assistant. " +
		"Respond concisely in the user's language. " +
		"Never invent file paths or media that don't exist."
	return a.completeMessages(ctx, runtime, system, history, 0.4)
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
