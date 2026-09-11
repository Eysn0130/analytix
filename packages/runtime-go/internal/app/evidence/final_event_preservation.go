package evidence

import (
	"context"
	"errors"
	"reflect"
	"strings"

	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// These observations retain original history only. They cannot supply a
// publication, terminal, mutation, or provider-history capability.
type PreservedFinalEventThreadV1 struct {
	Primary recoveryport.PrimaryThreadSnapshotV1
	Events  []map[string]any
	CAS     map[string]domainevidence.AcceptedFinalCASObservationV1
}

type PreservedFinalEventInventoryV1 struct {
	Threads    []PreservedFinalEventThreadV1
	Classified FinalAuthorityInventory
	Preserved  []domainevidence.PrivateAcceptedFinalRecord
}

// The composition root must reobserve the complete original held denominator,
// its physical binding and current installation signature on every call.
type FinalEventRestartPreservationV1 interface {
	ObserveOriginalFinalEventInventoryV1(context.Context) (PreservedFinalEventInventoryV1, error)
	ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error
}

func validatePreservedFinalEventInventoryV1(ctx context.Context, input PreservedFinalEventInventoryV1, observer FinalEventRestartPreservationV1) (map[string]bool, map[string]domainevidence.PrivateAcceptedFinalRecord, error) {
	if ctx == nil || observer == nil {
		return nil, nil, errors.New("original final-event observation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	threads := map[string]PreservedFinalEventThreadV1{}
	held := map[string]bool{}
	for _, snapshot := range input.Threads {
		id := snapshot.Primary.ThreadID
		if held[id] || !domainsecurity.IsSHA256Hex(snapshot.Primary.ThreadFileSHA256) || domainthread.ValidatePrimaryIdentityV1(id, snapshot.Primary.Thread) != nil {
			return nil, nil, errors.New("original final-event thread denominator is invalid")
		}
		threads[id], held[id] = snapshot, true
	}
	preserved := map[string]domainevidence.PrivateAcceptedFinalRecord{}
	for _, record := range input.Preserved {
		digest := record.AcceptedFinal.RecordDigest
		if !held[record.SecurityContext.ThreadID] || preserved[digest].AcceptedFinal.RecordDigest != "" || domainevidence.ValidatePrivateAcceptedFinalAuditAuthority(record) != nil {
			return nil, nil, errors.New("original final-event private denominator is invalid")
		}
		preserved[digest] = record
	}
	seen := map[string]bool{}
	winners := map[string]domainevidence.PrivateAcceptedFinalRecord{}
	allowed := map[string]AcceptedFinalEventReconciliationPlan{}
	add := func(record domainevidence.PrivateAcceptedFinalRecord, winner, repair bool) error {
		id, digest := record.SecurityContext.ThreadID, record.AcceptedFinal.RecordDigest
		if !held[id] {
			return nil
		}
		if seen[digest] || !reflect.DeepEqual(preserved[digest], record) {
			return errors.New("original final-event classification is missing, changed or overlapping")
		}
		seen[digest] = true
		snapshot := threads[id]
		observation, ok := snapshot.CAS[record.SecurityContext.TurnID]
		if !ok || domainevidence.ValidateAcceptedFinalCASObservationV1(observation) != nil || observation.ThreadID != id || observation.TurnID != record.SecurityContext.TurnID || observation.ThreadFileSHA256 != snapshot.Primary.ThreadFileSHA256 || observation.FrozenContext != record.SecurityContext {
			return errors.New("original final-event CAS is detached from its private record")
		}
		turn, err := preservedFinalEventTurnV1(snapshot.Primary.Thread, observation.TurnID)
		if err != nil {
			return err
		}
		digestOfTurn, err := domainevidence.AcceptedFinalTurnProjectionSHA256V1(turn)
		if err != nil || digestOfTurn != observation.TurnProjectionSHA256 {
			return errors.Join(errors.New("original final-event CAS turn changed"), err)
		}
		if winner != (observation.HasWinner && reflect.DeepEqual(observation.Winner, record.AcceptedFinal)) {
			return errors.New("original final-event public winner classification changed")
		}
		if repair {
			_, candidate, err := classifyUnresolvedPrivateFinal(record, observation)
			if err != nil || candidate == nil {
				return errors.Join(errors.New("original final-event prepared CAS changed"), err)
			}
		}
		if winner {
			// Audit reconstruction verifies historical bytes without current Fact
			// mutation authority. No resulting plan is returned to a writer.
			plan, err := planAcceptedFinalEventReconciliationFromSnapshot(record, nil, true, snapshot.Primary.Thread, snapshot.Events)
			if err != nil {
				return err
			}
			winners[digest], allowed[digest] = record, plan
		}
		return nil
	}
	for _, records := range [][]domainevidence.PrivateAcceptedFinalRecord{input.Classified.Committed, input.Classified.AuditOnlyPublicWinners} {
		for _, record := range records {
			if err := add(record, true, false); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, records := range [][]domainevidence.PrivateAcceptedFinalRecord{input.Classified.NotCommitted, input.Classified.AuditOnlyNotCommitted} {
		for _, record := range records {
			if err := add(record, false, false); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, repair := range input.Classified.PublicCommitRepairs {
		if err := add(repair.PrivateRecord, false, true); err != nil {
			return nil, nil, err
		}
	}
	if len(seen) != len(preserved) {
		return nil, nil, errors.New("original final-event private denominator is incomplete")
	}
	for id, snapshot := range threads {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if err := validatePreservedFinalEventThreadV1(id, snapshot, winners, allowed, observer); err != nil {
			return nil, nil, err
		}
	}
	return held, winners, ctx.Err()
}

func preservedFinalEventTurnV1(thread map[string]any, id string) (map[string]any, error) {
	for _, turn := range authorityTurns(thread) {
		if authorityString(turn, "id") == id {
			return turn, nil
		}
	}
	return nil, errors.New("original final-event turn is missing")
}

func validatePreservedFinalEventThreadV1(id string, snapshot PreservedFinalEventThreadV1, winners map[string]domainevidence.PrivateAcceptedFinalRecord, allowed map[string]AcceptedFinalEventReconciliationPlan, observer FinalEventRestartPreservationV1) error {
	thread := snapshot.Primary.Thread
	turns := map[string]map[string]any{}
	caseTurns := map[string]bool{}
	for _, turn := range authorityTurns(thread) {
		turnID := authorityString(turn, "id")
		if !domainthread.IsCanonicalRecordID(turnID) || turns[turnID] != nil {
			return errors.New("original final-event turn denominator is invalid")
		}
		turns[turnID] = turn
		if raw := turn["securityContext"]; raw != nil {
			frozen, err := domainsecurity.ParseTurnSecurityContext(raw)
			if err != nil || frozen.ThreadID != id || frozen.TurnID != turnID {
				return errors.Join(errors.New("original final-event context is invalid"), err)
			}
			caseTurns[turnID] = domainsecurity.TurnSecurityContextIsCaseSensitive(frozen)
		}
		if raw := turn["acceptedFinal"]; raw != nil {
			final, err := domainevidence.ParseAcceptedFinalRecord(raw)
			if err != nil || !reflect.DeepEqual(winners[final.RecordDigest].AcceptedFinal, final) {
				return errors.Join(errors.New("original final-event public winner is outside the private denominator"), err)
			}
		} else if caseTurns[turnID] && authorityTerminalStatus(authorityString(turn, "status")) {
			if err := observer.ValidateCaseCompactionAuthorityTurnV1(id, thread, turn); err != nil {
				return err
			}
		}
	}
	if err := validateFinalPublicationEventSequence(snapshot.Events); err != nil {
		return err
	}
	if err := validateInventoryPublicationMarkers(snapshot.Events, allowed); err != nil {
		return err
	}
	for _, event := range snapshot.Events {
		if err := domainevent.ValidatePublicRecord(event); err != nil {
			return err
		}
		markerFields := []string{"publicationCommitId", "acceptedFinalDigest", "publicationEventId", "publicationSlot", "publicationPayloadDigest"}
		claimsManifest := false
		for _, field := range markerFields {
			if _, present := event[field]; present {
				claimsManifest = true
			}
		}
		if claimsManifest {
			for _, field := range markerFields {
				value, ok := event[field].(string)
				if !ok || value == "" || strings.TrimSpace(value) != value {
					return errors.New("original final-event publication marker fields are incomplete or invalid")
				}
			}
		}
		turnID, kind := authorityString(event, "turnId"), authorityString(event, "kind")
		if authorityString(event, "threadId") != id || kind == "" || strings.TrimSpace(kind) != kind || turnID != "" && turns[turnID] == nil {
			return errors.New("original final-event row identity is invalid")
		}
		if authorityString(event, "publicationCommitId") != "" {
			continue
		} // exact historical manifest checked above
		if turns[turnID] == nil {
			if err := appturn.ValidateCaseEventPublication(thread, event); err != nil {
				return err
			}
		}
		if caseTurns[turnID] {
			item, _ := event["item"].(map[string]any)
			if kind == "assistant_text_delta" || kind == "assistant_reasoning_delta" || kind == "agent_reasoning" || kind == "turn_completed" || kind == "turn_failed" || kind == "turn_aborted" || kind == "item_completed" && contracts.StringField(item, "kind") == "assistant_text" {
				return errors.New("original case event lacks its publication manifest")
			}
		}
	}
	return nil
}

func observeFinalEventPreservationV1(ctx context.Context, observer FinalEventRestartPreservationV1, reader AcceptedFinalPublicReader, committed, legacy, audit []domainevidence.PrivateAcceptedFinalRecord) ([]domainevidence.PrivateAcceptedFinalRecord, error) {
	if observer == nil {
		return audit, nil
	}
	input, err := observer.ObserveOriginalFinalEventInventoryV1(ctx)
	if err != nil {
		return nil, err
	}
	held, winners, err := validatePreservedFinalEventInventoryV1(ctx, input, observer)
	if err != nil {
		return nil, err
	}
	ids, err := reader.AllThreadIDs()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if held[id] {
			return nil, errors.New("preserved thread entered executable event inventory")
		}
	}
	for _, records := range [][]domainevidence.PrivateAcceptedFinalRecord{committed, legacy} {
		for _, record := range records {
			if held[record.SecurityContext.ThreadID] {
				return nil, errors.New("preserved final entered executable or quarantine authority")
			}
		}
	}
	filtered := make([]domainevidence.PrivateAcceptedFinalRecord, 0, len(audit))
	seen := map[string]bool{}
	for _, record := range audit {
		if !held[record.SecurityContext.ThreadID] {
			filtered = append(filtered, record)
			continue
		}
		digest := record.AcceptedFinal.RecordDigest
		if seen[digest] || !reflect.DeepEqual(winners[digest], record) {
			return nil, errors.New("preserved audit winner changed or duplicated")
		}
		seen[digest] = true
	}
	return filtered, nil
}

func PreflightAcceptedFinalEventsWithPreservationV1(ctx context.Context, io FinalPublicationEventIO, store appturn.AcceptedFinalCompletionStore, reader AcceptedFinalPublicReader, committed, legacy, audit []domainevidence.PrivateAcceptedFinalRecord, observer FinalEventRestartPreservationV1) ([]AcceptedFinalEventReconciliationPlan, error) {
	if reader == nil {
		return nil, errors.New("final-event inventory reader is unavailable")
	}
	filtered, err := observeFinalEventPreservationV1(ctx, observer, reader, committed, legacy, audit)
	if err != nil {
		return nil, err
	}
	plans, err := PreflightAcceptedFinalEventReconciliationsWithQuarantine(ctx, io, store, reader, committed, legacy, filtered)
	if err != nil {
		return nil, err
	}
	if _, err := observeFinalEventPreservationV1(ctx, observer, reader, committed, legacy, audit); err != nil {
		return nil, err
	}
	return plans, nil
}

func ApplyAcceptedFinalEventsWithPreservationV1(ctx context.Context, io FinalPublicationEventIO, store appturn.AcceptedFinalCompletionStore, reader AcceptedFinalPublicReader, plans []AcceptedFinalEventReconciliationPlan, committed, legacy, audit []domainevidence.PrivateAcceptedFinalRecord, observer FinalEventRestartPreservationV1) error {
	if !io.validForApply() || store == nil {
		return errors.New("final-event apply dependencies are unavailable")
	}
	current, err := PreflightAcceptedFinalEventsWithPreservationV1(ctx, io, store, reader, committed, legacy, audit, observer)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, plans) {
		return errors.New("final-event repair inventory changed before apply")
	}
	guardedIO := io
	guardedIO.AppendEvents = func(ctx context.Context, target appturn.AcceptedFinalCompletionStore, events []map[string]any) ([]map[string]any, error) {
		if _, err := observeFinalEventPreservationV1(ctx, observer, reader, committed, legacy, audit); err != nil {
			return nil, err
		}
		return io.AppendEvents(ctx, target, events)
	}
	for _, plan := range plans {
		if _, err := observeFinalEventPreservationV1(ctx, observer, reader, committed, legacy, audit); err != nil {
			return err
		}
		if err := ApplyAcceptedFinalEventReconciliation(ctx, guardedIO, store, plan); err != nil {
			return err
		}
	}
	_, err = PreflightAcceptedFinalEventsWithPreservationV1(ctx, io, store, reader, committed, legacy, audit, observer)
	return err
}
