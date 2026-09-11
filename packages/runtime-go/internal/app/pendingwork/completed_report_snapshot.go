package pendingwork

import (
	"context"
	"errors"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// CompletedReportSnapshotV1 is immutable historical Core authority. It has
// no pending store, close method, or live settlement/release capability.
type CompletedReportSnapshotV1 struct {
	authority    authorityport.Authority
	receipts     map[string]domainpendingwork.PendingWorkReceiptV1
	dispositions map[string]domainpendingwork.PendingWorkDispositionV1
	primaries    *reportRestartPrimaryInventoryV1
}

// PrepareCompletedReportSnapshotV1 verifies the complete Original inventory,
// including every completed Core grant/result, before sealing its strict
// primary reads. Runtime callers must supply the Original denominator and
// bracket the snapshot with their physical/key observation boundary.
func PrepareCompletedReportSnapshotV1(ctx context.Context, receipts []domainpendingwork.PendingWorkReceiptV1, dispositions []domainpendingwork.PendingWorkDispositionV1, authority authorityport.Authority, reader recoveryport.PrimaryThreadReaderV1) (*CompletedReportSnapshotV1, error) {
	if ctx == nil || authority == nil || reader == nil || authority.KeyID() == "" || len(authority.PublicKey()) == 0 {
		return nil, ErrAuthorityUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	primaries := &reportRestartPrimaryInventoryV1{ctx: ctx, reader: reader, digests: map[string]string{}}
	inventory, err := (&Service{authority: authority, grants: primaries}).verifyTrustedSnapshotV1(ctx, receipts, dispositions)
	if err != nil {
		return nil, err
	}
	if err := primaries.sealV1(ctx); err != nil {
		return nil, err
	}
	prepared := &CompletedReportSnapshotV1{
		authority: authority, primaries: primaries,
		receipts: map[string]domainpendingwork.PendingWorkReceiptV1{}, dispositions: inventory.Dispositions,
	}
	for _, receipt := range inventory.Receipts {
		prepared.receipts[receipt.WorkID] = receipt
	}
	return prepared, nil
}

func (prepared *CompletedReportSnapshotV1) ResolveTrustedCompletedReportStageV1(ctx context.Context, workID string) (result TrustedCompletedReportStageV1, resultErr error) {
	empty := TrustedCompletedReportStageV1{}
	if prepared == nil || ctx == nil || !domainsecurity.IsSHA256Hex(workID) {
		return empty, ErrOperationMismatch
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	receipt, found := prepared.receipts[workID]
	if !found || receipt.Kind != domainpendingwork.KindReportStage || len(receipt.GrantMembers) != 1 {
		return empty, ErrOperationMismatch
	}
	disposition, found := prepared.dispositions[workID]
	if !found {
		return empty, ErrWorkAlreadyOpen
	}
	verifier := &Service{authority: prepared.authority}
	if disposition.Status != domainpendingwork.StatusCompleted {
		return empty, ErrGrantAuthority
	}
	verify := func() error {
		return errors.Join(verifier.verifyReceipt(ctx, receipt), verifier.verifyDisposition(ctx, disposition, receipt), ctx.Err())
	}
	if err := verify(); err != nil {
		return empty, errors.Join(ErrGrantAuthority, err)
	}
	defer func() {
		_, err := prepared.primaries.ReadPrimaryThreadSnapshotV1(ctx, receipt.Context.ThreadID)
		resultErr = errors.Join(resultErr, err, verify())
		if resultErr != nil {
			result = empty
		}
	}()
	snapshot, err := prepared.primaries.ReadPrimaryThreadSnapshotV1(ctx, receipt.Context.ThreadID)
	if err != nil {
		return empty, err
	}
	terminal, err := resolveCompletedInventoryDispositionFromThreadV1(receipt, disposition, snapshot.Thread)
	if err != nil || terminal == nil {
		return empty, errors.Join(ErrGrantAuthority, err)
	}
	return *terminal, nil
}
