package cloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestQuarkListPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/file/sort" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("_page"))
		w.Write([]byte(`{"status":200,"code":0,"data":{"list":[` + quarkPagePayload(page) + `]}}`))
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
	if len(entries) != 101 {
		t.Fatalf("entries = %d, want 101", len(entries))
	}
	if entries[100].ID != "f100" || entries[100].Name != "Movie.100.mkv" {
		t.Fatalf("last entry wrong: %#v", entries[100])
	}
}

func quarkPagePayload(page int) string {
	count := 100
	offset := 0
	if page > 1 {
		count = 1
		offset = 100
	}
	items := make([]string, 0, count)
	for i := 0; i < count; i++ {
		n := offset + i
		items = append(items, fmt.Sprintf(`{"fid":"f%d","file_name":"Movie.%03d.mkv","dir":false,"size":%d}`, n, n, n))
	}
	return strings.Join(items, ",")
}

func TestDeprecatedProviderPlaybackOverrideKeysAreIgnored(t *testing.T) {
	quark := newQuark(map[string]any{"cookie": "c", "force_302": "true"}, http.DefaultClient)
	if !quark.proxy {
		t.Fatalf("quark should keep safe proxy mode; force_302 is deprecated")
	}
	pan115 := new115(map[string]any{"cookie": "UID=1; CID=2", "force_proxy": "true"}, http.DefaultClient)
	if pan115.proxy {
		t.Fatalf("115 should keep safe direct mode; force_proxy is deprecated")
	}
	cd2 := newCloudDrive2(map[string]any{"url": "http://example.test/dav", "force_302": "true"}, http.DefaultClient)
	if !cd2.proxy {
		t.Fatalf("clouddrive2 should keep safe proxy mode; force_302 is deprecated")
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

func Test115ListPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		count := 100
		if offset > 0 {
			count = 1
		}
		items := make([]string, 0, count)
		for i := 0; i < count; i++ {
			n := offset + i
			items = append(items, fmt.Sprintf(`{"fid":"%d","n":"Movie.%03d.mkv","s":%d,"pc":"pick%d"}`, n, n, n, n))
		}
		w.Write([]byte(`{"state":true,"data":[` + strings.Join(items, ",") + `]}`))
	}))
	defer srv.Close()

	p, err := New(Type115, map[string]any{"cookie": "UID=1; CID=2", "base": srv.URL}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := p.List(context.Background(), "0")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 101 {
		t.Fatalf("entries = %d, want 101", len(entries))
	}
	if entries[100].ID != "100" || entries[100].PickCode != "pick100" {
		t.Fatalf("last entry wrong: %#v", entries[100])
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

func TestCloudDrive2WebDAVListAndResolve(t *testing.T) {
	var gotAuth, gotDepth, gotRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PROPFIND" && r.URL.Path == "/dav":
			gotAuth = r.Header.Get("Authorization")
			gotDepth = r.Header.Get("Depth")
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/115/</d:href>
    <d:propstat><d:prop><d:displayname>115</d:displayname><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/123/Movie.mkv</d:href>
    <d:propstat><d:prop><d:displayname>Movie.mkv</d:displayname><d:getcontentlength>789</d:getcontentlength><d:resourcetype/></d:prop></d:propstat>
  </d:response>
</d:multistatus>`))
		case r.Method == http.MethodGet && r.URL.Path == "/dav/123/Movie.mkv":
			gotAuth = r.Header.Get("Authorization")
			gotRange = r.Header.Get("Range")
			http.Redirect(w, r, "https://cdn.example.test/123/Movie.mkv?sign=1", http.StatusFound)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeCloudDrive2, map[string]any{"url": srv.URL + "/dav", "username": "u", "password": "p"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := p.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if gotDepth != "1" {
		t.Fatalf("Depth = %q, want 1", gotDepth)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("missing basic auth: %q", gotAuth)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	if !entries[0].IsDir || entries[0].ID != "/115" {
		t.Fatalf("dir entry wrong: %#v", entries[0])
	}
	if entries[1].IsDir || entries[1].ID != "/123/Movie.mkv" || entries[1].Size != 789 {
		t.Fatalf("file entry wrong: %#v", entries[1])
	}
	link, err := p.Resolve(context.Background(), entries[1].ID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if link.URL != "https://cdn.example.test/123/Movie.mkv?sign=1" {
		t.Fatalf("bad url: %s", link.URL)
	}
	if link.Proxy || len(link.Headers) != 0 {
		t.Fatalf("clouddrive2 video should resolve to pure 302 link: %#v", link)
	}
	if gotRange != "bytes=0-0" {
		t.Fatalf("resolve should probe with a tiny range, got %q", gotRange)
	}
}

func TestCloudDrive2ResolveFallsBackToWebDAVProxyWithoutRedirect(t *testing.T) {
	var getSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PROPFIND" && r.URL.Path == "/dav":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/dav/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response></d:multistatus>`))
		case r.Method == http.MethodGet && r.URL.Path == "/dav/123/Movie.mkv":
			getSeen = true
			w.Header().Set("Content-Range", "bytes 0-0/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("x"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeCloudDrive2, map[string]any{"url": srv.URL + "/dav", "username": "u", "password": "p"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/123/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if link.URL != srv.URL+"/dav/123/Movie.mkv" || !link.Proxy {
		t.Fatalf("link = %#v, want WebDAV proxy fallback", link)
	}
	if !strings.HasPrefix(link.Headers["Authorization"], "Basic ") {
		t.Fatalf("missing basic auth header: %#v", link.Headers)
	}
	if !getSeen {
		t.Fatal("expected CloudDrive2 WebDAV direct-link probe")
	}
}

