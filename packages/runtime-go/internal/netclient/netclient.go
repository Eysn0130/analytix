// Portions adapted from DeepSeek-Reasonix and modified by Analytix
// contributors. See THIRD_PARTY_NOTICES.md and the upstream provenance ledger.
package netclient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ModeAuto   = "auto"
	ModeEnv    = "env"
	ModeCustom = "custom"
	ModeOff    = "off"
)

type ProxySpec struct {
	Mode        string
	URL         string
	NoProxy     string
	Type        string
	Server      string
	Port        int
	Username    string
	Password    string
	DirectHosts []string
}

type TransportOptions struct {
	DialTimeout           time.Duration
	KeepAlive             time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	ClientTimeout         time.Duration
	ForceIPv4             bool
}

func NormalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ModeEnv:
		return ModeEnv
	case ModeCustom:
		return ModeCustom
	case ModeOff:
		return ModeOff
	default:
		return ModeAuto
	}
}

func Validate(spec ProxySpec) error {
	_, err := ProxyFunc(spec)
	return err
}

func ProxyFunc(spec ProxySpec) (func(*http.Request) (*url.URL, error), error) {
	base, err := baseProxyFunc(spec)
	if err != nil {
		return nil, err
	}
	return withDirectHosts(base, spec.DirectHosts), nil
}

func NewHTTPClient(spec ProxySpec, opts TransportOptions) (*http.Client, error) {
	transport, err := NewTransport(spec, opts)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport, Timeout: opts.ClientTimeout}, nil
}

func NewTransport(spec ProxySpec, opts TransportOptions) (*http.Transport, error) {
	transport := defaultTransport()
	proxy, err := ProxyFunc(spec)
	if err != nil {
		return nil, err
	}
	transport.Proxy = proxy
	if opts.DialTimeout != 0 || opts.KeepAlive != 0 || opts.ForceIPv4 {
		dialer := &net.Dialer{Timeout: opts.DialTimeout, KeepAlive: opts.KeepAlive}
		if opts.ForceIPv4 {
			if dialer.Timeout == 0 {
				dialer.Timeout = 30 * time.Second
			}
			if dialer.KeepAlive == 0 {
				dialer.KeepAlive = 30 * time.Second
			}
			transport.DialContext = func(ctx context.Context, _ string, address string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp4", address)
			}
		} else {
			transport.DialContext = dialer.DialContext
		}
	}
	if opts.TLSHandshakeTimeout != 0 {
		transport.TLSHandshakeTimeout = opts.TLSHandshakeTimeout
	}
	if opts.ResponseHeaderTimeout != 0 {
		transport.ResponseHeaderTimeout = opts.ResponseHeaderTimeout
	}
	return transport, nil
}

func Summary(spec ProxySpec) string {
	switch NormalizeMode(spec.Mode) {
	case ModeOff:
		return "off (direct)"
	case ModeEnv:
		return "env"
	case ModeCustom:
		proxyURL, err := customProxyURL(spec)
		if err != nil {
			return "custom (invalid)"
		}
		return "custom (" + redactProxyURL(proxyURL) + ")"
	default:
		return "auto (env)"
	}
}

func defaultTransport() *http.Transport {
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		return base.Clone()
	}
	return &http.Transport{Proxy: http.ProxyFromEnvironment}
}

func baseProxyFunc(spec ProxySpec) (func(*http.Request) (*url.URL, error), error) {
	switch NormalizeMode(spec.Mode) {
	case ModeOff:
		return nil, nil
	case ModeCustom:
		proxyURL, err := customProxyURL(spec)
		if err != nil {
			return nil, err
		}
		base := http.ProxyURL(proxyURL)
		noProxy := parseHostList(spec.NoProxy)
		return func(request *http.Request) (*url.URL, error) {
			if hostMatches(request.URL.Hostname(), noProxy) {
				return nil, nil
			}
			return base(request)
		}, nil
	case ModeEnv, ModeAuto:
		return http.ProxyFromEnvironment, nil
	default:
		return http.ProxyFromEnvironment, nil
	}
}

func withDirectHosts(proxy func(*http.Request) (*url.URL, error), hosts []string) func(*http.Request) (*url.URL, error) {
	if proxy == nil || len(hosts) == 0 {
		return proxy
	}
	directHosts := parseHostList(strings.Join(hosts, ","))
	return func(request *http.Request) (*url.URL, error) {
		if hostMatches(request.URL.Hostname(), directHosts) {
			return nil, nil
		}
		return proxy(request)
	}
}

func customProxyURL(spec ProxySpec) (*url.URL, error) {
	if raw := strings.TrimSpace(spec.URL); raw != "" {
		proxyURL, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("network proxy_url: %w", err)
		}
		proxyURL.Scheme = normalizeProxyScheme(proxyURL.Scheme)
		if err := validateProxyURL(proxyURL); err != nil {
			return nil, err
		}
		return proxyURL, nil
	}
	proxyType := normalizeProxyScheme(spec.Type)
	if proxyType == "" {
		proxyType = "http"
	}
	switch proxyType {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("network proxy type %q: must be http|https|socks5|socks5h", spec.Type)
	}
	server := strings.TrimSpace(spec.Server)
	if server == "" {
		return nil, fmt.Errorf("network proxy server is required when proxy mode is custom")
	}
	if spec.Port <= 0 || spec.Port > 65535 {
		return nil, fmt.Errorf("network proxy port must be 1..65535")
	}
	proxyURL := &url.URL{Scheme: proxyType, Host: net.JoinHostPort(server, strconv.Itoa(spec.Port))}
	if spec.Username != "" {
		if spec.Password != "" {
			proxyURL.User = url.UserPassword(spec.Username, spec.Password)
		} else {
			proxyURL.User = url.User(spec.Username)
		}
	}
	return proxyURL, nil
}

func normalizeProxyScheme(scheme string) string {
	normalized := strings.ToLower(strings.TrimSpace(scheme))
	if normalized == "socks" {
		return "socks5"
	}
	return normalized
}

func validateProxyURL(proxyURL *url.URL) error {
	switch strings.ToLower(proxyURL.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return fmt.Errorf("network proxy_url scheme %q: must be http|https|socks5|socks5h", proxyURL.Scheme)
	}
	if proxyURL.Hostname() == "" {
		return fmt.Errorf("network proxy_url host is required")
	}
	return nil
}

func redactProxyURL(proxyURL *url.URL) string {
	copyURL := *proxyURL
	if copyURL.User != nil {
		if username := copyURL.User.Username(); username != "" {
			copyURL.User = url.User(username)
		} else {
			copyURL.User = nil
		}
	}
	return copyURL.String()
}

func parseHostList(raw string) []string {
	hosts := []string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		host := strings.ToLower(strings.TrimSpace(part))
		host = strings.TrimPrefix(host, ".")
		if host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func hostMatches(host string, patterns []string) bool {
	normalized := strings.ToLower(strings.TrimSpace(host))
	if normalized == "" {
		return false
	}
	for _, pattern := range patterns {
		switch {
		case pattern == "*":
			return true
		case strings.HasPrefix(pattern, "*."):
			suffix := strings.TrimPrefix(pattern, "*.")
			if normalized == suffix || strings.HasSuffix(normalized, "."+suffix) {
				return true
			}
		case normalized == pattern || strings.HasSuffix(normalized, "."+pattern):
			return true
		}
	}
	return false
}
