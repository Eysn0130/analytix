package jobs

import (
	"context"
	"errors"
	"reflect"
	"strings"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

// RestartPreservationInputV1 carries denial-only dependencies already proved
// by the startup owner. OriginalRecords are raw original values, not AllRecords
// or LoadChildRun projections. JobIDs includes every signed reserved target,
// even when its record is absent. These values never authorize execution.
type RestartPreservationInputV1 struct {
	ThreadIDs         []string
	JobIDs            []string
	OriginalRecords   []Record
	OriginalInventory ChildRunInventoryV1
}

func NewManagerWithRestartPreservationV1(ctx context.Context, root string, verifier ChildCompletionReceiptVerifier, preserved *RestartPreservationInputV1, floors ...domainpendingwork.ChildIdentityFloorsV1) (*Manager, error) {
	return newManagerWithRestartPreservationV1(ctx, root, root, verifier, nil, false, preserved, floors...)
}

func NewManagerForSemanticStartupWithRestartPreservationV1(ctx context.Context, stageRoot, authorityRoot string, verifier ChildCompletionReceiptVerifier, witness *FrozenLegacyTypeScriptLineageWitnessV1, preserved *RestartPreservationInputV1, floors ...domainpendingwork.ChildIdentityFloorsV1) (*Manager, error) {
	return newManagerWithRestartPreservationV1(ctx, stageRoot, authorityRoot, verifier, witness, true, preserved, floors...)
}

func prepareConstructorRestartPreservationV1(ctx context.Context, root string, input *RestartPreservationInputV1) (*restartPreservationV1, error) {
	if input == nil {
		return nil, nil
	}
	scope := &restartPreservationV1{threads: map[string]bool{}, jobIDs: map[string]bool{}, records: map[string]string{}}
	for _, id := range input.ThreadIDs {
		if !domainthread.IsCanonicalRecordID(id) || scope.threads[id] {
			return nil, errors.New("child-run restart thread identity is invalid")
		}
		scope.threads[id] = true
	}
	for _, id := range input.JobIDs {
		if id != strings.TrimSpace(id) || !validPersistedJobID(id) || scope.jobIDs[id] {
			return nil, errors.New("child-run restart job identity is invalid")
		}
		scope.jobIDs[id] = true
	}
	provided, err := restartRecordDigestsV1(input.OriginalRecords)
	if err != nil {
		return nil, err
	}
	if err := validateChildRunInventoryShapeV1(root, input.OriginalInventory); err != nil {
		return nil, err
	}
	scope.inventory = input.OriginalInventory
	scope.inventory.Entries = append([]ChildRunInventoryEntryV1{}, input.OriginalInventory.Entries...)
	observed, err := ReadChildRunIdentitySnapshotV1(ctx, root)
	if err != nil {
		return nil, err
	}
	if err := scope.validateInventory(observed.Inventory); err != nil {
		return nil, err
	}
	for _, record := range observed.Records {
		related := scope.ownsThreads(record)
		if related != scope.jobIDs[record.ID] {
			return nil, errors.New("child-run constructor dependency scope is incomplete")
		}
		if !related {
			continue
		}
		digests, err := restartRecordDigestsV1([]Record{record})
		if err != nil || provided[record.ID] != digests[record.ID] {
			return nil, errors.New("child-run constructor original record changed")
		}
		scope.records[record.ID] = digests[record.ID]
	}
	if !reflect.DeepEqual(scope.records, provided) {
		return nil, errors.New("child-run constructor original inventory changed")
	}
	return scope, nil
}

func (scope *restartPreservationV1) ownsThreads(record Record) bool {
	return scope != nil && (scope.threads[strings.TrimSpace(record.ParentThreadID)] || scope.threads[strings.TrimSpace(record.ChildThreadID)] || record.SecurityBinding != nil && scope.threads[record.SecurityBinding.ParentThreadID])
}

func (scope *restartPreservationV1) ownsJob(id string) bool {
	if scope == nil {
		return false
	}
	_, recorded := scope.records[id]
	return scope.jobIDs[id] || recorded
}

func (scope *restartPreservationV1) validateInventory(current ChildRunInventoryV1) error {
	if scope == nil {
		return nil
	}
	before := scope.inventory
	if before.RootExists && !SameChildRunRootAuthorityV1(before, current) {
		return errors.New("child-run restart root authority changed")
	}
	originalHeld, currentHeld := map[string]ChildRunInventoryEntryV1{}, map[string]ChildRunInventoryEntryV1{}
	for _, entry := range before.Entries {
		if scope.ownsJob(entry.JobID) {
			originalHeld[entry.Name] = entry
		}
	}
	for _, entry := range current.Entries {
		if scope.ownsJob(entry.JobID) {
			currentHeld[entry.Name] = entry
		}
	}
	if !reflect.DeepEqual(originalHeld, currentHeld) {
		return errors.New("preserved child-run physical authority changed")
	}
	return nil
}
