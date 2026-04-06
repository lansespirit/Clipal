package config

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	supportedProxyURLSchemeList = "http, https, socks5, or socks5h"
	supportedProxyURLPrefixList = "http://, https://, socks5://, or socks5h://"
)

// ParseProxyURL validates a configured proxy URL and normalizes its scheme.
func ParseProxyURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return nil, fmt.Errorf("proxy_url must be an absolute %s URL", supportedProxyURLPrefixList)
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
		return parsed, nil
	default:
		return nil, fmt.Errorf("proxy_url scheme must be %s", supportedProxyURLSchemeList)
	}
}

// ValidateProxyURL reports whether a configured proxy URL is supported.
func ValidateProxyURL(raw string) error {
	_, err := ParseProxyURL(raw)
	return err
}
