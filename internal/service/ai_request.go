package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (a *AIService) completeMessages(ctx context.Context, cfg aiRuntimeConfig, system string, history []ChatTurn, temperature float64) (string, error) {
	req, err := buildAIRequest(ctx, cfg, system, history, temperature)
	if err != nil {
		return "", err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	const maxResponse = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxResponse {
		return "", errors.New("ai: response exceeds size limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if cfg.APIKey != "" {
			message = strings.ReplaceAll(message, cfg.APIKey, "[redacted]")
		}
		if len(message) > 1024 {
			message = message[:1024]
		}
		return "", fmt.Errorf("ai %s HTTP %d: %s", cfg.Protocol, resp.StatusCode, message)
	}
	text, err := parseAIResponse(cfg.Protocol, body)
	if err != nil && cfg.APIKey != "" {
		return "", errors.New(strings.ReplaceAll(err.Error(), cfg.APIKey, "[redacted]"))
	}
	return text, err
}

func buildAIRequest(ctx context.Context, cfg aiRuntimeConfig, system string, history []ChatTurn, temperature float64) (*http.Request, error) {
	if !supportedAIProtocol(cfg.Protocol) {
		return nil, fmt.Errorf("unsupported AI protocol: %s", cfg.Protocol)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("AI model is required; configure the model ID in API settings")
	}
	messages := make([]ChatTurn, 0, len(history))
	for _, turn := range history {
		if turn.Role != "user" && turn.Role != "assistant" {
			return nil, fmt.Errorf("unsupported AI message role: %s", turn.Role)
		}
		if strings.TrimSpace(turn.Content) != "" {
			messages = append(messages, turn)
		}
	}
	if len(messages) == 0 {
		return nil, errors.New("AI messages are empty")
	}
	withSystem := append([]ChatTurn{{Role: "system", Content: system}}, messages...)
	payload := map[string]any{"model": cfg.Model}
	switch cfg.Protocol {
	case "openai":
		payload["messages"], payload["temperature"] = withSystem, temperature
	case "responses":
		payload["instructions"], payload["input"], payload["store"] = system, messages, false
	case "anthropic":
		payload["system"], payload["messages"] = system, messages
		payload["max_tokens"], payload["temperature"] = 1024, temperature
	case "gemini":
		delete(payload, "model")
		contents := make([]map[string]any, 0, len(messages))
		for _, turn := range messages {
			role := turn.Role
			if role == "assistant" {
				role = "model"
			}
			contents = append(contents, map[string]any{"role": role, "parts": []map[string]string{{"text": turn.Content}}})
		}
		payload["contents"] = contents
		payload["systemInstruction"] = map[string]any{"parts": []map[string]string{{"text": system}}}
		payload["generationConfig"] = map[string]any{"temperature": temperature}
	case "ollama":
		payload["messages"], payload["stream"] = withSystem, false
		payload["options"] = map[string]any{"temperature": temperature}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint, err := aiEndpoint(cfg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	switch cfg.Protocol {
	case "anthropic":
		req.Header.Set("x-api-key", cfg.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case "gemini":
		req.Header.Set("x-goog-api-key", cfg.APIKey)
	default:
		if cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}
	}
	return req, nil
}

func aiEndpoint(cfg aiRuntimeConfig) (string, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.APIBase))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errors.New("AI Base URL must be an HTTP(S) URL without embedded credentials")
	}
	path := strings.TrimRight(u.Path, "/")
	suffix, version := "/chat/completions", "/v1"
	switch cfg.Protocol {
	case "responses":
		suffix = "/responses"
	case "anthropic":
		suffix = "/messages"
	case "ollama":
		suffix, version = "/chat", "/api"
	case "gemini":
		if strings.HasSuffix(path, ":generateContent") {
			u.Path, u.RawPath, u.Fragment = path, "", ""
			return u.String(), nil
		}
		model := strings.TrimPrefix(cfg.Model, "models/")
		if strings.ContainsAny(model, "/\\?#") {
			return "", errors.New("invalid Gemini model ID")
		}
		suffix, version = "/models/"+model+":generateContent", "/v1beta"
	}
	if !strings.HasSuffix(path, suffix) {
		if path == "" {
			path = version
		}
		path += suffix
	}
	u.Path, u.RawPath, u.Fragment = path, "", ""
	return u.String(), nil
}
