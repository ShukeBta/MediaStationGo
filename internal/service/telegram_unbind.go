package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) cmdUnbind(ctx context.Context, args []string) telegramCommandReply {
	targets := parseTelegramUnbindTargets(args)
	if len(targets) == 0 {
		return telegramCommandReply{Text: "用法：<code>/unbind 用户名1 用户名2</code>\n也支持逗号分隔，或使用 <code>tg:TelegramID</code> 按 Telegram ID 解绑。此命令只解绑 Bot，不删除媒体账号。"}
	}
	var removed int64
	var done []string
	var skipped []string
	var missing []string
	for _, target := range targets {
		if tgIDRaw, ok := strings.CutPrefix(strings.ToLower(target), "tg:"); ok {
			tgID, err := strconv.ParseInt(tgIDRaw, 10, 64)
			if err != nil || tgID == 0 {
				missing = append(missing, target)
				continue
			}
			n, err := s.deleteTelegramBindings(ctx, "telegram_user_id = ?", tgID)
			if err != nil {
				return telegramCommandReply{Text: "解绑失败：" + err.Error()}
			}
			if n == 0 {
				missing = append(missing, target)
				continue
			}
			removed += n
			done = append(done, target)
			continue
		}

		user, _ := s.repo.User.FindByUsername(ctx, target)
		if user == nil {
			user, _ = s.repo.User.FindByID(ctx, target)
		}
		if user == nil {
			missing = append(missing, target)
			continue
		}
		if user.Role == "admin" {
			skipped = append(skipped, user.Username+"(管理员)")
			continue
		}
		n, err := s.deleteTelegramBindings(ctx, "user_id = ?", user.ID)
		if err != nil {
			return telegramCommandReply{Text: "解绑失败：" + err.Error()}
		}
		if n == 0 {
			missing = append(missing, user.Username+"(未绑定)")
			continue
		}
		removed += n
		done = append(done, user.Username)
	}
	return formatUnbindResult("批量解绑完成", removed, done, skipped, missing)
}

func (s *TelegramBotService) cmdUnbindDuplicates(ctx context.Context) telegramCommandReply {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return telegramCommandReply{Text: "仓库不可用。"}
	}
	var bindings []model.TelegramBinding
	if err := s.repo.DB.WithContext(ctx).Order("updated_at desc, created_at desc").Find(&bindings).Error; err != nil {
		return telegramCommandReply{Text: "读取绑定失败：" + err.Error()}
	}
	seenTelegram := make(map[int64]string)
	seenUser := make(map[string]string)
	var removeIDs []string
	var removedLabels []string
	for _, binding := range bindings {
		remove := false
		if binding.UserID == "" || binding.TelegramUserID == 0 {
			remove = true
		} else if user, _ := s.repo.User.FindByID(ctx, binding.UserID); user == nil {
			remove = true
		} else if _, ok := seenTelegram[binding.TelegramUserID]; ok {
			remove = true
		} else if _, ok := seenUser[binding.UserID]; ok {
			remove = true
		}
		if remove {
			removeIDs = append(removeIDs, binding.ID)
			removedLabels = append(removedLabels, fmt.Sprintf("tg:%d", binding.TelegramUserID))
			continue
		}
		seenTelegram[binding.TelegramUserID] = binding.ID
		seenUser[binding.UserID] = binding.ID
	}
	if len(removeIDs) == 0 {
		return telegramCommandReply{Text: "未发现重复或无效绑定。"}
	}
	n, err := s.deleteTelegramBindings(ctx, "id IN ?", removeIDs)
	if err != nil {
		return telegramCommandReply{Text: "清理失败：" + err.Error()}
	}
	return formatUnbindResult("重复/无效绑定清理完成", n, removedLabels, nil, nil)
}

func (s *TelegramBotService) cmdUnbindInactive(ctx context.Context, args []string) telegramCommandReply {
	if len(args) == 0 {
		return telegramCommandReply{Text: "用法：<code>/unbind_inactive 天数</code>\n例如 <code>/unbind_inactive 30</code> 会解绑 30 天未登录的普通用户 Bot 绑定，不删除账号。"}
	}
	days, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil || days < 1 {
		return telegramCommandReply{Text: "天数必须是大于 0 的整数。"}
	}
	users, err := s.repo.User.List(ctx)
	if err != nil {
		return telegramCommandReply{Text: "读取用户失败：" + err.Error()}
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	recentWindow := time.Duration(days) * 24 * time.Hour
	var userIDs []string
	var done []string
	for _, user := range users {
		if user.Role == "admin" {
			continue
		}
		lastActive := user.CreatedAt
		if user.LastLoginAt != nil {
			lastActive = *user.LastLoginAt
		}
		if lastActive.IsZero() || lastActive.After(cutoff) {
			continue
		}
		if s.device != nil && s.device.UserRecentlyActive(ctx, user.ID, recentWindow) {
			continue
		}
		var count int64
		_ = s.repo.DB.WithContext(ctx).Model(&model.TelegramBinding{}).Where("user_id = ?", user.ID).Count(&count).Error
		if count == 0 {
			continue
		}
		userIDs = append(userIDs, user.ID)
		done = append(done, user.Username)
	}
	if len(userIDs) == 0 {
		return telegramCommandReply{Text: fmt.Sprintf("未发现 %d 天未登录且已绑定 Bot 的普通用户。", days)}
	}
	n, err := s.deleteTelegramBindings(ctx, "user_id IN ?", userIDs)
	if err != nil {
		return telegramCommandReply{Text: "解绑失败：" + err.Error()}
	}
	return formatUnbindResult(fmt.Sprintf("已解绑 %d 天未登录用户", days), n, done, nil, nil)
}

func parseTelegramUnbindTargets(args []string) []string {
	seen := make(map[string]struct{})
	var targets []string
	for _, arg := range args {
		for _, part := range strings.FieldsFunc(arg, func(r rune) bool {
			return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\t'
		}) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			key := strings.ToLower(part)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, part)
		}
	}
	return targets
}

func (s *TelegramBotService) deleteTelegramBindings(ctx context.Context, query string, args ...interface{}) (int64, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return 0, nil
	}
	tx := s.repo.DB.WithContext(ctx).Unscoped().Where(query, args...).Delete(&model.TelegramBinding{})
	return tx.RowsAffected, tx.Error
}

func formatUnbindResult(title string, removed int64, done, skipped, missing []string) telegramCommandReply {
	var sb strings.Builder
	sb.WriteString("<b>")
	sb.WriteString(title)
	sb.WriteString("</b>\n\n")
	sb.WriteString(fmt.Sprintf("已解绑：<b>%d</b> 条绑定", removed))
	if len(done) > 0 {
		sb.WriteString("\n目标：")
		sb.WriteString(formatShortList(done, 12))
	}
	if len(skipped) > 0 {
		sb.WriteString("\n跳过：")
		sb.WriteString(formatShortList(skipped, 8))
	}
	if len(missing) > 0 {
		sb.WriteString("\n未找到/未绑定：")
		sb.WriteString(formatShortList(missing, 8))
	}
	return telegramCommandReply{Text: sb.String()}
}

func formatShortList(items []string, limit int) string {
	if len(items) == 0 {
		return ""
	}
	if limit < 1 {
		limit = 1
	}
	out := items
	if len(out) > limit {
		out = out[:limit]
	}
	text := "<code>" + strings.Join(out, "</code>、<code>") + "</code>"
	if len(items) > limit {
		text += fmt.Sprintf(" 等 %d 项", len(items))
	}
	return text
}
