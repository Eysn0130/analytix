package runtimeapp

import (
	"context"
	"errors"
	"reflect"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	"analytix.local/runtime-go/internal/jobs"
)

type runtimeChildSemanticRecordsV1 map[string]domainjob.Record

func (records runtimeChildSemanticRecordsV1) cloneV1() runtimeChildSemanticRecordsV1 {
	copy := runtimeChildSemanticRecordsV1{}
	for id, record := range records {
		copy[id] = record
	}
	return copy
}

func (records runtimeChildSemanticRecordsV1) applyV1(operation domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error)) error {
	kind, id, addressed, err := childSemanticAddressV1(operation)
	if err != nil {
		return err
	}
	if !addressed || kind != jobs.ChildRunInventoryRecordV1 {
		return nil
	}
	switch operation.Kind {
	case domainstartup.SemanticOperationRemoveFile:
		delete(records, id)
	case domainstartup.SemanticOperationSetMode:
	case domainstartup.SemanticOperationInstallFile:
		if readAfter == nil {
			return errors.New("child-run semantic After bytes are unavailable")
		}
		body, err := readAfter(operation)
		if err != nil {
			return err
		}
		if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
			return errors.New("child-run semantic After bytes lost integrity")
		}
		record, err := jobs.ParseChildRunIdentityRecordV1(body, id)
		if err != nil {
			return err
		}
		records[id] = record
	default:
		return errors.New("child-run semantic record transition is invalid")
	}
	return nil
}

func (preserved runtimeReportRestartPreservationV1) validateRuntimeChildSemanticRecordsV1(ctx context.Context, observed jobs.ChildRunIdentitySnapshotV1, held map[string]bool, journal *persistencefs.AuthenticatedSemanticJournalObservationV1, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) error {
	physical := runtimeChildSemanticRecordsV1{}
	entries := map[string]jobs.ChildRunInventoryEntryV1{}
	for _, record := range observed.Records {
		if _, duplicate := physical[record.ID]; duplicate {
			return errors.New("child-run semantic identity is duplicated")
		}
		physical[record.ID] = record
	}
	for _, entry := range observed.Inventory.Entries {
		if entry.Kind == jobs.ChildRunInventoryRecordV1 {
			entries[entry.JobID] = entry
		}
	}
	original, final := physical.cloneV1(), physical.cloneV1()
	for index, operation := range journal.OperationsV1() {
		kind, id, addressed, err := childSemanticAddressV1(operation)
		if err != nil {
			return err
		}
		if !addressed || kind != jobs.ChildRunInventoryRecordV1 {
			continue
		}
		entry, present := entries[id]
		switch operation.Before.Type {
		case domainstartup.ManagedEntryTypeAbsent:
			if present {
				if index > journal.NextOperationV1() || operation.Kind != domainstartup.SemanticOperationInstallFile || entry.SizeBytes != operation.After.Size || entry.SHA256 != operation.After.SHA256 {
					return errors.New("child-run addition is not an applied signed operation")
				}
				after, err := journal.PhysicallyAfterV1(ctx, operation)
				if err != nil || !after {
					return errors.Join(errors.New("child-run addition has not reached signed After"), err)
				}
			}
			delete(original, id)
		case domainstartup.ManagedEntryTypeFile:
			if !present || entry.SizeBytes != operation.Before.Size || entry.SHA256 != operation.Before.SHA256 {
				return errors.New("child-run original Before bytes are unavailable")
			}
		default:
			return errors.New("child-run original record type is invalid")
		}
		if err := final.applyV1(operation, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return journal.ReadAfterV1(ctx, operation)
		}); err != nil {
			return err
		}
	}
	validate := func(records runtimeChildSemanticRecordsV1, verifier jobs.ChildCompletionReceiptVerifier) error {
		for id, record := range records {
			if err := ctx.Err(); err != nil {
				return err
			}
			var err error
			if source, exists := original[id]; exists && !held[id] && reflect.DeepEqual(source, record) {
				err = jobs.ValidateChildRunSemanticSourceV1(ctx, record, verifier)
			} else {
				err = jobs.ValidateCommittedChildRunRecordV1(ctx, record, verifier)
			}
			if err != nil {
				return err
			}
		}
		return nil
	}
	// A signed future repair may not wash away a malformed original record.
	if err := validate(original, runtimeOriginalStoredChildVerifierV1{preserved: preserved}); err != nil {
		return err
	}
	if err := validate(final, runtimeOriginalStoredChildVerifierV1{preserved: preserved, projection: &runtimeChildCompletionProjectionV1{operations: journal.OperationsV1(), readAfter: func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
		return journal.ReadAfterV1(ctx, operation)
	}}}); err != nil {
		return err
	}
	candidate := physical.cloneV1()
	for _, operation := range operations {
		if noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
			continue
		}
		if err := candidate.applyV1(operation, readAfter); err != nil {
			return err
		}
	}
	return validate(candidate, runtimeOriginalStoredChildVerifierV1{preserved: preserved, projection: &runtimeChildCompletionProjectionV1{operations: operations, readAfter: readAfter, noWriteOperationID: noWriteOperationID}})
}
