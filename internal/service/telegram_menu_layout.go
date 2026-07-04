package service

import (
	"context"
	"fmt"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// mainMenu builds the button-based menu, tailored to the user's binding and
// admin status. Ordinary users only see self-service actions; admins get an
// extra management section.
func (s *TelegramBotService) mainMenu(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage) telegramCommandReply {
	isAdmin := s.telegramUserIsAdmin(ctx, channel, msg.From.ID)
	user := s.boundUser(ctx, msg.From.ID)
	if telegramIsGroupChat(msg.Chat.Type) {
		return s.groupMainMenu(isAdmin, user)
	}
	return s.privateMainMenu(ctx, isAdmin, user)
}

func (s *TelegramBotService) groupMainMenu(isAdmin bool, user *model.User) telegramCommandReply {
	header := "<b>MediaStationGo 群组自助菜单</b>\n\n你还没有绑定媒体中心账号。绑定、注册、兑换等包含敏感信息的操作请私聊 Bot。"
	var rows [][]telegramInlineButton
	if user != nil {
		header = telegramUserMenuHeader("<b>MediaStationGo 群组自助菜单</b>", user)
		rows = telegramBoundUserMenuRows(user, false)
	}
	if isAdmin {
		header += "\n\n<b>管理员入口</b>"
		rows = append(rows, telegramAdminMenuRows()...)
	}
	return telegramCommandReply{Text: header, Buttons: rows}
}

func (s *TelegramBotService) privateMainMenu(ctx context.Context, isAdmin bool, user *model.User) telegramCommandReply {
	header := "<b>MediaStationGo</b>\n\n你还没有绑定媒体中心账号。"
	rows := s.privateUnboundMenuRows(ctx)
	if user != nil {
		header = telegramUserMenuHeader("<b>MediaStationGo</b>", user)
		rows = telegramBoundUserMenuRows(user, true)
	}
	if isAdmin {
		rows = append(rows, telegramAdminMenuRows()...)
	}
	return telegramCommandReply{Text: header, Buttons: rows}
}

func (s *TelegramBotService) privateUnboundMenuRows(ctx context.Context) [][]telegramInlineButton {
	rows := [][]telegramInlineButton{{{Text: "🔗 绑定账号", Data: "act_bind"}}}
	if s.openRegEnabled(ctx) {
		rows = append(rows, []telegramInlineButton{{Text: "📝 注册新账号", Data: "act_register"}})
	}
	return append(rows, []telegramInlineButton{{Text: "🎟 兑换码注册", Data: "act_redeem_register"}})
}

func telegramUserMenuHeader(title string, user *model.User) string {
	return fmt.Sprintf("%s\n\n账号：<b>%s</b>\n到期：<b>%s</b>\n成人目录：<b>%s</b>",
		title, user.Username, formatExpiry(user.ExpiredAt), telegramAdultVisibilityLabel(user.HideAdult))
}

func telegramAdultVisibilityLabel(hidden bool) string {
	if hidden {
		return "已隐藏"
	}
	return "已显示"
}

func telegramAdultToggleText(hidden bool) string {
	if hidden {
		return "🔞 显示成人目录"
	}
	return "🔞 隐藏成人目录"
}

func telegramBoundUserMenuRows(user *model.User, includePrivateActions bool) [][]telegramInlineButton {
	rows := [][]telegramInlineButton{
		{
			{Text: "👤 我的账号", Data: "act_account"},
			{Text: "📅 签到", Data: "act_signin"},
		},
		{
			{Text: "📱 我的设备", Data: "act_devices"},
			{Text: telegramAdultToggleText(user.HideAdult), Data: "adult_toggle"},
		},
	}
	if includePrivateActions {
		rows = append(rows,
			[]telegramInlineButton{
				{Text: "✏️ 改用户名", Data: "act_setname"},
				{Text: "🔑 改密码", Data: "act_setpass"},
			},
			[]telegramInlineButton{{Text: "🎟 兑换码续期", Data: "act_redeem_renew"}},
		)
	}
	return rows
}

func telegramAdminMenuRows() [][]telegramInlineButton {
	return [][]telegramInlineButton{
		{{Text: "—— 管理员 ——", Data: "noop"}},
		{
			{Text: "📊 容量/状态", Data: "adm_capacity"},
			{Text: "👥 用户管理", Data: "adm_users"},
		},
		{
			{Text: "🔓 开注设置", Data: "adm_openreg"},
			{Text: "🎟 生成兑换码", Data: "adm_gencode"},
		},
		{
			{Text: "⚙️ 设备策略", Data: "adm_devicepolicy"},
			{Text: "🛠 管理命令", Data: "adm_mgo_commands"},
		},
	}
}
