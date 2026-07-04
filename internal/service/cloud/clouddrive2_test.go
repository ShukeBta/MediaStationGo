package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestCloudDrive2ResolveRejectsWebDAVProxyFallbackWithoutRedirect(t *testing.T) {
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
	_, err = p.Resolve(context.Background(), "/123/Movie.mkv")
	if err == nil || !strings.Contains(err.Error(), "without CDN Location") || !strings.Contains(err.Error(), "refusing WebDAV/proxy fallback") {
		t.Fatalf("resolve error = %v, want pure 302 refusal", err)
	}
	if !getSeen {
		t.Fatal("expected CloudDrive2 WebDAV direct-link probe")
	}
}

func TestCloudDrive2MutableProviderUsesWebDAV(t *testing.T) {
	var mkcolSeen bool
	var destinations []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "MKCOL" && r.URL.Path == "/dav/TV":
			mkcolSeen = true
			w.WriteHeader(http.StatusCreated)
		case r.Method == "MOVE" && r.URL.Path == "/dav/TV":
			destinations = append(destinations, r.Header.Get("Destination"))
			if r.Header.Get("Overwrite") != "F" {
				t.Fatalf("Overwrite = %q, want F", r.Header.Get("Overwrite"))
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == "MOVE" && r.URL.Path == "/dav/Inbox/Movie.mkv":
			destinations = append(destinations, r.Header.Get("Destination"))
			if r.Header.Get("Overwrite") != "F" {
				t.Fatalf("Overwrite = %q, want F", r.Header.Get("Overwrite"))
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeCloudDrive2, map[string]any{"url": srv.URL + "/dav", "username": "u", "password": "p"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	mutable, ok := p.(MutableProvider)
	if !ok {
		t.Fatal("clouddrive2 should support mutable provider")
	}
	if _, err := mutable.Mkdir(context.Background(), "", "TV"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := mutable.Rename(context.Background(), "/TV", "电视剧"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	moved, err := mutable.(MovableProvider).Move(context.Background(), "/Inbox/Movie.mkv", "/电影/欧美电影/Movie (2026)", "Movie (2026).mkv")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !mkcolSeen || len(destinations) != 2 {
		t.Fatalf("mkcol=%v destinations=%#v, want mkdir and two MOVE calls", mkcolSeen, destinations)
	}
	if destinations[0] != srv.URL+"/dav/%E7%94%B5%E8%A7%86%E5%89%A7" {
		t.Fatalf("rename Destination = %q", destinations[0])
	}
	if destinations[1] != srv.URL+"/dav/%E7%94%B5%E5%BD%B1/%E6%AC%A7%E7%BE%8E%E7%94%B5%E5%BD%B1/Movie%20%282026%29/Movie%20%282026%29.mkv" {
		t.Fatalf("move Destination = %q", destinations[1])
	}
	if moved.ID != "/电影/欧美电影/Movie (2026)/Movie (2026).mkv" {
		t.Fatalf("moved entry = %#v", moved)
	}
}
