package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	"analytix.local/runtime-go/internal/jobs"
)

const runtimeChildSemanticRootV1 = "data/child-runs"

// Constructor verification of an immutable held record uses only its original
// proof. The live authority remains responsible for every independent record.
func (preserved runtimeReportRestartPreservationV1) childConstructorVerifierV1(live jobs.ChildCompletionReceiptVerifier) jobs.ChildCompletionReceiptVerifier {
	if preserved.report == nil || len(preserved.report.ThreadIDs()) == 0 {
		return live
	}
	return runtimeStoredChildConstructorVerifierV1{preserved: preserved, live: live}
}

type runtimeStoredChildConstructorVerifierV1 struct {
	preserved runtimeReportRestartPreservationV1
	live      jobs.ChildCompletionReceiptVerifier
}

func (verifier runtimeStoredChildConstructorVerifierV1) VerifyStoredChildCompletion(ctx context.Context, record domainjob.Record) error {
	if ctx == nil {
		return errors.New("stored child constructor context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	scope := verifier.preserved.report
	if scope == nil || verifier.preserved.core == nil {
		return errors.New("stored child constructor original authority is unavailable")
	}
	held := false
	for _, id := range scope.DeniedChildJobIDsV1() {
		held = held || id == record.ID
	}
	related := scope.OwnsThread(record.ParentThreadID) || scope.OwnsThread(record.ChildThreadID) ||
		record.SecurityBinding != nil && scope.OwnsThread(record.SecurityBinding.ParentThreadID)
	if held != related {
		return errors.New("stored child constructor dependency scope changed")
	}
	if held {
		for _, original := range verifier.preserved.core.jobRecords {
			if original.ID == record.ID && reflect.DeepEqual(original, record) {
				return (runtimeOriginalStoredChildVerifierV1{preserved: verifier.preserved}).VerifyStoredChildCompletion(ctx, record)
			}
		}
		return errors.New("stored child constructor original record changed")
	}
	if verifier.live == nil {
		return errors.New("stored child constructor live authority is unavailable")
	}
	return verifier.live.VerifyStoredChildCompletion(ctx, record)
}

func (preserved runtimeReportRestartPreservationV1) childConstructorInputV1(ctx context.Context) (*jobs.RestartPreservationInputV1, error) {
	if preserved.report == nil || len(preserved.report.ThreadIDs()) == 0 {
		return nil, nil
	}
	if preserved.core == nil {
		return nil, errors.New("original child-run constructor observation is unavailable")
	}
	if err := preserved.report.RevalidatePrimary(ctx); err != nil {
		return nil, err
	}
	input := &jobs.RestartPreservationInputV1{
		ThreadIDs: preserved.report.DeniedThreadIDsV1(), JobIDs: preserved.report.DeniedChildJobIDsV1(),
		OriginalInventory: preserved.core.jobs,
	}
	held := map[string]bool{}
	for _, id := range input.JobIDs {
		held[id] = true
	}
	for _, record := range preserved.core.jobRecords {
		if held[record.ID] {
			input.OriginalRecords = append(input.OriginalRecords, record)
		}
	}
	return input, nil
}

// A physical primary introduced or changed by this transaction is not an
// original parent/child witness. Check the entire authenticated program, not
// only its remaining suffix, before returning a scope to any startup consumer.
func validateRuntimeOriginalScopeJournalV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (resultErr error) {
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return err
	}
	if journal == nil {
		return nil
	}
	defer func() { resultErr = errors.Join(resultErr, journal.Revalidate(ctx)) }()
	for _, operation := range journal.OperationsV1() {
		for _, id := range scope.DeniedThreadIDsV1() {
			for _, path := range []string{"durable/threads/" + id, "durable/runtime-go/threads/" + id} {
				if operation.Path == path || strings.HasPrefix(operation.Path, path+"/") {
					return pendingworkapp.ErrRestartPreserved
				}
				if strings.HasPrefix(path, operation.Path+"/") &&
					(operation.Kind != domainstartup.SemanticOperationCreateDirectory || operation.Before.Type != domainstartup.ManagedEntryTypeAbsent || operation.After.Type != domainstartup.ManagedEntryTypeDirectory) {
					return pendingworkapp.ErrRestartPreserved
				}
			}
		}
	}
	return nil
}

func childSemanticAddressV1(operation domainstartup.SemanticStartupOperationV1) (jobs.ChildRunInventoryEntryKindV1, string, bool, error) {
	if !strings.HasPrefix(operation.Path, runtimeChildSemanticRootV1+"/") {
		return "", "", false, nil
	}
	name := strings.TrimPrefix(operation.Path, runtimeChildSemanticRootV1+"/")
	kind, id, ok := jobs.ClassifyChildRunInventoryNameV1(name)
	if !ok || strings.Contains(name, "/") {
		return "", "", false, errors.New("semantic child-run target grammar is invalid")
	}
	return kind, id, true, nil
}

// This denial-only guard checks original physical identity and the complete
// child-run owner's operational rules before any semantic effect.
func (preserved runtimeReportRestartPreservationV1) validateChildSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	core, scope := preserved.core, preserved.report
	if ctx == nil || core == nil || scope == nil || core.revalidateKey == nil {
		return errors.New("original child semantic preservation is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return err
	}
	if journal != nil {
		defer func() { resultErr = errors.Join(resultErr, journal.Revalidate(ctx)) }()
	}
	root := filepath.Join(core.roots.DataDir, "child-runs")
	observed, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, root)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, jobs.ValidateChildRunInventoryV1(root, observed.Inventory), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	held := map[string]bool{}
	for _, id := range scope.DeniedChildJobIDsV1() {
		held[id] = true
	}
	if core.jobs.RootExists && !jobs.SameChildRunRootAuthorityV1(core.jobs, observed.Inventory) {
		return errors.New("original child-run root identity changed")
	}
	originalHeld, currentHeld := map[string]jobs.ChildRunInventoryEntryV1{}, map[string]jobs.ChildRunInventoryEntryV1{}
	for _, entry := range core.jobs.Entries {
		if held[entry.JobID] {
			originalHeld[entry.Name] = entry
		}
	}
	for _, entry := range observed.Inventory.Entries {
		if held[entry.JobID] {
			currentHeld[entry.Name] = entry
		}
	}
	if !reflect.DeepEqual(originalHeld, currentHeld) {
		return errors.New("original held child-run physical inventory changed")
	}
	deniedRecord := func(record domainjob.Record) bool {
		return held[record.ID] || scope.OwnsThread(strings.TrimSpace(record.ParentThreadID)) || scope.OwnsThread(strings.TrimSpace(record.ChildThreadID)) || record.SecurityBinding != nil && scope.OwnsThread(record.SecurityBinding.ParentThreadID)
	}
	for _, record := range observed.Records {
		if deniedRecord(record) && !held[record.ID] {
			return pendingworkapp.ErrChildProducerInventoryIncomplete
		}
	}
	check := func(operation domainstartup.SemanticStartupOperationV1, allowNoWrite bool) error {
		if allowNoWrite && noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
			return nil
		}
		if len(held) != 0 && (operation.Path == runtimeChildSemanticRootV1 || strings.HasPrefix(runtimeChildSemanticRootV1, operation.Path+"/")) &&
			(operation.Kind != domainstartup.SemanticOperationCreateDirectory || operation.Before.Type != domainstartup.ManagedEntryTypeAbsent || operation.After.Type != domainstartup.ManagedEntryTypeDirectory) {
			return pendingworkapp.ErrRestartPreserved
		}
		_, id, addressed, err := childSemanticAddressV1(operation)
		if err != nil {
			return err
		}
		if addressed && held[id] {
			return pendingworkapp.ErrRestartPreserved
		}
		return nil
	}
	if journal != nil {
		// The entire signed program is checked, including its applied prefix.
		// A removed original job cannot be recast as a never-started reservation.
		for _, operation := range journal.OperationsV1() {
			if err := check(operation, false); err != nil {
				return err
			}
		}
	}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := check(operation, true); err != nil {
			return err
		}
		if noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
			continue
		}
		kind, id, addressed, err := childSemanticAddressV1(operation)
		if err != nil {
			return err
		}
		if !addressed || operation.Kind != domainstartup.SemanticOperationInstallFile {
			continue
		}
		if kind != jobs.ChildRunInventoryRecordV1 {
			return errors.New("semantic child-run install is outside the committed record owner")
		}
		if readAfter == nil {
			return errors.New("semantic child-run After bytes are unavailable")
		}
		body, err := readAfter(operation)
		if err != nil {
			return err
		}
		if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
			return errors.New("semantic child-run After bytes lost integrity")
		}
		record, err := jobs.ParseChildRunIdentityRecordV1(body, id)
		if err != nil {
			return err
		}
		if deniedRecord(record) {
			return pendingworkapp.ErrRestartPreserved
		}
	}
	return preserved.validateRuntimeChildSemanticRecordsV1(ctx, observed, held, journal, operations, readAfter, noWriteOperationID)
}
