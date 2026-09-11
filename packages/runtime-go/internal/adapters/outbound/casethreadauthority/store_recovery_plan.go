package casethreadauthority

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type PreparedRecoveryV1 struct {
	cas       *finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1
	validated bool
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("case thread authority prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("case thread authority prepared recovery root is invalid")
	}
	plan, err := finalauthorityadapter.PrepareSecurePrivateCASRecoveryIfPresent(
		ctx, absolute, maxCaseThreadAuthorityBytes, access,
	)
	if err != nil {
		return nil, err
	}
	if err := plan.Revalidate(ctx); err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{cas: plan}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.cas == nil {
		return errors.New("case thread authority recovery plan is invalid")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := validatePreparedRecoveryFilesV1(ctx, prepared.cas); err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

func validatePreparedRecoveryFilesV1(
	ctx context.Context,
	plan *finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1,
) error {
	_, err := preparedCaseThreadInventoryV1(ctx, plan, true)
	return err
}

// SnapshotInventory observes a complete, canonical original graph without
// opening a live store, creating an empty one, or performing CAS recovery.
// The application owner must still verify every installation signature.
func (prepared *PreparedRecoveryV1) SnapshotInventory(ctx context.Context) ([]domainsecurity.CaseThreadAuthorityRecord, error) {
	return prepared.snapshotV1(ctx, true)
}

// SnapshotCanonicalRecordsV1 permits a caller to account for an interrupted
// signed transaction. It does not claim that the physical graph is complete.
func (prepared *PreparedRecoveryV1) SnapshotCanonicalRecordsV1(ctx context.Context) ([]domainsecurity.CaseThreadAuthorityRecord, error) {
	return prepared.snapshotV1(ctx, false)
}

func (prepared *PreparedRecoveryV1) snapshotV1(ctx context.Context, complete bool) (_ []domainsecurity.CaseThreadAuthorityRecord, resultErr error) {
	if prepared == nil || prepared.cas == nil || ctx == nil {
		return nil, errors.New("case thread prepared snapshot is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, prepared.Revalidate(ctx)) }()
	return preparedCaseThreadInventoryV1(ctx, prepared.cas, complete)
}

func preparedCaseThreadInventoryV1(ctx context.Context, plan *finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1, complete bool) ([]domainsecurity.CaseThreadAuthorityRecord, error) {
	records := []domainsecurity.CaseThreadAuthorityRecord{}
	if err := plan.VisitCommittedFiles(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		record, err := domainsecurity.ParseCaseThreadAuthorityRecord(file.Body)
		canonical, canonicalErr := domainsecurity.CaseThreadAuthorityRecordBytes(record)
		if err != nil || canonicalErr != nil || record.RecordDigest != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("case thread authority prepared recovery record is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		records = append(records, record)
		return nil
	}); err != nil {
		return nil, err
	}
	if complete {
		if err := domainsecurity.ValidateCaseThreadAuthorityInventoryV1(records); err != nil {
			return nil, err
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].RecordDigest < records[j].RecordDigest })
	return records, nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.cas == nil {
		return errors.New("case thread authority recovery plan is invalid")
	}
	return prepared.cas.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	// The case-thread authority CAS is itself the production root and has no
	// intermediate owner directory. Leaf mutation still binds its exact root
	// identity through the recovery plan.
	return nil
}

func (prepared *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.cas == nil {
		return nil
	}
	return []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1{prepared.cas}
}
