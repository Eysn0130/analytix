package turnterminal

import (
	"context"
	"errors"

	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	turnterminalstoreport "analytix.local/runtime-go/internal/ports/turnterminalstore"
)

// VerifyTrustedInventoryV1 anchors every self-contained terminal signature to
// the installation authority. Store-level parsing proves structural integrity
// and cross-leaf intent membership; neither alone establishes installation
// trust.
func VerifyTrustedInventoryV1(ctx context.Context, store turnterminalstoreport.Store, authority finalauthorityport.Authority) error {
	if store == nil || authority == nil {
		return errors.New("turn terminal trusted inventory authority is unavailable")
	}
	if err := store.VisitIntents(ctx, func(intent domainturnterminal.TurnTerminalIntentV1) error {
		return verifyTrustedIntentV1(ctx, intent, authority)
	}); err != nil {
		return errors.Join(errors.New("turn terminal intent inventory is untrusted"), err)
	}
	if err := store.VisitDispositions(ctx, func(disposition domainturnterminal.TurnTerminalDispositionV1) error {
		return verifyTrustedDispositionV1(ctx, disposition, authority)
	}); err != nil {
		return errors.Join(errors.New("turn terminal disposition inventory is untrusted"), err)
	}
	return nil
}

// VerifyTrustedSnapshotV1 validates the complete in-memory graph, including
// unique context addresses and exact intent membership, before anchoring each
// signature to the current installation. It grants no terminal capability.
func VerifyTrustedSnapshotV1(ctx context.Context, intents []domainturnterminal.TurnTerminalIntentV1, dispositions []domainturnterminal.TurnTerminalDispositionV1, authority finalauthorityport.Authority) error {
	if ctx == nil || authority == nil {
		return errors.New("turn terminal trusted snapshot authority is unavailable")
	}
	byContext := make(map[string]domainturnterminal.TurnTerminalIntentV1, len(intents))
	for _, intent := range intents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyTrustedIntentV1(ctx, intent, authority); err != nil {
			return err
		}
		id := intent.SecurityContext.ContextDigest
		if _, found := byContext[id]; found {
			return errors.New("turn terminal trusted snapshot contains duplicate intents")
		}
		byContext[id] = intent
	}
	seen := make(map[string]bool, len(dispositions))
	for _, disposition := range dispositions {
		if err := ctx.Err(); err != nil {
			return err
		}
		intent, found := byContext[disposition.ContextDigest]
		if !found || seen[disposition.ContextDigest] || domainturnterminal.ValidateTurnTerminalDispositionForIntentV1(disposition, intent) != nil {
			return errors.New("turn terminal trusted snapshot lost exact intent membership")
		}
		if err := verifyTrustedDispositionV1(ctx, disposition, authority); err != nil {
			return err
		}
		seen[disposition.ContextDigest] = true
	}
	return ctx.Err()
}

// VerifyTrustedRecordSignaturesV1 authenticates original physical records
// without claiming their graph is complete. Callers interpreting a signed
// interrupted transaction must separately verify the complete projected graph.
func VerifyTrustedRecordSignaturesV1(ctx context.Context, intents []domainturnterminal.TurnTerminalIntentV1, dispositions []domainturnterminal.TurnTerminalDispositionV1, authority finalauthorityport.Authority) error {
	if ctx == nil || authority == nil {
		return errors.New("turn terminal record verification authority is unavailable")
	}
	for _, intent := range intents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyTrustedIntentV1(ctx, intent, authority); err != nil {
			return err
		}
	}
	for _, disposition := range dispositions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyTrustedDispositionV1(ctx, disposition, authority); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func verifyTrustedIntentV1(ctx context.Context, intent domainturnterminal.TurnTerminalIntentV1, authority finalauthorityport.Authority) error {
	keyID, publicKey, signature, err := domainturnterminal.TurnTerminalIntentV1AuthorityMaterial(intent)
	if err != nil {
		return err
	}
	return authority.VerifyTrusted(ctx, keyID, publicKey, domainturnterminal.TurnTerminalIntentV1SigningBytes(intent), signature)
}

func verifyTrustedDispositionV1(ctx context.Context, disposition domainturnterminal.TurnTerminalDispositionV1, authority finalauthorityport.Authority) error {
	keyID, publicKey, signature, err := domainturnterminal.TurnTerminalDispositionV1AuthorityMaterial(disposition)
	if err != nil {
		return err
	}
	return authority.VerifyTrusted(ctx, keyID, publicKey, domainturnterminal.TurnTerminalDispositionV1SigningBytes(disposition), signature)
}
