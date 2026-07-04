package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// QRSession is the handle returned by QRStart; the client renders QRImageURL
// and polls QRPoll until it returns a cookie.
type QRSession struct {
	UID        string `json:"uid"`
	Time       int64  `json:"time"`
	Sign       string `json:"sign"`
	QRImageURL string `json:"qr_image_url"`
}

// QR login hosts (overridable for tests).
var (
	qr115APIBase      = "https://qrcodeapi.115.com"
	qr115PassportBase = "https://passportapi.115.com"
)

// QRStart obtains a 115 QR-code login token + image URL.
func QRStart(ctx context.Context, client *http.Client) (*QRSession, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, qr115APIBase+"/api/1.0/web/1.0/token/", nil)
	req.Header.Set("User-Agent", defaultUA)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		State int `json:"state"`
		Data  struct {
			UID  string `json:"uid"`
			Time int64  `json:"time"`
			Sign string `json:"sign"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("115 qr: decode token: %w", err)
	}
	if r.State != 1 || r.Data.UID == "" {
		return nil, fmt.Errorf("115 qr: token request failed")
	}
	return &QRSession{
		UID:        r.Data.UID,
		Time:       r.Data.Time,
		Sign:       r.Data.Sign,
		QRImageURL: qr115APIBase + "/api/1.0/web/1.0/qrcode?uid=" + url.QueryEscape(r.Data.UID),
	}, nil
}

// QRStatus is the poll result.
type QRStatus struct {
	// State is one of: "waiting" (not scanned), "scanned" (scanned, awaiting
	// confirmation), "confirmed" (login approved; Cookie populated),
	// "expired" (token expired/cancelled).
	State  string `json:"state"`
	Cookie string `json:"cookie,omitempty"`
}

// QRPoll checks the QR session status; on confirmation it exchanges the token
// for a session cookie via the passport API.
func QRPoll(ctx context.Context, client *http.Client, sess *QRSession) (*QRStatus, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if sess == nil || sess.UID == "" {
		return nil, fmt.Errorf("115 qr: nil session")
	}
	q := url.Values{}
	q.Set("uid", sess.UID)
	q.Set("time", strconv.FormatInt(sess.Time, 10))
	q.Set("sign", sess.Sign)
	q.Set("_", strconv.FormatInt(timeNow().UnixMilli(), 10))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, qr115APIBase+"/get/status/?"+q.Encode(), nil)
	req.Header.Set("User-Agent", defaultUA)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		State int `json:"state"`
		Data  struct {
			Status int `json:"status"` // 0 waiting, 1 scanned, 2 confirmed, -1/-2 expired
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("115 qr: decode status: %w", err)
	}
	switch r.Data.Status {
	case 1:
		return &QRStatus{State: "scanned"}, nil
	case 2:
		cookie, err := qr115Exchange(ctx, client, sess.UID)
		if err != nil {
			return nil, err
		}
		return &QRStatus{State: "confirmed", Cookie: cookie}, nil
	case 0:
		return &QRStatus{State: "waiting"}, nil
	default:
		return &QRStatus{State: "expired"}, nil
	}
}

// qr115Exchange swaps an approved uid for a session cookie.
func qr115Exchange(ctx context.Context, client *http.Client, uid string) (string, error) {
	form := url.Values{}
	form.Set("account", uid)
	form.Set("app", "web")
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, qr115PassportBase+"/app/1.0/web/1.0/login/qrcode/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", defaultUA)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r struct {
		State int `json:"state"`
		Data  struct {
			Cookie map[string]string `json:"cookie"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("115 qr: decode login: %w", err)
	}
	if r.State != 1 || len(r.Data.Cookie) == 0 {
		return "", fmt.Errorf("115 qr: login exchange failed")
	}
	parts := make([]string, 0, len(r.Data.Cookie))
	for k, v := range r.Data.Cookie {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; "), nil
}
