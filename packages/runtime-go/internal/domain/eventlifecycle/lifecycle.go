// Package eventlifecycle owns the closed lifecycle vocabulary for newly
// persisted runtime events. Durable replay is intentionally outside this
// package: legacy records may remain readable without becoming valid inputs
// for a new append.
package eventlifecycle

import (
	"errors"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

var ErrInvalidEventLifecycleV1 = errors.New("new event lifecycle is outside the closed V1 allowlist")

// ValidateNewEventLifecycleV1 validates lifecycle control fields before a new
// runtime event is persisted. Callers must invoke it before acquiring a write
// lock or creating any durable event-log state.
func ValidateNewEventLifecycleV1(event map[string]any) error {
	kind, kindPresent, kindExact := exactStringV1(event, "kind")
	if !kindPresent || !kindExact || kind == "" {
		return invalidV1("event kind is missing or invalid")
	}
	if _, stagePresent := event["stage"]; stagePresent && kind != "pipeline_stage" {
		return invalidV1("event stage is not valid for its kind")
	}

	switch kind {
	case "thread_created":
		return requireStatusV1(event, "idle")
	case "thread_updated":
		return requireStatusV1(event, "idle", string(domainjob.StatusRunning), "archived", "deleted")
	case "turn_started":
		return requireStatusV1(event, string(domainjob.StatusRunning))
	case "turn_completed":
		return requireStatusV1(event, string(domainjob.StatusCompleted))
	case "turn_failed":
		return requireStatusV1(event, string(domainjob.StatusFailed))
	case "turn_aborted":
		return requireStatusV1(event, string(domainjob.StatusAborted))
	case "tool_progress":
		return requireStatusV1(event, string(domainjob.StatusRunning), "success", "error")
	case "tool_result_upload_wait":
		return requireStatusV1(event, "waiting")
	case "approval_requested", "user_input_requested":
		return requireStatusV1(event, "pending")
	case "approval_resolved":
		return requireStatusV1(event, "allowed", "denied", "expired")
	case "user_input_resolved":
		return requireStatusV1(event, "submitted", "cancelled")
	case "child_steer_queued":
		return requireStatusV1(event, string(domainjob.StatusQueued))
	case "child_steer_admitted":
		return requireStatusV1(event, "admitted")
	case "child_steer_rejected":
		return requireStatusV1(event, "rejected")
	case "child_pause_requested":
		return requireStatusV1(event, "requested")
	case "child_paused":
		return requireStatusV1(event, string(domainjob.StatusPaused))
	case "child_resume_requested":
		return requireStatusV1(event, string(domainjob.StatusResumeRequested))
	case "child_resumed":
		return requireStatusV1(event, "resumed")
	case "child_pause_rejected":
		return requireStatusV1(event, "rejected", "expired")
	case "pipeline_stage":
		if err := forbidStatusV1(event); err != nil {
			return err
		}
		stage, stagePresent, stageExact := exactStringV1(event, "stage")
		if !stagePresent || !stageExact || !validPipelineStageV1(stage) {
			return invalidV1("pipeline stage is missing or invalid")
		}
		return nil
	case "thread_rewound",
		"turn_steered",
		"item_created", "item_updated", "item_completed",
		"tool_call_started", "tool_call_finished", "tool_call_ready",
		"tool_storm_suppressed", "tool_catalog_changed",
		"compaction_started", "compaction_completed",
		"goal_updated", "goal_cleared", "goal_evidence_audit",
		"todos_updated", "todos_cleared", "autoresearch_state_audit",
		"checkpoint_captured", "checkpoint_rewind_rescue_created", "checkpoint_rewind_applied",
		"usage", "execution_grant_approved",
		// Store compatibility helpers are explicit members of the closed set;
		// arbitrary legacy kinds are never inferred from replay data.
		"thread_archived", "eventlog_compacted":
		return forbidStatusV1(event)
	default:
		return invalidV1("event kind is unknown")
	}
}

func exactStringV1(record map[string]any, key string) (string, bool, bool) {
	value, present := record[key]
	if !present {
		return "", false, false
	}
	text, ok := value.(string)
	if !ok || text != strings.TrimSpace(text) {
		return "", true, false
	}
	return text, true, true
}

func requireStatusV1(event map[string]any, allowed ...string) error {
	status, present, exact := exactStringV1(event, "status")
	if !present || !exact || status == "" {
		return invalidV1("event status is missing or invalid")
	}
	for _, candidate := range allowed {
		if status == candidate {
			return nil
		}
	}
	return invalidV1("event status is invalid for its kind")
}

func forbidStatusV1(event map[string]any) error {
	if _, present := event["status"]; present {
		return invalidV1("event status is not valid for its kind")
	}
	return nil
}

func validPipelineStageV1(stage string) bool {
	switch stage {
	case "setup", "pre_start", "post_start", "input_received", "input_cached", "input_routed",
		"input_compressed", "input_remembered", "pre_send", "post_send", "response_received",
		"provider_admission_rejected",
		"provider_retrying", "step_limit_finalizing", "provider_error", "empty_final_recovered", "loop_guard":
		return true
	}
	if exactDynamicSuffixV1(stage, "subagent_", domainjob.ValidStatusV1) {
		return true
	}
	if exactDynamicSuffixV1(stage, "background_job_delivery_", domainjob.ValidCompletionDeliveryStatusV1) {
		return true
	}
	if exactDynamicSuffixV1(stage, "background_job_auto_continue_", domainjob.ValidAutoContinueStatusV1) {
		return true
	}
	return exactDynamicSuffixV1(stage, "background_job_", domainjob.TerminalStatusV1)
}

func exactDynamicSuffixV1(stage, prefix string, valid func(string) bool) bool {
	if !strings.HasPrefix(stage, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(stage, prefix)
	return suffix != "" && suffix == strings.TrimSpace(suffix) && valid(suffix)
}

func invalidV1(detail string) error {
	return errors.Join(ErrInvalidEventLifecycleV1, errors.New(detail))
}
