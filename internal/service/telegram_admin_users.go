package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (s *TelegramBotService) replyUserList(ctx context.Context) telegramCommandReply {
	users, err := s.repo.User.List(ctx)
	if err != nil {
		return telegramCommandReply{Text: "读取用户失败：" + err.Error()}
	}
	if len(users) == 0 {
		return telegramCommandReply{Text: "暂无用户。"}
	}
	var rows [][]telegramInlineButton
	limit := len(users)
	if limit > 12 {
		limit = 12
	}
	for i := 0; i < limit; i++ {
		u := users[i]
		flag := ""
		if !u.IsActive {
			flag = "🚫"
		}
		if u.Role == "admin" {
			flag = "👑"
		}
		rows = append(rows, []telegramInlineButton{{Text: flag + " " + u.Username, Data: "usr:" + u.ID}})
	}
	rows = append(rows, []telegramInlineButton{{Text: "⬅️ 返回菜单", Data: "menu_main"}})
	return telegramCommandReply{Text: fmt.Sprintf("<b>用户管理</b>（共 %d 人，显示前 %d）\n点击用户进行操作：", len(users), limit), Buttons: rows}
}

func (s *TelegramBotService) replyUserActions(ctx context.Context, userID string) telegramCommandReply {
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil || u == nil {
		return telegramCommandReply{Text: "用户不存在。"}
	}
	protected := UserIsProtectedAccount(ctx, s.repo, u)
	text := fmt.Sprintf("<b>%s</b>\n角色：%s\n状态：%s\n到期：%s\n防共享警告：%d 次",
		u.Username, u.Role, map[bool]string{true: "正常", false: "已禁用"}[u.IsActive], formatExpiry(u.ExpiredAt), u.ShareWarnings)
	if protected {
		return telegramCommandReply{Text: text + "\n\n（受保护账号，不可禁用/删除）", Buttons: [][]telegramInlineButton{{{Text: "⬅️ 返回", Data: "adm_users"}}}}
	}
	banBtn := telegramInlineButton{Text: "🚫 禁用", Data: "uban:" + u.ID}
	if !u.IsActive {
		banBtn = telegramInlineButton{Text: "✅ 解禁", Data: "uunban:" + u.ID}
	}
	return telegramCommandReply{
		Text: text,
		Buttons: [][]telegramInlineButton{
			{banBtn, {Text: "⏳ 续期30天", Data: "urenew:" + u.ID + ":30"}},
			{{Text: "🗑 删除用户", Data: "udel:" + u.ID}},
			{{Text: "⬅️ 返回", Data: "adm_users"}},
		},
	}
}

func (s *TelegramBotService) replyUserBan(ctx context.Context, userID string, unban bool) telegramCommandReply {
	if !unban {
		if reason := s.protectReason(ctx, userID); reason != "" {
			return telegramCommandReply{Text: reason}
		}
	}
	updates := map[string]any{"is_active": unban}
	if unban {
		updates["share_warnings"] = 0
		updates["last_share_warn_at"] = nil
	}
	if err := s.repo.User.UpdateFields(ctx, userID, updates); err != nil {
		return telegramCommandReply{Text: "操作失败：" + err.Error()}
	}
	if unban {
		_ = s.repo.UserDevice.SetKickedByUser(ctx, userID, false)
	}
	return s.replyUserActions(ctx, userID)
}

func (s *TelegramBotService) replyUserDelete(ctx context.Context, userID string) telegramCommandReply {
	if reason := s.protectReason(ctx, userID); reason != "" {
		return telegramCommandReply{Text: reason}
	}
	u, _ := s.repo.User.FindByID(ctx, userID)
	_ = s.repo.UserDevice.DeleteByUser(ctx, userID)
	if err := s.repo.User.Delete(ctx, userID); err != nil {
		return telegramCommandReply{Text: "删除失败：" + err.Error()}
	}
	name := userID
	if u != nil {
		name = u.Username
	}
	return telegramCommandReply{Text: fmt.Sprintf("已删除用户 <b>%s</b>。", name), Buttons: [][]telegramInlineButton{{{Text: "⬅️ 返回", Data: "adm_users"}}}}
}

