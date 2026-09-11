package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	terminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	terminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

const runtimeTerminalSemanticRootV1 = "data/private/turn-terminal-authority"

type runtimeTerminalSemanticPreservationV1 struct {
	heldDigests map[string]string
}

type runtimeTerminalSemanticInventoryV1 struct {
	intents      map[string]domainturnterminal.TurnTerminalIntentV1
	dispositions map[string]domainturnterminal.TurnTerminalDispositionV1
}

type runtimeTerminalSemanticObservationV1 struct {
	prepared *terminalstore.PreparedRecoveryV1
	journal  *persistencefs.AuthenticatedSemanticJournalObservationV1
}

func (observation *runtimeTerminalSemanticObservationV1) Revalidate(ctx context.Context) error {
	if observation == nil || observation.prepared == nil {
		return errors.New("terminal semantic observation is unavailable")
	}
	err := observation.prepared.Revalidate(ctx)
	if observation.journal != nil {
		err = errors.Join(err, observation.journal.Revalidate(ctx))
	}
	return err
}

func (inventory runtimeTerminalSemanticInventoryV1) validateAuthenticatedFinalV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1, journal *persistencefs.AuthenticatedSemanticJournalObservationV1) error {
	operations := journal.OperationsV1()
	// A journal's future repair is not evidence that an arbitrary orphan was
	// produced by this transaction. A currently incomplete pair must contain
	// a canonical physical After from an operation that reached the cursor.
	completedInstall := func(leaf, id string, body []byte) (bool, error) {
		for index, operation := range operations {
			if index > journal.NextOperationV1() {
				break
			}
			if operation.Path == terminalSemanticPathV1(leaf, id) && operation.Kind == domainstartup.SemanticOperationInstallFile && operation.After.Size == int64(len(body)) && operation.After.SHA256 == domainsecurity.SHA256Hex(body) {
				return journal.PhysicallyAfterV1(ctx, operation)
			}
		}
		return false, nil
	}
	for id, disposition := range inventory.dispositions {
		intent, found := inventory.intents[id]
		if found && domainturnterminal.ValidateTurnTerminalDispositionForIntentV1(disposition, intent) == nil {
			continue
		}
		body, err := domainturnterminal.TurnTerminalDispositionV1Bytes(disposition)
		if err != nil {
			return err
		}
		explained, err := completedInstall("dispositions", id, body)
		if err != nil {
			return err
		}
		if found && !explained {
			body, err := domainturnterminal.TurnTerminalIntentV1Bytes(intent)
			if err != nil {
				return err
			}
			explained, err = completedInstall("intents", id, body)
			if err != nil {
				return err
			}
		}
		if !explained {
			return errors.New("terminal partial graph is not explained by an applied signed operation")
		}
	}
	projected := runtimeTerminalSemanticInventoryV1{intents: map[string]domainturnterminal.TurnTerminalIntentV1{}, dispositions: map[string]domainturnterminal.TurnTerminalDispositionV1{}}
	for id, intent := range inventory.intents {
		projected.intents[id] = intent
	}
	for id, disposition := range inventory.dispositions {
		projected.dispositions[id] = disposition
	}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !strings.HasPrefix(operation.Path, runtimeTerminalSemanticRootV1+"/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(operation.Path, runtimeTerminalSemanticRootV1+"/"), "/")
		if len(parts) <= 2 && (operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory) {
			if (parts[0] != "intents" && parts[0] != "dispositions") || (len(parts) == 2 && !domainprivatecas.ValidShardV1(parts[1])) {
				return errors.New("projected terminal directory grammar is invalid")
			}
			continue
		}
		if len(parts) != 3 || (parts[0] != "intents" && parts[0] != "dispositions") {
			return errors.New("projected terminal record grammar is invalid")
		}
		if residue, ok := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1]); ok {
			if residue.Kind != domainprivatecas.ResidueOrdinaryWriteV1 || operation.Kind != domainstartup.SemanticOperationRemoveFile || operation.Before.Type != domainstartup.ManagedEntryTypeFile || operation.After.Type != domainstartup.ManagedEntryTypeAbsent {
				return errors.New("projected terminal residue is outside the signed removal cut")
			}
			continue // No committed graph member; the remaining guard checks ownership.
		}
		id := strings.TrimSuffix(parts[2], ".json")
		if !strings.HasSuffix(parts[2], ".json") || !domainsecurity.IsSHA256Hex(id) || parts[1] != id[:2] {
			return errors.New("projected terminal record address is invalid")
		}
		if err := projected.applyRecordV1(operation, parts[0], id, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return journal.ReadAfterV1(ctx, operation)
		}); err != nil {
			return err
		}
	}
	intents := make([]domainturnterminal.TurnTerminalIntentV1, 0, len(projected.intents))
	dispositions := make([]domainturnterminal.TurnTerminalDispositionV1, 0, len(projected.dispositions))
	for _, intent := range projected.intents {
		intents = append(intents, intent)
	}
	for _, disposition := range projected.dispositions {
		dispositions = append(dispositions, disposition)
	}
	if err := terminalapp.VerifyTrustedSnapshotV1(ctx, intents, dispositions, core.verification); err != nil {
		return err
	}
	originalHeld, err := inventory.heldDigestsV1(scope)
	if err != nil {
		return err
	}
	projectedHeld, err := projected.heldDigestsV1(scope)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(originalHeld, projectedHeld) {
		return terminalapp.ErrRestartPreserved
	}
	return nil
}

