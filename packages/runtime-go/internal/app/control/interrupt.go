package control

import (
	"context"
	"errors"
	"strings"
	"time"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type TerminalItemEventsInput struct {
	ThreadID string
	TurnID   string
	Items    []map[string]any
}

type TurnAbortedEventInput struct {
	ThreadID              string
	TurnID                string
	Discard               bool
	Cancelled             bool
	CancelledPendingGates int
}

type InterruptAcceptedResponseInput struct {
	ThreadID  string
	TurnID    string
	Status    string
	Discard   bool
	Cancelled bool
}

type InterruptStore interface {
	appturn.FailureStore
	GetThread(string) (map[string]any, error)
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

type InterruptActiveTurnInput struct {
	Context                  context.Context
	Store                    InterruptStore
	ThreadID                 string
	TurnID                   string
	Discard                  bool
	Cancelled                bool
	TerminalItems            []map[string]any
	TerminalEvents           []map[string]any
	FinishedAt               string
	GateCancellations        []PendingGateCancellation
	GateCancellationsSettled bool
	GateCancellationReason   string
	CaseFinalizer            evidenceapp.CasePublicationFinalizer
	CaseContext              domainsecurity.TurnSecurityContext
	AcceptedAt               time.Time
}

type InterruptActiveTurnResult struct {
	Status      string
	Changed     bool
	Cancelled   bool
	RecordError error
	Response    map[string]any
}

func AbortedTurnFields(discard bool) map[string]any {
	return map[string]any{"discard": discard}
}

func TerminalItemCompletedEventsForTurn(threadID, turnID string, items []map[string]any) []map[string]any {
	return TerminalItemCompletedEvents(TerminalItemEventsInput{ThreadID: threadID, TurnID: turnID, Items: items})
}

func TerminalItemCompletedEvents(input TerminalItemEventsInput) []map[string]any {
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := strings.TrimSpace(input.TurnID)
	if threadID == "" || turnID == "" || len(input.Items) == 0 {
		return nil
	}
	events := make([]map[string]any, 0, len(input.Items))
	for _, item := range input.Items {
		if item == nil {
			continue
		}
		events = append(events, map[string]any{
			"kind":     "item_completed",
			"threadId": threadID,
			"turnId":   turnID,
			"itemId":   strings.TrimSpace(stringField(item, "id")),
			"item":     contracts.CloneMap(item),
		})
	}
	return events
}

func TurnAbortedEvent(threadID, turnID string, discard bool, cancelled bool, cancelledPendingGates int) map[string]any {
	return BuildTurnAbortedEvent(TurnAbortedEventInput{
		ThreadID:              threadID,
		TurnID:                turnID,
		Discard:               discard,
		Cancelled:             cancelled,
		CancelledPendingGates: cancelledPendingGates,
	})
}

func BuildTurnAbortedEvent(input TurnAbortedEventInput) map[string]any {
	return map[string]any{
		"kind":                  "turn_aborted",
		"threadId":              strings.TrimSpace(input.ThreadID),
		"turnId":                strings.TrimSpace(input.TurnID),
		"status":                "aborted",
		"discard":               input.Discard,
		"cancelled":             input.Cancelled,
		"cancelledPendingGates": input.CancelledPendingGates,
	}
}

func InterruptAcceptedResponseForTurn(threadID, turnID string, status string, discard bool, cancelled bool) map[string]any {
	return InterruptAcceptedResponse(InterruptAcceptedResponseInput{
		ThreadID:  threadID,
		TurnID:    turnID,
		Status:    status,
		Discard:   discard,
		Cancelled: cancelled,
	})
}

func InterruptAcceptedResponse(input InterruptAcceptedResponseInput) map[string]any {
	return map[string]any{
		"threadId":  strings.TrimSpace(input.ThreadID),
		"turnId":    strings.TrimSpace(input.TurnID),
		"status":    strings.TrimSpace(input.Status),
		"discard":   input.Discard,
		"cancelled": input.Cancelled,
	}
}

func InterruptActiveTurn(input InterruptActiveTurnInput) (InterruptActiveTurnResult, error) {
	caseRequired := domainsecurity.TurnSecurityContextIsCaseSensitive(input.CaseContext)
	cancelled := input.Cancelled || len(input.GateCancellations) > 0
	status, err := "", error(nil)
	changed := false
	var caseIntent *domainevidence.TerminalPublicationIntent
	if caseRequired {
		if input.CaseFinalizer == nil {
			return InterruptActiveTurnResult{}, errors.New("case publication finalizer is unavailable")
		}
		ctx := input.Context
		if ctx == nil {
			ctx = context.Background()
		}
		existingStatus, existingTerminal, inspectErr := inspectInterruptTerminal(input.Store, input.ThreadID, input.TurnID)
		if inspectErr != nil {
			return InterruptActiveTurnResult{}, inspectErr
		}
		if existingTerminal {
			status = existingStatus
			if status == "aborted" {
				recovery, ok := input.CaseFinalizer.(evidenceapp.CaseInterruptPublicationAuthority)
				if !ok || recovery == nil {
					return InterruptActiveTurnResult{}, errors.New("case interrupt recovery authority is unavailable")
				}
				intent, recovered, recoverErr := recovery.ReconcileCommittedInterrupt(ctx, input.Store, input.ThreadID, input.TurnID)
				if recoverErr != nil {
					return InterruptActiveTurnResult{}, recoverErr
				}
				if recovered {
					caseIntent = &intent
				}
			}
		} else {
			accepted, finalizeErr := evidenceapp.PersistCaseTerminalBoundary(ctx, input.CaseFinalizer, evidenceapp.PersistCaseBoundaryInput{
				Store: input.Store, Context: input.CaseContext, TerminalReason: evidenceapp.TerminalCancel,
				ThreadID: input.ThreadID, TurnID: input.TurnID, AcceptedAt: input.AcceptedAt, Discard: input.Discard,
				Cancelled: cancelled, CancelledGates: len(input.GateCancellations),
			})
			status, changed, err = accepted.Persistence.Status, accepted.Persistence.Changed, finalizeErr
			if finalizeErr == nil {
				caseIntent = &accepted.PublicationIntent
			}
		}
	} else {
		existingStatus, existingTerminal, inspectErr := inspectInterruptTerminal(input.Store, input.ThreadID, input.TurnID)
		if inspectErr != nil {
			return InterruptActiveTurnResult{}, inspectErr
		}
		if existingTerminal {
			status = existingStatus
			if _, reconcileErr := input.Store.RecordGeneralTerminalEventBundle(input.ThreadID, input.TurnID); reconcileErr != nil {
				return InterruptActiveTurnResult{}, reconcileErr
			}
		} else {
			finishedAt := strings.TrimSpace(input.FinishedAt)
			if finishedAt == "" {
				at := input.AcceptedAt.UTC()
				if at.IsZero() {
					at = time.Now().UTC()
				}
				finishedAt = at.Format(time.RFC3339Nano)
			}
			committed, commitErr := appturn.CommitGeneralFailureTerminal(appturn.PersistFailureInput{
				Store: input.Store, SecurityContext: input.CaseContext, TerminalReason: "cancel",
				ThreadID: input.ThreadID, TurnID: input.TurnID, FinishedAt: finishedAt,
				Failure: domainfailure.New(domainfailure.CodeTurnCancelled, nil),
				Interrupt: &appturn.GeneralTerminalInterruptMetadata{
					Discard: input.Discard, Cancelled: cancelled, CancelledPendingGates: len(input.GateCancellations),
				},
			})
			status, changed, err = committed.Status, committed.Changed, commitErr
		}
	}
	if err != nil {
		return InterruptActiveTurnResult{}, err
	}
	appliedDiscard, appliedCancelled, err := persistedInterruptMetadata(
		input.Store, input.ThreadID, input.TurnID, status, caseIntent,
	)
	if err != nil {
		return InterruptActiveTurnResult{}, err
	}
	var recordErrs []error
	if changed && !input.GateCancellationsSettled {
		for _, event := range PendingGateCancellationEvents(input.GateCancellations, firstNonEmptyInterruptString(input.GateCancellationReason, "turn_aborted")) {
			if _, _, err := input.Store.RecordEvent(event); err != nil {
				recordErrs = append(recordErrs, err)
			}
		}
	}
	return InterruptActiveTurnResult{
		Status:      status,
		Changed:     changed,
		Cancelled:   appliedCancelled,
		RecordError: errors.Join(recordErrs...),
		Response:    InterruptAcceptedResponseForTurn(input.ThreadID, input.TurnID, status, appliedDiscard, appliedCancelled),
	}, nil
}

func inspectInterruptTerminal(store InterruptStore, threadID, turnID string) (string, bool, error) {
	thread, err := store.GetThread(strings.TrimSpace(threadID))
	if err != nil {
		return "", false, err
	}
	status, found, terminal, err := appturn.InspectTerminalAuthorityV1(thread, strings.TrimSpace(turnID))
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, errors.New("interrupt turn is unavailable")
	}
	return status, terminal, nil
}

func persistedInterruptMetadata(
	store InterruptStore,
	threadID, turnID, status string,
	caseIntent *domainevidence.TerminalPublicationIntent,
) (discard, cancelled bool, err error) {
	if strings.TrimSpace(status) != "aborted" {
		return false, false, nil
	}
	thread, err := store.GetThread(strings.TrimSpace(threadID))
	if err != nil {
		return false, false, err
	}
	validatedStatus, found, terminal, err := appturn.InspectTerminalAuthorityV1(thread, turnID)
	if err != nil || !found || !terminal || validatedStatus != "aborted" {
		if err == nil {
			err = errors.New("persisted interrupt terminal authority is unavailable")
		}
		return false, false, err
	}
	for _, rawTurn := range interruptRecordList(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if strings.TrimSpace(stringField(turn, "id")) != strings.TrimSpace(turnID) {
			continue
		}
		if strings.TrimSpace(stringField(turn, "status")) != "aborted" {
			return false, false, errors.New("interrupt terminal status changed after persistence")
		}
		if turn["acceptedFinal"] == nil {
			metadata, resolveErr := appturn.ResolveCommittedGeneralInterruptMetadataV1(thread, turnID)
			if resolveErr != nil {
				return false, false, resolveErr
			}
			return metadata.Discard, metadata.Cancelled, nil
		}
		if caseIntent == nil {
			return false, false, errors.New("case interrupt private terminal intent is unavailable")
		}
		record, parseErr := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
		if parseErr != nil || record.TerminalReason != string(evidenceapp.TerminalCancel) ||
			domainevidence.ValidateTerminalPublicationIntent(*caseIntent, record.TerminalReason) != nil ||
			caseIntent.TerminalStatus != "aborted" ||
			!interruptProjectionMatchesIntent(turn, *caseIntent) {
			return false, false, errors.New("case interrupt terminal intent is inconsistent")
		}
		return caseIntent.Discard, caseIntent.Cancelled, nil
	}
	return false, false, errors.New("persisted interrupt turn is unavailable")
}

func interruptRecordList(value any) []any {
	switch records := value.(type) {
	case []any:
		return records
	case []map[string]any:
		out := make([]any, len(records))
		for index := range records {
			out[index] = records[index]
		}
		return out
	default:
		return nil
	}
}

func interruptProjectionMatchesIntent(turn map[string]any, intent domainevidence.TerminalPublicationIntent) bool {
	discard, discardOK := turn["discard"].(bool)
	cancelled, cancelledOK := turn["cancelled"].(bool)
	count, countOK := nonNegativeInterruptCount(turn["cancelledPendingGates"])
	return discardOK && cancelledOK && countOK && discard == intent.Discard && cancelled == intent.Cancelled &&
		count == intent.CancelledPendingGates && (count == 0 || cancelled)
}

func nonNegativeInterruptCount(value any) (int, bool) {
	switch count := value.(type) {
	case int:
		return count, count >= 0
	case int64:
		return int(count), count >= 0 && int64(int(count)) == count
	case float64:
		integer := int(count)
		return integer, count >= 0 && count == float64(integer)
	default:
		return 0, false
	}
}

func firstNonEmptyInterruptString(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}
