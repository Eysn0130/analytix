package server

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"time"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	controlapp "analytix.local/runtime-go/internal/app/control"
	eventrecordingapp "analytix.local/runtime-go/internal/app/eventrecording"
	modelapp "analytix.local/runtime-go/internal/app/model"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	admission "analytix.local/runtime-go/internal/domain/scheduleadmission"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

func cancelPendingDurableSteeringEntries(value any, cancelledAt string, reason string) []any {
	return turnapp.CancelPendingSteeringEntries(value, cancelledAt, reason)
}

func (s *DurableEventSessionStore) AdmitSteeringEntryForContext(
	threadID, turnID, expectedTurnID, expectedContextDigest string,
	entry map[string]any,
) (map[string]any, error) {
	return s.admitScheduleSteeringEntryV1(nil, threadID, turnID, expectedTurnID, expectedContextDigest, entry)
}
func (s *DurableEventSessionStore) admitScheduleSteeringEntryV1(carrier *scheduleAdmissionCarrierV1, threadID, turnID, expectedTurnID, expectedContextDigest string, entry map[string]any) (map[string]any, error) {
	return s.admitHostChildInputSteeringWithScheduleV1(nil, carrier, threadID, turnID, expectedTurnID, expectedContextDigest, entry)
}

func (s *DurableEventSessionStore) admitHostChildInputSteeringV1(child *hostChildInputCarrierV1, threadID, turnID, expectedTurnID, expectedContextDigest string, entry map[string]any) (map[string]any, error) {
	return s.admitHostChildInputSteeringWithScheduleV1(child, nil, threadID, turnID, expectedTurnID, expectedContextDigest, entry)
}

