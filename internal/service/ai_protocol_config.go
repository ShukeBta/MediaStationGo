package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

type aiProviderOptions struct {
	Provider string `json:"provider,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Model    string `json:"model,omitempty"`
}

func parseAIProviderOptions(raw string) (aiProviderOptions, error) {
	var opts aiProviderOptions
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &opts); err != nil {
			return opts, fmt.Errorf("AI options must be a JSON object: %w", err)
		}
	}
	if opts.Protocol != "" && !supportedAIProtocol(normalizeAIProtocol(opts.Provider, opts.Protocol)) {
		return opts, fmt.Errorf("unsupported AI protocol: %s", opts.Protocol)
	}
	return opts, nil
}

func normalizeAIProvider(provider string) string {
	switch provider = strings.ToLower(strings.TrimSpace(provider)); provider {
	case "", "openai-compatible", "responses", "openai_responses":
		return "openai"
	case "claude":
		return "anthropic"
	case "google":
		return "gemini"
	default:
		return provider
	}
}

func normalizeAIProtocol(provider, protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	implicit := protocol == ""
	if protocol == "" {
		protocol = strings.ToLower(strings.TrimSpace(provider))
	}
	switch protocol {
	case "", "openai", "openai-compatible", "chat_completions", "deepseek", "qwen":
		return "openai"
	case "responses", "openai_responses":
		return "responses"
	case "anthropic", "claude":
		return "anthropic"
	case "gemini", "google":
		return "gemini"
	default:
		if implicit && protocol != "ollama" {
			return "openai"
		}
		return protocol
	}
}

func supportedAIProtocol(protocol string) bool {
	switch protocol {
	case "openai", "responses", "anthropic", "gemini", "ollama":
		return true
	}
	return false
}

func isAIConfigProvider(provider string) bool {
	switch normalizeAIProvider(provider) {
	case "openai", "deepseek", "qwen", "anthropic", "gemini", "ollama":
		return true
	}
	return false
}

func aiCredentialFamily(protocol string) string {
	if protocol == "responses" {
		return "openai"
	}
	return protocol
}

func defaultAIBase(provider, protocol string) string {
	switch protocol {
	case "anthropic":
		return "https://api.anthropic.com/v1"
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta"
	case "ollama":
		return "http://localhost:11434"
	}
	switch provider {
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "qwen":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	return "https://api.openai.com/v1"
}
