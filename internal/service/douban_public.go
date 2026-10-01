package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"
)

const doubanPublicRetryDelay = 200 * time.Millisecond

// requestPublicJSON tolerates one interrupted search/feed request. The retry
// uses the same configured transport, cookie and total request budget. Proxy
// pool requests already have their own routing/fallback policy.
func (d *DoubanProvider) requestPublicJSON(ctx context.Context, requestURL, referer string) ([]byte, int, error) {
	resolved := d.resolveConfig(ctx)
	if resolved.UseProxyPool {
		return d.requestJSON(ctx, requestURL, referer)
	}
	d.resetRoute(resolved.Revision)
	ctx, cancel := context.WithTimeout(ctx, doubanRequestTimeout)
	defer cancel()
	result := d.doJSONRequest(ctx, d.client, resolved, requestURL, referer)
	if !shouldRetryDoubanPublicRequest(result) {
		return result.body, result.status, result.err
	}
	if err := ctx.Err(); err != nil {
		return nil, result.status, err
	}
	timer := time.NewTimer(doubanPublicRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, result.status, ctx.Err()
	case <-timer.C:
	}
	result = d.doJSONRequest(ctx, d.client, resolved, requestURL, referer)
	return result.body, result.status, result.err
}

func shouldRetryDoubanPublicRequest(result doubanHTTPResult) bool {
	// HTTP errors (including rate limits and permission failures) must retain
	// their original status and must not trigger another request.
	if result.status >= http.StatusBadRequest || result.err == nil || errors.Is(result.err, context.Canceled) {
		return false
	}
	if errors.Is(result.err, io.EOF) || errors.Is(result.err, io.ErrUnexpectedEOF) ||
		errors.Is(result.err, syscall.ECONNRESET) || errors.Is(result.err, syscall.ECONNABORTED) ||
		errors.Is(result.err, syscall.EPIPE) {
		return true
	}
	var networkErr net.Error
	return errors.As(result.err, &networkErr) && (networkErr.Timeout() || networkErr.Temporary())
}
