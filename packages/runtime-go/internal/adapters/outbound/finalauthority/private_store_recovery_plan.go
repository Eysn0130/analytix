package finalauthority

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

// PreparedPrivateStoreRecoveryV1 is the accepted-final owner's exact recovery
// plan. Semantic validation is complete before the plan can enter the global
// revalidation barrier.
type PreparedPrivateStoreRecoveryV1 struct {
	owner     *PreparedSecurePrivateCASOwnerRecoveryV1
	validated bool
}

func PreparePrivateStoreRecoveryV1(
	ctx context.Context,
	root string,
	access SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedPrivateStoreRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("private accepted final prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("private accepted final prepared recovery root is invalid")
	}
	owner, err := PrepareSecurePrivateCASOwnerRecoveryV1(ctx, absolute, []SecurePrivateCASOwnerLeafV1{
		{Name: "records", MaxBytes: maxPrivateAcceptedFinalBytes},
		{Name: "dispositions", MaxBytes: maxPrivateAcceptedFinalBytes},
	}, access)
	if err != nil {
		return nil, err
	}
	return &PreparedPrivateStoreRecoveryV1{owner: owner}, nil
}

func (prepared *PreparedPrivateStoreRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("private accepted final recovery plan is invalid")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := validatePreparedPrivateStoreRecoveryV1(ctx, prepared.owner); err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

func validatePreparedPrivateStoreRecoveryV1(
	ctx context.Context,
	owner *PreparedSecurePrivateCASOwnerRecoveryV1,
) error {
	records, dispositions, err := readCanonicalPrivateStoreRecoveryRecordsV1(ctx, owner)
	if err != nil {
		return err
	}
	return ValidatePrivateStoreInventoryV1(records, dispositions)
}

// SnapshotCanonicalRecordsV1 observes all physical records with the same
// prepared authority before and after the read. It makes no complete graph
// or current-installation trust claim for an interrupted signed transaction.
func (prepared *PreparedPrivateStoreRecoveryV1) SnapshotCanonicalRecordsV1(ctx context.Context) (_ []domainevidence.PrivateAcceptedFinalRecord, _ []domainevidence.AcceptedFinalDispositionRecord, resultErr error) {
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, prepared.Revalidate(ctx)) }()
	return readCanonicalPrivateStoreRecoveryRecordsV1(ctx, prepared.owner)
}

func (prepared *PreparedPrivateStoreRecoveryV1) SnapshotInventory(ctx context.Context) ([]domainevidence.PrivateAcceptedFinalRecord, []domainevidence.AcceptedFinalDispositionRecord, error) {
	records, dispositions, err := prepared.SnapshotCanonicalRecordsV1(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := ValidatePrivateStoreInventoryV1(records, dispositions); err != nil {
		return nil, nil, err
	}
	return records, dispositions, nil
}

func readCanonicalPrivateStoreRecoveryRecordsV1(ctx context.Context, owner *PreparedSecurePrivateCASOwnerRecoveryV1) ([]domainevidence.PrivateAcceptedFinalRecord, []domainevidence.AcceptedFinalDispositionRecord, error) {
	if owner == nil || !owner.Present() {
		return nil, nil, nil
	}
	records := []domainevidence.PrivateAcceptedFinalRecord{}
	dispositions := []domainevidence.AcceptedFinalDispositionRecord{}
	seenRecords, seenDispositions := map[string]bool{}, map[string]bool{}
	if err := owner.VisitCommittedFiles(ctx, "records", func(file SecurePrivateCASFile) error {
		record, err := domainevidence.ParsePrivateAcceptedFinalRecord(file.Body)
		canonical, canonicalErr := domainevidence.PrivateAcceptedFinalRecordBytes(record)
		if err != nil || canonicalErr != nil || record.AcceptedFinal.RecordDigest != file.Digest ||
			!bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("private accepted final recovery record is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		if seenRecords[file.Digest] {
			return errors.New("private accepted final recovery contains a duplicate record")
		}
		seenRecords[file.Digest] = true
		records = append(records, record)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	if err := owner.VisitCommittedFiles(ctx, "dispositions", func(file SecurePrivateCASFile) error {
		disposition, err := domainevidence.ParseAcceptedFinalDispositionRecord(file.Body)
		canonical, canonicalErr := domainevidence.AcceptedFinalDispositionRecordBytes(disposition)
		if err != nil || canonicalErr != nil || disposition.AcceptedFinalDigest != file.Digest ||
			!bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("accepted final disposition recovery record is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		if seenDispositions[file.Digest] {
			return errors.New("accepted final recovery contains a duplicate disposition")
		}
		seenDispositions[file.Digest] = true
		dispositions = append(dispositions, disposition)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return records, dispositions, nil
}

// ValidatePrivateStoreInventoryV1 preserves the owner's complete canonical
// domain graph rules for a typed observation or pure candidate. Installation
// key trust remains the caller's separate responsibility.
func ValidatePrivateStoreInventoryV1(records []domainevidence.PrivateAcceptedFinalRecord, dispositions []domainevidence.AcceptedFinalDispositionRecord) error {
	byID := make(map[string]domainevidence.PrivateAcceptedFinalRecord, len(records))
	for _, record := range records {
		if err := domainevidence.ValidatePrivateAcceptedFinalRecord(record); err != nil {
			return err
		}
		id := record.AcceptedFinal.RecordDigest
		if _, exists := byID[id]; exists {
			return errors.New("private accepted final inventory contains duplicate records")
		}
		byID[id] = record
	}
	seen := map[string]bool{}
	for _, disposition := range dispositions {
		if err := domainevidence.ValidateAcceptedFinalDispositionRecord(disposition); err != nil {
			return err
		}
		if seen[disposition.AcceptedFinalDigest] {
			return errors.New("private accepted final inventory contains duplicate dispositions")
		}
		seen[disposition.AcceptedFinalDigest] = true
		record, ok := byID[disposition.AcceptedFinalDigest]
		if !ok || disposition.PrivateRecordDigest != record.AcceptedFinal.PrivateRecordDigest ||
			disposition.ThreadID != record.AcceptedFinal.ThreadID || disposition.TurnID != record.AcceptedFinal.TurnID ||
			disposition.AuthorityKeyID != record.AcceptedFinal.AuthorityKeyID ||
			disposition.AuthorityPublicKey != record.AcceptedFinal.AuthorityPublicKey {
			return errors.New("accepted final disposition recovery lost its exact private accepted-final authority")
		}
		if disposition.SchemaVersion == domainevidence.AcceptedFinalDispositionRecordV2 &&
			disposition.WinnerDigest != disposition.AcceptedFinalDigest {
			winner, found := byID[disposition.WinnerDigest]
			if !found || winner.AcceptedFinal.ThreadID != disposition.ThreadID || winner.AcceptedFinal.TurnID != disposition.TurnID {
				return errors.New("accepted final disposition recovery lost its different public winner authority")
			}
		}
	}
	return nil
}

func (prepared *PreparedPrivateStoreRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("private accepted final recovery plan is invalid")
	}
	return prepared.owner.Revalidate(ctx)
}

func (prepared *PreparedPrivateStoreRecoveryV1) PrivateCASRecoveryTopologiesV3() []SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return []SecurePrivateCASRecoveryTopologyAuthorityV3{prepared.owner}
}

func (prepared *PreparedPrivateStoreRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return prepared.owner.SecurePrivateCASRecoveryPlansV2()
}
