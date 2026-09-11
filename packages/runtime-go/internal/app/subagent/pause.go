package subagent

import (
	"fmt"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

const (
	DefaultTaskJobPauseRequestTimeout = 5 * time.Minute
	DefaultTaskJobResumeTokenTTL      = 24 * time.Hour
)

type TaskJobPauseRequest struct {
	JobID           string
	ClientRequestID string
	SourceTurnID    string
}

type TaskJobResumeRequest struct {
	JobID           string
	ClientRequestID string
	ResumeToken     string
	SourceTurnID    string
}

type TaskJobPauseValidation struct {
	Request domainjob.PauseRequest
	Code    string
	Reason  string
}

type TaskJobPauseStore interface {
	LoadChildRun(id string) (domainjob.Record, error)
	UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error)
	AddPauseRequest(id string, request domainjob.PauseRequest) (domainjob.Record, domainjob.PauseRequest, error)
	MarkPauseRequestPaused(id string, requestID string, pausedAt string, token domainjob.ResumeToken) (domainjob.Record, domainjob.PauseRequest, error)
	MarkPauseResumeRequested(id string, requestID string) (domainjob.Record, domainjob.PauseRequest, error)
	RollbackPauseResumeRequested(id string, requestID string) (domainjob.Record, domainjob.PauseRequest, error)
	MarkPauseRequestResumed(id string, requestID string, resumedAt string) (domainjob.Record, domainjob.PauseRequest, error)
	CompletePauseResume(id string, requestID string) (domainjob.Record, domainjob.PauseRequest, error)
	RejectPauseRequest(id string, request domainjob.PauseRequest, reason string) (domainjob.Record, domainjob.PauseRequest, error)
	ExpirePauseRequest(id string, requestID string, expiredAt string, reason string) (domainjob.Record, domainjob.PauseRequest, error)
}

func TaskJobPauseRequestFromArgs(args map[string]any) (TaskJobPauseRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobPauseRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobPauseRequest{
		JobID:           jobID,
		ClientRequestID: strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
		SourceTurnID:    strings.TrimSpace(firstNonEmptyAnyString(args["source_turn_id"], args["sourceTurnId"])),
	}, nil, false
}

func TaskJobResumeRequestFromArgs(args map[string]any) (TaskJobResumeRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobResumeRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobResumeRequest{
		JobID:           jobID,
		ClientRequestID: strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
		ResumeToken:     strings.TrimSpace(firstNonEmptyAnyString(args["resume_token"], args["resumeToken"])),
		SourceTurnID:    strings.TrimSpace(firstNonEmptyAnyString(args["source_turn_id"], args["sourceTurnId"])),
	}, nil, false
}

func ValidateTaskJobPause(record domainjob.Record, parentThreadID string, request TaskJobPauseRequest, now time.Time) TaskJobPauseValidation {
	parentThreadID = strings.TrimSpace(parentThreadID)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	pauseRequest := BuildTaskJobPauseRequest(record, parentThreadID, request, now)
	if !TaskJobRecordAllowed(record, parentThreadID) {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorForbidden, Reason: "task job does not belong to parent thread"}
	}
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.ID) != strings.TrimSpace(request.JobID) {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorNotFound, Reason: "task job not found: " + strings.TrimSpace(request.JobID)}
	}
	if !JobIsSubagent(record) || strings.TrimSpace(record.ChildThreadID) == "" {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorValidation, Reason: "task job has no child run lineage"}
	}
	if !record.Background {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorValidation, Reason: "foreground child runs cannot be paused"}
	}
	if TaskJobTerminal(record) {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorConflict, Reason: "task job is terminal: " + strings.TrimSpace(record.Status)}
	}
	switch strings.TrimSpace(record.Status) {
	case string(domainjob.StatusQueued), string(domainjob.StatusRunning):
	default:
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorConflict, Reason: "task job cannot pause while status is " + strings.TrimSpace(record.Status)}
	}
	return TaskJobPauseValidation{Request: pauseRequest}
}

