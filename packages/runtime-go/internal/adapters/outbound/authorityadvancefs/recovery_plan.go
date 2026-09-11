package authorityadvancefs

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
)

// PreparedRecoveryV1 validates both journal leaves as one immutable owner
// before the shared private-CAS recovery transaction may mutate residue.
type PreparedRecoveryV1 struct {
	owner     *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1
	validated bool
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("authority advance prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("authority advance prepared recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx,
		filepath.Join(absolute, "v2"),
		[]finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
			{Name: "intents", MaxBytes: domainauthority.MaxMonotonicAdvanceJournalRecordBytesV2},
			{Name: "settlements", MaxBytes: domainauthority.MaxMonotonicAdvanceJournalRecordBytesV2},
		},
		access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{owner: owner}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("authority advance recovery plan is invalid")
	}
	if _, err := preparedRecoveryInventoryV2(ctx, prepared.owner); err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

// PreparedInventoryV2 contains the complete committed journal, with no write,
// recovery, or witness-freshness authority. Visitors receive newly parsed values.
type PreparedInventoryV2 struct {
	intents     [][]byte
	settlements [][]byte
}

func (snapshot *PreparedInventoryV2) HasRecords() bool {
	return snapshot != nil && (len(snapshot.intents) != 0 || len(snapshot.settlements) != 0)
}

func (snapshot *PreparedInventoryV2) VisitIntents(ctx context.Context, visit func(domainauthority.MonotonicAdvanceIntentV2) error) error {
	if snapshot == nil || ctx == nil || visit == nil {
		return errors.New("authority advance prepared intent inventory is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, body := range snapshot.intents {
		if err := ctx.Err(); err != nil {
			return err
		}
		intent, err := domainauthority.ParseMonotonicAdvanceIntentV2(body)
		if err != nil {
			return err
		}
		if err := visit(intent); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (snapshot *PreparedInventoryV2) VisitSettlements(ctx context.Context, visit func(domainauthority.MonotonicAdvanceSettlementV2) error) error {
	if snapshot == nil || ctx == nil || visit == nil {
		return errors.New("authority advance prepared settlement inventory is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, body := range snapshot.settlements {
		if err := ctx.Err(); err != nil {
			return err
		}
		settlement, err := domainauthority.ParseMonotonicAdvanceSettlementV2(body)
		if err != nil {
			return err
		}
		if err := visit(settlement); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (prepared *PreparedRecoveryV1) SnapshotInventory(ctx context.Context) (*PreparedInventoryV2, error) {
	if prepared == nil || prepared.owner == nil || ctx == nil {
		return nil, errors.New("authority advance prepared snapshot is unavailable")
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	snapshot, err := preparedRecoveryInventoryV2(ctx, prepared.owner)
	if err != nil {
		return nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func preparedRecoveryInventoryV2(ctx context.Context, owner *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1) (*PreparedInventoryV2, error) {
	snapshot := &PreparedInventoryV2{}
	intents := map[string]domainauthority.MonotonicAdvanceIntentV2{}
	settlements := map[string]domainauthority.MonotonicAdvanceSettlementV2{}
	if err := owner.VisitCommittedFiles(ctx, "intents", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		intent, err := domainauthority.ParseMonotonicAdvanceIntentV2(file.Body)
		canonical, canonicalErr := domainauthority.MonotonicAdvanceIntentV2Bytes(intent)
		if err != nil || canonicalErr != nil || intent.MutationID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("authority advance recovery contains a corrupt intent"), err, canonicalErr)
		}
		if _, duplicate := intents[intent.MutationID]; duplicate {
			return errors.New("authority advance recovery contains a duplicate intent")
		}
		snapshot.intents = append(snapshot.intents, append([]byte(nil), file.Body...))
		intents[intent.MutationID] = intent
		return nil
	}); err != nil {
		return nil, err
	}
	if err := owner.VisitCommittedFiles(ctx, "settlements", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		settlement, err := domainauthority.ParseMonotonicAdvanceSettlementV2(file.Body)
		canonical, canonicalErr := domainauthority.MonotonicAdvanceSettlementV2Bytes(settlement)
		if err != nil || canonicalErr != nil || settlement.MutationID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("authority advance recovery contains a corrupt settlement"), err, canonicalErr)
		}
		if _, duplicate := settlements[settlement.MutationID]; duplicate {
			return errors.New("authority advance recovery contains a duplicate settlement")
		}
		snapshot.settlements = append(snapshot.settlements, append([]byte(nil), file.Body...))
		settlements[settlement.MutationID] = settlement
		return nil
	}); err != nil {
		return nil, err
	}
	for mutationID, settlement := range settlements {
		intent, found := intents[mutationID]
		if !found || domainauthority.ValidateMonotonicAdvanceSettlementIntentHeaderV2(settlement, intent) != nil {
			return nil, errors.New("authority advance recovery settlement lost its exact intent")
		}
		if settlement.Kind == domainauthority.MonotonicAdvanceSettlementCommittedV2 &&
			domainauthority.ValidateMonotonicAdvanceSettlementForIntentV2(settlement, intent, nil) != nil {
			return nil, errors.New("authority advance recovery committed settlement is invalid")
		}
	}
	return snapshot, nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("authority advance recovery plan is invalid")
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
