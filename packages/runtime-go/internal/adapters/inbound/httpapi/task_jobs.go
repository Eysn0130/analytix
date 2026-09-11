package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

type TaskJobService interface {
	ListTaskJobs(parentThreadID string, request subagentapp.TaskJobListRequest) subagentapp.TaskJobServiceResult
	WaitTaskJobs(parentThreadID string, request subagentapp.TaskJobWaitRequest, emptyIsNotFound bool) subagentapp.TaskJobServiceResult
	OutputTaskJob(parentThreadID string, request subagentapp.TaskJobOutputRequest, useCursor bool) subagentapp.TaskJobServiceResult
	KillTaskJob(parentThreadID string, request subagentapp.TaskJobKillRequest) subagentapp.TaskJobServiceResult
	RestartTaskJob(parentThreadID string, request subagentapp.TaskJobRestartRequest) subagentapp.TaskJobServiceResult
	RecoverTaskJob(parentThreadID string, request subagentapp.TaskJobRecoverRequest) subagentapp.TaskJobServiceResult
	SteerTaskJob(context.Context, string, subagentapp.TaskJobSteerRequest) subagentapp.TaskJobServiceResult
	PauseTaskJob(parentThreadID string, request subagentapp.TaskJobPauseRequest) subagentapp.TaskJobServiceResult
	ResumeTaskJob(parentThreadID string, request subagentapp.TaskJobResumeRequest) subagentapp.TaskJobServiceResult
	ReviewTaskJobIsolation(parentThreadID string, request subagentapp.TaskJobIsolationReviewRequest) subagentapp.TaskJobServiceResult
	RejectTaskJobIsolation(parentThreadID string, request subagentapp.TaskJobIsolationRejectRequest) subagentapp.TaskJobServiceResult
	CleanupTaskJobIsolation(parentThreadID string, request subagentapp.TaskJobIsolationCleanupRequest) subagentapp.TaskJobServiceResult
	AcceptTaskJobIsolation(parentThreadID string, request subagentapp.TaskJobIsolationAcceptRequest) subagentapp.TaskJobServiceResult
	ConflictReportTaskJobIsolation(parentThreadID string, request subagentapp.TaskJobIsolationConflictReportRequest) subagentapp.TaskJobServiceResult
	RepairCheckTaskJobIsolation(parentThreadID string, request subagentapp.TaskJobIsolationRepairCheckRequest) subagentapp.TaskJobServiceResult
	RepairAcceptTaskJobIsolation(parentThreadID string, request subagentapp.TaskJobIsolationRepairAcceptRequest) subagentapp.TaskJobServiceResult
	ChildTodosTaskJob(parentThreadID string, request subagentapp.TaskJobChildTodosRequest) subagentapp.TaskJobServiceResult
	ProjectChildTodosTaskJob(parentThreadID string, request subagentapp.TaskJobChildTodoProjectRequest) subagentapp.TaskJobServiceResult
	RejectChildTodoProjectionTaskJob(parentThreadID string, request subagentapp.TaskJobChildTodoRejectRequest) subagentapp.TaskJobServiceResult
	AcceptChildTodoProjectionTaskJob(parentThreadID string, request subagentapp.TaskJobChildTodoAcceptRequest) subagentapp.TaskJobServiceResult
}

type TaskJobHandlers struct {
	Service TaskJobService
}

