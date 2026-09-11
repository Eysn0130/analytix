package subagent

import (
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

const (
	TaskJobErrorForbidden                          = "forbidden"
	TaskJobErrorNotFound                           = "not_found"
	TaskJobErrorValidation                         = "validation_error"
	TaskJobErrorConflict                           = "conflict"
	TaskJobErrorWorktreeIsolationAuthorityRequired = "worktree_isolation_authority_required"

	DefaultBackgroundJobContextLimit       = 8
	DefaultBackgroundJobContextOutputLimit = 1200
	DefaultBackgroundJobCancelWait         = 2 * time.Second
)

type TaskJobServiceResult struct {
	Response  map[string]any
	Record    domainjob.Record
	Records   []domainjob.Record
	IsError   bool
	ErrorCode string
	Err       error
}

func (s *Service) WaitTaskJobs(parentThreadID string, request TaskJobWaitRequest, emptyIsNotFound bool) TaskJobServiceResult {
	deadline := time.Now().Add(time.Duration(request.TimeoutMS) * time.Millisecond)
	records, forbidden := s.taskJobWaitRecords(request.JobIDs, parentThreadID)
	if forbidden {
		return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
	}
	for request.TimeoutMS > 0 && !TaskJobsTerminal(records) && time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		sleep := 25 * time.Millisecond
		if remaining < sleep {
			sleep = remaining
		}
		if sleep <= 0 {
			break
		}
		time.Sleep(sleep)
		records, forbidden = s.taskJobWaitRecords(request.JobIDs, parentThreadID)
		if forbidden {
			return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
		}
	}
	if request.TimeoutMS > 0 && TaskJobsTerminal(records) && s.state != nil {
		for _, record := range records {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				break
			}
			settled, _ := s.state.WaitBackgroundJobSettled(record.ID, remaining)
			if !settled {
				break
			}
		}
		records, forbidden = s.taskJobWaitRecords(request.JobIDs, parentThreadID)
		if forbidden {
			return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
		}
	}
	if emptyIsNotFound {
		response, isError := TaskJobWaitResult(records, request.JobIDs, time.Now().UTC(), DefaultTaskJobStalledAfter)
		result := TaskJobServiceResult{Response: response, Records: records, IsError: isError}
		if isError {
			result.ErrorCode = TaskJobErrorNotFound
		}
		return result
	}
	return TaskJobServiceResult{
		Response: map[string]any{"jobs": TaskJobRecordsAny(records, time.Now().UTC(), DefaultTaskJobStalledAfter)},
		Records:  records,
	}
}

