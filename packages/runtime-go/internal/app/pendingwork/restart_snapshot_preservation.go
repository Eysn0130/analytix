package pendingwork

import (
	"context"
	"errors"
	"sort"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// PlanReportRestartPreservationSnapshotV1 verifies complete prepared input
// before any live pending store exists. It derives the same immutable hold as
// the live planner and verifies completed work against strict original primary
// grant/results, without normalization, signing, disposal, or recovery.
func PlanReportRestartPreservationSnapshotV1(ctx context.Context, receipts []domainpendingwork.PendingWorkReceiptV1, dispositions []domainpendingwork.PendingWorkDispositionV1, authority authorityport.Authority, reader recoveryport.PrimaryThreadReaderV1, inherited ...ReportRestartInheritedHistoryV1) (ReportRestartScopeV1, error) {
	if ctx == nil || authority == nil || reader == nil || authority.KeyID() == "" || len(authority.PublicKey()) == 0 {
		return ReportRestartScopeV1{}, ErrAuthorityUnavailable
	}
	if len(inherited) > 1 {
		return ReportRestartScopeV1{}, errors.New("report restart history observation is ambiguous")
	}
	var history ReportRestartInheritedHistoryV1
	if len(inherited) == 1 {
		history = inherited[0]
	}
	primaries := &reportRestartPrimaryInventoryV1{ctx: ctx, reader: reader, digests: map[string]string{}}
	service := &Service{authority: authority, grants: primaries}
	inventory, err := service.verifyTrustedSnapshotV1(ctx, receipts, dispositions)
	if err != nil {
		return ReportRestartScopeV1{}, err
	}
	scope, err := planReportRestartPreservationV1(ctx, authority.KeyID(), inventory, primaries, history)
	if err != nil {
		return ReportRestartScopeV1{}, err
	}
	// Include every primary read for completed Core inventory validation, even
	// when it is outside the held threads. Only the held threads remain frozen
	// after installation, so independent ordinary work can subsequently proceed.
	if err := primaries.sealV1(ctx); err != nil {
		return ReportRestartScopeV1{}, err
	}
	return scope, nil
}

func (primaries *reportRestartPrimaryInventoryV1) sealV1(ctx context.Context) error {
	ids := make([]string, 0, len(primaries.digests))
	for id := range primaries.digests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, err := primaries.ReadPrimaryThreadSnapshotV1(ctx, id); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	primaries.sealed = true
	return nil
}

type reportRestartPrimaryInventoryV1 struct {
	ctx     context.Context
	reader  recoveryport.PrimaryThreadReaderV1
	digests map[string]string
	sealed  bool
}

func (reader *reportRestartPrimaryInventoryV1) GetThread(id string) (map[string]any, error) {
	snapshot, err := reader.ReadPrimaryThreadSnapshotV1(reader.ctx, id)
	return snapshot.Thread, err
}

func (reader *reportRestartPrimaryInventoryV1) ReadPrimaryThreadSnapshotV1(ctx context.Context, id string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	snapshot, err := reader.reader.ReadPrimaryThreadSnapshotV1(ctx, id)
	if err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	if snapshot.ThreadID != id || !domainsecurity.IsSHA256Hex(snapshot.ThreadFileSHA256) || domainthread.ValidatePrimaryIdentityV1(id, snapshot.Thread) != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("report restart primary inventory is invalid")
	}
	if previous, found := reader.digests[id]; found {
		if previous != snapshot.ThreadFileSHA256 {
			return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("report restart primary changed during inventory validation")
		}
		return snapshot, nil
	}
	if reader.sealed {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("report restart primary is outside the sealed inventory")
	}
	reader.digests[id] = snapshot.ThreadFileSHA256
	return snapshot, nil
}

func (reader *reportRestartPrimaryInventoryV1) ReadCommittedEventLogSHA256V1(ctx context.Context, id string) (string, error) {
	return reader.reader.ReadCommittedEventLogSHA256V1(ctx, id)
}
