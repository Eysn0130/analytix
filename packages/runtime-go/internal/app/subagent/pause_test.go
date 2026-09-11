package subagent

import (
	"fmt"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func pauseTestRecord() domainjob.Record {
	return domainjob.Record{
		ID:               "job_pause",
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
		PauseState: domainjob.ChildRunPauseState{
			CanPause: true,
		},
	}
}

func TestValidateTaskJobPauseAcceptsRunningBackgroundChild(t *testing.T) {
	record := pauseTestRecord()
	validation := ValidateTaskJobPause(record, "thr_parent", TaskJobPauseRequest{
		JobID:           "job_pause",
		ClientRequestID: "pause_client",
	}, time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC))
	if validation.Code != "" || validation.Request.ID != "pause_client" || validation.Request.Status != "requested" {
		t.Fatalf("valid pause should pass with stable request: %#v", validation)
	}
}

func TestValidateTaskJobPauseRejectsTerminalCrossParentMissingLineageAndForeground(t *testing.T) {
	base := pauseTestRecord()
	cases := []struct {
		name   string
		record domainjob.Record
		parent string
		code   string
		reason string
	}{
		{
			name: "completed",
			record: func() domainjob.Record {
				record := base
				record.Status = string(domainjob.StatusCompleted)
				return record
			}(),
			parent: "thr_parent",
			code:   TaskJobErrorConflict,
			reason: "terminal",
		},
		{
			name:   "cross parent",
			record: base,
			parent: "thr_other",
			code:   TaskJobErrorForbidden,
			reason: "does not belong",
		},
		{
			name: "missing lineage",
			record: func() domainjob.Record {
				record := base
				record.ChildThreadID = ""
				return record
			}(),
			parent: "thr_parent",
			code:   TaskJobErrorValidation,
			reason: "no child run lineage",
		},
		{
			name: "foreground",
			record: func() domainjob.Record {
				record := base
				record.Background = false
				return record
			}(),
			parent: "thr_parent",
			code:   TaskJobErrorValidation,
			reason: "foreground",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			validation := ValidateTaskJobPause(tc.record, tc.parent, TaskJobPauseRequest{JobID: "job_pause"}, time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC))
			if validation.Code != tc.code || !strings.Contains(validation.Reason, tc.reason) {
				t.Fatalf("unexpected validation result: %#v", validation)
			}
		})
	}
}

func TestValidateTaskJobResumeChecksPausedStateAndToken(t *testing.T) {
	record := pauseTestRecord()
	record.Status = string(domainjob.StatusPaused)
	record.PauseRequests = []domainjob.PauseRequest{{
		ID:                   "pause_1",
		ParentThreadID:       "thr_parent",
		ChildRunID:           "job_pause",
		JobID:                "job_pause",
		Status:               "paused",
		RequestedAt:          "2026-07-06T00:00:00Z",
		PausedAt:             "2026-07-06T00:00:01Z",
		ResumeToken:          "resume_ok",
		ResumeTokenExpiresAt: "2026-07-07T00:00:00Z",
	}}
	validation := ValidateTaskJobResume(record, "thr_parent", TaskJobResumeRequest{JobID: "job_pause", ResumeToken: "resume_ok"}, time.Date(2026, 7, 6, 0, 0, 2, 0, time.UTC))
	if validation.Code != "" || validation.Request.ID != "pause_1" {
		t.Fatalf("valid resume should pass: %#v", validation)
	}
	validation = ValidateTaskJobResume(record, "thr_parent", TaskJobResumeRequest{JobID: "job_pause", ResumeToken: "wrong"}, time.Date(2026, 7, 6, 0, 0, 2, 0, time.UTC))
	if validation.Code != TaskJobErrorForbidden || !strings.Contains(validation.Reason, "token") {
		t.Fatalf("token mismatch should reject: %#v", validation)
	}
}

func TestBuildChildPauseEventContainsAuditPayload(t *testing.T) {
	record := pauseTestRecord()
	record.Status = string(domainjob.StatusPaused)
	record.PauseState = domainjob.ChildRunPauseState{
		PauseRequestID: "pause_1",
		Status:         "paused",
		Paused:         true,
		CanResume:      true,
		LastPausedAt:   "2026-07-06T00:00:01Z",
	}
	request := domainjob.PauseRequest{
		ID:             "pause_1",
		JobID:          "job_pause",
		ChildRunID:     "job_pause",
		ParentThreadID: "thr_parent",
		Status:         "paused",
		RequestedAt:    "2026-07-06T00:00:00Z",
		PausedAt:       "2026-07-06T00:00:01Z",
		SourceTurnID:   "turn_parent",
	}
	event := BuildChildPauseEvent(ChildPauseEventInput{Record: record, Request: request, Status: "paused"})
	if event["kind"] != "child_paused" ||
		event["jobId"] != "job_pause" ||
		event["childRunId"] != "job_pause" ||
		event["pauseRequestId"] != "pause_1" {
		t.Fatalf("pause event identity mismatch: %#v", event)
	}
	child, ok := event["child"].(map[string]any)
	if !ok {
		t.Fatalf("pause event should include child metadata: %#v", event)
	}
	if child["pauseRequestId"] != "pause_1" ||
		child["pauseStatus"] != "paused" ||
		child["canResume"] != true ||
		child["paused"] != true {
		t.Fatalf("pause child metadata mismatch: %#v", child)
	}
}

func TestBuildChildPauseEventDoesNotReflectOpenStatusOrReasoning(t *testing.T) {
	t.Parallel()
	private := "paused<think>PRIVATE_PAUSE</think> account 6222020202020202020"
	record := domainjob.Record{ID: "job_pause", ParentThreadID: "thread", ParentTurnID: "turn", Status: string(domainjob.StatusPaused)}
	event := BuildChildPauseEvent(ChildPauseEventInput{
		Record: record, Request: domainjob.PauseRequest{ID: "pause", Status: private}, Status: private, Reason: private,
	})
	text := fmt.Sprint(event)
	if event["kind"] != "child_pause_unknown" || event["status"] != "unknown" ||
		strings.Contains(text, "PRIVATE_PAUSE") || strings.Contains(text, "6222020202020202020") {
		t.Fatalf("pause event reflected open control text: %#v", event)
	}
	response := TaskJobPauseResponse(record, domainjob.PauseRequest{ID: "pause", Status: "rejected"}, "rejected", private)
	if _, hasRawErrorAlias := response["error"]; hasRawErrorAlias || strings.Contains(fmt.Sprint(response), "PRIVATE_PAUSE") {
		t.Fatalf("pause response exposed a raw error alias: %#v", response)
	}
}
