package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const subscriptionDefaultsSetting = "subscriptions.default_rules"

// SubscriptionRuleDefaults is a template for new subscriptions, never a live
// override of saved per-work rules. Identity and episode totals are excluded.
type SubscriptionRuleDefaults struct {
	PollIntervalMinutes int     `json:"poll_interval_minutes"`
	Resolution          string  `json:"resolution"`
	Quality             string  `json:"quality"`
	Effects             string  `json:"effects"`
	ReleaseGroups       string  `json:"release_groups"`
	ExcludeWords        string  `json:"exclude_words"`
	MinSeeders          int     `json:"min_seeders"`
	MaxSeeders          int     `json:"max_seeders"`
	MinSizeGB           float64 `json:"min_size_gb"`
	MaxSizeGB           float64 `json:"max_size_gb"`
	FreeOnly            bool    `json:"free_only"`
	WashEnabled         bool    `json:"wash_enabled"`
	WashPriority        string  `json:"wash_priority"`
}

func (s *SubscriptionService) DefaultRules(ctx context.Context) (SubscriptionRuleDefaults, error) {
	rules := SubscriptionRuleDefaults{PollIntervalMinutes: 180, Resolution: "best", WashPriority: "balanced"}
	if s == nil || s.repo == nil || s.repo.Setting == nil || !s.repo.DB.Migrator().HasTable(&model.Setting{}) {
		return rules, nil
	}
	raw, err := s.repo.Setting.Get(ctx, subscriptionDefaultsSetting)
	if err != nil {
		return rules, err
	}
	if raw != "" {
		err = json.Unmarshal([]byte(raw), &rules)
	}
	return rules, err
}

func (s *SubscriptionService) SaveDefaultRules(ctx context.Context, rules SubscriptionRuleDefaults) error {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return errors.New("订阅设置服务不可用")
	}
	if err := validateSubscriptionDefaultRules(rules); err != nil {
		return err
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	return s.repo.Setting.Set(ctx, subscriptionDefaultsSetting, string(raw))
}

func validateSubscriptionDefaultRules(r SubscriptionRuleDefaults) error {
	if r.PollIntervalMinutes < 5 || r.PollIntervalMinutes > 1440 {
		return errors.New("扫描频率必须为 5–1440 分钟")
	}
	if !oneOf(r.Resolution, "best", "2160p", "1080p", "720p") || !oneOf(r.Quality, "", "best", "remux", "bluray", "web-dl", "hdtv") || !oneOf(r.WashPriority, "balanced", "resolution", "quality", "effects", "seeders") {
		return errors.New("分辨率、质量或洗版优先级无效")
	}
	if r.MinSeeders < 0 || r.MaxSeeders < 0 || (r.MaxSeeders > 0 && r.MaxSeeders < r.MinSeeders) || r.MinSizeGB < 0 || r.MaxSizeGB < 0 || (r.MaxSizeGB > 0 && r.MaxSizeGB < r.MinSizeGB) {
		return errors.New("做种数和体积范围无效，最大值为 0 表示不限")
	}
	if len([]rune(r.Effects)) > 128 || len([]rune(r.ReleaseGroups)) > 255 || len([]rune(r.ExcludeWords)) > 255 {
		return errors.New("特效、发布组或排除词过长")
	}
	return nil
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

type subscriptionRuleOverridesKey struct{}

// Explicit JSON fields override the template even when false, zero or empty.
func WithSubscriptionRuleOverrides(ctx context.Context, fields map[string]json.RawMessage) context.Context {
	return context.WithValue(ctx, subscriptionRuleOverridesKey{}, fields)
}

func (s *SubscriptionService) applyDefaultRules(ctx context.Context, sub *model.Subscription) error {
	rules, err := s.DefaultRules(ctx)
	if err != nil {
		return fmt.Errorf("读取默认订阅规则失败: %w", err)
	}
	raw, _ := json.Marshal(rules)
	var defaults map[string]json.RawMessage
	_ = json.Unmarshal(raw, &defaults)
	overrides, explicit := ctx.Value(subscriptionRuleOverridesKey{}).(map[string]json.RawMessage)
	if !explicit {
		// Non-HTTP callers retain nonzero rules they already supplied.
		current, _ := json.Marshal(sub)
		_ = json.Unmarshal(current, &overrides)
		if !sub.WashEnabled {
			delete(overrides, "wash_enabled")
		}
	}
	for key := range defaults {
		if _, present := overrides[key]; present {
			delete(defaults, key)
		}
	}
	// A cloud follow has no torrent seed/free/wash semantics.
	if subscriptionUsesResourceImport(sub) || strings.HasPrefix(sub.FeedURL, "resource-import://") {
		for _, key := range []string{"min_seeders", "max_seeders", "min_size_gb", "max_size_gb", "free_only", "wash_enabled", "wash_priority"} {
			delete(defaults, key)
		}
	}
	raw, _ = json.Marshal(defaults)
	return json.Unmarshal(raw, sub)
}