func (s *TelegramBotService) replyUserRenew(ctx context.Context, payload string) telegramCommandReply {
	parts := strings.Split(payload, ":") // <id>:<days>
	if len(parts) != 2 {
		return telegramCommandReply{Text: "参数错误。"}
	}
	days, _ := strconv.Atoi(parts[1])
	if err := s.applyRenewal(ctx, parts[0], days); err != nil {
		return telegramCommandReply{Text: "续期失败：" + err.Error()}
	}
	return s.replyUserActions(ctx, parts[0])
}

func (s *TelegramBotService) cmdUserRenew(ctx context.Context, args []string) telegramCommandReply {
	if len(args) < 2 {
		return telegramCommandReply{Text: "用法：<code>/renew_user 用户名 天数</code>，天数 0 表示永久。"}
	}
	user, _ := s.repo.User.FindByUsername(ctx, args[0])
	if user == nil {
		user, _ = s.repo.User.FindByID(ctx, args[0])
	}
	if user == nil {
		return telegramCommandReply{Text: "未找到用户。"}
	}
	days, err := strconv.Atoi(args[1])
	if err != nil || days < 0 {
		return telegramCommandReply{Text: "天数必须是非负整数。"}
	}
	if err := s.applyRenewal(ctx, user.ID, days); err != nil {
		return telegramCommandReply{Text: "续期失败：" + err.Error()}
	}
	return s.replyUserActions(ctx, user.ID)
}

func (s *TelegramBotService) cmdUserDelete(ctx context.Context, args []string) telegramCommandReply {
	if len(args) == 0 {
		return telegramCommandReply{Text: "用法：<code>/delete_user 用户名 confirm</code>\n为避免误删，最后一个参数必须是 confirm。"}
	}
	if len(args) < 2 || !strings.EqualFold(args[len(args)-1], "confirm") {
		return telegramCommandReply{Text: "删除用户需要确认：<code>/delete_user 用户名 confirm</code>"}
	}
	user, _ := s.repo.User.FindByUsername(ctx, args[0])
	if user == nil {
		user, _ = s.repo.User.FindByID(ctx, args[0])
	}
	if user == nil {
		return telegramCommandReply{Text: "未找到用户。"}
	}
	return s.replyUserDelete(ctx, user.ID)
}

// protectReason returns a non-empty message when a user must not be
// disabled/deleted (admins, default admin and protected-list users).
func (s *TelegramBotService) protectReason(ctx context.Context, userID string) string {
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil || u == nil {
		return "用户不存在。"
	}
	if u.Role == "admin" {
		return "管理员账号受保护，不可禁用/删除。"
	}
	if first, _ := s.repo.User.FirstAdmin(ctx); first != nil && first.ID == u.ID {
		return "默认管理员账号受保护，不可禁用/删除。"
	}
	if _, ok := ProtectedUserIDSet(ctx, s.repo)[u.ID]; ok {
		return "该账号在 Bot 保护名单中，不可禁用/删除。"
	}
	if s.device != nil && s.device.UserRecentlyActive(ctx, u.ID, realtimeSessionTTL) {
		return "该账号最近仍有实时活跃会话，为避免误删/误禁用，请先确认用户已下线。"
	}
	return ""
}

func (s *TelegramBotService) cmdUserBan(ctx context.Context, args []string, unban bool) telegramCommandReply {
	if len(args) == 0 {
		if unban {
			return telegramCommandReply{Text: "用法：<code>/unban 用户名</code>"}
		}
		return telegramCommandReply{Text: "用法：<code>/ban 用户名</code>"}
	}
	user, _ := s.repo.User.FindByUsername(ctx, args[0])
	if user == nil {
		user, _ = s.repo.User.FindByID(ctx, args[0])
	}
	if user == nil {
		return telegramCommandReply{Text: "未找到用户。"}
	}
	return s.replyUserBan(ctx, user.ID, unban)
}