func (s *DurableEventSessionStore) admitHostChildInputSteeringWithScheduleV1(child *hostChildInputCarrierV1, carrier *scheduleAdmissionCarrierV1, threadID, turnID, expectedTurnID, expectedContextDigest string, entry map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(expectedTurnID) != "" && strings.TrimSpace(expectedTurnID) != turnID {
		return nil, errDurableExpectedTurnMismatch
	}
	bound, err := domainsteering.BindPendingEntryV1(entry, expectedContextDigest)
	if err != nil {
		return nil, errors.Join(errDurableSteeringProjectionInvalid, err)
	}
	sealed, err := s.sealSteeringEntryAuthorityV1(bound, expectedContextDigest)
	if err != nil {
		return nil, err
	}
	if sourceID := stringField(entry, "userInputId"); sourceID != "" {
		thread, err := s.readThreadNoLock(threadID)
		if err != nil {
			return nil, err
		}
		previous, err := s.userInputLateReplyFromThreadNoLock(thread, sourceID, stringField(entry, "clientUserMessageId"))
		if err != nil {
			return nil, err
		}
		if previous != nil {
			if previous.State == "discarded" || previous.TurnID != turnID || stringField(previous.Entry, "clientUserMessageId") != stringField(entry, "clientUserMessageId") {
				return nil, errDurableSteeringIDConflict
			}
			admitted := cloneMap(previous.Entry)
			admitted["status"] = "pending"
			for _, key := range []string{"promotedAt", "promotedItemId", "promotionAuthorityKeyId", "promotionAuthorityPublicKey", "promotionAuthoritySignature"} {
				delete(admitted, key)
			}
			if !domainsteering.SamePendingEntryV1(admitted, sealed) {
				return nil, errDurableSteeringReplayMismatch
			}
			return cloneMap(previous.Entry), nil
		}
	}
	var thread map[string]any
	var turns []any
	var turn map[string]any
	if carrier != nil {
		var receipt *admission.ReceiptV1
		thread, receipt, err = s.readScheduleAdmissionNoLockV1(context.Background(), carrier.request)
		if err == nil && receipt != nil {
			return nil, admission.ErrExistingV1
		}
		if err == nil {
			turns, turn, err = domainsteering.CurrentExecutableTurnV1(thread, threadID, turnID, expectedContextDigest)
		}
		if err == nil && (!scheduleExecutionMatchesV1(carrier.request.Execution, turn) || stringField(entry, "clientUserMessageId") != carrier.request.AttemptID || stringField(entry, "admittedAt") != carrier.at.Format(time.RFC3339Nano)) {
			return nil, admission.ErrConflictV1
		}
	} else {
		thread, turns, turn, err = s.currentSteeringTurnNoLock(threadID, turnID, expectedContextDigest)
	}
	if err != nil {
		return nil, err
	}
	if child != nil {
		if carrier != nil || threadID != child.current.ChildThreadID || turnID != child.request.ExpectedChildTurnID || child.current.ID != child.request.ExpectedRunID || stringField(entry, "clientUserMessageId") != child.request.ClientMessageID {
			return nil, errHostChildInputConflictV1
		}
		if receipt, _, markerErr := s.hostChildInputReceiptFromPrimaryV1(thread, child.request); markerErr != nil {
			return nil, markerErr
		} else if receipt != nil {
			return nil, errHostChildInputExistingV1
		}
	}
	if !turnapp.TurnAcceptsSteering(turn) {
		return nil, errDurableTurnNotSteerable
	}
	updated, admitted, changed, err := domainsteering.AdmitEntryToTurnV1(
		threadID, turnID, turn, sealed, expectedContextDigest, s.verifySteeringEntryAuthorityV1,
	)
	if err != nil {
		return nil, err
	}
	if !changed {
		return admitted, nil
	}
	if stringField(admitted, "userInputId") != "" {
		candidate := cloneMap(thread)
		candidateTurns := append([]any(nil), turns...)
		candidateTurns[len(candidateTurns)-1] = updated
		candidate["turns"] = candidateTurns
		if err := s.preflightQuestionAdmissionNoLockV1(candidate, turnID, expectedContextDigest, admitted); err != nil {
			return nil, err
		}
	}
	if carrier != nil {
		marker := admission.NewMarkerV1(carrier.request, turnID, stringField(admitted, "id"), "steer", stringField(admitted, "admittedAt"), expectedContextDigest, stringField(admitted, "text"))
		markers, _ := updated[admission.MarkerFieldV1].([]any)
		updated[admission.MarkerFieldV1] = append(markers, marker.RecordV1())
	}
	if child != nil {
		binding := child.bindingV1(child.current.ID, turnID, stringField(admitted, "id"), "steer", stringField(admitted, "admittedAt"), expectedContextDigest, stringField(admitted, "text"))
		markers, _ := updated[domainjob.HostChildInputMarkerFieldV1].([]any)
		updated[domainjob.HostChildInputMarkerFieldV1] = append(markers, binding.RecordV1())
	}
	turns[len(turns)-1] = updated
	thread["turns"] = turns
	thread["status"] = "running"
	thread["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	if carrier != nil {
		if _, err := s.scheduleReceiptFromPrimaryV1(thread, carrier.request); err != nil {
			return nil, err
		}
		carrier.writeAttempted = true
	}
	if child != nil {
		if receipt, _, markerErr := s.hostChildInputReceiptFromPrimaryV1(thread, child.request); markerErr != nil || receipt == nil {
			return nil, errHostChildInputConflictV1
		}
		child.writeAttempted = true
	}
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		return nil, err
	}
	return admitted, nil
}

func (s *DurableEventSessionStore) PromotePendingSteeringEntriesForContext(
	threadID, turnID, expectedContextDigest string,
) ([]map[string]any, []map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, turns, turn, err := s.currentSteeringTurnNoLock(threadID, turnID, expectedContextDigest)
	if err != nil {
		return nil, nil, err
	}
	updated, entries, items, err := domainsteering.PromoteTurnEntriesV1(
		threadID, turnID, turn, expectedContextDigest, time.Now().UTC().Format(time.RFC3339Nano),
		s.verifySteeringEntryAuthorityV1, s.promoteSteeringEntryAuthorityV1,
	)
	if err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		return nil, nil, nil
	}
	candidate := cloneMap(thread)
	candidateTurns := append([]any(nil), turns...)
	candidateTurns[len(candidateTurns)-1] = updated
	candidate["turns"] = candidateTurns
	if err := s.preflightQuestionPromotionNoLockV1(candidate, turnID, entries, items); err != nil {
		return nil, nil, err
	}
	turns[len(turns)-1] = updated
	thread["turns"] = turns
	thread["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		return nil, nil, err
	}
	return entries, items, nil
}

