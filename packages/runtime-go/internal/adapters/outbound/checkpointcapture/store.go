package checkpointcapture

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	"analytix.local/runtime-go/internal/contracts"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	checkpointcaptureport "analytix.local/runtime-go/internal/ports/checkpointcapture"
)

type Store struct {
	locker         sync.Locker
	eventLog       *eventlog.Store
	highest        map[string]int
	readThread     func(string) (map[string]any, error)
	caseThreadView func(string, map[string]any) (map[string]any, error)
	project        func(map[string]any, map[string]any) (map[string]any, error)
	loadEvents     func(string, int) (eventlog.LoadResult, error)
	nextSeq        func(string) (int, error)
	persist        func(string, []map[string]any, bool) error
	publish        func(string, map[string]any)
}

func NewStore(locker sync.Locker, eventLog *eventlog.Store, highest map[string]int, readThread func(string) (map[string]any, error), caseThreadView func(string, map[string]any) (map[string]any, error), project func(map[string]any, map[string]any) (map[string]any, error), loadEvents func(string, int) (eventlog.LoadResult, error), nextSeq func(string) (int, error), persist func(string, []map[string]any, bool) error, publish func(string, map[string]any)) *Store {
	return &Store{locker: locker, eventLog: eventLog, highest: highest, readThread: readThread, caseThreadView: caseThreadView, project: project, loadEvents: loadEvents, nextSeq: nextSeq, persist: persist, publish: publish}
}

func (s *Store) ReconcileCheckpointCapturedEvent(request checkpointcaptureport.ReconcileRequest) error {
	identity, err := domaincheckpointref.ParseCapturedEventIdentity(request.Draft)
	if err != nil {
		return err
	}
	if !s.available(request.Publish) {
		return errors.New("checkpoint captured event persistence is unavailable")
	}
	s.locker.Lock()
	defer s.locker.Unlock()

	if contracts.SafeRecordID(identity.ThreadID) != identity.ThreadID {
		return errors.New("checkpoint captured event thread identity is invalid")
	}
	thread, err := s.readThread(identity.ThreadID)
	if err != nil {
		return err
	}
	if thread == nil {
		return os.ErrNotExist
	}
	if exactString(thread["id"]) != identity.ThreadID {
		return errors.New("checkpoint captured event thread authority is invalid")
	}
	thread, err = s.caseThreadView(identity.ThreadID, thread)
	if err != nil {
		return err
	}
	expected, err := s.project(thread, request.Draft)
	if err != nil {
		return err
	}
	projectedIdentity, err := domaincheckpointref.ParseCapturedEventIdentity(expected)
	if err != nil || projectedIdentity != identity {
		return errors.New("checkpoint captured event projection changed its identity")
	}
	persisted, found, err := s.findExact(identity, expected)
	if err != nil {
		return err
	}
	if found {
		return s.publishOnce(request.Publish, identity, persisted)
	}
	if !request.AllowWrite {
		return errors.New("checkpoint captured event is missing during read-only activation")
	}

	event := contracts.CloneMap(expected)
	event["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
	nextSeq, err := s.nextSeq(identity.ThreadID)
	if err != nil {
		return err
	}
	event["seq"] = float64(nextSeq)
	appendErr := s.persist(identity.ThreadID, []map[string]any{event}, true)
	persisted, found, readbackErr := s.findExact(identity, expected)
	if readbackErr == nil && found {
		if seq, ok := contracts.NumericSeq(persisted["seq"]); ok && seq > s.highest[identity.ThreadID] {
			s.highest[identity.ThreadID] = seq
		}
		return s.publishOnce(request.Publish, identity, persisted)
	}
	if appendErr != nil {
		return errors.Join(errors.New("checkpoint captured atomic append did not reconcile"), appendErr, readbackErr)
	}
	if readbackErr != nil {
		return readbackErr
	}
	return errors.New("checkpoint captured atomic append readback is missing or conflicting")
}

func (s *Store) available(publish bool) bool {
	return s != nil && s.locker != nil && s.eventLog != nil && s.highest != nil && s.readThread != nil &&
		s.caseThreadView != nil && s.project != nil && s.loadEvents != nil && s.nextSeq != nil && s.persist != nil && (!publish || s.publish != nil)
}

func (s *Store) findExact(identity domaincheckpointref.CapturedEventIdentity, expected map[string]any) (map[string]any, bool, error) {
	result, err := s.loadEvents(identity.ThreadID, 0)
	if err != nil {
		return nil, false, err
	}
	if len(result.Diagnostics) != 0 {
		return nil, false, errors.New("checkpoint captured event log contains invalid records")
	}
	count, conflict, persisted := domaincheckpointref.ExactCapturedEventCount(result.Events, identity.EventID, expected)
	if conflict || count > 1 {
		return nil, false, errors.New("checkpoint captured event identity conflicts with durable history")
	}
	return persisted, count == 1, nil
}

func (s *Store) publishOnce(enabled bool, identity domaincheckpointref.CapturedEventIdentity, event map[string]any) error {
	if !enabled || event == nil || !s.eventLog.MarkPublication(eventlog.PublicationNamespaceCheckpointCapture, identity.EventID) {
		return nil
	}
	s.publish(identity.ThreadID, event)
	return nil
}

func exactString(value any) string {
	text, _ := value.(string)
	if text == "" || text != strings.TrimSpace(text) {
		return ""
	}
	return text
}
