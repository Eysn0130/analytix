package server

import (
	"errors"
	"fmt"
	"os"
	"strings"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	acceptedfinaleventport "analytix.local/runtime-go/internal/ports/acceptedfinalevent"
)

func (s *DurableEventSessionStore) RecordGeneralTerminalEventBundle(threadID, turnID string) ([]map[string]any, error) {
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	if s == nil || threadID == "" || turnID == "" || safeDurableID(threadID) != threadID {
		return nil, turnapp.WithGeneralTerminalDetailV1(errors.New("general terminal event bundle identity is invalid"), turnapp.GeneralTerminalDetailIdentityV1)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.acceptedFinalEvents.PublicationReservedOwnerLocked(threadID) {
		return nil, turnapp.WithGeneralTerminalDetailV1(acceptedfinaleventport.ErrPublicationReserved, turnapp.GeneralTerminalDetailReservedV1)
	}
	return s.recordGeneralTerminalEventBundleNoLock(threadID, turnID)
}

func (s *DurableEventSessionStore) recordGeneralTerminalEventBundleNoLock(threadID, turnID string) ([]map[string]any, error) {
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return nil, turnapp.WithGeneralTerminalDetailV1(err, turnapp.GeneralTerminalDetailPrimaryV1)
	}
	if thread == nil {
		return nil, turnapp.WithGeneralTerminalDetailV1(os.ErrNotExist, turnapp.GeneralTerminalDetailPrimaryV1)
	}
	if stringField(thread, "id") != threadID {
		return nil, turnapp.WithGeneralTerminalDetailV1(errors.New("general terminal event thread identity is invalid"), turnapp.GeneralTerminalDetailPrimaryV1)
	}
	thread, err = s.caseThreadView(threadID, thread)
	if err != nil {
		return nil, turnapp.WithGeneralTerminalDetailV1(err, turnapp.GeneralTerminalDetailViewV1)
	}
	loaded, appendFrontier, err := s.loadEventsSinceFileWithFrontierNoLock(threadID, 0)
	if err != nil || len(loaded.Diagnostics) != 0 {
		return nil, turnapp.WithGeneralTerminalDetailV1(errors.Join(err, errors.New("general terminal event log is not replayable")), turnapp.GeneralTerminalDetailLogV1)
	}
	entries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(thread, loaded.Events)
	if err != nil {
		return nil, turnapp.WithGeneralTerminalDetailV1(err, turnapp.GeneralTerminalDetailInventoryV1)
	}
	entry, found := turnapp.GeneralTerminalPublicationEntryForTurnV1(entries, turnID)
	if !found {
		return nil, turnapp.WithGeneralTerminalDetailV1(errors.New("general terminal turn has no canonical publication outbox"), turnapp.GeneralTerminalDetailInventoryV1)
	}
	for _, candidate := range entries {
		if candidate.TurnID == turnID {
			break
		}
		if candidate.State != turnapp.GeneralTerminalPublicationCompleteV1 {
			return nil, turnapp.WithGeneralTerminalDetailV1(errors.New("general terminal publication cannot skip an earlier unsettled archive commit"), turnapp.GeneralTerminalDetailEarlierUnsettledV1)
		}
	}

	initiatedAppend := false
	appendErr := error(nil)
	if entry.State == turnapp.GeneralTerminalPublicationMissingV1 {
		highest, err := exactHighestGeneralTerminalSeqV1(threadID, loaded.Events)
		if err != nil {
			return nil, turnapp.WithGeneralTerminalDetailV1(err, turnapp.GeneralTerminalDetailFrontierV1)
		}
		bundle, err := turnapp.PrepareGeneralTerminalEventBundleV1(thread, turnID, entry.Commit, highest+1)
		if err != nil {
			return nil, turnapp.WithGeneralTerminalDetailV1(err, turnapp.GeneralTerminalDetailPrepareV1)
		}
		if s.beforePersistEventHook != nil {
			s.beforePersistEventHook()
		}
		initiatedAppend = true
		if s.generalTerminalAtomicAppend != nil {
			appendErr = s.generalTerminalAtomicAppend(threadID, bundle)
		} else {
			appendErr = s.eventLog.AppendEventsAtomicAtFrontier(threadID, bundle, appendFrontier)
		}
		if appendErr != nil && !eventlog.AtomicAppendCommitted(appendErr) {
			appendErr = turnapp.WithGeneralTerminalDetailV1(appendErr, turnapp.GeneralTerminalDetailAppendV1)
			s.recordGeneralTerminalPersistFailureNoLock(appendErr)
			return nil, appendErr
		}

		loaded, err = s.loadEventsSinceFileNoLock(threadID, 0)
		if err != nil || len(loaded.Diagnostics) != 0 {
			err = turnapp.WithGeneralTerminalDetailV1(errors.Join(appendErr, err, errors.New("general terminal atomic append readback is not replayable")), turnapp.GeneralTerminalDetailReadbackV1)
			s.recordGeneralTerminalPersistFailureNoLock(err)
			return nil, err
		}
		entries, err = turnapp.PreflightGeneralTerminalPublicationInventoryV1(thread, loaded.Events)
		if err != nil {
			err = turnapp.WithGeneralTerminalDetailV1(errors.Join(appendErr, err), turnapp.GeneralTerminalDetailExactBundleV1)
			s.recordGeneralTerminalPersistFailureNoLock(err)
			return nil, err
		}
		entry, found = turnapp.GeneralTerminalPublicationEntryForTurnV1(entries, turnID)
		if !found || entry.State != turnapp.GeneralTerminalPublicationCompleteV1 {
			err = turnapp.WithGeneralTerminalDetailV1(errors.Join(appendErr, errors.New("general terminal atomic append did not commit the exact bundle")), turnapp.GeneralTerminalDetailExactBundleV1)
			s.recordGeneralTerminalPersistFailureNoLock(err)
			return nil, err
		}
	}
	if entry.State != turnapp.GeneralTerminalPublicationCompleteV1 {
		return nil, turnapp.WithGeneralTerminalDetailV1(errors.New("general terminal event bundle remains incomplete"), turnapp.GeneralTerminalDetailExactBundleV1)
	}

	highest, err := exactHighestGeneralTerminalSeqV1(threadID, loaded.Events)
	if err != nil {
		return nil, turnapp.WithGeneralTerminalDetailV1(err, turnapp.GeneralTerminalDetailFrontierV1)
	}
	s.highestSeqByThread[threadID] = highest
	if initiatedAppend {
		s.writeAttempts += len(entry.Events)
		for _, event := range entry.Events {
			seq, _ := numericSeq(event["seq"])
			if pending := s.pendingEvents[threadID]; pending != nil {
				delete(pending, seq)
			}
		}
		if len(s.pendingEvents[threadID]) == 0 {
			delete(s.pendingEvents, threadID)
		}
	}
	if err := s.settleGeneralTerminalDerivedStateNoLock(entry); err != nil {
		err = turnapp.WithGeneralTerminalDetailV1(err, turnapp.GeneralTerminalDetailUsageSettleV1)
		s.recordGeneralTerminalPersistFailureNoLock(err)
		return nil, err
	}
	if err := s.publishGeneralTerminalBundleNoLock(threadID, entry.Events); err != nil {
		return nil, turnapp.WithGeneralTerminalDetailV1(err, turnapp.GeneralTerminalDetailLivePublicationV1)
	}
	out := make([]map[string]any, 0, len(entry.Events))
	for _, event := range entry.Events {
		out = append(out, cloneMap(event))
	}
	return out, nil
}

