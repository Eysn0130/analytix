package cachetelemetrystore

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
)

type PreparedRecoveryV1 struct {
	owner     *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1
	validated bool
}

type RecordSnapshotV1 struct {
	Intents     []domaincachetelemetry.ProviderAttemptIntentV1
	Settlements []domaincachetelemetry.ProviderAttemptSettlementV1
	Closures    []domaincachetelemetry.ProviderTurnClosureV1
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("provider cache telemetry prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("provider cache telemetry prepared recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx, absolute, []finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
			{Name: "attempts", MaxBytes: maxRecordBytes},
			{Name: "settlements", MaxBytes: maxRecordBytes},
			{Name: "turn-closures", MaxBytes: maxRecordBytes},
		}, access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{owner: owner}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("provider cache telemetry recovery plan is invalid")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := validatePreparedRecoveryOwnerV1(ctx, prepared.owner); err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

func validatePreparedRecoveryOwnerV1(
	ctx context.Context,
	owner *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1,
) error {
	snapshot, err := readCanonicalPreparedRecoveryRecordsV1(ctx, owner)
	if err != nil {
		return err
	}
	return ValidateProviderTelemetryInventoryV1(snapshot)
}

// SnapshotCanonicalRecordsV1 reads the complete physical three-leaf inventory
// without treating a signed interrupted graph as a complete live ledger.
func (prepared *PreparedRecoveryV1) SnapshotCanonicalRecordsV1(ctx context.Context) (_ RecordSnapshotV1, resultErr error) {
	if err := prepared.Revalidate(ctx); err != nil {
		return RecordSnapshotV1{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, prepared.Revalidate(ctx)) }()
	return readCanonicalPreparedRecoveryRecordsV1(ctx, prepared.owner)
}

func (prepared *PreparedRecoveryV1) SnapshotInventory(ctx context.Context) (_ RecordSnapshotV1, resultErr error) {
	snapshot, err := prepared.SnapshotCanonicalRecordsV1(ctx)
	if err != nil {
		return RecordSnapshotV1{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, prepared.Revalidate(ctx)) }()
	if err := ValidateProviderTelemetryInventoryV1(snapshot); err != nil {
		return RecordSnapshotV1{}, err
	}
	return snapshot, nil
}

// Reuse the live owner's exact complete graph validator, including ordinals,
// retries, timestamps, closure set/aggregate digests and the three-leaf union.
// Current installation key verification is a separate caller responsibility.
func ValidateProviderTelemetryInventoryV1(snapshot RecordSnapshotV1) error {
	attempts, settlements, closures := []finalauthorityadapter.SecurePrivateCASFile{}, []finalauthorityadapter.SecurePrivateCASFile{}, []finalauthorityadapter.SecurePrivateCASFile{}
	for _, intent := range snapshot.Intents {
		body, err := domaincachetelemetry.ProviderAttemptIntentV1Bytes(intent)
		if err != nil {
			return err
		}
		attempts = append(attempts, finalauthorityadapter.SecurePrivateCASFile{Digest: intent.IntentID, Body: body})
	}
	for _, settlement := range snapshot.Settlements {
		body, err := domaincachetelemetry.ProviderAttemptSettlementV1Bytes(settlement)
		if err != nil {
			return err
		}
		settlements = append(settlements, finalauthorityadapter.SecurePrivateCASFile{Digest: settlement.IntentID, Body: body})
	}
	for _, closure := range snapshot.Closures {
		body, err := domaincachetelemetry.ProviderTurnClosureV1Bytes(closure)
		if err != nil {
			return err
		}
		closures = append(closures, finalauthorityadapter.SecurePrivateCASFile{Digest: closure.TurnBindingHMAC, Body: body})
	}
	_, err := validateInventoryFilesV1(attempts, settlements, closures)
	return err
}

func readCanonicalPreparedRecoveryRecordsV1(ctx context.Context, owner *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1) (RecordSnapshotV1, error) {
	if owner == nil || !owner.Present() {
		return RecordSnapshotV1{}, nil
	}
	inventory := inventoryV1{
		intents:     make(map[string]domaincachetelemetry.ProviderAttemptIntentV1),
		settlements: make(map[string]domaincachetelemetry.ProviderAttemptSettlementV1),
		closures:    make(map[string]domaincachetelemetry.ProviderTurnClosureV1),
		turnIntents: make(map[string][]domaincachetelemetry.ProviderAttemptIntentV1),
		turnSettles: make(map[string][]domaincachetelemetry.ProviderAttemptSettlementV1),
	}
	if err := owner.VisitCommittedFiles(ctx, "attempts", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		intent, err := domaincachetelemetry.ParseProviderAttemptIntentV1(file.Body)
		canonical, canonicalErr := domaincachetelemetry.ProviderAttemptIntentV1Bytes(intent)
		if err != nil || canonicalErr != nil || intent.IntentID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("provider cache telemetry intent is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		if _, duplicate := inventory.intents[intent.IntentID]; duplicate {
			return errors.New("provider cache telemetry inventory contains a duplicate intent")
		}
		inventory.intents[intent.IntentID] = intent
		inventory.turnIntents[intent.TurnBindingHMAC] = append(inventory.turnIntents[intent.TurnBindingHMAC], intent)
		return nil
	}); err != nil {
		return RecordSnapshotV1{}, err
	}
	if err := owner.VisitCommittedFiles(ctx, "settlements", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		settlement, err := domaincachetelemetry.ParseProviderAttemptSettlementV1(file.Body)
		canonical, canonicalErr := domaincachetelemetry.ProviderAttemptSettlementV1Bytes(settlement)
		if err != nil || canonicalErr != nil || settlement.IntentID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("provider cache telemetry settlement is noncanonical or misaddressed"), err, canonicalErr)
		}
		if _, duplicate := inventory.settlements[settlement.IntentID]; duplicate {
			return errors.New("provider cache telemetry inventory contains a duplicate settlement")
		}
		inventory.settlements[settlement.IntentID] = settlement
		inventory.turnSettles[settlement.TurnBindingHMAC] = append(inventory.turnSettles[settlement.TurnBindingHMAC], settlement)
		return nil
	}); err != nil {
		return RecordSnapshotV1{}, err
	}
	if err := owner.VisitCommittedFiles(ctx, "turn-closures", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		closure, err := domaincachetelemetry.ParseProviderTurnClosureV1(file.Body)
		canonical, canonicalErr := domaincachetelemetry.ProviderTurnClosureV1Bytes(closure)
		if err != nil || canonicalErr != nil || closure.TurnBindingHMAC != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("provider cache telemetry closure is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		if _, duplicate := inventory.closures[closure.TurnBindingHMAC]; duplicate {
			return errors.New("provider cache telemetry inventory contains a duplicate turn closure")
		}
		inventory.closures[closure.TurnBindingHMAC] = closure
		return nil
	}); err != nil {
		return RecordSnapshotV1{}, err
	}
	snapshot := RecordSnapshotV1{}
	for _, intent := range inventory.intents {
		snapshot.Intents = append(snapshot.Intents, intent)
	}
	for _, settlement := range inventory.settlements {
		snapshot.Settlements = append(snapshot.Settlements, settlement)
	}
	for _, closure := range inventory.closures {
		snapshot.Closures = append(snapshot.Closures, closure)
	}
	return snapshot, nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("provider cache telemetry recovery plan is invalid")
	}
	return prepared.owner.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3{prepared.owner}
}

func (prepared *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return prepared.owner.SecurePrivateCASRecoveryPlansV2()
}
