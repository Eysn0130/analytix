package subagent

import (
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

const DefaultTaskJobWaitTimeoutMS = 60000

type TaskJobWaitRequest struct {
	JobIDs    []string
	TimeoutMS int
}

type TaskJobOutputRequest struct {
	JobID          string
	Offset         int
	Limit          int
	Filter         string
	ExplicitOffset bool
	Tail           bool
	Since          string
}

type TaskJobKillRequest struct {
	JobID  string
	Reason string
}

type TaskJobRestartRequest struct {
	JobID string
}

type TaskJobRecoverRequest struct {
	JobID      string
	DeliveryID string
	Reason     string
}

type TaskJobListRequest struct {
	Status     string
	Background *bool
	Limit      int
}

func TaskJobWaitRequestFromArgs(args map[string]any) TaskJobWaitRequest {
	timeoutMS := DefaultTaskJobWaitTimeoutMS
	if value, ok := numericAny(firstNonNilValue(args["timeout_seconds"], args["timeoutSeconds"])); ok {
		switch {
		case value <= 0:
			timeoutMS = 0
		case value > DefaultTaskJobWaitTimeoutMS/1000:
			timeoutMS = DefaultTaskJobWaitTimeoutMS
		default:
			timeoutMS = value * 1000
		}
	} else if value, ok := numericAny(firstNonNilValue(args["timeout_ms"], args["timeoutMs"])); ok {
		timeoutMS = value
	}
	if timeoutMS < 0 {
		timeoutMS = 0
	}
	if timeoutMS > DefaultTaskJobWaitTimeoutMS {
		timeoutMS = DefaultTaskJobWaitTimeoutMS
	}
	return TaskJobWaitRequest{
		JobIDs:    TaskJobIDsFromArgs(args),
		TimeoutMS: timeoutMS,
	}
}

func TaskJobIDsFromArgs(args map[string]any) []string {
	ids := []string{}
	ids = append(ids, stringList(args["jobIds"])...)
	ids = append(ids, stringList(args["job_ids"])...)
	for _, key := range []string{"jobId", "job_id", "id"} {
		if id := strings.TrimSpace(firstNonEmptyAnyString(args[key])); id != "" {
			ids = append(ids, id)
		}
	}
	return UniqueStringList(ids)
}

func TaskJobParentThreadIDFromArgs(args map[string]any) string {
	return strings.TrimSpace(firstNonEmptyAnyString(args["parentThreadId"], args["threadId"]))
}

func TaskJobOutputRequestFromArgs(args map[string]any) (TaskJobOutputRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobOutputRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	request := TaskJobOutputRequest{JobID: jobID}
	if value, ok := numericAny(args["offset"]); ok {
		request.Offset = value
		request.ExplicitOffset = true
	} else if value, ok := numericAny(args["cursor"]); ok {
		request.Offset = value
		request.ExplicitOffset = true
	}
	if value, ok := numericAny(args["limit"]); ok {
		request.Limit = value
	}
	request.Filter = strings.TrimSpace(firstNonEmptyAnyString(args["filter"]))
	request.Tail = boolField(args, "tail")
	request.Since = strings.TrimSpace(firstNonEmptyAnyString(args["since"]))
	return request, nil, false
}

func TaskJobKillRequestFromArgs(args map[string]any) (TaskJobKillRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobKillRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	reason := strings.TrimSpace(firstNonEmptyAnyString(args["reason"]))
	if reason == "" {
		reason = "killed by parent"
	}
	return TaskJobKillRequest{JobID: jobID, Reason: reason}, nil, false
}

func TaskJobRestartRequestFromArgs(args map[string]any) (TaskJobRestartRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobRestartRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobRestartRequest{JobID: jobID}, nil, false
}

func TaskJobRecoverRequestFromArgs(args map[string]any) (TaskJobRecoverRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobRecoverRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobRecoverRequest{
		JobID:      jobID,
		DeliveryID: strings.TrimSpace(firstNonEmptyAnyString(args["delivery_id"], args["deliveryId"])),
		Reason:     strings.TrimSpace(firstNonEmptyAnyString(args["reason"])),
	}, nil, false
}

func TaskJobListRequestFromArgs(args map[string]any) (TaskJobListRequest, map[string]any, bool) {
	request := TaskJobListRequest{
		Status: strings.TrimSpace(firstNonEmptyAnyString(args["status"])),
	}
	if request.Status != "" {
		if err := domainjob.ValidateStatusV1(request.Status); err != nil {
			return TaskJobListRequest{}, ValidationErrorResponse("status is outside the closed task-job lifecycle"), true
		}
	}
	if value, ok := args["background"].(bool); ok {
		request.Background = &value
	}
	if value, ok := numericAny(args["limit"]); ok && value > 0 {
		request.Limit = value
	}
	return request, nil, false
}

func TaskJobRecordAllowed(record domainjob.Record, parentThreadID string) bool {
	parentThreadID = strings.TrimSpace(parentThreadID)
	return parentThreadID != "" && strings.TrimSpace(record.ParentThreadID) == parentThreadID
}

func TaskJobWaitResult(records []domainjob.Record, jobIDs []string, now time.Time, stalledAfter time.Duration) (map[string]any, bool) {
	if len(records) == 0 {
		return TaskJobNotFoundResponse(""), true
	}
	return map[string]any{"jobs": TaskJobRecordsAny(records, now, stalledAfter)}, false
}

func TaskJobOutputResult(record domainjob.Record, request TaskJobOutputRequest, cursor int, now time.Time, stalledAfter time.Duration) (map[string]any, int, error) {
	if ChildOutputRequiresWithholding(record) {
		return TaskJobOutputWithheldResponseV1(record), 0, nil
	}
	offset := request.Offset
	if !request.ExplicitOffset {
		offset = cursor
	}
	if request.Tail && !request.ExplicitOffset && request.Limit > 0 {
		outputBytes := len([]byte(record.Output))
		offset = outputBytes - request.Limit
		if offset < 0 {
			offset = 0
		}
	}
	if request.Since != "" {
		since, err := time.Parse(time.RFC3339Nano, request.Since)
		if err != nil {
			return nil, offset, err
		}
		lastActivity := ParseTaskJobTime(record.UpdatedAt)
		if record.Output != "" && !lastActivity.IsZero() && !lastActivity.After(since.UTC()) {
			offset = len([]byte(record.Output))
		}
	}
	view, err := TaskJobOutputViewWithFilter(record, offset, request.Limit, request.Filter, now, stalledAfter)
	if err != nil {
		return nil, offset, err
	}
	nextOffset := offset
	if value, ok := numericAny(view["nextOffset"]); ok {
		nextOffset = value
	}
	view["cursor"] = float64(offset)
	view["nextCursor"] = float64(nextOffset)
	view["cursorAdvanced"] = !request.ExplicitOffset
	if request.Tail {
		view["tail"] = true
	}
	if request.Since != "" {
		view["since"] = request.Since
	}
	return view, nextOffset, nil
}

func TaskJobKillResult(record domainjob.Record, now time.Time, stalledAfter time.Duration) map[string]any {
	return map[string]any{"job": TaskJobRecordView(record, now, stalledAfter)}
}

func TaskJobRecoverResult(record domainjob.Record, result string, reason string, now time.Time, stalledAfter time.Duration) map[string]any {
	if SecurityBoundChildOutput(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	result = normalizedTaskJobRecoverResultV1(result)
	out := map[string]any{
		"result":             result,
		"jobId":              record.ID,
		"threadId":           record.ParentThreadID,
		"parentThreadId":     record.ParentThreadID,
		"parentTurnId":       record.ParentTurnID,
		"childThreadId":      record.ChildThreadID,
		"childTurnId":        record.ChildTurnID,
		"status":             domainjob.PublicStatusV1(record.Status),
		"deliveryId":         record.CompletionDeliveryID,
		"deliveryStatus":     domainjob.PublicCompletionDeliveryStatusV1(record.CompletionDeliveryStatus),
		"autoContinueStatus": domainjob.PublicAutoContinueStatusV1(record.AutoContinueStatus),
		"autoContinueTurnId": record.AutoContinueTurnID,
		"recoveryAttempt":    float64(record.RecoveryAttempt),
		"job":                TaskJobRecordView(record, now, stalledAfter),
	}
	if reason = domainjob.NormalizeOperationalReasonV1(reason); reason != "" {
		out["reason"] = reason
	}
	if strings.TrimSpace(record.CompletionDeliveryReason) != "" {
		out["deliveryReason"] = domainjob.NormalizeOperationalReasonV1(record.CompletionDeliveryReason)
	}
	if strings.TrimSpace(record.LateCompletionReason) != "" {
		out["lateCompletionReason"] = domainjob.NormalizeOperationalReasonV1(record.LateCompletionReason)
	}
	if strings.TrimSpace(record.DeadLetterReason) != "" {
		out["deadLetterReason"] = domainjob.NormalizeOperationalReasonV1(record.DeadLetterReason)
	}
	return out
}

func normalizedTaskJobRecoverResultV1(value string) string {
	switch strings.TrimSpace(value) {
	case "recovered", "duplicate_suppressed", "skipped", "not_recoverable", "parent_missing", "turn_missing", "dead_lettered", "superseded", "already_terminal":
		return strings.TrimSpace(value)
	default:
		return "not_recoverable"
	}
}

func ValidationErrorResponse(_ string) map[string]any {
	return map[string]any{"code": "validation_error"}
}

func ForbiddenTaskJobResponse() map[string]any {
	return map[string]any{"code": "forbidden"}
}

func TaskJobNotFoundResponse(_ string) map[string]any {
	return map[string]any{"code": "not_found"}
}

func firstNonNilValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
