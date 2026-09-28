package service

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"github.com/ShukeBta/MediaStationGo/internal/helper"
)

const fd2PPVDirectTimeoutSeconds = 30

type fd2PPVDirectFetchResult struct {
	body       string
	statusCode int
	cookies    []helper.FlareSolverrCookie
}

type fd2PPVDirectFetcher interface {
	fetch(
		ctx context.Context,
		targetURL string,
		userAgent string,
		cookies []helper.FlareSolverrCookie,
	) (fd2PPVDirectFetchResult, error)
}

type fd2PPVTLSFetcher struct {
	client tlsclient.HttpClient
}

func (f *fd2PPVTLSFetcher) fetch(
	ctx context.Context,
	targetURL string,
	userAgent string,
	cookies []helper.FlareSolverrCookie,
) (fd2PPVDirectFetchResult, error) {
	client, err := f.httpClient()
	if err != nil {
		return fd2PPVDirectFetchResult{}, err
	}
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return fd2PPVDirectFetchResult{}, fmt.Errorf("parse target URL: %w", err)
	}
	client.SetCookies(parsedURL, fd2PPVFHTTPCookies(cookies))

	request, err := fhttp.NewRequestWithContext(ctx, fhttp.MethodGet, targetURL, nil)
	if err != nil {
		return fd2PPVDirectFetchResult{}, fmt.Errorf("create request: %w", err)
	}
	request.Header = fhttp.Header{
		"accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
		"accept-language":           {"ja,en-US;q=0.9,en;q=0.8"},
		"referer":                   {strings.TrimRight(fd2PPVBaseURL, "/") + "/"},
		"sec-ch-ua":                 {`"Not_A Brand";v="99", "Chromium";v="133", "Google Chrome";v="133"`},
		"sec-ch-ua-mobile":          {"?0"},
		"sec-ch-ua-platform":        {`"Linux"`},
		"sec-fetch-dest":            {"document"},
		"sec-fetch-mode":            {"navigate"},
		"sec-fetch-site":            {"same-origin"},
		"sec-fetch-user":            {"?1"},
		"upgrade-insecure-requests": {"1"},
		"user-agent":                {firstNonEmpty(strings.TrimSpace(userAgent), fd2PPVChrome133UserAgent)},
		fhttp.HeaderOrderKey: {
			"accept",
			"accept-language",
			"referer",
			"sec-ch-ua",
			"sec-ch-ua-mobile",
			"sec-ch-ua-platform",
			"sec-fetch-dest",
			"sec-fetch-mode",
			"sec-fetch-site",
			"sec-fetch-user",
			"upgrade-insecure-requests",
			"user-agent",
		},
	}

	response, err := client.Do(request)
	if err != nil {
		return fd2PPVDirectFetchResult{}, fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return fd2PPVDirectFetchResult{}, fmt.Errorf("read response: %w", err)
	}
	return fd2PPVDirectFetchResult{
		body:       string(body),
		statusCode: response.StatusCode,
		cookies:    fd2PPVFlareSolverrCookies(client.GetCookies(parsedURL), parsedURL.Hostname()),
	}, nil
}

func (f *fd2PPVTLSFetcher) httpClient() (tlsclient.HttpClient, error) {
	if f.client != nil {
		return f.client, nil
	}
	jar := tlsclient.NewCookieJar()
	client, err := tlsclient.NewHttpClient(
		tlsclient.NewNoopLogger(),
		tlsclient.WithClientProfile(profiles.Chrome_133),
		tlsclient.WithCookieJar(jar),
		tlsclient.WithTimeoutSeconds(fd2PPVDirectTimeoutSeconds),
	)
	if err != nil {
		return nil, fmt.Errorf("create browser fingerprint client: %w", err)
	}
	f.client = client
	return f.client, nil
}

func fd2PPVFHTTPCookies(cookies []helper.FlareSolverrCookie) []*fhttp.Cookie {
	out := make([]*fhttp.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie.Name)
		if name == "" {
			continue
		}
		out = append(out, &fhttp.Cookie{
			Name:   name,
			Value:  cookie.Value,
			Domain: strings.TrimPrefix(strings.TrimSpace(cookie.Domain), "."),
			Path:   firstNonEmpty(strings.TrimSpace(cookie.Path), "/"),
			Secure: true,
		})
	}
	return out
}

func fd2PPVFlareSolverrCookies(cookies []*fhttp.Cookie, fallbackDomain string) []helper.FlareSolverrCookie {
	out := make([]helper.FlareSolverrCookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		out = append(out, helper.FlareSolverrCookie{
			Name:   cookie.Name,
			Value:  cookie.Value,
			Domain: firstNonEmpty(strings.TrimPrefix(strings.TrimSpace(cookie.Domain), "."), fallbackDomain),
			Path:   firstNonEmpty(strings.TrimSpace(cookie.Path), "/"),
		})
	}
	return out
}
