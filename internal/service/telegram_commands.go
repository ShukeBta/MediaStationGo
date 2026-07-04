// Package service — Telegram command registry and dispatch.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type telegramCommandHandler func(args []string) (telegramCommandReply, error)

type telegramCommandDefinition struct {
	Aliases       []string
	AdminOnly     bool
	AdminOnlyText string
	GroupAllowed  bool
	Handle        telegramCommandHandler
}

func (s *TelegramBotService) telegramCommandDefinitions(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage) []telegramCommandDefinition {
	adminOnly := "此命令仅管理员可用。"
	defs := s.telegramCoreCommandDefinitions(ctx, channel, msg)
	defs = append(defs, s.telegramSelfServiceCommandDefinitions(ctx, channel, msg)...)
	defs = append(defs, s.telegramAdminCoreCommandDefinitions(ctx, msg, adminOnly)...)
	defs = append(defs, s.telegramMgoUserCommandDefinitions(ctx, adminOnly)...)
	defs = append(defs, s.telegramMgoAuditCommandDefinitions(ctx, adminOnly)...)
	defs = append(defs, s.telegramMgoMaintenanceCommandDefinitions(ctx, channel, adminOnly)...)
	defs = append(defs, s.telegramMgoPolicyCommandDefinitions(ctx, channel, adminOnly)...)
	return defs
}

func (s *TelegramBotService) telegramCoreCommandDefinitions(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage) []telegramCommandDefinition {
	return []telegramCommandDefinition{
		{Aliases: []string{"/start"}, GroupAllowed: true, Handle: func(args []string) (telegramCommandReply, error) {
			if len(args) == 0 {
				return s.mainMenu(ctx, channel, msg), nil
			}
			return s.cmdStart(ctx, msg, args), nil
		}},
		{Aliases: []string{"/menu"}, GroupAllowed: true, Handle: func(args []string) (telegramCommandReply, error) {
			return s.mainMenu(ctx, channel, msg), nil
		}},
		{Aliases: []string{"/cancel"}, GroupAllowed: true, Handle: func(args []string) (telegramCommandReply, error) {
			s.takePending(int64(msg.From.ID))
			return telegramCommandReply{Text: "已取消当前操作。"}, nil
		}},
		{Aliases: []string{"/help"}, GroupAllowed: true, Handle: func(args []string) (telegramCommandReply, error) {
			return telegramCommandReply{Text: s.cmdHelp(ctx, msg)}, nil
		}},
	}
}

func (s *TelegramBotService) telegramSelfServiceCommandDefinitions(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage) []telegramCommandDefinition {
	return []telegramCommandDefinition{
		{Aliases: []string{"/hideadult", "/hide_adult", "/adult"}, GroupAllowed: true, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdHideAdult(ctx, msg, args), nil }},
		{Aliases: []string{"/account", "/me", "/myinfo"}, GroupAllowed: true, Handle: func(args []string) (telegramCommandReply, error) { return s.replyAccount(ctx, msg), nil }},
		{Aliases: []string{"/count"}, GroupAllowed: true, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdStats(ctx) }},
		{Aliases: []string{"/signin", "/checkin"}, GroupAllowed: true, Handle: func(args []string) (telegramCommandReply, error) { return s.replySignIn(ctx, msg), nil }},
		{Aliases: []string{"/devices"}, GroupAllowed: true, Handle: func(args []string) (telegramCommandReply, error) { return s.replyDevices(ctx, msg), nil }},
		{Aliases: []string{"/kick"}, GroupAllowed: true, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdKick(ctx, msg, args), nil }},
		{Aliases: []string{"/setname", "/rename"}, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdSetName(ctx, msg, args), nil }},
		{Aliases: []string{"/setpass", "/passwd", "/password"}, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdSetPass(ctx, msg, args), nil }},
		{Aliases: []string{"/redeem"}, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdRedeem(ctx, channel, msg, args), nil }},
		{Aliases: []string{"/redeem_register"}, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdRedeemRegister(ctx, channel, msg, args), nil
		}},
		{Aliases: []string{"/redeem_renew"}, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdRedeemRenew(ctx, msg, args), nil }},
		{Aliases: []string{"/register", "/reg", "/signup"}, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdRegister(ctx, channel, msg, args), nil }},
	}
}

