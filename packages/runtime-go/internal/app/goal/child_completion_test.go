package goal

import (
	"errors"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type childRunStarterStub struct {
	requests []domainjob.StartRequest
	err      error
}

func (stub *childRunStarterStub) StartChildRun(request domainjob.StartRequest) (domainjob.Record, error) {
	stub.requests = append(stub.requests, request)
	if stub.err != nil {
		return domainjob.Record{}, stub.err
	}
	return domainjob.Record{
		ID:                    "child_1",
		ParentGoalID:          request.ParentGoalID,
		ParentGoalObjective:   request.ParentGoalObjective,
		ParentThreadID:        request.ParentThreadID,
		ParentTurnID:          request.ParentTurnID,
		ParentToolCallID:      request.ParentToolCallID,
		SecurityBinding:       domainjob.CloneSecurityBinding(request.SecurityBinding),
		Kind:                  request.Kind,
		Status:                "completed",
		LineageKey:            "lineage_1",
		Model:                 request.Model,
		Effort:                request.Effort,
		ProfileSource:         request.ProfileSource,
		DefaultModelInherited: request.DefaultModelInherited,
		Output:                request.Output,
	}, nil
}

type childRunEventRecorderStub struct {
	events []map[string]any
	err    error
}

func (stub *childRunEventRecorderStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	stub.events = append(stub.events, event)
	return event, []string{"persist", "publish"}, stub.err
}

func TestRecordCompletedTurnChildRunStartsLineageAndRecordsEvent(t *testing.T) {
	starter := &childRunStarterStub{}
	events := &childRunEventRecorderStub{}
	terminalDigest := strings.Repeat("a", 64)
	record, recorded, err := RecordCompletedTurnChildRun(CompletedTurnChildRunInput{
		Starter:                 starter,
		Events:                  events,
		Goal:                    map[string]any{"id": " goal_1 ", "objective": " finish it "},
		ThreadID:                "thr_1",
		TurnID:                  "turn_1",
		TurnNumber:              7,
		Model:                   "gpt-5",
		Effort:                  "high",
		TerminalReferenceKind:   TerminalReferenceAcceptedFinal,
		TerminalReferenceDigest: terminalDigest,
		TotalTokens:             123,
		CacheHitRate:            0.5,
		RequestedModel:          "",
		SecurityContext:         testGoalChildSecurityContext(t),
	})
	if err != nil {
		t.Fatalf("record child run: %v", err)
	}
	if !recorded || record.ID != "child_1" {
		t.Fatalf("record result mismatch: recorded=%v record=%#v", recorded, record)
	}
	if len(starter.requests) != 1 {
		t.Fatalf("expected one child run request, got %#v", starter.requests)
	}
	request := starter.requests[0]
	if request.ParentGoalID != "goal_1" || request.ParentGoalObjective != "finish it" || request.ParentThreadID != "thr_1" || request.ParentTurnID != "turn_1" {
		t.Fatalf("lineage request mismatch: %#v", request)
	}
	if !request.DefaultModelInherited || request.SourceRef != "analytix-terminal-authority-ref/v1/accepted_final/sha256/"+terminalDigest ||
		request.Output != "" || request.Model != "gpt-5" || request.Effort != "high" {
		t.Fatalf("child request metadata mismatch: %#v", request)
	}
	if domainjob.ValidateSecurityBinding(request.SecurityBinding) != nil || request.ParentToolCallID == "" {
		t.Fatalf("goal child run did not receive a host security binding: %#v", request)
	}
	if len(events.events) != 1 {
		t.Fatalf("expected child event, got %#v", events.events)
	}
	child, _ := events.events[0]["child"].(map[string]any)
	if child["outputWithheld"] != true || child["factAnswerAllowed"] != false || child["canReadOutput"] != false {
		t.Fatalf("goal child event was not projected as untrusted metadata: %#v", child)
	}
}

func TestRecordCompletedTurnChildRunSkipsMissingGoal(t *testing.T) {
	starter := &childRunStarterStub{}
	events := &childRunEventRecorderStub{}
	_, recorded, err := RecordCompletedTurnChildRun(CompletedTurnChildRunInput{
		Starter: starter,
		Events:  events,
		Goal:    nil,
	})
	if err != nil || recorded {
		t.Fatalf("missing goal should skip without error: recorded=%v err=%v", recorded, err)
	}
	if len(starter.requests) != 0 || len(events.events) != 0 {
		t.Fatalf("missing goal should not touch dependencies")
	}
}

func TestRecordCompletedTurnChildRunPropagatesEventError(t *testing.T) {
	starter := &childRunStarterStub{}
	eventErr := errors.New("event failed")
	_, recorded, err := RecordCompletedTurnChildRun(CompletedTurnChildRunInput{
		Starter:                 starter,
		Events:                  &childRunEventRecorderStub{err: eventErr},
		Goal:                    map[string]any{"objective": "x"},
		ThreadID:                "thr_1",
		TurnID:                  "turn_1",
		TerminalReferenceKind:   TerminalReferenceGeneralCAS,
		TerminalReferenceDigest: strings.Repeat("b", 64),
		SecurityContext:         testGoalChildSecurityContext(t),
	})
	if recorded || !errors.Is(err, eventErr) {
		t.Fatalf("expected event error, recorded=%v err=%v", recorded, err)
	}
}

func TestRecordCompletedTurnChildRunRejectsInvalidPublicationDigest(t *testing.T) {
	starter := &childRunStarterStub{}
	_, recorded, err := RecordCompletedTurnChildRun(CompletedTurnChildRunInput{
		Starter:                 starter,
		Events:                  &childRunEventRecorderStub{},
		Goal:                    map[string]any{"objective": "x"},
		ThreadID:                "thr_1",
		TurnID:                  "turn_1",
		TerminalReferenceKind:   TerminalReferenceAcceptedFinal,
		TerminalReferenceDigest: "assistant prose",
		SecurityContext:         testGoalChildSecurityContext(t),
	})
	if recorded || err == nil {
		t.Fatalf("invalid publication digest must fail closed: recorded=%v err=%v", recorded, err)
	}
	if len(starter.requests) != 0 {
		t.Fatalf("invalid publication digest must not start a child run: %#v", starter.requests)
	}
}

func TestRecordCompletedTurnChildRunRejectsInvalidReasoningEffortBeforeEffects(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	starter := &childRunStarterStub{}
	events := &childRunEventRecorderStub{}
	_, recorded, err := RecordCompletedTurnChildRun(CompletedTurnChildRunInput{
		Starter: starter, Events: events, Goal: map[string]any{"objective": "x"},
		ThreadID: "thr_1", TurnID: "turn_1", Effort: sentinel,
		TerminalReferenceKind:   TerminalReferenceAcceptedFinal,
		TerminalReferenceDigest: strings.Repeat("a", 64),
		SecurityContext:         testGoalChildSecurityContext(t),
	})
	if recorded || err == nil || strings.Contains(err.Error(), sentinel) || len(starter.requests) != 0 || len(events.events) != 0 {
		t.Fatalf("invalid effort reached or was reflected by goal lineage: recorded=%v err=%v starts=%#v events=%#v", recorded, err, starter.requests, events.events)
	}
}

func testGoalChildSecurityContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
