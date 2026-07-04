package service

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// Bot / 设备管控相关的设置键。全部存储在 settings 表，由管理员通过
// Telegram Bot 命令调整。带安全默认值：所有自动处理策略默认关闭。
const (
	// 开放注册（开注名额）。
	SettingOpenRegEnabled = "telegram.openreg_enabled" // 是否开放注册
	SettingOpenRegLimit   = "telegram.openreg_limit"   // 本轮开注名额上限（0=不限）
	SettingOpenRegUsed    = "telegram.openreg_used"    // 本轮已用名额

	// 防共享（警告制：并发播放 / 登录终端 / 设备指纹）。
	SettingAntiShareEnabled  = "device.antishare_enabled"   // 总开关（默认关）
	SettingMaxConcurrentPlay = "device.max_concurrent_play" // 最大并发播放设备
	SettingMaxLoggedClients  = "device.max_logged_clients"  // 最大同时登录终端
	SettingWarnThreshold     = "device.warn_threshold"      // 警告几次后禁用
	SettingPlayWindowSeconds = "device.play_window_seconds" // 并发播放判定窗口（秒）
	SettingClientActiveDays  = "device.client_active_days"  // 登录设备活跃天数窗口

	// Mgo 保号规则。规则默认关闭；开启后满足任意一条保号规则即保留，
	// 所有启用规则都不满足才会进入清理候选。
	SettingAccountCleanupEnabled       = "device.account_cleanup_enabled"
	SettingAccountCleanupKeepMode      = "device.account_cleanup_keep_mode"      // legacy; normalized to any
	SettingAccountCleanupRequiredCount = "device.account_cleanup_required_count" // legacy; normalized to 1
	SettingAccountCleanupRules         = "device.account_cleanup_rules"          // JSON []accountCleanupRule
	SettingProtectedUserIDs            = "device.protected_user_ids"             // comma separated user IDs; Mgo /prouser
)

// botConfig 是设备管控的已解析配置（含默认值）。
type botConfig struct {
	AntiShareEnabled  bool
	MaxConcurrentPlay int
	MaxLoggedClients  int
	WarnThreshold     int
	PlayWindowSeconds int
	ClientActiveDays  int

	AccountCleanupEnabled       bool
	AccountCleanupKeepMode      string
	AccountCleanupRequiredCount int
	AccountCleanupRules         []accountCleanupRule
}

// accountCleanupRule is one admin-defined "保号" condition. A user is deleted
// only when the cleanup policy is enabled and the user does not satisfy any
// enabled keep rule.
//
// Supported types:
//   - watch_hours: watched hours in a random [min,max] day window >= MinHours
//   - recent_login: LastLoginAt is within WindowDaysMax days
//   - signin_streak: current sign-in streak >= MinCount
//   - account_age_grace: account age <= MinCount days (new-user grace)
type accountCleanupRule struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Enabled       bool    `json:"enabled"`
	WindowDaysMin int     `json:"window_days_min,omitempty"`
	WindowDaysMax int     `json:"window_days_max,omitempty"`
	MinHours      float64 `json:"min_hours,omitempty"`
	MinCount      int     `json:"min_count,omitempty"`
}

// defaultBotConfig returns the safe defaults requested by the operator.
func defaultBotConfig() botConfig {
	return botConfig{
		AntiShareEnabled:  false, // 防共享默认关闭，需管理员显式开启
		MaxConcurrentPlay: 3,
		MaxLoggedClients:  3,
		WarnThreshold:     2, // 两次警告后再犯禁用
		PlayWindowSeconds: 90,
		ClientActiveDays:  30,

		AccountCleanupEnabled:       false,
		AccountCleanupKeepMode:      "any",
		AccountCleanupRequiredCount: 1,
		AccountCleanupRules:         nil,
	}
}

// loadBotConfig reads the device-management configuration from settings,
// falling back to safe defaults for any missing/invalid key.
func loadBotConfig(ctx context.Context, repo *repository.Container) botConfig {
	cfg := defaultBotConfig()
	get := func(key string) string {
		v, err := repo.Setting.Get(ctx, key)
		if err != nil {
			return ""
		}
		return v
	}
	cfg.AntiShareEnabled = parseBoolSetting(get(SettingAntiShareEnabled), cfg.AntiShareEnabled)
	cfg.MaxConcurrentPlay = parseIntSettingDefault(get(SettingMaxConcurrentPlay), cfg.MaxConcurrentPlay)
	cfg.MaxLoggedClients = parseIntSettingDefault(get(SettingMaxLoggedClients), cfg.MaxLoggedClients)
	cfg.WarnThreshold = parseIntSettingDefault(get(SettingWarnThreshold), cfg.WarnThreshold)
	cfg.PlayWindowSeconds = parseIntSettingDefault(get(SettingPlayWindowSeconds), cfg.PlayWindowSeconds)
	cfg.ClientActiveDays = parseIntSettingDefault(get(SettingClientActiveDays), cfg.ClientActiveDays)
	cfg.AccountCleanupEnabled = parseBoolSetting(get(SettingAccountCleanupEnabled), cfg.AccountCleanupEnabled)
	// Historical builds allowed all/count modes, but the Mgo policy is now
	// explicitly "any": matching one keep rule is enough to avoid deletion.
	cfg.AccountCleanupKeepMode = "any"
	cfg.AccountCleanupRequiredCount = 1
	if raw := strings.TrimSpace(get(SettingAccountCleanupRules)); raw != "" {
		var rules []accountCleanupRule
		if err := json.Unmarshal([]byte(raw), &rules); err == nil {
			cfg.AccountCleanupRules = normalizeCleanupRules(rules)
		}
	}
	if cfg.AccountCleanupRequiredCount < 1 {
		cfg.AccountCleanupRequiredCount = 1
	}
	return cfg
}

// ProtectedUserIDSet returns the explicit Mgo-compatible protected user list.
// Admins and the default admin are protected separately by UserIsProtectedAccount.
func ProtectedUserIDSet(ctx context.Context, repo *repository.Container) map[string]struct{} {
	out := make(map[string]struct{})
	if repo == nil || repo.Setting == nil {
		return out
	}
	raw, err := repo.Setting.Get(ctx, SettingProtectedUserIDs)
	if err != nil {
		return out
	}
	for _, value := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '，' || r == '；' || r == ' ' || r == '\n' || r == '\t'
	}) {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

// SaveProtectedUserIDSet persists the explicit protected user list.
func SaveProtectedUserIDSet(ctx context.Context, repo *repository.Container, ids map[string]struct{}) error {
	if repo == nil || repo.Setting == nil {
		return nil
	}
	values := make([]string, 0, len(ids))
	for id := range ids {
		if strings.TrimSpace(id) != "" {
			values = append(values, id)
		}
	}
	sort.Strings(values)
	return repo.Setting.Set(ctx, SettingProtectedUserIDs, strings.Join(values, ","))
}

// UserIsProtectedAccount reports whether a user is protected from destructive
// Bot/device-policy operations.
func UserIsProtectedAccount(ctx context.Context, repo *repository.Container, user *model.User) bool {
	if repo == nil || user == nil {
		return true
	}
	if user.Role == "admin" {
		return true
	}
	if first, err := repo.User.FirstAdmin(ctx); err == nil && first != nil && first.ID == user.ID {
		return true
	}
	_, ok := ProtectedUserIDSet(ctx, repo)[user.ID]
	return ok
}

// parseIntSettingDefault parses an int setting, returning fallback on error.
func parseIntSettingDefault(value string, fallback int) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
