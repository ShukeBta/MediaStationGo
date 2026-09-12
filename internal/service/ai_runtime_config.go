package service

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

type aiRuntimeConfig struct {
	Enabled  bool
	Provider string
	Protocol string
	APIBase  string
	APIKey   string
	Model    string
}

func (a *AIService) resolveRuntimeConfig(ctx context.Context) aiRuntimeConfig {
	out := aiRuntimeConfig{
		Enabled: a.cfg.AI.Enabled, Provider: normalizeAIProvider(a.cfg.AI.Provider),
		Protocol: normalizeAIProtocol(a.cfg.AI.Provider, a.cfg.AI.Protocol),
		APIBase:  strings.TrimSpace(a.cfg.AI.APIBase), APIKey: strings.TrimSpace(a.cfg.AI.APIKey),
		Model: strings.TrimSpace(a.cfg.AI.Model),
	}
	// File defaults belong to OpenAI, not a different selected provider.
	if out.Provider != "openai" {
		if out.APIBase == defaultAIBase("openai", "openai") {
			out.APIBase = ""
		}
		if out.Model == "gpt-4o-mini" {
			out.Model = ""
		}
	}
	if a.apiConfig != nil {
		resolved, err := a.apiConfig.Resolve(ctx, out.Provider)
		if err != nil {
			if a.log != nil {
				a.log.Warn("ai: failed to resolve provider config", zap.String("provider", out.Provider), zap.Error(err))
			}
		} else if resolved.Enabled || resolved.APIKey != "" || resolved.BaseURL != "" || resolved.Extra != "" {
			opts, parseErr := parseAIProviderOptions(resolved.Extra)
			if parseErr != nil {
				out.Enabled = false
				return out
			}
			oldProvider, oldProtocol := out.Provider, out.Protocol
			if opts.Provider != "" {
				out.Provider = normalizeAIProvider(opts.Provider)
			}
			if opts.Protocol != "" {
				out.Protocol = normalizeAIProtocol(out.Provider, opts.Protocol)
				if opts.Provider == "" && aiCredentialFamily(out.Protocol) != "openai" {
					out.Provider = out.Protocol
				}
			} else if opts.Provider != "" {
				out.Protocol = normalizeAIProtocol(out.Provider, "")
			}
			if out.Provider != oldProvider || aiCredentialFamily(out.Protocol) != aiCredentialFamily(oldProtocol) {
				out.APIBase, out.APIKey, out.Model = "", "", ""
			}
			if resolved.BaseURL != "" {
				out.APIBase = strings.TrimSpace(resolved.BaseURL)
			}
			if resolved.APIKey != "" {
				out.APIKey = strings.TrimSpace(resolved.APIKey)
			}
			if opts.Model != "" {
				out.Model = strings.TrimSpace(opts.Model)
			}
			out.Enabled = resolved.Enabled
		}
	}
	if out.APIBase == "" {
		out.APIBase = defaultAIBase(out.Provider, out.Protocol)
	}
	if out.Model == "" {
		switch out.Provider {
		case "openai":
			out.Model = "gpt-4o-mini"
		case "deepseek":
			out.Model = "deepseek-chat"
		case "qwen":
			out.Model = "qwen-plus"
		}
	}
	out.Enabled = out.Enabled && (out.APIKey != "" || out.Protocol == "ollama")
	return out
}