func terminalSemanticPathV1(leaf, id string) string {
	return runtimeTerminalSemanticRootV1 + "/" + leaf + "/" + id[:2] + "/" + id + ".json"
}

func readRuntimeTerminalSemanticInventoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (_ runtimeTerminalSemanticInventoryV1, _ *runtimeTerminalSemanticObservationV1, resultErr error) {
	empty := runtimeTerminalSemanticInventoryV1{}
	if ctx == nil || core == nil || core.verification == nil || core.access == nil || core.revalidateKey == nil {
		return empty, nil, errors.New("semantic terminal observation authority is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return empty, nil, err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return empty, nil, err
	}
	prepared, err := terminalstore.PrepareRecoveryV1(ctx, filepath.Join(core.roots.DataDir, "private", "turn-terminal-authority"), core.access)
	if err != nil {
		return empty, nil, err
	}
	observation := &runtimeTerminalSemanticObservationV1{prepared: prepared, journal: journal}
	defer func() { resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx)) }()
	var intents []domainturnterminal.TurnTerminalIntentV1
	var dispositions []domainturnterminal.TurnTerminalDispositionV1
	if journal == nil {
		intents, dispositions, err = prepared.SnapshotInventory(ctx)
	} else {
		intents, dispositions, err = prepared.SnapshotCanonicalRecordsV1(ctx)
	}
	if err != nil {
		return empty, nil, err
	}
	if err := terminalapp.VerifyTrustedRecordSignaturesV1(ctx, intents, dispositions, core.verification); err != nil {
		return empty, nil, err
	}
	inventory := runtimeTerminalSemanticInventoryV1{intents: map[string]domainturnterminal.TurnTerminalIntentV1{}, dispositions: map[string]domainturnterminal.TurnTerminalDispositionV1{}}
	for _, intent := range intents {
		inventory.intents[intent.SecurityContext.ContextDigest] = intent
	}
	for _, disposition := range dispositions {
		inventory.dispositions[disposition.ContextDigest] = disposition
	}
	if journal == nil {
		if err := terminalapp.VerifyTrustedSnapshotV1(ctx, intents, dispositions, core.verification); err != nil {
			return empty, nil, err
		}
	} else if err := inventory.validateAuthenticatedFinalV1(ctx, core, scope, journal); err != nil {
		return empty, nil, err
	}
	return inventory, observation, nil
}

