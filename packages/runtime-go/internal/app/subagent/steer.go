package subagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

const (
	DefaultTaskJobSteerMaxRunes  = 4000
	DefaultTaskJobSteerMaxQueued = 5
)

type TaskJobSteerRequest struct {
	JobID            string
	Message          string
	ClientMessageID  string
	SourceTurnID     string
	SourceToolCallID string
	MaxMessageRunes  int
	MaxPendingSteers int
}

type TaskJobSteerValidation struct {
	Message domainjob.SteerMessage
	Code    string
	Reason  string
}

func TaskJobSteerRequestFromArgs(args map[string]any) (TaskJobSteerRequest, map[string]any, bool) {
	jobID, invalid := taskJobSteerStringArg(args, "job_id", "jobId", "id")
	if invalid {
		return TaskJobSteerRequest{}, ValidationErrorResponse("task job steer request has conflicting or invalid aliases"), true
	}
	if jobID == "" {
		return TaskJobSteerRequest{}, ValidationErrorResponse("job_id is required"), true
	}
	message, invalid := taskJobSteerStringArg(args, "message", "text")
	if invalid {
		return TaskJobSteerRequest{}, ValidationErrorResponse("task job steer request has conflicting or invalid aliases"), true
	}
	if message == "" {
		return TaskJobSteerRequest{}, ValidationErrorResponse("message is required"), true
	}
	clientMessageID, invalid := taskJobSteerStringArg(args, "client_message_id", "clientMessageId")
	if invalid {
		return TaskJobSteerRequest{}, ValidationErrorResponse("task job steer request has conflicting or invalid aliases"), true
	}
	if clientMessageID != "" && !contracts.IsCanonicalOpaqueUUIDV4(clientMessageID) {
		return TaskJobSteerRequest{}, ValidationErrorResponse("client_message_id must be an opaque UUID"), true
	}
	sourceTurnID, invalid := taskJobSteerStringArg(args, "source_turn_id", "sourceTurnId")
	if invalid {
		return TaskJobSteerRequest{}, ValidationErrorResponse("task job steer request has conflicting or invalid aliases"), true
	}
	sourceToolCallID, invalid := taskJobSteerStringArg(args, "source_tool_call_id", "sourceToolCallId")
	if invalid {
		return TaskJobSteerRequest{}, ValidationErrorResponse("task job steer request has conflicting or invalid aliases"), true
	}
	request := TaskJobSteerRequest{
		JobID:            jobID,
		Message:          message,
		ClientMessageID:  clientMessageID,
		SourceTurnID:     sourceTurnID,
		SourceToolCallID: sourceToolCallID,
	}
	return request, nil, false
}

func taskJobSteerStringArg(args map[string]any, keys ...string) (string, bool) {
	found := false
	value := ""
	for _, key := range keys {
		raw, present := args[key]
		if !present {
			continue
		}
		if found {
			return "", true
		}
		text, ok := raw.(string)
		if !ok {
			return "", true
		}
		found = true
		value = strings.TrimSpace(text)
	}
	return value, false
}

