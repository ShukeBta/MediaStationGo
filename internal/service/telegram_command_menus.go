package service

import (
	"context"
	"strings"
	"time"
)

func telegramSupportedCommand(cmd string) bool {
	cmd = telegramCommandName(cmd)
	if cmd == "" {
		return false
	}
	_, ok := telegramSupportedCommandSet[cmd]
	return ok
}

var telegramSupportedCommandSet = map[string]struct{}{
	"/start": {}, "/menu": {}, "/cancel": {}, "/help": {}, "/hideadult": {}, "/hide_adult": {}, "/adult": {},
	"/account": {}, "/me": {}, "/myinfo": {}, "/count": {}, "/signin": {}, "/checkin": {}, "/devices": {}, "/kick": {}, "/setname": {}, "/rename": {}, "/setpass": {}, "/passwd": {}, "/password": {},
	"/redeem": {}, "/redeem_register": {}, "/redeem_renew": {},
	"/register": {}, "/reg": {}, "/signup": {}, "/registration": {}, "/reg_switch": {}, "/openreg": {},
	"/capacity": {}, "/users": {}, "/kk": {}, "/gencode": {}, "/renew_user": {}, "/delete_user": {}, "/unbind": {}, "/unbind_duplicates": {}, "/unbind_inactive": {},
	"/devicepolicy": {}, "/policy": {}, "/antishare": {}, "/cleanup": {}, "/cleanup_mode": {}, "/cleanup_rule": {},
	"/ban": {}, "/unban": {}, "/status": {}, "/search": {}, "/downloads": {}, "/stats": {},
	"/renew": {}, "/ucr": {}, "/uinfo": {}, "/rmemby": {}, "/urm": {}, "/only_rm_emby": {}, "/only_rm_record": {},
	"/userip": {}, "/udeviceid": {}, "/auditip": {}, "/auditdevice": {}, "/auditclient": {},
	"/renewall": {}, "/callall": {}, "/syncunbound": {}, "/syncgroupm": {}, "/kick_not_emby": {}, "/scan_embyname": {},
	"/check_ex": {}, "/deleted": {}, "/low_activity": {}, "/uranks": {}, "/days_ranks": {}, "/week_ranks": {},
	"/embyadmin": {}, "/unbanall": {}, "/banall": {}, "/embylibs_unblockall": {}, "/embylibs_blockall": {},
	"/extraembylibs_unblockall": {}, "/extraembylibs_blockall": {}, "/proadmin": {}, "/revadmin": {},
	"/backup_db": {}, "/restore_from_db": {}, "/prouser": {}, "/revuser": {},
}

type telegramBotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

func telegramBotCommandMenu() []telegramBotCommand {
	return telegramPrivateBotCommandMenu()
}

func telegramPrivateBotCommandMenu() []telegramBotCommand {
	return []telegramBotCommand{
		{Command: "start", Description: "绑定账号或打开主菜单"},
		{Command: "menu", Description: "打开功能菜单"},
		{Command: "help", Description: "查看命令帮助"},
		{Command: "account", Description: "查看账号状态"},
		{Command: "signin", Description: "签到"},
		{Command: "devices", Description: "查看登录设备"},
		{Command: "kick", Description: "踢下线设备"},
		{Command: "setname", Description: "修改用户名(需当前密码)"},
		{Command: "setpass", Description: "修改密码(需当前密码)"},
		{Command: "hideadult", Description: "隐藏/显示成人媒体库"},
		{Command: "redeem", Description: "兑换注册码或续期码"},
		{Command: "register", Description: "注册新账号"},
	}
}

func telegramGroupBotCommandMenu() []telegramBotCommand {
	return []telegramBotCommand{
		{Command: "start", Description: "打开群组自助菜单"},
		{Command: "menu", Description: "打开群组自助菜单"},
		{Command: "help", Description: "查看群组可用命令"},
		{Command: "account", Description: "查看账号状态"},
		{Command: "signin", Description: "签到"},
		{Command: "devices", Description: "查看登录设备"},
		{Command: "kick", Description: "踢下线设备"},
		{Command: "hideadult", Description: "隐藏/显示成人媒体库"},
	}
}

func telegramAdminBotCommandMenu() []telegramBotCommand {
	commands := append([]telegramBotCommand{}, telegramPrivateBotCommandMenu()...)
	commands = append(commands,
		telegramBotCommand{Command: "status", Description: "系统运行状态(管理员)"},
		telegramBotCommand{Command: "search", Description: "搜索媒体库(管理员)"},
		telegramBotCommand{Command: "downloads", Description: "下载列表(管理员)"},
		telegramBotCommand{Command: "stats", Description: "媒体库统计(管理员)"},
		telegramBotCommand{Command: "users", Description: "用户管理(管理员)"},
		telegramBotCommand{Command: "cleanup", Description: "保号清理预览/确认(管理员)"},
		telegramBotCommand{Command: "cleanup_mode", Description: "查看固定保号模式(管理员)"},
		telegramBotCommand{Command: "cleanup_rule", Description: "Mgo保号规则管理(管理员)"},
	)
	commands = append(commands, telegramMgoAdminBotCommandMenu()...)
	return commands
}

