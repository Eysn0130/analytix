package webfetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	netclient "analytix.local/runtime-go/internal/netclient"
)

type Policy struct {
	AllowDomains []string
	DenyDomains  []string
}

type Request struct {
	URL      string
	MaxBytes int
	Timeout  time.Duration
}

type Client struct {
	Policy          Policy
	ProxySpec       netclient.ProxySpec
	ProviderID      string
	DefaultMaxBytes int
	DefaultTimeout  time.Duration
	UserAgent       string
	Now             func() time.Time
	OnProgress      func(host string)
}

type RoundTripper struct {
	ProxySpec netclient.ProxySpec
	Timeout   time.Duration
}

func ProxySpecForURL(proxyURL string) netclient.ProxySpec {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return netclient.ProxySpec{Mode: netclient.ModeAuto}
	}
	return netclient.ProxySpec{Mode: netclient.ModeCustom, URL: proxyURL}
}

func ParseRequest(args map[string]any, maxCap int, defaultMaxBytes int, minBytes int, defaultTimeout time.Duration) (Request, map[string]any, bool) {
	rawURL := strings.TrimSpace(firstNonEmptyString(args["url"]))
	if rawURL == "" {
		return Request{}, map[string]any{"code": "invalid_url", "error": "url is required"}, true
	}
	maxBytes := positiveLimit(args["max_bytes"], maxCap, maxCap)
	if maxBytes < minBytes {
		if maxCap < minBytes {
			maxBytes = maxCap
		} else {
			maxBytes = minBytes
		}
	}
	timeoutMS := positiveLimit(args["timeout_ms"], int(defaultTimeout/time.Millisecond), int(defaultTimeout/time.Millisecond))
	return Request{
		URL:      rawURL,
		MaxBytes: maxBytes,
		Timeout:  time.Duration(timeoutMS) * time.Millisecond,
	}, nil, false
}

func (c Client) Fetch(ctx context.Context, request Request) (map[string]any, bool) {
	startedAt := c.now()
	policyURL, failure, failed := ValidateURL(request.URL, c.Policy)
	if failed {
		failure["telemetry"] = Telemetry(startedAt, "blocked", c.ProviderID, request.URL, 0)
		return failure, true
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = c.defaultTimeout()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, policyURL.String(), nil)
	if err != nil {
		return map[string]any{"code": "fetch_failed", "error": err.Error()}, true
	}
	httpRequest.Header.Set("User-Agent", c.userAgent())
	httpRequest.Header.Set("Accept", "text/html,text/plain,application/json,application/xml,text/markdown,*/*;q=0.8")
	client := &http.Client{
		Timeout: timeout,
		Transport: RoundTripper{
			ProxySpec: c.ProxySpec,
			Timeout:   c.defaultTimeout(),
		},
		CheckRedirect: func(redirect *http.Request, via []*http.Request) error {
			if redirect == nil || redirect.URL == nil {
				return errors.New("redirect URL is missing")
			}
			if _, failure, failed := ValidateURL(redirect.URL.String(), c.Policy); failed {
				return fmt.Errorf("redirect blocked: %s", firstNonEmptyString(failure["error"], failure["code"]))
			}
			return nil
		},
	}
	if c.OnProgress != nil {
		c.OnProgress(policyURL.Hostname())
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return map[string]any{
			"code":      "fetch_failed",
			"error":     err.Error(),
			"url":       SanitizeURL(policyURL.String()),
			"telemetry": Telemetry(startedAt, "allowed", c.ProviderID, policyURL.String(), 0),
		}, true
	}
	defer response.Body.Close()
	body, truncated, readErr := ReadLimitedResponse(response.Body, request.MaxBytes, c.DefaultMaxBytes)
	if readErr != nil {
		return map[string]any{
			"code":       "fetch_failed",
			"error":      readErr.Error(),
			"status":     response.Status,
			"statusCode": float64(response.StatusCode),
			"url":        SanitizeURL(policyURL.String()),
			"finalUrl":   SanitizeURL(FinalResponseURL(response, policyURL.String())),
			"telemetry":  Telemetry(startedAt, "allowed", c.ProviderID, policyURL.String(), len(body)),
		}, true
	}
	finalURL := FinalResponseURL(response, policyURL.String())
	contentType := response.Header.Get("Content-Type")
	extractedTitle, extractedText := ExtractReadableText(string(body), contentType)
	retrievedAt := c.now().UTC().Format(time.RFC3339Nano)
	source := map[string]any{
		"sourceId":    SourceID(finalURL),
		"url":         SanitizeURL(finalURL),
		"retrievedAt": retrievedAt,
	}
	if extractedTitle != "" {
		source["title"] = extractedTitle
	}
	output := map[string]any{
		"sourceId":    source["sourceId"],
		"url":         SanitizeURL(policyURL.String()),
		"finalUrl":    SanitizeURL(finalURL),
		"retrievedAt": retrievedAt,
		"contentType": contentType,
		"text":        extractedText,
		"byteCount":   float64(len(body)),
		"truncated":   truncated,
		"sources":     []any{source},
		"citations":   []any{source},
		"telemetry":   Telemetry(startedAt, "allowed", c.ProviderID, policyURL.String(), len(body)),
	}
	if extractedTitle != "" {
		output["title"] = extractedTitle
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		output["code"] = "fetch_failed"
		output["error"] = fmt.Sprintf("HTTP %d", response.StatusCode)
		output["status"] = response.Status
		output["statusCode"] = float64(response.StatusCode)
		return output, true
	}
	return output, false
}