// PromotePendingSteeringEntryPrefixForContext performs the pending-prefix CAS
// and its durable turn update under the same store mutex. New tail admissions
// may therefore coexist with an earlier exact prefix, while a stale, skipped,
// or reordered expectation cannot promote a different batch.
func (s *DurableEventSessionStore) PromotePendingSteeringEntryPrefixForContext(
	threadID, turnID, expectedContextDigest string,
	expected []domainsteering.PendingEntryExpectationV1,
) ([]map[string]any, []map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, turns, turn, err := s.currentSteeringTurnNoLock(threadID, turnID, expectedContextDigest)
	if err != nil {
		return nil, nil, err
	}
	updated, entries, items, err := domainsteering.PromoteTurnEntryPrefixV1(
		threadID, turnID, turn, expected, expectedContextDigest, time.Now().UTC().Format(time.RFC3339Nano),
		s.verifySteeringEntryAuthorityV1, s.promoteSteeringEntryAuthorityV1,
	)
	if err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		return nil, nil, nil
	}
	candidate := cloneMap(thread)
	candidateTurns := append([]any(nil), turns...)
	candidateTurns[len(candidateTurns)-1] = updated
	candidate["turns"] = candidateTurns
	if err := s.preflightQuestionPromotionNoLockV1(candidate, turnID, entries, items); err != nil {
		return nil, nil, err
	}
	turns[len(turns)-1] = updated
	thread["turns"] = turns
	thread["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		return nil, nil, err
	}
	return entries, items, nil
}

func (s *DurableEventSessionStore) PendingSteeringEntriesForContext(
	threadID, turnID, expectedContextDigest string,
) ([]map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _, turn, err := s.currentSteeringTurnNoLock(threadID, turnID, expectedContextDigest)
	if err != nil {
		return nil, err
	}
	entries, err := domainsteering.PendingEntriesFromTurnV1(
		threadID, turnID, turn, expectedContextDigest, s.verifySteeringEntryAuthorityV1,
	)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *DurableEventSessionStore) currentSteeringTurnNoLock(
	threadID, turnID, expectedContextDigest string,
) (map[string]any, []any, map[string]any, error) {
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return nil, nil, nil, err
	}
	if thread == nil {
		return nil, nil, nil, os.ErrNotExist
	}
	turns, turn, err := domainsteering.CurrentExecutableTurnV1(thread, threadID, turnID, expectedContextDigest)
	if errors.Is(err, domainsteering.ErrTurnNotFound) {
		err = errDurableTurnNotFound
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return thread, turns, turn, nil
}

func (s *DurableEventSessionStore) sealSteeringEntryAuthorityV1(
	entry map[string]any,
	contextDigest string,
) (map[string]any, error) {
	if s.steeringAuthority == nil {
		return nil, errDurableSteeringAuthorityUnavailable
	}
	sealed, err := s.steeringAuthority.Seal(context.Background(), entry, contextDigest)
	if err != nil {
		return nil, errDurableSteeringAuthorityUnavailable
	}
	return sealed, nil
}

func (s *DurableEventSessionStore) verifySteeringEntryAuthorityV1(entry map[string]any, contextDigest string) error {
	if s.steeringAuthority == nil {
		return errDurableSteeringAuthorityUnavailable
	}
	if s.steeringAuthority.Verify(context.Background(), entry, contextDigest) != nil {
		return errDurableSteeringAuthorityUnavailable
	}
	return nil
}

func (s *DurableEventSessionStore) promoteSteeringEntryAuthorityV1(entry map[string]any, contextDigest string) (map[string]any, error) {
	if s.steeringAuthority == nil {
		return nil, errDurableSteeringAuthorityUnavailable
	}
	promoted, err := s.steeringAuthority.Promote(context.Background(), entry, contextDigest)
	if err != nil {
		return nil, errDurableSteeringAuthorityUnavailable
	}
	return promoted, nil
}

// userInputLateReplyRecordV1 is a verified view of the existing steering
// entry. It owns neither a queue nor a persisted delivery state.
type userInputLateReplyRecordV1 struct {
	TurnID, State, CaptureReceiptID string
	Entry                           map[string]any
}

func (s *DurableEventSessionStore) readUserInputLateReplyV1(threadID, sourceInputID, attemptID string) (*userInputLateReplyRecordV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return nil, err
	}
	return s.userInputLateReplyFromThreadNoLock(thread, sourceInputID, attemptID)
}

