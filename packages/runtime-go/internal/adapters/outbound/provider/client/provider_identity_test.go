package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type identityRecorderStub struct {
	productionTelemetryRecorderStub
}

func (*identityRecorderStub) DeriveStableProviderUserIDV1(context.Context, domainsecurity.TurnSecurityContext, string) (string, error) {
	return "ax1_" + strings.Repeat("a", 64), nil
}

func TestProductionUserIdentityRequiresExplicitCoreConsentAndOfficialEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name, base, format, protocol string
		consent, want, revoke        bool
	}{
		{"default off", "https://api.deepseek.com", "chat_completions", "deepseek-chat-completions", false, false, false},
		{"chat consent", "https://api.deepseek.com", "chat_completions", "deepseek-chat-completions", true, true, false},
		{"messages consent", "https://api.deepseek.com/anthropic", "messages", "deepseek-messages", true, true, false},
		{"gateway denied", "https://gateway.invalid", "chat_completions", "deepseek-chat-completions", true, false, false},
		{"model name insufficient", "https://api.deepseek.com", "chat_completions", "none", true, false, false},
		{"consent revoked before send", "https://api.deepseek.com", "chat_completions", "deepseek-chat-completions", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sends := 0
			client, err := NewProductionHTTPProviderClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				sends++
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Fatal("invalid wire body")
				}
				id := body["user_id"]
				if tc.format == "messages" {
					if metadata, ok := body["metadata"].(map[string]any); ok {
						id = metadata["user_id"]
					}
					if body["user_id"] != nil {
						t.Fatal("wrong Messages identity location")
					}
				}
				if (id != nil) != tc.want {
					t.Fatal("identity egress did not match explicit consent")
				}
				if tc.want && id != "ax1_"+strings.Repeat("a", 64) {
					t.Fatal("identity changed on wire")
				}
				// This transport never opens a socket. HTTP error still exercises
				// real construction, audit, ledger and currentness before dispatch.
				return &http.Response{StatusCode: 401, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"synthetic"}`))}, nil
			})}, &identityRecorderStub{})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			checks := 0
			if tc.consent {
				client.StableUserIDConsent = func(context.Context, domainsecurity.TurnSecurityContext, string, string) bool {
					checks++
					return !tc.revoke || checks == 1
				}
			}
			request := productionTelemetryRequestV1(t, tc.base)
			request.EndpointFormat, request.ReasoningProtocol = tc.format, tc.protocol
			_, err = client.Stream(context.Background(), request)
			if err == nil {
				t.Fatal("synthetic rejected request was accepted")
			}
			if tc.revoke {
				if sends != 0 {
					t.Fatal("revoked consent reached transport")
				}
			} else if sends != 1 {
				t.Fatal("request did not reach production transport")
			}
		})
	}
}
