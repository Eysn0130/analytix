package subagent

import (
	"context"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type TaskJobHTTPServiceDeps struct {
	Service             *Service
	Jobs                TaskJobSteerStore
	Turns               TurnSteeringStore
	RecordEvent         func(event map[string]any, context string)
	ParentWorkspace     func(parentThreadID string) string
	RecordLifecycle     func(record domainjob.Record, status string, message string)
	RestartJob          func(parentThreadID string, jobID string) TaskJobServiceResult
	BeginSteerAuthority func(context.Context, domainjob.Record, domainjob.SteerMessage) (TaskJobSteerAuthorization, string, error)
}

type ParentThreadReader interface {
	GetThread(string) (map[string]any, error)
}

func ParentWorkspaceResolver(reader ParentThreadReader) func(string) string {
	return func(threadID string) string {
		if reader == nil {
			return ""
		}
		thread, err := reader.GetThread(threadID)
		if err != nil || thread == nil {
			return ""
		}
		return strings.TrimSpace(firstNonEmptyAnyString(thread["workspace"]))
	}
}

type TaskJobHTTPService struct{ deps TaskJobHTTPServiceDeps }

func NewTaskJobHTTPService(deps TaskJobHTTPServiceDeps) TaskJobHTTPService {
	return TaskJobHTTPService{deps: deps}
}

func (s TaskJobHTTPService) service() *Service { return s.deps.Service }

func (s TaskJobHTTPService) ListTaskJobs(parentThreadID string, request TaskJobListRequest) TaskJobServiceResult {
	return s.service().ListTaskJobViews(parentThreadID, request)
}

func (s TaskJobHTTPService) WaitTaskJobs(parentThreadID string, request TaskJobWaitRequest, emptyIsNotFound bool) TaskJobServiceResult {
	return s.service().WaitTaskJobs(parentThreadID, request, emptyIsNotFound)
}

func (s TaskJobHTTPService) OutputTaskJob(parentThreadID string, request TaskJobOutputRequest, useCursor bool) TaskJobServiceResult {
	return s.service().OutputTaskJob(parentThreadID, request, useCursor)
}

func (s TaskJobHTTPService) KillTaskJob(parentThreadID string, request TaskJobKillRequest) TaskJobServiceResult {
	result := s.service().KillTaskJob(parentThreadID, request)
	if !result.IsError && strings.TrimSpace(result.Record.ID) != "" && s.deps.RecordLifecycle != nil {
		s.deps.RecordLifecycle(result.Record, result.Record.Status, domainjob.NormalizeFailureCode(result.Record.FailureCode, result.Record.Status, true))
	}
	return result
}

func (s TaskJobHTTPService) RestartTaskJob(parentThreadID string, request TaskJobRestartRequest) TaskJobServiceResult {
	if s.deps.RestartJob == nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job restart is not available"), IsError: true, ErrorCode: TaskJobErrorValidation}
	}
	return s.deps.RestartJob(parentThreadID, request.JobID)
}

