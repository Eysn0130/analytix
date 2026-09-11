package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

const runtimeRegistrySemanticRootV1 = "data/private/evidence-registry"

type runtimeOriginalSemanticFilesV1 map[string]evidenceregistrystore.OriginalLegacyEntryV1

type runtimeRegistrySemanticPreservationV1 struct {
	original       runtimeOriginalSemanticFilesV1
	recoveryBefore runtimeOriginalSemanticFilesV1
	held           map[string]string
	sharedModes    map[string]uint32
	unavailable    bool
}

type runtimeRegistrySemanticObservationV1 struct {
	prepared           *evidenceregistrystore.PreparedRecoveryV2
	journal            *persistencefs.AuthenticatedSemanticJournalObservationV1
	revalidateContexts func(context.Context) error
	unavailable        bool
}

func (observation *runtimeRegistrySemanticObservationV1) Revalidate(ctx context.Context) error {
	if observation == nil || observation.prepared == nil {
		return errors.New("original registry semantic observation is unavailable")
	}
	err := observation.prepared.RevalidatePhysicalV2(ctx)
	if observation.journal != nil {
		err = errors.Join(err, observation.journal.Revalidate(ctx))
	}
	if observation.revalidateContexts != nil {
		err = errors.Join(err, observation.revalidateContexts(ctx))
	}
	return err
}

func (files runtimeOriginalSemanticFilesV1) cloneV1() runtimeOriginalSemanticFilesV1 {
	copied := runtimeOriginalSemanticFilesV1{}
	for name, entry := range files {
		entry.Body = append([]byte(nil), entry.Body...)
		copied[name] = entry
	}
	return copied
}

func runtimeOriginalSemanticStateV1(entry evidenceregistrystore.OriginalLegacyEntryV1, found bool) domainstartup.SemanticEntryStateV1 {
	if !found {
		return domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
	}
	if entry.Directory {
		return domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeDirectory, Mode: entry.Mode | uint32(os.ModeDir)}
	}
	return domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: entry.Mode, Size: int64(len(entry.Body)), SHA256: domainsecurity.SHA256Hex(entry.Body)}
}

func registrySemanticRelativeV1(operation domainstartup.SemanticStartupOperationV1) (string, bool) {
	if operation.Path == runtimeRegistrySemanticRootV1 {
		return ".", true
	}
	if strings.HasPrefix(operation.Path, runtimeRegistrySemanticRootV1+"/") {
		return strings.TrimPrefix(operation.Path, runtimeRegistrySemanticRootV1+"/"), true
	}
	return "", false
}

func validateRuntimeOriginalSemanticAncestorV1(root string, operation domainstartup.SemanticStartupOperationV1) error {
	if strings.HasPrefix(root, operation.Path+"/") && operation.Before != operation.After && operation.Kind != domainstartup.SemanticOperationCreateDirectory {
		return errors.New("semantic operation mutates original owner ancestor")
	}
	return nil
}

func (files runtimeOriginalSemanticFilesV1) applyV1(operation domainstartup.SemanticStartupOperationV1, name string, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error)) error {
	switch operation.After.Type {
	case domainstartup.ManagedEntryTypeAbsent:
		delete(files, name)
	case domainstartup.ManagedEntryTypeDirectory:
		if operation.After.Mode & ^(uint32(os.ModeDir)|0o777) != 0 {
			return errors.New("original semantic directory mode is invalid")
		}
		files[name] = evidenceregistrystore.OriginalLegacyEntryV1{Directory: true, Mode: operation.After.Mode & 0o777}
	case domainstartup.ManagedEntryTypeFile:
		var body []byte
		if operation.Kind == domainstartup.SemanticOperationSetMode {
			current, found := files[name]
			if !found || current.Directory {
				return errors.New("original registry mode target is unavailable")
			}
			body = current.Body
		} else {
			if readAfter == nil {
				return errors.New("original registry semantic After bytes are unavailable")
			}
			var err error
			body, err = readAfter(operation)
			if err != nil {
				return err
			}
		}
		if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
			return errors.New("original registry semantic After lost integrity")
		}
		files[name] = evidenceregistrystore.OriginalLegacyEntryV1{Mode: operation.After.Mode, Body: append([]byte(nil), body...)}
	default:
		return errors.New("original registry semantic transition is invalid")
	}
	return nil
}

