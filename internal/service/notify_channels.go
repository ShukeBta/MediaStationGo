// Package service — notification channel CRUD + multi-channel dispatch.
//
// The original NotifierService reads a single set of keys from the
// settings table. NotifyChannelService persists *named* channels in
// their own table so the operator can add multiple Telegram bots, Bark
// servers, etc. and pick which events flow to which channel.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// NotifyChannelService manages notify channels and dispatches messages.
type NotifyChannelService struct {
	log    *zap.Logger
	repo   *repository.Container
	client *http.Client
}

// NewNotifyChannelService is the constructor.
func NewNotifyChannelService(log *zap.Logger, repo *repository.Container) *NotifyChannelService {
	return &NotifyChannelService{
		log:    log,
		repo:   repo,
		client: NewExternalHTTPClient(10 * time.Second),
	}
}

// ChannelInput is the shape accepted by Create / Update. Config is a
// generic map; it gets serialised to JSON before being persisted.
type ChannelInput struct {
	Name    string         `json:"name" binding:"required"`
	Type    string         `json:"type" binding:"required"`
	Config  map[string]any `json:"config"`
	Events  []string       `json:"events"`
	Enabled *bool          `json:"enabled,omitempty"`
}

// channelView is the public shape — Config is decoded back to a map so
// the React form can edit it directly without unwrapping JSON twice.
type channelView struct {
	model.NotifyChannel
	Config map[string]any `json:"config"`
	Events []string       `json:"events"`
}

// toView decodes Config + Events from their persisted JSON strings.
func toView(n model.NotifyChannel) channelView {
	v := channelView{NotifyChannel: n}
	if n.Config != "" {
		_ = json.Unmarshal([]byte(n.Config), &v.Config)
	}
	if v.Config == nil {
		v.Config = map[string]any{}
	}
	if n.Events != "" {
		_ = json.Unmarshal([]byte(n.Events), &v.Events)
	}
	if v.Events == nil {
		v.Events = []string{}
	}
	return v
}

// List returns every channel as a decoded view.
func (s *NotifyChannelService) List(ctx context.Context) ([]channelView, error) {
	rows, err := s.repo.NotifyChannel.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]channelView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toView(r))
	}
	return out, nil
}

// Create persists a new channel.
func (s *NotifyChannelService) Create(ctx context.Context, in ChannelInput) (*channelView, error) {
	normalizeChannelInput(&in)
	if err := validateChannel(in); err != nil {
		return nil, err
	}
	cfgBlob, _ := json.Marshal(in.Config)
	evBlob, _ := json.Marshal(in.Events)
	n := &model.NotifyChannel{
		Name:    strings.TrimSpace(in.Name),
		Type:    in.Type,
		Config:  string(cfgBlob),
		Events:  string(evBlob),
		Enabled: true,
	}
	if in.Enabled != nil {
		n.Enabled = *in.Enabled
	}
	if err := s.repo.NotifyChannel.Create(ctx, n); err != nil {
		return nil, err
	}
	if n.Type == "telegram" && n.Enabled {
		if err := registerTelegramBotCommands(ctx, telegramStringConfigFromAny(in.Config)); err != nil && s.log != nil {
			s.log.Warn("telegram setMyCommands failed", zap.Error(sanitizeTelegramError(err)))
		}
	}
	v := toView(*n)
	return &v, nil
}

