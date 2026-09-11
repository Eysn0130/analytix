package runtimeapp

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	evidencestore "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthority"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// Closed history is not a held execution scope. Semantic startup may
// normalize its primary, but must preserve the exact report/Core result graph.
type runtimeCompletedReportSemanticPreservationV1 struct {
	core        *runtimeChildIdentityStartupV1
	publication *runtimePublicationSemanticPreservationV1
	advance     *runtimeAuthorityAdvanceStartupV2
	history     *runtimeOriginalReportHistoryV1
	reports     runtimeOriginalSemanticFilesV1
	evidence    runtimeOriginalSemanticFilesV1
	primaryIDs  map[string]string
	pending     map[string]bool
}

func prepareRuntimeCompletedReportSemanticPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, advance *runtimeAuthorityAdvanceStartupV2, history *runtimeOriginalReportHistoryV1, original, final, associatedOriginal, associatedFinal map[string]runtimeOriginalSemanticFilesV1) (*runtimeCompletedReportSemanticPreservationV1, error) {
	if !reflect.DeepEqual(runtimeReportCommittedFilesV1(original["report-publication"]), runtimeReportCommittedFilesV1(final["report-publication"])) {
		return nil, errors.New("completed report immutable history changed in signed Final")
	}
	if err := validateRuntimeReportEvidenceOriginalV1(associatedOriginal["evidence-authority"], associatedFinal["evidence-authority"]); err != nil {
		return nil, err
	}
	preserved := &runtimeCompletedReportSemanticPreservationV1{
		core: core, publication: publication, advance: advance, history: history,
		reports: original["report-publication"].cloneV1(), evidence: associatedOriginal["evidence-authority"].cloneV1(),
		primaryIDs: map[string]string{}, pending: map[string]bool{},
	}
	for id, entry := range core.primaries.entries {
		if _, err := core.primaries.ReadPrimaryThreadSnapshotV1(ctx, id); err != nil {
			return nil, err
		}
		preserved.primaryIDs[entry.primary.Path] = id
	}
	for _, entry := range history.plan.Attempts {
		for _, leaf := range []string{"receipts", "dispositions"} {
			preserved.pending[pendingSemanticPathV1(leaf, entry.Stage.WorkID)] = true
		}
		if core.pendingSemantic != nil {
			if got, found := core.pendingSemantic.final.Dispositions[entry.Stage.WorkID]; found != (entry.Disposition != nil) || (found && !reflect.DeepEqual(got, *entry.Disposition)) {
				return nil, errors.New("completed report disposition changed in signed Final")
			}
		}
	}
	return preserved, nil
}

// Native physical/domain parsers have already classified these files. Ordinary
// temporary writes may be cleaned up; committed bytes and modes stay exact.
func runtimeReportCommittedFilesV1(files runtimeOriginalSemanticFilesV1) runtimeOriginalSemanticFilesV1 {
	result := runtimeOriginalSemanticFilesV1{}
	for name, entry := range files {
		if entry.Directory {
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) >= 3 {
			if _, residue := domainprivatecas.ClassifyRecordResidueNameV1(parts[len(parts)-1], parts[len(parts)-2]); residue {
				continue
			}
		}
		result[name] = entry
	}
	return result
}

func (preserved *runtimeCompletedReportSemanticPreservationV1) ValidateSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	return preserved.withSemanticCandidateV1(ctx, operations, readAfter, noWriteOperationID, nil)
}