func ValidateTaskJobSteer(record domainjob.Record, parentThreadID string, request TaskJobSteerRequest, now time.Time) TaskJobSteerValidation {
	parentThreadID = strings.TrimSpace(parentThreadID)
	messageText := strings.TrimSpace(request.Message)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	maxRunes := request.MaxMessageRunes
	if maxRunes <= 0 {
		maxRunes = DefaultTaskJobSteerMaxRunes
	}
	maxPending := request.MaxPendingSteers
	if maxPending <= 0 {
		maxPending = DefaultTaskJobSteerMaxQueued
	}
	message := BuildTaskJobSteerMessage(record, parentThreadID, request, now)
	if !TaskJobRecordAllowed(record, parentThreadID) {
		return TaskJobSteerValidation{Message: message, Code: TaskJobErrorForbidden, Reason: "task job does not belong to parent thread"}
	}
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.ID) != strings.TrimSpace(request.JobID) {
		return TaskJobSteerValidation{Message: message, Code: TaskJobErrorNotFound, Reason: "task job not found: " + strings.TrimSpace(request.JobID)}
	}
	if !JobIsSubagent(record) || strings.TrimSpace(record.ChildThreadID) == "" {
		return TaskJobSteerValidation{Message: message, Code: TaskJobErrorValidation, Reason: "task job has no child run lineage"}
	}
	if !record.Background {
		return TaskJobSteerValidation{Message: message, Code: TaskJobErrorValidation, Reason: "foreground child runs cannot be steered"}
	}
	if TaskJobTerminal(record) {
		return TaskJobSteerValidation{Message: message, Code: TaskJobErrorConflict, Reason: "task job is terminal: " + strings.TrimSpace(record.Status)}
	}
	switch strings.TrimSpace(record.Status) {
	case string(domainjob.StatusQueued), string(domainjob.StatusRunning):
	default:
		return TaskJobSteerValidation{Message: message, Code: TaskJobErrorConflict, Reason: "task job cannot accept steer while status is " + strings.TrimSpace(record.Status)}
	}
	if messageText == "" {
		return TaskJobSteerValidation{Message: message, Code: TaskJobErrorValidation, Reason: "message is required"}
	}
	if clientMessageID := strings.TrimSpace(request.ClientMessageID); clientMessageID != "" && !contracts.IsCanonicalOpaqueUUIDV4(clientMessageID) {
		return TaskJobSteerValidation{Message: message, Code: TaskJobErrorValidation, Reason: "client_message_id must be an opaque UUID"}
	}
	if len([]rune(messageText)) > maxRunes {
		return TaskJobSteerValidation{Message: message, Code: TaskJobErrorValidation, Reason: fmt.Sprintf("message is too long (max %d characters)", maxRunes)}
	}
	if TaskJobPendingSteerCount(record) >= maxPending {
		return TaskJobSteerValidation{Message: message, Code: TaskJobErrorConflict, Reason: fmt.Sprintf("too many pending steer messages (max %d)", maxPending)}
	}
	return TaskJobSteerValidation{Message: message}
}

func BuildTaskJobSteerMessage(record domainjob.Record, parentThreadID string, request TaskJobSteerRequest, now time.Time) domainjob.SteerMessage {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	messageID := strings.TrimSpace(request.ClientMessageID)
	if !contracts.IsCanonicalOpaqueUUIDV4(messageID) {
		messageID = contracts.NewOpaqueUUIDV4(
			"analytix/task-job-steer-message-id/v1",
			contracts.SafeRecordID(record.ID),
			now,
		)
	}
	return domainjob.SteerMessage{
		ID:               messageID,
		ParentThreadID:   strings.TrimSpace(parentThreadID),
		ChildRunID:       strings.TrimSpace(record.ID),
		JobID:            strings.TrimSpace(record.ID),
		Text:             strings.TrimSpace(request.Message),
		Status:           "queued",
		CreatedAt:        now.UTC().Format(time.RFC3339Nano),
		SourceTurnID:     strings.TrimSpace(request.SourceTurnID),
		SourceToolCallID: strings.TrimSpace(request.SourceToolCallID),
	}
}

func TaskJobPendingSteerCount(record domainjob.Record) int {
	count := 0
	for _, message := range record.Steers {
		if strings.TrimSpace(message.Status) == "queued" {
			count++
		}
	}
	return count
}

func TaskJobSteerTurnEntry(record domainjob.Record, message domainjob.SteerMessage) map[string]any {
	entry := map[string]any{
		"id":                      domainsteering.EntryIDV1(record.ChildTurnID, message.ID),
		"clientUserMessageId":     message.ID,
		"text":                    message.Text,
		"admittedAt":              message.CreatedAt,
		"delivery":                "steer",
		"jobId":                   record.ID,
		"childRunId":              record.ID,
		"steerMessageId":          message.ID,
		"parentThreadId":          record.ParentThreadID,
		"childThreadId":           record.ChildThreadID,
		"jobProjectionVersion":    message.ProjectionVersion,
		"jobContentDigest":        message.ContentDigest,
		"jobContextDigest":        message.ContextDigest,
		"jobAuthorityDigest":      message.AuthorityDigest,
		"jobQueueAuthorityDigest": message.QueueAuthorityDigest,
	}
	if message.SourceTurnID != "" {
		entry["sourceTurnId"] = message.SourceTurnID
	}
	if message.SourceToolCallID != "" {
		entry["sourceToolCallId"] = message.SourceToolCallID
	}
	if message.LogicalEffect != "" {
		entry["logicalEffect"] = string(message.LogicalEffect)
		entry["ordinaryWork"] = message.OrdinaryWork
	}
	return entry
}

