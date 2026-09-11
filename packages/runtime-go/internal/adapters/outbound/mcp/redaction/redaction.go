package redaction

import (
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	"net/url"
	"regexp"
	"strings"
)

func RedactedDiagnostic(headers, env map[string]string, rawURL string) (string, bool) {
	rawSecrets := ExplicitAuthValues(headers, env, rawURL)
	cleanHeaders := RedactAuthMap(headers)
	cleanEnv := RedactAuthMap(env)
	cleanURL := RedactAuthURL(rawURL)
	parts := []string{
		"transport=http",
		"url=" + cleanURL,
		"Authorization=" + cleanHeaders["Authorization"],
		"MCP_API_KEY=" + cleanEnv["MCP_API_KEY"],
		"DEBUG=" + cleanEnv["DEBUG"],
	}
	diagnostic := strings.Join(parts, " ")
	return diagnostic, ContainsAnyValue(diagnostic, rawSecrets)
}

func RedactAuthMap(input map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range input {
		if IsAuthish(key) || ContainsExplicitAuthMaterial(value) {
			output[key] = "<redacted>"
			continue
		}
		output[key] = value
	}
	return output
}

func RedactAuthURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return raw
	}
	if parsed.User != nil {
		parsed.User = url.User("<redacted>")
	}
	query := parsed.Query()
	for key := range query {
		if IsAuthQueryKey(key) {
			query.Set(key, "<redacted>")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func IsAuthish(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(lower, "auth") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "credential") ||
		strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "api-key") ||
		strings.Contains(lower, "apikey") ||
		strings.Contains(lower, "cookie")
}

func ContainsExplicitAuthMaterial(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "access_token") ||
		strings.Contains(lower, "id_token") ||
		strings.Contains(lower, "refresh_token") ||
		strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "api-key") ||
		strings.Contains(lower, "apikey") ||
		strings.Contains(lower, "bearer ")
}

func RedactAuthText(value string) string {
	output := strings.TrimSpace(value)
	if output == "" {
		return ""
	}
	output = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`).ReplaceAllString(output, "Bearer <redacted>")
	output = regexp.MustCompile(`(?i)(authorization|x-api-key|api[_ -]?key|apikey|access_token|refresh_token|id_token|token)\s*[:=]\s*["']?[^"',\s}]+`).ReplaceAllString(output, "$1=<redacted>")
	return output
}

func DiagnosticText(text string, headers, env map[string]string, rawURL string) string {
	output := text
	for _, secret := range ExplicitAuthValues(headers, env, rawURL) {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		output = strings.ReplaceAll(output, secret, "<redacted>")
	}
	return RedactRestrictedPIIText(RedactAuthText(output))
}

func CatalogText(text string) string {
	output := strings.TrimSpace(text)
	if output == "" {
		return ""
	}
	parsed, err := url.Parse(output)
	if err == nil && parsed != nil && parsed.Scheme != "" {
		output = RedactAuthURL(output)
	}
	return RedactRestrictedPIIText(RedactAuthText(output))
}

func RedactRestrictedPIIText(value string) string {
	return domainprivacy.ProjectText(value).Text
}

func ExplicitAuthValues(headers, env map[string]string, rawURL string) []string {
	values := make([]string, 0)
	appendSecret := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}
	for key, value := range headers {
		if IsAuthish(key) || ContainsExplicitAuthMaterial(value) {
			appendSecret(value)
		}
	}
	for key, value := range env {
		if IsAuthish(key) || ContainsExplicitAuthMaterial(value) {
			appendSecret(value)
		}
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err == nil && parsed != nil {
		if parsed.User != nil {
			appendSecret(parsed.User.Username())
			if password, ok := parsed.User.Password(); ok {
				appendSecret(password)
			}
		}
		for key, queryValues := range parsed.Query() {
			if !IsAuthQueryKey(key) {
				continue
			}
			for _, value := range queryValues {
				appendSecret(value)
			}
		}
	}
	return values
}

func IsAuthQueryKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return lower == "key" || IsAuthish(lower)
}

func ContainsAnyValue(text string, values []string) bool {
	for _, value := range values {
		if value != "" && strings.Contains(text, value) {
			return true
		}
	}
	return false
}
