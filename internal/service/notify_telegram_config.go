package service

import "strings"

func telegramTargetChatIDs(cfg map[string]string) []string {
	seen := map[string]bool{}
	targets := []string{}
	for _, key := range []string{"group_chat_id", "channel_chat_id"} {
		chatID := strings.TrimSpace(cfg[key])
		if chatID == "" || seen[chatID] {
			continue
		}
		seen[chatID] = true
		targets = append(targets, chatID)
	}
	if len(targets) == 0 {
		chatID := strings.TrimSpace(cfg["chat_id"])
		if strings.HasPrefix(chatID, "-") {
			targets = append(targets, chatID)
		} else if chatID != "" && strings.TrimSpace(cfg["admin_user_ids"]) == "" {
			targets = append(targets, chatID)
		}
	}
	if len(targets) == 0 {
		for _, userID := range telegramConfiguredUserIDs(cfg["admin_user_ids"]) {
			if seen[userID] {
				continue
			}
			seen[userID] = true
			targets = append(targets, userID)
		}
	}
	return targets
}

func telegramConfiguredUserIDs(raw string) []string {
	out := []string{}
	for _, value := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '，' || r == ' ' || r == '\n' || r == '\t'
	}) {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
