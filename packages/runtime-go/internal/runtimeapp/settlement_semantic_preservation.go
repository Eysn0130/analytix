package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	evidencesettlement "analytix.local/runtime-go/internal/adapters/outbound/evidencesettlement"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

const runtimeSettlementSemanticRootV1 = "data/private/evidence-settlements"

type runtimeSettlementSemanticPreservationV1 struct {
	original       runtimeOriginalSemanticFilesV1
	recoveryBefore runtimeOriginalSemanticFilesV1
	held           map[string]string
	sharedModes    map[string]uint32
}

type runtimeSettlementSemanticObservationV1 struct {
	prepared           *evidencesettlement.PreparedInventoryV1
	journal            *persistencefs.AuthenticatedSemanticJournalObservationV1
	revalidateContexts func(context.Context) error
}

func (observation *runtimeSettlementSemanticObservationV1) Revalidate(ctx context.Context) error {
	if observation == nil || observation.prepared == nil {
		return errors.New("original settlement semantic observation is unavailable")
	}
	err := observation.prepared.Revalidate(ctx)
	if observation.journal != nil {
		err = errors.Join(err, observation.journal.Revalidate(ctx))
	}
	if observation.revalidateContexts != nil {
		err = errors.Join(err, observation.revalidateContexts(ctx))
	}
	return err
}

func settlementSemanticRelativeV1(operation domainstartup.SemanticStartupOperationV1) (string, bool) {
	if operation.Path == runtimeSettlementSemanticRootV1 {
		return ".", true
	}
	if strings.HasPrefix(operation.Path, runtimeSettlementSemanticRootV1+"/") {
		return strings.TrimPrefix(operation.Path, runtimeSettlementSemanticRootV1+"/"), true
	}
	return "", false
}

func (files runtimeOriginalSemanticFilesV1) verifySettlementsV1(ctx context.Context, core *runtimeChildIdentityStartupV1, contexts []domainsecurity.TurnSecurityContext) (map[string]domainevidence.PreparedEvidenceSettlement, error) {
	byContext := map[string]domainsecurity.TurnSecurityContext{}
	for _, frozen := range contexts {
		if _, found := byContext[frozen.ContextDigest]; found {
			return nil, errors.New("original settlement context is duplicated")
		}
		byContext[frozen.ContextDigest] = frozen
	}
	records := map[string]domainevidence.PreparedEvidenceSettlement{}
	total := int64(0)
	if len(files) > domainstartup.MaxManagedSnapshotEntriesV1+1 {
		return nil, errors.New("original settlement inventory exceeds entry bound")
	}
	for name, entry := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if path.Clean(name) != name || path.IsAbs(name) || strings.Contains(name, "\\") || strings.HasPrefix(name, "../") || entry.Mode & ^uint32(0o777) != 0 || runtime.GOOS != "windows" && entry.Mode&0o077 != 0 {
			return nil, errors.New("original settlement address or mode is invalid")
		}
		if name != "." {
			parent, found := files[path.Dir(name)]
			if !found || !parent.Directory {
				return nil, errors.New("original settlement parent is absent")
			}
		}
		if entry.Directory {
			parts := strings.Split(name, "/")
			if len(entry.Body) != 0 || name != "." && name != "prepared" && (len(parts) != 2 || parts[0] != "prepared" || len(parts[1]) != 2 || !domainsecurity.IsSHA256Hex(parts[1]+strings.Repeat("0", 62))) {
				return nil, errors.New("original settlement directory grammar is invalid")
			}
			continue
		}
		total += int64(len(entry.Body))
		if total > domainstartup.MaxSemanticStagedTotalBytesV1 {
			return nil, errors.New("original settlement inventory exceeds byte bound")
		}
		record, err := evidencesettlement.ParsePreparedInventoryFileV1(name, entry.Body)
		if err != nil {
			return nil, err
		}
		if record.SecurityContext != byContext[record.SecurityContext.ContextDigest] {
			return nil, errors.New("original prepared settlement is detached from durable context")
		}
		key, public, signature, err := domainevidence.EvidenceSettlementAuthorityMaterial(record)
		if err != nil {
			return nil, err
		}
		if err := core.verification.VerifyTrusted(ctx, key, public, domainevidence.EvidenceSettlementSigningBytes(record), signature); err != nil {
			return nil, err
		}
		if _, found := records[record.SettlementID]; found {
			return nil, errors.New("original prepared settlement is duplicated")
		}
		records[record.SettlementID] = record
	}
	if len(files) > 0 {
		root, found := files["."]
		if !found || !root.Directory {
			return nil, errors.New("original settlement root is invalid")
		}
	}
	return records, ctx.Err()
}

