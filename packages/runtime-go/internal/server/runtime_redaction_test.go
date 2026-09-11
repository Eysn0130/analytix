package server

import (
	"errors"
	"strings"
	"testing"

	apploop "analytix.local/runtime-go/internal/app/loop"
)

func TestSanitizeProviderRetryMessageRedactsBearerAndSpacedAPIKey(t *testing.T) {
	message := `provider failed Authorization: Bearer runtime-secret-token api key: plain-secret-token token=response-secret-token sk-liveSecret123456`

	got := apploop.SanitizeProviderRetryMessage(errors.New(message))
	for _, secret := range []string{
		"runtime-secret-token",
		"plain-secret-token",
		"response-secret-token",
		"sk-liveSecret123456",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized retry message leaked %q: %s", secret, got)
		}
	}
	if got != "The turn failed before a verified response was available." {
		t.Fatalf("expected fixed host message, got %q", got)
	}
}
