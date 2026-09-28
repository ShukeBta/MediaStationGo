package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm/clause"
)

// ErrTelegramWebhookUnauthorized denotes an absent or invalid Telegram source secret.
var ErrTelegramWebhookUnauthorized = errors.New("invalid Telegram webhook credentials")

func telegramWebhookSecretKey(botToken string) string {
	digest := sha256.Sum256([]byte(botToken))
	return "telegram.webhook." + hex.EncodeToString(digest[:]) + ".secret"
}

// Persist before contacting Telegram and reuse the value on retries/restarts. An
// unsuccessful registration must not rotate a secret used by an existing webhook.
func (s *TelegramBotService) ensureTelegramWebhookSecret(ctx context.Context, botToken string) (string, error) {
	key := telegramWebhookSecretKey(botToken)
	secret, err := s.repo.Setting.Get(ctx, key)
	if err != nil || secret != "" {
		return secret, err
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	setting := model.Setting{Key: key, Value: hex.EncodeToString(random[:]), UpdatedAt: time.Now()}
	// Concurrent registrations must converge on one secret, including across processes.
	if err := s.repo.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&setting).Error; err != nil {
		return "", err
	}
	secret, err = s.repo.Setting.Get(ctx, key)
	if err != nil {
		return "", err
	}
	if secret == "" {
		return "", errors.New("Telegram webhook secret is unavailable")
	}
	return secret, nil
}

// HandleAuthenticatedWebhook verifies the Telegram secret before parsing or
// dispatching an update. Matching channels are restricted to the authenticated
// Bot, so a sender/chat ID cannot select another Bot's administrator settings.
func (s *TelegramBotService) HandleAuthenticatedWebhook(ctx context.Context, body []byte, secret string) error {
	if secret == "" {
		return ErrTelegramWebhookUnauthorized
	}
	channels, err := s.repo.NotifyChannel.ListByType(ctx, "telegram")
	if err != nil {
		return err
	}
	var authenticated []model.NotifyChannel
	for i := range channels {
		if !channels[i].Enabled {
			continue
		}
		botToken := strings.TrimSpace(s.telegramChannelConfig(&channels[i])["bot_token"])
		if botToken == "" {
			continue
		}
		expected, err := s.repo.Setting.Get(ctx, telegramWebhookSecretKey(botToken))
		if err != nil {
			return err
		}
		if expected != "" && subtle.ConstantTimeCompare([]byte(expected), []byte(secret)) == 1 {
			authenticated = append(authenticated, channels[i])
		}
	}
	if len(authenticated) == 0 {
		return ErrTelegramWebhookUnauthorized
	}
	var update TelegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		return fmt.Errorf("invalid update: %w", err)
	}
	msg := update.Message
	if update.CallbackQuery != nil {
		if update.CallbackQuery.Message == nil {
			return nil
		}
		copied := *update.CallbackQuery.Message
		copied.From = update.CallbackQuery.From
		msg = &copied
	}
	if msg == nil {
		return nil
	}
	var channel *model.NotifyChannel
	for i := range authenticated {
		candidate := &authenticated[i]
		if telegramIsGroupChat(msg.Chat.Type) {
			if s.telegramChatAllowed(candidate, msg.Chat.ID) {
				channel = candidate
				break
			}
			continue
		}
		if channel == nil {
			channel = candidate
		}
		if s.telegramUserIsAdmin(ctx, candidate, msg.From.ID) || s.telegramUserCanBind(ctx, candidate, msg.From.ID) {
			channel = candidate
			break
		}
	}
	if channel == nil {
		return nil
	}
	return s.handleTelegramUpdate(ctx, update, channel)
}