func (s *DurableEventSessionStore) userInputLateReplyFromThreadNoLock(thread map[string]any, sourceInputID, attemptID string) (*userInputLateReplyRecordV1, error) {
	if thread == nil || stringField(thread, "status") == "deleted" {
		return nil, errDurableSteeringProjectionInvalid
	}
	turns, ok := thread["turns"].([]any)
	if !ok {
		return nil, errDurableSteeringProjectionInvalid
	}
	var selected, active *userInputLateReplyRecordV1
	seenAttempts := map[string]bool{}
	for turnIndex, rawTurn := range turns {
		turn, ok := rawTurn.(map[string]any)
		if !ok {
			return nil, errDurableSteeringProjectionInvalid
		}
		turnID := stringField(turn, "id")
		entries, _ := turn["steering"].([]any)
		for _, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				return nil, errDurableSteeringProjectionInvalid
			}
			source := stringField(entry, "userInputId")
			clientID := stringField(entry, "clientUserMessageId")
			if attemptID != "" && clientID == attemptID && source != sourceInputID {
				return nil, errDurableSteeringIDConflict
			}
			if source != sourceInputID {
				continue
			}
			if seenAttempts[clientID] || entry["id"] != domainsteering.EntryIDV1(turnID, clientID) {
				return nil, errDurableSteeringIDConflict
			}
			seenAttempts[clientID] = true
			frozen, err := turnapp.FrozenSecurityContextForTurn(thread, turnID)
			if err != nil || frozen.ThreadID != stringField(thread, "id") || !domainsecurity.IsSHA256Hex(frozen.ContextDigest) {
				return nil, errDurableSteeringProjectionInvalid
			}
			items, ok := turn["items"].([]any)
			if !ok {
				return nil, errDurableSteeringProjectionInvalid
			}
			var item map[string]any
			matches := 0
			for _, rawItem := range items {
				candidate, ok := rawItem.(map[string]any)
				if !ok {
					return nil, errDurableSteeringProjectionInvalid
				}
				if stringField(candidate, "id") == stringField(entry, "id") || stringField(candidate, "id") == clientID {
					item = candidate
					matches++
				}
			}
			state := ""
			switch stringField(entry, "status") {
			case "pending":
				if matches != 0 || turnIndex != len(turns)-1 || stringField(turn, "status") != "running" || !turnapp.TurnAcceptsSteering(turn) || domainsteering.ValidatePendingEntryForContextV1(entry, frozen.ContextDigest) != nil || s.verifySteeringEntryAuthorityV1(entry, frozen.ContextDigest) != nil {
					return nil, errDurableSteeringProjectionInvalid
				}
				state = "queued"
			case "promoted":
				if matches != 1 {
					return nil, errDurableSteeringProjectionInvalid
				}
				if _, err := domainsteering.NewPromotionCommitV1(stringField(thread, "id"), turnID, frozen.ContextDigest, entry, item, s.verifySteeringEntryAuthorityV1); err != nil {
					return nil, errDurableSteeringProjectionInvalid
				}
				state = "accepted"
			case "cancelled":
				if domainsteering.HasUserInputDiscardAuthorityV1(entry) {
					if matches != 0 || domainsteering.ValidateCancelledUserInputEntryForContextV1(entry, frozen.ContextDigest) != nil || s.verifySteeringEntryAuthorityV1(entry, frozen.ContextDigest) != nil {
						return nil, errDurableSteeringProjectionInvalid
					}
					state = "discarded"
					break
				}
				admitted := cloneMap(entry)
				admitted["status"] = "pending"
				delete(admitted, "cancelledAt")
				delete(admitted, "cancelReason")
				if matches != 0 || !turnapp.IsTerminalStatus(stringField(turn, "status")) || stringField(entry, "cancelledAt") != stringField(turn, "finishedAt") || stringField(entry, "cancelReason") != stringField(turn, "status") || domainsteering.ValidatePendingEntryForContextV1(admitted, frozen.ContextDigest) != nil || s.verifySteeringEntryAuthorityV1(admitted, frozen.ContextDigest) != nil {
					return nil, errDurableSteeringProjectionInvalid
				}
				state = "discarded"
			default:
				return nil, errDurableSteeringProjectionInvalid
			}
			record := &userInputLateReplyRecordV1{TurnID: turnID, State: state, CaptureReceiptID: stringField(entry, "userInputCaptureReceiptId"), Entry: cloneMap(entry)}
			if state != "discarded" {
				if active != nil {
					return nil, errDurableSteeringIDConflict
				}
				active = record
			}
			if attemptID != "" && clientID == attemptID {
				if selected != nil {
					return nil, errDurableSteeringIDConflict
				}
				selected = record
			} else if attemptID == "" {
				if selected == nil {
					selected = record
				} else {
					currentAt, currentErr := time.Parse(time.RFC3339Nano, stringField(entry, "admittedAt"))
					selectedAt, selectedErr := time.Parse(time.RFC3339Nano, stringField(selected.Entry, "admittedAt"))
					if currentErr != nil || selectedErr != nil {
						return nil, errDurableSteeringProjectionInvalid
					}
					if currentAt.After(selectedAt) {
						selected = record
					}
				}
			}
		}
	}
	if attemptID != "" {
		if selected != nil {
			return selected, nil
		}
		if active != nil {
			return nil, errDurableSteeringIDConflict
		}
		return nil, nil
	}
	if active != nil {
		return active, nil
	}
	return selected, nil
}

