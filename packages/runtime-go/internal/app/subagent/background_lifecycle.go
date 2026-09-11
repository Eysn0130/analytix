package subagent

import (
	"strings"

	"analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func BackgroundJobCompletionItemID(turnID string, event map[string]any) string {
	if event == nil {
		return ""
	}
	stage := strings.TrimSpace(firstNonEmptyAnyString(event["stage"]))
	if !strings.HasPrefix(stage, "background_job_") {
		return ""
	}
	status := normalizedChildStatus(strings.TrimPrefix(stage, "background_job_"))
	stage = "background_job_" + status
	details, _ := event["details"].(map[string]any)
	if details == nil || details["notificationKind"] != "background_job_completion" {
		return ""
	}
	jobID := strings.TrimSpace(firstNonEmptyAnyString(details["jobId"], details["childRunId"]))
	if jobID == "" {
		return ""
	}
	return "item_progress_" + contracts.SafeRecordID(turnID) + "_" + contracts.SafeRecordID(stage) + "_" + contracts.SafeRecordID(jobID)
}

func BackgroundJobCompletionDeliveryID(turnID string, event map[string]any) string {
	jobID := BackgroundJobIDFromEvent(event)
	if strings.TrimSpace(turnID) == "" || jobID == "" {
		return ""
	}
	return "delivery_" + contracts.SafeRecordID(turnID) + "_" + contracts.SafeRecordID(jobID)
}

func BackgroundJobIDFromEvent(event map[string]any) string {
	if event == nil {
		return ""
	}
	details, _ := event["details"].(map[string]any)
	if details == nil || details["notificationKind"] != "background_job_completion" {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyAnyString(details["jobId"], details["childRunId"]))
}

func BackgroundCompletionDeliveryItemID(turnID, deliveryID, status string) string {
	turnID = strings.TrimSpace(turnID)
	deliveryID = strings.TrimSpace(deliveryID)
	status = normalizedBackgroundDeliveryStatus(status)
	if turnID == "" || deliveryID == "" || status == "" {
		return ""
	}
	return "item_progress_" + contracts.SafeRecordID(turnID) + "_background_job_delivery_" + contracts.SafeRecordID(status) + "_" + contracts.SafeRecordID(deliveryID)
}

func BackgroundCompletionDeliveryCallID(deliveryID, status string) string {
	return "background_delivery_" + contracts.SafeRecordID(normalizedBackgroundDeliveryStatus(status)) + "_" + contracts.SafeRecordID(deliveryID)
}

func BackgroundAutoContinueTerminalStatus(status string) bool {
	return domainjob.TerminalStatusV1(status)
}

func BackgroundAutoContinueItemID(turnID, jobID, status string) string {
	turnID = strings.TrimSpace(turnID)
	jobID = strings.TrimSpace(jobID)
	status = domainjob.PublicAutoContinueStatusV1(status)
	if turnID == "" || jobID == "" || status == "" {
		return ""
	}
	return "item_progress_" + contracts.SafeRecordID(turnID) + "_background_job_auto_continue_" + contracts.SafeRecordID(status) + "_" + contracts.SafeRecordID(jobID)
}

func BackgroundAutoContinueCallID(jobID, status string) string {
	status = normalizedBackgroundAutoContinueStatus(status)
	return "background_auto_continue_" + contracts.SafeRecordID(status) + "_" + contracts.SafeRecordID(jobID)
}

func BackgroundAutoContinueStatusMessage(status, reason, errorText string) string {
	status = normalizedBackgroundAutoContinueStatus(status)
	reason = domainjob.NormalizeOperationalReasonV1(reason)
	errorText = ""
	switch status {
	case "starting":
		return "background job is preparing to wake the parent thread"
	case "started":
		return "background job woke the parent thread"
	case "skipped":
		if detail := strings.TrimSpace(firstNonEmptyAnyString(reason, errorText)); detail != "" {
			return "background job did not auto-continue the parent: " + detail
		}
		return "background job did not auto-continue the parent"
	case "failed":
		if detail := strings.TrimSpace(firstNonEmptyAnyString(errorText, reason)); detail != "" {
			return "background job auto-continue failed: " + detail
		}
		return "background job auto-continue failed"
	default:
		return "background job auto-continue status unavailable"
	}
}

func normalizedBackgroundAutoContinueStatus(status string) string {
	status = domainjob.PublicAutoContinueStatusV1(status)
	if status == "" {
		return domainjob.PublicOperationalStatusUnknownV1
	}
	return status
}