func (files runtimeOriginalSemanticFilesV1) heldSettlementsV1(scope *pendingworkapp.ReportRestartScopeV1) (map[string]string, error) {
	held := map[string]string{}
	for name, entry := range files {
		if entry.Directory {
			continue
		}
		record, err := evidencesettlement.ParsePreparedInventoryFileV1(name, entry.Body)
		if err != nil {
			return nil, err
		}
		if scope.OwnsThread(record.SecurityContext.ThreadID) {
			body, _ := json.Marshal(runtimeOriginalSemanticStateV1(entry, true))
			held[name] = string(body)
		}
	}
	return held, nil
}

func (preserved *runtimeSettlementSemanticPreservationV1) validateHeldV1(files runtimeOriginalSemanticFilesV1, scope *pendingworkapp.ReportRestartScopeV1) error {
	held, err := files.heldSettlementsV1(scope)
	if err != nil || !reflect.DeepEqual(held, preserved.held) {
		return errors.Join(errors.New("semantic settlement candidate changed original held inventory"), err)
	}
	for name, mode := range preserved.sharedModes {
		entry, found := files[name]
		if !found || !entry.Directory || entry.Mode != mode {
			return errors.New("semantic settlement candidate changed original shared directory")
		}
	}
	return nil
}

func readRuntimeSettlementSemanticInventoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1, saved runtimeOriginalSemanticFilesV1) (_ runtimeOriginalSemanticFilesV1, _ runtimeOriginalSemanticFilesV1, _ []domainsecurity.TurnSecurityContext, _ *runtimeSettlementSemanticObservationV1, resultErr error) {
	if ctx == nil || core == nil || core.verification == nil || core.revalidateKey == nil || scope == nil {
		return nil, nil, nil, nil, errors.New("original settlement semantic authority is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return nil, nil, nil, nil, err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	prepared, err := evidencesettlement.ObservePreparedInventoryV1(ctx, filepath.Join(core.roots.DataDir, "private", "evidence-settlements"), core.access)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	observation := &runtimeSettlementSemanticObservationV1{prepared: prepared, journal: journal}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	bodies, err := prepared.SnapshotPreparedFileBytesV1(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	modes, err := prepared.SnapshotFileModesV1(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	physical := runtimeOriginalSemanticFilesV1{}
	for name, mode := range modes {
		body, file := bodies[name]
		physical[name] = evidenceregistrystore.OriginalLegacyEntryV1{Directory: !file, Mode: mode, Body: body}
	}
	contexts, revalidateContexts, err := runtimeRegistryContextsV1(ctx, core)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	observation.revalidateContexts = revalidateContexts
	if _, err := physical.verifySettlementsV1(ctx, core, contexts); err != nil {
		return nil, nil, nil, nil, err
	}
	original, final := physical.cloneV1(), physical.cloneV1()
	for index, operation := range journal.OperationsV1() {
		if err := validateRuntimeOriginalSemanticAncestorV1(runtimeSettlementSemanticRootV1, operation); err != nil {
			return nil, nil, nil, nil, err
		}
		name, owned := settlementSemanticRelativeV1(operation)
		if !owned {
			continue
		}
		entry, found := physical[name]
		state := runtimeOriginalSemanticStateV1(entry, found)
		if state != operation.Before {
			if index > journal.NextOperationV1() || state != operation.After {
				return nil, nil, nil, nil, errors.New("settlement physical state is outside authenticated semantic prefix")
			}
			after, err := journal.PhysicallyAfterV1(ctx, operation)
			if err != nil || !after {
				return nil, nil, nil, nil, errors.Join(errors.New("settlement physical state has not reached signed After"), err)
			}
		}
		switch operation.Before.Type {
		case domainstartup.ManagedEntryTypeAbsent:
			delete(original, name)
		case domainstartup.ManagedEntryTypeDirectory:
			original[name] = evidenceregistrystore.OriginalLegacyEntryV1{Directory: true, Mode: operation.Before.Mode & 0o777}
		case domainstartup.ManagedEntryTypeFile:
			if !found || entry.Directory || int64(len(entry.Body)) != operation.Before.Size || domainsecurity.SHA256Hex(entry.Body) != operation.Before.SHA256 {
				previous, exists := saved[name]
				if !exists || runtimeOriginalSemanticStateV1(previous, true) != operation.Before {
					return nil, nil, nil, nil, errors.New("settlement semantic original Before bytes are unavailable")
				}
				entry = previous
			}
			entry.Mode = operation.Before.Mode
			original[name] = entry
		default:
			return nil, nil, nil, nil, errors.New("settlement original entry type is invalid")
		}
		if err := final.applyV1(operation, name, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return journal.ReadAfterV1(ctx, operation)
		}); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	for _, endpoint := range []runtimeOriginalSemanticFilesV1{original, final} {
		if _, err := endpoint.verifySettlementsV1(ctx, core, contexts); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	return original, physical, contexts, observation, nil
}

func prepareRuntimeSettlementSemanticPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (_ *runtimeSettlementSemanticPreservationV1, resultErr error) {
	original, physical, _, observation, err := readRuntimeSettlementSemanticInventoryV1(ctx, core, scope, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	held, err := original.heldSettlementsV1(scope)
	if err != nil {
		return nil, err
	}
	modes := map[string]uint32{}
	for name, entry := range original {
		if entry.Directory {
			modes[name] = entry.Mode
		}
	}
	preserved := &runtimeSettlementSemanticPreservationV1{original: original.cloneV1(), recoveryBefore: original.cloneV1(), held: held, sharedModes: modes}
	for _, operation := range observation.journal.OperationsV1() {
		name, owned := settlementSemanticRelativeV1(operation)
		if !owned {
			continue
		}
		if err := physical.applyV1(operation, name, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return observation.journal.ReadAfterV1(ctx, operation)
		}); err != nil {
			return nil, err
		}
	}
	if err := preserved.validateHeldV1(physical, scope); err != nil {
		return nil, err
	}
	return preserved, nil
}

func (preserved runtimeReportRestartPreservationV1) validateSettlementSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if preserved.settlements == nil || preserved.core == nil || preserved.report == nil {
		return errors.New("original settlement preservation is unavailable")
	}
	original, candidate, contexts, observation, err := readRuntimeSettlementSemanticInventoryV1(ctx, preserved.core, preserved.report, preserved.settlements.recoveryBefore)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
		if resultErr == nil {
			preserved.settlements.recoveryBefore = original.cloneV1()
		}
	}()
	if err := preserved.settlements.validateHeldV1(original, preserved.report); err != nil {
		return err
	}
	for _, operation := range observation.journal.OperationsV1() {
		name, owned := settlementSemanticRelativeV1(operation)
		if !owned {
			continue
		}
		if err := candidate.applyV1(operation, name, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return observation.journal.ReadAfterV1(ctx, operation)
		}); err != nil {
			return err
		}
	}
	for _, operation := range operations {
		if noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
			continue
		}
		if err := validateRuntimeOriginalSemanticAncestorV1(runtimeSettlementSemanticRootV1, operation); err != nil {
			return err
		}
		name, owned := settlementSemanticRelativeV1(operation)
		if !owned {
			continue
		}
		if operation.Kind == domainstartup.SemanticOperationInstallFile {
			if readAfter == nil {
				return errors.New("settlement semantic After bytes are unavailable")
			}
			body, err := readAfter(operation)
			if err != nil {
				return err
			}
			record, err := evidencesettlement.ParsePreparedInventoryFileV1(name, body)
			if err != nil {
				return err
			}
			canonical, err := domainevidence.PreparedEvidenceSettlementBytes(record)
			if err != nil || !bytes.Equal(body, canonical) {
				return errors.Join(errors.New("settlement semantic After encoding is noncanonical"), err)
			}
			if err := candidate.applyV1(operation, name, func(domainstartup.SemanticStartupOperationV1) ([]byte, error) { return body, nil }); err != nil {
				return err
			}
		} else if err := candidate.applyV1(operation, name, readAfter); err != nil {
			return err
		}
	}
	if _, err := candidate.verifySettlementsV1(ctx, preserved.core, contexts); err != nil {
		return err
	}
	return preserved.settlements.validateHeldV1(candidate, preserved.report)
}