func ValidateTaskJobResume(record domainjob.Record, parentThreadID string, request TaskJobResumeRequest, now time.Time) TaskJobPauseValidation {
	parentThreadID = strings.TrimSpace(parentThreadID)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	pauseRequest := LatestPausedRequest(record)
	if strings.TrimSpace(pauseRequest.ID) == "" {
		pauseRequest = BuildTaskJobPauseRequest(record, parentThreadID, TaskJobPauseRequest{JobID: request.JobID, ClientRequestID: request.ClientRequestID, SourceTurnID: request.SourceTurnID}, now)
	}
	if !TaskJobRecordAllowed(record, parentThreadID) {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorForbidden, Reason: "task job does not belong to parent thread"}
	}
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.ID) != strings.TrimSpace(request.JobID) {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorNotFound, Reason: "task job not found: " + strings.TrimSpace(request.JobID)}
	}
	if !JobIsSubagent(record) || strings.TrimSpace(record.ChildThreadID) == "" {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorValidation, Reason: "task job has no child run lineage"}
	}
	if !record.Background {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorValidation, Reason: "foreground child runs cannot be resumed"}
	}
	if TaskJobTerminal(record) {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorConflict, Reason: "task job is terminal: " + strings.TrimSpace(record.Status)}
	}
	if strings.TrimSpace(record.Status) != string(domainjob.StatusPaused) {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorConflict, Reason: "task job cannot resume while status is " + strings.TrimSpace(record.Status)}
	}
	if strings.TrimSpace(pauseRequest.ID) == "" || strings.TrimSpace(pauseRequest.Status) != "paused" {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorConflict, Reason: "task job has no paused request"}
	}
	if token := strings.TrimSpace(request.ResumeToken); token != "" && token != strings.TrimSpace(pauseRequest.ResumeToken) {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorForbidden, Reason: "resume token mismatch"}
	}
	if expiresAt := ParseTaskJobTime(pauseRequest.ResumeTokenExpiresAt); !expiresAt.IsZero() && now.After(expiresAt) {
		return TaskJobPauseValidation{Request: pauseRequest, Code: TaskJobErrorConflict, Reason: "resume token expired"}
	}
	return TaskJobPauseValidation{Request: pauseRequest}
}

func BuildTaskJobPauseRequest(record domainjob.Record, parentThreadID string, request TaskJobPauseRequest, now time.Time) domainjob.PauseRequest {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	requestID := strings.TrimSpace(request.ClientRequestID)
	if requestID == "" {
		requestID = "pause_" + contracts.SafeRecordID(record.ID) + "_" + fmt.Sprintf("%d", now.UnixNano())
	} else {
		requestID = contracts.SafeRecordID(requestID)
	}
	return domainjob.PauseRequest{
		ID:             requestID,
		ParentThreadID: strings.TrimSpace(parentThreadID),
		ChildRunID:     strings.TrimSpace(record.ID),
		JobID:          strings.TrimSpace(record.ID),
		Status:         "requested",
		RequestedAt:    now.UTC().Format(time.RFC3339Nano),
		SourceTurnID:   strings.TrimSpace(request.SourceTurnID),
	}
}

func BuildResumeToken(record domainjob.Record, pauseRequest domainjob.PauseRequest, now time.Time) domainjob.ResumeToken {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return domainjob.ResumeToken{
		ResumeToken:    "resume_" + contracts.SafeRecordID(record.ID) + "_" + contracts.SafeRecordID(pauseRequest.ID) + "_" + fmt.Sprintf("%d", now.UnixNano()),
		IssuedAt:       now.UTC().Format(time.RFC3339Nano),
		ExpiresAt:      now.Add(DefaultTaskJobResumeTokenTTL).UTC().Format(time.RFC3339Nano),
		ChildRunID:     strings.TrimSpace(record.ID),
		ParentThreadID: strings.TrimSpace(record.ParentThreadID),
	}
}

func LatestPauseRequest(record domainjob.Record) domainjob.PauseRequest {
	var out domainjob.PauseRequest
	for _, request := range record.PauseRequests {
		if strings.TrimSpace(request.ID) == "" {
			continue
		}
		if out.ID == "" || request.RequestedAt >= out.RequestedAt {
			out = request
		}
	}
	return out
}

func LatestRequestedPause(record domainjob.Record) domainjob.PauseRequest {
	var out domainjob.PauseRequest
	for _, request := range record.PauseRequests {
		if strings.TrimSpace(request.Status) != "requested" {
			continue
		}
		if out.ID == "" || request.RequestedAt >= out.RequestedAt {
			out = request
		}
	}
	return out
}

