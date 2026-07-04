package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) cmdMgoSyncUnbound(ctx context.Context, args []string) telegramCommandReply {
	var users []model.User
	if err := s.repo.DB.WithContext(ctx).
		Where("role <> ?", "admin").
		Where("NOT EXISTS (SELECT 1 FROM telegram_bindings WHERE telegram_bindings.user_id = users.id AND telegram_bindings.deleted_at IS NULL)").
		Order("created_at asc").Find(&users).Error; err != nil {
		return telegramCommandReply{Text: "查询失败：" + err.Error()}
	}
	if len(args) >= 2 && strings.EqualFold(args[0], "delete") && strings.EqualFold(args[1], "confirm") {
		deleted := 0
		for _, user := range users {
			if UserIsProtectedAccount(ctx, s.repo, &user) {
				continue
			}
			_ = s.repo.UserDevice.DeleteByUser(ctx, user.ID)
			if err := s.repo.User.Delete(ctx, user.ID); err == nil {
				deleted++
			}
		}
		return telegramCommandReply{Text: fmt.Sprintf("已删除未绑定 Bot 的普通用户：<b>%d</b> 个。", deleted)}
	}
	if len(users) == 0 {
		return telegramCommandReply{Text: "没有未绑定 Bot 的普通用户。"}
	}
	names := make([]string, 0, minInt(len(users), 20))
	for i, user := range users {
		if i >= 20 {
			break
		}
		names = append(names, user.Username)
	}
	return telegramCommandReply{Text: fmt.Sprintf("未绑定 Bot 的普通用户：<b>%d</b> 个。\n%s\n\n如需删除：<code>/syncunbound delete confirm</code>", len(users), telegramInlineCodeList(names))}
}

func (s *TelegramBotService) cmdMgoCheckExpired(ctx context.Context, args []string) telegramCommandReply {
	now := time.Now()
	var users []model.User
	if err := s.repo.DB.WithContext(ctx).Where("expired_at IS NOT NULL AND expired_at < ?", now).Order("expired_at asc").Find(&users).Error; err != nil {
		return telegramCommandReply{Text: "查询失败：" + err.Error()}
	}
	if len(args) >= 2 && strings.EqualFold(args[0], "disable") && strings.EqualFold(args[1], "confirm") {
		disabled := 0
		for _, user := range users {
			if UserIsProtectedAccount(ctx, s.repo, &user) {
				continue
			}
			if err := s.repo.User.UpdateFields(ctx, user.ID, map[string]any{"is_active": false}); err == nil {
				disabled++
			}
		}
		return telegramCommandReply{Text: fmt.Sprintf("已禁用过期普通用户：<b>%d</b> 个。", disabled)}
	}
	if len(users) == 0 {
		return telegramCommandReply{Text: "没有过期用户。"}
	}
	lines := make([]string, 0, minInt(len(users), 20))
	for i, user := range users {
		if i >= 20 {
			break
		}
		lines = append(lines, fmt.Sprintf("%s（%s）", user.Username, formatExpiry(user.ExpiredAt)))
	}
	return telegramCommandReply{Text: fmt.Sprintf("过期用户：<b>%d</b> 个。\n%s\n\n如需禁用：<code>/check_ex disable confirm</code>", len(users), telegramInlineCodeList(lines))}
}

func (s *TelegramBotService) cmdMgoScanNames(ctx context.Context) telegramCommandReply {
	var rows []struct {
		Username string
		Count    int64
	}
	if err := s.repo.DB.WithContext(ctx).Table("users").
		Select("LOWER(username) AS username, COUNT(*) AS count").
		Group("LOWER(username)").Having("COUNT(*) > 1").Scan(&rows).Error; err != nil {
		return telegramCommandReply{Text: "扫描失败：" + err.Error()}
	}
	if len(rows) == 0 {
		return telegramCommandReply{Text: "未发现同名用户记录。"}
	}
	var out []string
	for _, row := range rows {
		out = append(out, fmt.Sprintf("%s x%d", row.Username, row.Count))
	}
	return telegramCommandReply{Text: "<b>同名用户记录</b>\n" + telegramInlineCodeList(out)}
}

func (s *TelegramBotService) cmdMgoRanks(ctx context.Context, window time.Duration, byDuration bool) telegramCommandReply {
	since := time.Now().Add(-window)
	title := "播放次数排行"
	selectExpr := "COUNT(*) AS score"
	if byDuration {
		title = "观影时长排行"
		selectExpr = "COALESCE(SUM(position_ms), 0) AS score"
	}
	q := s.repo.DB.WithContext(ctx).Table("playback_histories").
		Select("users.username, " + selectExpr).
		Joins("JOIN users ON users.id = playback_histories.user_id").
		Group("users.username").
		Order("score DESC").
		Limit(10)
	if window > 0 {
		q = q.Where("playback_histories.watched_at >= ?", since)
	}
	var rows []struct {
		Username string
		Score    int64
	}
	if err := q.Scan(&rows).Error; err != nil {
		return telegramCommandReply{Text: "排行查询失败：" + err.Error()}
	}
	if len(rows) == 0 {
		return telegramCommandReply{Text: "暂无排行数据。"}
	}
	var out []string
	for i, row := range rows {
		score := fmt.Sprintf("%d 次", row.Score)
		if byDuration {
			score = humanDurationFromMillis(row.Score)
		}
		out = append(out, fmt.Sprintf("%d. %s — %s", i+1, row.Username, score))
	}
	return telegramCommandReply{Text: "<b>" + title + "</b>\n\n<code>" + strings.Join(out, "\n") + "</code>"}
}
