package authorityadvance

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"

	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	storeport "analytix.local/runtime-go/internal/ports/authorityadvance"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// VerifyTrustedInventoryV2 pins the complete local authority-advance journal
// to one installation authority. It does not infer witness freshness or turn
// an unsettled intent into a committed effect.
func VerifyTrustedInventoryV2(
	ctx context.Context,
	intents storeport.IntentInventoryStore,
	settlements storeport.SettlementInventoryStore,
	authority finalauthorityport.Authority,
) error {
	if ctx == nil || intents == nil || settlements == nil || authority == nil {
		return errors.New("authority advance trusted inventory dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	type lineage struct{ installationID, enrollmentID string }
	var observed lineage
	validateLineage := func(installationID, enrollmentID string) error {
		candidate := lineage{installationID: installationID, enrollmentID: enrollmentID}
		if observed == (lineage{}) {
			observed = candidate
			return nil
		}
		if observed != candidate {
			return errors.New("authority advance journal mixes installation or enrollment lineages")
		}
		return nil
	}
	trusted := func(keyID, encodedKey, encodedSignature string, signingBytes []byte) error {
		publicKey, keyErr := base64.RawURLEncoding.DecodeString(encodedKey)
		signature, signatureErr := base64.RawURLEncoding.DecodeString(encodedSignature)
		if keyErr != nil || signatureErr != nil || keyID != authority.KeyID() ||
			!bytes.Equal(publicKey, authority.PublicKey()) ||
			authority.VerifyTrusted(ctx, keyID, publicKey, signingBytes, signature) != nil {
			return errors.New("authority advance record lacks current installation authority")
		}
		return nil
	}
	intentByMutation := map[string]domainauthority.MonotonicAdvanceIntentV2{}
	if err := intents.VisitIntents(ctx, func(intent domainauthority.MonotonicAdvanceIntentV2) error {
		if _, duplicate := intentByMutation[intent.MutationID]; duplicate ||
			domainauthority.ValidateMonotonicAdvanceIntentV2(intent) != nil ||
			trusted(intent.AuthorityKeyID, intent.AuthorityPublicKey, intent.AuthoritySignature,
				domainauthority.MonotonicAdvanceIntentSigningBytesV2(intent)) != nil {
			return errors.New("authority advance intent inventory is untrusted")
		}
		if err := validateLineage(intent.InstallationID, intent.EnrollmentID); err != nil {
			return err
		}
		intentByMutation[intent.MutationID] = intent
		return nil
	}); err != nil {
		return err
	}
	settlementByMutation := map[string]bool{}
	if err := settlements.VisitSettlements(ctx, func(settlement domainauthority.MonotonicAdvanceSettlementV2) error {
		if settlementByMutation[settlement.MutationID] {
			return errors.New("authority advance settlement inventory repeats a mutation")
		}
		intent, found := intentByMutation[settlement.MutationID]
		if !found || domainauthority.ValidateMonotonicAdvanceSettlementIntentHeaderV2(settlement, intent) != nil ||
			trusted(settlement.AuthorityKeyID, settlement.AuthorityPublicKey, settlement.AuthoritySignature,
				domainauthority.MonotonicAdvanceSettlementSigningBytesV2(settlement)) != nil {
			return errors.New("authority advance settlement inventory is untrusted or orphaned")
		}
		if settlement.Kind == domainauthority.MonotonicAdvanceSettlementCommittedV2 &&
			domainauthority.ValidateMonotonicAdvanceSettlementForIntentV2(settlement, intent, nil) != nil {
			return errors.New("authority advance committed settlement inventory is invalid")
		}
		if err := validateLineage(settlement.InstallationID, settlement.EnrollmentID); err != nil {
			return err
		}
		settlementByMutation[settlement.MutationID] = true
		return nil
	}); err != nil {
		return err
	}
	return nil
}
