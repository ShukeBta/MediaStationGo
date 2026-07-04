package service

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) telegramBinding(ctx context.Context, telegramUserID int) *model.TelegramBinding {
	if telegramUserID == 0 {
		return nil
	}
	var binding model.TelegramBinding
	err := s.repo.DB.WithContext(ctx).Where("telegram_user_id = ?", int64(telegramUserID)).First(&binding).Error
	if err != nil {
		return nil
	}
	return &binding
}

func (s *TelegramBotService) unbindTelegramUser(ctx context.Context, telegramUserID int) error {
	if s == nil || s.repo == nil || s.repo.DB == nil || telegramUserID == 0 {
		return nil
	}
	return s.repo.DB.WithContext(ctx).Unscoped().
		Where("telegram_user_id = ?", int64(telegramUserID)).
		Delete(&model.TelegramBinding{}).Error
}

func (s *TelegramBotService) upsertTelegramBinding(ctx context.Context, msg *TelegramMessage, userID string) error {
	name := strings.TrimSpace(msg.From.FirstName)
	if msg.From.Username != "" {
		name = "@" + strings.TrimSpace(msg.From.Username)
	}
	telegramUserID := int64(msg.From.ID)
	return s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.TelegramBinding
		err := tx.Where("telegram_user_id = ?", telegramUserID).First(&existing).Error
		if err == nil {
			if err := s.replaceTelegramAccountBindingTx(ctx, tx, userID, telegramUserID); err != nil {
				return err
			}
			if err := tx.Model(&existing).Updates(map[string]any{
				"telegram_name": name,
				"chat_id":       telegramBindingChatIDForMessage(msg, &existing),
				"user_id":       userID,
			}).Error; telegramBindingUniqueErr(err) {
				return errTelegramAccountAlreadyBound
			} else if err != nil {
				return err
			}
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Unscoped().Where("telegram_user_id = ?", telegramUserID).Delete(&model.TelegramBinding{}).Error; err != nil {
			return err
		}
		if err := s.replaceTelegramAccountBindingTx(ctx, tx, userID, telegramUserID); err != nil {
			return err
		}
		err = tx.Create(&model.TelegramBinding{
			TelegramUserID: telegramUserID,
			TelegramName:   name,
			ChatID:         telegramBindingChatIDForMessage(msg, nil),
			UserID:         userID,
		}).Error
		if telegramBindingUniqueErr(err) {
			return errTelegramAccountAlreadyBound
		}
		return err
	})
}

func telegramBindingChatIDForMessage(msg *TelegramMessage, existing *model.TelegramBinding) int64 {
	if msg == nil {
		if existing != nil {
			return existing.ChatID
		}
		return 0
	}
	if msg.Chat.Type == "" || msg.Chat.Type == "private" {
		return int64(msg.Chat.ID)
	}
	if existing != nil && existing.ChatID > 0 {
		return existing.ChatID
	}
	return int64(msg.From.ID)
}

func telegramPrivateChatIDFromBinding(binding model.TelegramBinding) int64 {
	if binding.ChatID > 0 {
		return binding.ChatID
	}
	return binding.TelegramUserID
}

func (s *TelegramBotService) replaceTelegramAccountBindingTx(ctx context.Context, tx *gorm.DB, userID string, telegramUserID int64) error {
	return tx.WithContext(ctx).Unscoped().
		Where("user_id = ? AND telegram_user_id <> ?", userID, telegramUserID).
		Delete(&model.TelegramBinding{}).Error
}

func telegramBindingUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "idx_telegram_bindings_user_id_active") ||
		strings.Contains(msg, "telegram_bindings.user_id") ||
		(strings.Contains(msg, "unique") && strings.Contains(msg, "telegram_bindings"))
}

func parseStartCredentials(args []string) (string, string) {
	if len(args) >= 2 {
		return strings.TrimSpace(args[0]), strings.TrimSpace(strings.Join(args[1:], " "))
	}
	if len(args) == 1 {
		raw := strings.TrimSpace(args[0])
		for _, sep := range []string{"-", "：", ":"} {
			if parts := strings.SplitN(raw, sep, 2); len(parts) == 2 {
				return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			}
		}
	}
	return "", ""
}

func userNameOrFallback(user *model.User) string {
	if user == nil || strings.TrimSpace(user.Username) == "" {
		return "未知用户"
	}
	return user.Username
}
