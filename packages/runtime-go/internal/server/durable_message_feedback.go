// Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
// Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"time"

	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type messageFeedbackSnapshotV1 struct {
	thread map[string]any
	public map[string]any
	items  []domainthread.MessageFeedbackItemV1
}

func (s *DurableEventSessionStore) messageFeedbackSnapshotNoLockV1(threadID string, project func(map[string]any) (map[string]any, error)) (messageFeedbackSnapshotV1, error) {
	if err := s.requireRestartWritableNoLockV1(threadID); err != nil {
		return messageFeedbackSnapshotV1{}, err
	}
	if project == nil || !domainthread.IsCanonicalRecordID(threadID) {
		return messageFeedbackSnapshotV1{}, domainthread.ErrMessageFeedbackInvalidV1
	}
	// A missing primary is recovery work, not an empty feedback list. Do not
	// establish feedback ownership from a rendering sidecar or create a Session.
	if _, err := os.Stat(s.threadPath(threadID)); err != nil {
		return messageFeedbackSnapshotV1{}, err
	}
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return messageFeedbackSnapshotV1{}, err
	}
	if thread == nil || stringField(thread, "status") == "deleted" {
		return messageFeedbackSnapshotV1{}, os.ErrNotExist
	}
	public, err := project(thread)
	if err != nil {
		return messageFeedbackSnapshotV1{}, err
	}
	if public == nil || stringField(public, "id") != threadID {
		return messageFeedbackSnapshotV1{}, domainthread.ErrMessageFeedbackInvalidV1
	}
	loaded, err := s.eventLog.LoadSince(threadID, 0)
	if err != nil {
		return messageFeedbackSnapshotV1{}, err
	}
	if len(loaded.Diagnostics) != 0 {
		return messageFeedbackSnapshotV1{}, errors.New("message feedback log is invalid")
	}
	items, err := domainthread.MessageFeedbackCurrentV1(loaded.Events, threadID, stringField(thread, "createdAt"))
	return messageFeedbackSnapshotV1{thread: thread, public: public, items: items}, err
}

func feedbackItemV1(items []domainthread.MessageFeedbackItemV1, messageID string) *domainthread.MessageFeedbackItemV1 {
	for _, item := range items {
		if item.MessageID == messageID {
			copy := item
			return &copy
		}
	}
	return nil
}

func feedbackAssistantTargetV1(thread map[string]any, messageID string) bool {
	turns, _ := thread["turns"].([]any)
	found := 0
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "id") != messageID {
				continue
			}
			if stringField(item, "kind") != "assistant_text" || stringField(item, "role") != "assistant" || stringField(item, "status") != "completed" || stringField(item, "threadId") != stringField(thread, "id") || stringField(item, "turnId") != stringField(turn, "id") || strings.TrimSpace(stringField(item, "text")) == "" {
				return false
			}
			found++
		}
	}
	return found == 1
}

func (s *DurableEventSessionStore) ListMessageFeedbackV1(threadID string, project func(map[string]any) (map[string]any, error)) ([]domainthread.MessageFeedbackItemV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, err := s.messageFeedbackSnapshotNoLockV1(threadID, project)
	return snapshot.items, err
}

