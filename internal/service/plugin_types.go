package service

import (
	"context"
	"errors"
	"time"
)

var (
	ErrPluginNotFound      = errors.New("plugin not found")
	ErrPluginDisabled      = errors.New("plugin is disabled")
	ErrPluginBusy          = errors.New("plugin is running")
	ErrPluginInvalidConfig = errors.New("invalid plugin configuration")
)

type PluginConfigField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Default     bool   `json:"default"`
}

type PluginManifest struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Version      string              `json:"version"`
	Author       string              `json:"author"`
	Capabilities []string            `json:"capabilities"`
	ConfigFields []PluginConfigField `json:"config_fields"`
}

type PluginMetric struct {
	Label string `json:"label"`
	Value int64  `json:"value"`
}

type PluginRunResult struct {
	Summary     string         `json:"summary"`
	Metrics     []PluginMetric `json:"metrics"`
	CompletedAt time.Time      `json:"completed_at"`
	DurationMS  int64          `json:"duration_ms"`
}

type PluginInfo struct {
	PluginManifest
	Enabled   bool             `json:"enabled"`
	Config    map[string]any   `json:"config"`
	Status    string           `json:"status"`
	LastRun   *PluginRunResult `json:"last_run"`
	LastError string           `json:"last_error"`
}

type PluginUpdate struct {
	Enabled *bool          `json:"enabled"`
	Config  map[string]any `json:"config"`
}

// Plugin implementations are compiled into the application and must honor ctx.
// Config is validated and copied before Run. Plugins must not spawn unowned work.
type Plugin interface {
	Manifest() PluginManifest
	ValidateConfig(map[string]any) error
	Run(context.Context, map[string]any) (PluginRunResult, error)
}

type pluginSettings interface {
	Get(context.Context, string) (string, error)
	Set(context.Context, string, string) error
}
