package subagent

import (
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type TaskJobIsolationReviewRequest struct {
	JobID           string
	ClientRequestID string
}

type TaskJobIsolationRejectRequest struct {
	JobID           string
	ClientRequestID string
	Reason          string
	ApprovalID      string
}

type TaskJobIsolationCleanupRequest struct {
	JobID           string
	ClientRequestID string
}

type TaskJobIsolationAcceptRequest struct {
	JobID           string
	ClientRequestID string
	MergeRequestID  string
	ApprovalID      string
	ParentWorkspace string
}

type TaskJobIsolationConflictReportRequest struct {
	JobID           string
	ClientRequestID string
	ParentWorkspace string
}

type TaskJobIsolationRepairCheckRequest struct {
	JobID            string
	ClientRequestID  string
	ConflictReportID string
	RepairPatch      string
	ParentWorkspace  string
}

type TaskJobIsolationRepairAcceptRequest struct {
	JobID           string
	ClientRequestID string
	RepairReviewID  string
	ApprovalID      string
	ParentWorkspace string
}

func TaskJobIsolationReviewRequestFromArgs(args map[string]any) (TaskJobIsolationReviewRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobIsolationReviewRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobIsolationReviewRequest{
		JobID:           jobID,
		ClientRequestID: strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
	}, nil, false
}

func TaskJobIsolationRejectRequestFromArgs(args map[string]any) (TaskJobIsolationRejectRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobIsolationRejectRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobIsolationRejectRequest{
		JobID:           jobID,
		ClientRequestID: strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
		Reason:          BoundedText(firstNonEmptyAnyString(args["reason"]), 1000),
		ApprovalID:      strings.TrimSpace(firstNonEmptyAnyString(args["approval_id"], args["approvalId"])),
	}, nil, false
}

func TaskJobIsolationCleanupRequestFromArgs(args map[string]any) (TaskJobIsolationCleanupRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobIsolationCleanupRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobIsolationCleanupRequest{
		JobID:           jobID,
		ClientRequestID: strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
	}, nil, false
}

func TaskJobIsolationAcceptRequestFromArgs(args map[string]any) (TaskJobIsolationAcceptRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobIsolationAcceptRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	approvalID := strings.TrimSpace(firstNonEmptyAnyString(args["approval_id"], args["approvalId"]))
	if approvalID == "" {
		return TaskJobIsolationAcceptRequest{}, ValidationErrorResponse("approval_id is required"), true
	}
	return TaskJobIsolationAcceptRequest{
		JobID:           jobID,
		ClientRequestID: strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
		MergeRequestID:  strings.TrimSpace(firstNonEmptyAnyString(args["merge_request_id"], args["mergeRequestId"], args["client_request_id"], args["clientRequestId"])),
		ApprovalID:      approvalID,
		ParentWorkspace: strings.TrimSpace(firstNonEmptyAnyString(args["parent_workspace"], args["parentWorkspace"])),
	}, nil, false
}

func TaskJobIsolationConflictReportRequestFromArgs(args map[string]any) (TaskJobIsolationConflictReportRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobIsolationConflictReportRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobIsolationConflictReportRequest{
		JobID:           jobID,
		ClientRequestID: strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
		ParentWorkspace: strings.TrimSpace(firstNonEmptyAnyString(args["parent_workspace"], args["parentWorkspace"])),
	}, nil, false
}

func TaskJobIsolationRepairCheckRequestFromArgs(args map[string]any) (TaskJobIsolationRepairCheckRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobIsolationRepairCheckRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	conflictReportID := strings.TrimSpace(firstNonEmptyAnyString(args["conflict_report_id"], args["conflictReportId"]))
	if conflictReportID == "" {
		return TaskJobIsolationRepairCheckRequest{}, ValidationErrorResponse("conflict_report_id is required"), true
	}
	repairPatch := firstNonEmptyAnyString(args["repair_patch"], args["repairPatch"], args["patch"])
	if strings.TrimSpace(repairPatch) == "" {
		return TaskJobIsolationRepairCheckRequest{}, ValidationErrorResponse("repair_patch is required"), true
	}
	return TaskJobIsolationRepairCheckRequest{
		JobID:            jobID,
		ClientRequestID:  strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
		ConflictReportID: conflictReportID,
		RepairPatch:      repairPatch,
		ParentWorkspace:  strings.TrimSpace(firstNonEmptyAnyString(args["parent_workspace"], args["parentWorkspace"])),
	}, nil, false
}

func TaskJobIsolationRepairAcceptRequestFromArgs(args map[string]any) (TaskJobIsolationRepairAcceptRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobIsolationRepairAcceptRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	repairReviewID := strings.TrimSpace(firstNonEmptyAnyString(args["repair_review_id"], args["repairReviewId"]))
	if repairReviewID == "" {
		return TaskJobIsolationRepairAcceptRequest{}, ValidationErrorResponse("repair_review_id is required"), true
	}
	approvalID := strings.TrimSpace(firstNonEmptyAnyString(args["approval_id"], args["approvalId"]))
	if approvalID == "" {
		return TaskJobIsolationRepairAcceptRequest{}, ValidationErrorResponse("approval_id is required"), true
	}
	return TaskJobIsolationRepairAcceptRequest{
		JobID:           jobID,
		ClientRequestID: strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
		RepairReviewID:  repairReviewID,
		ApprovalID:      approvalID,
		ParentWorkspace: strings.TrimSpace(firstNonEmptyAnyString(args["parent_workspace"], args["parentWorkspace"])),
	}, nil, false
}

func (s *Service) ReviewTaskJobIsolation(parentThreadID string, request TaskJobIsolationReviewRequest) TaskJobServiceResult {
	record, ok, forbidden, err := s.taskJobRecord(request.JobID, parentThreadID)
	if forbidden {
		return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
	}
	if !ok {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	if strings.TrimSpace(record.IsolationMode) != string(domainjob.IsolationWorktree) || strings.TrimSpace(record.WorktreePath) == "" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job is not an isolated worktree job"), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation}
	}
	if !TaskJobTerminal(record) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job must be terminal before isolation review"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	return worktreeIsolationAuthorityBlockedResult(record)
}

func (s *Service) RejectTaskJobIsolation(parentThreadID string, request TaskJobIsolationRejectRequest) TaskJobServiceResult {
	record, result, ok := s.validateIsolationDecisionRecord(parentThreadID, request.JobID)
	if !ok {
		return result
	}
	if !TaskJobTerminal(record) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job must be terminal before isolation result can be rejected"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if strings.TrimSpace(record.MergeStatus) == "cleaned" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job isolation already cleaned"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if !isolationReviewedForParentDecision(record.MergeStatus) || strings.TrimSpace(record.MergeStatus) == "rejected" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job isolation must be reviewed before rejection"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	return worktreeIsolationAuthorityBlockedResult(record)
}

func (s *Service) CleanupTaskJobIsolation(parentThreadID string, request TaskJobIsolationCleanupRequest) TaskJobServiceResult {
	record, result, ok := s.validateIsolationDecisionRecord(parentThreadID, request.JobID)
	if !ok {
		return result
	}
	if !TaskJobTerminal(record) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job must be terminal before isolation cleanup"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if strings.TrimSpace(record.MergeStatus) == "cleaned" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job isolation already cleaned"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	mergeStatus := strings.TrimSpace(record.MergeStatus)
	if !isolationCleanupAllowedForParentDecision(mergeStatus) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job isolation must be reviewed, rejected, or accepted before cleanup"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if mergeStatus == "accepted" {
		if len(record.AcceptDecisions) == 0 {
			return TaskJobServiceResult{Response: ValidationErrorResponse("accepted isolation cleanup requires an accept decision receipt"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
		}
		if strings.TrimSpace(record.AcceptDecisions[len(record.AcceptDecisions)-1].ID) == "" {
			return TaskJobServiceResult{Response: ValidationErrorResponse("accepted isolation cleanup requires an accept decision receipt"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
		}
	}
	return worktreeIsolationAuthorityBlockedResult(record)
}

func (s *Service) AcceptTaskJobIsolation(parentThreadID string, request TaskJobIsolationAcceptRequest) TaskJobServiceResult {
	record, result, ok := s.validateIsolationDecisionRecord(parentThreadID, request.JobID)
	if !ok {
		return result
	}
	if !TaskJobTerminal(record) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job must be terminal before isolation accept"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	switch strings.TrimSpace(record.MergeStatus) {
	case "review_requested", "clean":
	default:
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job isolation must be reviewed and clean before accept"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if worktreeReviewMergeStatus(record.ChangedFiles) == "conflicted" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job isolation has conflicted files"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	return worktreeIsolationAuthorityBlockedResult(record)
}

func (s *Service) ConflictReportTaskJobIsolation(parentThreadID string, request TaskJobIsolationConflictReportRequest) TaskJobServiceResult {
	record, result, ok := s.validateIsolationDecisionRecord(parentThreadID, request.JobID)
	if !ok {
		return result
	}
	if !TaskJobTerminal(record) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job must be terminal before conflict report"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if strings.TrimSpace(record.MergeStatus) != "conflicted" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job isolation must be conflicted before conflict report"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	return worktreeIsolationAuthorityBlockedResult(record)
}

func (s *Service) RepairCheckTaskJobIsolation(parentThreadID string, request TaskJobIsolationRepairCheckRequest) TaskJobServiceResult {
	record, result, ok := s.validateIsolationDecisionRecord(parentThreadID, request.JobID)
	if !ok {
		return result
	}
	if !TaskJobTerminal(record) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job must be terminal before repair check"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if strings.TrimSpace(record.MergeStatus) != "conflicted" && strings.TrimSpace(record.MergeStatus) != "repair_rejected" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job isolation must be conflicted before repair check"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	return worktreeIsolationAuthorityBlockedResult(record)
}

func (s *Service) RepairAcceptTaskJobIsolation(parentThreadID string, request TaskJobIsolationRepairAcceptRequest) TaskJobServiceResult {
	record, result, ok := s.validateIsolationDecisionRecord(parentThreadID, request.JobID)
	if !ok {
		return result
	}
	if !TaskJobTerminal(record) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job must be terminal before repair accept"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if strings.TrimSpace(record.MergeStatus) != "repair_checked" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job isolation must have a clean repair check before repair accept"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	return worktreeIsolationAuthorityBlockedResult(record)
}

func (s *Service) validateIsolationDecisionRecord(parentThreadID, jobID string) (domainjob.Record, TaskJobServiceResult, bool) {
	record, ok, forbidden, err := s.taskJobRecord(jobID, parentThreadID)
	if forbidden {
		return domainjob.Record{}, TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}, false
	}
	if !ok {
		return domainjob.Record{}, TaskJobServiceResult{Response: TaskJobNotFoundResponse(jobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}, false
	}
	if strings.TrimSpace(record.IsolationMode) != string(domainjob.IsolationWorktree) || strings.TrimSpace(record.WorktreePath) == "" {
		return record, TaskJobServiceResult{Response: ValidationErrorResponse("task job is not an isolated worktree job"), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation}, false
	}
	if !strings.HasPrefix(strings.TrimSpace(record.WorktreeBranch), "codex/subagent/") {
		return record, TaskJobServiceResult{Response: ValidationErrorResponse("task job isolation branch is not analytix-owned"), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation}, false
	}
	return record, TaskJobServiceResult{}, true
}

func worktreeIsolationAuthorityBlockedResult(record domainjob.Record) TaskJobServiceResult {
	return TaskJobServiceResult{
		Response: map[string]any{
			"code":    TaskJobErrorWorktreeIsolationAuthorityRequired,
			"message": "Worktree isolation controls require host-issued durable authority.",
		},
		Record: record, IsError: true, ErrorCode: TaskJobErrorWorktreeIsolationAuthorityRequired,
	}
}

func isolationReviewedForParentDecision(status string) bool {
	switch strings.TrimSpace(status) {
	case "review_requested", "conflicted", "rejected":
		return true
	default:
		return false
	}
}

func isolationCleanupAllowedForParentDecision(status string) bool {
	switch strings.TrimSpace(status) {
	case "review_requested", "conflicted", "rejected", "accepted":
		return true
	default:
		return false
	}
}

func TaskJobIsolationReviewResponse(record domainjob.Record, request TaskJobIsolationReviewRequest) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	reviewID := contracts.SafeRecordID(strings.TrimSpace(request.ClientRequestID))
	if reviewID == "" {
		reviewID = "review_" + contracts.SafeRecordID(record.ID) + "_" + contracts.SafeRecordID(time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	out := map[string]any{
		"status":                 "reviewed",
		"reviewId":               reviewID,
		"jobId":                  record.ID,
		"childRunId":             record.ID,
		"childThreadId":          record.ChildThreadID,
		"mergeStatus":            firstNonEmptyAnyString(record.MergeStatus, "review_requested"),
		"touchedParentWorkspace": false,
	}
	AddIsolationMetadata(out, record)
	out["changedFileGroups"] = WorktreeChangedFileGroups(record.ChangedFiles)
	if strings.TrimSpace(record.DiffSummary) != "" {
		out["diffStat"] = strings.TrimSpace(record.DiffSummary)
	}
	return out
}

func TaskJobIsolationConflictReportResponse(record domainjob.Record, report domainjob.ConflictReport) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	out := map[string]any{
		"status":                 "conflicted",
		"jobId":                  record.ID,
		"childRunId":             record.ID,
		"childThreadId":          record.ChildThreadID,
		"mergeStatus":            "conflicted",
		"conflictReportId":       report.ID,
		"baseCommit":             report.BaseCommit,
		"parentHeadAtConflict":   report.ParentHeadAtConflict,
		"childHeadAtConflict":    report.ChildHeadAtConflict,
		"sourcePatchDigest":      report.SourcePatchDigest,
		"conflictSummary":        report.ConflictSummary,
		"touchedParentWorkspace": report.TouchedParentWorkspace,
		"createdAt":              report.CreatedAt,
	}
	out["conflictFiles"] = changedFilesAny(report.ConflictFiles)
	AddIsolationMetadata(out, record)
	return out
}

func TaskJobIsolationRejectResponse(record domainjob.Record, decision domainjob.MergeDecision) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	out := map[string]any{
		"status":          "rejected",
		"jobId":           record.ID,
		"childRunId":      record.ID,
		"childThreadId":   record.ChildThreadID,
		"mergeStatus":     "rejected",
		"mergeDecisionId": decision.ID,
		"decision":        decision.Decision,
		"createdAt":       decision.CreatedAt,
	}
	if strings.TrimSpace(decision.Reason) != "" {
		out["reason"] = strings.TrimSpace(decision.Reason)
	}
	AddIsolationMetadata(out, record)
	return out
}

func TaskJobIsolationRepairCheckResponse(record domainjob.Record, review domainjob.RepairPatchReview) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	out := map[string]any{
		"status":                 firstNonEmptyAnyString(review.DryRunStatus, "repair_checked"),
		"jobId":                  record.ID,
		"childRunId":             record.ID,
		"childThreadId":          record.ChildThreadID,
		"mergeStatus":            firstNonEmptyAnyString(record.MergeStatus, "repair_checked"),
		"repairReviewId":         review.ID,
		"conflictReportId":       review.ConflictReportID,
		"repairPatchDigest":      review.RepairPatchDigest,
		"expectedParentHead":     review.ExpectedParentHead,
		"dryRunStatus":           review.DryRunStatus,
		"createdAt":              review.CreatedAt,
		"touchedParentWorkspace": false,
	}
	if strings.TrimSpace(review.RejectedReason) != "" {
		out["rejectedReason"] = strings.TrimSpace(review.RejectedReason)
		out["reason"] = strings.TrimSpace(review.RejectedReason)
	}
	out["repairChangedFiles"] = changedFilesAny(review.ChangedFiles)
	AddIsolationMetadata(out, record)
	return out
}

func TaskJobIsolationRepairAcceptResponse(record domainjob.Record, decision domainjob.RepairDecision) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	out := map[string]any{
		"status":             "repair_accepted",
		"jobId":              record.ID,
		"childRunId":         record.ID,
		"childThreadId":      record.ChildThreadID,
		"mergeStatus":        "repair_accepted",
		"repairDecisionId":   decision.ID,
		"repairReviewId":     decision.RepairReviewID,
		"approvalId":         decision.ApprovalID,
		"parentHeadBefore":   decision.ParentHeadBefore,
		"parentHeadAfter":    decision.ParentHeadAfter,
		"appliedPatchDigest": decision.AppliedPatchDigest,
		"createdAt":          decision.CreatedAt,
		"autoCleanup":        false,
	}
	AddIsolationMetadata(out, record)
	return out
}

func TaskJobIsolationCleanupResponse(record domainjob.Record, receipt domainjob.CleanupReceipt) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	out := map[string]any{
		"status":           "cleaned",
		"jobId":            record.ID,
		"childRunId":       record.ID,
		"childThreadId":    record.ChildThreadID,
		"mergeStatus":      "cleaned",
		"cleanupReceiptId": receipt.ID,
		"removed":          receipt.Removed,
		"worktreePath":     receipt.WorktreePath,
		"worktreeBranch":   receipt.Branch,
		"createdAt":        receipt.CreatedAt,
	}
	if strings.TrimSpace(receipt.AcceptDecisionID) != "" {
		out["acceptDecisionId"] = strings.TrimSpace(receipt.AcceptDecisionID)
		out["cleanupAcceptDecisionId"] = strings.TrimSpace(receipt.AcceptDecisionID)
	}
	if strings.TrimSpace(receipt.RetainedReason) != "" {
		out["retainedReason"] = strings.TrimSpace(receipt.RetainedReason)
	}
	AddIsolationMetadata(out, record)
	return out
}