func (h TaskJobHandlers) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "task_job_service_missing", "message": "task job service missing"})
		return
	}
	switch r.URL.Path {
	case "/v1/runtime/task-jobs/list":
		h.handleList(w, r)
	case "/v1/runtime/task-jobs/wait":
		h.handleWait(w, r)
	case "/v1/runtime/task-jobs/output":
		h.handleOutput(w, r)
	case "/v1/runtime/task-jobs/kill":
		h.handleKill(w, r)
	case "/v1/runtime/task-jobs/restart":
		h.handleRestart(w, r)
	case "/v1/runtime/task-jobs/recover":
		h.handleRecover(w, r)
	case "/v1/runtime/task-jobs/steer":
		h.handleSteer(w, r)
	case "/v1/runtime/task-jobs/pause":
		h.handlePause(w, r)
	case "/v1/runtime/task-jobs/resume":
		h.handleResume(w, r)
	case "/v1/runtime/task-jobs/isolation-review":
		h.handleIsolationReview(w, r)
	case "/v1/runtime/task-jobs/isolation-reject":
		h.handleIsolationReject(w, r)
	case "/v1/runtime/task-jobs/isolation-cleanup":
		h.handleIsolationCleanup(w, r)
	case "/v1/runtime/task-jobs/isolation-accept":
		h.handleIsolationAccept(w, r)
	case "/v1/runtime/task-jobs/isolation-conflict-report":
		h.handleIsolationConflictReport(w, r)
	case "/v1/runtime/task-jobs/isolation-repair-check":
		h.handleIsolationRepairCheck(w, r)
	case "/v1/runtime/task-jobs/isolation-repair-accept":
		h.handleIsolationRepairAccept(w, r)
	case "/v1/runtime/task-jobs/child-todos":
		h.handleChildTodos(w, r)
	case "/v1/runtime/task-jobs/child-todos/project":
		h.handleChildTodosProject(w, r)
	case "/v1/runtime/task-jobs/child-todos/reject":
		h.handleChildTodosReject(w, r)
	case "/v1/runtime/task-jobs/child-todos/accept":
		h.handleChildTodosAccept(w, r)
	default:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "task job route not found"})
	}
}

func (h TaskJobHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job list request")
	if !ok {
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	request, validation, invalid := subagentapp.TaskJobListRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": validation["error"]})
		return
	}
	result := h.Service.ListTaskJobs(parentThreadID, request)
	if result.ErrorCode == subagentapp.TaskJobErrorForbidden {
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "task job does not belong to parent thread"})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"jobs":  subagentapp.ChildOutputMetadataRecordsAny(result.Records),
		"count": float64(len(result.Records)),
	})
}

func (h TaskJobHandlers) handleWait(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job wait request")
	if !ok {
		return
	}
	request := subagentapp.TaskJobWaitRequestFromArgs(body)
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	timeoutMS := 0
	if body["timeoutMs"] != nil || body["timeout_ms"] != nil || body["timeoutSeconds"] != nil || body["timeout_seconds"] != nil {
		timeoutMS = request.TimeoutMS
	}
	result := h.Service.WaitTaskJobs(parentThreadID, subagentapp.TaskJobWaitRequest{
		JobIDs:    request.JobIDs,
		TimeoutMS: timeoutMS,
	}, false)
	if result.ErrorCode == subagentapp.TaskJobErrorForbidden {
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "task job does not belong to parent thread"})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"jobs": subagentapp.ChildOutputMetadataRecordsAny(result.Records),
	})
}

func (h TaskJobHandlers) handleOutput(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job output request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobOutputRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	result := h.Service.OutputTaskJob(parentThreadID, request, false)
	switch result.ErrorCode {
	case subagentapp.TaskJobErrorNotFound:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "task job not found: " + request.JobID})
	case subagentapp.TaskJobErrorForbidden:
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "task job does not belong to parent thread"})
	case subagentapp.TaskJobErrorValidation:
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "task job output request failed validation"})
	default:
		response := subagentapp.TaskJobOutputWithheldResponseV1(result.Record)
		if err := subagentapp.ValidateTaskJobOutputResponseV1(response); err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{
				"code": "task_job_output_schema_invalid", "message": "task job output failed host schema validation",
			})
			return
		}
		WriteJSON(w, http.StatusOK, response)
	}
}

func (h TaskJobHandlers) handleKill(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job kill request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobKillRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	result := h.Service.KillTaskJob(parentThreadID, request)
	switch result.ErrorCode {
	case subagentapp.TaskJobErrorNotFound:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "task job not found: " + request.JobID})
	case subagentapp.TaskJobErrorForbidden:
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "task job does not belong to parent thread"})
	case subagentapp.TaskJobErrorValidation:
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "task job kill request failed validation"})
	case subagentapp.TaskJobErrorConflict:
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": "task job cancellation did not settle"})
	default:
		if result.IsError {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": "task job cancellation failed"})
			return
		}
		job := subagentapp.TaskJobMetadataProjectionV1(result.Record, nil)
		if stringField(job, "id") == "" {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{
				"code": "task_job_metadata_schema_invalid", "message": "task job metadata failed host schema validation",
			})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"job": job})
	}
}

