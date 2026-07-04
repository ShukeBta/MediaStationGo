package service

import (
	"strings"
	"testing"
)

func TestParseCleanupRuleCommandWithNamedWatchHours(t *testing.T) {
	rule, err := parseCleanupRuleCommand([]string{"watch_hours", "watch_3_5d_6h", "观看3到5天满6小时", "3", "5", "6"})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Type != "watch_hours" || rule.ID != "watch_3_5d_6h" || rule.Name != "观看3到5天满6小时" {
		t.Fatalf("unexpected rule identity: %+v", rule)
	}
	if !rule.Enabled || rule.WindowDaysMin != 3 || rule.WindowDaysMax != 5 || rule.MinHours != 6 {
		t.Fatalf("unexpected watch-hours rule values: %+v", rule)
	}
}

func TestFormatCleanupRulesShowsUsefulDetails(t *testing.T) {
	text := formatCleanupRules([]accountCleanupRule{{
		ID:            "login_7d",
		Type:          "recent_login",
		Name:          "七天内登录",
		Enabled:       true,
		WindowDaysMax: 7,
	}})
	for _, want := range []string{"保号规则", "<code>login_7d</code>", "七天内登录", "最近登录", "启用"} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatCleanupRules() missing %q in %q", want, text)
		}
	}
}
