package model

import (
	"net/url"
	"strings"
)

// CustomEndpointRequestShape classifies protocol shape without performing I/O.
// Provider transport and the application loop use the same interpretation.
func CustomEndpointRequestShape(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "chat_completions"
	}
	path := strings.ToLower(strings.TrimRight(parsed.EscapedPath(), "/"))
	switch {
	case strings.HasSuffix(path, "/responses"):
		return "responses"
	case strings.HasSuffix(path, "/messages"):
		return "messages"
	case strings.HasSuffix(path, "/chat/completions"), strings.HasSuffix(path, "/completions"):
		return "chat_completions"
	default:
		return "chat_completions"
	}
}
