package subagent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domaintodo "analytix.local/runtime-go/internal/domain/todo"
)

const (
	TodoScopeParent     = "parent"
	TodoScopeChild      = "child"
	TodoScopeProjection = "projection"

	ChildTodoProjectionProposed   = "proposed"
	ChildTodoProjectionAccepted   = "accepted"
	ChildTodoProjectionRejected   = "rejected"
	ChildTodoProjectionSuperseded = "superseded"

	MaxChildTodoProjectionItems = 200
)

type ChildTodoStore interface {
	GetTodos(threadID string) (map[string]any, error)
	SetTodos(threadID string, items []any) (map[string]any, error)
}

type TaskJobChildTodosRequest struct {
	JobID string
}

type TaskJobChildTodoProjectRequest struct {
	JobID           string
	ClientRequestID string
	SourceTurnID    string
}

type TaskJobChildTodoRejectRequest struct {
	JobID           string
	ProjectionID    string
	ClientRequestID string
	Reason          string
}

type TaskJobChildTodoAcceptRequest struct {
	JobID                        string
	ProjectionID                 string
	ClientRequestID              string
	ApprovalID                   string
	ExpectedParentTodosUpdatedAt string
	SelectedChildTodoIDs         []string
}

func TaskJobChildTodosRequestFromArgs(args map[string]any) (TaskJobChildTodosRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobChildTodosRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobChildTodosRequest{JobID: jobID}, nil, false
}

func TaskJobChildTodoProjectRequestFromArgs(args map[string]any) (TaskJobChildTodoProjectRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobChildTodoProjectRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobChildTodoProjectRequest{
		JobID:           jobID,
		ClientRequestID: strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
		SourceTurnID:    strings.TrimSpace(firstNonEmptyAnyString(args["source_turn_id"], args["sourceTurnId"])),
	}, nil, false
}

func TaskJobChildTodoRejectRequestFromArgs(args map[string]any) (TaskJobChildTodoRejectRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobChildTodoRejectRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobChildTodoRejectRequest{
		JobID:           jobID,
		ProjectionID:    strings.TrimSpace(firstNonEmptyAnyString(args["projection_id"], args["projectionId"])),
		ClientRequestID: strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
		Reason:          strings.TrimSpace(firstNonEmptyAnyString(args["reason"])),
	}, nil, false
}

