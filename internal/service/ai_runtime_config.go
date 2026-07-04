package service

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

type aiRuntimeConfig struct {
	Enabled  bool
	Provider string
	APIBase  string
	APIKey   string
	Model    string
}

func (a *AIService) resolveRuntimeConfig(ctx context.Context) aiRuntimeConfig {
	out := aiRuntimeConfig{
		Enabled:  a.cfg.AI.Enabled && strings.TrimSpace(a.cfg.AI.APIKey) != "",
		Provider: strings.TrimSpace(a.cfg.AI.Provider),
		APIBase:  strings.TrimSpace(a.cfg.AI.APIBase),
		APIKey:   strings.TrimSpace(a.cfg.AI.APIKey),
		Model:    strings.TrimSpace(a.cfg.AI.Model),
	}
	if out.Provider == "" {
		out.Provider = "openai"
	}
	if out.APIBase == "" {
		out.APIBase = "https://api.openai.com/v1"
	}
	if out.Model == "" {
		out.Model = "gpt-4o-mini"
	}

	if a.apiConfig != nil {
		resolved, err := a.apiConfig.Resolve(ctx, "openai")
		if err != nil {
			if a.log != nil {
				a.log.Warn("ai: failed to resolve openai api config", zap.Error(err))
			}
			return out
		}
		if resolved.BaseURL != "" {
			out.APIBase = strings.TrimSpace(resolved.BaseURL)
		}
		if resolved.APIKey != "" {
			out.APIKey = strings.TrimSpace(resolved.APIKey)
		}
		if resolved.Enabled && out.APIKey != "" {
			out.Enabled = true
			out.Provider = "openai"
			return out
		}
		if !resolved.Enabled && (resolved.APIKey != "" || resolved.BaseURL != "" || resolved.Extra != "") {
			out.Enabled = false
		}
	}
	return out
}
