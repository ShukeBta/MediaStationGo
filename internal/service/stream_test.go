package service

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestWithAuthTokenPropagatesToInternalRedirect(t *testing.T) {
	// <video src=/api/stream/{id}?token=JWT> follows the 302 to the cloud
	// play endpoint, which must stay authenticated.
	r := &http.Request{Header: http.Header{}, URL: &url.URL{RawQuery: "token=jwt123&profile=p"}}
	got := withAuthToken("/api/cloud/play/cloud115?ref=abc", r)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Query().Get("token") != "jwt123" {
		t.Fatalf("token not propagated: %q", got)
	}
	if u.Query().Get("ref") != "abc" {
		t.Fatalf("existing query lost: %q", got)
	}
}

func TestWithAuthTokenNeverLeaksToAbsoluteURL(t *testing.T) {
	// An absolute external direct link (e.g. cloud CDN) must NOT receive the JWT.
	r := &http.Request{Header: http.Header{}, URL: &url.URL{RawQuery: "token=jwt123"}}
	got := withAuthToken("https://cdn.115.example/x.mp4?sig=1", r)
	if strings.Contains(got, "jwt123") {
		t.Fatalf("JWT leaked to external URL: %q", got)
	}
	if got != "https://cdn.115.example/x.mp4?sig=1" {
		t.Fatalf("external URL mutated: %q", got)
	}
}

func TestRequestTokenFromBearerHeader(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer hdrtok")
	r := &http.Request{Header: h, URL: &url.URL{}}
	if got := requestToken(r); got != "hdrtok" {
		t.Fatalf("bearer token not extracted: %q", got)
	}
}

func TestAppendQueryToHLSSegments(t *testing.T) {
	in := "#EXTM3U\n#EXTINF:4.0,\nseg_00000.ts\n#EXTINF:4.0,\nseg_00001.ts?old=1\n"
	got := appendQueryToHLSSegments(in, "token=abc")
	if !strings.Contains(got, "seg_00000.ts?token=abc") {
		t.Fatalf("missing tokenized segment: %q", got)
	}
	if !strings.Contains(got, "seg_00001.ts?old=1") {
		t.Fatalf("existing query should be preserved: %q", got)
	}
}
