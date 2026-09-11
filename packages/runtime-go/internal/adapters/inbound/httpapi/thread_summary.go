package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadsummaryapp "analytix.local/runtime-go/internal/app/threadsummary"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

const maxThreadSummaryOutputLimit = 2000000

type ThreadSummaryService interface {
	Summary(threadID string) (map[string]any, error)
	LoadContext(threadID string) (threadsummaryapp.Context, error)
	CommandOutput(context threadsummaryapp.Context, taskID string, offset int, limit int) (map[string]any, bool)
	OutputTaskJob(parentThreadID string, request subagentapp.TaskJobOutputRequest, useCursor bool) subagentapp.TaskJobServiceResult
	KillTaskJob(parentThreadID string, request subagentapp.TaskJobKillRequest) subagentapp.TaskJobServiceResult
	RestartTaskJob(parentThreadID string, jobID string) subagentapp.TaskJobServiceResult
}

type ThreadSummaryHandlers struct {
	Service ThreadSummaryService
}

func (h ThreadSummaryHandlers) HandleThreadSummaryPath(w http.ResponseWriter, r *http.Request, rest string) {
	threadID, suffix, ok := strings.Cut(rest, "/summary")
	if !ok || strings.TrimSpace(threadID) == "" {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "thread_summary_service_missing", "message": "thread summary service missing"})
		return
	}
	if suffix == "" {
		h.handleSummary(w, r, threadID)
		return
	}
	taskID, action, ok := parseThreadSummaryTaskSuffix(suffix)
	if !ok {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
		return
	}
	if taskID == "" && action == "invalid-task-id" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid summary task id"})
		return
	}
	switch action {
	case "output":
		h.handleTaskOutput(w, r, threadID, taskID)
	case "kill":
		h.handleTaskKill(w, r, threadID, taskID)
	case "restart":
		h.handleTaskRestart(w, r, threadID, taskID)
	default:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}

func parseThreadSummaryTaskSuffix(suffix string) (string, string, bool) {
	if !strings.HasPrefix(suffix, "/tasks/") {
		return "", "", false
	}
	taskPath := strings.TrimPrefix(suffix, "/tasks/")
	encodedTaskID, action, ok := strings.Cut(taskPath, "/")
	if !ok || strings.TrimSpace(encodedTaskID) == "" {
		return "", "", false
	}
	taskID, err := url.PathUnescape(encodedTaskID)
	if err != nil {
		return "", "invalid-task-id", true
	}
	return taskID, action, true
}

func (h ThreadSummaryHandlers) handleSummary(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	summary, err := h.Service.Summary(threadID)
	if h.writeError(w, threadID, err) {
		return
	}
	if err := threadsummaryapp.ValidateSummaryResponseV1(summary, threadID); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"code": "thread_summary_schema_invalid", "message": "thread summary failed host schema validation",
		})
		return
	}
	WriteJSON(w, http.StatusOK, summary)
}

func (h ThreadSummaryHandlers) handleTaskOutput(w http.ResponseWriter, r *http.Request, threadID string, taskID string) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	context, ok := h.loadContext(w, threadID)
	if !ok {
		return
	}
	if context.CaseRestricted {
		h.writeCaseHistoryRestricted(w)
		return
	}
	offset := IntQuery(r.URL.Query(), "offset")
	limit := IntQuery(r.URL.Query(), "limit")
	if limit > maxThreadSummaryOutputLimit {
		limit = maxThreadSummaryOutputLimit
	}
	if jobID, ok := summaryTaskJobID(taskID); ok {
		h.handleTaskJobOutput(w, threadID, taskID, jobID, offset, limit)
		return
	}
	if output, ok := h.Service.CommandOutput(context, taskID, offset, limit); ok {
		if err := threadsummaryapp.ValidateTaskOutputResponseV1(output, taskID); err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{
				"code": "thread_summary_output_schema_invalid", "message": "summary task output failed host schema validation",
			})
			return
		}
		WriteJSON(w, http.StatusOK, output)
		return
	}
	WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "summary task not found: " + taskID})
}

