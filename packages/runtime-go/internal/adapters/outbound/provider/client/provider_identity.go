package client

import (
	"context"
	"errors"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type stableProviderIdentityDeriver interface {
	DeriveStableProviderUserIDV1(context.Context, domainsecurity.TurnSecurityContext, string) (string, error)
}

// A nil consent policy is the production default. A model name, successful
// HTTP response, user-provided URL, or compatible gateway grants no consent.
// The callback is installed by Core composition, never deserialized settings.
type StableProviderUserIDConsent func(context.Context, domainsecurity.TurnSecurityContext, string, string) bool

func (c *HTTPProviderClient) attachConsentedProviderUserID(ctx context.Context, request *domainmodel.Request, endpoint string, body map[string]any) error {
	if c == nil || c.StableUserIDConsent == nil || request.PrivateProviderTelemetry == nil {
		return nil
	}
	protocol := request.ReasoningProtocol
	chat := endpoint == "https://api.deepseek.com/v1/chat/completions" && protocol == "deepseek-chat-completions"
	messages := endpoint == "https://api.deepseek.com/anthropic/v1/messages" && protocol == "deepseek-messages"
	if !chat && !messages {
		return nil
	}
	scope := request.PrivateProviderTelemetry.SecurityContext
	providerID := request.ProviderID
	consent := c.StableUserIDConsent
	if !consent(ctx, scope, providerID, endpoint) {
		return nil
	}
	deriver, ok := c.telemetryRecorder.(stableProviderIdentityDeriver)
	if !ok {
		return errors.New("consented provider identity authority is unavailable")
	}
	id, err := deriver.DeriveStableProviderUserIDV1(ctx, scope, providerID)
	if err != nil || len(id) != 68 || id[:4] != "ax1_" || !domainsecurity.IsSHA256Hex(id[4:]) {
		return errors.New("provider identity derivation is invalid")
	}
	if chat {
		body["user_id"] = id
	} else {
		body["metadata"] = map[string]any{"user_id": id}
	}
	previous := request.PrivateProviderCurrentnessBeforeSend
	request.PrivateProviderCurrentnessBeforeSend = func(attempt int) error {
		if previous != nil {
			if err := previous(attempt); err != nil {
				return err
			}
		}
		if !consent(ctx, scope, providerID, endpoint) {
			return errors.New("provider identity egress consent is no longer current")
		}
		return nil
	}
	return nil
}
