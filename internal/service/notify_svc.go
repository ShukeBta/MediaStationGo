// Package service — 通知服务事件分发引擎。
//
// NotifyService 管理所有通知渠道，根据事件类型将通知分发给
// 订阅了该事件的渠道。支持 4 种内置事件类型和 5 种通知渠道。
package service

import (
	"context"
	"encoding/json"
	"sync"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// 通知事件类型常量。
const (
	EventSubscriptionHit       = "subscription_hit"
	EventSubscriptionCompleted = "subscription_follow_completed"
	EventSubscriptionStopped   = "subscription_follow_stopped"
	EventDownloadComplete      = "download_complete"
	EventScrapeFailed          = "scrape_failed"
	EventSystemAlert           = "system_alert"
	EventLibraryIngest         = "library_ingest"
)

// NotifyEvent 是通知事件的数据结构。
type NotifyEvent struct {
	Type    string                 `json:"type"`
	Title   string                 `json:"title"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// NotifyProvider 定义通知渠道的发送接口。
type NotifyProvider interface {
	Send(ctx context.Context, cfg map[string]string, event NotifyEvent) error
	ValidateConfig(cfg map[string]string) error
}

// NotifyService 是事件驱动的通知分发引擎。
type NotifyService struct {
	log    *zap.Logger
	repo   *repository.Container
	crypto *CryptoService

	mu        sync.RWMutex
	providers map[string]NotifyProvider // type -> provider
}

// NewNotifyService 创建通知服务。
func NewNotifyService(log *zap.Logger, repo *repository.Container, crypto *CryptoService) *NotifyService {
	ns := &NotifyService{
		log:       log,
		repo:      repo,
		crypto:    crypto,
		providers: make(map[string]NotifyProvider),
	}
	// 注册内置 Provider
	ns.registerProviders()
	return ns
}

// registerProviders 注册所有内置通知 Provider。
func (s *NotifyService) registerProviders() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers["telegram"] = &TelegramProvider{}
	s.providers["wechat"] = &WechatProvider{}
	s.providers["bark"] = &BarkProvider{}
	s.providers["webhook"] = &WebhookProvider{}
	s.providers["email"] = &EmailProvider{}
}

// Dispatch 将事件分发给所有订阅了该事件类型的已启用渠道。
func (s *NotifyService) Dispatch(ctx context.Context, event NotifyEvent) {
	channels, err := s.repo.NotifyChannel.ListByEvent(ctx, event.Type)
	if err != nil {
		s.log.Error("failed to list channels for event",
			zap.String("event", event.Type),
			zap.Error(err),
		)
		return
	}

	for _, ch := range channels {
		go func(channel model.NotifyChannel) {
			if sendErr := s.sendToChannel(ctx, channel, event); sendErr != nil {
				s.log.Error("failed to send notification",
					zap.String("channel", channel.Name),
					zap.String("type", channel.Type),
					zap.String("event", event.Type),
					zap.Error(sendErr),
				)
			}
		}(ch)
	}
}

// SendTest 向指定渠道发送测试通知。
func (s *NotifyService) SendTest(ctx context.Context, channelID string) error {
	ch, err := s.repo.NotifyChannel.FindByID(ctx, channelID)
	if err != nil {
		return err
	}
	if ch == nil {
		return ErrNotifyChannelNotFound
	}

	testEvent := NotifyEvent{
		Type:    "test",
		Title:   "MediaStationGo 测试通知",
		Message: "这是一条测试通知，如果您看到此消息，说明通知渠道配置正确。",
	}

	return s.sendToChannel(ctx, *ch, testEvent)
}

// ValidateChannelConfig 验证渠道配置是否合法。
func (s *NotifyService) ValidateChannelConfig(channelType string, config map[string]string) error {
	s.mu.RLock()
	provider, ok := s.providers[channelType]
	s.mu.RUnlock()
	if !ok {
		return ErrUnknownNotifyType
	}
	return provider.ValidateConfig(config)
}

// GetProviderTypes 返回支持的通知渠道类型列表。
func (s *NotifyService) GetProviderTypes() []NotifyProviderInfo {
	return []NotifyProviderInfo{
		{Type: "telegram", Name: "Telegram", Description: "通过 Telegram Bot 发送消息"},
		{Type: "wechat", Name: "Server酱", Description: "通过 Server酱 推送到微信"},
		{Type: "bark", Name: "Bark", Description: "通过 Bark 推送到 iOS"},
		{Type: "webhook", Name: "Webhook", Description: "通过自定义 HTTP Webhook 发送"},
		{Type: "email", Name: "Email", Description: "通过 SMTP 发送邮件"},
	}
}

// NotifyProviderInfo 描述通知渠道类型信息。
type NotifyProviderInfo struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// sendToChannel 解密渠道配置并通过对应的 Provider 发送通知。
func (s *NotifyService) sendToChannel(ctx context.Context, channel model.NotifyChannel, event NotifyEvent) error {
	// 解密配置
	configStr := channel.Config
	if s.crypto != nil && configStr != "" {
		configStr = s.crypto.Decrypt(configStr)
	}

	var cfg map[string]string
	if err := json.Unmarshal([]byte(configStr), &cfg); err != nil {
		return err
	}

	s.mu.RLock()
	provider, ok := s.providers[channel.Type]
	s.mu.RUnlock()
	if !ok {
		return ErrUnknownNotifyType
	}

	return provider.Send(ctx, cfg, event)
}

// 通知服务错误定义。
var (
	ErrNotifyChannelNotFound = &NotifyError{Code: "CHANNEL_NOT_FOUND", Message: "notification channel not found"}
	ErrUnknownNotifyType     = &NotifyError{Code: "UNKNOWN_TYPE", Message: "unknown notification type"}
)

// NotifyError 是通知服务专用错误类型。
type NotifyError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error 实现 error 接口。
func (e *NotifyError) Error() string {
	return e.Message
}
