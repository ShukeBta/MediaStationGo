package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type pluginState struct {
	Enabled bool           `json:"enabled"`
	Config  map[string]any `json:"config"`
}

type pluginEntry struct {
	mu        sync.Mutex
	plugin    Plugin
	manifest  PluginManifest
	running   bool
	lastRun   *PluginRunResult
	lastError string
}

// PluginService stores configuration durably; execution records last until restart.
// Registry entries are added during startup before the manager is published.
type PluginService struct {
	settings pluginSettings
	entries  map[string]*pluginEntry
}

func NewPluginService(repos *repository.Container) *PluginService {
	s := &PluginService{settings: repos.Setting, entries: make(map[string]*pluginEntry)}
	// A static, validated manifest: registration failure is a programming error.
	if err := s.Register(&librarySummaryPlugin{db: repos.DB}); err != nil {
		panic(err)
	}
	return s
}

var pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// Register must be called only while constructing the service, before serving requests.
func (s *PluginService) Register(plugin Plugin) error {
	manifest := plugin.Manifest()
	if !pluginIDPattern.MatchString(manifest.ID) || manifest.Name == "" || manifest.Version == "" {
		return fmt.Errorf("invalid plugin manifest")
	}
	if _, exists := s.entries[manifest.ID]; exists {
		return fmt.Errorf("duplicate plugin %s", manifest.ID)
	}
	defaults := make(map[string]any)
	for _, field := range manifest.ConfigFields {
		if field.Type != "boolean" || field.Key == "" {
			return fmt.Errorf("invalid plugin config field")
		}
		if _, exists := defaults[field.Key]; exists {
			return fmt.Errorf("duplicate plugin config field")
		}
		defaults[field.Key] = field.Default
	}
	if err := plugin.ValidateConfig(defaults); err != nil {
		return err
	}
	manifest.Capabilities = append([]string{}, manifest.Capabilities...)
	manifest.ConfigFields = append([]PluginConfigField{}, manifest.ConfigFields...)
	s.entries[manifest.ID] = &pluginEntry{plugin: plugin, manifest: manifest}
	return nil
}

func (s *PluginService) List(ctx context.Context) ([]PluginInfo, error) {
	ids := make([]string, 0, len(s.entries))
	for id := range s.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	items := make([]PluginInfo, 0, len(ids))
	for _, id := range ids {
		entry := s.entries[id]
		entry.mu.Lock()
		state, err := s.load(ctx, entry)
		info := entry.info(state)
		entry.mu.Unlock()
		if err != nil {
			return nil, err
		}
		items = append(items, info)
	}
	return items, nil
}

func (s *PluginService) Update(ctx context.Context, id string, update PluginUpdate) (PluginInfo, error) {
	entry, ok := s.entries[id]
	if !ok {
		return PluginInfo{}, ErrPluginNotFound
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.running {
		return PluginInfo{}, ErrPluginBusy
	}
	if update.Enabled == nil && update.Config == nil {
		return PluginInfo{}, ErrPluginInvalidConfig
	}
	state, err := s.load(ctx, entry)
	if err != nil {
		return PluginInfo{}, err
	}
	if update.Config != nil {
		state.Config = clonePluginConfig(update.Config)
		if err := validatePluginConfig(entry, state.Config); err != nil {
			return PluginInfo{}, err
		}
	}
	if update.Enabled != nil {
		state.Enabled = *update.Enabled
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return PluginInfo{}, fmt.Errorf("%w: %v", ErrPluginInvalidConfig, err)
	}
	if err := s.settings.Set(ctx, "plugins."+id, string(encoded)); err != nil {
		return PluginInfo{}, err
	}
	return entry.info(state), nil
}

func (s *PluginService) load(ctx context.Context, entry *pluginEntry) (pluginState, error) {
	state := pluginState{Config: make(map[string]any)}
	for _, field := range entry.manifest.ConfigFields {
		state.Config[field.Key] = field.Default
	}
	value, err := s.settings.Get(ctx, "plugins."+entry.manifest.ID)
	if err != nil {
		return state, err
	}
	if value != "" {
		if err := json.Unmarshal([]byte(value), &state); err != nil {
			return state, fmt.Errorf("plugin settings are invalid: %w", err)
		}
	}
	if err := validatePluginConfig(entry, state.Config); err != nil {
		return state, err
	}
	return state, nil
}

func validatePluginConfig(entry *pluginEntry, config map[string]any) error {
	known := make(map[string]bool)
	for _, field := range entry.manifest.ConfigFields {
		known[field.Key] = true
		if _, ok := config[field.Key].(bool); !ok {
			return fmt.Errorf("%w: %s 必须是布尔值", ErrPluginInvalidConfig, field.Label)
		}
	}
	for key := range config {
		if !known[key] {
			return fmt.Errorf("%w: 未知配置项 %s", ErrPluginInvalidConfig, key)
		}
	}
	if err := entry.plugin.ValidateConfig(config); err != nil {
		return fmt.Errorf("%w: %v", ErrPluginInvalidConfig, err)
	}
	return nil
}

func clonePluginConfig(config map[string]any) map[string]any {
	clone := make(map[string]any, len(config))
	for key, value := range config {
		clone[key] = value
	}
	return clone
}

func (entry *pluginEntry) info(state pluginState) PluginInfo {
	status := "disabled"
	if state.Enabled {
		status = "ready"
		if entry.lastError != "" {
			status = "error"
		}
	}
	if entry.running {
		status = "running"
	}
	manifest := entry.manifest
	manifest.Capabilities = append([]string{}, manifest.Capabilities...)
	manifest.ConfigFields = append([]PluginConfigField{}, manifest.ConfigFields...)
	return PluginInfo{PluginManifest: manifest, Enabled: state.Enabled, Config: clonePluginConfig(state.Config),
		Status: status, LastRun: clonePluginResult(entry.lastRun), LastError: entry.lastError}
}

func clonePluginResult(result *PluginRunResult) *PluginRunResult {
	if result == nil {
		return nil
	}
	clone := *result
	clone.Metrics = append([]PluginMetric{}, result.Metrics...)
	return &clone
}
