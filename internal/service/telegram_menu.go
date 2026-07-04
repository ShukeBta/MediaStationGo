package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// pendingTTL bounds how long a button-initiated text prompt stays valid.
const pendingTTL = 5 * time.Minute

func (s *TelegramBotService) setPending(userID int64, kind string) {
	s.pendingMu.Lock()
	s.pending[userID] = pendingInput{Kind: kind, CreatedAt: time.Now()}
	s.pendingMu.Unlock()
}

func (s *TelegramBotService) takePending(userID int64) (pendingInput, bool) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	p, ok := s.pending[userID]
	if ok {
		delete(s.pending, userID)
	}
	if ok && time.Since(p.CreatedAt) > pendingTTL {
		return pendingInput{}, false
	}
	return p, ok
}

// boundUser resolves the local user bound to a Telegram account, or nil.
func (s *TelegramBotService) boundUser(ctx context.Context, telegramUserID int) *model.User {
	binding := s.telegramBinding(ctx, telegramUserID)
	if binding == nil {
		return nil
	}
	u, _ := s.repo.User.FindByID(ctx, binding.UserID)
	return u
}

// handleMenuCallback routes inline-button taps. Returns (reply, handled).
func (s *TelegramBotService) handleMenuCallback(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, data string) (telegramCommandReply, bool) {
	isAdmin := s.telegramUserIsAdmin(ctx, channel, msg.From.ID)
	isGroup := telegramIsGroupChat(msg.Chat.Type)
	if reply, handled := s.handleUserMenuCallback(ctx, channel, msg, data, isGroup); handled {
		return reply, true
	}
	if !isAdmin {
		if isGroup {
			return telegramCommandReply{}, true
		}
		return telegramCommandReply{Text: "此功能仅管理员可用。"}, true
	}
	return s.handleAdminMenuCallback(ctx, msg, data)
}

func (s *TelegramBotService) handleUserMenuCallback(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, data string, isGroup bool) (telegramCommandReply, bool) {
	switch {
	case data == "noop":
		return telegramCommandReply{}, true
	case data == "menu_main":
		return s.mainMenu(ctx, channel, msg), true
	case data == "act_account":
		return s.replyAccount(ctx, msg), true
	case data == "act_signin":
		return s.replySignIn(ctx, msg), true
	case data == "act_devices":
		return s.replyDevices(ctx, msg), true
	case strings.HasPrefix(data, "kick:"):
		return s.replyKick(ctx, msg, strings.TrimPrefix(data, "kick:")), true
	}
	return s.handlePrivatePromptMenuCallback(ctx, msg, data, isGroup)
}

func (s *TelegramBotService) handlePrivatePromptMenuCallback(ctx context.Context, msg *TelegramMessage, data string, isGroup bool) (telegramCommandReply, bool) {
	switch data {
	case "act_bind":
		return telegramPrivateOnlyMenuReply(isGroup, "绑定账号", "请发送：<code>/start 用户名 密码</code> 绑定已有账号。"), true
	case "act_register":
		if isGroup {
			return telegramCommandReply{Text: telegramGroupPrivateUserHint("注册账号")}, true
		}
		if !s.openRegEnabled(ctx) {
			return telegramCommandReply{Text: "注册功能未开放，请联系管理员。"}, true
		}
		s.setPending(int64(msg.From.ID), "register")
		return telegramCommandReply{Text: "请发送新账号的 <b>用户名 密码</b>（空格分隔），例如：<code>alice mypass123</code>"}, true
	case "act_redeem_register":
		if isGroup {
			return telegramCommandReply{Text: telegramGroupPrivateUserHint("兑换码注册")}, true
		}
		s.setPending(int64(msg.From.ID), "redeem_register")
		return telegramCommandReply{Text: "请发送你的<b>注册兑换码</b>，例如：<code>ABCD2345EFGH</code>\n（兑换后会要求设置用户名密码）"}, true
	case "act_redeem_renew":
		if isGroup {
			return telegramCommandReply{Text: telegramGroupPrivateUserHint("兑换码续期")}, true
		}
		s.setPending(int64(msg.From.ID), "redeem_renew")
		return telegramCommandReply{Text: "请发送你的<b>续期兑换码</b>，将为当前绑定账号续期。"}, true
	case "act_setname":
		return s.setPendingPrivatePrompt(msg, isGroup, "修改用户名", "setname", "请发送：<code>当前密码 新用户名</code>。"), true
	case "act_setpass":
		return s.setPendingPrivatePrompt(msg, isGroup, "修改密码", "setpass", "请发送：<code>当前密码 新密码</code>（新密码至少 6 位）。"), true
	}
	return telegramCommandReply{}, false
}

func telegramPrivateOnlyMenuReply(isGroup bool, action, privateText string) telegramCommandReply {
	if isGroup {
		return telegramCommandReply{Text: telegramGroupPrivateUserHint(action)}
	}
	return telegramCommandReply{Text: privateText}
}