func (s *DurableEventSessionStore) PutMessageFeedbackV1(threadID, messageID string, request domainthread.MessageFeedbackPutRequestV1, project func(map[string]any) (map[string]any, error)) (domainthread.MessageFeedbackItemV1, error) {
	if err := domainthread.ValidateMessageFeedbackPutV1(request); err != nil {
		return domainthread.MessageFeedbackItemV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, err := s.messageFeedbackSnapshotNoLockV1(threadID, project)
	if err != nil {
		return domainthread.MessageFeedbackItemV1{}, err
	}
	if !feedbackAssistantTargetV1(snapshot.public, messageID) {
		return domainthread.MessageFeedbackItemV1{}, domainthread.ErrMessageFeedbackTargetNotFoundV1
	}
	current := feedbackItemV1(snapshot.items, messageID)
	if current == nil && request.IfVersion != nil || current != nil && (request.IfVersion == nil || *request.IfVersion != current.Version) {
		return domainthread.MessageFeedbackItemV1{}, &domainthread.MessageFeedbackVersionConflictV1{Current: current}
	}
	if current != nil && current.Rating == request.Rating && reflect.DeepEqual(current.Note, request.Note) && reflect.DeepEqual(current.Category, request.Category) {
		return *current, nil
	}
	version, err := newFeedbackVersionV1()
	if err != nil {
		return domainthread.MessageFeedbackItemV1{}, err
	}
	now := time.Now().UnixMilli()
	item := domainthread.MessageFeedbackItemV1{MessageID: messageID, Rating: request.Rating, Note: request.Note, Category: request.Category, Version: version, CreatedAt: now, UpdatedAt: now}
	if current != nil {
		item.CreatedAt = current.CreatedAt
		if item.UpdatedAt <= current.UpdatedAt {
			item.UpdatedAt = current.UpdatedAt + 1
		}
	}
	if err := domainthread.ValidateMessageFeedbackItemV1(item); err != nil {
		return domainthread.MessageFeedbackItemV1{}, err
	}
	itemBody, err := json.Marshal(item)
	if err != nil {
		return domainthread.MessageFeedbackItemV1{}, err
	}
	var publicItem map[string]any
	if err := json.Unmarshal(itemBody, &publicItem); err != nil {
		return domainthread.MessageFeedbackItemV1{}, err
	}
	// Existing recursive persistence guards inspect maps, not arbitrary structs.
	event := map[string]any{"kind": domainthread.MessageFeedbackPutKindV1, "threadId": threadID, "threadCreatedAt": stringField(snapshot.thread, "createdAt"), "feedback": publicItem}
	if err := s.appendMessageFeedbackNoLockV1(event, project); err != nil {
		return domainthread.MessageFeedbackItemV1{}, err
	}
	return item, nil
}

func (s *DurableEventSessionStore) DeleteMessageFeedbackV1(threadID, messageID, version string, project func(map[string]any) (map[string]any, error)) error {
	if err := domainthread.ValidateMessageFeedbackVersionV1(version); err != nil {
		return err
	}
	if !domainthread.IsCanonicalRecordID(messageID) {
		return domainthread.ErrMessageFeedbackInvalidV1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, err := s.messageFeedbackSnapshotNoLockV1(threadID, project)
	if err != nil {
		return err
	}
	current := feedbackItemV1(snapshot.items, messageID)
	if current == nil {
		return nil
	}
	if current.Version != version {
		return &domainthread.MessageFeedbackVersionConflictV1{Current: current}
	}
	return s.appendMessageFeedbackNoLockV1(map[string]any{"kind": domainthread.MessageFeedbackDeleteKindV1, "threadId": threadID, "threadCreatedAt": stringField(snapshot.thread, "createdAt"), "messageId": messageID}, project)
}

func (s *DurableEventSessionStore) appendMessageFeedbackNoLockV1(event map[string]any, project func(map[string]any) (map[string]any, error)) error {
	threadID := stringField(event, "threadId")
	seq, err := s.nextSeqNoLock(threadID)
	if err != nil {
		return err
	}
	event["seq"], event["timestamp"] = seq, time.Now().UTC().Format(time.RFC3339Nano)
	if err := domainthread.ValidateMessageFeedbackEventV1(event); err != nil {
		return err
	}
	// Uses the existing reservation, restart, atomic log and persistence guards.
	// Unlike public RecordEvent this dedicated owner never publishes to subscribers.
	if err := s.persistRecordedEventsNoLock(threadID, []map[string]any{event}, true); err != nil {
		return err
	}
	s.highestSeqByThread[threadID] = seq
	confirmed, err := s.messageFeedbackSnapshotNoLockV1(threadID, project)
	if err != nil {
		return err
	}
	if stringField(confirmed.thread, "createdAt") != event["threadCreatedAt"] {
		return errors.New("message feedback lifecycle changed")
	}
	loaded, err := s.eventLog.LoadSince(threadID, seq-1)
	if err != nil || len(loaded.Diagnostics) != 0 || len(loaded.Events) != 1 {
		return errors.Join(errors.New("message feedback persistence was not confirmed"), err)
	}
	wanted, _ := json.Marshal(event)
	actual, _ := json.Marshal(loaded.Events[0])
	if string(wanted) != string(actual) {
		return errors.New("message feedback durable tail changed")
	}
	return nil
}

func newFeedbackVersionV1() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	hexValue := hex.EncodeToString(raw[:])
	return hexValue[:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:], nil
}