func LatestPausedRequest(record domainjob.Record) domainjob.PauseRequest {
	var out domainjob.PauseRequest
	for _, request := range record.PauseRequests {
		if strings.TrimSpace(request.Status) != "paused" {
			continue
		}
		if out.ID == "" || request.PausedAt >= out.PausedAt {
			out = request
		}
	}
	return out
}

func AddPauseStateMetadata(out map[string]any, record domainjob.Record) {
	out["canPause"] = record.PauseState.CanPause
	out["canResume"] = record.PauseState.CanResume
	out["paused"] = record.PauseState.Paused
	if strings.TrimSpace(record.PauseState.PauseRequestID) != "" {
		out["pauseRequestId"] = record.PauseState.PauseRequestID
	}
	if strings.TrimSpace(record.PauseState.Status) != "" {
		out["pauseStatus"] = domainjob.PublicPauseStatusV1(record.PauseState.Status)
	}
	if record.PauseState.PauseCount > 0 {
		out["pauseCount"] = float64(record.PauseState.PauseCount)
	}
	if strings.TrimSpace(record.PauseState.RequestedAt) != "" {
		out["pauseRequestedAt"] = record.PauseState.RequestedAt
	}
	if strings.TrimSpace(record.PauseState.LastPausedAt) != "" {
		out["lastPausedAt"] = record.PauseState.LastPausedAt
	}
	if strings.TrimSpace(record.PauseState.LastResumedAt) != "" {
		out["lastResumedAt"] = record.PauseState.LastResumedAt
	}
	if strings.TrimSpace(record.PauseState.ResumeTokenIssuedAt) != "" {
		out["resumeTokenIssuedAt"] = record.PauseState.ResumeTokenIssuedAt
	}
	if strings.TrimSpace(record.PauseState.ResumeTokenExpiresAt) != "" {
		out["resumeTokenExpiresAt"] = record.PauseState.ResumeTokenExpiresAt
	}
}

func TaskJobPauseResponse(record domainjob.Record, request domainjob.PauseRequest, status string, reason string) map[string]any {
	status = strings.TrimSpace(status)
	if status == "" {
		status = strings.TrimSpace(request.Status)
	}
	status = domainjob.PublicPauseStatusV1(status)
	if ChildOutputRequiresWithholding(record) {
		out := map[string]any{
			"kind":           "task_job_pause_control",
			"status":         status,
			"state":          domainjob.PublicStatusV1(record.Status),
			"jobId":          strings.TrimSpace(record.ID),
			"childRunId":     strings.TrimSpace(record.ID),
			"pauseRequestId": strings.TrimSpace(request.ID),
			"requested":      status == "requested",
			"paused":         status == "paused",
			"rejected":       status == "rejected",
			"expired":        status == "expired",
			"resumed":        status == "resumed" || status == "resume_requested",
			"outputWithheld": true,
		}
		if request.RequestedAt != "" {
			out["requestedAt"] = request.RequestedAt
		}
		if request.PausedAt != "" {
			out["pausedAt"] = request.PausedAt
		}
		if request.ResumedAt != "" {
			out["resumedAt"] = request.ResumedAt
		}
		if request.ResumeTokenIssuedAt != "" {
			out["resumeTokenIssuedAt"] = request.ResumeTokenIssuedAt
		}
		if request.ResumeTokenExpiresAt != "" {
			out["resumeTokenExpiresAt"] = request.ResumeTokenExpiresAt
		}
		if reason = ProjectOrdinaryJobText(reason); reason != "" {
			out["reason"] = reason
		}
		return out
	}
	out := map[string]any{
		"status":         status,
		"state":          domainjob.PublicStatusV1(record.Status),
		"jobId":          record.ID,
		"childRunId":     record.ID,
		"childThreadId":  record.ChildThreadID,
		"childTurnId":    record.ChildTurnID,
		"pauseRequestId": request.ID,
		"requested":      status == "requested",
		"paused":         status == "paused",
		"rejected":       status == "rejected",
		"expired":        status == "expired",
		"resumed":        status == "resumed",
		"canPause":       record.PauseState.CanPause,
		"canResume":      record.PauseState.CanResume,
	}
	if request.RequestedAt != "" {
		out["requestedAt"] = request.RequestedAt
	}
	if request.PausedAt != "" {
		out["pausedAt"] = request.PausedAt
	}
	if request.ResumedAt != "" {
		out["resumedAt"] = request.ResumedAt
	}
	if request.ResumeToken != "" {
		out["resumeToken"] = request.ResumeToken
	}
	if request.ResumeTokenIssuedAt != "" {
		out["resumeTokenIssuedAt"] = request.ResumeTokenIssuedAt
	}
	if request.ResumeTokenExpiresAt != "" {
		out["resumeTokenExpiresAt"] = request.ResumeTokenExpiresAt
	}
	if reason = ProjectOrdinaryJobText(reason); reason != "" {
		out["reason"] = reason
	}
	return out
}

