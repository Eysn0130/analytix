package subagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

func steerTestRecord() domainjob.Record {
	return domainjob.Record{
		ID:               "job_steer",
		Kind:             "subagent",
		Status:           string(domainjob.StatusRunning),
		ParentThreadID:   "thr_parent",
		ParentTurnID:     "turn_parent",
		ParentToolItemID: "item_parent",
		ParentToolCallID: "call_parent",
		ChildThreadID:    "thr_child",
		ChildTurnID:      "turn_child",
		Label:            "research",
		Background:       true,
		SteerState: domainjob.ChildRunState{
			CanAcceptSteer: true,
		},
	}
}

func TestValidateTaskJobSteerAcceptsRunningBackgroundChild(t *testing.T) {
	const clientMessageID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	record := steerTestRecord()
	validation := ValidateTaskJobSteer(record, "thr_parent", TaskJobSteerRequest{
		JobID:           "job_steer",
		Message:         "focus on tests",
		ClientMessageID: clientMessageID,
	}, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	if validation.Code != "" || validation.Message.ID != clientMessageID || validation.Message.Text != "focus on tests" {
		t.Fatalf("valid steer should pass with stable message: %#v", validation)
	}
}

func TestTaskJobSteerRejectsNonOpaqueClientMessageID(t *testing.T) {
	const completeAccount = "6222021234567890123"
	if _, response, invalid := TaskJobSteerRequestFromArgs(map[string]any{
		"jobId": "job_steer", "message": "continue", "clientMessageId": completeAccount,
	}); !invalid || strings.Contains(fmt.Sprint(response), completeAccount) {
		t.Fatalf("full account client id was accepted or reflected: invalid=%v response=%#v", invalid, response)
	}
	validation := ValidateTaskJobSteer(steerTestRecord(), "thr_parent", TaskJobSteerRequest{
		JobID: "job_steer", Message: "continue", ClientMessageID: completeAccount,
	}, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	if validation.Code != TaskJobErrorValidation || strings.Contains(validation.Message.ID, completeAccount) {
		t.Fatalf("direct task-job steer did not fail closed: %#v", validation)
	}
}

func TestTaskJobSteerRequestRejectsConflictingAliases(t *testing.T) {
	for _, args := range []map[string]any{
		{"jobId": "job_1", "id": "job_2", "message": "continue"},
		{"jobId": "job_1", "message": "continue", "text": "replace"},
		{"jobId": "job_1", "message": "continue", "client_message_id": "one", "clientMessageId": "two"},
		{"jobId": "job_1", "message": "continue", "source_turn_id": "one", "sourceTurnId": "two"},
	} {
		if _, _, invalid := TaskJobSteerRequestFromArgs(args); !invalid {
			t.Fatalf("conflicting steer aliases were accepted: %#v", args)
		}
	}
}

func TestValidateTaskJobSteerRejectsTerminalCrossParentMissingLineageAndLimits(t *testing.T) {
	base := steerTestRecord()
	cases := []struct {
		name    string
		record  domainjob.Record
		parent  string
		request TaskJobSteerRequest
		code    string
		reason  string
	}{
		{
			name: "completed",
			record: func() domainjob.Record {
				record := base
				record.Status = string(domainjob.StatusCompleted)
				return record
			}(),
			parent:  "thr_parent",
			request: TaskJobSteerRequest{JobID: "job_steer", Message: "again"},
			code:    TaskJobErrorConflict,
			reason:  "terminal",
		},
		{
			name:    "cross parent",
			record:  base,
			parent:  "thr_other",
			request: TaskJobSteerRequest{JobID: "job_steer", Message: "again"},
			code:    TaskJobErrorForbidden,
			reason:  "does not belong",
		},
		{
			name:    "missing lineage",
			record:  func() domainjob.Record { record := base; record.ChildThreadID = ""; return record }(),
			parent:  "thr_parent",
			request: TaskJobSteerRequest{JobID: "job_steer", Message: "again"},
			code:    TaskJobErrorValidation,
			reason:  "no child run lineage",
		},
		{
			name:    "missing job",
			record:  base,
			parent:  "thr_parent",
			request: TaskJobSteerRequest{JobID: "job_missing", Message: "again"},
			code:    TaskJobErrorNotFound,
			reason:  "not found",
		},
		{
			name:    "too long",
			record:  base,
			parent:  "thr_parent",
			request: TaskJobSteerRequest{JobID: "job_steer", Message: "12345", MaxMessageRunes: 4},
			code:    TaskJobErrorValidation,
			reason:  "too long",
		},
		{
			name: "pending limit",
			record: func() domainjob.Record {
				record := base
				record.Steers = []domainjob.SteerMessage{{ID: "steer_1", Status: "queued"}}
				return record
			}(),
			parent:  "thr_parent",
			request: TaskJobSteerRequest{JobID: "job_steer", Message: "again", MaxPendingSteers: 1},
			code:    TaskJobErrorConflict,
			reason:  "too many pending",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			validation := ValidateTaskJobSteer(tc.record, tc.parent, tc.request, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
			if validation.Code != tc.code || !strings.Contains(validation.Reason, tc.reason) {
				t.Fatalf("unexpected validation result: %#v", validation)
			}
		})
	}
}

func TestBuildChildSteerEventContainsAuditPayloadAndNoPromptText(t *testing.T) {
	record := steerTestRecord()
	record.SteerState = domainjob.ChildRunState{
		PendingSteers:  1,
		AdmittedSteers: 0,
		SteerCount:     1,
		CanAcceptSteer: true,
		LastSteerAt:    "2026-07-01T00:00:00Z",
	}
	message := domainjob.SteerMessage{
		ID:               "steer_1",
		JobID:            "job_steer",
		ChildRunID:       "job_steer",
		ParentThreadID:   "thr_parent",
		Text:             "sensitive parent instruction",
		Status:           "queued",
		CreatedAt:        "2026-07-01T00:00:00Z",
		SourceTurnID:     "turn_parent",
		SourceToolCallID: "call_parent",
	}
	event := BuildChildSteerEvent(ChildSteerEventInput{
		Record:  record,
		Message: message,
		Status:  "queued",
	})
	if event["kind"] != "child_steer_queued" ||
		event["jobId"] != "job_steer" ||
		event["childRunId"] != "job_steer" ||
		event["steerMessageId"] != "steer_1" {
		t.Fatalf("steer event identity mismatch: %#v", event)
	}
	child, ok := event["child"].(map[string]any)
	if !ok {
		t.Fatalf("steer event should include child metadata: %#v", event)
	}
	if child["steerMessageId"] != "steer_1" ||
		child["steerStatus"] != "queued" ||
		child["pendingSteers"] != float64(1) ||
		child["canAcceptSteer"] != true {
		t.Fatalf("steer child metadata mismatch: %#v", child)
	}
	if strings.Contains(strings.Join([]string{
		firstNonEmptyAnyString(event["message"]),
		firstNonEmptyAnyString(event["text"]),
		firstNonEmptyAnyString(child["message"]),
		firstNonEmptyAnyString(child["text"]),
	}, " "), "sensitive parent instruction") {
		t.Fatalf("steer event should not expose full message text: %#v", event)
	}
}

func TestBuildChildSteerEventDoesNotReflectOpenStatusOrReasoning(t *testing.T) {
	t.Parallel()
	private := "queued<think>PRIVATE_STEER</think> account 6222020202020202020"
	record := domainjob.Record{ID: "job_steer", ParentThreadID: "thread", ParentTurnID: "turn", Status: string(domainjob.StatusRunning)}
	event := BuildChildSteerEvent(ChildSteerEventInput{
		Record: record, Message: domainjob.SteerMessage{ID: "steer", Status: private}, Status: private, Reason: private,
	})
	text := fmt.Sprint(event)
	if event["kind"] != "child_steer_unknown" || event["status"] != "unknown" ||
		strings.Contains(text, "PRIVATE_STEER") || strings.Contains(text, "6222020202020202020") {
		t.Fatalf("steer event reflected open control text: %#v", event)
	}
	response := TaskJobSteerResponse(record, domainjob.SteerMessage{ID: "steer", Status: "rejected"}, "rejected", private)
	if _, hasRawErrorAlias := response["error"]; hasRawErrorAlias || strings.Contains(fmt.Sprint(response), "PRIVATE_STEER") {
		t.Fatalf("steer response exposed a raw error alias: %#v", response)
	}
}

func TestTaskJobSteerRejectsMissingOrMismatchedChildFrozenContextBeforeQueue(t *testing.T) {
	for _, test := range []struct {
		name    string
		blocker string
		err     error
	}{
		{name: "missing authority", err: errors.New("missing frozen context")},
		{name: "risk raise", blocker: "case_risk_raise"},
	} {
		t.Run(test.name, func(t *testing.T) {
			jobs := &taskJobSteerStoreStub{record: steerTestRecord()}
			turns := &taskJobTurnSteerStoreStub{}
			events := 0
			result := SteerRuntimeTaskJob(TaskJobSteerRuntimeDeps{
				Context: context.Background(), Jobs: jobs, Turns: turns,
				RecordEvent: func(map[string]any, string) { events++ },
				BeginAuthority: func(context.Context, domainjob.Record, domainjob.SteerMessage) (TaskJobSteerAuthorization, string, error) {
					return TaskJobSteerAuthorization{}, test.blocker, test.err
				},
			}, "thr_parent", TaskJobSteerRequest{JobID: "job_steer", Message: "CASE_SENTINEL_MUST_NOT_PERSIST"}, time.Now().UTC())
			if !result.IsError || jobs.queueCalls != 0 || jobs.rejectCalls != 0 || turns.calls != 0 || events != 0 {
				t.Fatalf("security rejection wrote steer state: result=%#v jobs=%#v turns=%#v events=%d", result, jobs, turns, events)
			}
			if test.blocker != "" && (result.Response["code"] != "new_turn_required" || result.Response["blockerCode"] != test.blocker) {
				t.Fatalf("risk raise did not return fixed new-turn boundary: %#v", result.Response)
			}
		})
	}
}

func TestRejectedTaskJobSteerNeverPersistsUnprojectedContent(t *testing.T) {
	record := steerTestRecord()
	record.Status = string(domainjob.StatusCompleted)
	jobs := &taskJobSteerStoreStub{record: record}
	sentinel := "6222021234567890 PROVIDER_REASONING_SENTINEL"
	result := SteerRuntimeTaskJob(TaskJobSteerRuntimeDeps{Jobs: jobs}, "thr_parent", TaskJobSteerRequest{
		JobID: record.ID, Message: sentinel, ClientMessageID: "rejected_1",
	}, time.Now().UTC())
	if !result.IsError || jobs.queueCalls != 0 || jobs.rejectCalls != 0 {
		t.Fatalf("pre-authority validation persisted rejected content: result=%#v jobs=%#v", result, jobs)
	}
	for _, message := range jobs.record.Steers {
		if strings.Contains(message.Text, sentinel) {
			t.Fatalf("rejected unprojected content entered durable job state: %#v", message)
		}
	}
}

func TestTaskJobSteerContextDigestAndProjectionBindTurnWrite(t *testing.T) {
	digest := domainsecurity.SHA256Hex([]byte("child-context"))
	jobs := &taskJobSteerStoreStub{record: steerTestRecord()}
	turns := &taskJobTurnSteerStoreStub{}
	releases := 0
	result := SteerRuntimeTaskJob(TaskJobSteerRuntimeDeps{
		Context: context.Background(), Jobs: jobs, Turns: turns,
		BeginAuthority: func(context.Context, domainjob.Record, domainjob.SteerMessage) (TaskJobSteerAuthorization, string, error) {
			return TaskJobSteerAuthorization{
				ExpectedContextDigest: digest, ProjectedText: "account_[MASKED]",
				EffectBinding: domainsteering.EntryLogicalEffectBinding{
					LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
				},
				QueueAuthority: domainjob.SteerQueueAuthorityV1{Version: domainjob.SteerQueueAuthorityVersionV1},
				Release:        func() { releases++ },
			}, "", nil
		},
	}, "thr_parent", TaskJobSteerRequest{JobID: "job_steer", Message: "raw-account-sentinel"}, time.Now().UTC())
	if result.IsError || jobs.queueCalls != 1 || turns.calls != 1 || releases != 1 ||
		jobs.queued.Text != "account_[MASKED]" || turns.expectedContextDigest != digest ||
		firstNonEmptyAnyString(turns.entry["text"]) != "account_[MASKED]" {
		t.Fatalf("context-bound projected steer mismatch: result=%#v jobs=%#v turns=%#v releases=%d", result, jobs, turns, releases)
	}
}

func TestTaskJobSteerSettlementFailureDoesNotClaimRejected(t *testing.T) {
	digest := domainsecurity.SHA256Hex([]byte("child-context"))
	jobs := &taskJobSteerStoreStub{record: steerTestRecord(), rejectErr: errors.New("disk unavailable")}
	turns := &taskJobTurnSteerStoreStub{err: errors.New("context changed")}
	rejectedEvents := 0
	result := SteerRuntimeTaskJob(TaskJobSteerRuntimeDeps{
		Context: context.Background(), Jobs: jobs, Turns: turns,
		RecordEvent: func(event map[string]any, _ string) {
			if event["kind"] == "child_steer_rejected" {
				rejectedEvents++
			}
		},
		BeginAuthority: func(context.Context, domainjob.Record, domainjob.SteerMessage) (TaskJobSteerAuthorization, string, error) {
			return TaskJobSteerAuthorization{
				ExpectedContextDigest: digest, ProjectedText: "safe projected text",
				EffectBinding: domainsteering.EntryLogicalEffectBinding{
					LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
				},
				QueueAuthority: domainjob.SteerQueueAuthorityV1{Version: domainjob.SteerQueueAuthorityVersionV1},
			}, "", nil
		},
	}, "thr_parent", TaskJobSteerRequest{JobID: "job_steer", Message: "raw sentinel"}, time.Now().UTC())
	if !result.IsError || result.Response["status"] != "blocked" || result.Response["rejected"] == true ||
		jobs.rejectCalls != 1 || rejectedEvents != 0 {
		t.Fatalf("failed rejection produced false settlement audit: result=%#v jobs=%#v rejectedEvents=%d", result, jobs, rejectedEvents)
	}
}

func TestQueuedTaskJobSteerRevalidatesAtChildTurnAdmission(t *testing.T) {
	record := steerTestRecord()
	record.ChildTurnID = "turn_child"
	record.Steers = []domainjob.SteerMessage{{ID: "steer_pending", Text: "queued-sensitive", Status: "queued"}}
	jobs := &taskJobSteerStoreStub{record: record}
	turns := &taskJobTurnSteerStoreStub{}
	authorityCalls := 0
	err := PrepareRuntimeTaskJobSteeringForTurn(TaskJobSteerRuntimeDeps{
		Context: context.Background(), Jobs: jobs, Turns: turns,
		BeginAuthority: func(context.Context, domainjob.Record, domainjob.SteerMessage) (TaskJobSteerAuthorization, string, error) {
			authorityCalls++
			return TaskJobSteerAuthorization{}, "case_risk_raise", nil
		},
	}, record.ID, record.ChildThreadID, "turn_child")
	if err != nil || authorityCalls != 1 || jobs.rejectCalls != 1 || turns.calls != 0 {
		t.Fatalf("queued steer bypassed revalidation: err=%v calls=%d jobs=%#v turns=%#v", err, authorityCalls, jobs, turns)
	}
}

type taskJobSteerStoreStub struct {
	record      domainjob.Record
	queued      domainjob.SteerMessage
	queueCalls  int
	rejectCalls int
	rejectErr   error
	admitCalls  int
	admitErr    error
}

func TestQueuedTaskJobSteerKeepsOriginalMessageWhenAuthorityUnavailable(t *testing.T) {
	record := steerTestRecord()
	record.Steers = []domainjob.SteerMessage{{ID: "steer-pending", Text: "synthetic authorized guidance", Status: "queued"}}
	jobs := &taskJobSteerStoreStub{record: record}
	turns := &taskJobTurnSteerStoreStub{}
	events := 0
	err := PrepareRuntimeTaskJobSteeringForTurn(TaskJobSteerRuntimeDeps{Context: context.Background(), Jobs: jobs, Turns: turns, RecordEvent: func(map[string]any, string) { events++ }, BeginAuthority: func(context.Context, domainjob.Record, domainjob.SteerMessage) (TaskJobSteerAuthorization, string, error) {
		return TaskJobSteerAuthorization{}, "", errors.New("synthetic first-turn observation unavailable")
	}}, record.ID, record.ChildThreadID, record.ChildTurnID)
	if err == nil || jobs.rejectCalls != 0 || turns.calls != 0 || events != 0 {
		t.Fatal("unavailable first-turn authority replaced original queued steer with a rejected tombstone")
	}
}

func (s *taskJobSteerStoreStub) LoadChildRun(string) (domainjob.Record, error) { return s.record, nil }
func (s *taskJobSteerStoreStub) UpdateChildRun(_ string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	if request.ChildThreadID != "" {
		s.record.ChildThreadID = request.ChildThreadID
	}
	if request.ChildTurnID != "" {
		s.record.ChildTurnID = request.ChildTurnID
	}
	return s.record, nil
}
func (s *taskJobSteerStoreStub) QueueSteerMessage(_ string, _ domainjob.SteerQueueAuthorityV1, message domainjob.SteerMessage) (domainjob.Record, domainjob.SteerMessage, error) {
	s.queueCalls++
	s.queued = message
	s.record.Steers = append(s.record.Steers, message)
	return s.record, message, nil
}
func (s *taskJobSteerStoreStub) AdmitSteerMessage(_ string, messageID string, admittedAt string) (domainjob.Record, domainjob.SteerMessage, error) {
	s.admitCalls++
	if s.admitErr != nil {
		return s.record, domainjob.SteerMessage{}, s.admitErr
	}
	for _, message := range s.record.Steers {
		if message.ID == messageID {
			message.Status = "admitted"
			message.AdmittedAt = admittedAt
			return s.record, message, nil
		}
	}
	return s.record, domainjob.SteerMessage{}, errors.New("steer message not found")
}
func (s *taskJobSteerStoreStub) SettleSteerPromotionExact(
	_ string,
	expected domainjob.SteerMessage,
	settlement domainjob.SteerPromotionSettlementV1,
) (domainjob.Record, domainjob.SteerMessage, error) {
	s.admitCalls++
	if s.admitErr != nil {
		return s.record, domainjob.SteerMessage{}, s.admitErr
	}
	expected.Status = "admitted"
	expected.AdmittedAt = settlement.PromotedAt
	expected.PromotionCommitID = settlement.PromotionCommitID
	expected.PromotionEntryID = settlement.PromotionEntryID
	return s.record, expected, nil
}
func (s *taskJobSteerStoreStub) RejectSteerMessage(_ string, message domainjob.SteerMessage, _ string) (domainjob.Record, domainjob.SteerMessage, error) {
	s.rejectCalls++
	if s.rejectErr != nil {
		return s.record, domainjob.SteerMessage{}, s.rejectErr
	}
	message.Status = "rejected"
	return s.record, message, nil
}

type taskJobTurnSteerStoreStub struct {
	calls                 int
	expectedContextDigest string
	entry                 map[string]any
	err                   error
}

func TestTaskJobPromotionRequiresExactQueuedAuthority(t *testing.T) {
	const contextDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	message := domainjob.SteerMessage{
		ID: "steer_1", ParentThreadID: "thread_parent", ChildRunID: "job_1", JobID: "job_1",
		Text: "authorized guidance", ProjectionVersion: domainjob.SteerMessageProjectionVersionV1,
		ContextDigest: contextDigest, AuthorityDigest: contextDigest,
		QueueAuthorityDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Status:               "queued", CreatedAt: "2026-07-18T01:02:03Z", SourceTurnID: "turn_parent", SourceToolCallID: "call_1",
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
	}
	message.ContentDigest = domainjob.SteerMessageContentDigestV1(message)
	record := domainjob.Record{
		ID: "job_1", ParentThreadID: "thread_parent", ChildThreadID: "thread_child", ChildTurnID: "turn_child",
		Steers: []domainjob.SteerMessage{message},
	}
	jobs := &taskJobSteerStoreStub{record: record}
	entry := TaskJobSteerTurnEntry(record, message)
	if err := ValidateRuntimeTaskJobSteersForPromotion(jobs, []map[string]any{entry}, "thread_child", "turn_child", contextDigest); err != nil {
		t.Fatalf("exact queued authority should validate: %v", err)
	}
	tampered := map[string]any{}
	for key, value := range entry {
		tampered[key] = value
	}
	tampered["text"] = "different account 6222021234567890123"
	if err := ValidateRuntimeTaskJobSteersForPromotion(jobs, []map[string]any{tampered}, "thread_child", "turn_child", contextDigest); err == nil {
		t.Fatal("turn entry with different content must not consume queued authority")
	}
	if err := ValidateRuntimeTaskJobSteersForPromotion(jobs, []map[string]any{entry}, "thread_child", "turn_other", contextDigest); err == nil {
		t.Fatal("queued authority must not cross child turns")
	}
}

func TestTaskJobPromotionPreservesExactLegacyBindingFallback(t *testing.T) {
	const contextDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	message := domainjob.SteerMessage{
		ID: "steer_legacy", ParentThreadID: "thread_parent", ChildRunID: "job_legacy", JobID: "job_legacy",
		Text: "legacy guidance", ProjectionVersion: domainjob.SteerMessageProjectionVersionV1,
		ContextDigest: contextDigest, AuthorityDigest: contextDigest,
		QueueAuthorityDigest: strings.Repeat("b", 64), Status: "queued", CreatedAt: "2026-07-18T01:02:03Z",
	}
	message.ContentDigest = domainjob.SteerMessageContentDigestV1(message)
	record := domainjob.Record{
		ID: "job_legacy", ParentThreadID: "thread_parent", ChildThreadID: "thread_child", ChildTurnID: "turn_child",
		Steers: []domainjob.SteerMessage{message},
	}
	entry := TaskJobSteerTurnEntry(record, message)
	if _, found := entry["logicalEffect"]; found {
		t.Fatalf("legacy entry unexpectedly gained a new signed field: %#v", entry)
	}
	jobs := &taskJobSteerStoreStub{record: record}
	if err := ValidateRuntimeTaskJobSteersForPromotion(jobs, []map[string]any{entry}, "thread_child", "turn_child", contextDigest); err != nil {
		t.Fatalf("exact legacy task-job steer should defer to frozen-context fallback: %v", err)
	}
	tampered := map[string]any{}
	for key, value := range entry {
		tampered[key] = value
	}
	tampered["logicalEffect"] = string(domainsecurity.LogicalEffectOrdinary)
	tampered["ordinaryWork"] = true
	if err := ValidateRuntimeTaskJobSteersForPromotion(jobs, []map[string]any{tampered}, "thread_child", "turn_child", contextDigest); err == nil {
		t.Fatal("one-sided logical effect upgrade escaped exact ledger validation")
	}
}

func TestTaskJobPromotionSettlementFailureIsReturned(t *testing.T) {
	const contextDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	message := domainjob.SteerMessage{
		ID: "steer_1", ParentThreadID: "thread_parent", ChildRunID: "job_1", JobID: "job_1",
		Text: "authorized guidance", ProjectionVersion: domainjob.SteerMessageProjectionVersionV1,
		ContextDigest: contextDigest, AuthorityDigest: contextDigest,
		QueueAuthorityDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Status:               "queued", CreatedAt: "2026-07-18T01:02:03Z",
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
	}
	message.ContentDigest = domainjob.SteerMessageContentDigestV1(message)
	record := domainjob.Record{ID: "job_1", ParentThreadID: "thread_parent", ChildThreadID: "thread_child", ChildTurnID: "turn_child"}
	entry := TaskJobSteerTurnEntry(record, message)
	commit := domainsteering.PromotionCommitV1{
		Version: domainsteering.ProjectionVersionV1, CommitID: "steer_commit_" + strings.Repeat("c", 64),
		ThreadID: "thread_child", TurnID: "turn_child", ContextDigest: contextDigest,
		EntryID: entry["id"].(string), ClientUserMessageID: message.ID, PromotedAt: "2026-07-18T01:02:04Z",
		Origin: "task_job", JobID: record.ID, SteerMessageID: message.ID, Entry: entry,
	}
	jobs := &taskJobSteerStoreStub{admitErr: errors.New("injected settlement failure")}
	if err := RecordRuntimeTaskJobSteersAdmitted(TaskJobSteerRuntimeDeps{Jobs: jobs}, []domainsteering.PromotionCommitV1{commit}); err == nil {
		t.Fatal("task-job settlement failure must block provider delivery")
	}
	if jobs.admitCalls != 1 {
		t.Fatalf("unexpected settlement attempts: %d", jobs.admitCalls)
	}
}

func (s *taskJobTurnSteerStoreStub) AdmitSteeringEntryForContext(
	_, _, _, expectedContextDigest string,
	entry map[string]any,
) (map[string]any, error) {
	s.calls++
	s.expectedContextDigest = expectedContextDigest
	s.entry = entry
	return entry, s.err
}

func TestQueuedTaskJobSteerPreservesOriginalWhenCurrentValidatorUnavailable(t *testing.T) {
	parent, binding := runtimeStateSecurityFixture(t, "thr_parent", "turn_parent", "case-steer", "snapshot-steer", 1)
	frozen, _ := runtimeStateSecurityFixture(t, "thr_child", "turn_child", "case-steer", "snapshot-steer", 1)
	record := steerTestRecord()
	record.SecurityBinding = binding
	record.Steers = []domainjob.SteerMessage{{ID: "steer-original", Text: "continue the next check", Status: "queued"}}
	jobs := &taskJobSteerStoreStub{record: record}
	turns := &taskJobTurnSteerStoreStub{}
	state := NewRuntimeState()
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), parent, time.Second); err != nil {
		t.Fatal(err)
	}
	events := 0
	err := PrepareRuntimeTaskJobSteeringForTurn(TaskJobSteerRuntimeDeps{Context: context.Background(), Jobs: jobs, Turns: turns, RecordEvent: func(map[string]any, string) { events++ }, BeginAuthority: func(ctx context.Context, current domainjob.Record, message domainjob.SteerMessage) (TaskJobSteerAuthorization, string, error) {
		return BeginCurrentTaskJobSteerAuthorityV1(ctx, CurrentTaskJobSteerAuthorityDepsV1{Jobs: jobs, State: state, Control: controlapp.NewController(nil), Blocker: func(domainjob.Record) string { return "" }, ObserveFirstTurn: func(context.Context, domainjob.Record) (domainsecurity.TurnSecurityContext, bool, error) {
			return frozen, true, nil
		}}, current, message)
	}}, record.ID, record.ChildThreadID, record.ChildTurnID)
	if err == nil || jobs.rejectCalls != 0 || turns.calls != 0 || events != 0 {
		t.Fatal("unavailable current validator replaced original queued guidance with rejection")
	}
}
