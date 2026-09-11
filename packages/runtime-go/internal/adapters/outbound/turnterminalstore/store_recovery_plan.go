package turnterminalstore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

type PreparedRecoveryV1 struct {
	owner     *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1
	validated bool
}

func PrepareRecoveryV1(ctx context.Context, root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("turn terminal prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("turn terminal prepared recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx, absolute, []finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
			{Name: "intents", MaxBytes: maxRecordBytes},
			{Name: "dispositions", MaxBytes: maxRecordBytes},
		}, access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{owner: owner}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("turn terminal recovery plan is invalid")
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

func validatePreparedRecoveryOwnerV1(ctx context.Context, owner *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1) error {
	_, _, err := preparedRecoveryInventoryV1(ctx, owner)
	return err
}

// SnapshotInventory reads the complete canonical committed graph without
// opening a writable store or recovering residues. Installation trust is
// separately verified by the app owner.
func (prepared *PreparedRecoveryV1) SnapshotInventory(ctx context.Context) ([]domainturnterminal.TurnTerminalIntentV1, []domainturnterminal.TurnTerminalDispositionV1, error) {
	if prepared == nil || prepared.owner == nil || ctx == nil {
		return nil, nil, errors.New("turn terminal prepared snapshot is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	intents, dispositions, err := preparedRecoveryInventoryV1(ctx, prepared.owner)
	if err != nil {
		return nil, nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	return intents, dispositions, nil
}

func preparedRecoveryInventoryV1(ctx context.Context, owner *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1) ([]domainturnterminal.TurnTerminalIntentV1, []domainturnterminal.TurnTerminalDispositionV1, error) {
	intents, dispositions, err := readCanonicalPreparedRecoveryRecordsV1(ctx, owner)
	if err != nil {
		return nil, nil, err
	}
	byContext := make(map[string]domainturnterminal.TurnTerminalIntentV1, len(intents))
	for _, intent := range intents {
		byContext[intent.SecurityContext.ContextDigest] = intent
	}
	for _, disposition := range dispositions {
		intent, found := byContext[disposition.ContextDigest]
		if !found || domainturnterminal.ValidateTurnTerminalDispositionForIntentV1(disposition, intent) != nil {
			return nil, nil, errors.New("turn terminal prepared disposition lost its exact intent authority")
		}
	}
	return intents, dispositions, nil
}

// SnapshotCanonicalRecordsV1 returns original physical records only. Each
// record is canonical and correctly addressed, but this method deliberately
// makes no cross-leaf graph or installation-trust claim. A startup observer
// must independently authenticate any journal that explains a partial graph,
// verify every signature, and validate the complete projected graph.
func (prepared *PreparedRecoveryV1) SnapshotCanonicalRecordsV1(ctx context.Context) ([]domainturnterminal.TurnTerminalIntentV1, []domainturnterminal.TurnTerminalDispositionV1, error) {
	if prepared == nil || prepared.owner == nil || ctx == nil {
		return nil, nil, errors.New("turn terminal canonical observation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	intents, dispositions, err := readCanonicalPreparedRecoveryRecordsV1(ctx, prepared.owner)
	if err != nil {
		return nil, nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	return intents, dispositions, nil
}

func readCanonicalPreparedRecoveryRecordsV1(
	ctx context.Context,
	owner *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1,
) ([]domainturnterminal.TurnTerminalIntentV1, []domainturnterminal.TurnTerminalDispositionV1, error) {
	if owner == nil {
		return nil, nil, errors.New("turn terminal prepared owner is unavailable")
	}
	if !owner.Present() {
		return nil, nil, nil
	}
	orderedIntents := []domainturnterminal.TurnTerminalIntentV1{}
	orderedDispositions := []domainturnterminal.TurnTerminalDispositionV1{}
	intents := make(map[string]domainturnterminal.TurnTerminalIntentV1)
	if err := owner.VisitCommittedFiles(ctx, "intents", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		intent, err := domainturnterminal.ParseTurnTerminalIntentV1(file.Body)
		if err != nil || intent.SecurityContext.ContextDigest != file.Digest {
			return errors.Join(errors.New("turn terminal prepared intent is non-canonical or content-address corrupt"), err)
		}
		if _, duplicate := intents[file.Digest]; duplicate {
			return errors.New("turn terminal prepared intent inventory contains a duplicate context")
		}
		intents[file.Digest] = intent
		orderedIntents = append(orderedIntents, intent)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	dispositions := make(map[string]struct{})
	if err := owner.VisitCommittedFiles(ctx, "dispositions", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		disposition, err := domainturnterminal.ParseTurnTerminalDispositionV1(file.Body)
		if err != nil || disposition.ContextDigest != file.Digest {
			return errors.Join(errors.New("turn terminal prepared disposition is non-canonical or content-address corrupt"), err)
		}
		if _, duplicate := dispositions[file.Digest]; duplicate {
			return errors.New("turn terminal prepared disposition inventory contains a duplicate context")
		}
		dispositions[file.Digest] = struct{}{}
		orderedDispositions = append(orderedDispositions, disposition)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return orderedIntents, orderedDispositions, nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("turn terminal recovery plan is invalid")
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
