// Package service — Telegram Bot 交互命令服务。
//
// 处理通过 Telegram Bot API 接收的用户命令，提供系统状态查询、
// 媒体搜索、下载管理等功能。同时支持 Webhook 和 Long Polling 两种模式。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// TelegramUpdate 是 Telegram Bot API 推送的 update 对象。
type TelegramUpdate struct {
	UpdateID      int                    `json:"update_id"`
	Message       *TelegramMessage       `json:"message,omitempty"`
	CallbackQuery *TelegramCallbackQuery `json:"callback_query,omitempty"`
}

// TelegramMessage 是 Telegram 消息对象。
type TelegramMessage struct {
	MessageID int          `json:"message_id"`
	From      TelegramUser `json:"from"`
	Chat      TelegramChat `json:"chat"`
	Text      string       `json:"text,omitempty"`
	Date      int          `json:"date"`
}

type TelegramCallbackQuery struct {
	ID      string           `json:"id"`
	From    TelegramUser     `json:"from"`
	Message *TelegramMessage `json:"message,omitempty"`
	Data    string           `json:"data,omitempty"`
}

// TelegramUser 是 Telegram 用户对象。
type TelegramUser struct {
	ID        int    `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

// TelegramChat 是 Telegram 聊天对象。
type TelegramChat struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
}

type telegramCommandReply struct {
	Text    string
	Buttons [][]telegramInlineButton
}

type telegramInlineButton struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

// TelegramBotService 处理 Telegram Bot 的交互命令。
type TelegramBotService struct {
	log    *zap.Logger
	repo   *repository.Container
	crypto *CryptoService
	auth   *AuthService
	device *DeviceService
	backup *BackupService

	pollingMu     sync.Mutex
	pollingCancel map[string]context.CancelFunc // bot_token -> cancel

	pendingMu sync.Mutex
	pending   map[int64]pendingInput // telegram_user_id -> awaited text input
}

// pendingInput tracks a button-initiated action that awaits the user's next
// text message (e.g. tapping「注册」then sending "用户名 密码").
type pendingInput struct {
	Kind      string // register / redeem_register / redeem_renew / setname / setpass / openreg_limit / gencode_user
	CreatedAt time.Time
}

// SetDeviceService wires the device-management service used by the device
// menu (list / kick) and enforcement notifications.
func (s *TelegramBotService) SetDeviceService(d *DeviceService) { s.device = d }

// SetBackupService wires database backup/restore commands.
func (s *TelegramBotService) SetBackupService(b *BackupService) { s.backup = b }

// NotifyUserByID sends a Telegram message to the local user identified by
// userID, resolved through their Telegram binding. Used by enforcement to warn
// users before destructive actions. No-op when the user has no binding.
func (s *TelegramBotService) NotifyUserByID(ctx context.Context, userID, text string) {
	if userID == "" || strings.TrimSpace(text) == "" {
		return
	}
	var binding model.TelegramBinding
	if err := s.repo.DB.WithContext(ctx).Where("user_id = ?", userID).First(&binding).Error; err != nil {
		return
	}
	targetChatID := telegramPrivateChatIDFromBinding(binding)
	if targetChatID == 0 {
		return
	}
	channel := s.findChannelByChatID(ctx, int(binding.ChatID))
	if channel == nil {
		channels, err := s.repo.NotifyChannel.ListByType(ctx, "telegram")
		if err != nil || len(channels) == 0 {
			return
		}
		channel = &channels[0]
	}
	_ = s.reply(ctx, channel, int(targetChatID), telegramCommandReply{Text: text})
}

// NewTelegramBotService 创建 Telegram Bot 服务。
func NewTelegramBotService(log *zap.Logger, repo *repository.Container, crypto *CryptoService, auth *AuthService) *TelegramBotService {
	return &TelegramBotService{
		log:           log,
		repo:          repo,
		crypto:        crypto,
		auth:          auth,
		pollingCancel: make(map[string]context.CancelFunc),
		pending:       make(map[int64]pendingInput),
	}
}

// TelegramRegistrationSettingKey 控制普通用户是否可以通过 Bot 注册新账号。
// 默认关闭，只有管理员在系统设置 / Bot 管理命令中显式开启后才允许注册。
const TelegramRegistrationSettingKey = "telegram.registration_enabled"

var errTelegramAccountAlreadyBound = errors.New("该媒体账号已绑定其他 Telegram，请联系管理员解绑")

// registrationEnabled 读取注册开关；默认关闭。
func (s *TelegramBotService) registrationEnabled(ctx context.Context) bool {
	v, err := s.repo.Setting.Get(ctx, TelegramRegistrationSettingKey)
	if err != nil {
		return false
	}
	return parseBoolSetting(v, false)
}

// setRegistrationEnabled 持久化注册开关。
func (s *TelegramBotService) setRegistrationEnabled(ctx context.Context, enabled bool) error {
	return s.repo.Setting.Set(ctx, TelegramRegistrationSettingKey, strconv.FormatBool(enabled))
}

// HandleWebhook 处理 Telegram 推送的 Webhook/Polling 消息。
func (s *TelegramBotService) HandleWebhook(ctx context.Context, body []byte) error {
	var update TelegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		return fmt.Errorf("invalid update: %w", err)
	}
	return s.handleTelegramUpdate(ctx, update, nil)
}

func (s *TelegramBotService) handleTelegramUpdate(ctx context.Context, update TelegramUpdate, channelHint *model.NotifyChannel) error {
	if update.CallbackQuery != nil {
		return s.handleCallback(ctx, update.CallbackQuery, channelHint)
	}

	if update.Message == nil || update.Message.Text == "" {
		return nil
	}

	msg := update.Message
	text := strings.TrimSpace(msg.Text)

	// Button-initiated text prompts (register / redeem / change name·password /
	// open-reg limit) arrive as ordinary messages. Consume them here before the
	// command gate so the button-driven menu can collect free-form input.
	if !telegramIsCommandText(text) {
		if msg.Chat.Type == "" || msg.Chat.Type == "private" {
			if channel := s.channelForMessage(ctx, msg, channelHint); channel != nil {
				if reply, handled := s.handlePendingText(ctx, channel, msg, text); handled {
					if reply.Text != "" {
						if err := s.reply(ctx, channel, msg.Chat.ID, reply); err != nil {
							s.log.Error("reply failed", zap.Error(err))
						}
					}
					s.deleteTelegramSourceMessage(channel, msg.Chat.ID, msg.MessageID)
					return nil
				}
				if looksLikeRedemptionCode(text) {
					reply := s.cmdRedeem(ctx, channel, msg, []string{text})
					if reply.Text != "" {
						if err := s.reply(ctx, channel, msg.Chat.ID, reply); err != nil {
							s.log.Error("reply failed", zap.Error(err))
						}
					}
					s.deleteTelegramSourceMessage(channel, msg.Chat.ID, msg.MessageID)
					return nil
				}
			}
		}
		return nil
	}
	if msg.Chat.Type != "" && msg.Chat.Type != "private" && !telegramSupportedCommand(telegramCommandName(text)) {
		return nil
	}

	s.log.Info("telegram command received",
		zap.Int("chat_id", msg.Chat.ID),
		zap.String("user", msg.From.Username),
		zap.String("text", text),
	)

	// 获取该消息可使用的 Telegram 通知渠道配置。群组/频道消息必须来自
	// 已配置的群组/频道；私聊消息会选择一个可验证该用户成员身份的 Bot。
	channel := s.channelForMessage(ctx, msg, channelHint)
	if channel == nil {
		s.log.Warn("telegram channel not allowed or not configured",
			zap.Int("chat_id", msg.Chat.ID),
			zap.String("chat_type", msg.Chat.Type),
			zap.Int("telegram_user_id", msg.From.ID),
		)
		return nil
	}

	// 解析并执行命令
	reply, err := s.executeCommand(ctx, channel, msg, text)
	if err != nil {
		s.log.Error("command failed", zap.Error(err))
		_ = s.replyForMessage(ctx, channel, msg, telegramCommandReply{Text: "命令执行失败: " + err.Error()})
		s.deleteTelegramSourceMessage(channel, msg.Chat.ID, msg.MessageID)
		return nil
	}

	if reply.Text != "" {
		if err := s.replyForMessage(ctx, channel, msg, reply); err != nil {
			s.log.Error("reply failed", zap.Error(err))
		}
		s.deleteTelegramSourceMessage(channel, msg.Chat.ID, msg.MessageID)
	}

	return nil
}

func telegramIsCommandText(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/") && telegramCommandName(text) != ""
}

func telegramCommandName(text string) string {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return ""
	}
	cmd := strings.ToLower(strings.TrimSpace(parts[0]))
	if !strings.HasPrefix(cmd, "/") {
		return ""
	}
	if at := strings.Index(cmd, "@"); at > 0 {
		cmd = cmd[:at]
	}
	return cmd
}

func telegramIsGroupChat(chatType string) bool {
	return chatType != "" && chatType != "private"
}

func telegramPrivateMessageForUser(msg *TelegramMessage) *TelegramMessage {
	if msg == nil || !telegramIsGroupChat(msg.Chat.Type) {
		return msg
	}
	copied := *msg
	copied.Chat = TelegramChat{ID: msg.From.ID, Type: "private"}
	return &copied
}

func telegramGroupPrivateAdminHint() string {
	return "群组内不展示管理面板；管理员可在已绑定群组直接发送文本管理命令，涉及账号凭据的操作仍请私聊 Bot。"
}

func telegramGroupPrivateUserHint(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		action = "此操作"
	}
	return action + "包含账号凭据或敏感信息，请私聊 Bot 操作；群组内仅开放账号状态、签到、设备与成人目录开关。"
}

func telegramGroupPrivateDeliverySentHint() string {
	return "已把你的 Bot 面板/执行结果私聊发送给你。若没收到，请先私聊 Bot 发送 <code>/start</code>。"
}

func telegramGroupPrivateDeliveryFailedHint() string {
	return "无法私聊发送给你。请先打开 Bot 私聊窗口发送 <code>/start</code>，再回群里使用命令。"
}