func ValidateURL(rawURL string, policy Policy) (*url.URL, map[string]any, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil || !parsed.IsAbs() {
		return nil, map[string]any{"code": "invalid_url", "error": "URL must be absolute"}, true
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, map[string]any{"code": "policy_blocked", "error": "only http and https URLs are allowed"}, true
	}
	if parsed.User != nil {
		return nil, map[string]any{"code": "policy_blocked", "error": "URL credentials are not allowed"}, true
	}
	hostname := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if hostname == "" {
		return nil, map[string]any{"code": "invalid_url", "error": "URL hostname is required"}, true
	}
	if ip := net.ParseIP(hostname); ip != nil && BlockedIP(ip) {
		return nil, map[string]any{"code": "ssrf_blocked", "error": "refusing to fetch internal address " + hostname}, true
	}
	for _, domain := range policy.DenyDomains {
		if DomainMatches(hostname, domain) {
			return nil, map[string]any{"code": "policy_blocked", "error": "domain is denied: " + hostname}, true
		}
	}
	if len(policy.AllowDomains) > 0 {
		allowed := false
		for _, domain := range policy.AllowDomains {
			if DomainMatches(hostname, domain) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, map[string]any{"code": "policy_blocked", "error": "domain is not allowed: " + hostname}, true
		}
	}
	return parsed, nil, false
}

func (rt RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, errors.New("request URL is missing")
	}
	if ip := net.ParseIP(req.URL.Hostname()); ip != nil && BlockedIP(ip) {
		return nil, fmt.Errorf("refusing to fetch internal address %s", ip.String())
	}
	proxyFunc, err := netclient.ProxyFunc(rt.ProxySpec)
	if err != nil {
		return nil, err
	}
	var proxyURL *url.URL
	if proxyFunc != nil {
		proxyURL, err = proxyFunc(req)
		if err != nil {
			return nil, err
		}
	}
	transport := Transport(proxyURL, rt.timeout())
	defer transport.CloseIdleConnections()
	return transport.RoundTrip(req)
}

func Transport(proxyURL *url.URL, timeout time.Duration) *http.Transport {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.TLSHandshakeTimeout = timeout
	base.ResponseHeaderTimeout = timeout
	if proxyURL != nil {
		base.Proxy = http.ProxyURL(proxyURL)
		base.DialContext = (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext
		return base
	}
	base.Proxy = nil
	base.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		return DialContext(ctx, network, address, timeout)
	}
	return base
}