func runtimeRegistryContextsV1(ctx context.Context, core *runtimeChildIdentityStartupV1) ([]domainsecurity.TurnSecurityContext, func(context.Context) error, error) {
	snapshot, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, core.roots)
	if err != nil {
		return nil, nil, err
	}
	_, primaries, err := readRuntimeOriginalPrimaryInventoryV1(ctx, core.roots, snapshot)
	if err != nil {
		return nil, nil, err
	}
	contexts := []domainsecurity.TurnSecurityContext{}
	for _, primary := range primaries {
		raw, present := primary["turns"]
		if !present || raw == nil {
			continue
		}
		turns, ok := raw.([]any)
		if !ok {
			return nil, nil, errors.New("original registry primary turns are invalid")
		}
		for _, rawTurn := range turns {
			turn, ok := rawTurn.(map[string]any)
			if !ok {
				return nil, nil, errors.New("original registry primary turn is invalid")
			}
			value, present := turn["securityContext"]
			if !present || value == nil {
				continue
			}
			frozen, err := domainsecurity.ParseTurnSecurityContext(value)
			if err != nil || frozen.ThreadID != primary["id"] || frozen.TurnID != turn["id"] {
				return nil, nil, errors.Join(errors.New("original registry primary context is detached"), err)
			}
			contexts = append(contexts, frozen)
		}
	}
	revalidate := func(ctx context.Context) error {
		current, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, core.roots)
		if err != nil || !reflect.DeepEqual(snapshot, current) {
			return errors.Join(errors.New("original registry primary denominator changed"), err)
		}
		return nil
	}
	if err := revalidate(ctx); err != nil {
		return nil, nil, err
	}
	return contexts, revalidate, nil
}

func readRuntimeRegistrySemanticInventoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1, saved runtimeOriginalSemanticFilesV1) (_ runtimeOriginalSemanticFilesV1, _ runtimeOriginalSemanticFilesV1, _ []domainsecurity.TurnSecurityContext, _ *runtimeRegistrySemanticObservationV1, resultErr error) {
	if ctx == nil || core == nil || core.verification == nil || core.revalidateKey == nil || scope == nil {
		return nil, nil, nil, nil, errors.New("original registry semantic authority is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return nil, nil, nil, nil, err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	prepared, err := evidenceregistrystore.PrepareRecoveryV2(ctx, filepath.Join(core.roots.DataDir, "private", "evidence-registry"), core.access)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	observation := &runtimeRegistrySemanticObservationV1{prepared: prepared, journal: journal}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	files, err := prepared.SnapshotOriginalFilesV1(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	contexts, revalidateContexts, err := runtimeRegistryContextsV1(ctx, core)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	observation.revalidateContexts = revalidateContexts
	domain, err := observeRuntimeOriginalRegistryDomainV1(ctx, core, files, contexts, false)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	observation.unavailable = domain.unavailable
	physical := runtimeOriginalSemanticFilesV1(files)
	original, final := physical.cloneV1(), physical.cloneV1()
	for index, operation := range journal.OperationsV1() {
		if err := validateRuntimeOriginalSemanticAncestorV1(runtimeRegistrySemanticRootV1, operation); err != nil {
			return nil, nil, nil, nil, err
		}
		name, owned := registrySemanticRelativeV1(operation)
		if !owned {
			continue
		}
		entry, found := physical[name]
		state := runtimeOriginalSemanticStateV1(entry, found)
		if state != operation.Before {
			if index > journal.NextOperationV1() || state != operation.After {
				return nil, nil, nil, nil, errors.New("registry physical state is outside authenticated semantic prefix")
			}
			after, err := journal.PhysicallyAfterV1(ctx, operation)
			if err != nil || !after {
				return nil, nil, nil, nil, errors.Join(errors.New("registry physical state has not reached signed After"), err)
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
					return nil, nil, nil, nil, errors.New("registry semantic original Before bytes are unavailable")
				}
				entry = previous
			}
			entry.Mode = operation.Before.Mode
			original[name] = entry
		default:
			return nil, nil, nil, nil, errors.New("registry semantic original entry type is invalid")
		}
		if err := final.applyV1(operation, name, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return journal.ReadAfterV1(ctx, operation)
		}); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	for _, endpoint := range []runtimeOriginalSemanticFilesV1{original, final} {
		domain, err := observeRuntimeOriginalRegistryDomainV1(ctx, core, endpoint, contexts, true)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		observation.unavailable = observation.unavailable || domain.unavailable
	}
	return original, physical, contexts, observation, nil
}

func (files runtimeOriginalSemanticFilesV1) heldRegistryV1(scope *pendingworkapp.ReportRestartScopeV1, contexts []domainsecurity.TurnSecurityContext) (map[string]string, error) {
	held := map[string]string{}
	for _, frozen := range contexts {
		if !scope.OwnsThread(frozen.ThreadID) {
			continue
		}
		key := domainevidence.EvidenceRegistryProjectionKey(frozen.ThreadID, frozen.TurnID)
		for _, suffix := range []string{".head.json", ".jsonl"} {
			name := ".registry-projections/" + key + suffix
			entry, found := files[name]
			body, _ := json.Marshal(runtimeOriginalSemanticStateV1(entry, found))
			held[name] = string(body)
		}
	}
	for name, entry := range files {
		if entry.Directory {
			continue
		}
		if name == ".registry-authority-index.json" {
			index, err := domainevidence.ParseEvidenceRegistryAuthorityIndex(entry.Body)
			if err != nil {
				return nil, err
			}
			for _, row := range index.Entries {
				if scope.OwnsThread(row.ThreadID) {
					body, err := json.Marshal(row)
					if err != nil {
						return nil, err
					}
					held["index-row:"+row.ThreadID+"\x00"+row.TurnID] = string(body)
				}
			}
		} else if strings.HasPrefix(name, "indexes/") && strings.HasSuffix(name, ".json") {
			index, err := domainevidence.ParseEvidenceRegistryAuthorityIndexV2(entry.Body)
			if err != nil {
				return nil, err
			}
			if scope.OwnsThread(index.Entry.ThreadID) {
				body, _ := json.Marshal(runtimeOriginalSemanticStateV1(entry, true))
				held[name] = string(body)
			}
		} else if (strings.HasPrefix(name, ".registry-capsules/") || strings.HasPrefix(name, "capsules/")) && strings.HasSuffix(name, ".json") {
			capsule, err := domainevidence.ParseEvidenceRegistryAuthorityCapsule(entry.Body)
			if err != nil {
				return nil, err
			}
			if scope.OwnsThread(capsule.SecurityContext.ThreadID) {
				body, _ := json.Marshal(runtimeOriginalSemanticStateV1(entry, true))
				held[name] = string(body)
			}
		} else if strings.HasSuffix(name, ".tmp") {
			// Opaque crash residues do not prove an independent context.
			body, _ := json.Marshal(runtimeOriginalSemanticStateV1(entry, true))
			held[name] = string(body)
		}
	}
	return held, nil
}

func prepareRuntimeRegistrySemanticPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (_ *runtimeRegistrySemanticPreservationV1, resultErr error) {
	original, physical, contexts, observation, err := readRuntimeRegistrySemanticInventoryV1(ctx, core, scope, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	var held map[string]string
	if !observation.unavailable {
		held, err = original.heldRegistryV1(scope, contexts)
		if err != nil {
			return nil, err
		}
	}
	modes := map[string]uint32{}
	for name, entry := range original {
		if entry.Directory || name == ".registry-authority-index.json" || name == ".registry.lock" {
			modes[name] = entry.Mode
		}
	}
	preserved := &runtimeRegistrySemanticPreservationV1{original: original.cloneV1(), recoveryBefore: original.cloneV1(), held: held, sharedModes: modes, unavailable: observation.unavailable}
	for _, operation := range observation.journal.OperationsV1() {
		name, owned := registrySemanticRelativeV1(operation)
		if !owned {
			continue
		}
		if err := physical.applyV1(operation, name, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return observation.journal.ReadAfterV1(ctx, operation)
		}); err != nil {
			return nil, err
		}
	}
	if err := preserved.validateHeldV1(physical, scope, contexts); err != nil {
		return nil, err
	}
	return preserved, nil
}

func (preserved *runtimeRegistrySemanticPreservationV1) validateHeldV1(files runtimeOriginalSemanticFilesV1, scope *pendingworkapp.ReportRestartScopeV1, contexts []domainsecurity.TurnSecurityContext) error {
	if preserved.unavailable {
		if !reflect.DeepEqual(files, preserved.original) {
			return errors.New("semantic candidate changed unavailable original registry owner")
		}
		return nil
	}
	held, err := files.heldRegistryV1(scope, contexts)
	if err != nil || !reflect.DeepEqual(held, preserved.held) {
		return errors.Join(errors.New("semantic registry candidate changed original held inventory"), err)
	}
	for name, mode := range preserved.sharedModes {
		entry, found := files[name]
		if !found || entry.Mode != mode {
			return errors.New("semantic registry candidate changed original shared mode or presence")
		}
	}
	return nil
}

func (preserved runtimeReportRestartPreservationV1) validateRegistrySemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if preserved.registry == nil || preserved.core == nil || preserved.report == nil {
		return errors.New("original registry preservation is unavailable")
	}
	original, candidate, contexts, observation, err := readRuntimeRegistrySemanticInventoryV1(ctx, preserved.core, preserved.report, preserved.registry.recoveryBefore)
	if err != nil {
		return err
	}
	if observation.unavailable != preserved.registry.unavailable {
		return errors.New("original registry domain availability changed")
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
		if resultErr == nil {
			// Retain only the latest fully verified transaction Before. A
			// following independent program may start from the previous Final.
			// The immutable held projection above never changes.
			preserved.registry.recoveryBefore = original.cloneV1()
		}
	}()
	validate := func(files runtimeOriginalSemanticFilesV1) error {
		return preserved.registry.validateHeldV1(files, preserved.report, contexts)
	}
	if err := validate(original); err != nil {
		return err
	}
	// Verify the full authenticated remaining program, even when a caller asks
	// about a single operation. A late bad index must prevent an earlier write.
	for _, operation := range observation.journal.OperationsV1() {
		name, owned := registrySemanticRelativeV1(operation)
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
		if err := validateRuntimeOriginalSemanticAncestorV1(runtimeRegistrySemanticRootV1, operation); err != nil {
			return err
		}
		name, owned := registrySemanticRelativeV1(operation)
		if !owned {
			continue
		}
		if path.Clean(name) != name {
			return errors.New("semantic registry address is invalid")
		}
		if err := candidate.applyV1(operation, name, readAfter); err != nil {
			return err
		}
	}
	if !preserved.registry.unavailable {
		if _, err := parseRuntimeOriginalRegistryInventoryV1(ctx, preserved.core, candidate, contexts); err != nil {
			return err
		}
	}
	return validate(candidate)
}