func (inventory runtimeTerminalSemanticInventoryV1) heldDigestsV1(scope *pendingworkapp.ReportRestartScopeV1) (map[string]string, error) {
	contexts := map[string]domainsecurity.TurnSecurityContext{}
	for _, frozen := range scope.Contexts() {
		contexts[frozen.ContextDigest] = frozen
	}
	for id := range inventory.dispositions {
		if _, held := contexts[id]; held {
			if _, found := inventory.intents[id]; !found {
				return nil, errors.New("original held terminal disposition lacks its original intent")
			}
		}
	}
	digests := map[string]string{}
	for id, intent := range inventory.intents {
		if !scope.OwnsThread(intent.SecurityContext.ThreadID) {
			continue
		}
		frozen, found := contexts[id]
		if !found || !reflect.DeepEqual(frozen, intent.SecurityContext) {
			return nil, errors.New("semantic terminal held context is outside the original primary inventory")
		}
		body, err := domainturnterminal.TurnTerminalIntentV1Bytes(intent)
		if err != nil {
			return nil, err
		}
		digests[terminalSemanticPathV1("intents", id)] = domainsecurity.SHA256Hex(body)
		if disposition, found := inventory.dispositions[id]; found {
			if err := domainturnterminal.ValidateTurnTerminalDispositionForIntentV1(disposition, intent); err != nil {
				return nil, err
			}
			body, err := domainturnterminal.TurnTerminalDispositionV1Bytes(disposition)
			if err != nil {
				return nil, err
			}
			digests[terminalSemanticPathV1("dispositions", id)] = domainsecurity.SHA256Hex(body)
		}
	}
	return digests, nil
}

func prepareRuntimeTerminalSemanticPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (*runtimeTerminalSemanticPreservationV1, error) {
	inventory, _, err := readRuntimeTerminalSemanticInventoryV1(ctx, core, scope)
	if err != nil {
		return nil, err
	}
	digests, err := inventory.heldDigestsV1(scope)
	if err != nil {
		return nil, err
	}
	return &runtimeTerminalSemanticPreservationV1{heldDigests: digests}, nil
}

