package pendingwork

import (
	"context"
	"errors"
	"reflect"
	"sort"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// OriginalChildScopePrimaryReaderV1 must prove absence from both original
// primary families. A lookup error, normalized view or absent sidecar is not
// proof that a reserved child primary never existed.
type OriginalChildScopePrimaryReaderV1 interface {
	recoveryport.PrimaryThreadReaderV1
	ObserveOriginalPrimaryPresenceV1(context.Context, string) (recoveryport.PrimaryThreadSnapshotV1, bool, error)
}

// ExtendReportRestartChildScopeV1 closes the denial graph over the entire
// original signed vector. Disposition, expiry and missing jobs never authorize
// retry or release a target. Every traversed parent is bound to its original
// frozen context and the exact issued grant prefix before its children enter
// the scope. This function only observes records; it cannot allocate or sign.
func ExtendReportRestartChildScopeV1(ctx context.Context, original ReportRestartScopeV1, inventory TrustedInventoryV1, records []domainjob.Record, authority authorityport.Authority, reader OriginalChildScopePrimaryReaderV1) (ReportRestartScopeV1, error) {
	if ctx == nil || authority == nil || reader == nil || original.keyID == "" || original.keyID != authority.KeyID() {
		return ReportRestartScopeV1{}, ErrAuthorityUnavailable
	}
	if err := original.RevalidatePrimary(ctx); err != nil {
		return ReportRestartScopeV1{}, err
	}
	dispositions := make([]domainpendingwork.PendingWorkDispositionV1, 0, len(inventory.Dispositions))
	for _, disposition := range inventory.Dispositions {
		dispositions = append(dispositions, disposition)
	}
	verified, err := VerifyTrustedSnapshotV1(ctx, inventory.Receipts, dispositions, authority)
	if err != nil {
		return ReportRestartScopeV1{}, err
	}
	if _, err := DeriveChildIdentityFloorsV1(verified, nil, nil); err != nil {
		return ReportRestartScopeV1{}, err
	}
	scope := original
	scope.primaryDigests, scope.contexts, scope.pendingDigests, scope.absentPrimaries = map[string]string{}, map[string]domainsecurity.TurnSecurityContext{}, map[string]string{}, map[string]bool{}
	scope.turnIDs = append([]string(nil), original.turnIDs...)
	scope.reader, scope.presenceReader = reader, reader
	for id, digest := range original.primaryDigests {
		scope.primaryDigests[id] = digest
	}
	for id, frozen := range original.contexts {
		scope.contexts[id] = frozen
	}
	for id := range original.absentPrimaries {
		scope.absentPrimaries[id] = true
	}
	jobs := map[string]domainjob.Record{}
	for _, record := range records {
		if record.ID == "" || jobs[record.ID].ID != "" {
			return ReportRestartScopeV1{}, ErrChildProducerInventoryIncomplete
		}
		jobs[record.ID] = record
	}
	visited, heldJobs := map[string]bool{}, map[string]bool{}
	for changed := true; changed; {
		changed = false
		for _, receipt := range verified.Receipts {
			if receipt.ChildProducer == nil || visited[receipt.WorkID] || !scope.OwnsThread(receipt.Context.ThreadID) {
				continue
			}
			frozen, exists := scope.contexts[receipt.Context.ContextDigest]
			if !exists || !receiptMatchesContext(receipt, frozen) {
				return ReportRestartScopeV1{}, ErrChildProducerInventoryIncomplete
			}
			snapshot, err := scope.ReadPrimaryThreadSnapshotV1(ctx, frozen.ThreadID)
			if err != nil {
				return ReportRestartScopeV1{}, err
			}
			registry, err := executiongrantapp.RegistryFromThread(frozen.ThreadID, snapshot.Thread, frozen.TurnID)
			if err != nil || len(receipt.GrantMembers) != 1 {
				return ReportRestartScopeV1{}, ErrChildProducerInventoryIncomplete
			}
			entry, exists := domainsecurity.ExecutionGrantRegistryEntryByID(registry, receipt.GrantMembers[0].GrantID)
			if !exists || !childProducingSideEffect(entry.Grant.ToolName) || !writeEffectReceiptMatchesRegistry(receipt, snapshot.Thread, entry.Grant) {
				return ReportRestartScopeV1{}, ErrChildProducerInventoryIncomplete
			}
			binding, err := domainjob.NewSecurityBinding(frozen, entry.Grant, entry.Grant.ToolCallID)
			if err != nil || binding.BindingDigest != receipt.ChildProducer.ParentBindingDigest {
				return ReportRestartScopeV1{}, ErrChildProducerInventoryIncomplete
			}
			for _, target := range receipt.ChildProducer.Children {
				if heldJobs[target.JobID] {
					return ReportRestartScopeV1{}, ErrChildProducerInventoryIncomplete
				}
				heldJobs[target.JobID] = true
				if record, exists := jobs[target.JobID]; exists {
					if !reflect.DeepEqual(record.SecurityBinding, binding) {
						return ReportRestartScopeV1{}, ErrChildProducerInventoryIncomplete
					}
					if _, err := DeriveChildIdentityFloorsV1(TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{receipt}}, []domainjob.Record{record}, nil); err != nil {
						return ReportRestartScopeV1{}, err
					}
				}
				if scope.OwnsThread(target.ChildThreadID) {
					continue
				}
				child, present, err := reader.ObserveOriginalPrimaryPresenceV1(ctx, target.ChildThreadID)
				if err != nil {
					return ReportRestartScopeV1{}, err
				}
				if !present {
					scope.absentPrimaries[target.ChildThreadID] = true
				} else {
					if child.ThreadID != target.ChildThreadID || !domainsecurity.IsSHA256Hex(child.ThreadFileSHA256) || domainthread.ValidatePrimaryIdentityV1(child.ThreadID, child.Thread) != nil {
						return ReportRestartScopeV1{}, ErrChildProducerInventoryIncomplete
					}
					scope.primaryDigests[child.ThreadID] = child.ThreadFileSHA256
					if err := scope.observeOriginalTurnsV1(ctx, child.ThreadID, child.Thread, verified); err != nil {
						return ReportRestartScopeV1{}, errors.Join(ErrChildProducerInventoryIncomplete, err)
					}
				}
				changed = true
			}
			visited[receipt.WorkID] = true
		}
	}
	for _, record := range records {
		if (scope.OwnsThread(record.ParentThreadID) || scope.OwnsThread(record.ChildThreadID)) && !heldJobs[record.ID] {
			return ReportRestartScopeV1{}, ErrChildProducerInventoryIncomplete
		}
	}
	for _, receipt := range verified.Receipts {
		if scope.OwnsThread(receipt.Context.ThreadID) {
			scope.pendingDigests[receipt.WorkID] = reportRestartPendingDigest(receipt, verified.Dispositions[receipt.WorkID])
		}
	}
	sort.Strings(scope.turnIDs)
	scope.childJobIDs = heldJobs
	if err := errors.Join(scope.ValidateTrustedPendingInventoryV1(ctx, verified), original.RevalidatePrimary(ctx)); err != nil {
		return ReportRestartScopeV1{}, err
	}
	return scope, nil
}