func (s TaskJobHTTPService) RecoverTaskJob(parentThreadID string, request TaskJobRecoverRequest) TaskJobServiceResult {
	service := s.service()
	if service == nil {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound}
	}
	record, ok, forbidden, err := service.taskJobRecord(request.JobID, parentThreadID)
	if forbidden {
		return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
	}
	if !ok {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	now := time.Now().UTC()
	if strings.TrimSpace(request.DeliveryID) != "" &&
		strings.TrimSpace(record.CompletionDeliveryID) != "" &&
		strings.TrimSpace(request.DeliveryID) != strings.TrimSpace(record.CompletionDeliveryID) {
		return TaskJobServiceResult{
			Response:  ValidationErrorResponse("delivery_id does not match task job delivery"),
			Record:    record,
			IsError:   true,
			ErrorCode: TaskJobErrorConflict,
		}
	}
	if record.LateCompletionSuppressed {
		return TaskJobServiceResult{
			Response: TaskJobRecoverResult(record, recoverResultForRecord(record, "duplicate_suppressed"), firstNonEmptyAnyString(record.LateCompletionReason, "late_completion_suppressed"), now, DefaultTaskJobStalledAfter),
			Record:   record,
		}
	}
	if TaskJobTerminal(record) {
		return s.recoverTerminalTaskJob(record, request, now)
	}
	diagnostics := taskJobControlDiagnostics(record, now, DefaultTaskJobStalledAfter)
	heartbeatStatus := strings.TrimSpace(firstNonEmptyAnyString(diagnostics["heartbeatStatus"]))
	switch heartbeatStatus {
	case "stale", "lease_expired", "orphaned":
		if service.state != nil {
			found, stopped, cancelErr := service.state.CancelBackgroundJobAndWait(record.ID, DefaultBackgroundJobCancelWait)
			if found && (cancelErr != nil || !stopped) {
				if cancelErr == nil {
					cancelErr = ErrBackgroundJobStopTimeout
				}
				return TaskJobServiceResult{
					Response: ValidationErrorResponse(cancelErr.Error()), Record: record,
					IsError: true, ErrorCode: TaskJobErrorConflict, Err: cancelErr,
				}
			}
		}
		reason := domainjob.NormalizeOperationalReasonV1(request.Reason)
		if reason == "" {
			reason = "manual_recover_" + heartbeatStatus
		}
		orphaned := true
		if s.deps.Jobs == nil {
			return TaskJobServiceResult{Response: ValidationErrorResponse("task job recovery is not available"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
		}
		updated, err := s.deps.Jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
			Status:         string(domainjob.StatusInterrupted),
			FailureCode:    domainjob.FailureChildInterrupted,
			Orphaned:       &orphaned,
			RecoveryStatus: "recovered",
			RecoveryReason: reason,
		})
		if err != nil {
			return TaskJobServiceResult{Response: ValidationErrorResponse(err.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
		}
		if s.deps.RecordLifecycle != nil {
			s.deps.RecordLifecycle(updated, updated.Status, domainjob.FailureChildInterrupted)
			if refreshed, err := s.deps.Jobs.LoadChildRun(record.ID); err == nil {
				updated = refreshed
			}
		}
		return TaskJobServiceResult{Response: TaskJobRecoverResult(updated, "recovered", reason, now, DefaultTaskJobStalledAfter), Record: updated}
	case "running", "paused":
		return TaskJobServiceResult{Response: TaskJobRecoverResult(record, "skipped", "job_active", now, DefaultTaskJobStalledAfter), Record: record}
	default:
		return TaskJobServiceResult{Response: TaskJobRecoverResult(record, "not_recoverable", firstNonEmptyAnyString(heartbeatStatus, record.Status), now, DefaultTaskJobStalledAfter), Record: record}
	}
}

func (s TaskJobHTTPService) recoverTerminalTaskJob(record domainjob.Record, request TaskJobRecoverRequest, now time.Time) TaskJobServiceResult {
	if !record.Background || strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) {
		return TaskJobServiceResult{
			Response: TaskJobRecoverResult(record, recoverResultForRecord(record, "already_terminal"), firstNonEmptyAnyString(record.LateCompletionReason, record.Status), now, DefaultTaskJobStalledAfter),
			Record:   record,
		}
	}
	switch strings.TrimSpace(record.CompletionDeliveryStatus) {
	case "delivered":
		return TaskJobServiceResult{Response: TaskJobRecoverResult(record, "duplicate_suppressed", "duplicate_delivery", now, DefaultTaskJobStalledAfter), Record: record}
	case "skipped":
		return TaskJobServiceResult{Response: TaskJobRecoverResult(record, "not_recoverable", "already_terminal", now, DefaultTaskJobStalledAfter), Record: record}
	}
	if s.deps.Jobs == nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job recovery is not available"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	deliveryID := firstNonEmptyAnyString(request.DeliveryID, record.CompletionDeliveryID, taskJobDeliveryID(record))
	reason := domainjob.NormalizeOperationalReasonV1(request.Reason)
	if reason == "" {
		reason = "manual_recovery"
	}
	updated, err := s.deps.Jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
		CompletionDeliveryID:     deliveryID,
		CompletionDeliveryStatus: "retry",
		CompletionDeliveryReason: reason,
		RecoveryStatus:           "recovering",
		RecoveryReason:           reason,
	})
	if err != nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse(err.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
	}
	if s.deps.RecordLifecycle != nil {
		s.deps.RecordLifecycle(updated, updated.Status, "background_job_completed")
		if refreshed, err := s.deps.Jobs.LoadChildRun(record.ID); err == nil {
			updated = refreshed
		}
	}
	return TaskJobServiceResult{Response: TaskJobRecoverResult(updated, recoverResultForRecord(updated, "recovered"), firstNonEmptyAnyString(updated.CompletionDeliveryReason, reason), now, DefaultTaskJobStalledAfter), Record: updated}
}

func taskJobDeliveryID(record domainjob.Record) string {
	if strings.TrimSpace(record.ParentTurnID) == "" || strings.TrimSpace(record.ID) == "" {
		return ""
	}
	return "delivery_" + contracts.SafeRecordID(record.ParentTurnID) + "_" + contracts.SafeRecordID(record.ID)
}

