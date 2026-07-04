package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// channelSubscribes returns true when the channel's Events list contains the
// event, or when the list is the legacy empty/"all events" value.
func channelSubscribes(n model.NotifyChannel, event string) bool {
	if event == "" || n.Events == "" {
		return true
	}
	var ev []string
	if err := json.Unmarshal([]byte(n.Events), &ev); err != nil {
		return true
	}
	if len(ev) == 0 {
		return true
	}
	for _, e := range ev {
		switch e {
		case NotifyEventNone:
			return false
		case NotifyEventAll:
			return true
		}
		if e == event {
			return true
		}
	}
	return false
}

// dispatchOne is the inner dispatcher; the channel type drives which
// HTTP request gets built.
func (s *NotifyChannelService) dispatchOne(ctx context.Context, n model.NotifyChannel, title, body string) error {
	return s.dispatchOneEvent(ctx, n, NotifyEvent{Title: title, Message: body})
}

func (s *NotifyChannelService) dispatchOneEvent(ctx context.Context, n model.NotifyChannel, event NotifyEvent) error {
	cfg := map[string]any{}
	_ = json.Unmarshal([]byte(n.Config), &cfg)
	title := event.Title
	body := event.Message

	switch n.Type {
	case "telegram":
		telegramCfg := telegramStringConfigFromAny(cfg)
		token := telegramCfg["bot_token"]
		chats := telegramTargetChatIDs(telegramCfg)
		if token == "" || len(chats) == 0 {
			return errors.New("telegram missing bot_token / group_chat_id / channel_chat_id")
		}
		text := formatTelegramNotification(event)
		photoURL := telegramEventPhotoURL(event)
		var firstErr error
		for _, chat := range chats {
			if photoURL != "" && len(text) <= 1024 {
				form := url.Values{}
				form.Set("chat_id", chat)
				form.Set("photo", photoURL)
				form.Set("caption", text)
				form.Set("parse_mode", "HTML")
				if err := telegramPostForm(ctx, telegramCfg, "sendPhoto", form, 15*time.Second); err == nil {
					continue
				} else if firstErr == nil {
					firstErr = err
				}
			}
			form := url.Values{}
			form.Set("chat_id", chat)
			form.Set("text", text)
			form.Set("parse_mode", "HTML")
			if err := telegramPostForm(ctx, telegramCfg, "sendMessage", form, 15*time.Second); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr

	case "bark":
		key := str(cfg["device_key"])
		if key == "" {
			return errors.New("bark missing device_key")
		}
		server := str(cfg["server"])
		if server == "" {
			server = "https://api.day.app"
		}
		u := fmt.Sprintf("%s/%s/%s/%s",
			strings.TrimRight(server, "/"),
			url.PathEscape(key),
			url.PathEscape(title),
			url.PathEscape(body),
		)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		return s.do(req)

	case "wechat":
		key := str(cfg["sendkey"])
		if key == "" {
			return errors.New("wechat missing sendkey")
		}
		u := fmt.Sprintf("https://sctapi.ftqq.com/%s.send", url.PathEscape(key))
		form := url.Values{}
		form.Set("title", title)
		form.Set("desp", body)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return s.do(req)

	case "webhook":
		urlS := str(cfg["url"])
		if urlS == "" {
			return errors.New("webhook missing url")
		}
		method := strings.ToUpper(str(cfg["method"]))
		if method == "" {
			method = "POST"
		}
		// Substitute {{title}} / {{message}} in the body template.
		bodyTpl := str(cfg["body_template"])
		if bodyTpl == "" {
			bodyTpl = `{"title":"{{title}}","message":"{{message}}"}`
		}
		bodyStr := strings.NewReplacer("{{title}}", title, "{{message}}", body).Replace(bodyTpl)
		req, _ := http.NewRequestWithContext(ctx, method, urlS, strings.NewReader(bodyStr))
		// Apply custom headers (encoded as JSON in the config).
		if hdrRaw := str(cfg["headers"]); hdrRaw != "" {
			var hdr map[string]string
			if err := json.Unmarshal([]byte(hdrRaw), &hdr); err == nil {
				for k, v := range hdr {
					req.Header.Set(k, v)
				}
			}
		}
		if req.Header.Get("Content-Type") == "" && method != http.MethodGet {
			req.Header.Set("Content-Type", "application/json")
		}
		return s.do(req)
	}
	return fmt.Errorf("unknown channel type %q", n.Type)
}

func (s *NotifyChannelService) do(req *http.Request) error {
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("upstream returned %d", resp.StatusCode)
	}
	return nil
}
