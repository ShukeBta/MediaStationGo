package cloud

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQuarkListAndResolve(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		switch {
		case r.URL.Path == "/file/sort":
			if r.URL.Query().Get("pdir_fid") != "0" {
				t.Errorf("unexpected pdir_fid %q", r.URL.Query().Get("pdir_fid"))
			}
			w.Write([]byte(`{"status":200,"code":0,"data":{"list":[
				{"fid":"d1","file_name":"Movies","dir":true,"size":0},
				{"fid":"f1","file_name":"Inception.mkv","dir":false,"size":123}]}}`))
		case r.URL.Path == "/file/download":
			if r.Method != http.MethodPost {
				t.Errorf("download must be POST, got %s", r.Method)
			}
			w.Write([]byte(`{"status":200,"code":0,"data":[{"fid":"f1","download_url":"https://cdn.quark/x.mkv?sign=1"}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeQuark, map[string]any{"cookie": "kps=abc", "base": srv.URL}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := p.List(context.Background(), "0")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 || !entries[0].IsDir || entries[1].Name != "Inception.mkv" || entries[1].Size != 123 {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	if gotCookie != "kps=abc" {
		t.Fatalf("cookie not forwarded: %q", gotCookie)
	}
	link, err := p.Resolve(context.Background(), "f1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if link.URL != "https://cdn.quark/x.mkv?sign=1" {
		t.Fatalf("bad url: %s", link.URL)
	}
	if !link.Proxy {
		t.Fatalf("quark should default to proxy mode")
	}
	if link.Headers["Cookie"] != "kps=abc" {
		t.Fatalf("resolve must carry cookie header: %#v", link.Headers)
	}
}

func TestQuarkForce302(t *testing.T) {
	p := newQuark(map[string]any{"cookie": "c", "force_302": "true"}, http.DefaultClient)
	if p.proxy {
		t.Fatalf("force_302 should disable proxy mode")
	}
}

func Test115ListAndResolve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files":
			if r.URL.Query().Get("cid") != "0" {
				t.Errorf("bad cid %q", r.URL.Query().Get("cid"))
			}
			w.Write([]byte(`{"state":true,"data":[
				{"cid":"100","n":"Movies","s":0},
				{"fid":"200","n":"Inception.mkv","s":456,"pc":"pick200"}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(Type115, map[string]any{"cookie": "UID=1; CID=2", "base": srv.URL}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	// The downurl endpoint is m115-encrypted end-to-end (the server side
	// requires 115's private key), so stub the decrypted payload via the seam
	// and assert the pickcode→URL extraction. The live crypto/transport path is
	// exercised by integration testing against the real 115 API.
	p115, ok := p.(*pan115Provider)
	if !ok {
		t.Fatalf("expected *pan115Provider, got %T", p)
	}
	p115.downURLPayload = func(ctx context.Context, pickcode string) ([]byte, error) {
		if pickcode != "pick200" {
			t.Errorf("bad pickcode %q", pickcode)
		}
		return []byte(`{"200":{"file_name":"Inception.mkv","file_size":"456","url":{"url":"https://cdn.115/x.mkv?t=1"}}}`), nil
	}
	entries, err := p.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries: %#v", entries)
	}
	if !entries[0].IsDir || entries[0].ID != "100" {
		t.Fatalf("dir entry wrong: %#v", entries[0])
	}
	if entries[1].IsDir || entries[1].PickCode != "pick200" || entries[1].Size != 456 {
		t.Fatalf("file entry wrong: %#v", entries[1])
	}
	link, err := p.Resolve(context.Background(), "pick200")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if link.URL != "https://cdn.115/x.mkv?t=1" {
		t.Fatalf("bad url: %s", link.URL)
	}
	if link.Proxy {
		t.Fatalf("115 should default to 302 (no proxy)")
	}
}

// Test115DownURLEndpointAndError exercises the live fetchDownURLPayload path:
// it must POST an m115-encrypted `data` body to /app/chrome/downurl?t=... and
// surface 115's error when state=false (no decryption needed for that branch).
func Test115DownURLEndpointAndError(t *testing.T) {
	var gotData, gotT string
	pro := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/chrome/downurl" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotT = r.URL.Query().Get("t")
		_ = r.ParseForm()
		gotData = r.PostFormValue("data")
		w.Write([]byte(`{"state":false,"error":"not exist"}`))
	}))
	defer pro.Close()

	p, err := New(Type115, map[string]any{"cookie": "UID=1", "pro_base": pro.URL}, pro.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Resolve(context.Background(), "pickX")
	if err == nil || !strings.Contains(err.Error(), "not exist") {
		t.Fatalf("want upstream error surfaced, got %v", err)
	}
	if gotT == "" {
		t.Errorf("missing t query param")
	}
	if gotData == "" {
		t.Errorf("missing encrypted data body")
	}
	if _, derr := base64.StdEncoding.DecodeString(gotData); derr != nil {
		t.Errorf("data body is not base64: %v", derr)
	}
}

func Test115QRFlow(t *testing.T) {
	// status sequence: waiting → scanned → confirmed
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/1.0/web/1.0/token/":
			w.Write([]byte(`{"state":1,"data":{"uid":"U1","time":1700,"sign":"S1"}}`))
		case "/get/status/":
			if r.URL.Query().Get("uid") != "U1" {
				t.Errorf("bad uid %q", r.URL.Query().Get("uid"))
			}
			calls++
			switch calls {
			case 1:
				w.Write([]byte(`{"state":1,"data":{"status":0}}`))
			case 2:
				w.Write([]byte(`{"state":1,"data":{"status":1}}`))
			default:
				w.Write([]byte(`{"state":1,"data":{"status":2}}`))
			}
		default:
			t.Errorf("unexpected api path %s", r.URL.Path)
		}
	}))
	defer api.Close()
	passport := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/1.0/web/1.0/login/qrcode/" {
			t.Errorf("unexpected passport path %s", r.URL.Path)
		}
		w.Write([]byte(`{"state":1,"data":{"cookie":{"UID":"u","CID":"c","SEID":"s"}}}`))
	}))
	defer passport.Close()

	oldA, oldP := qr115APIBase, qr115PassportBase
	qr115APIBase, qr115PassportBase = api.URL, passport.URL
	defer func() { qr115APIBase, qr115PassportBase = oldA, oldP }()

	ctx := context.Background()
	sess, err := QRStart(ctx, api.Client())
	if err != nil {
		t.Fatalf("qr start: %v", err)
	}
	if sess.UID != "U1" || sess.QRImageURL == "" {
		t.Fatalf("bad session: %#v", sess)
	}
	want := []string{"waiting", "scanned", "confirmed"}
	for i, exp := range want {
		st, err := QRPoll(ctx, api.Client(), sess)
		if err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
		if st.State != exp {
			t.Fatalf("poll %d: want %s got %s", i, exp, st.State)
		}
		if exp == "confirmed" {
			if st.Cookie == "" || !containsAll(st.Cookie, "UID=u", "SEID=s") {
				t.Fatalf("confirmed must yield cookie: %q", st.Cookie)
			}
		}
	}
}

func TestUnsupportedProvider(t *testing.T) {
	if _, err := New("dropbox", nil, nil); err != ErrUnsupported {
		t.Fatalf("want ErrUnsupported, got %v", err)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

var _ = time.Second