func telegramMgoAdminBotCommandMenu() []telegramBotCommand {
	return []telegramBotCommand{
		// 用户管理：保留 Sakura/Mgo 常用命令名，剔除 /urm、/only_rm_emby 等重复别名。
		{Command: "ucr", Description: "Mgo用户: 创建账号"},
		{Command: "uinfo", Description: "Mgo用户: 查询账号"},
		{Command: "rmemby", Description: "Mgo用户: 删除账号"},
		{Command: "only_rm_record", Description: "Mgo用户: 仅删Bot绑定"},
		{Command: "renewall", Description: "Mgo用户: 批量续期"},

		// 审计：按 IP、设备指纹、客户端和 Telegram 绑定信息排查共享。
		{Command: "userip", Description: "Mgo审计: 查询用户IP"},
		{Command: "auditip", Description: "Mgo审计: 按IP审计"},
		{Command: "auditdevice", Description: "Mgo审计: 按设备审计"},
		{Command: "auditclient", Description: "Mgo审计: 按客户端审计"},
		{Command: "udeviceid", Description: "Mgo审计: 按设备ID审计"},

		// 清理：/low_activity 是 /deleted 的兼容别名，不显示在命令栏。
		{Command: "syncunbound", Description: "Mgo清理: 未绑定账号"},
		{Command: "syncgroupm", Description: "Mgo清理: 校验群成员"},
		{Command: "check_ex", Description: "Mgo清理: 检查过期账号"},
		{Command: "deleted", Description: "Mgo清理: 保号清理预览"},

		// 权限：批量禁用、保护用户、媒体库播放权限。
		{Command: "embyadmin", Description: "Mgo权限: 设置管理员"},
		{Command: "banall", Description: "Mgo权限: 批量禁用用户"},
		{Command: "unbanall", Description: "Mgo权限: 批量解禁用户"},
		{Command: "prouser", Description: "Mgo权限: 加入保护名单"},
		{Command: "revuser", Description: "Mgo权限: 移出保护名单"},
		{Command: "embylibs_blockall", Description: "Mgo权限: 批量禁用媒体库"},
		{Command: "embylibs_unblockall", Description: "Mgo权限: 批量开放媒体库"},

		// 运维：Bot 管理员与数据库备份恢复。
		{Command: "proadmin", Description: "Mgo运维: 添加Bot管理员"},
		{Command: "revadmin", Description: "Mgo运维: 移除Bot管理员"},
		{Command: "backup_db", Description: "Mgo运维: 备份数据库"},
		{Command: "restore_from_db", Description: "Mgo运维: 恢复数据库"},
	}
}

func registerTelegramBotCommands(ctx context.Context, cfg map[string]string) error {
	if strings.TrimSpace(cfg["bot_token"]) == "" {
		return nil
	}
	normalizeTelegramConfig(cfg)
	if err := telegramSetBotCommands(ctx, cfg, telegramPrivateBotCommandMenu(), nil); err != nil {
		return err
	}
	if err := telegramSetBotCommands(ctx, cfg, telegramPrivateBotCommandMenu(), map[string]interface{}{"type": "all_private_chats"}); err != nil {
		return err
	}
	if err := telegramSetBotCommands(ctx, cfg, telegramGroupBotCommandMenu(), map[string]interface{}{"type": "all_group_chats"}); err != nil {
		return err
	}
	_ = telegramSetBotCommands(ctx, cfg, telegramAdminBotCommandMenu(), map[string]interface{}{"type": "all_chat_administrators"})

	adminCommands := telegramAdminBotCommandMenu()
	for _, adminID := range telegramConfiguredUserIDs(cfg["admin_user_ids"]) {
		_ = telegramSetBotCommands(ctx, cfg, adminCommands, map[string]interface{}{"type": "chat", "chat_id": adminID})
	}
	return nil
}

func telegramSetBotCommands(ctx context.Context, cfg map[string]string, commands []telegramBotCommand, scope map[string]interface{}) error {
	payload := map[string]interface{}{"commands": commands}
	if scope != nil {
		payload["scope"] = scope
	}
	return telegramPostJSON(ctx, cfg, "setMyCommands", payload, 15*time.Second)
}