func (h ThreadSummaryHandlers) handleTaskJobOutput(w http.ResponseWriter, threadID string, taskID string, jobID string, offset int, limit int) {
	result := h.Service.OutputTaskJob(threadID, subagentapp.TaskJobOutputRequest{
		JobID:  jobID,
		Offset: offset,
		Limit:  limit,
	}, false)
	if result.ErrorCode == subagentapp.TaskJobErrorNotFound && (result.Err == nil || errors.Is(result.Err, os.ErrNotExist)) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "summary task not found: " + taskID})
		return
	}
	if result.ErrorCode == subagentapp.TaskJobErrorNotFound {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": "summary task lookup failed"})
		return
	}
	if result.ErrorCode == subagentapp.TaskJobErrorForbidden {
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "summary task does not belong to this thread: " + taskID})
		return
	}
	if result.ErrorCode == subagentapp.TaskJobErrorValidation {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "summary task output request failed validation"})
		return
	}
	if !validSummaryTaskJobResultIdentityV1(result.Record, threadID, jobID) {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"code": "task_job_output_schema_invalid", "message": "summary task output failed host schema validation",
		})
		return
	}
	output := subagentapp.TaskJobOutputWithheldResponseV1(result.Record)
	if err := subagentapp.ValidateTaskJobOutputResponseV1(output); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"code": "task_job_output_schema_invalid", "message": "summary task output failed host schema validation",
		})
		return
	}
	response := threadSummaryTaskJobOutputWithheldResponseV1(taskID, stringField(output, "status"))
	if err := threadsummaryapp.ValidateTaskOutputResponseV1(response, taskID); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"code": "task_job_output_schema_invalid", "message": "summary task output failed host schema validation",
		})
		return
	}
	WriteJSON(w, http.StatusOK, response)
}

func threadSummaryTaskJobOutputWithheldResponseV1(taskID string, status string) map[string]any {
	return map[string]any{
		"schemaVersion":     1,
		"availability":      "withheld",
		"taskId":            taskID,
		"status":            domainjob.PublicStatusV1(status),
		"reasonCode":        "security_bound_child_output",
		"outputWithheld":    true,
		"outputTrustStatus": "untrusted_child_output",
		"factAnswerAllowed": false,
		"evidenceAuthority": false,
		"canReadOutput":     false,
		"canContinueParent": false,
	}
}

func (h ThreadSummaryHandlers) handleTaskKill(w http.ResponseWriter, r *http.Request, threadID string, taskID string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	jobID, ok := summaryTaskJobID(taskID)
	if !ok {
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": "summary task can not be killed by the runtime route: " + taskID})
		return
	}
	context, loaded := h.loadContext(w, threadID)
	if !loaded {
		return
	}
	if context.CaseRestricted {
		h.writeCaseHistoryRestricted(w)
		return
	}
	result := h.Service.KillTaskJob(threadID, subagentapp.TaskJobKillRequest{
		JobID:  jobID,
		Reason: "killed from thread summary",
	})
	if result.ErrorCode == subagentapp.TaskJobErrorNotFound && (result.Err == nil || errors.Is(result.Err, os.ErrNotExist)) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "summary task not found: " + taskID})
		return
	}
	if result.ErrorCode == subagentapp.TaskJobErrorNotFound {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": "summary task lookup failed"})
		return
	}
	if result.ErrorCode == subagentapp.TaskJobErrorForbidden {
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "summary task does not belong to this thread: " + taskID})
		return
	}
	if result.ErrorCode == subagentapp.TaskJobErrorValidation {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "summary task kill request failed validation"})
		return
	}
	if !validSummaryTaskJobResultIdentityV1(result.Record, threadID, jobID) {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"code": "thread_summary_mutation_schema_invalid", "message": "summary task mutation failed host schema validation",
		})
		return
	}
	response := map[string]any{"task": threadSummaryTaskJobMetadataV1(result.Record)}
	if err := threadsummaryapp.ValidateTaskMutationResponseV1(response, "taskjob:"+jobID); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"code": "thread_summary_mutation_schema_invalid", "message": "summary task mutation failed host schema validation",
		})
		return
	}
	WriteJSON(w, http.StatusOK, response)
}

