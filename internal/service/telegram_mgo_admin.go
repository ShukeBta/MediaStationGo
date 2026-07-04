package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) cmdMgoAdminRole(ctx context.Context, args []string) telegramCommandReply {
	if len(args) < 2 {
		return telegramCommandReply{Text: "用法：<code>/embyadmin 用户名 on|off</code>"}
	}
	user := s.findMgoBotUser(ctx, args[0])
	if user == nil {
		return telegramCommandReply{Text: "未找到用户。"}
	}
	enable := parseOnOff(args[1])
	if enable == nil {
		return telegramCommandReply{Text: "第二个参数请使用 on/off。"}
	}
	if !*enable {
		if first, _ := s.repo.User.FirstAdmin(ctx); first != nil && first.ID == user.ID {
			return telegramCommandReply{Text: "默认管理员不可降级。"}
		}
	}
	role := "user"
	if *enable {
		role = "admin"
	}
	if err := s.repo.User.UpdateFields(ctx, user.ID, map[string]any{"role": role}); err != nil {
		return telegramCommandReply{Text: "更新失败：" + err.Error()}
	}
	return telegramCommandReply{Text: fmt.Sprintf("已将 <b>%s</b> 角色设置为 <b>%s</b>。", user.Username, role)}
}

func (s *TelegramBotService) cmdMgoMediaAccessAll(ctx context.Context, allow bool) telegramCommandReply {
	users, err := s.repo.User.List(ctx)
	if err != nil {
		return telegramCommandReply{Text: "读取用户失败：" + err.Error()}
	}
	updated := 0
	for _, user := range users {
		if user.Role == "admin" {
			continue
		}
		perm, err := s.repo.Permission.FindByUserID(ctx, user.ID)
		if err != nil {
			continue
		}
		if perm == nil {
			perm = DefaultPermissions(user.ID)
			perm.CanPlayMedia = allow
			if err := s.repo.Permission.Create(ctx, perm); err != nil {
				continue
			}
		}
		if err := s.repo.DB.WithContext(ctx).Model(&model.UserPermission{}).
			Where("user_id = ?", user.ID).
			Update("can_play_media", allow).Error; err == nil {
			updated++
		}
	}
	state := "关闭"
	if allow {
		state = "开启"
	}
	return telegramCommandReply{Text: fmt.Sprintf("已为普通用户%s媒体播放权限：<b>%d</b> 个。", state, updated)}
}

func (s *TelegramBotService) cmdMgoBotAdmin(ctx context.Context, channel *model.NotifyChannel, args []string, add bool) telegramCommandReply {
	if channel == nil {
		return telegramCommandReply{Text: "Telegram 渠道不存在。"}
	}
	if len(args) == 0 {
		return telegramCommandReply{Text: "用法：<code>/proadmin TelegramID</code> 或 <code>/revadmin TelegramID</code>"}
	}
	tgID := strings.TrimPrefix(strings.TrimSpace(args[0]), "tg:")
	if _, err := strconv.ParseInt(tgID, 10, 64); err != nil {
		return telegramCommandReply{Text: "TelegramID 必须是数字。"}
	}
	cfg := s.telegramChannelConfig(channel)
	ids := telegramConfiguredUserIDs(cfg["admin_user_ids"])
	seen := make(map[string]bool, len(ids)+1)
	var next []string
	for _, id := range ids {
		if id == tgID {
			seen[id] = true
			if add {
				next = append(next, id)
			}
			continue
		}
		if id != "" {
			next = append(next, id)
		}
	}
	if add && !seen[tgID] {
		next = append(next, tgID)
	}
	cfg["admin_user_ids"] = strings.Join(next, ",")
	raw, _ := json.Marshal(cfg)
	updated := *channel
	updated.Config = string(raw)
	if s.crypto != nil {
		updated.Config = s.crypto.Encrypt(updated.Config)
	}
	if err := s.repo.NotifyChannel.Update(ctx, &updated); err != nil {
		return telegramCommandReply{Text: "更新管理员列表失败：" + err.Error()}
	}
	if add {
		return telegramCommandReply{Text: "已添加 Bot 管理员：<code>" + tgID + "</code>"}
	}
	return telegramCommandReply{Text: "已移除 Bot 管理员：<code>" + tgID + "</code>"}
}

func (s *TelegramBotService) cmdMgoProtectedUser(ctx context.Context, args []string, protect bool) telegramCommandReply {
	if len(args) == 0 || strings.EqualFold(args[0], "list") {
		return s.cmdMgoProtectedUserList(ctx)
	}
	user := s.findMgoBotUser(ctx, args[0])
	if user == nil {
		return telegramCommandReply{Text: "未找到用户。"}
	}
	ids := ProtectedUserIDSet(ctx, s.repo)
	if protect {
		ids[user.ID] = struct{}{}
		if err := SaveProtectedUserIDSet(ctx, s.repo, ids); err != nil {
			return telegramCommandReply{Text: "保存保护名单失败：" + err.Error()}
		}
		return telegramCommandReply{Text: fmt.Sprintf("已加入保护名单：<b>%s</b>。\n该用户不会被 Bot 自动清理、批量禁用或删除。", user.Username)}
	}
	delete(ids, user.ID)
	if err := SaveProtectedUserIDSet(ctx, s.repo, ids); err != nil {
		return telegramCommandReply{Text: "保存保护名单失败：" + err.Error()}
	}
	return telegramCommandReply{Text: fmt.Sprintf("已移出保护名单：<b>%s</b>。", user.Username)}
}

func (s *TelegramBotService) cmdMgoProtectedUserList(ctx context.Context) telegramCommandReply {
	ids := ProtectedUserIDSet(ctx, s.repo)
	if len(ids) == 0 {
		return telegramCommandReply{Text: "保护名单为空。管理员和默认管理员始终自动保护。"}
	}
	names := make([]string, 0, len(ids))
	for id := range ids {
		if user, _ := s.repo.User.FindByID(ctx, id); user != nil {
			names = append(names, user.Username)
		} else {
			names = append(names, id+"(用户不存在)")
		}
	}
	sort.Strings(names)
	return telegramCommandReply{Text: fmt.Sprintf("保护名单：<b>%d</b> 个。\n%s", len(names), telegramInlineCodeList(names))}
}