func TestOpenListWebDAVListAndResolve(t *testing.T) {
	var gotPath, gotDepth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/api/fs/get" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "PROPFIND" || r.URL.Path != "/dav" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotPath = r.URL.Path
		gotDepth = r.Header.Get("Depth")
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/Cloud/Movie.mkv</d:href>
    <d:propstat><d:prop><d:displayname>Movie.mkv</d:displayname><d:getcontentlength>1024</d:getcontentlength><d:resourcetype/></d:prop></d:propstat>
  </d:response>
</d:multistatus>`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"url": srv.URL + "/dav"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if p.Type() != TypeOpenList {
		t.Fatalf("type = %q, want %q", p.Type(), TypeOpenList)
	}
	entries, err := p.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if gotPath != "/dav" {
		t.Fatalf("path = %q, want /dav", gotPath)
	}
	if gotDepth != "1" {
		t.Fatalf("Depth = %q, want 1", gotDepth)
	}
	if len(entries) != 1 || entries[0].ID != "/Cloud/Movie.mkv" || entries[0].Size != 1024 {
		t.Fatalf("entries = %#v", entries)
	}
	link, err := p.Resolve(context.Background(), entries[0].ID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if link.URL != srv.URL+"/dav/Cloud/Movie.mkv" || !link.Proxy {
		t.Fatalf("link = %#v, want WebDAV proxy fallback", link)
	}
}

func TestOpenListListUsesAPIUsernamePasswordWithoutWebDAVFallback(t *testing.T) {
	var loginSeen, listSeen, davSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/auth/login":
			loginSeen = true
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode login body: %v", err)
			}
			if body["username"] != "alice" || body["password"] != "secret" {
				t.Fatalf("login body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"token":"api-token"}}`))
		case "/api/fs/list":
			listSeen = true
			if r.Header.Get("Authorization") != "api-token" {
				t.Fatalf("Authorization = %q, want api-token", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[{"name":"Movies","is_dir":true,"size":0},{"name":"Movie.mkv","is_dir":false,"size":1024}],"total":2}}`))
		case "/dav":
			davSeen = true
			w.WriteHeader(http.StatusMultiStatus)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "username": "alice", "password": "secret"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := p.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !loginSeen || !listSeen {
		t.Fatalf("expected api login/list, login=%v list=%v", loginSeen, listSeen)
	}
	if davSeen {
		t.Fatal("openlist API credentials should not fall back to WebDAV")
	}
	if len(entries) != 2 || entries[0].ID != "/Movies" || !entries[0].IsDir || entries[1].ID != "/Movie.mkv" || entries[1].Size != 1024 {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestOpenListListAPIFailureDoesNotFallbackToWebDAV(t *testing.T) {
	var davSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":500,"message":"bad password"}`))
		case "/dav":
			davSeen = true
			w.WriteHeader(http.StatusMultiStatus)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "username": "alice", "password": "bad"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.List(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "api login failed") || !strings.Contains(err.Error(), "bad password") {
		t.Fatalf("list error = %v, want api login failure", err)
	}
	if davSeen {
		t.Fatal("openlist API failure fell back to WebDAV")
	}
}

func TestOpenListResolveUsesAPIRawURLFor302Playback(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv?sign=1"}}`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotPath != "/api/fs/get" {
		t.Fatalf("api path = %q, want /api/fs/get", gotPath)
	}
	if gotAuth != "alist-token" {
		t.Fatalf("Authorization = %q, want token", gotAuth)
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=1" {
		t.Fatalf("url = %q", link.URL)
	}
	if link.Proxy {
		t.Fatalf("openlist raw_url without required headers should be 302 playback")
	}
}