// Every remaining operation is inspected against original reserved context
// addresses, then projected into a complete current-key graph. Held records
// are bound before recovery, so neither disappearance nor a new held prefix
// can be mistaken for the original state.
func (preserved runtimeReportRestartPreservationV1) validateTerminalSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if preserved.terminal == nil || preserved.report == nil {
		return errors.New("original terminal preservation is unavailable")
	}
	inventory, prepared, err := readRuntimeTerminalSemanticInventoryV1(ctx, preserved.core, preserved.report)
	if err != nil {
		return err
	}
	// Revalidate the same physical inventory after the candidate-only check.
	defer func() {
		resultErr = errors.Join(resultErr, prepared.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
	}()
	originalDigests, err := inventory.heldDigestsV1(preserved.report)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(originalDigests, preserved.terminal.heldDigests) {
		return terminalapp.ErrRestartPreserved
	}
	heldPaths := map[string]bool{}
	for _, frozen := range preserved.report.Contexts() {
		for _, leaf := range []string{"intents", "dispositions"} {
			heldPaths[terminalSemanticPathV1(leaf, frozen.ContextDigest)] = true
		}
	}
	originalIntents := make(map[string]domainturnterminal.TurnTerminalIntentV1, len(inventory.intents))
	for id, intent := range inventory.intents {
		originalIntents[id] = intent
	}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if operation.OperationID == noWriteOperationID {
			continue
		}
		for heldPath := range heldPaths {
			if operation.Path == heldPath || strings.HasPrefix(heldPath, operation.Path+"/") &&
				(operation.Kind != domainstartup.SemanticOperationCreateDirectory || operation.Before.Type != domainstartup.ManagedEntryTypeAbsent || operation.After.Type != domainstartup.ManagedEntryTypeDirectory) {
				return terminalapp.ErrRestartPreserved
			}
		}
		if !strings.HasPrefix(operation.Path, runtimeTerminalSemanticRootV1+"/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(operation.Path, runtimeTerminalSemanticRootV1+"/"), "/")
		if len(parts) <= 2 && (operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory) {
			if (parts[0] != "intents" && parts[0] != "dispositions") || (len(parts) == 2 && !domainprivatecas.ValidShardV1(parts[1])) {
				return errors.New("semantic terminal directory grammar is invalid")
			}
			continue
		}
		if len(parts) != 3 || (parts[0] != "intents" && parts[0] != "dispositions") {
			return errors.New("semantic terminal target grammar is invalid")
		}
		if residue, ok := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1]); ok {
			if residue.Kind != domainprivatecas.ResidueOrdinaryWriteV1 || operation.Kind != domainstartup.SemanticOperationRemoveFile || operation.Before.Type != domainstartup.ManagedEntryTypeFile || operation.After.Type != domainstartup.ManagedEntryTypeAbsent {
				return errors.New("semantic terminal residue is outside the authenticated removal cut")
			}
			id := residue.OriginalName[1:65]
			intent, found := originalIntents[id]
			if !found {
				return errors.New("semantic terminal residue lacks an independent trusted intent")
			}
			if heldPaths[terminalSemanticPathV1(parts[0], id)] || preserved.report.OwnsThread(intent.SecurityContext.ThreadID) {
				return terminalapp.ErrRestartPreserved
			}
			continue
		}
		id := strings.TrimSuffix(parts[2], ".json")
		if !strings.HasSuffix(parts[2], ".json") || !domainsecurity.IsSHA256Hex(id) || parts[1] != id[:2] {
			return errors.New("semantic terminal record address is invalid")
		}
		if err := inventory.applyRecordV1(operation, parts[0], id, readAfter); err != nil {
			return err
		}
		if parts[0] == "intents" && operation.Kind == domainstartup.SemanticOperationInstallFile && preserved.report.OwnsThread(inventory.intents[id].SecurityContext.ThreadID) {
			return terminalapp.ErrRestartPreserved
		}
	}
	intents := make([]domainturnterminal.TurnTerminalIntentV1, 0, len(inventory.intents))
	dispositions := make([]domainturnterminal.TurnTerminalDispositionV1, 0, len(inventory.dispositions))
	for _, intent := range inventory.intents {
		intents = append(intents, intent)
	}
	for _, disposition := range inventory.dispositions {
		dispositions = append(dispositions, disposition)
	}
	if err := terminalapp.VerifyTrustedSnapshotV1(ctx, intents, dispositions, preserved.core.verification); err != nil {
		return err
	}
	finalDigests, err := inventory.heldDigestsV1(preserved.report)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(finalDigests, preserved.terminal.heldDigests) {
		return terminalapp.ErrRestartPreserved
	}
	return nil
}

func (inventory runtimeTerminalSemanticInventoryV1) applyRecordV1(operation domainstartup.SemanticStartupOperationV1, leaf, id string, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error)) error {
	switch operation.Kind {
	case domainstartup.SemanticOperationRemoveFile:
		if leaf == "intents" {
			delete(inventory.intents, id)
		} else {
			delete(inventory.dispositions, id)
		}
	case domainstartup.SemanticOperationSetMode:
	case domainstartup.SemanticOperationInstallFile:
		if readAfter == nil {
			return errors.New("semantic terminal after bytes are unavailable")
		}
		body, err := readAfter(operation)
		if err != nil {
			return err
		}
		if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
			return errors.New("semantic terminal after bytes lost integrity")
		}
		if leaf == "intents" {
			intent, err := domainturnterminal.ParseTurnTerminalIntentV1(body)
			if err != nil || intent.SecurityContext.ContextDigest != id {
				return errors.New("semantic terminal intent is noncanonical or misaddressed")
			}
			inventory.intents[id] = intent
		} else {
			disposition, err := domainturnterminal.ParseTurnTerminalDispositionV1(body)
			if err != nil || disposition.ContextDigest != id {
				return errors.New("semantic terminal disposition is noncanonical or misaddressed")
			}
			inventory.dispositions[id] = disposition
		}
	default:
		return errors.New("semantic terminal record transition is invalid")
	}
	return nil
}