func (h TaskJobHandlers) handleRestart(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job restart request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobRestartRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	result := h.Service.RestartTaskJob(parentThreadID, request)
	switch result.ErrorCode {
	case subagentapp.TaskJobErrorNotFound:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "task job not found: " + request.JobID})
	case subagentapp.TaskJobErrorForbidden:
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "task job does not belong to parent thread"})
	case subagentapp.TaskJobErrorValidation:
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": stringField(result.Response, "error")})
	default:
		WriteJSON(w, http.StatusOK, result.Response)
	}
}

func (h TaskJobHandlers) handleRecover(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job recover request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobRecoverRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.RecoverTaskJob(parentThreadID, request), request.JobID)
}

func (h TaskJobHandlers) handleSteer(w http.ResponseWriter, r *http.Request) {
	body, ok := strictTaskJobSteerBody(w, r)
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobSteerRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	result := h.Service.SteerTaskJob(r.Context(), parentThreadID, request)
	switch result.ErrorCode {
	case subagentapp.TaskJobErrorNotFound:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "task job not found: " + request.JobID})
	case subagentapp.TaskJobErrorForbidden:
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "task job does not belong to parent thread"})
	case subagentapp.TaskJobErrorValidation:
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": stringField(result.Response, "error")})
	case subagentapp.TaskJobErrorConflict:
		if stringField(result.Response, "code") == "new_turn_required" {
			WriteJSON(w, http.StatusConflict, result.Response)
			return
		}
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": stringField(result.Response, "error")})
	default:
		WriteJSON(w, http.StatusOK, result.Response)
	}
}

const (
	maxTaskJobSteerRequestBytes = 24 * 1024
	maxTaskJobSteerStringBytes  = 8 * 1024
)

func strictTaskJobSteerBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	raw, ok := boundedTaskJobSteerBody(w, r)
	if !ok {
		return nil, false
	}
	fields, err := domainjsonstrict.DecodeRawObject(raw, domainjsonstrict.Options{
		MaxBytes: maxTaskJobSteerRequestBytes, MaxDepth: 2, MaxTokens: 32, MaxStringBytes: maxTaskJobSteerStringBytes,
	})
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid task job steer request"})
		return nil, false
	}
	allowed := map[string]struct{}{
		"job_id": {}, "jobId": {}, "id": {}, "message": {}, "text": {},
		"client_message_id": {}, "clientMessageId": {}, "parentThreadId": {}, "threadId": {},
	}
	for _, aliases := range [][]string{
		{"job_id", "jobId", "id"}, {"message", "text"}, {"client_message_id", "clientMessageId"}, {"parentThreadId", "threadId"},
	} {
		count := 0
		for _, key := range aliases {
			if _, found := fields[key]; found {
				count++
			}
		}
		if count > 1 {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid task job steer request"})
			return nil, false
		}
	}
	body := make(map[string]any, len(fields))
	for key, value := range fields {
		if _, found := allowed[key]; !found {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid task job steer request"})
			return nil, false
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid task job steer request"})
			return nil, false
		}
		body[key] = text
	}
	return body, true
}

func boundedTaskJobSteerBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	if r.Body == nil {
		return nil, true
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxTaskJobSteerRequestBytes+1))
	if err != nil || len(data) > maxTaskJobSteerRequestBytes {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid task job steer request"})
		return nil, false
	}
	return json.RawMessage(data), true
}

func (h TaskJobHandlers) handlePause(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job pause request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobPauseRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	result := h.Service.PauseTaskJob(parentThreadID, request)
	switch result.ErrorCode {
	case subagentapp.TaskJobErrorNotFound:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "task job not found: " + request.JobID})
	case subagentapp.TaskJobErrorForbidden:
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "task job does not belong to parent thread"})
	case subagentapp.TaskJobErrorValidation:
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": stringField(result.Response, "error")})
	case subagentapp.TaskJobErrorConflict:
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": stringField(result.Response, "error")})
	default:
		WriteJSON(w, http.StatusOK, result.Response)
	}
}

func (h TaskJobHandlers) handleResume(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job resume request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobResumeRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	result := h.Service.ResumeTaskJob(parentThreadID, request)
	switch result.ErrorCode {
	case subagentapp.TaskJobErrorNotFound:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "task job not found: " + request.JobID})
	case subagentapp.TaskJobErrorForbidden:
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": stringField(result.Response, "error")})
	case subagentapp.TaskJobErrorValidation:
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": stringField(result.Response, "error")})
	case subagentapp.TaskJobErrorConflict:
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": stringField(result.Response, "error")})
	default:
		WriteJSON(w, http.StatusOK, result.Response)
	}
}