func TaskJobSteerResponse(record domainjob.Record, message domainjob.SteerMessage, status string, reason string) map[string]any {
	if SecurityBoundChildOutput(record) {
		return SecurityBoundChildOutputProjection(record)
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = strings.TrimSpace(message.Status)
	}
	status = domainjob.PublicSteerStatusV1(status)
	out := map[string]any{
		"status":         status,
		"jobId":          record.ID,
		"childRunId":     record.ID,
		"childThreadId":  record.ChildThreadID,
		"childTurnId":    record.ChildTurnID,
		"steerMessageId": message.ID,
		"queued":         status == "queued",
		"admitted":       status == "admitted",
		"rejected":       status == "rejected",
		"canAcceptSteer": record.SteerState.CanAcceptSteer,
		"pendingSteers":  float64(record.SteerState.PendingSteers),
		"admittedSteers": float64(record.SteerState.AdmittedSteers),
		"steerCount":     float64(record.SteerState.SteerCount),
		"createdAt":      message.CreatedAt,
	}
	if message.AdmittedAt != "" {
		out["admittedAt"] = message.AdmittedAt
	}
	if reason = ProjectOrdinaryJobText(reason); reason != "" {
		out["reason"] = reason
	}
	return out
}

func AddSteerStateMetadata(out map[string]any, record domainjob.Record) {
	out["canAcceptSteer"] = record.SteerState.CanAcceptSteer
	out["pendingSteers"] = float64(record.SteerState.PendingSteers)
	out["admittedSteers"] = float64(record.SteerState.AdmittedSteers)
	out["steerCount"] = float64(record.SteerState.SteerCount)
	if strings.TrimSpace(record.SteerState.LastSteerAt) != "" {
		out["lastSteerAt"] = record.SteerState.LastSteerAt
	}
}

type TaskJobSteerStore interface {
	LoadChildRun(id string) (domainjob.Record, error)
	UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error)
	QueueSteerMessage(id string, authority domainjob.SteerQueueAuthorityV1, message domainjob.SteerMessage) (domainjob.Record, domainjob.SteerMessage, error)
	SettleSteerPromotionExact(id string, expected domainjob.SteerMessage, settlement domainjob.SteerPromotionSettlementV1) (domainjob.Record, domainjob.SteerMessage, error)
	RejectSteerMessage(id string, message domainjob.SteerMessage, reason string) (domainjob.Record, domainjob.SteerMessage, error)
}

// TaskJobPendingSteerTurnBinder is the additional exact CAS needed only when
// previously queued guidance reaches its now-committed reserved first turn.
type TaskJobPendingSteerTurnBinder interface {
	BindPendingSteerMessageForTurn(string, domainjob.SteerQueueAuthorityV1, domainjob.SteerMessage) (domainjob.Record, domainjob.SteerMessage, error)
}

type TurnSteeringStore interface {
	AdmitSteeringEntryForContext(
		threadID, turnID, expectedTurnID, expectedContextDigest string,
		entry map[string]any,
	) (map[string]any, error)
}

// TaskJobSteerAuthorization is issued by the host after it proves that a
// child steer cannot raise the risk or mutate the context frozen for the
// target turn. The effect lease must remain held until the context-bound
// steering write finishes. A pending authorization is permitted only before
// the child has a turn and therefore carries no turn mutation authority.
type TaskJobSteerAuthorization struct {
	ExpectedContextDigest string
	PendingUntilChildTurn bool
	ProjectedText         string
	EffectBinding         domainsteering.EntryLogicalEffectBinding
	QueueAuthority        domainjob.SteerQueueAuthorityV1
	Release               func()
}

