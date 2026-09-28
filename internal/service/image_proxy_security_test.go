package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"go.uber.org/zap"
)

func TestImageProxyBlocksLocalNamesAndRedirects(t *testing.T) {
	for _, target := range []string{"http://localhost/image.jpg", "http://LOCALHOST./image.jpg", "http://sub.localhost/image.jpg", "http://127.0.0.1/image.jpg", "http://[::1]/image.jpg", "http://169.254.169.254/image.jpg"} {
		t.Run(target, func(t *testing.T) {
			p := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
			calls := 0
			p.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls > 1 {
					t.Fatal("redirect reached internal transport")
				}
				return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {target}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
			})}
			if _, err := p.validateURL(target); err == nil {
				t.Error("direct internal URL allowed")
			}
			_, _, err := p.Fetch(t.Context(), "https://public.example/image.jpg")
			if err == nil {
				t.Fatal("internal redirect allowed")
			}
			if calls != 1 {
				t.Fatalf("transport calls = %d", calls)
			}
		})
	}
}

func TestImageProxyRejectsPrivateDNSBeforeDial(t *testing.T) {
	for _, addresses := range [][]net.IPAddr{{{IP: net.ParseIP("127.0.0.1")}}, {{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("10.0.0.1")}}, {{IP: net.ParseIP("::ffff:192.168.1.2")}}} {
		p := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
		p.imageLookupIP = func(context.Context, string) ([]net.IPAddr, error) { return addresses, nil }
		calls := 0
		p.client = &http.Client{Transport: &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
			calls++
			return nil, errors.New("unexpected network request")
		}}}
		if _, _, err := p.Fetch(t.Context(), "https://img1.doubanio.com/private.jpg"); err == nil {
			t.Fatal("private DNS address allowed")
		}
		if calls != 0 {
			t.Fatalf("dial called %d times", calls)
		}
	}
}

func TestImageProxyPinsPublicDNSAndRetainsHost(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Host != "public.example" {
			t.Errorf("Host = %q", r.Host)
		}
		_, _ = w.Write(testJPEG)
	}))
	defer server.Close()
	p := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	lookups := 0
	p.imageLookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		lookups++
		if lookups > 1 {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	serverURL, _ := url.Parse(server.URL)
	p.client = &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "93.184.216.34:80" {
			t.Fatalf("dial address was not pinned: %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, serverURL.Host)
	}}}
	if _, _, err := p.Fetch(t.Context(), "http://public.example/image.jpg"); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || lookups != 1 {
		t.Fatalf("requests=%d lookups=%d", requests, lookups)
	}
}

func TestImageProxyRetriesOtherValidatedPublicAddresses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(testJPEG) }))
	defer server.Close()
	serverURL, _ := url.Parse(server.URL)
	p := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	lookups := 0
	p.imageLookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		lookups++
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("93.184.216.35")}}, nil
	}
	var attempts []string
	base := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		attempts = append(attempts, address)
		if address == "93.184.216.34:80" {
			return nil, errors.New("first address unreachable")
		}
		if address != "93.184.216.35:80" {
			return nil, errors.New("unvalidated dial target")
		}
		return (&net.Dialer{}).DialContext(ctx, network, serverURL.Host)
	}}
	client := p.securedImageClient(&http.Client{Transport: base})
	response, err := client.Get("http://public.example/image.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if len(attempts) != 2 || lookups != 1 {
		t.Fatalf("attempts=%v lookups=%d", attempts, lookups)
	}
}

func TestImageProxyPinsTargetThroughConfiguredProxy(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "public.example" {
			t.Errorf("upstream Host = %q", r.Host)
		}
		_, _ = w.Write(testJPEG)
	}))
	defer origin.Close()
	originURL, _ := url.Parse(origin.URL)
	proxyCalls := 0
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls++
		if r.Method != http.MethodConnect || r.Host != "93.184.216.34:80" {
			t.Errorf("proxy target = %s %q", r.Method, r.Host)
			w.WriteHeader(400)
			return
		}
		upstream, err := net.Dial("tcp", originURL.Host)
		if err != nil {
			t.Error(err)
			return
		}
		defer upstream.Close()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		go func() { _, _ = io.Copy(upstream, conn) }()
		_, _ = io.Copy(conn, upstream)
	}))
	defer proxyServer.Close()
	proxyURL, _ := url.Parse(proxyServer.URL)
	p := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	p.imageLookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	p.client = &http.Client{Transport: &http.Transport{Proxy: func(req *http.Request) (*url.URL, error) {
		if req.URL.Host != "public.example" {
			t.Errorf("proxy selected using rewritten host %q", req.URL.Host)
		}
		return proxyURL, nil
	}}}
	if _, _, err := p.Fetch(t.Context(), "http://public.example/image.jpg"); err != nil {
		t.Fatal(err)
	}
	if proxyCalls != 1 {
		t.Fatalf("proxy requests = %d", proxyCalls)
	}
}

func TestImageProxyPinsHTTPSAndRetainsCertificateHostname(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "example.com" || r.TLS.ServerName != "example.com" {
			t.Errorf("Host=%q SNI=%q", r.Host, r.TLS.ServerName)
		}
		_, _ = w.Write(testJPEG)
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	serverURL, _ := url.Parse(server.URL)
	p := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	p.imageLookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	p.client = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "93.184.216.34:443" {
			t.Fatalf("dial address = %q", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, serverURL.Host)
	}}}
	if _, _, err := p.Fetch(t.Context(), "https://example.com/image.jpg"); err != nil {
		t.Fatal(err)
	}
}

func TestImageProxyChecksRedirectDNSWithoutFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://private.example/image.jpg", http.StatusFound)
	}))
	defer server.Close()
	serverURL, _ := url.Parse(server.URL)
	p := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	lookups, dials := 0, 0
	p.imageLookupIP = func(_ context.Context, host string) ([]net.IPAddr, error) {
		lookups++
		ip := "93.184.216.34"
		if host == "private.example" {
			ip = "10.0.0.1"
		}
		return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil
	}
	p.client = &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		dials++
		if address != "93.184.216.34:80" {
			t.Fatalf("unsafe dial address = %q", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, serverURL.Host)
	}}}
	if _, _, err := p.Fetch(t.Context(), "http://img1.doubanio.com/image.jpg"); !errors.Is(err, errImageProxyInternalTarget) {
		t.Fatalf("error = %v", err)
	}
	if lookups != 2 || dials != 1 {
		t.Fatalf("lookups=%d dials=%d", lookups, dials)
	}
}
