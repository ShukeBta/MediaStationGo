package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func telegramPostForm(ctx context.Context, cfg map[string]string, method string, form url.Values, timeout time.Duration) error {
	apiURL, err := telegramMethodURL(cfg, cfg["bot_token"], method)
	if err != nil {
		return err
	}
	return telegramDoWithFallback(ctx, cfg, http.MethodPost, apiURL, form.Encode(), "application/x-www-form-urlencoded", timeout)
}

func telegramPostJSON(ctx context.Context, cfg map[string]string, method string, payload any, timeout time.Duration) error {
	apiURL, err := telegramMethodURL(cfg, cfg["bot_token"], method)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return telegramDoWithFallback(ctx, cfg, http.MethodPost, apiURL, string(body), "application/json", timeout)
}

func telegramPostMultipart(ctx context.Context, cfg map[string]string, method string, fields map[string]string, fileField, fileName string, file []byte, timeout time.Duration) error {
	apiURL, err := telegramMethodURL(cfg, cfg["bot_token"], method)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			_ = writer.Close()
			return err
		}
	}
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		_ = writer.Close()
		return err
	}
	if _, err := part.Write(file); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return telegramDoWithFallback(ctx, cfg, http.MethodPost, apiURL, body.String(), writer.FormDataContentType(), timeout)
}

func telegramFetchRemotePhoto(ctx context.Context, cfg map[string]string, rawURL string, timeout time.Duration) ([]byte, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, "", errors.New("telegram photo url required")
	}
	var lastErr error
	for _, client := range telegramHTTPClients(timeout, cfg) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("User-Agent", "MediaStationGo/1.0")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = sanitizeTelegramError(err)
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024+1))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("photo fetch error %d", resp.StatusCode)
			continue
		}
		if len(body) == 0 {
			lastErr = errors.New("photo fetch returned empty body")
			continue
		}
		if len(body) > 10*1024*1024 {
			lastErr = errors.New("photo too large")
			continue
		}
		contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
		return body, contentType, nil
	}
	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", errors.New("photo fetch failed")
}

func deleteTelegramWebhook(ctx context.Context, cfg map[string]string) error {
	payload := map[string]any{
		"drop_pending_updates": false,
	}
	return telegramPostJSON(ctx, cfg, "deleteWebhook", payload, 15*time.Second)
}

func telegramDo(client *http.Client, req *http.Request) error {
	resp, err := client.Do(req)
	if err != nil {
		return sanitizeTelegramError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("telegram api error %d: %s", resp.StatusCode, sanitizeTelegramText(string(body)))
	}
	return nil
}

func telegramDoWithFallback(ctx context.Context, cfg map[string]string, method, apiURL, body, contentType string, timeout time.Duration) error {
	var lastErr error
	for _, client := range telegramHTTPClients(timeout, cfg) {
		req, err := http.NewRequestWithContext(ctx, method, apiURL, strings.NewReader(body))
		if err != nil {
			return err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if err := telegramDo(client, req); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("telegram request failed")
}

func telegramPostJSONDecode(ctx context.Context, cfg map[string]string, method string, payload any, timeout time.Duration, out any) error {
	apiURL, err := telegramMethodURL(cfg, cfg["bot_token"], method)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var lastErr error
	for _, client := range telegramHTTPClients(timeout, cfg) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(body)))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = sanitizeTelegramError(err)
			continue
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("telegram api error %d: %s", resp.StatusCode, sanitizeTelegramText(string(respBody)))
			continue
		}
		if out != nil {
			return json.Unmarshal(respBody, out)
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("telegram request failed")
}

func telegramGetJSONDecode(ctx context.Context, cfg map[string]string, method string, timeout time.Duration, out any) error {
	apiURL, err := telegramMethodURL(cfg, cfg["bot_token"], method)
	if err != nil {
		return err
	}
	var lastErr error
	for _, client := range telegramHTTPClients(timeout, cfg) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = sanitizeTelegramError(err)
			continue
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("telegram api error %d: %s", resp.StatusCode, sanitizeTelegramText(string(respBody)))
			continue
		}
		if out != nil {
			return json.Unmarshal(respBody, out)
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("telegram request failed")
}

func telegramStringConfigFromAny(cfg map[string]any) map[string]string {
	out := make(map[string]string, len(cfg))
	for key, value := range cfg {
		out[key] = str(value)
	}
	normalizeTelegramConfig(out)
	return out
}
