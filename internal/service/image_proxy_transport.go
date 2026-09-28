package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

var errImageProxyInternalTarget = errors.New("requests to private/internal hosts are not allowed")

type imageSafeTransport struct {
	base   *http.Transport
	lookup func(context.Context, string) ([]net.IPAddr, error)
}

// Resolve once, validate every returned address, and pin the URL used by both
// direct connections and configured proxies. Checking DNS and then dialing the
// original hostname would permit a second lookup (including proxy-side DNS) to
// select an internal address. Preserve HTTP Host and TLS SNI for public CDNs.
func (t *imageSafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Hostname()
	if isPrivateHost(host) {
		return nil, errImageProxyInternalTarget
	}
	addresses := []net.IPAddr{{IP: net.ParseIP(host)}}
	if addresses[0].IP == nil {
		var err error
		addresses, err = t.lookup(req.Context(), host)
		if err != nil {
			return nil, err
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no addresses for image host %s", host)
	}
	for _, address := range addresses {
		if address.IP == nil || address.Zone != "" || isPrivateHost(address.IP.String()) {
			return nil, errImageProxyInternalTarget
		}
	}
	port := req.URL.Port()
	if port == "" {
		port = "80"
		if req.URL.Scheme == "https" {
			port = "443"
		}
	}
	var proxy *url.URL
	if t.base.Proxy != nil {
		var err error
		proxy, err = t.base.Proxy(req)
		if err != nil {
			return nil, err
		}
	}
	transport := t.base.Clone()
	transport.Proxy = http.ProxyURL(proxy)
	// These per-request transports must not retain idle sockets after their
	// response is consumed. The original-image cache amortizes connections.
	transport.DisableKeepAlives = true
	transport.DialTLS = nil
	transport.DialTLSContext = nil
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	}
	transport.TLSClientConfig.ServerName = host
	if proxy != nil && (proxy.Scheme == "http" || proxy.Scheme == "https") {
		// Tunnel even plaintext HTTP. Absolute-form proxy requests let some
		// proxies replace the original Host with the pinned IP, breaking CDN
		// virtual hosting; CONNECT keeps the routing address and Host separate.
		transport.Proxy = nil
		transport.DialContext = imageProxyTunnelDialer(t.base, proxy)
	}
	var lastErr error
	for _, address := range addresses {
		request := req.Clone(req.Context())
		request.Host = req.URL.Host
		request.URL.Host = net.JoinHostPort(address.IP.String(), port)
		response, err := transport.RoundTrip(request)
		if response != nil {
			response.Request = req
		}
		if err == nil || req.Context().Err() != nil {
			return response, err
		}
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		lastErr = err
		// Images are GETs. Retry another address from the same validated DNS
		// result when one CDN address or address family is unavailable.
	}
	return nil, lastErr
}

func imageProxyTunnelDialer(base *http.Transport, proxy *url.URL) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, target string) (_ net.Conn, err error) {
		port := proxy.Port()
		if port == "" {
			port = "80"
			if proxy.Scheme == "https" {
				port = "443"
			}
		}
		dial := base.DialContext
		if dial == nil {
			dial = (&net.Dialer{Timeout: 15 * time.Second}).DialContext
		}
		conn, err := dial(ctx, network, net.JoinHostPort(proxy.Hostname(), port))
		if err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				_ = conn.Close()
			}
		}()
		rawConn := conn
		stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
		defer stop()
		if proxy.Scheme == "https" {
			cfg := &tls.Config{MinVersion: tls.VersionTLS12}
			if base.TLSClientConfig != nil {
				cfg = base.TLSClientConfig.Clone()
			}
			cfg.ServerName = proxy.Hostname()
			cfg.NextProtos = []string{"http/1.1"}
			tlsConn := tls.Client(conn, cfg)
			if err = tlsConn.HandshakeContext(ctx); err != nil {
				return nil, err
			}
			conn = tlsConn
		}
		headers := base.ProxyConnectHeader.Clone()
		if base.GetProxyConnectHeader != nil {
			headers, err = base.GetProxyConnectHeader(ctx, proxy, target)
			if err != nil {
				return nil, err
			}
			headers = headers.Clone()
		}
		if headers == nil {
			headers = make(http.Header)
		}
		if proxy.User != nil {
			password, _ := proxy.User.Password()
			headers.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(proxy.User.Username()+":"+password)))
		}
		request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: headers}
		if err = request.Write(conn); err != nil {
			return nil, err
		}
		response, err := http.ReadResponse(bufio.NewReader(conn), request)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("image proxy CONNECT: %s", response.Status)
		}
		return conn, nil
	}
}

func (p *ImageProxy) securedImageClient(client *http.Client) *http.Client {
	secured := *client
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if base, ok := transport.(*http.Transport); ok {
		lookup := p.imageLookupIP
		if lookup == nil {
			lookup = net.DefaultResolver.LookupIPAddr
		}
		secured.Transport = &imageSafeTransport{base: base, lookup: lookup}
	}
	secured.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if _, err := p.validateURL(req.URL.String()); err != nil {
			return err
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &secured
}