type TaskJobSteerRuntimeDeps struct {
	Context        context.Context
	Jobs           TaskJobSteerStore
	Turns          TurnSteeringStore
	RecordEvent    func(event map[string]any, context string)
	Authorize      func(string, domainjob.Record) bool
	BeginAuthority func(context.Context, domainjob.Record, domainjob.SteerMessage) (TaskJobSteerAuthorization, string, error)
}

func SteerRuntimeTaskJob(deps TaskJobSteerRuntimeDeps, parentThreadID string, request TaskJobSteerRequest, now time.Time) TaskJobServiceResult {
	if deps.Jobs == nil {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound}
	}
	record, err := deps.Jobs.LoadChildRun(request.JobID)
	if err != nil {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound, Err: err}
	}
	if !TaskJobRecordAllowed(record, parentThreadID) || (deps.Authorize != nil && !deps.Authorize(parentThreadID, record)) {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(request.JobID), IsError: true, ErrorCode: TaskJobErrorNotFound}
	}
	validation := ValidateTaskJobSteer(record, parentThreadID, request, now)
	if validation.Code != "" {
		return rejectRuntimeTaskJobSteer(deps, record, validation)
	}
	authorization, blocker, authorityErr := beginRuntimeTaskJobSteerAuthority(deps, record, validation.Message)
	if authorization.Release != nil {
		defer authorization.Release()
	}
	if authorityErr != nil {
		return taskJobSteerAuthorityFailure(record, validation.Message)
	}
	if blocker != "" {
		return taskJobSteerNewTurnRequired(record, validation.Message, blocker)
	}
	if strings.TrimSpace(authorization.ProjectedText) == "" {
		return taskJobSteerAuthorityFailure(record, validation.Message)
	}
	validation.Message.Text = strings.TrimSpace(authorization.ProjectedText)
	validation.Message.LogicalEffect = authorization.EffectBinding.LogicalEffect
	validation.Message.OrdinaryWork = authorization.EffectBinding.OrdinaryWork
	if domainjob.ValidateSteerMessageLogicalEffectBindingV1(validation.Message) != nil ||
		validation.Message.LogicalEffect == "" {
		return taskJobSteerAuthorityFailure(record, validation.Message)
	}
	if authorization.PendingUntilChildTurn {
		if authorization.ExpectedContextDigest != "" || !authorization.QueueAuthority.PendingUntilChildTurn {
			return taskJobSteerAuthorityFailure(record, validation.Message)
		}
	} else if strings.TrimSpace(record.ChildTurnID) == "" || authorization.QueueAuthority.PendingUntilChildTurn ||
		strings.TrimSpace(authorization.ExpectedContextDigest) == "" {
		return taskJobSteerAuthorityFailure(record, validation.Message)
	}
	record, message, err := deps.Jobs.QueueSteerMessage(record.ID, authorization.QueueAuthority, validation.Message)
	if err != nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse(err.Error()), IsError: true, ErrorCode: TaskJobErrorValidation, Err: err}
	}
	recordTaskJobSteerEvent(deps, record, message, "queued", "")
	if authorization.PendingUntilChildTurn {
		return TaskJobServiceResult{Response: TaskJobSteerResponse(record, message, "queued", ""), Record: record}
	}
	if err := AdmitRuntimeTaskJobSteer(deps.Turns, record, message, authorization.ExpectedContextDigest); err != nil {
		const reason = "task job steer admission failed"
		updated, rejected, rejectErr := deps.Jobs.RejectSteerMessage(record.ID, message, reason)
		if rejectErr != nil {
			return TaskJobServiceResult{
				Response: TaskJobSteerResponse(record, message, "blocked", "task job steer settlement unavailable"),
				Record:   record, IsError: true, ErrorCode: TaskJobErrorConflict, Err: errors.Join(err, rejectErr),
			}
		}
		recordTaskJobSteerEvent(deps, updated, rejected, "rejected", reason)
		return TaskJobServiceResult{Response: TaskJobSteerResponse(updated, rejected, "rejected", reason), Record: updated, IsError: true, ErrorCode: TaskJobErrorConflict, Err: err}
	}
	return TaskJobServiceResult{Response: TaskJobSteerResponse(record, message, "queued", ""), Record: record}
}

