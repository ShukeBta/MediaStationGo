package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	_, err = p.Resolve(context.Background(), entries[0].ID)
	if err == nil || !strings.Contains(err.Error(), "pure 302 playback requires OpenList raw_url") {
		t.Fatalf("openlist video resolve should require raw_url instead of WebDAV proxy fallback, err=%v", err)
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

func TestOpenListMutableProviderUsesAPI(t *testing.T) {
	var mkdirPath, renamePath, renameName, moveSrcDir, moveDstDir string
	var moveNames []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/mkdir":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode mkdir body: %v", err)
			}
			mkdirPath = body["path"]
			if r.Header.Get("Authorization") != "alist-token" {
				t.Fatalf("mkdir Authorization = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		case "/api/fs/rename":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode rename body: %v", err)
			}
			renamePath = body["path"]
			renameName = body["name"]
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		case "/api/fs/move":
			var body struct {
				SrcDir string   `json:"src_dir"`
				DstDir string   `json:"dst_dir"`
				Names  []string `json:"names"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode move body: %v", err)
			}
			moveSrcDir = body.SrcDir
			moveDstDir = body.DstDir
			moveNames = body.Names
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	mutable, ok := p.(MutableProvider)
	if !ok {
		t.Fatal("openlist should support mutable provider")
	}
	created, err := mutable.Mkdir(context.Background(), "/电视剧", "欧美剧")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if mkdirPath != "/电视剧/欧美剧" || created.ID != "/电视剧/欧美剧" || !created.IsDir {
		t.Fatalf("mkdir path=%q entry=%#v", mkdirPath, created)
	}
	renamed, err := mutable.Rename(context.Background(), "/电视剧/欧美剧", "美剧")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamePath != "/电视剧/欧美剧" || renameName != "美剧" || renamed.ID != "/电视剧/美剧" {
		t.Fatalf("rename path=%q name=%q entry=%#v", renamePath, renameName, renamed)
	}
	moved, err := mutable.(MovableProvider).Move(context.Background(), "/待整理/Show.S01E01.mkv", "/动漫/国漫/Show/Season 01", "Show - S01E01.mkv")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moveSrcDir != "/待整理" || moveDstDir != "/动漫/国漫/Show/Season 01" || len(moveNames) != 1 || moveNames[0] != "Show.S01E01.mkv" {
		t.Fatalf("move src=%q dst=%q names=%#v", moveSrcDir, moveDstDir, moveNames)
	}
	if renamePath != "/动漫/国漫/Show/Season 01/Show.S01E01.mkv" || renameName != "Show - S01E01.mkv" {
		t.Fatalf("post-move rename path=%q name=%q", renamePath, renameName)
	}
	if moved.ID != "/动漫/国漫/Show/Season 01/Show - S01E01.mkv" {
		t.Fatalf("moved entry = %#v", moved)
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
