package model

import (
	"errors"
	"testing"
)

func TestParseEndpointFormatSeparatesMissingKnownAndUnknown(t *testing.T) {
	for input, want := range map[string]string{
		"chat": "chat_completions", "/v1/chat/completions": "chat_completions",
		"responses": "responses", "v1/messages": "messages", "custom-full-path": "custom_endpoint",
	} {
		got, supplied, err := ParseEndpointFormat(input)
		if err != nil || !supplied || got != want {
			t.Fatalf("ParseEndpointFormat(%q) = %q, %t, %v; want %q, true, nil", input, got, supplied, err, want)
		}
	}
	if got, supplied, err := ParseEndpointFormat("  "); err != nil || supplied || got != "" {
		t.Fatalf("missing endpoint format = %q, %t, %v; want empty, false, nil", got, supplied, err)
	}
	for _, input := range []string{"typoo", "chat_completionz", "unknown/messages"} {
		got, supplied, err := ParseEndpointFormat(input)
		if got != "" || !supplied || !errors.Is(err, ErrInvalidEndpointFormat) {
			t.Fatalf("unknown endpoint format %q did not fail closed: %q, %t, %v", input, got, supplied, err)
		}
		if NormalizeEndpointFormat(input) != "" || OptionalEndpointFormat(input) != "" {
			t.Fatalf("unknown endpoint format %q was normalized to a wire protocol", input)
		}
	}
}