func beginRuntimeTaskJobSteerAuthority(
	deps TaskJobSteerRuntimeDeps,
	record domainjob.Record,
	message domainjob.SteerMessage,
) (TaskJobSteerAuthorization, string, error) {
	if deps.BeginAuthority == nil {
		return TaskJobSteerAuthorization{}, "", errors.New("task job steer authority is unavailable")
	}
	ctx := deps.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return TaskJobSteerAuthorization{}, "", err
	}
	return deps.BeginAuthority(ctx, record, message)
}

func taskJobSteerAuthorityFailure(record domainjob.Record, message domainjob.SteerMessage) TaskJobServiceResult {
	const reason = "task job steer security authority is unavailable"
	return TaskJobServiceResult{
		Response: TaskJobSteerResponse(record, message, "rejected", reason), Record: record,
		IsError: true, ErrorCode: TaskJobErrorConflict, Err: errors.New(reason),
	}
}

func taskJobSteerNewTurnRequired(record domainjob.Record, message domainjob.SteerMessage, blocker string) TaskJobServiceResult {
	response := TaskJobSteerResponse(record, message, "rejected", "a newly admitted turn is required")
	response["code"] = "new_turn_required"
	response["blockerCode"] = strings.TrimSpace(blocker)
	response["threadId"] = strings.TrimSpace(record.ChildThreadID)
	response["turnId"] = strings.TrimSpace(record.ChildTurnID)
	return TaskJobServiceResult{Response: response, Record: record, IsError: true, ErrorCode: TaskJobErrorConflict}
}

func rejectRuntimeTaskJobSteer(deps TaskJobSteerRuntimeDeps, record domainjob.Record, validation TaskJobSteerValidation) TaskJobServiceResult {
	// Validation runs before the host has issued projection and context
	// authority. Persisting the rejected input here would turn raw PII,
	// reasoning, or hostile text into durable job state.
	return TaskJobServiceResult{Response: ValidationErrorResponse(validation.Reason), IsError: true, ErrorCode: validation.Code}
}

func PrepareRuntimeTaskJobSteeringForTurn(deps TaskJobSteerRuntimeDeps, childRunID string, childThreadID string, childTurnID string) error {
	childRunID = strings.TrimSpace(childRunID)
	if childRunID == "" || deps.Jobs == nil {
		return nil
	}
	record, err := deps.Jobs.LoadChildRun(childRunID)
	if err != nil {
		return err
	}
	if record.ChildThreadID != childThreadID || record.ChildTurnID != childTurnID || childTurnID == "" {
		return errors.New("task job steer reserved first-turn identity changed")
	}
	for _, message := range record.Steers {
		if strings.TrimSpace(message.Status) != "queued" {
			continue
		}
		authorization, blocker, authorityErr := beginRuntimeTaskJobSteerAuthority(deps, record, message)
		if authorityErr != nil || (blocker == "" && (authorization.PendingUntilChildTurn ||
			strings.TrimSpace(authorization.ExpectedContextDigest) == "" || strings.TrimSpace(authorization.ProjectedText) == "")) {
			if authorization.Release != nil {
				authorization.Release()
			}
			return errors.Join(errors.New("task job steer security authority is unavailable"), authorityErr)
		}
		if blocker != "" {
			if authorization.Release != nil {
				authorization.Release()
			}
			reason := "a newly admitted turn is required"
			updated, rejected, rejectErr := deps.Jobs.RejectSteerMessage(record.ID, message, reason)
			if rejectErr != nil {
				return rejectErr
			}
			recordTaskJobSteerEvent(deps, updated, rejected, "rejected", reason)
			record = updated
			continue
		}
		// Queue content remains immutable. Revalidation may confirm it, but
		// cannot rewrite its projection or logical intent under the old digest.
		if authorization.ProjectedText != message.Text || authorization.EffectBinding.LogicalEffect != message.LogicalEffect ||
			authorization.EffectBinding.OrdinaryWork != message.OrdinaryWork ||
			domainjob.ValidateSteerMessageLogicalEffectBindingV1(message) != nil || message.LogicalEffect == "" ||
			message.ContentDigest != domainjob.SteerMessageContentDigestV1(message) {
			if authorization.Release != nil {
				authorization.Release()
			}
			return errors.New("task job steer queued content authority changed")
		}
		if message.ContextDigest == "" {
			binder, ok := deps.Jobs.(TaskJobPendingSteerTurnBinder)
			if !ok {
				if authorization.Release != nil {
					authorization.Release()
				}
				return errors.New("task job pending steer turn binding is unavailable")
			}
			var bindErr error
			record, message, bindErr = binder.BindPendingSteerMessageForTurn(record.ID, authorization.QueueAuthority, message)
			if bindErr != nil {
				if authorization.Release != nil {
					authorization.Release()
				}
				return bindErr
			}
		}
		admitErr := AdmitRuntimeTaskJobSteer(deps.Turns, record, message, authorization.ExpectedContextDigest)
		if authorization.Release != nil {
			authorization.Release()
		}
		if admitErr != nil {
			// A failed primary observation/write is not a definitive rejection
			// of the already-authorized original queued guidance.
			return admitErr
		}

	}
	return nil
}

