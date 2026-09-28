package service

import (
	"errors"
	"net/url"
	"strings"
)

const (
	ProxyPoolTypeNormal = "normal"
	ProxyPoolTypeResin  = "resin"
)

func effectiveProxyPoolType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ProxyPoolTypeNormal
	}
	return value
}

func normalizeProxyPoolType(raw string) (string, error) {
	value := effectiveProxyPoolType(raw)
	if value != ProxyPoolTypeNormal && value != ProxyPoolTypeResin {
		return "", errors.New("proxy pool type must be normal or resin")
	}
	return value, nil
}

func normalizeResinProxyOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("resin proxy address must be an HTTP(S) origin")
	}
	return u.Scheme + "://" + u.Host, nil
}
