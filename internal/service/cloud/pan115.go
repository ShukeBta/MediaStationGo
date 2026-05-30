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

// pan115Provider implements 115 网盘 via cookie auth.
//
// 115 has removed its desktop clients, so cookies must come from the mobile
// app / web (115.com) or a QR-code login (see QR* helpers below). Directory
// listing uses the public web API; download resolves a file's pickcode to a
// CDN URL that, like Alist's default 115 behaviour, is served by 302 redirect.
type pan115Provider struct {
	cookie  string
	ua      string
	webBase string // https://webapi.115.com (override in tests)
	client  *http.Client
	proxy   bool
}

const pan115WebBase = "https://webapi.115.com"

func new115(cfg map[string]any, client *http.Client) *pan115Provider {
	web := str(cfg["base"])
	if web == "" {
		web = pan115WebBase
	}
	ua := str(cfg["ua"])
	if ua == "" {
		ua = defaultUA
	}
	// 115 CDN download URLs work with a plain 302 (Alist's recommended mode),
	// so offload by default; admin can force proxy mode if their network needs it.
	proxy := false
	if _, ok := cfg["force_proxy"]; ok && boolish(cfg["force_proxy"]) {
		proxy = true
	}
	return &pan115Provider{
		cookie:  str(cfg["cookie"]),
		ua:      ua,
		webBase: strings.TrimRight(web, "/"),
		client:  client,
		proxy:   proxy,
	}
}

func (p *pan115Provider) Type() string { return Type115 }

func (p *pan115Provider) get(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", p.cookie)
	req.Header.Set("User-Agent", p.ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	return p.client.Do(req)
}

func (p *pan115Provider) Ping(ctx context.Context) error {
	if p.cookie == "" {
		return fmt.Errorf("115: missing cookie")
	}
	_, err := p.List(ctx, "0")
	return err
}

func (p *pan115Provider) List(ctx context.Context, dirID string) ([]FileEntry, error) {
	if dirID == "" {
		dirID = "0"
	}
	q := url.Values{}
	q.Set("aid", "1")
	q.Set("cid", dirID)
	q.Set("o", "user_ptime")
	q.Set("asc", "0")
	q.Set("offset", "0")
	q.Set("show_dir", "1")
	q.Set("limit", "100")
	q.Set("format", "json")
	resp, err := p.get(ctx, p.webBase+"/files?"+q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		State bool   `json:"state"`
		Error string `json:"error"`
		Data  []struct {
			Fid string      `json:"fid"` // file id (files only)
			Cid string      `json:"cid"` // category id (dirs use this)
			N   string      `json:"n"`   // name
			S   json.Number `json:"s"`   // size
			Pc  string      `json:"pc"`  // pickcode
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("115: decode list: %w", err)
	}
	if !r.State {
		return nil, fmt.Errorf("115: list failed: %s", r.Error)
	}
	out := make([]FileEntry, 0, len(r.Data))
	for _, it := range r.Data {
		isDir := it.Fid == ""
		id := it.Fid
		if isDir {
			id = it.Cid
		}
		size, _ := it.S.Int64()
		out = append(out, FileEntry{
			ID:       id,
			Name:     it.N,
			IsDir:    isDir,
			Size:     size,
			PickCode: it.Pc,
		})
	}
	return out, nil
}

// Resolve accepts a pickcode (preferred) and returns the CDN download URL.
func (p *pan115Provider) Resolve(ctx context.Context, pickcode string) (*DirectLink, error) {
	if pickcode == "" {
		return nil, fmt.Errorf("115: empty pickcode")
	}
	u := fmt.Sprintf("%s/files/download?pickcode=%s&_=%d", p.webBase, url.QueryEscape(pickcode), nowUnix())
	resp, err := p.get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		State   bool   `json:"state"`
		Error   string `json:"error"`
		FileURL string `json:"file_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("115: decode download: %w", err)
	}
	if !r.State || r.FileURL == "" {
		msg := r.Error
		if msg == "" {
			msg = "no file_url"
		}
		return nil, fmt.Errorf("115: download failed: %s", msg)
	}
	return &DirectLink{
		URL: r.FileURL,
		Headers: map[string]string{
			"User-Agent": p.ua,
			"Cookie":     p.cookie,
		},
		Proxy: p.proxy,
	}, nil
}

// nowUnix is a seam for deterministic tests.
var nowUnix = func() int64 { return timeNow().Unix() }

// ─── QR-code login ───────────────────────────────────────────────────────────

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
