package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	liveevents "analytix.local/runtime-go/internal/adapters/outbound/liveevents"
	eventrecordingapp "analytix.local/runtime-go/internal/app/eventrecording"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	acceptedfinaleventport "analytix.local/runtime-go/internal/ports/acceptedfinalevent"
	"analytix.local/runtime-go/internal/protocol"
)

func isMutatingG2Route(route protocol.G2RouteReplayCase) bool {
	return route.Method == "PATCH" || route.Method == "POST"
}

func (s *DurableEventSessionStore) SeedFromG2Routes(routes []protocol.G2RouteReplayCase) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, route := range routes {
		if route.ResponseKind != "json" || len(route.Response.Body) == 0 || isMutatingG2Route(route) {
			continue
		}
		switch route.ID {
		case "thread-list-default":
			var list struct {
				Threads []map[string]any `json:"threads"`
			}
			if err := json.Unmarshal(route.Response.Body, &list); err != nil {
				return err
			}
			for _, thread := range list.Threads {
				if _, ok := thread["turns"]; !ok {
					thread["turns"] = []any{}
				}
				if err := s.upsertThreadIfAbsentNoLock(thread, true); err != nil {
					return err
				}
			}
		case "thread-read-detail":
			var thread map[string]any
			if err := json.Unmarshal(route.Response.Body, &thread); err != nil {
				return err
			}
			if err := s.upsertThreadIfAbsentNoLock(thread, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *DurableEventSessionStore) RecordEvent(draft map[string]any) (map[string]any, []string, error) {
	recorded, order, err := s.recordEvents([]map[string]any{draft}, false)
	if err != nil {
		return nil, order, err
	}
	return recorded[0], order, nil
}

func (s *DurableEventSessionStore) RecordEventsAtomic(drafts []map[string]any) ([]map[string]any, []string, error) {
	return s.recordEvents(drafts, true)
}

func (s *DurableEventSessionStore) recordEvents(
	drafts []map[string]any,
	atomic bool,
) ([]map[string]any, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, draft := range drafts {
		if err := s.requireRestartWritableNoLockV1(stringField(draft, "threadId")); err != nil {
			return nil, nil, err
		}
	}
	result, order, err := eventrecordingapp.RecordBatch(eventrecordingapp.Input{
		Drafts: drafts, Atomic: atomic,
		PublicationReserved: s.acceptedFinalEvents.PublicationReservedOwnerLocked,
		ReadThread:          s.readThreadNoLock, ProjectThread: s.caseThreadView,
		BeforeRecord: s.beforeRecordEventHook, NextSequence: s.nextSeqNoLock,
		Persist: s.persistRecordedEventsNoLock, MissingThread: os.ErrNotExist,
	})
	if err != nil {
		return nil, order, err
	}
	s.highestSeqByThread[result.ThreadID] = result.HighestSequence
	if atomic {
		s.publishEventBundleNoLock(result.ThreadID, result.Events)
	} else {
		s.publishEventNoLock(result.ThreadID, result.Events[0])
	}
	return result.Recorded, order, nil
}

func (s *DurableEventSessionStore) nextSeqNoLock(threadID string) (int, error) {
	if err := s.requireRestartWritableNoLockV1(threadID); err != nil {
		return 0, err
	}
	if current, ok := s.highestSeqByThread[threadID]; ok {
		return current + 1, nil
	}
	maxSeq, err := s.eventLog.HighestSeq(threadID)
	if err != nil {
		return 0, err
	}
	s.highestSeqByThread[threadID] = maxSeq
	return maxSeq + 1, nil
}

func (s *DurableEventSessionStore) persistRecordedEventsNoLock(threadID string, events []map[string]any, atomic bool) error {
	if s.acceptedFinalEvents.PublicationReservedOwnerLocked(threadID) {
		return acceptedfinaleventport.ErrPublicationReserved
	}
	return s.persistRecordedEventsUncheckedNoLock(threadID, events, atomic)
}

func (s *DurableEventSessionStore) persistRecordedEventsUncheckedNoLock(threadID string, events []map[string]any, atomic bool) error {
	if err := s.requireRestartWritableNoLockV1(threadID); err != nil {
		return err
	}
	if s.beforePersistEventHook != nil {
		s.beforePersistEventHook()
	}
	err := s.eventLog.AppendEvents(threadID, events, atomic)
	if err != nil {
		if atomic && eventlog.AtomicAppendCommitted(err) {
			if highest, readErr := s.eventLog.HighestSeqAfterCommittedTail(threadID, events); readErr == nil {
				s.highestSeqByThread[threadID] = highest
			} else {
				delete(s.highestSeqByThread, threadID)
			}
		}
		s.eventPersistFailures += 1
		s.lastEventPersistError = err.Error()
		return err
	}
	for _, event := range events {
		s.writeAttempts += 1
		if stringField(event, "kind") == "usage" {
			if err := s.usageIndex.AppendEventOwnerLocked(event); err != nil {
				s.eventPersistFailures += 1
				s.lastEventPersistError = err.Error()
			}
		}
		seq, _ := numericSeq(event["seq"])
		if pending := s.pendingEvents[threadID]; pending != nil {
			delete(pending, seq)
			if len(pending) == 0 {
				delete(s.pendingEvents, threadID)
			}
		}
	}
	return nil
}

// pendingUsageEventsOwnerLocked snapshots the canonical pending usage events
// while the durable store owner lock is held by the usage index adapter.
func (s *DurableEventSessionStore) pendingUsageEventsOwnerLocked(threadID string) []map[string]any {
	threadID = strings.TrimSpace(threadID)
	events := []map[string]any{}
	for pendingThreadID, pending := range s.pendingEvents {
		if threadID != "" && pendingThreadID != threadID {
			continue
		}
		for _, event := range pending {
			if stringField(event, "kind") == "usage" {
				events = append(events, cloneMap(event))
			}
		}
	}
	return events
}

func (s *DurableEventSessionStore) SubscribeEvents(threadID string) (<-chan map[string]any, func()) {
	ch := make(chan map[string]any, 1024)
	s.mu.Lock()
	if s.subscribers == nil {
		s.subscribers = map[string]map[chan map[string]any]struct{}{}
	}
	if s.subscribers[threadID] == nil {
		s.subscribers[threadID] = map[chan map[string]any]struct{}{}
	}
	s.subscribers[threadID][ch] = struct{}{}
	s.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			s.mu.Lock()
			if subscribers := s.subscribers[threadID]; subscribers != nil {
				if _, subscribed := subscribers[ch]; subscribed {
					delete(subscribers, ch)
					close(ch)
					if len(subscribers) == 0 {
						delete(s.subscribers, threadID)
					}
				}
			}
			s.mu.Unlock()
		})
	}
	return ch, unsubscribe
}

