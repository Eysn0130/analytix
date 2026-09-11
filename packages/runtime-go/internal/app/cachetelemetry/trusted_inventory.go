package cachetelemetry

import (
	"context"
	"errors"
	"fmt"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	cachetelemetrystoreport "analytix.local/runtime-go/internal/ports/cachetelemetrystore"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// VerifyTrustedInventoryV1 upgrades structural self-consistency to installation
// authority membership. Embedded public keys are never accepted as authority
// until every signature is verified by the current final authority.
func VerifyTrustedInventoryV1(
	ctx context.Context,
	store cachetelemetrystoreport.Store,
	authority finalauthorityport.Authority,
) error {
	if ctx == nil || store == nil || authority == nil {
		return errors.New("provider cache telemetry trusted inventory dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := store.VisitInventory(ctx, func(intent domaincachetelemetry.ProviderAttemptIntentV1) error {
		keyID, publicKey, signature, err := domaincachetelemetry.ProviderAttemptIntentV1AuthorityMaterial(intent)
		if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domaincachetelemetry.ProviderAttemptIntentV1SigningBytes(intent), signature) != nil {
			return errors.New("provider cache telemetry intent lacks trusted installation authority")
		}
		return nil
	}, func(settlement domaincachetelemetry.ProviderAttemptSettlementV1) error {
		keyID, publicKey, signature, err := domaincachetelemetry.ProviderAttemptSettlementV1AuthorityMaterial(settlement)
		if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domaincachetelemetry.ProviderAttemptSettlementV1SigningBytes(settlement), signature) != nil {
			return errors.New("provider cache telemetry settlement lacks trusted installation authority")
		}
		return nil
	}, func(closure domaincachetelemetry.ProviderTurnClosureV1) error {
		keyID, publicKey, signature, err := domaincachetelemetry.ProviderTurnClosureV1AuthorityMaterial(closure)
		if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domaincachetelemetry.ProviderTurnClosureV1SigningBytes(closure), signature) != nil {
			return errors.New("provider cache telemetry closure lacks trusted installation authority")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("verify provider cache telemetry inventory: %w", err)
	}
	return nil
}

// VerifyTrustedRecordSignaturesV1 verifies every physical or candidate record
// without claiming that an interrupted three-leaf ledger is a complete graph.
// Preserve key, root, I/O and cancellation errors from the authority.
func VerifyTrustedRecordSignaturesV1(ctx context.Context, intents []domaincachetelemetry.ProviderAttemptIntentV1, settlements []domaincachetelemetry.ProviderAttemptSettlementV1, closures []domaincachetelemetry.ProviderTurnClosureV1, authority finalauthorityport.Authority) error {
	if ctx == nil || authority == nil {
		return errors.New("provider telemetry record verification authority is unavailable")
	}
	for _, intent := range intents {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, publicKey, signature, err := domaincachetelemetry.ProviderAttemptIntentV1AuthorityMaterial(intent)
		if err != nil {
			return err
		}
		if err := authority.VerifyTrusted(ctx, key, publicKey, domaincachetelemetry.ProviderAttemptIntentV1SigningBytes(intent), signature); err != nil {
			return err
		}
	}
	for _, settlement := range settlements {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, publicKey, signature, err := domaincachetelemetry.ProviderAttemptSettlementV1AuthorityMaterial(settlement)
		if err != nil {
			return err
		}
		if err := authority.VerifyTrusted(ctx, key, publicKey, domaincachetelemetry.ProviderAttemptSettlementV1SigningBytes(settlement), signature); err != nil {
			return err
		}
	}
	for _, closure := range closures {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, publicKey, signature, err := domaincachetelemetry.ProviderTurnClosureV1AuthorityMaterial(closure)
		if err != nil {
			return err
		}
		if err := authority.VerifyTrusted(ctx, key, publicKey, domaincachetelemetry.ProviderTurnClosureV1SigningBytes(closure), signature); err != nil {
			return err
		}
	}
	return ctx.Err()
}