func (s *TelegramBotService) telegramAdminCoreCommandDefinitions(ctx context.Context, msg *TelegramMessage, adminOnly string) []telegramCommandDefinition {
	return []telegramCommandDefinition{
		{Aliases: []string{"/registration", "/reg_switch", "/openreg"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdRegistrationToggle(ctx, args), nil }},
		{Aliases: []string{"/capacity"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.replyCapacity(ctx), nil }},
		{Aliases: []string{"/users", "/kk"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.replyUserList(ctx), nil }},
		{Aliases: []string{"/gencode"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdGenCode(ctx, msg, args), nil }},
		{Aliases: []string{"/renew_user"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdUserRenew(ctx, args), nil }},
		{Aliases: []string{"/delete_user"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdUserDelete(ctx, args), nil }},
		{Aliases: []string{"/unbind"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdUnbind(ctx, args), nil }},
		{Aliases: []string{"/unbind_duplicates"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdUnbindDuplicates(ctx), nil }},
		{Aliases: []string{"/unbind_inactive"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdUnbindInactive(ctx, args), nil }},
		{Aliases: []string{"/devicepolicy", "/policy"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdDevicePolicy(ctx, args), nil }},
		{Aliases: []string{"/antishare"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdAntiShare(ctx, args), nil }},
		{Aliases: []string{"/cleanup"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdCleanup(ctx, args), nil }},
		{Aliases: []string{"/cleanup_mode"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdCleanupMode(ctx, args), nil }},
		{Aliases: []string{"/cleanup_rule"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdCleanupRule(ctx, args), nil }},
		{Aliases: []string{"/ban"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdUserBan(ctx, args, false), nil }},
		{Aliases: []string{"/unban"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdUserBan(ctx, args, true), nil }},
		{Aliases: []string{"/status"}, AdminOnly: true, AdminOnlyText: "此命令仅管理员可用。普通用户只能使用 /start 绑定账号，并通过按钮隐藏成人目录。", Handle: func(args []string) (telegramCommandReply, error) { return s.cmdStatus(ctx) }},
		{Aliases: []string{"/search"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdSearch(ctx, args) }},
		{Aliases: []string{"/downloads"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdDownloads(ctx) }},
		{Aliases: []string{"/stats"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdStats(ctx) }},
		{Aliases: []string{"/renew"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdUserRenew(ctx, args), nil }},
	}
}

func (s *TelegramBotService) telegramMgoUserCommandDefinitions(ctx context.Context, adminOnly string) []telegramCommandDefinition {
	return []telegramCommandDefinition{
		{Aliases: []string{"/ucr"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoCreateUser(ctx, args), nil }},
		{Aliases: []string{"/uinfo"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoUserInfo(ctx, args), nil }},
		{Aliases: []string{"/rmemby", "/urm", "/only_rm_emby"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoDeleteUser(ctx, args), nil }},
		{Aliases: []string{"/only_rm_record"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoOnlyRemoveRecord(ctx, args), nil }},
		{Aliases: []string{"/userip"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoUserIP(ctx, args), nil }},
	}
}

func (s *TelegramBotService) telegramMgoAuditCommandDefinitions(ctx context.Context, adminOnly string) []telegramCommandDefinition {
	return []telegramCommandDefinition{
		{Aliases: []string{"/udeviceid"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoAuditDevices(ctx, "udeviceid", args), nil
		}},
		{Aliases: []string{"/auditip"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoAuditDevices(ctx, "auditip", args), nil
		}},
		{Aliases: []string{"/auditdevice"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoAuditDevices(ctx, "auditdevice", args), nil
		}},
		{Aliases: []string{"/auditclient"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoAuditDevices(ctx, "auditclient", args), nil
		}},
	}
}

func (s *TelegramBotService) telegramMgoMaintenanceCommandDefinitions(ctx context.Context, channel *model.NotifyChannel, adminOnly string) []telegramCommandDefinition {
	return []telegramCommandDefinition{
		{Aliases: []string{"/renewall"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoRenewAll(ctx, args), nil }},
		{Aliases: []string{"/callall"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoCallAll(ctx, channel, args), nil }},
		{Aliases: []string{"/syncunbound"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoSyncUnbound(ctx, args), nil }},
		{Aliases: []string{"/syncgroupm"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoSyncGroup(ctx, channel, args), nil
		}},
		{Aliases: []string{"/kick_not_emby"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoUnsupported("群内无号用户清理", "<code>/syncgroupm</code> 可检查已绑定账号是否仍在群内；Telegram Bot API 无法枚举全部群成员，因此不能可靠找出“在群但无号”的用户。"), nil
		}},
		{Aliases: []string{"/scan_embyname"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoScanNames(ctx), nil }},
		{Aliases: []string{"/check_ex"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoCheckExpired(ctx, args), nil }},
		{Aliases: []string{"/deleted", "/low_activity"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdCleanup(ctx, []string{"run"}), nil }},
		{Aliases: []string{"/uranks"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoRanks(ctx, 0, true), nil }},
		{Aliases: []string{"/days_ranks"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoRanks(ctx, 24*time.Hour, false), nil
		}},
		{Aliases: []string{"/week_ranks"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoRanks(ctx, 7*24*time.Hour, false), nil
		}},
	}
}

