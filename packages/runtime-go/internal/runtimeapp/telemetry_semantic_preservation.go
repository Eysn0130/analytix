package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"

	telemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	telemetryapp "analytix.local/runtime-go/internal/app/cachetelemetry"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domaintelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const runtimeTelemetrySemanticRootV1 = "data/private/provider-cache-telemetry"

type runtimeTelemetrySemanticPreservationV1 struct {
	heldBindings map[string]bool
	heldDigests  map[string]string
}

type runtimeTelemetrySemanticInventoryV1 map[string][]byte

type runtimeTelemetrySemanticObservationV1 struct {
	prepared *telemetrystore.PreparedRecoveryV1
	journal  *persistencefs.AuthenticatedSemanticJournalObservationV1
}

func (observation *runtimeTelemetrySemanticObservationV1) Revalidate(ctx context.Context) error {
	if observation == nil || observation.prepared == nil {
		return errors.New("telemetry semantic observation is unavailable")
	}
	err := observation.prepared.Revalidate(ctx)
	if observation.journal != nil {
		err = errors.Join(err, observation.journal.Revalidate(ctx))
	}
	return err
}

func telemetrySemanticPathV1(leaf, id string) string {
	return runtimeTelemetrySemanticRootV1 + "/" + leaf + "/" + id[:2] + "/" + id + ".json"
}

func runtimeTelemetryHeldBindingsV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (map[string]bool, error) {
	if ctx == nil || core == nil || scope == nil || core.revalidateKey == nil {
		return nil, errors.New("original telemetry context authority is unavailable")
	}
	observer, ok := core.verification.(interface {
		ObserveProviderTurnBindingHMACV1(context.Context, domainsecurity.TurnSecurityContext) (string, error)
	})
	if !ok {
		return nil, errors.New("fixed-purpose original telemetry binding observer is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return nil, err
	}
	bindings := map[string]bool{}
	for _, frozen := range scope.Contexts() {
		binding, err := observer.ObserveProviderTurnBindingHMACV1(ctx, frozen)
		if err != nil {
			return nil, err
		}
		if !domainsecurity.IsSHA256Hex(binding) {
			return nil, errors.New("original telemetry binding is invalid")
		}
		bindings[binding] = true
	}
	if err := errors.Join(core.revalidateKey(ctx), scope.RevalidatePrimary(ctx)); err != nil {
		return nil, err
	}
	return bindings, nil
}

func telemetrySemanticAddressV1(operation domainstartup.SemanticStartupOperationV1) (leaf, id string, record, residue bool, err error) {
	if operation.Path == runtimeTelemetrySemanticRootV1 {
		if operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory {
			return "", "", false, false, nil
		}
		return "", "", false, false, errors.New("semantic telemetry owner transition is invalid")
	}
	if !strings.HasPrefix(operation.Path, runtimeTelemetrySemanticRootV1+"/") {
		return "", "", false, false, nil
	}
	parts := strings.Split(strings.TrimPrefix(operation.Path, runtimeTelemetrySemanticRootV1+"/"), "/")
	validLeaf := parts[0] == "attempts" || parts[0] == "settlements" || parts[0] == "turn-closures"
	if len(parts) <= 2 && (operation.Before.Type == domainstartup.ManagedEntryTypeDirectory || operation.After.Type == domainstartup.ManagedEntryTypeDirectory) {
		if !validLeaf || len(parts) == 2 && !domainprivatecas.ValidShardV1(parts[1]) {
			return "", "", false, false, errors.New("semantic telemetry directory grammar is invalid")
		}
		return "", "", false, false, nil
	}
	if len(parts) != 3 || !validLeaf {
		return "", "", false, false, errors.New("semantic telemetry target grammar is invalid")
	}
	if residue, ok := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1]); ok {
		if residue.Kind != domainprivatecas.ResidueOrdinaryWriteV1 || operation.Kind != domainstartup.SemanticOperationRemoveFile || operation.Before.Type != domainstartup.ManagedEntryTypeFile || operation.After.Type != domainstartup.ManagedEntryTypeAbsent {
			return "", "", false, false, errors.New("semantic telemetry residue is outside the signed removal cut")
		}
		return parts[0], residue.OriginalName[1:65], false, true, nil
	}
	id = strings.TrimSuffix(parts[2], ".json")
	if !strings.HasSuffix(parts[2], ".json") || !domainsecurity.IsSHA256Hex(id) || parts[1] != id[:2] {
		return "", "", false, false, errors.New("semantic telemetry address is invalid")
	}
	return parts[0], id, true, false, nil
}