func recoverResultForRecord(record domainjob.Record, fallback string) string {
	if strings.Contains(strings.TrimSpace(record.LateCompletionReason), "superseded") ||
		strings.Contains(strings.TrimSpace(record.CompletionDeliveryReason), "superseded") {
		return "superseded"
	}
	if record.LateCompletionSuppressed {
		return "duplicate_suppressed"
	}
	switch strings.TrimSpace(record.CompletionDeliveryStatus) {
	case "delivered":
		if fallback == "recovered" {
			return "recovered"
		}
		return "duplicate_suppressed"
	case "dead_letter":
		switch strings.TrimSpace(record.CompletionDeliveryReason) {
		case "parent_thread_missing":
			return "parent_missing"
		case "parent_turn_missing":
			return "turn_missing"
		case "parent_thread_archived":
			return "not_recoverable"
		default:
			return "dead_lettered"
		}
	case "skipped":
		return "not_recoverable"
	}
	if fallback != "" {
		return fallback
	}
	return "skipped"
}

func (s TaskJobHTTPService) SteerTaskJob(ctx context.Context, parentThreadID string, request TaskJobSteerRequest) TaskJobServiceResult {
	return SteerRuntimeTaskJob(TaskJobSteerRuntimeDeps{
		Context: ctx, Jobs: s.deps.Jobs, Turns: s.deps.Turns, RecordEvent: s.deps.RecordEvent,
		Authorize: s.service().RecordAuthorized, BeginAuthority: s.deps.BeginSteerAuthority,
	}, parentThreadID, request, time.Now().UTC())
}

func (s TaskJobHTTPService) PauseTaskJob(parentThreadID string, request TaskJobPauseRequest) TaskJobServiceResult {
	return RecordTaskJobPauseResult(s.service().PauseTaskJob(parentThreadID, request), "requested", s.deps.RecordEvent)
}

func (s TaskJobHTTPService) ResumeTaskJob(parentThreadID string, request TaskJobResumeRequest) TaskJobServiceResult {
	return RecordTaskJobPauseResult(s.service().ResumeTaskJob(parentThreadID, request), "resume_requested", s.deps.RecordEvent)
}

func (s TaskJobHTTPService) ReviewTaskJobIsolation(parentThreadID string, request TaskJobIsolationReviewRequest) TaskJobServiceResult {
	return s.service().ReviewTaskJobIsolation(parentThreadID, request)
}

func (s TaskJobHTTPService) RejectTaskJobIsolation(parentThreadID string, request TaskJobIsolationRejectRequest) TaskJobServiceResult {
	return s.service().RejectTaskJobIsolation(parentThreadID, request)
}

func (s TaskJobHTTPService) CleanupTaskJobIsolation(parentThreadID string, request TaskJobIsolationCleanupRequest) TaskJobServiceResult {
	return s.service().CleanupTaskJobIsolation(parentThreadID, request)
}

func (s TaskJobHTTPService) AcceptTaskJobIsolation(parentThreadID string, request TaskJobIsolationAcceptRequest) TaskJobServiceResult {
	return s.service().AcceptTaskJobIsolation(parentThreadID, request)
}

func (s TaskJobHTTPService) ConflictReportTaskJobIsolation(parentThreadID string, request TaskJobIsolationConflictReportRequest) TaskJobServiceResult {
	return s.service().ConflictReportTaskJobIsolation(parentThreadID, request)
}

func (s TaskJobHTTPService) RepairCheckTaskJobIsolation(parentThreadID string, request TaskJobIsolationRepairCheckRequest) TaskJobServiceResult {
	return s.service().RepairCheckTaskJobIsolation(parentThreadID, request)
}

func (s TaskJobHTTPService) RepairAcceptTaskJobIsolation(parentThreadID string, request TaskJobIsolationRepairAcceptRequest) TaskJobServiceResult {
	return s.service().RepairAcceptTaskJobIsolation(parentThreadID, request)
}

func (s TaskJobHTTPService) ChildTodosTaskJob(parentThreadID string, request TaskJobChildTodosRequest) TaskJobServiceResult {
	return s.service().ChildTodosTaskJob(parentThreadID, request)
}

func (s TaskJobHTTPService) ProjectChildTodosTaskJob(parentThreadID string, request TaskJobChildTodoProjectRequest) TaskJobServiceResult {
	return s.service().ProjectChildTodosTaskJob(parentThreadID, request)
}

func (s TaskJobHTTPService) RejectChildTodoProjectionTaskJob(parentThreadID string, request TaskJobChildTodoRejectRequest) TaskJobServiceResult {
	return s.service().RejectChildTodoProjectionTaskJob(parentThreadID, request)
}

func (s TaskJobHTTPService) AcceptChildTodoProjectionTaskJob(parentThreadID string, request TaskJobChildTodoAcceptRequest) TaskJobServiceResult {
	return s.service().AcceptChildTodoProjectionTaskJob(parentThreadID, request)
}