func (h TaskJobHandlers) handleIsolationReview(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job isolation review request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobIsolationReviewRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	result := h.Service.ReviewTaskJobIsolation(parentThreadID, request)
	switch result.ErrorCode {
	case subagentapp.TaskJobErrorNotFound:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "task job not found: " + request.JobID})
	case subagentapp.TaskJobErrorForbidden:
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "task job does not belong to parent thread"})
	case subagentapp.TaskJobErrorValidation:
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": stringField(result.Response, "error")})
	case subagentapp.TaskJobErrorConflict:
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": stringField(result.Response, "error")})
	case subagentapp.TaskJobErrorWorktreeIsolationAuthorityRequired:
		WriteJSON(w, http.StatusConflict, result.Response)
	default:
		WriteJSON(w, http.StatusOK, result.Response)
	}
}

func (h TaskJobHandlers) handleIsolationReject(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job isolation reject request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobIsolationRejectRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.RejectTaskJobIsolation(parentThreadID, request), request.JobID)
}

func (h TaskJobHandlers) handleIsolationCleanup(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job isolation cleanup request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobIsolationCleanupRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.CleanupTaskJobIsolation(parentThreadID, request), request.JobID)
}

func (h TaskJobHandlers) handleIsolationAccept(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job isolation accept request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobIsolationAcceptRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.AcceptTaskJobIsolation(parentThreadID, request), request.JobID)
}

func (h TaskJobHandlers) handleIsolationConflictReport(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job isolation conflict report request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobIsolationConflictReportRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.ConflictReportTaskJobIsolation(parentThreadID, request), request.JobID)
}

func (h TaskJobHandlers) handleIsolationRepairCheck(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job isolation repair check request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobIsolationRepairCheckRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.RepairCheckTaskJobIsolation(parentThreadID, request), request.JobID)
}

func (h TaskJobHandlers) handleIsolationRepairAccept(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job isolation repair accept request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobIsolationRepairAcceptRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.RepairAcceptTaskJobIsolation(parentThreadID, request), request.JobID)
}

func (h TaskJobHandlers) handleChildTodos(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job child todos request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobChildTodosRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.ChildTodosTaskJob(parentThreadID, request), request.JobID)
}

func (h TaskJobHandlers) handleChildTodosProject(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job child todo projection request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobChildTodoProjectRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.ProjectChildTodosTaskJob(parentThreadID, request), request.JobID)
}

func (h TaskJobHandlers) handleChildTodosReject(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job child todo projection reject request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobChildTodoRejectRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.RejectChildTodoProjectionTaskJob(parentThreadID, request), request.JobID)
}

func (h TaskJobHandlers) handleChildTodosAccept(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid task job child todo projection accept request")
	if !ok {
		return
	}
	request, validation, invalid := subagentapp.TaskJobChildTodoAcceptRequestFromArgs(body)
	if invalid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": stringField(validation, "code"), "message": stringField(validation, "error")})
		return
	}
	parentThreadID := subagentapp.TaskJobParentThreadIDFromArgs(body)
	if strings.TrimSpace(parentThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId is required"})
		return
	}
	writeTaskJobServiceResult(w, h.Service.AcceptChildTodoProjectionTaskJob(parentThreadID, request), request.JobID)
}

func writeTaskJobServiceResult(w http.ResponseWriter, result subagentapp.TaskJobServiceResult, jobID string) {
	switch result.ErrorCode {
	case subagentapp.TaskJobErrorNotFound:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "task job not found: " + jobID})
	case subagentapp.TaskJobErrorForbidden:
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "task job does not belong to parent thread"})
	case subagentapp.TaskJobErrorValidation:
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": stringField(result.Response, "error")})
	case subagentapp.TaskJobErrorConflict:
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": stringField(result.Response, "error")})
	case subagentapp.TaskJobErrorWorktreeIsolationAuthorityRequired:
		WriteJSON(w, http.StatusConflict, result.Response)
	default:
		WriteJSON(w, http.StatusOK, result.Response)
	}
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}