func AdmitRuntimeTaskJobSteer(store TurnSteeringStore, record domainjob.Record, message domainjob.SteerMessage, expectedContextDigest string) error {
	if strings.TrimSpace(record.ChildThreadID) == "" || strings.TrimSpace(record.ChildTurnID) == "" {
		return nil
	}
	if store == nil || strings.TrimSpace(expectedContextDigest) == "" {
		return errors.New("runtime steering store unavailable")
	}
	_, err := store.AdmitSteeringEntryForContext(
		record.ChildThreadID, record.ChildTurnID, record.ChildTurnID, expectedContextDigest,
		TaskJobSteerTurnEntry(record, message),
	)
	return err
}

func ValidateRuntimeTaskJobSteersForPromotion(
	jobs TaskJobSteerStore,
	entries []map[string]any,
	expectedThreadID string,
	expectedTurnID string,
	expectedContextDigest string,
) error {
	for _, entry := range entries {
		jobID := strings.TrimSpace(firstNonEmptyAnyString(entry["jobId"], entry["childRunId"]))
		if jobID == "" {
			continue
		}
		if jobs == nil {
			return errors.New("task job steering authority is unavailable")
		}
		messageID := strings.TrimSpace(firstNonEmptyAnyString(entry["steerMessageId"], entry["clientUserMessageId"]))
		record, err := jobs.LoadChildRun(jobID)
		if err != nil || record.ID != jobID || record.ChildThreadID != strings.TrimSpace(expectedThreadID) ||
			record.ChildThreadID != strings.TrimSpace(firstNonEmptyAnyString(entry["childThreadId"])) ||
			record.ChildTurnID == "" || record.ChildTurnID != strings.TrimSpace(expectedTurnID) {
			return errors.New("task job steering record authority is invalid")
		}
		var queued *domainjob.SteerMessage
		for index := range record.Steers {
			if strings.TrimSpace(record.Steers[index].ID) == messageID {
				queued = &record.Steers[index]
				break
			}
		}
		jobProjectionVersion, versionOK := exactTaskJobSteerVersion(entry["jobProjectionVersion"])
		entryBinding, entryBindingPresent, entryBindingErr := domainsteering.LogicalEffectBindingFromEntryV1(entry)
		queuedBindingErr := error(nil)
		queuedBindingPresent := false
		if queued != nil {
			queuedBindingErr = domainjob.ValidateSteerMessageLogicalEffectBindingV1(*queued)
			queuedBindingPresent = queued.LogicalEffect != ""
		}
		if queued == nil || strings.TrimSpace(queued.Status) != "queued" || queued.ProjectionVersion != domainjob.SteerMessageProjectionVersionV1 ||
			queuedBindingErr != nil || entryBindingErr != nil || entryBindingPresent != queuedBindingPresent ||
			(entryBindingPresent && (entryBinding.LogicalEffect != queued.LogicalEffect || entryBinding.OrdinaryWork != queued.OrdinaryWork)) ||
			queued.ContentDigest != domainjob.SteerMessageContentDigestV1(*queued) ||
			!versionOK || jobProjectionVersion != queued.ProjectionVersion ||
			strings.TrimSpace(expectedContextDigest) == "" || queued.ContextDigest != strings.TrimSpace(expectedContextDigest) ||
			queued.AuthorityDigest != strings.TrimSpace(firstNonEmptyAnyString(entry["jobAuthorityDigest"])) ||
			queued.QueueAuthorityDigest != strings.TrimSpace(firstNonEmptyAnyString(entry["jobQueueAuthorityDigest"])) ||
			queued.ContentDigest != strings.TrimSpace(firstNonEmptyAnyString(entry["jobContentDigest"])) ||
			queued.ContextDigest != strings.TrimSpace(firstNonEmptyAnyString(entry["jobContextDigest"])) ||
			queued.JobID != jobID || queued.ChildRunID != jobID || queued.ParentThreadID != strings.TrimSpace(firstNonEmptyAnyString(entry["parentThreadId"])) ||
			queued.Text != strings.TrimSpace(firstNonEmptyAnyString(entry["text"])) || queued.CreatedAt != strings.TrimSpace(firstNonEmptyAnyString(entry["admittedAt"])) ||
			queued.SourceTurnID != strings.TrimSpace(firstNonEmptyAnyString(entry["sourceTurnId"])) ||
			queued.SourceToolCallID != strings.TrimSpace(firstNonEmptyAnyString(entry["sourceToolCallId"])) {
			return errors.New("task job steering message does not match queued authority")
		}
	}
	return nil
}