func TaskJobChildTodoAcceptRequestFromArgs(args map[string]any) (TaskJobChildTodoAcceptRequest, map[string]any, bool) {
	jobID := strings.TrimSpace(firstNonEmptyAnyString(args["job_id"], args["jobId"], args["id"]))
	if jobID == "" {
		return TaskJobChildTodoAcceptRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	return TaskJobChildTodoAcceptRequest{
		JobID:                        jobID,
		ProjectionID:                 strings.TrimSpace(firstNonEmptyAnyString(args["projection_id"], args["projectionId"])),
		ClientRequestID:              strings.TrimSpace(firstNonEmptyAnyString(args["client_request_id"], args["clientRequestId"])),
		ApprovalID:                   strings.TrimSpace(firstNonEmptyAnyString(args["approval_id"], args["approvalId"])),
		ExpectedParentTodosUpdatedAt: strings.TrimSpace(firstNonEmptyAnyString(args["expected_parent_todos_updated_at"], args["expectedParentTodosUpdatedAt"])),
		SelectedChildTodoIDs:         stringList(firstNonNilValue(args["selected_child_todo_ids"], args["selectedChildTodoIds"])),
	}, nil, false
}

func (s *Service) ChildTodosTaskJob(parentThreadID string, request TaskJobChildTodosRequest) TaskJobServiceResult {
	record, ok, forbidden, err := s.taskJobRecord(request.JobID, parentThreadID)
	if forbidden {
		return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
	}
	if !ok {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	return TaskJobServiceResult{Response: TaskJobChildTodosResponse(record), Record: record}
}

func (s *Service) ProjectChildTodosTaskJob(parentThreadID string, request TaskJobChildTodoProjectRequest) TaskJobServiceResult {
	if s.jobs == nil {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound}
	}
	record, ok, forbidden, err := s.taskJobRecord(request.JobID, parentThreadID)
	if forbidden {
		return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
	}
	if !ok {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	if ChildOutputRequiresWithholding(record) {
		return childTodoAuthorityBlockedResult(record)
	}
	if strings.TrimSpace(record.ChildThreadID) == "" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("task job does not have child thread lineage"), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation}
	}
	if !TaskJobTerminal(record) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("child todo projection requires a terminal child job"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if s.childTodos == nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse("child todo projection is not available"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	todos, err := s.childTodos.GetTodos(record.ChildThreadID)
	if err != nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse("child todos could not be read: " + err.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	list := BuildChildTodoList(record, todos, request, now)
	projection := BuildChildTodoProjection(record, list, request, now)
	updated, err := s.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
		ChildTodoList:       &list,
		ChildTodoProjection: &projection,
	})
	if err != nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse(err.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
	}
	return TaskJobServiceResult{Response: TaskJobChildTodoProjectResponse(updated, list, projection), Record: updated}
}

func (s *Service) RejectChildTodoProjectionTaskJob(parentThreadID string, request TaskJobChildTodoRejectRequest) TaskJobServiceResult {
	if s.jobs == nil {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound}
	}
	record, ok, forbidden, err := s.taskJobRecord(request.JobID, parentThreadID)
	if forbidden {
		return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
	}
	if !ok {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	if ChildOutputRequiresWithholding(record) {
		return childTodoAuthorityBlockedResult(record)
	}
	projection, found := FindChildTodoProjection(record, request.ProjectionID)
	if !found {
		return TaskJobServiceResult{Response: ValidationErrorResponse("child todo projection not found"), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation}
	}
	if projection.Status == ChildTodoProjectionRejected {
		return TaskJobServiceResult{Response: TaskJobChildTodoRejectResponse(record, projection), Record: record}
	}
	if projection.Status != ChildTodoProjectionProposed {
		return TaskJobServiceResult{Response: ValidationErrorResponse("child todo projection is not proposed"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	projection.Status = ChildTodoProjectionRejected
	projection.RejectedAt = now
	updated, err := s.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{ChildTodoProjection: &projection})
	if err != nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse(err.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
	}
	return TaskJobServiceResult{Response: TaskJobChildTodoRejectResponse(updated, projection), Record: updated}
}

func (s *Service) AcceptChildTodoProjectionTaskJob(parentThreadID string, request TaskJobChildTodoAcceptRequest) TaskJobServiceResult {
	if s.jobs == nil {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound}
	}
	if strings.TrimSpace(request.ApprovalID) == "" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("approval_id is required"), IsError: true, ErrorCode: TaskJobErrorValidation}
	}
	if strings.TrimSpace(request.ExpectedParentTodosUpdatedAt) == "" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("expectedParentTodosUpdatedAt is required"), IsError: true, ErrorCode: TaskJobErrorValidation}
	}
	record, ok, forbidden, err := s.taskJobRecord(request.JobID, parentThreadID)
	if forbidden {
		return TaskJobServiceResult{Response: ForbiddenTaskJobResponse(), IsError: true, ErrorCode: TaskJobErrorForbidden}
	}
	if !ok {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	if ChildOutputRequiresWithholding(record) {
		return childTodoAuthorityBlockedResult(record)
	}
	projection, found := FindChildTodoProjection(record, request.ProjectionID)
	if !found {
		return TaskJobServiceResult{Response: ValidationErrorResponse("child todo projection not found"), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation}
	}
	if projection.Status == ChildTodoProjectionAccepted {
		if decision, ok := FindProjectionDecision(record, projection.ID); ok {
			return TaskJobServiceResult{Response: TaskJobChildTodoAcceptResponse(record, projection, decision, nil), Record: record}
		}
		return TaskJobServiceResult{Response: ValidationErrorResponse("accepted child todo projection is missing its decision receipt"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if projection.Status != ChildTodoProjectionProposed {
		return TaskJobServiceResult{Response: ValidationErrorResponse("child todo projection is not proposed"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	if s.childTodos == nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse("parent todo projection accept is not available"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	parentTodos, err := s.childTodos.GetTodos(parentThreadID)
	if err != nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse("parent todos could not be read: " + err.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
	}
	if parentTodos == nil {
		parentTodos = map[string]any{"threadId": parentThreadID, "items": []any{}, "updatedAt": ""}
	}
	if current := strings.TrimSpace(firstNonEmptyAnyString(parentTodos["updatedAt"])); current != strings.TrimSpace(request.ExpectedParentTodosUpdatedAt) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("parent todos changed since projection review"), Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
	}
	nextItems, decision := AcceptChildTodoProjection(record, projection, parentTodos, request, time.Now().UTC().Format(time.RFC3339Nano))
	if len(decision.AcceptedItems) == 0 {
		decision.ParentTodosAfterDigest = decision.ParentTodosBeforeDigest
		projection.Status = ChildTodoProjectionAccepted
		projection.AcceptedAt = decision.CreatedAt
		updated, err := s.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
			ChildTodoProjection: &projection,
			ProjectionDecision:  &decision,
		})
		if err != nil {
			return TaskJobServiceResult{Response: ValidationErrorResponse(err.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
		}
		return TaskJobServiceResult{Response: TaskJobChildTodoAcceptResponse(updated, projection, decision, parentTodos), Record: updated}
	}
	updatedTodos, err := s.childTodos.SetTodos(parentThreadID, nextItems)
	if err != nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse("parent todos could not be updated: " + err.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
	}
	decision.ParentTodosAfterDigest = ParentTodosDigest(updatedTodos)
	projection.Status = ChildTodoProjectionAccepted
	projection.AcceptedAt = decision.CreatedAt
	updated, err := s.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
		ChildTodoProjection: &projection,
		ProjectionDecision:  &decision,
	})
	if err != nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse(err.Error()), Record: record, IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
	}
	s.recordTodosUpdated(parentThreadID, updatedTodos)
	return TaskJobServiceResult{Response: TaskJobChildTodoAcceptResponse(updated, projection, decision, updatedTodos), Record: updated}
}

func BuildChildTodoList(record domainjob.Record, todos map[string]any, request TaskJobChildTodoProjectRequest, now string) domainjob.ChildTodoList {
	seed := firstNonEmptyAnyString(request.ClientRequestID, fmt.Sprintf("%d", len(record.ChildTodoLists)+1))
	id := "child_todos_" + contracts.SafeRecordID(record.ID+"_"+seed)
	return domainjob.ChildTodoList{
		ID:             id,
		ParentThreadID: strings.TrimSpace(record.ParentThreadID),
		ChildThreadID:  strings.TrimSpace(record.ChildThreadID),
		ChildRunID:     strings.TrimSpace(record.ID),
		JobID:          strings.TrimSpace(record.ID),
		Scope:          TodoScopeChild,
		Items:          ChildTodoItemsFromTodos(todos),
		CreatedAt:      now,
		UpdatedAt:      firstNonEmptyAnyString(todos["updatedAt"], now),
		SourceTurnID:   firstNonEmptyAnyString(request.SourceTurnID, record.ChildTurnID),
	}
}

func BuildChildTodoProjection(record domainjob.Record, list domainjob.ChildTodoList, request TaskJobChildTodoProjectRequest, now string) domainjob.ChildTodoProjection {
	seed := firstNonEmptyAnyString(request.ClientRequestID, fmt.Sprintf("%d", len(record.ChildTodoProjections)+1))
	id := "child_todo_projection_" + contracts.SafeRecordID(record.ID+"_"+seed)
	items := make([]domainjob.ChildTodoProjectionItem, 0, len(list.Items))
	evidenceIDs := map[string]bool{}
	withhold := ChildOutputRequiresWithholding(record)
	for _, item := range list.Items {
		status, reason, ok := normalizeChildTodoLifecycle(item.Status, item.StatusReasonCode)
		if !ok {
			continue
		}
		projected := domainjob.ChildTodoProjectionItem{
			ID: item.ID, Status: status, StatusReasonCode: reason, ParentTodoRef: item.ParentTodoRef,
		}
		if withhold {
			projected.ID = ""
			projected.ParentTodoRef = ""
		} else {
			projected.Content = item.Content
			projected.EvidenceIDs = append([]string(nil), item.EvidenceIDs...)
			for _, id := range item.EvidenceIDs {
				if strings.TrimSpace(id) != "" {
					evidenceIDs[strings.TrimSpace(id)] = true
				}
			}
		}
		items = append(items, projected)
	}
	if !withhold {
		for _, id := range EvidenceIDsFromChildOutput(record.Output) {
			evidenceIDs[id] = true
		}
	}
	return domainjob.ChildTodoProjection{
		ID:              id,
		ParentThreadID:  strings.TrimSpace(record.ParentThreadID),
		ChildThreadID:   strings.TrimSpace(record.ChildThreadID),
		ChildRunID:      strings.TrimSpace(record.ID),
		JobID:           strings.TrimSpace(record.ID),
		ChildTodoListID: list.ID,
		ProjectedItems:  items,
		Summary:         ChildTodoProjectionSummary(items),
		EvidenceIDs:     sortedKeys(evidenceIDs),
		Status:          ChildTodoProjectionProposed,
		CreatedAt:       now,
	}
}

func AcceptChildTodoProjection(record domainjob.Record, projection domainjob.ChildTodoProjection, parentTodos map[string]any, request TaskJobChildTodoAcceptRequest, now string) ([]any, domainjob.ProjectionDecision) {
	parentItems := parentTodoItemsByID(parentTodos)
	selected := selectedChildTodoIDSet(request.SelectedChildTodoIDs)
	items := listAny(parentTodos["items"])
	nextItems := make([]any, 0, len(items))
	for _, raw := range items {
		nextItems = append(nextItems, contracts.CloneValue(raw))
	}
	accepted := []domainjob.AcceptedProjectionItem{}
	skipped := []domainjob.SkippedProjectionItem{}
	for _, item := range projection.ProjectedItems {
		childTodoID := strings.TrimSpace(item.ID)
		parentTodoID := strings.TrimSpace(item.ParentTodoRef)
		if len(selected) > 0 && !selected[childTodoID] {
			skipped = append(skipped, skippedProjectionItem(item, "not_selected"))
			continue
		}
		if normalizeChildTodoStatus(item.Status) != "completed" {
			skipped = append(skipped, skippedProjectionItem(item, "child_todo_not_completed"))
			continue
		}
		if parentTodoID == "" {
			skipped = append(skipped, skippedProjectionItem(item, "missing_parent_todo_ref"))
			continue
		}
		parentIndex, ok := parentItems[parentTodoID]
		if !ok {
			skipped = append(skipped, skippedProjectionItem(item, "parent_todo_not_found"))
			continue
		}
		parentItem, _ := nextItems[parentIndex].(map[string]any)
		if parentItem == nil {
			skipped = append(skipped, skippedProjectionItem(item, "parent_todo_invalid"))
			continue
		}
		previousStatus := strings.TrimSpace(firstNonEmptyAnyString(parentItem["status"]))
		if previousStatus == "" {
			previousStatus = "pending"
		}
		if previousStatus == "completed" {
			skipped = append(skipped, skippedProjectionItem(item, "parent_todo_already_completed"))
			continue
		}
		if previousStatus == "failed" || previousStatus == "canceled" {
			skipped = append(skipped, skippedProjectionItem(item, "parent_todo_requires_retry"))
			continue
		}
		parentItem["status"] = "completed"
		delete(parentItem, "statusReasonCode")
		accepted = append(accepted, domainjob.AcceptedProjectionItem{
			ChildTodoID:          childTodoID,
			ParentTodoID:         parentTodoID,
			Action:               "complete_existing",
			PreviousParentStatus: previousStatus,
			NextParentStatus:     "completed",
			EvidenceIDs:          append([]string(nil), item.EvidenceIDs...),
		})
	}
	for _, id := range request.SelectedChildTodoIDs {
		if id == "" {
			continue
		}
		if !projectionContainsChildTodoID(projection, id) {
			skipped = append(skipped, domainjob.SkippedProjectionItem{ChildTodoID: id, Reason: "child_todo_not_found"})
		}
	}
	seed := firstNonEmptyAnyString(request.ClientRequestID, request.ApprovalID, fmt.Sprintf("%d", len(record.ProjectionDecisions)+1))
	decision := domainjob.ProjectionDecision{
		ID:                           "child_todo_projection_decision_" + contracts.SafeRecordID(record.ID+"_"+projection.ID+"_"+seed),
		ParentThreadID:               strings.TrimSpace(record.ParentThreadID),
		ChildThreadID:                strings.TrimSpace(record.ChildThreadID),
		ChildRunID:                   strings.TrimSpace(record.ID),
		JobID:                        strings.TrimSpace(record.ID),
		ProjectionID:                 strings.TrimSpace(projection.ID),
		Decision:                     ChildTodoProjectionAccepted,
		ApprovalID:                   strings.TrimSpace(request.ApprovalID),
		AcceptedItems:                accepted,
		SkippedItems:                 skipped,
		ParentTodosBeforeDigest:      ParentTodosDigest(parentTodos),
		ExpectedParentTodosUpdatedAt: strings.TrimSpace(request.ExpectedParentTodosUpdatedAt),
		CreatedAt:                    now,
	}
	return nextItems, decision
}

func ChildTodoItemsFromTodos(todos map[string]any) []domainjob.ChildTodoItem {
	items := listAny(todos["items"])
	out := make([]domainjob.ChildTodoItem, 0, len(items))
	for index, raw := range items {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		content := BoundedText(strings.TrimSpace(firstNonEmptyAnyString(item["content"])), 1000)
		if content == "" {
			continue
		}
		id := strings.TrimSpace(firstNonEmptyAnyString(item["id"]))
		if id == "" {
			id = fmt.Sprintf("todo_%d", index+1)
		}
		status, reason, valid := normalizeChildTodoLifecycle(
			firstNonEmptyAnyString(item["status"]), firstNonEmptyAnyString(item["statusReasonCode"]),
		)
		if !valid {
			continue
		}
		next := domainjob.ChildTodoItem{
			ID: id, Content: content, Status: status, StatusReasonCode: reason,
			EvidenceIDs:   stringList(firstNonNilValue(item["evidenceIds"], item["evidence_ids"])),
			ParentTodoRef: strings.TrimSpace(firstNonEmptyAnyString(item["parentTodoRef"], item["parent_todo_ref"])),
			CreatedAt:     strings.TrimSpace(firstNonEmptyAnyString(item["createdAt"], item["created_at"])),
			UpdatedAt:     strings.TrimSpace(firstNonEmptyAnyString(item["updatedAt"], item["updated_at"])),
		}
		out = append(out, next)
		if len(out) >= MaxChildTodoProjectionItems {
			break
		}
	}
	return out
}

func normalizeChildTodoStatus(value string) string {
	status, err := domaintodo.ParseStatus(value)
	if err != nil {
		return ""
	}
	return string(status)
}

func normalizeChildTodoLifecycle(statusValue string, reasonValue string) (string, string, bool) {
	status, err := domaintodo.ParseStatus(statusValue)
	if err != nil || domaintodo.ValidateStatusReason(status, reasonValue) != nil {
		return "", "", false
	}
	return string(status), strings.TrimSpace(reasonValue), true
}

func EvidenceIDsFromChildOutput(summary string) []string {
	evidence, ok := EvidenceBundleFromSummary(summary)
	if !ok {
		return nil
	}
	ids := map[string]bool{}
	for _, raw := range evidence {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		for _, key := range []string{"id", "evidenceId", "evidence_id", "receiptId", "receipt_id"} {
			if id := strings.TrimSpace(firstNonEmptyAnyString(item[key])); id != "" {
				ids[id] = true
			}
		}
	}
	return sortedKeys(ids)
}

func ChildTodoProjectionSummary(items []domainjob.ChildTodoProjectionItem) string {
	counts := ChildTodoProjectionCounts(items)
	return fmt.Sprintf("%d child todos: %d completed, %d in progress, %d pending, %d failed, %d canceled",
		counts["total"], counts["completed"], counts["in_progress"], counts["pending"], counts["failed"], counts["canceled"])
}

func ChildTodoProjectionCounts(items []domainjob.ChildTodoProjectionItem) map[string]int {
	counts := map[string]int{}
	for _, item := range items {
		status, _, ok := normalizeChildTodoLifecycle(item.Status, item.StatusReasonCode)
		if !ok {
			continue
		}
		counts["total"]++
		counts[status]++
	}
	return counts
}

func FindChildTodoProjection(record domainjob.Record, projectionID string) (domainjob.ChildTodoProjection, bool) {
	projectionID = strings.TrimSpace(projectionID)
	for index := len(record.ChildTodoProjections) - 1; index >= 0; index-- {
		projection := record.ChildTodoProjections[index]
		if projectionID == "" || strings.TrimSpace(projection.ID) == projectionID {
			return projection, true
		}
	}
	return domainjob.ChildTodoProjection{}, false
}

func FindProjectionDecision(record domainjob.Record, projectionID string) (domainjob.ProjectionDecision, bool) {
	projectionID = strings.TrimSpace(projectionID)
	for index := len(record.ProjectionDecisions) - 1; index >= 0; index-- {
		decision := record.ProjectionDecisions[index]
		if projectionID == "" || strings.TrimSpace(decision.ProjectionID) == projectionID {
			return decision, true
		}
	}
	return domainjob.ProjectionDecision{}, false
}

func AddChildTodoMetadata(out map[string]any, record domainjob.Record) {
	if ChildOutputRequiresWithholding(record) {
		return
	}
	list, hasList := LatestChildTodoList(record)
	projection, hasProjection := FindChildTodoProjection(record, "")
	if hasList {
		counts := childTodoItemCounts(list.Items)
		out["childTodoListId"] = list.ID
		out["childTodoScope"] = list.Scope
		out["childTodoCount"] = float64(counts["total"])
		out["childTodoCompletedCount"] = float64(counts["completed"])
		out["childTodoInProgressCount"] = float64(counts["in_progress"])
		out["childTodoPendingCount"] = float64(counts["pending"])
		out["childTodoFailedCount"] = float64(counts["failed"])
		out["childTodoCanceledCount"] = float64(counts["canceled"])
	}
	if hasProjection {
		out["childTodoProjectionId"] = projection.ID
		out["childTodoProjectionStatus"] = projection.Status
		out["childTodoProjectionItemCount"] = float64(len(projection.ProjectedItems))
		if strings.TrimSpace(projection.Summary) != "" {
			out["childTodoProjectionSummary"] = projection.Summary
		}
		if len(projection.EvidenceIDs) > 0 {
			out["childTodoProjectionEvidenceCount"] = float64(len(projection.EvidenceIDs))
		}
		if strings.TrimSpace(projection.AcceptedAt) != "" {
			out["childTodoProjectionAcceptedAt"] = projection.AcceptedAt
		}
		if strings.TrimSpace(projection.RejectedAt) != "" {
			out["childTodoProjectionRejectedAt"] = projection.RejectedAt
		}
		counts := projectionMappingCounts(projection.ProjectedItems)
		out["childTodoProjectionMappedCount"] = float64(counts["mapped"])
		out["childTodoProjectionCompletedMappedCount"] = float64(counts["completed_mapped"])
		if decision, hasDecision := FindProjectionDecision(record, projection.ID); hasDecision {
			out["childTodoProjectionDecisionId"] = decision.ID
			out["childTodoProjectionDecision"] = decision.Decision
			if strings.TrimSpace(decision.ApprovalID) != "" {
				out["childTodoProjectionApprovalId"] = decision.ApprovalID
			}
			out["childTodoProjectionAcceptedItemCount"] = float64(len(decision.AcceptedItems))
			out["childTodoProjectionSkippedItemCount"] = float64(len(decision.SkippedItems))
		}
	}
}

func LatestChildTodoList(record domainjob.Record) (domainjob.ChildTodoList, bool) {
	for index := len(record.ChildTodoLists) - 1; index >= 0; index-- {
		list := record.ChildTodoLists[index]
		if strings.TrimSpace(list.ID) != "" {
			return list, true
		}
	}
	return domainjob.ChildTodoList{}, false
}

func childTodoItemCounts(items []domainjob.ChildTodoItem) map[string]int {
	counts := map[string]int{}
	for _, item := range items {
		status, _, ok := normalizeChildTodoLifecycle(item.Status, item.StatusReasonCode)
		if !ok {
			continue
		}
		counts["total"]++
		counts[status]++
	}
	return counts
}

func projectionMappingCounts(items []domainjob.ChildTodoProjectionItem) map[string]int {
	counts := map[string]int{}
	for _, item := range items {
		if strings.TrimSpace(item.ParentTodoRef) == "" {
			continue
		}
		counts["mapped"]++
		if status, _, ok := normalizeChildTodoLifecycle(item.Status, item.StatusReasonCode); ok && status == "completed" {
			counts["completed_mapped"]++
		}
	}
	return counts
}

func TaskJobChildTodosResponse(record domainjob.Record) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	out := map[string]any{
		"jobId":                record.ID,
		"childRunId":           record.ID,
		"childThreadId":        record.ChildThreadID,
		"childTodoLists":       record.ChildTodoLists,
		"childTodoProjections": record.ChildTodoProjections,
		"projectionDecisions":  record.ProjectionDecisions,
		"job":                  TaskJobRecordView(record, time.Now().UTC(), DefaultTaskJobStalledAfter),
	}
	AddChildTodoMetadata(out, record)
	return out
}

func TaskJobChildTodoProjectResponse(record domainjob.Record, list domainjob.ChildTodoList, projection domainjob.ChildTodoProjection) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	out := TaskJobChildTodosResponse(record)
	out["status"] = projection.Status
	out["childTodoList"] = list
	out["projection"] = projection
	out["projectionId"] = projection.ID
	out["childTodoListId"] = list.ID
	return out
}

func TaskJobChildTodoRejectResponse(record domainjob.Record, projection domainjob.ChildTodoProjection) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	out := TaskJobChildTodosResponse(record)
	out["status"] = projection.Status
	out["projection"] = projection
	out["projectionId"] = projection.ID
	return out
}