type ChildPauseEventInput struct {
	Record  domainjob.Record
	Request domainjob.PauseRequest
	Status  string
	Reason  string
}

type RuntimeEventRecorder func(map[string]any, string)
type ChildPauseEventRecorder func(domainjob.Record, domainjob.PauseRequest, string, string)

func BindChildPauseEventRecorder(recorder RuntimeEventRecorder) ChildPauseEventRecorder {
	if recorder == nil {
		return nil
	}
	return func(record domainjob.Record, request domainjob.PauseRequest, status string, reason string) {
		RecordChildPauseEvent(recorder, record, request, status, reason)
	}
}

func RecordChildPauseEvent(recorder RuntimeEventRecorder, record domainjob.Record, request domainjob.PauseRequest, status string, reason string) {
	if recorder == nil {
		return
	}
	status = domainjob.PublicPauseStatusV1(status)
	recorder(BuildChildPauseEvent(ChildPauseEventInput{Record: record, Request: request, Status: status, Reason: reason}), "job pause "+status)
}

func RecordTaskJobPauseResult(result TaskJobServiceResult, defaultStatus string, recorder RuntimeEventRecorder) TaskJobServiceResult {
	if recorder == nil || strings.TrimSpace(result.Record.ID) == "" {
		return result
	}
	status := strings.TrimSpace(defaultStatus)
	if status == "" {
		status = "requested"
	}
	if result.IsError {
		status = "rejected"
	}
	RecordChildPauseEvent(recorder, result.Record, LatestPauseRequest(result.Record), status, firstNonEmptyAnyString(result.Response["error"], result.Response["reason"]))
	return result
}

func BuildChildPauseEvent(input ChildPauseEventInput) map[string]any {
	record := input.Record
	request := input.Request
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = strings.TrimSpace(request.Status)
	}
	status = domainjob.PublicPauseStatusV1(status)
	kind := "child_pause_" + status
	switch status {
	case "requested":
		kind = "child_pause_requested"
	case "paused":
		kind = "child_paused"
	case "resume_requested":
		kind = "child_resume_requested"
	case "resumed":
		kind = "child_resumed"
	case "rejected", "expired":
		kind = "child_pause_rejected"
	}
	child := EventChild(record, record.Status, "")
	AddPauseStateMetadata(child, record)
	child["pauseRequestId"] = request.ID
	child["pauseStatus"] = status
	reason := ProjectOrdinaryJobText(input.Reason)
	if reason != "" {
		child["lastPauseReason"] = reason
	}
	return map[string]any{
		"kind":           kind,
		"threadId":       record.ParentThreadID,
		"turnId":         record.ParentTurnID,
		"itemId":         record.ParentToolItemID,
		"callId":         record.ParentToolCallID,
		"toolName":       JobDefaultToolName(record),
		"status":         status,
		"jobId":          record.ID,
		"childRunId":     record.ID,
		"childThreadId":  record.ChildThreadID,
		"childTurnId":    record.ChildTurnID,
		"pauseRequestId": request.ID,
		"parentThreadId": record.ParentThreadID,
		"sourceTurnId":   request.SourceTurnID,
		"createdAt":      request.RequestedAt,
		"pausedAt":       request.PausedAt,
		"resumedAt":      request.ResumedAt,
		"reason":         reason,
		"child":          child,
	}
}