func (preserved *runtimeCompletedReportSemanticPreservationV1) withSemanticCandidateV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string, use func(*runtimeOriginalReportHistoryV1) error) (resultErr error) {
	if ctx == nil || preserved == nil {
		return errRuntimeReportRestartReconciliationRequired
	}
	if err := errors.Join(context.Cause(ctx), preserved.core.revalidateKey(ctx), preserved.core.originalRegistryTrust.Revalidate(ctx)); err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, context.Cause(ctx), preserved.core.revalidateKey(ctx), preserved.core.originalRegistryTrust.Revalidate(ctx))
	}()
	evidence, revalidateEvidence, err := preserved.projectEvidenceV1(ctx, operations, readAfter, noWriteOperationID)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, revalidateEvidence()) }()
	primaries := runtimeReportSemanticPrimariesV1{}
	physical := runtimeReportSemanticPrimariesV1{}
	for id, entry := range preserved.core.primaries.entries {
		// Reobserve the anchored physical cut. A previously applied harmless
		// primary rewrite is allowed only while its original report graph holds.
		snapshot, err := entry.reader.ReadPrimaryThreadSnapshotV1(ctx, id)
		if err != nil {
			return err
		}
		primaries[id] = snapshot
		physical[id] = snapshot
	}
	defer func() {
		for id, before := range physical {
			after, err := preserved.core.primaries.entries[id].reader.ReadPrimaryThreadSnapshotV1(ctx, id)
			if err == nil && before.ThreadFileSHA256 != after.ThreadFileSHA256 {
				err = errors.New("completed report physical primary changed during candidate audit")
			}
			resultErr = errors.Join(resultErr, err)
		}
	}()
	for _, operation := range operations {
		if operation.OperationID == noWriteOperationID && noWriteOperationID != "" {
			continue
		}
		for _, owner := range []string{"report-publication", "evidence-authority", "authority-advance", "pii-authorization", "controlled-artifact-access-v2"} {
			if err := validateRuntimeOriginalSemanticAncestorV1("data/private/"+owner, operation); err != nil {
				return err
			}
			name, owned := runtimeAssociatedRelativeV1(owner, operation)
			if !owned {
				continue
			}
			if preserved.history.mixedClosed == nil && owner == "controlled-artifact-access-v2" && operation.Kind == domainstartup.SemanticOperationInstallFile && operation.Before.Type == domainstartup.ManagedEntryTypeAbsent && operation.After.Type == domainstartup.ManagedEntryTypeFile {
				// The complete projected inventory below permits only the exact
				// full-length indeterminate closure of an Original-open receipt.
				continue
			}
			if owner == "evidence-authority" && operation.Kind == domainstartup.SemanticOperationInstallFile && operation.Before.Type == domainstartup.ManagedEntryTypeAbsent && operation.After.Type == domainstartup.ManagedEntryTypeFile {
				// Independently validated additions do not replace any historical
				// witness. The complete projected graph is audited below.
				continue
			}
			// These owners have no completed-report semantic record writer.
			// Native ordinary residue cleanup and empty topology are independent.
			if operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory {
				if operation.Kind == domainstartup.SemanticOperationCreateDirectory || operation.Kind == domainstartup.SemanticOperationRemoveDirectory {
					continue
				}
			}
			parts := strings.Split(name, "/")
			if len(parts) >= 3 {
				residue, ok := domainprivatecas.ClassifyRecordResidueNameV1(path.Base(name), parts[len(parts)-2])
				if ok && residue.Kind == domainprivatecas.ResidueOrdinaryWriteV1 && operation.Kind == domainstartup.SemanticOperationRemoveFile && operation.Before.Type == domainstartup.ManagedEntryTypeFile && operation.After.Type == domainstartup.ManagedEntryTypeAbsent {
					continue
				}
			}
			return fmt.Errorf("completed report semantic plan changed an immutable authority record: owner=%s operation=%s", owner, operation.Kind)
		}
		for target := range preserved.pending {
			if operation.Path == target || strings.HasPrefix(target, operation.Path+"/") {
				return errors.New("completed report semantic plan changed its Core receipt or disposition")
			}
		}
		for target, id := range preserved.primaryIDs {
			for _, family := range []string{"durable/threads/", "durable/runtime-go/threads/"} {
				threadRoot := family + id
				if target != threadRoot+"/thread.json" && (operation.Path == threadRoot || strings.HasPrefix(operation.Path, threadRoot+"/")) {
					return errors.New("completed report candidate primary has conflicting families")
				}
			}
			if strings.HasPrefix(target, operation.Path+"/") {
				return errors.New("completed report semantic plan changed a primary ancestor")
			}
			if operation.Path != target {
				continue
			}
			if operation.Kind != domainstartup.SemanticOperationInstallFile || operation.Before.Type != domainstartup.ManagedEntryTypeFile || operation.After.Type != domainstartup.ManagedEntryTypeFile || readAfter == nil {
				return errors.New("completed report candidate primary is unavailable")
			}
			body, err := readAfter(operation)
			if err != nil || int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
				return errors.Join(errors.New("completed report candidate primary lost After integrity"), err)
			}
			snapshot, err := finalauthority.ParsePrimaryThreadSnapshotV1(ctx, id, body)
			if err != nil {
				return err
			}
			primaries[id] = snapshot
		}
	}
	candidate, err := prepareRuntimeReportHistoryEndpointV1(ctx, preserved.core, preserved.publication, preserved.advance, preserved.core.pendingInventory, preserved.reports, evidence, primaries)
	if err != nil {
		return err
	}
	if preserved.history.mixedClosed != nil {
		if err := preserved.history.validateMixedCandidateV1(ctx, preserved, candidate, primaries); err != nil {
			return err
		}
	} else if preserved.history.closedFailed {
		if !preserved.history.matchesClosedFailedPlanV1(candidate.plan) {
			return errors.New("closed report semantic candidate changed its historical prefix")
		}
		if err := validateRuntimeClosedFailedCoreV1(ctx, preserved.core, preserved.publication, preserved.advance, candidate.plan, preserved.core.pendingInventory, primaries); err != nil {
			return err
		}
		if err := preserved.history.validateClosedResultsV1(ctx, primaries); err != nil {
			return err
		}
	} else if preserved.history.closedCompletedPrefix {
		if !preserved.history.matchesClosedCompletedPrefixPlanV1(candidate.plan) {
			return errors.New("closed completed report semantic candidate changed its historical prefix")
		}
		if err := validateRuntimeClosedCompletedCoreV1(ctx, preserved.core, preserved.publication, preserved.advance, candidate.plan, preserved.core.pendingInventory, primaries); err != nil {
			return err
		}
	} else if !preserved.history.matchesCompletedPlanV1(candidate.plan) {
		return errors.New("completed report semantic candidate changed its historical plan")
	}
	if preserved.history.controlled != nil {
		if err := preserved.history.controlled.validateCurrentV2(ctx, preserved.core, preserved.publication, candidate, preserved.reports, operations, readAfter, noWriteOperationID); err != nil {
			return err
		}
	}
	if use != nil {
		return use(candidate)
	}
	return nil
}