func TaskJobChildTodoAcceptResponse(record domainjob.Record, projection domainjob.ChildTodoProjection, decision domainjob.ProjectionDecision, parentTodos map[string]any) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	out := TaskJobChildTodosResponse(record)
	out["status"] = projection.Status
	out["projection"] = projection
	out["projectionId"] = projection.ID
	out["decision"] = decision
	out["decisionId"] = decision.ID
	out["acceptedItemCount"] = float64(len(decision.AcceptedItems))
	out["skippedItemCount"] = float64(len(decision.SkippedItems))
	if parentTodos != nil {
		out["parentTodos"] = contracts.CloneMap(parentTodos)
	}
	return out
}

func childTodoAuthorityBlockedResult(record domainjob.Record) TaskJobServiceResult {
	return TaskJobServiceResult{
		Response:  ValidationErrorResponse("child todo control requires host-bound projection authority"),
		Record:    record,
		IsError:   true,
		ErrorCode: TaskJobErrorConflict,
	}
}

func ParentTodosDigest(todos map[string]any) string {
	cloned := contracts.CloneMap(todos)
	data, err := json.Marshal(cloned)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func parentTodoItemsByID(todos map[string]any) map[string]int {
	out := map[string]int{}
	for index, raw := range listAny(todos["items"]) {
		item, _ := raw.(map[string]any)
		if id := strings.TrimSpace(firstNonEmptyAnyString(item["id"])); id != "" {
			out[id] = index
		}
	}
	return out
}

func selectedChildTodoIDSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func projectionContainsChildTodoID(projection domainjob.ChildTodoProjection, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, item := range projection.ProjectedItems {
		if strings.TrimSpace(item.ID) == id {
			return true
		}
	}
	return false
}

func skippedProjectionItem(item domainjob.ChildTodoProjectionItem, reason string) domainjob.SkippedProjectionItem {
	return domainjob.SkippedProjectionItem{
		ChildTodoID:  strings.TrimSpace(item.ID),
		ParentTodoID: strings.TrimSpace(item.ParentTodoRef),
		Reason:       strings.TrimSpace(reason),
		EvidenceIDs:  append([]string(nil), item.EvidenceIDs...),
	}
}

func (s *Service) recordTodosUpdated(threadID string, todos map[string]any) {
	if s.events == nil {
		return
	}
	_, _, _ = s.events.RecordEvent(map[string]any{
		"kind":     "todos_updated",
		"threadId": strings.TrimSpace(threadID),
		"todos":    contracts.CloneMap(todos),
	})
}

func ParentGoalNotCompletedByChildProjectionError() error {
	return errors.New("child todo projections are parent-visible proposals only and cannot complete the parent goal")
}

func sortedKeys(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
