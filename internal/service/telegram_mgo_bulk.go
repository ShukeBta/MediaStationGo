package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) cmdMgoRenewAll(ctx context.Context, args []string) telegramCommandReply {
	if len(args) < 2 || !strings.EqualFold(args[len(args)-1], "confirm") {
		return telegramCommandReply{Text: "批量续期需要确认：<code>/renewall 天数 confirm</code>"}
	}
	days, err := strconv.Atoi(args[0])
	if err != nil || days < 0 {
		return telegramCommandReply{Text: "天数必须是非负整数，0 表示永久。"}
	}
	users, err := s.repo.User.List(ctx)
	if err != nil {
		return telegramCommandReply{Text: "读取用户失败：" + err.Error()}
	}
	var count int
	for _, user := range users {
		if user.Role == "admin" {
			continue
		}
		if err := s.applyRenewal(ctx, user.ID, days); err == nil {
			count++
		}
	}
	return telegramCommandReply{Text: fmt.Sprintf("批量续期完成：<b>%d</b> 个普通用户。", count)}
}

func (s *TelegramBotService) cmdMgoBanAll(ctx context.Context, active bool, args []string) telegramCommandReply {
	if len(args) == 0 || !strings.EqualFold(args[len(args)-1], "confirm") {
		action := "banall"
		if active {
			action = "unbanall"
		}
		return telegramCommandReply{Text: fmt.Sprintf("批量操作需要确认：<code>/%s confirm</code>", action)}
	}
	users, err := s.repo.User.List(ctx)
	if err != nil {
		return telegramCommandReply{Text: "读取用户失败：" + err.Error()}
	}
	var count int
	for _, user := range users {
		if !active && UserIsProtectedAccount(ctx, s.repo, &user) {
			continue
		}
		if active && user.Role == "admin" {
			continue
		}
		updates := map[string]any{"is_active": active}
		if active {
			updates["share_warnings"] = 0
			updates["last_share_warn_at"] = nil
		}
		if err := s.repo.User.UpdateFields(ctx, user.ID, updates); err == nil {
			_ = s.repo.UserDevice.SetKickedByUser(ctx, user.ID, !active)
			count++
		}
	}
	if active {
		return telegramCommandReply{Text: fmt.Sprintf("已解禁普通用户：<b>%d</b> 个。", count)}
	}
	return telegramCommandReply{Text: fmt.Sprintf("已禁用普通用户：<b>%d</b> 个。", count)}
}

func (s *TelegramBotService) cmdMgoCallAll(ctx context.Context, channel *model.NotifyChannel, args []string) telegramCommandReply {
	message := strings.TrimSpace(strings.Join(args, " "))
	if message == "" {
		return telegramCommandReply{Text: "用法：<code>/callall 消息内容</code>"}
	}
	if strings.TrimSpace(s.telegramChannelConfig(channel)["bot_token"]) == "" {
		return telegramCommandReply{Text: "当前 Telegram 渠道未配置 bot_token，无法群发。"}
	}
	var bindings []model.TelegramBinding
	if err := s.repo.DB.WithContext(ctx).Find(&bindings).Error; err != nil {
		return telegramCommandReply{Text: "读取绑定失败：" + err.Error()}
	}
	sent := 0
	for _, binding := range bindings {
		if binding.ChatID == 0 {
			continue
		}
		if err := s.reply(ctx, channel, int(binding.ChatID), telegramCommandReply{Text: message}); err == nil {
			sent++
		}
	}
	return telegramCommandReply{Text: fmt.Sprintf("群发完成：成功发送 <b>%d</b> 个绑定用户。", sent)}
}