func TestOpenListResolveCollapsesHostedRawURLRedirectToCDN(t *testing.T) {
	var probeSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"/d/Cloud/Movie.mkv?sign=1"}}`))
		case "/d/Cloud/Movie.mkv":
			probeSeen = true
			if r.Header.Get("Range") != "bytes=0-0" {
				t.Fatalf("probe Range = %q", r.Header.Get("Range"))
			}
			http.Redirect(w, r, "https://cdn.example.test/movie.mkv?sign=cdn", http.StatusFound)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !probeSeen {
		t.Fatal("expected OpenList-hosted raw_url probe")
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=cdn" || link.Proxy || len(link.Headers) != 0 {
		t.Fatalf("link = %#v, want collapsed CDN 302 playback", link)
	}
}

func TestOpenListResolveLogsInWithUsernamePasswordForAPIRawURL(t *testing.T) {
	var loginSeen bool
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/auth/login":
			loginSeen = true
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode login body: %v", err)
			}
			if body["username"] != "alice" || body["password"] != "secret" {
				t.Fatalf("login body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"token":"api-token"}}`))
		case "/api/fs/get":
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv?sign=1"}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "username": "alice", "password": "secret"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !loginSeen {
		t.Fatalf("expected api login before fs/get")
	}
	if gotAuth != "api-token" {
		t.Fatalf("Authorization = %q, want api-token", gotAuth)
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=1" || link.Proxy {
		t.Fatalf("link = %#v, want raw_url 302 playback", link)
	}
}

func TestOpenListResolveProxiesAPIRawURLWhenHeadersRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"/dav/Cloud/Movie.mkv","header":{"Cookie":"sid=abc"}}}`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if link.URL != srv.URL+"/dav/Cloud/Movie.mkv" || !link.Proxy || link.Headers["Cookie"] != "sid=abc" {
		t.Fatalf("link = %#v, want raw_url proxy with headers", link)
	}
}

func TestOpenListResolveProxiesHostedRawURLWithoutCDNRedirect(t *testing.T) {
	var probeSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"/d/Cloud/Movie.mkv?sign=1"}}`))
		case "/d/Cloud/Movie.mkv":
			probeSeen = true
			w.Header().Set("Content-Range", "bytes 0-0/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("x"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !probeSeen {
		t.Fatal("expected OpenList-hosted raw_url probe")
	}
	if link.URL != srv.URL+"/d/Cloud/Movie.mkv?sign=1" || !link.Proxy || len(link.Headers) != 0 {
		t.Fatalf("link = %#v, want hosted raw_url proxy fallback", link)
	}
}

func TestOpenListResolveFallsBackToWebDAVWhenAPIRawURLFails(t *testing.T) {
	var davSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":500,"message":"driver cannot provide raw_url"}`))
		case "/dav/Cloud/Movie.mkv":
			davSeen = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if link.URL != srv.URL+"/dav/Cloud/Movie.mkv" || !link.Proxy {
		t.Fatalf("link = %#v, want WebDAV proxy fallback", link)
	}
	if davSeen {
		t.Fatal("resolve should return a WebDAV proxy link without probing full file")
	}
}

func TestOpenListRootURLDefaultsToDAV(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/dav/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response></d:multistatus>`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"url": srv.URL + "/"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.List(context.Background(), ""); err != nil {
		t.Fatalf("list: %v", err)
	}
	if gotPath != "/dav" {
		t.Fatalf("path = %q, want /dav", gotPath)
	}
}

func TestOpenListURLForKeepsNonASCIIPathSingleEncoded(t *testing.T) {
	p := newOpenList(map[string]any{"url": "http://example.test:5244/dav/"}, nil)
	got := p.urlFor("/动画电影/爱宠大机密2 (2019) {tmdb-412117}")
	if strings.Contains(got, "%25E") {
		t.Fatalf("url is double-escaped: %s", got)
	}
	want := "http://example.test:5244/dav/%E5%8A%A8%E7%94%BB%E7%94%B5%E5%BD%B1/%E7%88%B1%E5%AE%A0%E5%A4%A7%E6%9C%BA%E5%AF%862%20%282019%29%20%7Btmdb-412117%7D"
	if got != want {
		t.Fatalf("url = %s, want %s", got, want)
	}
}

func TestOpenListDAVStatusErrorIncludesBodyHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte("请先填写有效Cookie并保存"))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"url": srv.URL + "/dav"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.List(context.Background(), "")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "请先填写有效Cookie并保存") || !strings.Contains(err.Error(), "WebDAV 地址") {
		t.Fatalf("unexpected error: %v", err)
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