func (inventory runtimeTelemetrySemanticInventoryV1) snapshotV1() (telemetrystore.RecordSnapshotV1, map[string]string, error) {
	snapshot, bindings := telemetrystore.RecordSnapshotV1{}, map[string]string{}
	for path, body := range inventory {
		leaf, id, record, _, err := telemetrySemanticAddressV1(domainstartup.SemanticStartupOperationV1{Path: path})
		if err != nil || !record {
			return snapshot, nil, errors.Join(errors.New("telemetry candidate address is invalid"), err)
		}
		switch leaf {
		case "attempts":
			intent, err := domaintelemetry.ParseProviderAttemptIntentV1(body)
			canonical, canonicalErr := domaintelemetry.ProviderAttemptIntentV1Bytes(intent)
			if err != nil || canonicalErr != nil || intent.IntentID != id || !bytes.Equal(body, canonical) {
				return snapshot, nil, errors.New("telemetry semantic intent is noncanonical or misaddressed")
			}
			snapshot.Intents = append(snapshot.Intents, intent)
			bindings[path] = intent.TurnBindingHMAC
		case "settlements":
			settlement, err := domaintelemetry.ParseProviderAttemptSettlementV1(body)
			canonical, canonicalErr := domaintelemetry.ProviderAttemptSettlementV1Bytes(settlement)
			if err != nil || canonicalErr != nil || settlement.IntentID != id || !bytes.Equal(body, canonical) {
				return snapshot, nil, errors.New("telemetry semantic settlement is noncanonical or misaddressed")
			}
			snapshot.Settlements = append(snapshot.Settlements, settlement)
			bindings[path] = settlement.TurnBindingHMAC
		case "turn-closures":
			closure, err := domaintelemetry.ParseProviderTurnClosureV1(body)
			canonical, canonicalErr := domaintelemetry.ProviderTurnClosureV1Bytes(closure)
			if err != nil || canonicalErr != nil || closure.TurnBindingHMAC != id || !bytes.Equal(body, canonical) {
				return snapshot, nil, errors.New("telemetry semantic closure is noncanonical or misaddressed")
			}
			snapshot.Closures = append(snapshot.Closures, closure)
			bindings[path] = closure.TurnBindingHMAC
		}
	}
	return snapshot, bindings, nil
}

func (inventory runtimeTelemetrySemanticInventoryV1) verifyV1(ctx context.Context, authority authorityport.Authority, complete bool) error {
	snapshot, _, err := inventory.snapshotV1()
	if err != nil {
		return err
	}
	if err := telemetryapp.VerifyTrustedRecordSignaturesV1(ctx, snapshot.Intents, snapshot.Settlements, snapshot.Closures, authority); err != nil {
		return err
	}
	if complete {
		return telemetrystore.ValidateProviderTelemetryInventoryV1(snapshot)
	}
	return nil
}

func (inventory runtimeTelemetrySemanticInventoryV1) heldDigestsV1(held map[string]bool) (map[string]string, error) {
	_, bindings, err := inventory.snapshotV1()
	if err != nil {
		return nil, err
	}
	digests := map[string]string{}
	for path, binding := range bindings {
		if held[binding] {
			digests[path] = domainsecurity.SHA256Hex(inventory[path])
		}
	}
	return digests, nil
}

func (inventory runtimeTelemetrySemanticInventoryV1) cloneV1() runtimeTelemetrySemanticInventoryV1 {
	copy := runtimeTelemetrySemanticInventoryV1{}
	for path, body := range inventory {
		copy[path] = body
	}
	return copy
}