func (s *TelegramBotService) setPendingPrivatePrompt(msg *TelegramMessage, isGroup bool, action, kind, text string) telegramCommandReply {
	if isGroup {
		return telegramCommandReply{Text: telegramGroupPrivateUserHint(action)}
	}
	s.setPending(int64(msg.From.ID), kind)
	return telegramCommandReply{Text: text}
}

func (s *TelegramBotService) handleAdminMenuCallback(ctx context.Context, msg *TelegramMessage, data string) (telegramCommandReply, bool) {
	if reply, handled := s.handleAdminRegistrationCallback(ctx, msg, data); handled {
		return reply, true
	}
	if reply, handled := s.handleAdminUserCallback(ctx, data); handled {
		return reply, true
	}
	switch {
	case data == "adm_capacity":
		return s.replyCapacity(ctx), true
	case data == "adm_devicepolicy":
		return s.replyDevicePolicy(ctx), true
	case data == "adm_mgo_commands":
		return telegramCommandReply{Text: telegramMgoAdminCommandHelp(), Buttons: [][]telegramInlineButton{{{Text: "⬅️ 返回菜单", Data: "menu_main"}}}}, true
	case strings.HasPrefix(data, "dp_toggle:"):
		return s.replyDevicePolicyToggle(ctx, strings.TrimPrefix(data, "dp_toggle:")), true
	}
	return telegramCommandReply{}, false
}

func (s *TelegramBotService) handleAdminRegistrationCallback(ctx context.Context, msg *TelegramMessage, data string) (telegramCommandReply, bool) {
	switch {
	case data == "adm_openreg":
		return s.replyOpenRegMenu(ctx), true
	case data == "adm_openreg_close":
		_ = s.closeRegistration(ctx)
		return telegramCommandReply{Text: "已关闭注册。"}, true
	case strings.HasPrefix(data, "adm_openreg_set:"):
		n, _ := strconv.Atoi(strings.TrimPrefix(data, "adm_openreg_set:"))
		if err := s.openRegistration(ctx, n); err != nil {
			return telegramCommandReply{Text: "开注失败：" + err.Error()}, true
		}
		label := "不限"
		if n > 0 {
			label = fmt.Sprintf("%d 个名额", n)
		}
		return telegramCommandReply{Text: "已开放注册：" + label + "。"}, true
	case data == "adm_gencode":
		return s.replyGenCodeMenu(), true
	case strings.HasPrefix(data, "gc:"):
		return s.replyGenCode(ctx, msg, data), true
	}
	return telegramCommandReply{}, false
}

func (s *TelegramBotService) handleAdminUserCallback(ctx context.Context, data string) (telegramCommandReply, bool) {
	switch {
	case data == "adm_users":
		return s.replyUserList(ctx), true
	case strings.HasPrefix(data, "usr:"):
		return s.replyUserActions(ctx, strings.TrimPrefix(data, "usr:")), true
	case strings.HasPrefix(data, "uban:"):
		return s.replyUserBan(ctx, strings.TrimPrefix(data, "uban:"), false), true
	case strings.HasPrefix(data, "uunban:"):
		return s.replyUserBan(ctx, strings.TrimPrefix(data, "uunban:"), true), true
	case strings.HasPrefix(data, "udel:"):
		return s.replyUserDelete(ctx, strings.TrimPrefix(data, "udel:")), true
	case strings.HasPrefix(data, "urenew:"):
		return s.replyUserRenew(ctx, strings.TrimPrefix(data, "urenew:")), true
	}
	return telegramCommandReply{}, false
}

// handlePendingText consumes a button-initiated text prompt. Returns (reply,
// handled). handled=false means there was no pending prompt for this user.
func (s *TelegramBotService) handlePendingText(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, text string) (telegramCommandReply, bool) {
	p, ok := s.takePending(int64(msg.From.ID))
	if !ok {
		return telegramCommandReply{}, false
	}
	switch p.Kind {
	case "register":
		return s.cmdRegister(ctx, channel, msg, strings.Fields(text)), true
	case "redeem_register":
		return s.redeemRegisterFlow(ctx, channel, msg, text), true
	case "redeem_renew":
		return s.redeemRenewFlow(ctx, msg, text), true
	case "setname":
		return s.selfSetName(ctx, msg, text), true
	case "setpass":
		return s.selfSetPass(ctx, msg, text), true
	case "openreg_limit":
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || n < 0 {
			return telegramCommandReply{Text: "请输入有效的非负整数。"}, true
		}
		if err := s.openRegistration(ctx, n); err != nil {
			return telegramCommandReply{Text: "开注失败：" + err.Error()}, true
		}
		return telegramCommandReply{Text: fmt.Sprintf("已开放注册：%d 个名额。", n)}, true
	}
	return telegramCommandReply{}, false
}
