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
	proBase string // https://proapi.115.com (override in tests)
	client  *http.Client
	proxy   bool

	// downURLPayload fetches and decrypts the app/chrome/downurl response for a
	// pickcode, returning the raw JSON payload (map of file id -> info). It is a
	// seam so tests can bypass the live 115 crypto/transport.
	downURLPayload func(ctx context.Context, pickcode string) ([]byte, error)
}

const (
	pan115WebBase = "https://webapi.115.com"
	pan115ProBase = "https://proapi.115.com"
)

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
	// so offload by default. The global cloud playback setting decides whether
	// clients receive a STRMURL entry or a /Videos stream entry.
	proxy := false
	pro := str(cfg["pro_base"])
	if pro == "" {
		pro = pan115ProBase
	}
	p := &pan115Provider{
		cookie:  str(cfg["cookie"]),
		ua:      ua,
		webBase: strings.TrimRight(web, "/"),
		proBase: strings.TrimRight(pro, "/"),
		client:  client,
		proxy:   proxy,
	}
	p.downURLPayload = p.fetchDownURLPayload
	return p
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
	const pageSize = 100
	out := make([]FileEntry, 0, pageSize)
	for offset := 0; ; offset += pageSize {
		q := url.Values{}
		q.Set("aid", "1")
		q.Set("cid", dirID)
		q.Set("o", "user_ptime")
		q.Set("asc", "0")
		q.Set("offset", strconv.Itoa(offset))
		q.Set("show_dir", "1")
		q.Set("limit", strconv.Itoa(pageSize))
		q.Set("format", "json")
		resp, err := p.get(ctx, p.webBase+"/files?"+q.Encode())
		if err != nil {
			return nil, err
		}
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
		err = json.NewDecoder(resp.Body).Decode(&r)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("115: decode list: %w", err)
		}
		if !r.State {
			return nil, fmt.Errorf("115: list failed: %s", r.Error)
		}
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
		if len(r.Data) < pageSize {
			break
		}
	}
	return out, nil
}

// Resolve accepts a pickcode (preferred) and returns the CDN download URL.
//
// 115 deprecated the plain web /files/download endpoint (it no longer returns
// file_url for ordinary cookies). We use the current app/chrome/downurl
// endpoint, which takes an m115-encrypted body and returns an m115-encrypted
// payload mapping the file id to a short-lived, OSS-signed CDN URL suitable for
// a 302 redirect (the same approach Alist's 115 driver uses).
func (p *pan115Provider) Resolve(ctx context.Context, pickcode string) (*DirectLink, error) {
	if pickcode == "" {
		return nil, fmt.Errorf("115: empty pickcode")
	}
	raw, err := p.downURLPayload(ctx, pickcode)
	if err != nil {
		return nil, err
	}
	var payload map[string]struct {
		FileName string      `json:"file_name"`
		FileSize json.Number `json:"file_size"`
		URL      struct {
			URL string `json:"url"`
		} `json:"url"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("115: decode downurl: %w", err)
	}
	for _, info := range payload {
		if info.URL.URL == "" {
			continue
		}
		return &DirectLink{
			URL: info.URL.URL,
			Headers: map[string]string{
				"User-Agent": p.ua,
				"Cookie":     p.cookie,
			},
			Proxy: p.proxy,
		}, nil
	}
	return nil, fmt.Errorf("115: download failed: no url")
}

// fetchDownURLPayload performs the live encrypted app/chrome/downurl request and
// returns the decrypted JSON payload.
func (p *pan115Provider) fetchDownURLPayload(ctx context.Context, pickcode string) ([]byte, error) {
	key := m115GenerateKey()
	params, err := json.Marshal(map[string]string{"pickcode": pickcode})
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("data", m115Encode(params, key))
	u := fmt.Sprintf("%s/app/chrome/downurl?t=%d", p.proBase, nowUnix())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", p.cookie)
	req.Header.Set("User-Agent", p.ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		State bool   `json:"state"`
		Error string `json:"error"`
		Data  string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("115: decode downurl: %w", err)
	}
	if !r.State || r.Data == "" {
		msg := r.Error
		if msg == "" {
			msg = "no data"
		}
		return nil, fmt.Errorf("115: download failed: %s", msg)
	}
	out, err := m115Decode(r.Data, key)
	if err != nil {
		return nil, fmt.Errorf("115: decrypt downurl: %w", err)
	}
	return out, nil
}

// nowUnix is a seam for deterministic tests.
var nowUnix = func() int64 { return timeNow().Unix() }