func exactTaskJobSteerVersion(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case float64:
		if typed == float64(int(typed)) {
			return int(typed), true
		}
	}
	return 0, false
}

func RecordRuntimeTaskJobSteersAdmitted(deps TaskJobSteerRuntimeDeps, commits []domainsteering.PromotionCommitV1) error {
	if deps.Jobs == nil {
		for _, commit := range commits {
			if commit.Origin == "task_job" || strings.TrimSpace(commit.JobID) != "" {
				return errors.New("task job steering settlement authority is unavailable")
			}
		}
		return nil
	}
	for _, commit := range commits {
		if commit.Origin != "task_job" && strings.TrimSpace(commit.JobID) == "" {
			continue
		}
		expected, err := taskJobSteerMessageFromPromotionCommit(commit)
		if err != nil {
			return err
		}
		record, message, err := deps.Jobs.SettleSteerPromotionExact(commit.JobID, expected, domainjob.SteerPromotionSettlementV1{
			Version: domainjob.SteerPromotionSettlementVersionV1, PromotionCommitID: commit.CommitID,
			PromotionEntryID: commit.EntryID, ContextDigest: commit.ContextDigest, PromotedAt: commit.PromotedAt,
		})
		if err != nil {
			return errors.New("task job steering settlement failed")
		}
		recordTaskJobSteerEvent(deps, record, message, "admitted", "")
	}
	return nil
}