func (s *DurableEventSessionStore) publishEventNoLock(threadID string, event map[string]any) {
	if s.requireRestartWritableNoLockV1(threadID) != nil {
		return
	}
	s.publishOrders = append(s.publishOrders, []string{"persist", "publish"})
	for ch := range s.subscribers[threadID] {
		select {
		case ch <- cloneMap(event):
		default:
		}
	}
}

// publishEventBundleNoLock gives each live subscriber all events or none. A
// subscriber without enough buffered capacity is disconnected before any
// event in the bundle is enqueued, so reconnect/replay is its only recovery
// path and a high-risk accepted final cannot be observed as a live prefix.
func (s *DurableEventSessionStore) publishEventBundleNoLock(threadID string, events []map[string]any) {
	if s.requireRestartWritableNoLockV1(threadID) != nil {
		return
	}
	if len(events) == 0 {
		return
	}
	liveevents.BroadcastBundleNoPrefix(s.subscribers[threadID], events, cloneMap)
	if len(s.subscribers[threadID]) == 0 {
		delete(s.subscribers, threadID)
	}
	for range events {
		s.publishOrders = append(s.publishOrders, []string{"persist", "publish"})
	}
}

func (s *DurableEventSessionStore) publishAcceptedFinalBatchNoLock(
	threadID string,
	batch domainevent.AcceptedFinalDeliveryBatchV2,
) {
	if s.requireRestartWritableNoLockV1(threadID) != nil {
		return
	}
	event := domainevent.AcceptedFinalDeliveryBatchV2Map(batch)
	if event == nil || threadID != batch.ThreadID {
		return
	}
	liveevents.BroadcastBundleNoPrefix(s.subscribers[threadID], []map[string]any{event}, cloneMap)
	if len(s.subscribers[threadID]) == 0 {
		delete(s.subscribers, threadID)
	}
	for range batch.Events {
		s.publishOrders = append(s.publishOrders, []string{"persist", "publish"})
	}
}

func (s *DurableEventSessionStore) AppendRawEventLine(threadID string, line string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireRestartWritableNoLockV1(threadID); err != nil {
		return err
	}
	if s.acceptedFinalEvents.PublicationReservedOwnerLocked(threadID) {
		return acceptedfinaleventport.ErrPublicationReserved
	}
	var candidate map[string]any
	if json.Unmarshal([]byte(line), &candidate) == nil && candidate != nil {
		thread, err := s.readThreadNoLock(threadID)
		if err != nil {
			return err
		}
		if thread == nil {
			return os.ErrNotExist
		}
		thread, err = s.caseThreadView(threadID, thread)
		if err != nil {
			return err
		}
		if err := turnapp.ValidateGenericTerminalEventPathForThreadV1(thread, candidate); err != nil {
			return err
		}
		if err := turnapp.ValidateExactCaseEventProjection(thread, candidate); err != nil {
			return err
		}
	}
	seq, ok, err := s.eventLog.AppendRawLine(threadID, line)
	if err != nil {
		return err
	}
	s.writeAttempts += 1
	if ok && seq > s.highestSeqByThread[threadID] {
		s.highestSeqByThread[threadID] = seq
	}
	return nil
}