func summaryTaskJobID(taskID string) (string, bool) {
	taskID = strings.TrimSpace(taskID)
	for _, prefix := range []string{"taskjob:", "run:"} {
		if strings.HasPrefix(taskID, prefix) {
			id := strings.TrimSpace(strings.TrimPrefix(taskID, prefix))
			return id, id != ""
		}
	}
	if strings.HasPrefix(taskID, "job-") {
		return taskID, true
	}
	return "", false
}

func (h ThreadSummaryHandlers) handleTaskRestart(w http.ResponseWriter, r *http.Request, threadID string, taskID string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	context, ok := h.loadContext(w, threadID)
	if !ok {
		return
	}
	if context.CaseRestricted {
		h.writeCaseHistoryRestricted(w)
		return
	}
	if jobID, ok := summaryTaskJobID(taskID); ok {
		h.handleTaskJobRestart(w, threadID, taskID, jobID)
		return
	}
	WriteJSON(w, http.StatusConflict, map[string]any{
		"code": "conflict", "message": "summary task restart requires a new authorized tool call",
	})
}

func (h ThreadSummaryHandlers) handleTaskJobRestart(w http.ResponseWriter, threadID string, taskID string, jobID string) {
	result := h.Service.RestartTaskJob(threadID, jobID)
	if result.ErrorCode == subagentapp.TaskJobErrorNotFound && (result.Err == nil || errors.Is(result.Err, os.ErrNotExist)) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "summary task not found: " + taskID})
		return
	}
	if result.ErrorCode == subagentapp.TaskJobErrorNotFound {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": "summary task lookup failed"})
		return
	}
	if result.ErrorCode == subagentapp.TaskJobErrorForbidden {
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "summary task does not belong to this thread: " + taskID})
		return
	}
	if result.ErrorCode == subagentapp.TaskJobErrorValidation {
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": "summary task restart is unavailable"})
		return
	}
	if !validSummaryTaskJobResultIdentityV1(result.Record, threadID, jobID) {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"code": "thread_summary_mutation_schema_invalid", "message": "summary task mutation failed host schema validation",
		})
		return
	}
	response := map[string]any{"task": threadSummaryTaskJobMetadataV1(result.Record)}
	if err := threadsummaryapp.ValidateTaskMutationResponseV1(response, "taskjob:"+jobID); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"code": "thread_summary_mutation_schema_invalid", "message": "summary task mutation failed host schema validation",
		})
		return
	}
	WriteJSON(w, http.StatusOK, response)
}

func threadSummaryTaskJobMetadataV1(record domainjob.Record) map[string]any {
	return threadsummaryapp.TaskFromJob(record, time.Now().UTC())
}

func (h ThreadSummaryHandlers) loadContext(w http.ResponseWriter, threadID string) (threadsummaryapp.Context, bool) {
	context, err := h.Service.LoadContext(threadID)
	if h.writeError(w, threadID, err) {
		return threadsummaryapp.Context{}, false
	}
	if strings.TrimSpace(context.ThreadID) != strings.TrimSpace(threadID) ||
		strings.TrimSpace(stringField(context.Thread, "id")) != strings.TrimSpace(threadID) {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"code": "thread_summary_context_invalid", "message": "thread summary context failed host identity validation",
		})
		return threadsummaryapp.Context{}, false
	}
	return context, true
}

func validSummaryTaskJobResultIdentityV1(record domainjob.Record, threadID string, jobID string) bool {
	return strings.TrimSpace(record.ID) == strings.TrimSpace(jobID) &&
		strings.TrimSpace(record.ParentThreadID) == strings.TrimSpace(threadID)
}

func (h ThreadSummaryHandlers) writeCaseHistoryRestricted(w http.ResponseWriter) {
	WriteJSON(w, http.StatusForbidden, map[string]any{
		"code": "case_history_restricted", "message": "case task output is unavailable until trusted evidence projection succeeds",
	})
}

func (h ThreadSummaryHandlers) writeError(w http.ResponseWriter, threadID string, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, threadsummaryapp.ErrThreadNotFound) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found: " + threadID})
		return true
	}
	WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": "thread summary failed"})
	return true
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	case float32:
		return float64(typed)
	default:
		return 0
	}
}

func boolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}
