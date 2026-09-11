package subagent

import (
	"strings"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
)

const (
	untrustedChildOutputStatus = "untrusted_child_output"
	securityBoundOutputMessage = "security-bound child output withheld"
)

func SecurityBoundChildOutput(record domainjob.Record) bool {
	return record.SecurityBinding != nil
}

// ChildOutputRequiresWithholding is the public-output boundary. Job kind,
// lineage markers, and a missing binding are not positive output authority.
// Task output remains host-internal until a later evidence/publication
// contract introduces a separate, affirmative public projection.
func ChildOutputRequiresWithholding(domainjob.Record) bool {
	return true
}

func SecurityBoundChildOutputProjection(record domainjob.Record) map[string]any {
	return ChildOutputMetadataProjection(record)
}

// ChildOutputMetadataProjection is safe for current and legacy job records.
// It intentionally withholds Output/Error/artifact and every fact-bearing
// child payload even when the historical record predates SecurityBindingV2.
func ChildOutputMetadataProjection(record domainjob.Record) map[string]any {
	out := securityBoundChildOutputProjection(record.ID, record.Status, record.Background)
	if receipt := record.ForegroundChildHandoffReceipt; receipt != nil && domainjob.ValidateForegroundChildHandoffReceiptV1(*receipt) == nil {
		out["handoffStatus"] = receipt.Status
		out["handoffReceiptDigest"] = receipt.ReceiptDigest
		out["submissionDigest"] = receipt.SubmissionDigest
		out["privacyProjectionDigest"] = receipt.PrivacyProjectionDigest
	}
	return out
}

func ChildOutputMetadataRecordsAny(records []domainjob.Record) []any {
	out := make([]any, 0, len(records))
	for _, record := range records {
		out = append(out, TaskJobMetadataProjectionV1(record, nil))
	}
	return out
}

func PublicJobLifecycleRecordV1(record domainjob.Record) domainjob.Record {
	record.Status = domainjob.PublicStatusV1(record.Status)
	record.Kind = domainjob.PublicKindV1(record.Kind)
	record.AutoContinueStatus = domainjob.PublicAutoContinueStatusV1(record.AutoContinueStatus)
	record.CompletionDeliveryStatus = domainjob.PublicCompletionDeliveryStatusV1(record.CompletionDeliveryStatus)
	record.RecoveryStatus = domainjob.PublicRecoveryStatusV1(record.RecoveryStatus)
	record.PauseState.Status = publicOptionalPauseStatusV1(record.PauseState.Status)
	if len(record.PauseRequests) > 0 {
		record.PauseRequests = append([]domainjob.PauseRequest(nil), record.PauseRequests...)
		for index := range record.PauseRequests {
			record.PauseRequests[index].Status = domainjob.PublicPauseStatusV1(record.PauseRequests[index].Status)
		}
	}
	if len(record.Steers) > 0 {
		record.Steers = append([]domainjob.SteerMessage(nil), record.Steers...)
		for index := range record.Steers {
			record.Steers[index].Status = domainjob.PublicSteerStatusV1(record.Steers[index].Status)
		}
	}
	return record
}

func publicOptionalPauseStatusV1(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return domainjob.PublicPauseStatusV1(value)
}

func SecurityBoundOutputMap(output map[string]any) bool {
	if output == nil {
		return false
	}
	if withheld, _ := output["outputWithheld"].(bool); withheld {
		return true
	}
	if strings.TrimSpace(firstNonEmptyAnyString(output["outputTrustStatus"])) == untrustedChildOutputStatus {
		return true
	}
	canRead, hasCanRead := output["canReadOutput"].(bool)
	return hasCanRead && !canRead
}

// ProjectOrdinaryJobMap is the single projection for readable background-job
// output. Security-bound child output uses the stricter closed allowlist above.
func ProjectOrdinaryJobMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	projected := domainordinary.ProjectValueV1(value)
	out, _ := projected.(map[string]any)
	public, ok := domainevent.SanitizePublicValue(out, false)
	if !ok {
		return nil
	}
	sanitized, _ := public.(map[string]any)
	return sanitized
}

func ProjectOrdinaryJobText(value string) string {
	public, err := domainevent.FilterPublicText(value)
	if err != nil {
		return ""
	}
	return domainordinary.ProjectTextV1(public)
}

func PublicChildOutputProjection(output map[string]any) map[string]any {
	if !SecurityBoundOutputMap(output) {
		return output
	}
	id := strings.TrimSpace(firstNonEmptyAnyString(output["jobId"], output["childRunId"], output["childId"], output["id"]))
	status := strings.TrimSpace(firstNonEmptyAnyString(output["status"], output["childStatus"]))
	background, _ := output["background"].(bool)
	return securityBoundChildOutputProjection(id, status, background)
}

func securityBoundChildOutputProjection(id string, status string, background bool) map[string]any {
	id = publicTaskJobReferenceIDV1(id)
	status = normalizedChildStatus(status)
	return map[string]any{
		"kind":              "subagent_task",
		"id":                id,
		"childId":           id,
		"childRunId":        id,
		"jobId":             id,
		"status":            status,
		"childStatus":       status,
		"background":        background,
		"terminal":          taskJobTerminalStatus(status),
		"outputWithheld":    true,
		"outputTrustStatus": untrustedChildOutputStatus,
		"factAnswerAllowed": false,
		"evidenceAuthority": false,
		"canReadOutput":     false,
		"canContinueParent": false,
	}
}

func normalizedChildStatus(status string) string {
	return domainjob.PublicStatusV1(status)
}

func normalizedToolProgressStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "running", "success", "error":
		return strings.TrimSpace(status)
	case string(domainjob.StatusQueued), string(domainjob.StatusPauseRequested), string(domainjob.StatusPaused),
		string(domainjob.StatusResumeRequested), string(domainjob.StatusResuming):
		return "running"
	case string(domainjob.StatusCompleted):
		return "success"
	case string(domainjob.StatusFailed), string(domainjob.StatusAborted), string(domainjob.StatusInterrupted),
		string(domainjob.StatusKilled), string(domainjob.StatusCanceled), string(domainjob.StatusTimeout):
		return "error"
	default:
		return "unknown"
	}
}
