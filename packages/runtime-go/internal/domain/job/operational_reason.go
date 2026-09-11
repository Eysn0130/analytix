package job

import "strings"

const OperationalReasonWithheld = "job_operational_reason_withheld"

// NormalizeOperationalReasonV1 admits only host-owned control-plane codes.
// Arbitrary user, provider, process, filesystem, and error text must never be
// persisted as a job reason or reflected through progress events.
func NormalizeOperationalReasonV1(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch value {
	case "already_terminal",
		"append_parent_turn_failed",
		"auto_continue_already_started",
		"auto_continue_already_starting",
		"auto_continue_reservation_unavailable",
		"auto_continue_start_failed",
		"auto_continue_starter_unavailable",
		"auto_continue_turn_conflict",
		"build_notification_item_failed",
		"child_completion_not_continuable",
		"child_completion_receipt_invalid",
		"child_run_terminal",
		"child_completion_receipt_required",
		"child_completion_receipt_unexpected",
		"completion_delivery_dead_letter",
		"completion_delivery_not_delivered",
		"completion_delivery_recovered",
		"completion_delivery_retry",
		"duplicate_delivery",
		"duplicate_suppressed",
		"job_active",
		"job_auto_continue_state_changed",
		"job_delegated_tool_manifest_invalid",
		"job_not_completed",
		"job_record_missing",
		"job_security_authority_unavailable",
		"job_security_binding_missing",
		"late_completion_suppressed",
		"legacy_private_steer_removed",
		"lease_expired",
		"manual_recover_lease_expired",
		"manual_recover_orphaned",
		"manual_recover_stale",
		"manual_recovery",
		"missing_parent_lineage",
		"orphaned",
		"parent_execution_grant_expired",
		"parent_execution_grant_expiry_invalid",
		"parent_execution_grant_inactive",
		"parent_execution_grant_mismatch",
		"parent_execution_grant_missing",
		"parent_execution_grant_registry_invalid",
		"parent_missing",
		"parent_has_pending_gate",
		"parent_not_idle",
		"parent_security_context_mismatch",
		"parent_thread_archived",
		"parent_thread_missing",
		"parent_tool_identity_invalid",
		"parent_tool_result_invalid",
		"parent_turn_missing",
		"parent_turn_not_completed",
		"parent_turn_not_latest",
		"runtime_restart_orphaned_running_job",
		"runtime_startup_recovery",
		"runtime_store_unavailable",
		"superseded",
		"turn_missing":
		return value
	default:
		return OperationalReasonWithheld
	}
}