// The source reservation and stable attempt lookup occur in the same store
// mutex as admission above. No public event is sufficient proof of delivery.
func sameUserInputLateReplyRecordV1(a, b *userInputLateReplyRecordV1) bool {
	return reflect.DeepEqual(a, b)
}

func (s *DurableEventSessionStore) cancelUserInputSteeringEntryAuthorityV1(entry map[string]any, contextDigest string) (map[string]any, error) {
	if s.steeringAuthority == nil {
		return nil, errDurableSteeringAuthorityUnavailable
	}
	cancelled, err := s.steeringAuthority.CancelUserInput(context.Background(), entry, contextDigest)
	if err != nil {
		return nil, errDurableSteeringAuthorityUnavailable
	}
	return cancelled, nil
}

// discardUserInputLateReplyV1 shares the admission/promotion mutex. The Host
// receipt callback supplies the exact, verified source/capture/context binding.
func (s *DurableEventSessionStore) discardUserInputLateReplyV1(ctx context.Context, threadID, sourceInputID, attemptID, targetTurnID, captureReceiptID, contextDigest string, expected domainsteering.PendingEntryExpectationV1) (*userInputLateReplyRecordV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx == nil || ctx.Err() != nil {
		return nil, errDurableSteeringProjectionInvalid
	}
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return nil, err
	}
	previous, err := s.userInputLateReplyFromThreadNoLock(thread, sourceInputID, attemptID)
	if err != nil || previous == nil {
		return nil, errors.Join(err, errDurableSteeringProjectionInvalid)
	}
	if previous.TurnID != targetTurnID || previous.CaptureReceiptID != captureReceiptID || stringField(previous.Entry, "contextDigest") != contextDigest || stringField(previous.Entry, "id") != expected.ID || stringField(previous.Entry, "contentDigest") != expected.ContentDigest {
		return nil, errDurableSteeringReplayMismatch
	}
	if previous.State == "accepted" {
		return nil, domainsteering.ErrEntryPromoted
	}
	if previous.State == "discarded" {
		if domainsteering.ValidateCancelledUserInputEntryForContextV1(previous.Entry, contextDigest) != nil {
			return nil, errDurableSteeringProjectionInvalid
		}
		return previous, nil
	}
	turns, turn, err := domainsteering.CurrentExecutableTurnV1(thread, threadID, targetTurnID, contextDigest)
	if err != nil || !turnapp.TurnAcceptsSteering(turn) {
		return nil, errors.Join(err, errDurableTurnNotSteerable)
	}
	updated, cancelled, changed, err := domainsteering.CancelUserInputEntryInTurnV1(threadID, targetTurnID, turn, expected, sourceInputID, captureReceiptID, contextDigest, time.Now().UTC().Format(time.RFC3339Nano), s.verifySteeringEntryAuthorityV1, s.cancelUserInputSteeringEntryAuthorityV1)
	if err != nil {
		return nil, err
	}
	if !changed {
		return previous, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	turns[len(turns)-1] = updated
	thread["turns"] = turns
	thread["updatedAt"] = stringField(cancelled, "cancelledAt")
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		return nil, err
	}
	return &userInputLateReplyRecordV1{TurnID: targetTurnID, State: "discarded", CaptureReceiptID: captureReceiptID, Entry: cloneMap(cancelled)}, nil
}

