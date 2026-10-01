package cachetelemetry

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// DeriveStableProviderUserIDV1 only derives a pseudonym. The HTTP owner must
// separately hold explicit, current Core egress consent before attaching it.
// No turn, time, nonce, API key, or model-generated identity participates.
func (service *DurableService) DeriveStableProviderUserIDV1(ctx context.Context, scope domainsecurity.TurnSecurityContext, providerID string) (string, error) {
	if service == nil || service.authority == nil || domainsecurity.ValidateTurnSecurityContextForExecution(scope) != nil || providerID == "" {
		return "", errors.New("provider identity authority is unavailable")
	}
	body, err := json.Marshal(struct {
		Tenant, User, Workspace, Case, Binding, Provider string
	}{scope.TenantID, scope.UserID, scope.WorkspaceRealPath, scope.CaseID, scope.CaseBindingHash, providerID})
	if err != nil {
		return "", errors.New("provider identity scope is invalid")
	}
	key, err := service.authority.Sign(ctx, []byte("analytix.provider-user-id/key/v1"))
	if err != nil || len(key) < sha256.Size {
		return "", errors.New("provider identity derivation is unavailable")
	}
	defer zeroBytesV1(key)
	return "ax1_" + providerTelemetryHMACV1(key, "privacy-domain", body), nil
}