func (s *TelegramBotService) telegramMgoPolicyCommandDefinitions(ctx context.Context, channel *model.NotifyChannel, adminOnly string) []telegramCommandDefinition {
	return []telegramCommandDefinition{
		{Aliases: []string{"/embyadmin"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoAdminRole(ctx, args), nil }},
		{Aliases: []string{"/unbanall"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoBanAll(ctx, true, args), nil }},
		{Aliases: []string{"/banall"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoBanAll(ctx, false, args), nil }},
		{Aliases: []string{"/embylibs_unblockall", "/extraembylibs_unblockall"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoMediaAccessAll(ctx, true), nil
		}},
		{Aliases: []string{"/embylibs_blockall", "/extraembylibs_blockall"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoMediaAccessAll(ctx, false), nil
		}},
		{Aliases: []string{"/proadmin"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoBotAdmin(ctx, channel, args, true), nil
		}},
		{Aliases: []string{"/revadmin"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoBotAdmin(ctx, channel, args, false), nil
		}},
		{Aliases: []string{"/backup_db"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoBackupDB(ctx), nil }},
		{Aliases: []string{"/restore_from_db"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) { return s.cmdMgoRestoreDB(ctx, args), nil }},
		{Aliases: []string{"/prouser"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoProtectedUser(ctx, args, true), nil
		}},
		{Aliases: []string{"/revuser"}, AdminOnly: true, AdminOnlyText: adminOnly, Handle: func(args []string) (telegramCommandReply, error) {
			return s.cmdMgoProtectedUser(ctx, args, false), nil
		}},
	}
}

func (s *TelegramBotService) telegramCommandRegistry(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage) map[string]telegramCommandDefinition {
	defs := s.telegramCommandDefinitions(ctx, channel, msg)
	registry := make(map[string]telegramCommandDefinition, len(defs)*2)
	for _, def := range defs {
		for _, alias := range def.Aliases {
			registry[alias] = def
		}
	}
	return registry
}

// executeCommand parses and dispatches Telegram commands through a registry so
// adding a command does not grow a monolithic switch.
func (s *TelegramBotService) executeCommand(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, text string) (telegramCommandReply, error) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return telegramCommandReply{}, nil
	}

	cmd := telegramCommandName(parts[0])
	args := parts[1:]
	if msg.Chat.Type != "" && msg.Chat.Type != "private" && !s.telegramChatAllowed(channel, msg.Chat.ID) {
		return telegramCommandReply{Text: "此群组/频道未绑定到 Bot 管理入口，请在通知渠道里填写「绑定群组 ID」或「绑定频道 ID」。"}, nil
	}

	def, ok := s.telegramCommandRegistry(ctx, channel, msg)[cmd]
	if !ok {
		return telegramCommandReply{Text: fmt.Sprintf("未知命令: %s\n\n输入 /help 查看可用命令列表。", cmd)}, nil
	}
	if telegramIsGroupChat(msg.Chat.Type) && !def.GroupAllowed {
		if !def.AdminOnly || !s.telegramUserIsAdmin(ctx, channel, msg.From.ID) {
			return telegramCommandReply{}, nil
		}
	}
	if def.AdminOnly && !s.telegramUserIsAdmin(ctx, channel, msg.From.ID) {
		return telegramCommandReply{Text: def.AdminOnlyText}, nil
	}
	reply, err := def.Handle(args)
	if telegramIsGroupChat(msg.Chat.Type) && def.AdminOnly && !def.GroupAllowed {
		if !s.telegramUserIsAdmin(ctx, channel, msg.From.ID) {
			reply.Buttons = nil
		}
	}
	return reply, err
}