func DialContext(ctx context.Context, network string, address string, timeout time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if BlockedIP(ip) {
			return nil, fmt.Errorf("refusing to fetch internal address %s", ip.String())
		}
		dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses resolved for %s", host)
	}
	for _, resolved := range ips {
		if BlockedIP(resolved.IP) {
			return nil, fmt.Errorf("refusing to fetch internal address %s (resolves to %s)", host, resolved.IP.String())
		}
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

var cgnatRange = mustCIDR("100.64.0.0/10")

func mustCIDR(raw string) *net.IPNet {
	_, cidr, err := net.ParseCIDR(raw)
	if err != nil {
		panic(err)
	}
	return cidr
}

func BlockedIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() {
		return false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	return ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		cgnatRange.Contains(ip)
}

func ReadLimitedResponse(body io.Reader, maxBytes int, defaultMaxBytes int) ([]byte, bool, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(body, int64(maxBytes)+1))
	if err != nil {
		return data, false, err
	}
	if len(data) > maxBytes {
		return data[:maxBytes], true, nil
	}
	return data, false, nil
}

func ExtractReadableText(raw string, contentType string) (string, string) {
	if !strings.Contains(strings.ToLower(contentType), "html") && !LooksLikeHTML(raw) {
		return "", NormalizeWhitespace(strings.ToValidUTF8(raw, ""))
	}
	title := ""
	if match := titleRE.FindStringSubmatch(raw); len(match) > 1 {
		title = NormalizeWhitespace(html.UnescapeString(match[1]))
	}
	text := commentRE.ReplaceAllString(raw, " ")
	text = scriptRE.ReplaceAllString(text, " ")
	text = styleRE.ReplaceAllString(text, " ")
	text = tagRE.ReplaceAllString(text, " ")
	text = NormalizeWhitespace(html.UnescapeString(text))
	return title, text
}

func LooksLikeHTML(raw string) bool {
	head := strings.ToLower(raw)
	if len(head) > 512 {
		head = head[:512]
	}
	return strings.Contains(head, "<!doctype html") || strings.Contains(head, "<html")
}

var (
	titleRE   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	commentRE = regexp.MustCompile(`(?is)<!--.*?-->`)
	scriptRE  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRE   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	tagRE     = regexp.MustCompile(`(?is)<[^>]+>`)
)

func NormalizeWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func FinalResponseURL(response *http.Response, fallback string) string {
	if response != nil && response.Request != nil && response.Request.URL != nil {
		return response.Request.URL.String()
	}
	return fallback
}

func SourceID(finalURL string) string {
	sum := sha256.Sum256([]byte(finalURL))
	return "web_" + hex.EncodeToString(sum[:])[:16]
}

func SanitizeURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return raw
	}
	parsed.User = nil
	return parsed.String()
}

func Telemetry(startedAt time.Time, policy string, providerID string, rawURL string, byteCount int) map[string]any {
	return map[string]any{
		"durationMs": float64(time.Since(startedAt).Milliseconds()),
		"policy":     policy,
		"provider":   providerID,
		"url":        SanitizeURL(rawURL),
		"byteCount":  float64(byteCount),
	}
}

func DomainMatches(hostname string, domain string) bool {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	domain = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(domain, ".")))
	return hostname != "" && domain != "" && (hostname == domain || strings.HasSuffix(hostname, "."+domain))
}

func (rt RoundTripper) timeout() time.Duration {
	if rt.Timeout > 0 {
		return rt.Timeout
	}
	return 15 * time.Second
}

func (c Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c Client) defaultTimeout() time.Duration {
	if c.DefaultTimeout > 0 {
		return c.DefaultTimeout
	}
	return 15 * time.Second
}

func (c Client) userAgent() string {
	if strings.TrimSpace(c.UserAgent) != "" {
		return c.UserAgent
	}
	return "analytix-web-fetch/1.0"
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func positiveLimit(value any, fallback int, max int) int {
	limit := fallback
	switch typed := value.(type) {
	case int:
		limit = typed
	case int64:
		limit = int(typed)
	case float64:
		limit = int(typed)
	case jsonNumber:
		if parsed, err := typed.Int64(); err == nil {
			limit = int(parsed)
		}
	}
	if limit <= 0 {
		limit = fallback
	}
	if max > 0 && limit > max {
		return max
	}
	return limit
}

type jsonNumber interface {
	Int64() (int64, error)
}
