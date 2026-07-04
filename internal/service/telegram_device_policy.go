package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (s *TelegramBotService) replyDevicePolicy(ctx context.Context) telegramCommandReply {
	cfg := loadBotConfig(ctx, s.repo)
	text := fmt.Sprintf(
		"<b>设备策略</b>\n\n① 防共享：<b>%s</b>\n   并发播放终端上限 %d / 登录终端上限 %d；同一终端多个 App 只算 1 台，App 作为登录渠道记录。\n   设备指纹异常警告 %d 次后禁用账号。\n\n② Mgo 保号规则：<b>%s</b>\n   保号模式：%s；启用规则 %d 条。\n\n<b>命令：</b>\n<code>/antishare on play=3 login=3 warn=2</code>\n<code>/cleanup run</code> 预览候选\n<code>/cleanup run confirm</code> 确认清理\n<code>/cleanup on|off</code>\n<code>/cleanup_rule list|add|edit|修改|del|enable|disable</code>\n\n策略默认关闭；清理前会先预览候选；满足任意一条保号规则即保留；管理员/受保护账号永不自动处理。",
		onOff(cfg.AntiShareEnabled), cfg.MaxConcurrentPlay, cfg.MaxLoggedClients, cfg.WarnThreshold,
		onOff(cfg.AccountCleanupEnabled), cleanupModeLabel(cfg.AccountCleanupKeepMode), countEnabledCleanupRules(cfg.AccountCleanupRules))
	return telegramCommandReply{
		Text: text,
		Buttons: [][]telegramInlineButton{
			{{Text: toggleLabel("防共享", cfg.AntiShareEnabled), Data: "dp_toggle:antishare"}},
			{{Text: toggleLabel("保号规则", cfg.AccountCleanupEnabled), Data: "dp_toggle:cleanup"}},
			{{Text: "⬅️ 返回菜单", Data: "menu_main"}},
		},
	}
}

func (s *TelegramBotService) cmdDevicePolicy(ctx context.Context, args []string) telegramCommandReply {
	if len(args) == 0 || strings.EqualFold(args[0], "status") {
		return s.replyDevicePolicy(ctx)
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "run", "sweep":
		return s.cmdCleanup(ctx, []string{"run"})
	default:
		return telegramCommandReply{Text: "用法：<code>/devicepolicy</code> 查看策略，或使用 <code>/antishare</code>、<code>/cleanup</code>、<code>/cleanup_rule</code> 管理。"}
	}
}

func (s *TelegramBotService) cmdAntiShare(ctx context.Context, args []string) telegramCommandReply {
	if len(args) == 0 || strings.EqualFold(args[0], "status") {
		return s.replyDevicePolicy(ctx)
	}
	enabled, ok := parseCommandBool(args[0])
	if !ok {
		return telegramCommandReply{Text: "用法：<code>/antishare on|off [play=3] [login=3] [warn=2]</code>，login 表示登录终端设备上限，同一终端多个 App 不重复计数。"}
	}
	if err := s.repo.Setting.Set(ctx, SettingAntiShareEnabled, strconv.FormatBool(enabled)); err != nil {
		return telegramCommandReply{Text: "更新失败：" + err.Error()}
	}
	for _, arg := range args[1:] {
		key, value, ok := strings.Cut(arg, "=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 1 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "play", "maxplay", "播放":
			_ = s.repo.Setting.Set(ctx, SettingMaxConcurrentPlay, strconv.Itoa(n))
		case "login", "client", "clients", "登录":
			_ = s.repo.Setting.Set(ctx, SettingMaxLoggedClients, strconv.Itoa(n))
		case "warn", "warnings", "警告":
			_ = s.repo.Setting.Set(ctx, SettingWarnThreshold, strconv.Itoa(n))
		}
	}
	return s.replyDevicePolicy(ctx)
}

func (s *TelegramBotService) cmdCleanup(ctx context.Context, args []string) telegramCommandReply {
	if len(args) == 0 || strings.EqualFold(args[0], "status") {
		return s.replyDevicePolicy(ctx)
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "on", "true", "1", "开启", "enable":
		if err := s.repo.Setting.Set(ctx, SettingAccountCleanupEnabled, "true"); err != nil {
			return telegramCommandReply{Text: "开启失败：" + err.Error()}
		}
		return s.replyDevicePolicy(ctx)
	case "off", "false", "0", "关闭", "disable":
		if err := s.repo.Setting.Set(ctx, SettingAccountCleanupEnabled, "false"); err != nil {
			return telegramCommandReply{Text: "关闭失败：" + err.Error()}
		}
		return s.replyDevicePolicy(ctx)
	case "run", "sweep", "巡检", "preview", "预览":
		device := s.device
		if device == nil {
			device = NewDeviceService(s.log, s.repo)
		}
		if len(args) > 1 && isCleanupConfirmArg(args[1]) {
			cfg := loadBotConfig(ctx, s.repo)
			if !cfg.AccountCleanupEnabled {
				return telegramCommandReply{Text: "保号规则未开启，不会清理账号。"}
			}
			if countEnabledCleanupRules(cfg.AccountCleanupRules) == 0 {
				return telegramCommandReply{Text: "没有启用的保号规则，不会清理账号。"}
			}
			removed, err := device.SweepAccountCleanup(ctx)
			if err != nil {
				return telegramCommandReply{Text: "确认清理失败：" + err.Error()}
			}
			return telegramCommandReply{Text: fmt.Sprintf("保号规则确认清理完成，已清理 <b>%d</b> 个账号。", removed)}
		}
		candidates, err := device.PreviewAccountCleanup(ctx)
		if err != nil {
			return telegramCommandReply{Text: "巡检预览失败：" + err.Error()}
		}
		return telegramCommandReply{Text: s.formatCleanupPreview(ctx, candidates)}
	default:
		return telegramCommandReply{Text: "用法：<code>/cleanup on|off</code>、<code>/cleanup run</code> 预览、<code>/cleanup run confirm</code> 确认清理"}
	}
}