func (inventory runtimeTelemetrySemanticInventoryV1) applyV1(operation domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error)) error {
	switch operation.Kind {
	case domainstartup.SemanticOperationRemoveFile:
		delete(inventory, operation.Path)
	case domainstartup.SemanticOperationSetMode:
	case domainstartup.SemanticOperationInstallFile:
		if readAfter == nil {
			return errors.New("telemetry semantic After bytes are unavailable")
		}
		body, err := readAfter(operation)
		if err != nil {
			return err
		}
		if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
			return errors.New("telemetry semantic After bytes lost integrity")
		}
		inventory[operation.Path] = append([]byte(nil), body...)
		if _, _, err := (runtimeTelemetrySemanticInventoryV1{operation.Path: body}).snapshotV1(); err != nil {
			return err
		}
	default:
		return errors.New("telemetry semantic record transition is invalid")
	}
	return nil
}

// Original and final graphs are validated independently. A staged settlement
// may explain a signed retry cut but never repairs an arbitrary original gap.
func readRuntimeTelemetrySemanticInventoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1, held map[string]bool) (_ runtimeTelemetrySemanticInventoryV1, _ runtimeTelemetrySemanticInventoryV1, _ *runtimeTelemetrySemanticObservationV1, resultErr error) {
	if ctx == nil || core == nil || core.verification == nil || core.revalidateKey == nil || scope == nil {
		return nil, nil, nil, errors.New("telemetry semantic authority is unavailable")
	}
	if err := core.revalidateKey(ctx); err != nil {
		return nil, nil, nil, err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return nil, nil, nil, err
	}
	prepared, err := telemetrystore.PrepareRecoveryV1(ctx, filepath.Join(core.roots.DataDir, "private", "provider-cache-telemetry"), core.access)
	if err != nil {
		return nil, nil, nil, err
	}
	observation := &runtimeTelemetrySemanticObservationV1{prepared: prepared, journal: journal}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	snapshot, err := prepared.SnapshotCanonicalRecordsV1(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	physical := runtimeTelemetrySemanticInventoryV1{}
	for _, intent := range snapshot.Intents {
		body, err := domaintelemetry.ProviderAttemptIntentV1Bytes(intent)
		if err != nil {
			return nil, nil, nil, err
		}
		physical[telemetrySemanticPathV1("attempts", intent.IntentID)] = body
	}
	for _, settlement := range snapshot.Settlements {
		body, err := domaintelemetry.ProviderAttemptSettlementV1Bytes(settlement)
		if err != nil {
			return nil, nil, nil, err
		}
		physical[telemetrySemanticPathV1("settlements", settlement.IntentID)] = body
	}
	for _, closure := range snapshot.Closures {
		body, err := domaintelemetry.ProviderTurnClosureV1Bytes(closure)
		if err != nil {
			return nil, nil, nil, err
		}
		physical[telemetrySemanticPathV1("turn-closures", closure.TurnBindingHMAC)] = body
	}
	if err := physical.verifyV1(ctx, core.verification, false); err != nil {
		return nil, nil, nil, err
	}
	original, final := physical.cloneV1(), physical.cloneV1()
	for index, operation := range journal.OperationsV1() {
		_, _, record, _, err := telemetrySemanticAddressV1(operation)
		if err != nil {
			return nil, nil, nil, err
		}
		if !record {
			continue
		}
		body, present := physical[operation.Path]
		switch operation.Before.Type {
		case domainstartup.ManagedEntryTypeAbsent:
			if present {
				if index > journal.NextOperationV1() || operation.Kind != domainstartup.SemanticOperationInstallFile || int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
					return nil, nil, nil, errors.New("telemetry physical addition is not an applied signed operation")
				}
				after, err := journal.PhysicallyAfterV1(ctx, operation)
				if err != nil || !after {
					return nil, nil, nil, errors.Join(errors.New("telemetry physical addition has not reached signed After"), err)
				}
			}
			delete(original, operation.Path)
		case domainstartup.ManagedEntryTypeFile:
			if !present || int64(len(body)) != operation.Before.Size || domainsecurity.SHA256Hex(body) != operation.Before.SHA256 {
				return nil, nil, nil, errors.New("telemetry semantic original Before bytes are unavailable")
			}
		default:
			return nil, nil, nil, errors.New("telemetry semantic original record type is invalid")
		}
		if err := final.applyV1(operation, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
			return journal.ReadAfterV1(ctx, operation)
		}); err != nil {
			return nil, nil, nil, err
		}
	}
	for _, inventory := range []runtimeTelemetrySemanticInventoryV1{original, final} {
		if err := inventory.verifyV1(ctx, core.verification, true); err != nil {
			return nil, nil, nil, err
		}
	}
	originalHeld, err := original.heldDigestsV1(held)
	if err != nil {
		return nil, nil, nil, err
	}
	finalHeld, err := final.heldDigestsV1(held)
	if err != nil || !reflect.DeepEqual(originalHeld, finalHeld) {
		return nil, nil, nil, errors.Join(errors.New("telemetry semantic candidate changed original hold"), err)
	}
	return original, physical, observation, nil
}

func prepareRuntimeTelemetrySemanticPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1, scope *pendingworkapp.ReportRestartScopeV1) (*runtimeTelemetrySemanticPreservationV1, error) {
	held, err := runtimeTelemetryHeldBindingsV1(ctx, core, scope)
	if err != nil {
		return nil, err
	}
	original, _, _, err := readRuntimeTelemetrySemanticInventoryV1(ctx, core, scope, held)
	if err != nil {
		return nil, err
	}
	digests, err := original.heldDigestsV1(held)
	if err != nil {
		return nil, err
	}
	return &runtimeTelemetrySemanticPreservationV1{heldBindings: held, heldDigests: digests}, nil
}

func (preserved runtimeReportRestartPreservationV1) validateTelemetrySemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if preserved.telemetry == nil || preserved.core == nil || preserved.report == nil {
		return errors.New("original telemetry preservation is unavailable")
	}
	held, err := runtimeTelemetryHeldBindingsV1(ctx, preserved.core, preserved.report)
	if err != nil || !reflect.DeepEqual(held, preserved.telemetry.heldBindings) {
		return errors.Join(errors.New("original telemetry context bindings changed"), err)
	}
	original, candidate, observation, err := readRuntimeTelemetrySemanticInventoryV1(ctx, preserved.core, preserved.report, held)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
	}()
	digests, err := original.heldDigestsV1(held)
	if err != nil || !reflect.DeepEqual(digests, preserved.telemetry.heldDigests) {
		return errors.Join(errors.New("original telemetry held inventory changed"), err)
	}
	snapshot, originalBindings, err := original.snapshotV1()
	if err != nil {
		return err
	}
	heldPaths := map[string]bool{}
	for path := range digests {
		heldPaths[path] = true
	}
	for binding := range held {
		heldPaths[telemetrySemanticPathV1("turn-closures", binding)] = true
	}
	for _, intent := range snapshot.Intents {
		if held[intent.TurnBindingHMAC] {
			heldPaths[telemetrySemanticPathV1("settlements", intent.IntentID)] = true
		}
	}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if operation.OperationID == noWriteOperationID {
			continue
		}
		for path := range heldPaths {
			if operation.Path == path || strings.HasPrefix(path, operation.Path+"/") && (operation.Kind != domainstartup.SemanticOperationCreateDirectory || operation.Before.Type != domainstartup.ManagedEntryTypeAbsent || operation.After.Type != domainstartup.ManagedEntryTypeDirectory) {
				return errors.New("semantic operation mutates original held telemetry authority")
			}
		}
		leaf, id, record, residue, err := telemetrySemanticAddressV1(operation)
		if err != nil {
			return err
		}
		if residue {
			binding := originalBindings[telemetrySemanticPathV1(leaf, id)]
			if binding == "" && leaf == "settlements" {
				binding = originalBindings[telemetrySemanticPathV1("attempts", id)]
			}
			if binding == "" || held[binding] {
				return errors.New("telemetry residue lacks independent original authority")
			}
		}
		if !record {
			continue
		}
		if err := candidate.applyV1(operation, readAfter); err != nil {
			return err
		}
		if operation.Kind == domainstartup.SemanticOperationInstallFile {
			_, bindings, err := (runtimeTelemetrySemanticInventoryV1{operation.Path: candidate[operation.Path]}).snapshotV1()
			if err != nil {
				return err
			}
			if held[bindings[operation.Path]] {
				return errors.New("semantic operation installs new held telemetry authority")
			}
		}
	}
	if err := candidate.verifyV1(ctx, preserved.core.verification, true); err != nil {
		return err
	}
	finalHeld, err := candidate.heldDigestsV1(held)
	if err != nil || !reflect.DeepEqual(finalHeld, preserved.telemetry.heldDigests) {
		return errors.Join(errors.New("telemetry semantic candidate changed original hold"), err)
	}
	return nil
}
