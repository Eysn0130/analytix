package runtimeapp

import (
	"context"
	"errors"
	"strings"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domaintelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

// This adapter exposes only verification. Its private projection index is
// never installed in the runtime and cannot supply a continuation capability.
type runtimeOriginalStoredChildVerifierV1 struct {
	preserved  runtimeReportRestartPreservationV1
	projection *runtimeChildCompletionProjectionV1
}

type runtimeChildCompletionProjectionV1 struct {
	operations         []domainstartup.SemanticStartupOperationV1
	readAfter          func(domainstartup.SemanticStartupOperationV1) ([]byte, error)
	noWriteOperationID string
}

func (projection *runtimeChildCompletionProjectionV1) activeOperationsV1() []domainstartup.SemanticStartupOperationV1 {
	if projection == nil {
		return nil
	}
	result := []domainstartup.SemanticStartupOperationV1{}
	for _, operation := range projection.operations {
		if projection.noWriteOperationID != "" && operation.OperationID == projection.noWriteOperationID {
			continue
		}
		result = append(result, operation)
	}
	return result
}

func requireOriginalChildProofMemberV1(journal *persistencefs.AuthenticatedSemanticJournalObservationV1, path string, size int64, digest string) error {
	for _, operation := range journal.OperationsV1() {
		if operation.Path != path {
			continue
		}
		if operation.Before.Type != domainstartup.ManagedEntryTypeFile || operation.Before.Size != size || operation.Before.SHA256 != digest {
			return errors.New("stored child proof lacks original Before bytes")
		}
	}
	return nil
}

type runtimeOriginalChildParentReaderV1 struct {
	ctx        context.Context
	primaries  *runtimeOriginalPrimaryInventoryV1
	journal    *persistencefs.AuthenticatedSemanticJournalObservationV1
	projection *runtimeChildCompletionProjectionV1
}

func (reader runtimeOriginalChildParentReaderV1) GetThread(id string) (map[string]any, error) {
	if reader.projection != nil {
		body, changed, err := reader.projection.publicAfterV1(reader.primaries, id, "thread.json")
		if err != nil {
			return nil, err
		}
		if changed {
			snapshot, err := finalauthority.ParsePrimaryThreadSnapshotV1(reader.ctx, id, body)
			return snapshot.Thread, err
		}
		snapshot, err := reader.primaries.ReadPrimaryThreadSnapshotV1(reader.ctx, id)
		return snapshot.Thread, err
	}
	entry, exists := reader.primaries.entries[id]
	if !exists {
		return nil, errors.New("stored child parent is outside original inventory")
	}
	if err := requireOriginalChildProofMemberV1(reader.journal, entry.primary.Path, entry.primary.Size, entry.primary.SHA256); err != nil {
		return nil, err
	}
	snapshot, err := reader.primaries.ReadPrimaryThreadSnapshotV1(reader.ctx, id)
	return snapshot.Thread, err
}

func (inventory *runtimeOriginalPrimaryInventoryV1) observeOriginalChildFinalV1(ctx context.Context, journal *persistencefs.AuthenticatedSemanticJournalObservationV1, record domainevidence.PrivateAcceptedFinalRecord) (_ domainevidence.AcceptedFinalCASObservationV1, resultErr error) {
	empty := domainevidence.AcceptedFinalCASObservationV1{}
	threadID, turnID := record.SecurityContext.ThreadID, record.SecurityContext.TurnID
	entry, exists := inventory.entries[threadID]
	if !exists || entry.events.Type != "file" {
		return empty, errors.New("stored child public proof is outside original inventory")
	}
	for _, file := range []persistencefs.EntryRecord{entry.primary, entry.events} {
		if err := requireOriginalChildProofMemberV1(journal, file.Path, file.Size, file.SHA256); err != nil {
			return empty, err
		}
	}
	check := func() error {
		if _, err := inventory.ReadPrimaryThreadSnapshotV1(ctx, threadID); err != nil {
			return err
		}
		digest, err := entry.reader.ReadCommittedEventLogSHA256V1(ctx, threadID)
		if err != nil || digest != entry.events.SHA256 {
			return errors.Join(errors.New("original child event log changed"), err)
		}
		return ctx.Err()
	}
	if err := check(); err != nil {
		return empty, err
	}
	defer func() { resultErr = errors.Join(resultErr, check()) }()
	observations, err := entry.reader.ReadAcceptedFinalCASObservations(ctx, threadID, []string{turnID})
	if err != nil {
		return empty, err
	}
	observation, exists := observations[turnID]
	if !exists || observation.ThreadFileSHA256 != entry.digest {
		return empty, errors.New("stored child CAS lost original primary binding")
	}
	loaded, frontier, err := eventlog.NewStore(entry.eventRoot).ObserveSinceWithFrontier(ctx, threadID, 0)
	if err != nil || len(loaded.Diagnostics) != 0 || !frontier.Exists || frontier.SHA256 != entry.events.SHA256 || frontier.Size != entry.events.Size {
		return empty, errors.Join(errors.New("stored child event proof is not exact"), err)
	}
	if err := validateRuntimeChildPublicationEventsV1(record, loaded.Events); err != nil {
		return empty, err
	}
	return observation, nil
}

func (verifier runtimeOriginalStoredChildVerifierV1) VerifyStoredChildCompletion(ctx context.Context, record domainjob.Record) (resultErr error) {
	core, scope := verifier.preserved.core, verifier.preserved.report
	if ctx == nil || core == nil || scope == nil || record.ChildCompletionReceipt == nil {
		return errors.New("original stored child completion authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	caseObservation, err := readRuntimeCaseThreadSemanticInventoryV1(ctx, core, scope)
	if err != nil {
		return err
	}
	defer func() {
		for _, id := range []string{record.ParentThreadID, record.ChildThreadID} {
			if _, exists := core.primaries.entries[id]; exists {
				_, err := core.primaries.ReadPrimaryThreadSnapshotV1(ctx, id)
				resultErr = errors.Join(resultErr, err)
			}
		}
		resultErr = errors.Join(resultErr, caseObservation.Revalidate(ctx), core.revalidateKey(ctx), scope.RevalidatePrimary(ctx))
	}()
	accepted, acceptedPhysical, acceptedObservation, err := readRuntimeAcceptedFinalSemanticInventoryV1(ctx, core, scope)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, acceptedObservation.Revalidate(ctx)) }()
	terminals, terminalObservation, err := readRuntimeTerminalSemanticInventoryV1(ctx, core, scope)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, terminalObservation.Revalidate(ctx)) }()
	heldBindings, err := runtimeTelemetryHeldBindingsV1(ctx, core, scope)
	if err != nil {
		return err
	}
	telemetry, telemetryPhysical, telemetryObservation, err := readRuntimeTelemetrySemanticInventoryV1(ctx, core, scope, heldBindings)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, telemetryObservation.Revalidate(ctx)) }()
	contexts := caseObservation.originalContexts
	if verifier.projection != nil {
		accepted, telemetry = acceptedPhysical.cloneV1(), telemetryPhysical.cloneV1()
		caseCandidate := caseObservation.physical.cloneV1()
		for _, operation := range verifier.projection.activeOperationsV1() {
			if err := ctx.Err(); err != nil {
				return err
			}
			id, isCase, _, err := caseThreadSemanticAddressV1(operation)
			if err != nil {
				return err
			}
			if isCase {
				if err := caseCandidate.applyV1(ctx, operation, id, verifier.projection.readAfter, core.verification); err != nil {
					return err
				}
			}
			leaf, id, isFinal, _, err := acceptedFinalSemanticAddressV1(operation)
			if err != nil {
				return err
			}
			if isFinal {
				if err := accepted.applyV1(operation, leaf, id, verifier.projection.readAfter); err != nil {
					return err
				}
			}
			_, _, isTelemetry, _, err := telemetrySemanticAddressV1(operation)
			if err != nil {
				return err
			}
			if isTelemetry {
				if err := telemetry.applyV1(operation, verifier.projection.readAfter); err != nil {
					return err
				}
			}
			if strings.HasPrefix(operation.Path, runtimeTerminalSemanticRootV1+"/") {
				parts := strings.Split(strings.TrimPrefix(operation.Path, runtimeTerminalSemanticRootV1+"/"), "/")
				if len(parts) == 3 && (parts[0] == "intents" || parts[0] == "dispositions") && strings.HasSuffix(parts[2], ".json") {
					id := strings.TrimSuffix(parts[2], ".json")
					if !domainsecurity.IsSHA256Hex(id) || parts[1] != id[:2] {
						return errors.New("candidate child terminal address is invalid")
					}
					if err := terminals.applyRecordV1(operation, parts[0], id, verifier.projection.readAfter); err != nil {
						return err
					}
				}
			}
		}
		contexts, err = casethreadapp.VerifyCommittedContextInventoryV1(ctx, caseCandidate.recordsV1(), core.verification)
		if err != nil {
			return err
		}
		if err := accepted.verifyV1(ctx, core.verification, true); err != nil {
			return err
		}
		if err := telemetry.verifyV1(ctx, core.verification, true); err != nil {
			return err
		}
	}
	var final domainevidence.PrivateAcceptedFinalRecord
	var disposition domainevidence.AcceptedFinalDispositionRecord
	found := false
	for id, candidate := range accepted.records {
		candidateDisposition, exists := accepted.dispositions[id]
		if candidate.SecurityContext.ThreadID != record.ChildThreadID || candidate.SecurityContext.TurnID != record.ChildTurnID || !exists || candidateDisposition.State != domainevidence.AcceptedFinalCommitted {
			continue
		}
		if found {
			return errors.New("stored child committed final turn is duplicated")
		}
		final, disposition, found = candidate, candidateDisposition, true
	}
	if !found {
		return errors.New("stored child lacks original committed accepted final")
	}
	contextDigest := final.SecurityContext.ContextDigest
	intent, intentOK := terminals.intents[contextDigest]
	terminal, terminalOK := terminals.dispositions[contextDigest]
	if !intentOK || !terminalOK {
		return errors.New("stored child lacks original terminal-complete pair")
	}
	intentBody, err := domainturnterminal.TurnTerminalIntentV1Bytes(intent)
	if err != nil {
		return err
	}
	terminalBody, err := domainturnterminal.TurnTerminalDispositionV1Bytes(terminal)
	if err != nil {
		return err
	}
	if verifier.projection == nil {
		for leaf, body := range map[string][]byte{"intents": intentBody, "dispositions": terminalBody} {
			if err := requireOriginalChildProofMemberV1(terminalObservation.journal, terminalSemanticPathV1(leaf, contextDigest), int64(len(body)), domainsecurity.SHA256Hex(body)); err != nil {
				return err
			}
		}
	}
	snapshot, _, err := telemetry.snapshotV1()
	if err != nil {
		return err
	}
	var closure domaintelemetry.ProviderTurnClosureV1
	found = false
	for _, candidate := range snapshot.Closures {
		if candidate.ClosureID == terminal.ProviderClosureID {
			if found {
				return errors.New("stored child provider closure is duplicated")
			}
			closure, found = candidate, true
		}
	}
	if !found {
		return errors.New("stored child lacks original provider closure")
	}
	bindingObserver, ok := core.verification.(interface {
		ObserveProviderTurnBindingHMACV1(context.Context, domainsecurity.TurnSecurityContext) (string, error)
	})
	if !ok {
		return errors.New("stored child provider binding observer is unavailable")
	}
	binding, err := bindingObserver.ObserveProviderTurnBindingHMACV1(ctx, final.SecurityContext)
	if err != nil || closure.TurnBindingHMAC != binding {
		return errors.Join(errors.New("stored child provider closure is outside its original context"), err)
	}
	var observation domainevidence.AcceptedFinalCASObservationV1
	if verifier.projection == nil {
		observation, err = core.primaries.observeOriginalChildFinalV1(ctx, caseObservation.journal, final)
	} else {
		observation, err = verifier.projection.observeFinalV1(ctx, core.primaries, final)
	}
	if err != nil {
		return err
	}
	index := gateprojection.NewTrustedFinalProjectionIndex(core.verification)
	if err := index.SeedTerminalComplete(ctx, []gateprojection.TerminalCompleteFinalAuthorityV1{{PrivateFinal: final, Intent: intent, ProviderClosure: closure, PublicObservation: observation, AcceptedFinalDisposition: disposition, TerminalDisposition: terminal}}); err != nil {
		return err
	}
	stored := subagentapp.NewStoredChildCompletionVerifierV1(contexts.CommittedContextV1, index, core.verification, runtimeOriginalChildParentReaderV1{ctx: ctx, primaries: core.primaries, journal: caseObservation.journal, projection: verifier.projection})
	return stored.VerifyStoredChildCompletion(ctx, record)
}