func isCleanupConfirmArg(arg string) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "confirm", "yes", "delete", "确认", "清理", "删除":
		return true
	default:
		return false
	}
}

func (s *TelegramBotService) formatCleanupPreview(ctx context.Context, candidates []accountCleanupCandidate) string {
	cfg := loadBotConfig(ctx, s.repo)
	if !cfg.AccountCleanupEnabled {
		return "保号规则未开启，不会清理账号。"
	}
	if countEnabledCleanupRules(cfg.AccountCleanupRules) == 0 {
		return "没有启用的保号规则，不会清理账号。"
	}
	if len(candidates) == 0 {
		return "保号规则预览完成：没有需要清理的账号。"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>保号规则预览</b>\n\n将清理候选：<b>%d</b> 个账号。\n当前只是预览，未删除任何账号。\n\n", len(candidates)))
	limit := len(candidates)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		candidate := candidates[i]
		sb.WriteString(fmt.Sprintf("%d. <b>%s</b>\n%s\n", i+1, escapeHTML(candidate.Username), escapeHTML(candidate.Details)))
	}
	if len(candidates) > limit {
		sb.WriteString(fmt.Sprintf("……另有 %d 个候选未展示。\n", len(candidates)-limit))
	}
	sb.WriteString("\n确认无误后再执行：<code>/cleanup run confirm</code>")
	return sb.String()
}

func (s *TelegramBotService) cmdCleanupMode(ctx context.Context, args []string) telegramCommandReply {
	if err := s.repo.Setting.Set(ctx, SettingAccountCleanupKeepMode, "any"); err != nil {
		return telegramCommandReply{Text: "更新失败：" + err.Error()}
	}
	if err := s.repo.Setting.Set(ctx, SettingAccountCleanupRequiredCount, "1"); err != nil {
		return telegramCommandReply{Text: "更新失败：" + err.Error()}
	}
	reply := s.replyDevicePolicy(ctx)
	reply.Text = "Mgo 保号模式固定为：满足任意一条启用规则即保留；只有全部规则都不满足才进入清理候选。\n\n" + reply.Text
	return reply
}

func (s *TelegramBotService) cmdCleanupRule(ctx context.Context, args []string) telegramCommandReply {
	rules := s.currentCleanupRules(ctx)
	if len(args) == 0 {
		return telegramCommandReply{Text: formatCleanupRules(rules)}
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "list", "ls", "status":
		return telegramCommandReply{Text: formatCleanupRules(rules)}
	case "help", "?", "帮助":
		return telegramCommandReply{Text: cleanupRuleHelp()}
	case "del", "delete", "rm":
		if len(args) < 2 {
			return telegramCommandReply{Text: "用法：<code>/cleanup_rule del 规则ID</code>"}
		}
		next := make([]accountCleanupRule, 0, len(rules))
		removed := false
		for _, r := range rules {
			if r.ID == args[1] {
				removed = true
				continue
			}
			next = append(next, r)
		}
		if !removed {
			return telegramCommandReply{Text: "未找到该规则。"}
		}
		if err := s.saveCleanupRules(ctx, next); err != nil {
			return telegramCommandReply{Text: "保存失败：" + err.Error()}
		}
		return telegramCommandReply{Text: "已删除规则。\n\n" + formatCleanupRules(next)}
	case "enable", "on", "disable", "off":
		if len(args) < 2 {
			return telegramCommandReply{Text: "用法：<code>/cleanup_rule enable|disable 规则ID</code>"}
		}
		enable := action == "enable" || action == "on"
		changed := false
		for i := range rules {
			if rules[i].ID == args[1] {
				rules[i].Enabled = enable
				changed = true
			}
		}
		if !changed {
			return telegramCommandReply{Text: "未找到该规则。"}
		}
		if err := s.saveCleanupRules(ctx, rules); err != nil {
			return telegramCommandReply{Text: "保存失败：" + err.Error()}
		}
		return telegramCommandReply{Text: "已更新规则状态。\n\n" + formatCleanupRules(rules)}
	case "add", "set", "edit", "update", "修改", "更新", "改":
		rule, err := parseCleanupRuleCommand(args[1:])
		if err != nil {
			return telegramCommandReply{Text: err.Error() + "\n\n" + cleanupRuleHelp()}
		}
		updated := false
		for i := range rules {
			if rules[i].ID == rule.ID {
				rules[i] = rule
				updated = true
				break
			}
		}
		if !updated {
			rules = append(rules, rule)
		}
		rules = normalizeCleanupRules(rules)
		if err := s.saveCleanupRules(ctx, rules); err != nil {
			return telegramCommandReply{Text: "保存失败：" + err.Error()}
		}
		actionText := "已新增规则。"
		if updated {
			actionText = "已更新规则。"
		}
		return telegramCommandReply{Text: actionText + "\n\n" + formatCleanupRules(rules)}
	default:
		return telegramCommandReply{Text: cleanupRuleHelp()}
	}
}

func (s *TelegramBotService) replyDevicePolicyToggle(ctx context.Context, which string) telegramCommandReply {
	cfg := loadBotConfig(ctx, s.repo)
	switch which {
	case "antishare":
		_ = s.repo.Setting.Set(ctx, SettingAntiShareEnabled, strconv.FormatBool(!cfg.AntiShareEnabled))
	case "cleanup":
		_ = s.repo.Setting.Set(ctx, SettingAccountCleanupEnabled, strconv.FormatBool(!cfg.AccountCleanupEnabled))
	}
	return s.replyDevicePolicy(ctx)
}