func validateRuntimeReportEvidenceOriginalV1(original, candidate runtimeOriginalSemanticFilesV1) error {
	for name, before := range runtimeReportCommittedFilesV1(original) {
		if after, found := candidate[name]; !found || !reflect.DeepEqual(before, after) {
			return errors.New("completed report original witness bytes, mode or presence changed")
		}
	}
	return nil
}

// Reobserve the native physical cut, then project the full signed Final and
// prospective plan. All additions pass the complete historical graph audit.
func (preserved *runtimeCompletedReportSemanticPreservationV1) projectEvidenceV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (_ runtimeOriginalSemanticFilesV1, _ func() error, resultErr error) {
	observation, err := evidencestore.PrepareOriginalObservationV1(ctx, filepath.Join(preserved.core.roots.DataDir, "private", "evidence-authority"), preserved.core.access)
	if err != nil {
		return nil, nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, observation.RevalidatePhysicalV1(ctx)) }()
	raw, err := observation.SnapshotOriginalFilesV1(ctx)
	if err != nil {
		return nil, nil, err
	}
	candidate := runtimeOriginalSemanticFilesV1(raw).cloneV1()
	// Future After bytes must never heal a missing or changed Original.
	if err := validateRuntimeReportEvidenceOriginalV1(preserved.evidence, candidate); err != nil {
		return nil, nil, err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, preserved.core.roots, preserved.core.originalCreateProofV1())
	if err != nil {
		return nil, nil, err
	}
	revalidate := func() error {
		err := observation.RevalidatePhysicalV1(ctx)
		if journal != nil {
			err = errors.Join(err, journal.Revalidate(ctx))
		}
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, revalidate()) }()
	project := func(operations []domainstartup.SemanticStartupOperationV1, reader func(domainstartup.SemanticStartupOperationV1) ([]byte, error)) error {
		for _, operation := range operations {
			if operation.OperationID == noWriteOperationID && noWriteOperationID != "" {
				continue
			}
			if err := validateRuntimeOriginalSemanticAncestorV1("data/private/evidence-authority", operation); err != nil {
				return err
			}
			if name, owned := runtimeAssociatedRelativeV1("evidence-authority", operation); owned {
				if err := candidate.applyV1(operation, name, reader); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := project(journal.OperationsV1(), func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
		return journal.ReadAfterV1(ctx, operation)
	}); err != nil {
		return nil, nil, err
	}
	if err := project(operations, readAfter); err != nil {
		return nil, nil, err
	}
	if err := validateRuntimeReportEvidenceOriginalV1(preserved.evidence, candidate); err != nil {
		return nil, nil, err
	}
	return candidate, revalidate, nil
}

type runtimeReportSemanticPrimariesV1 map[string]recoveryport.PrimaryThreadSnapshotV1

func (primaries runtimeReportSemanticPrimariesV1) ReadPrimaryThreadSnapshotV1(ctx context.Context, id string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	snapshot, found := primaries[id]
	return cloneRuntimeOriginalReportValueV1(ctx, snapshot, found)
}

func (runtimeReportSemanticPrimariesV1) ReadCommittedEventLogSHA256V1(context.Context, string) (string, error) {
	return "", errors.New("completed report semantic primary cannot supply event authority")
}
