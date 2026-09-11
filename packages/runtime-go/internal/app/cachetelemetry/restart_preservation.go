package cachetelemetry

import (
	"context"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var ErrRestartPreserved = errors.New("provider telemetry turn is preserved after restart")

// PreserveRestartContextsV1 installs the startup owner's immutable hold from
// exact frozen Core contexts. It is denial-only: it cannot authorize ordinary
// work or replace a missing context with a partial thread/turn binding. The
// caller must establish and retain the complete trusted dependency scope.
func (service *DurableService) PreserveRestartContextsV1(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) error {
	if service == nil || service.authority == nil || service.store == nil {
		return forbidProviderRetryV1("preserve provider restart contexts", errors.New("durable service is unavailable"))
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.restartPreserved != nil {
		return forbidProviderRetryV1("preserve provider restart contexts", errors.New("restart preservation is already installed"))
	}
	if err := contextErrorV1(ctx); err != nil {
		return forbidProviderRetryV1("preserve provider restart contexts", err)
	}
	bindings := make(map[string]struct{}, len(contexts))
	for _, frozen := range contexts {
		if err := domainsecurity.ValidateTurnSecurityContext(frozen); err != nil || frozen.Version != domainsecurity.TurnSecurityContextVersionV2 {
			return forbidProviderRetryV1("preserve provider restart contexts", errors.New("frozen V2 context is invalid"))
		}
		binding, err := service.turnBindingHMACV1(ctx, frozen)
		if err != nil {
			return forbidProviderRetryV1("preserve provider restart contexts", err)
		}
		bindings[binding] = struct{}{}
	}
	// Preserving a binding never excuses a corrupt or foreign signed record.
	if _, _, _, err := service.fullInventoryV1(ctx); err != nil {
		return forbidProviderRetryV1("preserve provider restart contexts", err)
	}
	if err := contextErrorV1(ctx); err != nil {
		return forbidProviderRetryV1("preserve provider restart contexts", err)
	}
	service.restartPreserved = bindings
	return nil
}

// All callers hold service.mu through their last durable write. Installation
// and every writer therefore share one ordering boundary.
func (service *DurableService) restartPreservesBindingV1(binding string) bool {
	_, preserved := service.restartPreserved[binding]
	return preserved
}
