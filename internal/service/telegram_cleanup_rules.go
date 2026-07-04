package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func (s *TelegramBotService) currentCleanupRules(ctx context.Context) []accountCleanupRule {
	cfg := loadBotConfig(ctx, s.repo)
	return cfg.AccountCleanupRules
}

func (s *TelegramBotService) saveCleanupRules(ctx context.Context, rules []accountCleanupRule) error {
	raw, err := json.Marshal(normalizeCleanupRules(rules))
	if err != nil {
		return err
	}
	return s.repo.Setting.Set(ctx, SettingAccountCleanupRules, string(raw))
}

func parseCommandBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "1", "yes", "enable", "enabled", "开启", "开":
		return true, true
	case "off", "false", "0", "no", "disable", "disabled", "关闭", "关":
		return false, true
	default:
		return false, false
	}
}

func parseCleanupRuleCommand(args []string) (accountCleanupRule, error) {
	if len(args) < 2 {
		return accountCleanupRule{}, fmt.Errorf("新增规则参数不足")
	}
	rule := accountCleanupRule{
		Type:          strings.ToLower(strings.TrimSpace(args[0])),
		ID:            strings.TrimSpace(args[1]),
		Enabled:       true,
		WindowDaysMin: 3,
		WindowDaysMax: 5,
		MinHours:      6,
		MinCount:      1,
	}
	switch rule.Type {
	case "watch_hours":
		name, values := cleanupRuleNameAndValues(args[2:], 3)
		rule.Name = name
		if len(values) >= 3 {
			rule.WindowDaysMin, _ = strconv.Atoi(values[0])
			rule.WindowDaysMax, _ = strconv.Atoi(values[1])
			rule.MinHours, _ = strconv.ParseFloat(values[2], 64)
			if rule.Name == "" {
				rule.Name = fmt.Sprintf("%d~%d 天观看满 %s 小时", rule.WindowDaysMin, rule.WindowDaysMax, formatRuleHours(rule.MinHours))
			}
		}
	case "recent_login":
		name, values := cleanupRuleNameAndValues(args[2:], 1)
		rule.Name = name
		if len(values) >= 1 {
			rule.WindowDaysMax, _ = strconv.Atoi(values[0])
			if rule.Name == "" {
				rule.Name = fmt.Sprintf("%d 天内登录", rule.WindowDaysMax)
			}
		}
	case "signin_streak", "account_age_grace":
		name, values := cleanupRuleNameAndValues(args[2:], 1)
		rule.Name = name
		if len(values) >= 1 {
			rule.MinCount, _ = strconv.Atoi(values[0])
			if rule.Name == "" {
				if rule.Type == "signin_streak" {
					rule.Name = fmt.Sprintf("连续签到 %d 天", rule.MinCount)
				} else {
					rule.Name = fmt.Sprintf("新号宽限 %d 天", rule.MinCount)
				}
			}
		}
	default:
		return accountCleanupRule{}, fmt.Errorf("不支持的规则类型：%s", rule.Type)
	}
	normalized := normalizeCleanupRules([]accountCleanupRule{rule})
	if len(normalized) == 0 {
		return accountCleanupRule{}, fmt.Errorf("规则无效")
	}
	return normalized[0], nil
}

func cleanupRuleNameAndValues(args []string, numericCount int) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) >= numericCount && cleanupRuleValuesAreNumeric(args[:numericCount]) {
		return "", args
	}
	return strings.TrimSpace(args[0]), args[1:]
}

func cleanupRuleValuesAreNumeric(values []string) bool {
	for _, value := range values {
		if _, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err != nil {
			return false
		}
	}
	return true
}

func formatCleanupRules(rules []accountCleanupRule) string {
	if len(rules) == 0 {
		return "<b>保号规则</b>\n\n暂无规则。"
	}
	var sb strings.Builder
	sb.WriteString("<b>保号规则</b>\n")
	for i, r := range rules {
		state := map[bool]string{true: "启用", false: "停用"}[r.Enabled]
		detail := cleanupRuleDetail(r)
		parts := []string{
			fmt.Sprintf("\n%d. <code>%s</code>", i+1, r.ID),
		}
		if shouldShowCleanupRuleName(r, detail) {
			parts = append(parts, r.Name)
		}
		parts = append(parts, cleanupRuleTypeLabel(r.Type), state)
		if detail != "" {
			parts = append(parts, detail)
		}
		sb.WriteString(strings.Join(parts, " · "))
	}
	return sb.String()
}

func shouldShowCleanupRuleName(r accountCleanupRule, detail string) bool {
	name := strings.TrimSpace(r.Name)
	if name == "" || strings.EqualFold(name, r.ID) {
		return false
	}
	if detail != "" && strings.EqualFold(name, detail) {
		return false
	}
	return true
}

func cleanupRuleDetail(r accountCleanupRule) string {
	switch r.Type {
	case "watch_hours":
		return fmt.Sprintf("%d~%d 天 %s 小时", r.WindowDaysMin, r.WindowDaysMax, formatRuleHours(r.MinHours))
	case "recent_login":
		return fmt.Sprintf("%d 天内登录", r.WindowDaysMax)
	case "signin_streak":
		return fmt.Sprintf("连续签到 %d 天", r.MinCount)
	case "account_age_grace":
		return fmt.Sprintf("新号宽限 %d 天", r.MinCount)
	default:
		return ""
	}
}

func formatRuleHours(hours float64) string {
	if hours == float64(int(hours)) {
		return strconv.Itoa(int(hours))
	}
	return fmt.Sprintf("%.1f", hours)
}

func cleanupRuleTypeLabel(t string) string {
	switch t {
	case "watch_hours":
		return "观看时长"
	case "recent_login":
		return "最近登录"
	case "signin_streak":
		return "连续签到"
	case "account_age_grace":
		return "新号宽限"
	default:
		return t
	}
}

func cleanupRuleHelp() string {
	return "<b>Mgo 保号规则命令</b>\n\n" +
		"<code>/cleanup_rule list</code> — 查看规则\n" +
		"<code>/cleanup_rule add watch_hours watch_3_5d_6h 观看3到5天满6小时 3 5 6</code>\n" +
		"<code>/cleanup_rule add recent_login login_7d 七天内登录 7</code>\n" +
		"<code>/cleanup_rule add signin_streak sign_3 连续签到3天 3</code>\n" +
		"<code>/cleanup_rule add account_age_grace new_7d 新号宽限7天 7</code>\n" +
		"<code>/cleanup_rule edit 规则类型 规则ID 名称 参数...</code> — 修改同 ID 规则\n" +
		"<code>/cleanup_rule 修改 规则类型 规则ID 名称 参数...</code> — 中文修改入口\n" +
		"<code>/cleanup_rule enable 规则ID</code> / <code>disable 规则ID</code>\n" +
		"<code>/cleanup_rule del 规则ID</code>\n\n" +
		"保号模式固定为：满足任意一条启用规则即保留；全部不满足才会清理。"
}

func onOff(b bool) string {
	return map[bool]string{true: "已开启", false: "已关闭"}[b]
}

func toggleLabel(name string, enabled bool) string {
	if enabled {
		return "关闭" + name
	}
	return "开启" + name
}

func cleanupModeLabel(mode string) string {
	return "满足任意一条"
}

func countEnabledCleanupRules(rules []accountCleanupRule) int {
	n := 0
	for _, r := range rules {
		if r.Enabled {
			n++
		}
	}
	return n
}
