package eventlifecycle

import (
	"errors"
	"testing"
)

func TestValidateNewEventLifecycleV1AcceptsCompleteClosedSet(t *testing.T) {
	tests := []map[string]any{
		{"kind": "thread_created", "status": "idle"},
		{"kind": "thread_updated", "status": "idle"},
		{"kind": "thread_updated", "status": "running"},
		{"kind": "thread_updated", "status": "archived"},
		{"kind": "thread_updated", "status": "deleted"},
		{"kind": "turn_started", "status": "running"},
		{"kind": "turn_completed", "status": "completed"},
		{"kind": "turn_failed", "status": "failed"},
		{"kind": "turn_aborted", "status": "aborted"},
		{"kind": "tool_progress", "status": "running"},
		{"kind": "tool_progress", "status": "success"},
		{"kind": "tool_progress", "status": "error"},
		{"kind": "tool_result_upload_wait", "status": "waiting"},
		{"kind": "approval_requested", "status": "pending"},
		{"kind": "approval_resolved", "status": "allowed"},
		{"kind": "approval_resolved", "status": "denied"},
		{"kind": "approval_resolved", "status": "expired"},
		{"kind": "user_input_requested", "status": "pending"},
		{"kind": "user_input_resolved", "status": "submitted"},
		{"kind": "user_input_resolved", "status": "cancelled"},
		{"kind": "child_steer_queued", "status": "queued"},
		{"kind": "child_steer_admitted", "status": "admitted"},
		{"kind": "child_steer_rejected", "status": "rejected"},
		{"kind": "child_pause_requested", "status": "requested"},
		{"kind": "child_paused", "status": "paused"},
		{"kind": "child_resume_requested", "status": "resume_requested"},
		{"kind": "child_resumed", "status": "resumed"},
		{"kind": "child_pause_rejected", "status": "rejected"},
		{"kind": "child_pause_rejected", "status": "expired"},
	}
	for _, kind := range []string{
		"thread_rewound", "turn_steered", "item_created", "item_updated", "item_completed",
		"tool_call_started", "tool_call_finished", "tool_call_ready", "tool_storm_suppressed", "tool_catalog_changed",
		"compaction_started", "compaction_completed", "goal_updated", "goal_cleared", "goal_evidence_audit",
		"todos_updated", "todos_cleared", "autoresearch_state_audit", "checkpoint_captured",
		"checkpoint_rewind_rescue_created", "checkpoint_rewind_applied", "usage", "execution_grant_approved",
		"thread_archived", "eventlog_compacted",
	} {
		tests = append(tests, map[string]any{"kind": kind})
	}
	for _, stage := range []string{
		"setup", "pre_start", "post_start", "input_received", "input_cached", "input_routed",
		"input_compressed", "input_remembered", "pre_send", "post_send", "response_received",
		"provider_admission_rejected",
		"provider_retrying", "step_limit_finalizing", "provider_error", "empty_final_recovered", "loop_guard",
		"subagent_queued", "subagent_running", "subagent_pause_requested", "subagent_paused",
		"subagent_resume_requested", "subagent_resuming", "subagent_completed", "subagent_failed",
		"subagent_aborted", "subagent_interrupted", "subagent_killed", "subagent_canceled", "subagent_timeout",
		"background_job_completed", "background_job_failed", "background_job_aborted", "background_job_interrupted",
		"background_job_killed", "background_job_canceled", "background_job_timeout",
		"background_job_delivery_pending", "background_job_delivery_retry", "background_job_delivery_delivered",
		"background_job_delivery_skipped", "background_job_delivery_dead_letter",
		"background_job_auto_continue_starting", "background_job_auto_continue_started",
		"background_job_auto_continue_skipped", "background_job_auto_continue_failed",
	} {
		tests = append(tests, map[string]any{"kind": "pipeline_stage", "stage": stage})
	}
	for _, event := range tests {
		if err := ValidateNewEventLifecycleV1(event); err != nil {
			t.Fatalf("closed lifecycle was rejected: event=%#v err=%v", event, err)
		}
	}
}

func TestValidateNewEventLifecycleV1RejectsAnythingOutsideClosedSet(t *testing.T) {
	tests := []struct {
		name  string
		event map[string]any
	}{
		{name: "nil event", event: nil},
		{name: "unknown kind", event: map[string]any{"kind": "provider_fabricated_lifecycle"}},
		{name: "transport only kind", event: map[string]any{"kind": "heartbeat"}},
		{name: "non string kind", event: map[string]any{"kind": true}},
		{name: "trimmed kind", event: map[string]any{"kind": " usage"}},
		{name: "missing status", event: map[string]any{"kind": "thread_created"}},
		{name: "wrong status", event: map[string]any{"kind": "turn_completed", "status": "failed"}},
		{name: "non string status", event: map[string]any{"kind": "turn_completed", "status": true}},
		{name: "trimmed status", event: map[string]any{"kind": "turn_completed", "status": " completed "}},
		{name: "unexpected status", event: map[string]any{"kind": "usage", "status": "completed"}},
		{name: "unexpected stage", event: map[string]any{"kind": "usage", "stage": "response_received"}},
		{name: "missing stage", event: map[string]any{"kind": "pipeline_stage"}},
		{name: "non string stage", event: map[string]any{"kind": "pipeline_stage", "stage": true}},
		{name: "unknown stage", event: map[string]any{"kind": "pipeline_stage", "stage": "provider_invented"}},
		{name: "trimmed stage", event: map[string]any{"kind": "pipeline_stage", "stage": " response_received"}},
		{name: "pipeline status", event: map[string]any{"kind": "pipeline_stage", "stage": "response_received", "status": "completed"}},
		{name: "retired case recovery", event: map[string]any{"kind": "pipeline_stage", "stage": "case_fund_final_recovered"}},
		{name: "unknown child status", event: map[string]any{"kind": "pipeline_stage", "stage": "subagent_unknown"}},
		{name: "non terminal background status", event: map[string]any{"kind": "pipeline_stage", "stage": "background_job_running"}},
		{name: "empty child suffix", event: map[string]any{"kind": "pipeline_stage", "stage": "subagent_"}},
		{name: "child suffix leading space", event: map[string]any{"kind": "pipeline_stage", "stage": "subagent_ completed"}},
		{name: "child suffix leading tab", event: map[string]any{"kind": "pipeline_stage", "stage": "subagent_\tcompleted"}},
		{name: "background suffix leading space", event: map[string]any{"kind": "pipeline_stage", "stage": "background_job_ completed"}},
		{name: "delivery suffix leading space", event: map[string]any{"kind": "pipeline_stage", "stage": "background_job_delivery_ pending"}},
		{name: "auto continue suffix leading space", event: map[string]any{"kind": "pipeline_stage", "stage": "background_job_auto_continue_ starting"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateNewEventLifecycleV1(test.event)
			if !errors.Is(err, ErrInvalidEventLifecycleV1) {
				t.Fatalf("validation error=%v, want closed lifecycle rejection", err)
			}
		})
	}
}