func (s *Service) ListTaskJobs(parentThreadID string) ([]domainjob.Record, error) {
	if s.jobs == nil {
		return nil, nil
	}
	records, err := s.jobs.List(parentThreadID)
	if err != nil {
		return records, err
	}
	filtered := make([]domainjob.Record, 0, len(records))
	for _, record := range records {
		if s.RecordAuthorized(parentThreadID, record) {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}

func (s *Service) ListTaskJobViews(parentThreadID string, request TaskJobListRequest) TaskJobServiceResult {
	records, err := s.ListTaskJobs(parentThreadID)
	if err != nil {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(""), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	records = FilterTaskJobRecords(records, request)
	now := time.Now().UTC()
	return TaskJobServiceResult{
		Response: map[string]any{
			"jobs":  TaskJobRecordsAnyWithState(records, now, DefaultTaskJobStalledAfter, s.state),
			"count": float64(len(records)),
		},
		Records: records,
	}
}

func (s *Service) BackgroundJobContextNote(parentThreadID string, now time.Time) (string, []domainjob.Record, error) {
	records, err := s.ListTaskJobs(parentThreadID)
	if err != nil {
		return "", nil, err
	}
	if len(records) == 0 {
		return "", nil, nil
	}
	records = BackgroundJobContextRecords(records, DefaultBackgroundJobContextLimit)
	if len(records) == 0 {
		return "", nil, nil
	}
	note := BackgroundJobContextNote(records, now, DefaultTaskJobStalledAfter, DefaultBackgroundJobContextOutputLimit)
	return note, records, nil
}

func (s *Service) OutputTaskJob(parentThreadID string, request TaskJobOutputRequest, _ bool) TaskJobServiceResult {
	record, ok, forbidden, err := s.taskJobRecord(request.JobID, parentThreadID)
	if forbidden {
		return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
	}
	if !ok {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	return TaskJobServiceResult{Response: TaskJobOutputWithheldResponseV1(record), Record: record}
}

func (s *Service) KillTaskJob(parentThreadID string, request TaskJobKillRequest) TaskJobServiceResult {
	return s.killTaskJobWithWait(parentThreadID, request, DefaultBackgroundJobCancelWait)
}

func (s *Service) killTaskJobWithWait(parentThreadID string, request TaskJobKillRequest, cancelWait time.Duration) TaskJobServiceResult {
	record, ok, forbidden, err := s.taskJobRecord(request.JobID, parentThreadID)
	if forbidden {
		return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
	}
	if !ok {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	if TaskJobTerminal(record) {
		return TaskJobServiceResult{Response: TaskJobKillResult(record, time.Now().UTC(), DefaultTaskJobStalledAfter), Record: record}
	}
	if s.state != nil {
		found, stopped, cancelErr := s.state.CancelBackgroundJobAndWait(request.JobID, cancelWait)
		if found {
			if cancelErr != nil || !stopped {
				if cancelErr == nil {
					cancelErr = ErrBackgroundJobStopTimeout
				}
				return TaskJobServiceResult{
					Response: ValidationErrorResponse(cancelErr.Error()), Record: record,
					IsError: true, ErrorCode: TaskJobErrorConflict, Err: cancelErr,
				}
			}
			if s.jobs != nil {
				if refreshed, loadErr := s.jobs.LoadChildRun(request.JobID); loadErr == nil {
					record = refreshed
				} else {
					return TaskJobServiceResult{Response: ValidationErrorResponse(loadErr.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: loadErr}
				}
			}
		}
	}
	if s.jobs != nil && !TaskJobTerminal(record) {
		updated, updateErr := s.jobs.UpdateChildRun(request.JobID, domainjob.UpdateRequest{
			Status:      string(domainjob.StatusKilled),
			FailureCode: domainjob.FailureChildKilled,
		})
		if updateErr != nil {
			return TaskJobServiceResult{Response: ValidationErrorResponse(updateErr.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: updateErr}
		}
		record = updated
	}
	return TaskJobServiceResult{Response: TaskJobKillResult(record, time.Now().UTC(), DefaultTaskJobStalledAfter), Record: record}
}

func (s *Service) PauseTaskJob(parentThreadID string, request TaskJobPauseRequest) TaskJobServiceResult {
	store, ok := s.jobs.(TaskJobPauseStore)
	if s.jobs == nil || !ok {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound}
	}
	record, err := store.LoadChildRun(request.JobID)
	if err != nil {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	if !s.RecordAuthorized(parentThreadID, record) {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound}
	}
	now := time.Now().UTC()
	validation := ValidateTaskJobPause(record, parentThreadID, request, now)
	if validation.Code != "" {
		return rejectRuntimeTaskJobPause(store, record, validation)
	}
	if s.pauseRuntime == nil {
		return rejectRuntimeTaskJobPause(store, record, TaskJobPauseValidation{Request: validation.Request, Code: TaskJobErrorConflict, Reason: "task job runner is not active; cannot pause safely"})
	}
	if _, ok := s.pauseRuntime.RequestBackgroundJobPause(record.ID, validation.Request.ID, now); !ok {
		return rejectRuntimeTaskJobPause(store, record, TaskJobPauseValidation{Request: validation.Request, Code: TaskJobErrorConflict, Reason: "task job runner is not active; cannot pause safely"})
	}
	record, pauseRequest, err := store.AddPauseRequest(record.ID, validation.Request)
	if err != nil {
		_ = s.pauseRuntime.ClearBackgroundJobPauseRequest(record.ID, validation.Request.ID)
		return TaskJobServiceResult{Response: ValidationErrorResponse(err.Error()), IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
	}
	return TaskJobServiceResult{Response: TaskJobPauseResponse(record, pauseRequest, "requested", ""), Record: record}
}

func (s *Service) ResumeTaskJob(parentThreadID string, request TaskJobResumeRequest) TaskJobServiceResult {
	store, ok := s.jobs.(TaskJobPauseStore)
	if s.jobs == nil || !ok {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound}
	}
	record, err := store.LoadChildRun(request.JobID)
	if err != nil {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	if !s.RecordAuthorized(parentThreadID, record) {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound}
	}
	now := time.Now().UTC()
	validation := ValidateTaskJobResume(record, parentThreadID, request, now)
	if validation.Code != "" {
		return TaskJobServiceResult{Response: ValidationErrorResponse(validation.Reason), IsError: true, ErrorCode: validation.Code}
	}
	if s.pauseRuntime == nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job runner is not active; cannot resume safely"), IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	snapshot, ok := s.pauseRuntime.BackgroundJobPauseSnapshot(record.ID)
	if !ok || snapshot.Status != "paused" || strings.TrimSpace(snapshot.PauseRequestID) != strings.TrimSpace(validation.Request.ID) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job runner is not paused; cannot resume safely"), IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	record, pauseRequest, err := store.MarkPauseResumeRequested(record.ID, validation.Request.ID)
	if err != nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse(err.Error()), IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
	}
	if _, ok := s.pauseRuntime.ResumeBackgroundJob(record.ID, validation.Request.ID); !ok {
		rolledBack, _, rollbackErr := store.RollbackPauseResumeRequested(record.ID, validation.Request.ID)
		if rollbackErr != nil {
			return TaskJobServiceResult{Response: ValidationErrorResponse("task job resume failed closed during durable rollback"), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: rollbackErr}
		}
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job runner is not paused; cannot resume safely"), Record: rolledBack, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	return TaskJobServiceResult{Response: TaskJobPauseResponse(record, pauseRequest, "resume_requested", ""), Record: record}
}

func rejectRuntimeTaskJobPause(store TaskJobPauseStore, record domainjob.Record, validation TaskJobPauseValidation) TaskJobServiceResult {
	if validation.Code == TaskJobErrorForbidden || validation.Code == TaskJobErrorNotFound ||
		strings.TrimSpace(record.ID) == "" || store == nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse(validation.Reason), IsError: true, ErrorCode: validation.Code}
	}
	record, pauseRequest, err := store.RejectPauseRequest(record.ID, validation.Request, validation.Reason)
	if err == nil {
		return TaskJobServiceResult{Response: TaskJobPauseResponse(record, pauseRequest, "rejected", validation.Reason), Record: record, IsError: true, ErrorCode: validation.Code}
	}
	return TaskJobServiceResult{Response: ValidationErrorResponse(validation.Reason), IsError: true, ErrorCode: validation.Code, Err: err}
}

func (s *Service) taskJobWaitRecords(jobIDs []string, parentThreadID string) ([]domainjob.Record, bool) {
	if s.jobs == nil {
		return nil, false
	}
	if len(jobIDs) == 0 {
		records, err := s.ListTaskJobs(parentThreadID)
		if err != nil {
			return nil, false
		}
		return records, false
	}
	records := make([]domainjob.Record, 0, len(jobIDs))
	for _, jobID := range jobIDs {
		record, err := s.jobs.LoadChildRun(jobID)
		if err == nil {
			if !s.RecordAuthorized(parentThreadID, record) {
				continue
			}
			records = append(records, record)
		}
	}
	return records, false
}

func (s *Service) taskJobRecord(jobID string, parentThreadID string) (domainjob.Record, bool, bool, error) {
	if s.jobs == nil {
		return domainjob.Record{}, false, false, nil
	}
	record, err := s.jobs.LoadChildRun(jobID)
	if err != nil {
		return domainjob.Record{}, false, false, err
	}
	if !s.RecordAuthorized(parentThreadID, record) {
		return domainjob.Record{}, false, false, nil
	}
	return record, true, false, nil
}

func (s *Service) RecordAuthorized(parentThreadID string, record domainjob.Record) bool {
	return s != nil && TaskJobRecordAllowed(record, parentThreadID) && (s.authorizeRecord == nil || s.authorizeRecord(parentThreadID, record))
}
