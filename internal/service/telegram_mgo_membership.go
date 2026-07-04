package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) cmdMgoSyncGroup(ctx context.Context, channel *model.NotifyChannel, args []string) telegramCommandReply {
	chatIDs := s.telegramMembershipChatIDs(channel)
	if len(chatIDs) == 0 {
		return telegramCommandReply{Text: "未配置可校验成员的群组/频道 ID。请在 Telegram 通知渠道设置 group_chat_id 或 channel_chat_id。"}
	}
	if strings.TrimSpace(s.telegramChannelConfig(channel)["bot_token"]) == "" {
		return telegramCommandReply{Text: "当前 Telegram 渠道未配置 bot_token，无法校验群成员。"}
	}
	var bindings []model.TelegramBinding
	if err := s.repo.DB.WithContext(ctx).Find(&bindings).Error; err != nil {
		return telegramCommandReply{Text: "读取绑定失败：" + err.Error()}
	}
	type staleBinding struct {
		User    model.User
		Binding model.TelegramBinding
	}
	var stale []staleBinding
	for _, binding := range bindings {
		if binding.TelegramUserID == 0 || binding.UserID == "" {
			continue
		}
		user, _ := s.repo.User.FindByID(ctx, binding.UserID)
		if user == nil || UserIsProtectedAccount(ctx, s.repo, user) {
			continue
		}
		// 仅当所有绑定群组/频道都「查实不是成员」时才判定为可清理；
		// getChatMember 出错（membershipUnknown）时保守跳过，避免误删。
		confirmedNo := true
		for _, chatID := range chatIDs {
			if s.telegramChatMembership(ctx, channel, chatID, int(binding.TelegramUserID)) != membershipNo {
				confirmedNo = false
				break
			}
		}
		if confirmedNo {
			stale = append(stale, staleBinding{User: *user, Binding: binding})
		}
	}
	if len(stale) == 0 {
		return telegramCommandReply{Text: "所有已绑定账号都仍在配置的群组/频道中。"}
	}
	if len(args) >= 2 && strings.EqualFold(args[0], "delete") && strings.EqualFold(args[1], "confirm") {
		deleted := 0
		for _, item := range stale {
			_ = s.repo.UserDevice.DeleteByUser(ctx, item.User.ID)
			if err := s.repo.User.Delete(ctx, item.User.ID); err == nil {
				deleted++
			}
		}
		return telegramCommandReply{Text: fmt.Sprintf("已删除不在群组/频道中的普通账号：<b>%d</b> 个。", deleted)}
	}
	names := make([]string, 0, minInt(len(stale), 20))
	for i, item := range stale {
		if i >= 20 {
			break
		}
		names = append(names, fmt.Sprintf("%s(tg:%d)", item.User.Username, item.Binding.TelegramUserID))
	}
	return telegramCommandReply{Text: fmt.Sprintf("不在配置群组/频道中的绑定账号：<b>%d</b> 个。\n%s\n\n删除需确认：<code>/syncgroupm delete confirm</code>", len(stale), telegramInlineCodeList(names))}
}

func (s *TelegramBotService) telegramMembershipChatIDs(channel *model.NotifyChannel) []string {
	cfg := s.telegramChannelConfig(channel)
	seen := map[string]struct{}{}
	var out []string
	for _, key := range []string{"group_chat_id", "channel_chat_id", "command_chat_id"} {
		value := strings.TrimSpace(cfg[key])
		if value == "" {
			continue
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		if value := strings.TrimSpace(cfg["chat_id"]); strings.HasPrefix(value, "-") {
			out = append(out, value)
		}
	}
	return out
}