func TaskJobIsolationAcceptResponse(record domainjob.Record, decision domainjob.AcceptDecision) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	out := map[string]any{
		"status":             "accepted",
		"jobId":              record.ID,
		"childRunId":         record.ID,
		"childThreadId":      record.ChildThreadID,
		"mergeStatus":        "accepted",
		"acceptDecisionId":   decision.ID,
		"approvalId":         decision.ApprovalID,
		"mergeRequestId":     decision.MergeRequestID,
		"baseCommit":         decision.BaseCommit,
		"parentHeadBefore":   decision.ParentHeadBefore,
		"parentHeadAfter":    decision.ParentHeadAfter,
		"appliedPatchDigest": decision.AppliedPatchDigest,
		"createdAt":          decision.CreatedAt,
		"autoCleanup":        false,
	}
	AddIsolationMetadata(out, record)
	return out
}

func decisionID(clientID, jobID string, now time.Time) string {
	value := contracts.SafeRecordID(strings.TrimSpace(clientID))
	if value != "" {
		return value
	}
	return "decision_" + contracts.SafeRecordID(jobID) + "_" + contracts.SafeRecordID(now.Format("20060102T150405.000000000Z"))
}

func WorktreeChangedFileGroups(files []domainjob.ChangedFile) map[string]any {
	groups := map[string][]map[string]string{
		"added":      {},
		"modified":   {},
		"deleted":    {},
		"renamed":    {},
		"conflicted": {},
	}
	for _, file := range files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			continue
		}
		status := strings.TrimSpace(file.Status)
		entry := map[string]string{"path": path}
		if status != "" {
			entry["status"] = status
		}
		group := worktreeChangedFileGroup(status)
		groups[group] = append(groups[group], entry)
	}
	out := map[string]any{}
	for key, values := range groups {
		out[key] = values
	}
	return out
}

func changedFilesAny(files []domainjob.ChangedFile) []any {
	out := make([]any, 0, len(files))
	for _, file := range files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			continue
		}
		entry := map[string]any{"path": path}
		if strings.TrimSpace(file.Status) != "" {
			entry["status"] = strings.TrimSpace(file.Status)
		}
		out = append(out, entry)
	}
	return out
}

func worktreeReviewMergeStatus(files []domainjob.ChangedFile) string {
	for _, file := range files {
		if worktreeChangedFileGroup(file.Status) == "conflicted" {
			return "conflicted"
		}
	}
	return "review_requested"
}

func worktreeChangedFileGroup(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "modified"
	}
	if strings.Contains(status, "U") || status == "AA" || status == "DD" {
		return "conflicted"
	}
	if strings.Contains(status, "R") {
		return "renamed"
	}
	if strings.Contains(status, "D") {
		return "deleted"
	}
	if strings.Contains(status, "A") || status == "??" {
		return "added"
	}
	return "modified"
}