func (s *DurableEventSessionStore) preflightQuestionAdmissionNoLockV1(thread map[string]any, turnID, contextDigest string, admitted map[string]any) error {
	if err := threadapp.ValidatePublicHistory(thread); err != nil {
		return errors.Join(errors.New("Question admission history"), err)
	}
	if err := s.preflightQuestionRecoveryNoLockV1(thread); err != nil {
		return errors.Join(errors.New("Question admission recovery"), err)
	}
	draft := controlapp.BuildTurnSteeredEvent(controlapp.TurnSteeredEventInput{Entry: admitted, ThreadID: stringField(thread, "id"), TurnID: turnID, Text: stringField(admitted, "text"), DisplayText: stringField(admitted, "displayText"), ClientUserMessageID: stringField(admitted, "clientUserMessageId")})
	if err := s.preflightQuestionEventsNoLockV1(thread, []map[string]any{draft}); err != nil {
		return errors.Join(errors.New("Question admission event"), err)
	}
	turns, turn, err := domainsteering.CurrentExecutableTurnV1(thread, stringField(thread, "id"), turnID, contextDigest)
	if err != nil {
		return err
	}
	updated, entries, items, err := domainsteering.PromoteTurnEntriesV1(stringField(thread, "id"), turnID, turn, contextDigest, time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC).Format(time.RFC3339Nano), s.verifySteeringEntryAuthorityV1, s.promoteSteeringEntryAuthorityV1)
	if err != nil {
		return err
	}
	candidate := cloneMap(thread)
	turns[len(turns)-1] = updated
	candidate["turns"] = turns
	return s.preflightQuestionPromotionNoLockV1(candidate, turnID, entries, items)
}

func (s *DurableEventSessionStore) preflightQuestionPromotionNoLockV1(thread map[string]any, turnID string, entries, items []map[string]any) error {
	question := false
	for _, entry := range entries {
		question = question || stringField(entry, "userInputId") != ""
	}
	if !question {
		return nil
	}
	if err := threadapp.ValidatePublicHistory(thread); err != nil {
		return errors.Join(errors.New("Question promotion history"), err)
	}
	if err := s.preflightQuestionRecoveryNoLockV1(thread); err != nil {
		return errors.Join(errors.New("Question promotion recovery"), err)
	}
	events := modelapp.BuildPromotedSteering(modelapp.PromotedSteeringInput{ThreadID: stringField(thread, "id"), TurnID: turnID, Entries: entries, Items: items}).ItemCreatedEvents
	if err := s.preflightQuestionEventsNoLockV1(thread, events); err != nil {
		return errors.Join(errors.New("Question promotion event"), err)
	}
	return nil
}

func (s *DurableEventSessionStore) preflightQuestionEventsNoLockV1(thread map[string]any, drafts []map[string]any) error {
	if len(drafts) == 0 {
		return nil
	}
	for _, draft := range drafts {
		if err := domainevent.ValidateQuestionReplyResourceLimitsV1(draft); err != nil {
			return err
		}
	}
	// RecordBatch is the real producer of publication projection, timestamp,
	// sequence and metadata. This preview persists nothing and does not call the
	// store's lock-taking RecordEvent. Longest valid time/int encodings cover
	// metadata growth until actual emission; the adapter rechecks actual bytes.
	_, _, err := eventrecordingapp.RecordBatch(eventrecordingapp.Input{
		Drafts: drafts, Atomic: len(drafts) > 1, PublicationReserved: s.acceptedFinalEvents.PublicationReservedOwnerLocked,
		ReadThread: func(string) (map[string]any, error) { return cloneMap(thread), nil }, ProjectThread: s.caseThreadView,
		NextSequence: func(string) (int, error) { return int(^uint(0)>>1) - len(drafts), nil },
		Now:          func() time.Time { return time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC) }, MissingThread: os.ErrNotExist,
		Persist: func(_ string, events []map[string]any, _ bool) error {
			for _, event := range events {
				if _, err := eventlog.EncodeEventJSONLFrameV1(event); err != nil {
					return err
				}
			}
			return nil
		},
	})
	return err
}