func (s *DurableEventSessionStore) LoadEventsSince(threadID string, afterSeq int) (DurableLoadEventsResult, error) {
	return s.LoadEventsSinceContext(context.Background(), threadID, afterSeq)
}

func (s *DurableEventSessionStore) LoadEventsSinceContext(ctx context.Context, threadID string, afterSeq int) (DurableLoadEventsResult, error) {
	if ctx == nil {
		return DurableLoadEventsResult{}, errors.New("event replay context is required")
	}
	if err := ctx.Err(); err != nil {
		return DurableLoadEventsResult{}, err
	}
	s.mu.Lock()
	s.eventReplayReadCount += 1
	pending := eventlog.SnapshotPendingEvents(s.pendingEvents[threadID], afterSeq)
	s.mu.Unlock()

	result, err := s.eventLog.LoadSinceContext(ctx, threadID, afterSeq)
	if err != nil {
		return DurableLoadEventsResult{}, err
	}
	return eventlog.MergePendingEvents(afterSeq, result, pending), nil
}

// LoadPublicEventsSince expands a cursor that lands inside an accepted-final
// manifest back to the manifest's first event. The SSE adapter then emits the
// manifest as one transport batch and advances the public cursor only to its
// final sequence.
func (s *DurableEventSessionStore) LoadPublicEventsSince(threadID string, afterSeq int) (DurableLoadEventsResult, error) {
	return s.LoadPublicEventsSinceContext(context.Background(), threadID, afterSeq)
}

func (s *DurableEventSessionStore) LoadPublicEventsSinceContext(ctx context.Context, threadID string, afterSeq int) (DurableLoadEventsResult, error) {
	result, err := s.LoadEventsSinceContext(ctx, threadID, 0)
	if err != nil {
		return result, err
	}
	if len(result.Diagnostics) != 0 {
		result.Events = nil
		return result, errors.New("public event replay contains invalid durable records")
	}
	filtered, err := domainevent.AcceptedFinalReplayEventsAfter(result.Events, afterSeq)
	if err != nil {
		result.Events = nil
		return result, err
	}
	result.Events = filtered
	return result, nil
}

func (s *DurableEventSessionStore) LoadEventsSinceNoLock(threadID string, afterSeq int) (DurableLoadEventsResult, error) {
	result, err := s.loadEventsSinceFileNoLock(threadID, afterSeq)
	if err != nil {
		return DurableLoadEventsResult{}, err
	}
	return s.mergePendingEventsNoLock(threadID, afterSeq, result), nil
}

func (s *DurableEventSessionStore) loadEventsSinceFileNoLock(threadID string, afterSeq int) (DurableLoadEventsResult, error) {
	s.eventReplayReadCount += 1
	return s.eventLog.LoadSince(threadID, afterSeq)
}

func (s *DurableEventSessionStore) loadEventsSinceFileWithFrontierNoLock(
	threadID string,
	afterSeq int,
) (DurableLoadEventsResult, eventlog.EventLogFrontierV1, error) {
	s.eventReplayReadCount += 1
	return s.eventLog.LoadSinceWithFrontier(threadID, afterSeq)
}

func (s *DurableEventSessionStore) mergePendingEventsNoLock(threadID string, afterSeq int, result DurableLoadEventsResult) DurableLoadEventsResult {
	return eventlog.MergePendingEvents(afterSeq, result, eventlog.SnapshotPendingEvents(s.pendingEvents[threadID], afterSeq))
}

func (s *DurableEventSessionStore) HighestSeq(threadID string) (int, error) {
	s.mu.Lock()
	if current, ok := s.highestSeqByThread[threadID]; ok {
		for seq := range s.pendingEvents[threadID] {
			if seq > current {
				current = seq
			}
		}
		s.highestSeqByThread[threadID] = current
		s.mu.Unlock()
		return current, nil
	}
	pending := eventlog.SnapshotPendingEvents(s.pendingEvents[threadID], 0)
	s.mu.Unlock()

	maxSeq, err := s.eventLog.HighestSeq(threadID)
	if err != nil {
		return 0, err
	}
	for _, event := range pending {
		if seq, ok := numericSeq(event["seq"]); ok && seq > maxSeq {
			maxSeq = seq
		}
	}
	s.mu.Lock()
	if current, ok := s.highestSeqByThread[threadID]; ok && current > maxSeq {
		maxSeq = current
	}
	for seq := range s.pendingEvents[threadID] {
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	s.highestSeqByThread[threadID] = maxSeq
	s.mu.Unlock()
	return maxSeq, nil
}

func (s *DurableEventSessionStore) highestSeqNoLock(threadID string) (int, error) {
	maxSeq, err := s.eventLog.HighestSeq(threadID)
	if err != nil {
		return 0, err
	}
	for seq := range s.pendingEvents[threadID] {
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	if current, ok := s.highestSeqByThread[threadID]; ok && current > maxSeq {
		maxSeq = current
	}
	s.highestSeqByThread[threadID] = maxSeq
	return maxSeq, nil
}

func (s *DurableEventSessionStore) NewlineTerminated(threadID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.eventLog.NewlineTerminated(threadID)
}
