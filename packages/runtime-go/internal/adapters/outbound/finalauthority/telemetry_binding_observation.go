package finalauthority

import (
	"context"
	"errors"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// ObserveProviderTurnBindingHMACV1 derives only the fixed telemetry turn
// binding from an original V2 context. No general signing method is enabled;
// neither signature, key material, nor a new persisted authority is returned.
func (verification *ExistingFileVerificationV1) ObserveProviderTurnBindingHMACV1(ctx context.Context, frozen domainsecurity.TurnSecurityContext) (string, error) {
	key, err := verification.load(ctx)
	if err != nil {
		return "", err
	}
	binding, err := observeProviderTurnBindingHMACV1(ctx, key, frozen)
	if err := errors.Join(err, verification.Revalidate(ctx)); err != nil {
		return "", err
	}
	return binding, nil
}

func (authority *AnchoredFileAuthority) ObserveProviderTurnBindingHMACV1(ctx context.Context, frozen domainsecurity.TurnSecurityContext) (string, error) {
	if err := authority.ValidateCurrentInstallation(ctx); err != nil {
		return "", err
	}
	key, err := authority.revalidate()
	if err != nil {
		return "", err
	}
	binding, err := observeProviderTurnBindingHMACV1(ctx, key, frozen)
	if err := errors.Join(err, authority.ValidateCurrentInstallation(ctx)); err != nil {
		return "", err
	}
	return binding, nil
}

func observeProviderTurnBindingHMACV1(ctx context.Context, authority *FileAuthority, frozen domainsecurity.TurnSecurityContext) (string, error) {
	if ctx == nil || frozen.Version != domainsecurity.TurnSecurityContextVersionV2 || domainsecurity.ValidateTurnSecurityContext(frozen) != nil {
		return "", errors.New("original telemetry V2 context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	key, err := authority.Sign(ctx, []byte(domaincachetelemetry.ProviderTelemetryKeyDerivationDomainV1))
	if err != nil {
		return "", err
	}
	defer func() {
		for index := range key {
			key[index] = 0
		}
	}()
	return domaincachetelemetry.ProviderTurnBindingHMACV1(key, frozen)
}