func taskJobSteerMessageFromPromotionCommit(commit domainsteering.PromotionCommitV1) (domainjob.SteerMessage, error) {
	entry := commit.Entry
	version, versionOK := exactTaskJobSteerVersion(entry["jobProjectionVersion"])
	message := domainjob.SteerMessage{
		ID:                   strings.TrimSpace(firstNonEmptyAnyString(entry["steerMessageId"])),
		ParentThreadID:       strings.TrimSpace(firstNonEmptyAnyString(entry["parentThreadId"])),
		ChildRunID:           strings.TrimSpace(firstNonEmptyAnyString(entry["childRunId"])),
		JobID:                strings.TrimSpace(firstNonEmptyAnyString(entry["jobId"])),
		Text:                 strings.TrimSpace(firstNonEmptyAnyString(entry["text"])),
		ProjectionVersion:    version,
		ContentDigest:        strings.TrimSpace(firstNonEmptyAnyString(entry["jobContentDigest"])),
		ContextDigest:        strings.TrimSpace(firstNonEmptyAnyString(entry["jobContextDigest"])),
		AuthorityDigest:      strings.TrimSpace(firstNonEmptyAnyString(entry["jobAuthorityDigest"])),
		QueueAuthorityDigest: strings.TrimSpace(firstNonEmptyAnyString(entry["jobQueueAuthorityDigest"])),
		Status:               "queued", CreatedAt: strings.TrimSpace(firstNonEmptyAnyString(entry["admittedAt"])),
		SourceTurnID:     strings.TrimSpace(firstNonEmptyAnyString(entry["sourceTurnId"])),
		SourceToolCallID: strings.TrimSpace(firstNonEmptyAnyString(entry["sourceToolCallId"])),
		LogicalEffect:    domainsecurity.LogicalEffect(strings.TrimSpace(firstNonEmptyAnyString(entry["logicalEffect"]))),
		OrdinaryWork:     boolField(entry, "ordinaryWork"),
	}
	if commit.Version != domainsteering.ProjectionVersionV1 || commit.Origin != "task_job" ||
		strings.TrimSpace(commit.CommitID) == "" || strings.TrimSpace(commit.EntryID) == "" ||
		commit.JobID != message.JobID || commit.SteerMessageID != message.ID || commit.ContextDigest != message.ContextDigest ||
		!versionOK || version != domainjob.SteerMessageProjectionVersionV1 ||
		domainjob.ValidateSteerMessageLogicalEffectBindingV1(message) != nil ||
		message.ContentDigest != domainjob.SteerMessageContentDigestV1(message) {
		return domainjob.SteerMessage{}, errors.New("task job steering promotion commit does not match queued authority")
	}
	return message, nil
}

func recordTaskJobSteerEvent(deps TaskJobSteerRuntimeDeps, record domainjob.Record, message domainjob.SteerMessage, status string, reason string) {
	if deps.RecordEvent == nil {
		return
	}
	status = domainjob.PublicSteerStatusV1(status)
	deps.RecordEvent(BuildChildSteerEvent(ChildSteerEventInput{Record: record, Message: message, Status: status, Reason: reason}), "child steer "+status)
}

type ChildSteerEventInput struct {
	Record  domainjob.Record
	Message domainjob.SteerMessage
	Status  string
	Reason  string
}

func BuildChildSteerEvent(input ChildSteerEventInput) map[string]any {
	record := input.Record
	message := input.Message
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = strings.TrimSpace(message.Status)
	}
	status = domainjob.PublicSteerStatusV1(status)
	kind := "child_steer_" + status
	child := EventChild(record, record.Status, "")
	AddSteerStateMetadata(child, record)
	child["steerMessageId"] = message.ID
	child["steerStatus"] = status
	reason := ProjectOrdinaryJobText(input.Reason)
	if reason != "" {
		child["lastSteerReason"] = reason
	}
	event := map[string]any{
		"kind":             kind,
		"threadId":         record.ParentThreadID,
		"turnId":           record.ParentTurnID,
		"itemId":           record.ParentToolItemID,
		"callId":           record.ParentToolCallID,
		"toolName":         JobDefaultToolName(record),
		"status":           status,
		"jobId":            record.ID,
		"childRunId":       record.ID,
		"childThreadId":    record.ChildThreadID,
		"childTurnId":      record.ChildTurnID,
		"steerMessageId":   message.ID,
		"parentThreadId":   record.ParentThreadID,
		"sourceTurnId":     message.SourceTurnID,
		"sourceToolCallId": message.SourceToolCallID,
		"createdAt":        message.CreatedAt,
		"admittedAt":       message.AdmittedAt,
		"reason":           reason,
		"child":            child,
	}
	return event
}