func (s *DurableEventSessionStore) publishGeneralTerminalBundleNoLock(threadID string, events []map[string]any) error {
	marked := 0
	for _, event := range events {
		eventID := strings.TrimSpace(stringField(event, "generalTerminalEventId"))
		if eventID == "" || stringField(event, "threadId") != threadID {
			return errors.New("general terminal live publication manifest is invalid")
		}
		if s.eventLog.PublicationMarked(eventlog.PublicationNamespaceGeneralTerminal, eventID) {
			marked++
		}
	}
	if marked != 0 && marked != len(events) {
		return errors.New("general terminal live publication is partial")
	}
	if marked == len(events) {
		return nil
	}
	for _, event := range events {
		s.eventLog.MarkPublication(eventlog.PublicationNamespaceGeneralTerminal, stringField(event, "generalTerminalEventId"))
	}
	s.publishEventBundleNoLock(threadID, events)
	return nil
}

func exactHighestGeneralTerminalSeqV1(threadID string, events []map[string]any) (int, error) {
	highest := 0
	seen := map[int]bool{}
	for _, event := range events {
		seq, ok := numericSeq(event["seq"])
		if !ok || seq <= 0 || seen[seq] || strings.TrimSpace(stringField(event, "threadId")) != threadID {
			return 0, errors.New("general terminal event frontier is invalid")
		}
		seen[seq] = true
		if seq > highest {
			highest = seq
		}
	}
	return highest, nil
}

func (s *DurableEventSessionStore) settleGeneralTerminalDerivedStateNoLock(
	entry turnapp.GeneralTerminalPublicationInventoryEntryV1,
) error {
	if entry.State != turnapp.GeneralTerminalPublicationCompleteV1 {
		return errors.New("general terminal derived state requires a complete publication bundle")
	}
	usageEvent := turnapp.GeneralTerminalUsageEventV1(entry.Events)
	if usageEvent == nil {
		return nil
	}
	return s.usageIndex.SettleTerminalEventOwnerLocked(usageEvent)
}

func (s *DurableEventSessionStore) recordGeneralTerminalPersistFailureNoLock(err error) {
	if err == nil {
		return
	}
	s.eventPersistFailures++
	s.lastEventPersistError = fmt.Sprintf("general terminal publication: %v", err)
}
