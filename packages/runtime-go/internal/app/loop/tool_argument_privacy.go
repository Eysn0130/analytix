package loop

import (
	"errors"
	"net"
	"net/url"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const providerFundsAccountFlowToolNameV1 = "mcp__analytix_funds__analyze_account_flows"

const providerWebFetchToolNameV1 = "web_fetch"

// validateAttemptPrivateToolArgumentsV1 decodes once before inspecting the
// public shape. Funds selectors are provider-safe aliases; no authority-ref
// exception exists at this boundary.
func validateAttemptPrivateToolArgumentsV1(call domainmodel.ToolCall) error {
	arguments, err := domainsecurity.DecodeCanonicalJSONObject(call.Arguments)
	if err != nil {
		return err
	}
	// An IP-literal web target is execution-private network input, not a value
	// that may be copied into a durable/public tool-call record. Keep the exact
	// URL for the web-fetch SSRF policy and runner, but replace only its parsed
	// host for this public-material inspection. Path, query, fragment, and
	// user-info remain inspected so this narrow exception cannot carry case
	// references, account identifiers, or provider reasoning to a network
	// effect.
	if call.Name == providerWebFetchToolNameV1 {
		if rawURL, ok := arguments["url"].(string); ok {
			parsed, parseErr := url.Parse(rawURL)
			if parseErr == nil {
				if net.ParseIP(parsed.Hostname()) != nil {
					parsed.Host = "privacy.invalid"
				}
				if err := validateWebFetchURLPrivacyLayersV1(parsed.String()); err != nil {
					return err
				}
				inspection := make(map[string]any, len(arguments))
				for field, value := range arguments {
					inspection[field] = value
				}
				inspection["url"] = parsed.String()
				arguments = inspection
			}
		}
	}
	return domainevent.ValidatePublicRecord(map[string]any{
		"kind":      "tool_call",
		"arguments": arguments,
	})
}

func validateWebFetchURLPrivacyLayersV1(value string) error {
	const maximumDecodingLayers = 4
	for range maximumDecodingLayers {
		if err := domainevent.ValidatePublicRecord(map[string]any{"url": value}); err != nil {
			return err
		}
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return err
		}
		if decoded == value {
			return nil
		}
		value = decoded
	}
	return errors.New("web fetch URL privacy encoding is too deeply nested")
}
