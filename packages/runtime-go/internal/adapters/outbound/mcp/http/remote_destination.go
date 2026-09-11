package httpmcp

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

var (
	ErrMCPRemoteDestinationDisallowed = errors.New("MCP remote destination is not public")
	ErrMCPRemoteProxyDisallowed       = errors.New("MCP remote proxy is disabled")
)

type remoteEndpoint struct {
	scheme   string
	hostname string
}

type remoteIPResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type remoteContextDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

var deniedMCPRemotePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("::ffff:0:0/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func parseMCPRemoteEndpoint(rawURL string) (remoteEndpoint, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return remoteEndpoint{}, ErrMCPRemoteDestinationDisallowed
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return remoteEndpoint{}, ErrMCPRemoteDestinationDisallowed
	}
	if parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" || parsed.Fragment != "" {
		return remoteEndpoint{}, ErrMCPRemoteDestinationDisallowed
	}
	hostname := strings.TrimSpace(parsed.Hostname())
	if hostname == "" || strings.Contains(hostname, "%") || strings.ContainsAny(hostname, `/\`) {
		return remoteEndpoint{}, ErrMCPRemoteDestinationDisallowed
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return remoteEndpoint{}, ErrMCPRemoteDestinationDisallowed
		}
	}
	normalizedHost := strings.ToLower(strings.TrimSuffix(hostname, "."))
	if normalizedHost == "" || normalizedHost == "localhost" || strings.HasSuffix(normalizedHost, ".localhost") {
		return remoteEndpoint{}, ErrMCPRemoteDestinationDisallowed
	}
	if address, err := netip.ParseAddr(normalizedHost); err == nil {
		if !isPublicMCPRemoteAddress(address) {
			return remoteEndpoint{}, ErrMCPRemoteDestinationDisallowed
		}
	} else {
		if looksLikeAmbiguousNumericHost(normalizedHost) ||
			!strings.Contains(normalizedHost, ".") ||
			isSpecialUseMCPRemoteHostname(normalizedHost) {
			return remoteEndpoint{}, ErrMCPRemoteDestinationDisallowed
		}
	}
	return remoteEndpoint{scheme: scheme, hostname: normalizedHost}, nil
}

func secureMCPRemoteDialContext(
	resolver remoteIPResolver,
	dialer remoteContextDialer,
) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network string, address string) (net.Conn, error) {
		if resolver == nil || dialer == nil {
			return nil, ErrMCPRemoteDestinationDisallowed
		}
		host, port, err := net.SplitHostPort(strings.TrimSpace(address))
		if err != nil || strings.TrimSpace(port) == "" || strings.Contains(host, "%") {
			return nil, ErrMCPRemoteDestinationDisallowed
		}
		host = strings.TrimSpace(strings.Trim(host, "[]"))
		if host == "" || looksLikeAmbiguousNumericHost(host) {
			if parsed, parseErr := netip.ParseAddr(host); parseErr != nil || !isPublicMCPRemoteAddress(parsed) {
				return nil, ErrMCPRemoteDestinationDisallowed
			}
		}

		addresses := make([]netip.Addr, 0, 2)
		if parsed, parseErr := netip.ParseAddr(host); parseErr == nil {
			addresses = append(addresses, parsed)
		} else {
			resolved, resolveErr := resolver.LookupNetIP(ctx, "ip", host)
			if resolveErr != nil || len(resolved) == 0 {
				return nil, ErrMCPRemoteDestinationDisallowed
			}
			addresses = append(addresses, resolved...)
		}
		for _, resolved := range addresses {
			if !isPublicMCPRemoteAddress(resolved) {
				return nil, ErrMCPRemoteDestinationDisallowed
			}
		}

		var lastErr error
		for _, resolved := range addresses {
			connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			return nil, ErrMCPRemoteDestinationDisallowed
		}
		return nil, lastErr
	}
}

func isPublicMCPRemoteAddress(address netip.Addr) bool {
	if !address.IsValid() || address.Zone() != "" || address.Is4In6() || !address.IsGlobalUnicast() ||
		address.IsUnspecified() || address.IsLoopback() || address.IsPrivate() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() ||
		address.IsMulticast() {
		return false
	}
	for _, prefix := range deniedMCPRemotePrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func looksLikeAmbiguousNumericHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return false
	}
	allDecimalPunctuation := true
	for _, character := range host {
		if (character < '0' || character > '9') && character != '.' {
			allDecimalPunctuation = false
			break
		}
	}
	if allDecimalPunctuation {
		return true
	}
	if strings.HasPrefix(host, "0x") && len(host) > 2 {
		for _, character := range host[2:] {
			if !isASCIIHex(character) {
				return false
			}
		}
		return true
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" {
			return false
		}
		if strings.HasPrefix(label, "0x") && len(label) > 2 {
			for _, character := range label[2:] {
				if !isASCIIHex(character) {
					return false
				}
			}
			continue
		}
		for _, character := range label {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func isSpecialUseMCPRemoteHostname(host string) bool {
	for _, suffix := range []string{
		".internal",
		".invalid",
		".lan",
		".local",
		".localdomain",
		".localhost",
		".onion",
		".test",
		".home.arpa",
	} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func isASCIIHex(character rune) bool {
	return character >= '0' && character <= '9' ||
		character >= 'a' && character <= 'f'
}
