package service

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	cleanupRuleWindowDaysRE = regexp.MustCompile(`(?i)(?:^|[_-])(\d+)[_-](\d+)\s*d(?:$|[_-])`)
	cleanupRuleSingleDayRE  = regexp.MustCompile(`(?i)(?:^|[_-])(\d+)\s*d(?:$|[_-])`)
	cleanupRuleHoursRE      = regexp.MustCompile(`(?i)(?:^|[_-])(\d+(?:\.\d+)?)\s*h(?:$|[_-])`)
	cleanupRuleNumberRE     = regexp.MustCompile(`(?i)(?:^|[_-])(\d+)(?:$|[_-])`)
)

func normalizeCleanupKeepMode(value, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "any", "all", "count":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		if fallback == "" {
			return "any"
		}
		return fallback
	}
}

func normalizeCleanupRules(rules []accountCleanupRule) []accountCleanupRule {
	out := make([]accountCleanupRule, 0, len(rules))
	for _, r := range rules {
		r.Type = strings.ToLower(strings.TrimSpace(r.Type))
		r.ID = strings.TrimSpace(r.ID)
		r.Name = strings.TrimSpace(r.Name)
		if r.ID == "" {
			r.ID = r.Type
		}
		if r.Name == "" {
			r.Name = r.ID
		}
		if shouldInferCleanupRuleValues(r) {
			inferCleanupRuleValuesFromID(&r)
		}
		if r.WindowDaysMin < 1 {
			r.WindowDaysMin = 1
		}
		if r.WindowDaysMax < r.WindowDaysMin {
			r.WindowDaysMax = r.WindowDaysMin
		}
		if r.MinCount < 0 {
			r.MinCount = 0
		}
		if r.MinHours < 0 {
			r.MinHours = 0
		}
		switch r.Type {
		case "watch_hours", "recent_login", "signin_streak", "account_age_grace":
			out = append(out, r)
		}
	}
	return out
}

func shouldInferCleanupRuleValues(rule accountCleanupRule) bool {
	name := strings.TrimSpace(rule.Name)
	return name == "" || strings.EqualFold(name, strings.TrimSpace(rule.ID))
}

func inferCleanupRuleValuesFromID(rule *accountCleanupRule) {
	if rule == nil {
		return
	}
	key := strings.ToLower(strings.TrimSpace(rule.ID + "_" + rule.Name))
	switch rule.Type {
	case "watch_hours":
		if minDays, maxDays := cleanupRuleWindowDays(key); minDays > 0 && maxDays > 0 {
			rule.WindowDaysMin = minDays
			rule.WindowDaysMax = maxDays
		} else if days := cleanupRuleSingleDay(key); days > 0 {
			rule.WindowDaysMin = days
			rule.WindowDaysMax = days
		}
		if hours := cleanupRuleHours(key); hours > 0 {
			rule.MinHours = hours
		}
	case "recent_login":
		if days := cleanupRuleSingleDay(key); days > 0 {
			rule.WindowDaysMax = days
		}
	case "account_age_grace":
		if days := cleanupRuleSingleDay(key); days > 0 {
			rule.MinCount = days
		}
	case "signin_streak":
		if n := cleanupRuleTrailingNumber(key); n > 0 {
			rule.MinCount = n
		}
	}
}

func cleanupRuleWindowDays(value string) (int, int) {
	if m := cleanupRuleWindowDaysRE.FindStringSubmatch(value); len(m) >= 3 {
		minDays, _ := strconv.Atoi(m[1])
		maxDays, _ := strconv.Atoi(m[2])
		if maxDays < minDays {
			maxDays = minDays
		}
		return minDays, maxDays
	}
	return 0, 0
}

func cleanupRuleSingleDay(value string) int {
	if m := cleanupRuleSingleDayRE.FindStringSubmatch(value); len(m) >= 2 {
		days, _ := strconv.Atoi(m[1])
		return days
	}
	return 0
}

func cleanupRuleHours(value string) float64 {
	if m := cleanupRuleHoursRE.FindStringSubmatch(value); len(m) >= 2 {
		hours, _ := strconv.ParseFloat(m[1], 64)
		return hours
	}
	return 0
}

func cleanupRuleTrailingNumber(value string) int {
	matches := cleanupRuleNumberRE.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return 0
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(last[1])
	return n
}