// Update applies a partial patch to an existing channel.
func (s *NotifyChannelService) Update(ctx context.Context, id string, in ChannelInput) (*channelView, error) {
	normalizeChannelInput(&in)
	if err := validateChannel(in); err != nil {
		return nil, err
	}
	cfgBlob, _ := json.Marshal(in.Config)
	evBlob, _ := json.Marshal(in.Events)
	patch := map[string]any{
		"name":   strings.TrimSpace(in.Name),
		"type":   in.Type,
		"config": string(cfgBlob),
		"events": string(evBlob),
	}
	if in.Enabled != nil {
		patch["enabled"] = *in.Enabled
	}
	// Fetch existing row, apply patch via repo Update
	existing, err := s.repo.NotifyChannel.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("channel not found")
	}
	existing.Name = patch["name"].(string)
	existing.Type = patch["type"].(string)
	existing.Config = patch["config"].(string)
	existing.Events = patch["events"].(string)
	if en, ok := patch["enabled"]; ok {
		existing.Enabled = en.(bool)
	}
	if err := s.repo.NotifyChannel.Update(ctx, existing); err != nil {
		return nil, err
	}
	if existing.Type == "telegram" && existing.Enabled {
		if err := registerTelegramBotCommands(ctx, telegramStringConfigFromAny(in.Config)); err != nil && s.log != nil {
			s.log.Warn("telegram setMyCommands failed", zap.Error(sanitizeTelegramError(err)))
		}
	}
	row, err := s.repo.NotifyChannel.FindByID(ctx, id)
	if err != nil || row == nil {
		return nil, err
	}
	v := toView(*row)
	return &v, nil
}

// Delete removes the channel.
func (s *NotifyChannelService) Delete(ctx context.Context, id string) error {
	return s.repo.NotifyChannel.Delete(ctx, id)
}

// Test sends a "测试通知" through a single channel.
func (s *NotifyChannelService) Test(ctx context.Context, id string) error {
	row, err := s.repo.NotifyChannel.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return errors.New("channel not found")
	}
	return s.dispatchOne(ctx, *row, "MediaStationGo 测试通知", "如果你看到这条消息,说明该通道工作正常。")
}

const (
	NotifyEventAll  = "__all__"
	NotifyEventNone = "__none__"
)

// Broadcast sends a message to every enabled channel that subscribes to
// `event`. Legacy empty Events values mean "all events"; the explicit
// NotifyEventNone sentinel means the channel stays enabled but receives no
// event push.
func (s *NotifyChannelService) Broadcast(ctx context.Context, title, body, event string) {
	s.BroadcastEvent(ctx, NotifyEvent{
		Type:    event,
		Title:   title,
		Message: body,
	})
}

// BroadcastEvent sends one structured event to every subscribed enabled
// channel. Rich channels such as Telegram can use Data fields for artwork and
// cleaner formatting while simpler channels keep receiving title/body text.
func (s *NotifyChannelService) BroadcastEvent(ctx context.Context, event NotifyEvent) {
	rows, err := s.repo.NotifyChannel.ListEnabled(ctx)
	if err != nil {
		s.log.Warn("notify list failed", zap.Error(err))
		return
	}
	for _, r := range rows {
		if !channelSubscribes(r, event.Type) {
			continue
		}
		if err := s.dispatchOneEvent(ctx, r, event); err != nil {
			s.log.Warn("notify dispatch failed", zap.String("channel", r.Name), zap.Error(err))
		}
	}
}

// validateChannel rejects obviously-malformed inputs early so the API
// returns a useful 400 rather than a database constraint error.
func validateChannel(in ChannelInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name required")
	}
	switch in.Type {
	case "telegram", "wechat", "bark", "webhook", "email":
	default:
		return fmt.Errorf("unsupported channel type %q", in.Type)
	}
	if in.Type == "telegram" {
		cfg := in.Config
		if str(cfg["bot_token"]) == "" {
			return errors.New("telegram bot_token required")
		}
		if str(cfg["admin_user_ids"]) == "" {
			return errors.New("telegram admin_user_ids required")
		}
	}
	return nil
}

func normalizeChannelInput(in *ChannelInput) {
	if in == nil || in.Type != "telegram" {
		return
	}
	if in.Config == nil {
		in.Config = map[string]any{}
	}
	chatID := str(in.Config["chat_id"])
	if chatID == "" {
		return
	}
	if strings.HasPrefix(chatID, "-") && str(in.Config["group_chat_id"]) == "" && str(in.Config["channel_chat_id"]) == "" && str(in.Config["command_chat_id"]) == "" {
		in.Config["group_chat_id"] = chatID
		return
	}
	if !strings.HasPrefix(chatID, "-") && str(in.Config["admin_user_ids"]) == "" {
		in.Config["admin_user_ids"] = chatID
	}
}

// str safely extracts a string from an interface{} loaded from JSON.
func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
