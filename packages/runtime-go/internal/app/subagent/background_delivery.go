package subagent

import (
	"errors"
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type BackgroundDeliveryThreadStore interface {
	GetThread(string) (map[string]any, error)
	EnsureBackgroundDeliveryItemExact(string, string, map[string]any) (BackgroundDeliveryEnsureItemResultV1, error)
}

type BackgroundDeliveryEnsureItemResultV1 string

const (
	BackgroundDeliveryItemInsertedV1      BackgroundDeliveryEnsureItemResultV1 = "inserted"
	BackgroundDeliveryItemExistingExactV1 BackgroundDeliveryEnsureItemResultV1 = "existing_exact"
	BackgroundDeliveryItemConflictV1      BackgroundDeliveryEnsureItemResultV1 = "conflict"
)

type BackgroundDeliveryJobStore interface {
	LoadChildRun(string) (domainjob.Record, error)
	UpdateChildRun(string, domainjob.UpdateRequest) (domainjob.Record, error)
}

type BackgroundDeliveryService struct {
	Threads             BackgroundDeliveryThreadStore
	Jobs                BackgroundDeliveryJobStore
	SecurityBlocker     func(domainjob.Record) string
	SecurityBlockerAt   func(domainjob.Record, time.Time) string
	CompletionAuthority BackgroundAutoContinueCompletionAuthorityV1
	AutoContinueStarter BackgroundAutoContinueStarterV1
	RecordEvent         func(map[string]any, string)
	LifecyclePublisher  BackgroundCompletionLifecyclePublisherV1
	MarkDeadLetter      func(domainjob.Record, string) error
	LifecycleDiagnostic func(string)
}

func (service *BackgroundDeliveryService) BindLifecycleRuntimeV1(
	publisher BackgroundCompletionLifecyclePublisherV1,
	markDeadLetter func(domainjob.Record, string) error,
	diagnostic func(string),
) {
	if service == nil {
		return
	}
	service.LifecyclePublisher = publisher
	service.MarkDeadLetter = markDeadLetter
	service.LifecycleDiagnostic = diagnostic
}

type backgroundCompletionAdmissionV1 struct {
	event      map[string]any
	itemID     string
	deliveryID string
}

type backgroundDeliveryLedgerFieldsV1 struct {
	status    string
	reason    string
	errorText string
	timestamp string
}

func (service BackgroundDeliveryService) PersistCompletionItems(threadID, turnID, callID, toolName string, events []map[string]any) {
	for _, event := range events {
		jobID := BackgroundJobIDFromEvent(event)
		if jobID == "" {
			continue
		}
		if service.Jobs == nil {
			continue
		}
		record, err := service.Jobs.LoadChildRun(jobID)
		if err != nil {
			service.UpdateCompletion(jobID, BackgroundJobCompletionDeliveryID(turnID, event), BackgroundJobCompletionItemID(turnID, event), "dead_letter", "job_record_missing", "")
			continue
		}
		admission, reason := service.admitCompletion(threadID, turnID, callID, toolName, record, event)
		if reason != "" {
			service.UpdateCompletion(jobID, admission.deliveryID, admission.itemID, "dead_letter", reason, "")
			continue
		}
		pending := service.UpdateCompletion(jobID, admission.deliveryID, admission.itemID, "pending", "", "")
		if pending.ID != "" && pending.CompletionDeliveryStatus == "dead_letter" {
			continue
		}
		if pending.ID == "" {
			latest, loadErr := service.Jobs.LoadChildRun(jobID)
			if loadErr != nil || (latest.CompletionDeliveryStatus != "delivered" &&
				!(latest.CompletionDeliveryStatus == "" && latest.RecoveryStatus == "recovered")) {
				continue
			}
		}
		item, ok := BuildBackgroundJobCompletionNotificationItem(BackgroundJobCompletionNotificationItemInput{
			ThreadID: threadID, TurnID: turnID, ItemID: admission.itemID, CallID: callID, ToolName: toolName,
			CreatedAt: backgroundCompletionTimestamp(record), Event: admission.event,
		})
		if !ok {
			service.UpdateCompletion(jobID, admission.deliveryID, admission.itemID, "dead_letter", "build_notification_item_failed", "")
			continue
		}
		result, err := service.Threads.EnsureBackgroundDeliveryItemExact(threadID, turnID, item)
		if err != nil {
			service.UpdateCompletion(jobID, admission.deliveryID, admission.itemID, "dead_letter", "append_parent_turn_failed", "")
			continue
		}
		switch result {
		case BackgroundDeliveryItemInsertedV1:
			service.UpdateCompletion(jobID, admission.deliveryID, admission.itemID, "delivered", "", "")
		case BackgroundDeliveryItemExistingExactV1:
			service.UpdateCompletion(jobID, admission.deliveryID, admission.itemID, "delivered", "duplicate_delivery", "")
		case BackgroundDeliveryItemConflictV1:
			service.UpdateCompletion(jobID, admission.deliveryID, admission.itemID, "dead_letter", "parent_tool_identity_invalid", "")
		default:
			service.UpdateCompletion(jobID, admission.deliveryID, admission.itemID, "dead_letter", "append_parent_turn_failed", "")
		}
	}
}

func (service BackgroundDeliveryService) admitCompletion(
	threadID, turnID, callID, toolName string,
	record domainjob.Record,
	event map[string]any,
) (backgroundCompletionAdmissionV1, string) {
	admission := backgroundCompletionAdmissionV1{}
	if service.Threads == nil {
		return admission, "runtime_store_unavailable"
	}
	if !record.Background {
		return admission, "parent_tool_identity_invalid"
	}
	if !TaskJobTerminal(record) {
		return admission, "parent_tool_identity_invalid"
	}
	if threadID != strings.TrimSpace(record.ParentThreadID) {
		return admission, "parent_security_context_mismatch"
	}
	if turnID != strings.TrimSpace(record.ParentTurnID) {
		return admission, "parent_security_context_mismatch"
	}
	if reason := service.completionSecurityBlocker(record); reason != "" {
		return admission, reason
	}
	if reason := service.CompletionBlocker(threadID, turnID); reason != "" {
		return admission, reason
	}
	thread, err := service.Threads.GetThread(threadID)
	if err != nil || thread == nil {
		return admission, "parent_thread_missing"
	}
	_, expectedCallID, expectedToolName, ok := JobToolIdentity(thread, record)
	if !ok || callID != expectedCallID || toolName != expectedToolName {
		return admission, "parent_tool_identity_invalid"
	}
	if event == nil || event["kind"] != "pipeline_stage" ||
		backgroundStringField(event, "threadId") != strings.TrimSpace(record.ParentThreadID) ||
		backgroundStringField(event, "turnId") != strings.TrimSpace(record.ParentTurnID) {
		return admission, "parent_tool_identity_invalid"
	}
	stage := strings.TrimSpace(backgroundStringField(event, "stage"))
	if !strings.HasPrefix(stage, "background_job_") {
		return admission, "parent_tool_identity_invalid"
	}
	details, _ := event["details"].(map[string]any)
	jobID := backgroundStringField(details, "jobId")
	childRunID := backgroundStringField(details, "childRunId")
	if details == nil || details["notificationKind"] != "background_job_completion" ||
		jobID != strings.TrimSpace(record.ID) || childRunID != strings.TrimSpace(record.ID) {
		return admission, "parent_tool_identity_invalid"
	}
	stageStatus := strings.TrimSpace(strings.TrimPrefix(stage, "background_job_"))
	detailStatus := strings.TrimSpace(backgroundStringField(details, "status"))
	recordStatus := strings.TrimSpace(record.Status)
	if stageStatus != recordStatus || (detailStatus != "" && detailStatus != recordStatus) || normalizedChildStatus(stageStatus) != recordStatus {
		return admission, "parent_tool_identity_invalid"
	}
	safeEvent, ok := BuildBackgroundJobCompletionNotificationEvent(JobLifecycleEventInput{
		ThreadID: threadID, TurnID: turnID, CallID: expectedCallID, ToolName: expectedToolName,
		Record: record, Status: recordStatus, ProgressStatus: normalizedToolProgressStatus(recordStatus),
	})
	if !ok {
		return admission, "build_notification_item_failed"
	}
	admission.event = safeEvent
	admission.itemID = BackgroundJobCompletionItemID(turnID, safeEvent)
	admission.deliveryID = BackgroundJobCompletionDeliveryID(turnID, safeEvent)
	if existing := strings.TrimSpace(record.CompletionDeliveryItemID); existing != "" && existing != admission.itemID {
		return admission, "parent_tool_identity_invalid"
	}
	if existing := strings.TrimSpace(record.CompletionDeliveryID); existing != "" {
		admission.deliveryID = existing
	}
	if admission.itemID == "" || admission.deliveryID == "" {
		return admission, "parent_tool_identity_invalid"
	}
	return admission, ""
}

func (service BackgroundDeliveryService) CompletionBlocker(threadID, turnID string) string {
	if service.Threads == nil {
		return "runtime_store_unavailable"
	}
	thread, err := service.Threads.GetThread(threadID)
	if err != nil || thread == nil {
		return "parent_thread_missing"
	}
	if strings.TrimSpace(backgroundStringField(thread, "status")) == "archived" || backgroundBoolField(thread, "archived") {
		return "parent_thread_archived"
	}
	if _, ok := FindTurn(thread, turnID); !ok {
		return "parent_turn_missing"
	}
	return ""
}

func (service BackgroundDeliveryService) UpdateCompletion(jobID, deliveryID, itemID, status, reason, errorText string) domainjob.Record {
	if service.Jobs == nil || strings.TrimSpace(jobID) == "" {
		return domainjob.Record{}
	}
	reason = domainjob.NormalizeOperationalReasonV1(reason)
	errorText = ""
	updated, err := service.Jobs.UpdateChildRun(jobID, domainjob.UpdateRequest{
		CompletionDeliveryID: strings.TrimSpace(deliveryID), CompletionDeliveryItemID: strings.TrimSpace(itemID),
		CompletionDeliveryStatus: strings.TrimSpace(status), CompletionDeliveryReason: strings.TrimSpace(reason),
		CompletionDeliveryError: strings.TrimSpace(errorText),
	})
	if err != nil {
		return domainjob.Record{}
	}
	ledgerResult, _ := service.persistCompletionLedgerItemExact(updated)
	if ledgerResult == BackgroundDeliveryItemConflictV1 && strings.TrimSpace(updated.CompletionDeliveryStatus) != "dead_letter" {
		deadLettered, updateErr := service.Jobs.UpdateChildRun(jobID, domainjob.UpdateRequest{
			CompletionDeliveryID: strings.TrimSpace(deliveryID), CompletionDeliveryItemID: strings.TrimSpace(itemID),
			CompletionDeliveryStatus: "dead_letter", CompletionDeliveryReason: "parent_tool_identity_invalid",
		})
		if updateErr != nil {
			return domainjob.Record{}
		}
		_, _ = service.persistCompletionLedgerItemExact(deadLettered)
		return deadLettered
	}
	return updated
}

func (service BackgroundDeliveryService) MaybeAutoContinue(record domainjob.Record, status, message string) {
	_ = message
	_ = service.maybeAutoContinue(record, status)
}

func (service BackgroundDeliveryService) AutoContinueGate(record domainjob.Record) string {
	return service.autoContinueAuthorityGate(record)
}

func (service BackgroundDeliveryService) RepairSettledCompletionLedger(record domainjob.Record) error {
	if !completionDeliverySettled(record.CompletionDeliveryStatus) {
		return errors.New("background delivery record is not settled")
	}
	result, err := service.persistCompletionLedgerItemExact(record)
	if err != nil {
		return err
	}
	if result == BackgroundDeliveryItemConflictV1 {
		return errors.New("background delivery ledger identity conflicts")
	}
	return nil
}

func (service BackgroundDeliveryService) persistCompletionLedgerItemExact(record domainjob.Record) (BackgroundDeliveryEnsureItemResultV1, error) {
	if service.Threads == nil {
		return "", errors.New("runtime store is unavailable")
	}
	if !record.Background || !TaskJobTerminal(record) {
		return "", errors.New("background delivery record is ineligible")
	}
	fields := backgroundDeliveryLedgerFields(record)
	if blocker := service.securityBlockerAt(record, fields.timestamp); blocker != "" {
		return "", errors.New(blocker)
	}
	threadID, turnID := strings.TrimSpace(record.ParentThreadID), strings.TrimSpace(record.ParentTurnID)
	jobID, deliveryID, status := strings.TrimSpace(record.ID), strings.TrimSpace(record.CompletionDeliveryID), fields.status
	if threadID == "" || turnID == "" || jobID == "" || deliveryID == "" || status == "" || fields.timestamp == "" {
		return "", errors.New("background delivery ledger identity is incomplete")
	}
	if blocker := service.CompletionBlocker(threadID, turnID); blocker != "" {
		return "", errors.New(blocker)
	}
	itemID := BackgroundCompletionDeliveryItemID(turnID, deliveryID, status)
	if itemID == "" {
		return "", errors.New("background delivery ledger item identity is invalid")
	}
	item, ok := BuildBackgroundJobDeliveryLedgerItem(BackgroundJobDeliveryLedgerItemInput{
		ThreadID: threadID, TurnID: turnID, ItemID: itemID, CallID: BackgroundCompletionDeliveryCallID(deliveryID, status),
		CreatedAt: fields.timestamp, Record: record, Status: status, Reason: fields.reason, Error: fields.errorText,
	})
	if !ok {
		return "", errors.New("background delivery ledger item is invalid")
	}
	result, err := service.Threads.EnsureBackgroundDeliveryItemExact(threadID, turnID, item)
	if err != nil {
		return "", err
	}
	if result == BackgroundDeliveryItemInsertedV1 {
		service.recordEvent(map[string]any{
			"kind": "item_created", "threadId": threadID, "turnId": turnID, "itemId": itemID, "timestamp": fields.timestamp, "item": item,
		}, "background job delivery ledger item")
	}
	return result, nil
}

func (service BackgroundDeliveryService) updateAutoContinue(record domainjob.Record, status, turnID, reason, errorText string, lateSuppressed bool) domainjob.Record {
	if service.Jobs == nil || strings.TrimSpace(record.ID) == "" {
		return record
	}
	reason = domainjob.NormalizeOperationalReasonV1(reason)
	errorText = ""
	request := domainjob.UpdateRequest{
		AutoContinueStatus: strings.TrimSpace(status), AutoContinueTurnID: strings.TrimSpace(turnID),
		AutoContinueReason: strings.TrimSpace(reason), AutoContinueError: strings.TrimSpace(errorText),
	}
	if lateSuppressed {
		value := true
		request.LateCompletionSuppressed = &value
		request.LateCompletionReason = strings.TrimSpace(reason)
	}
	if updated, err := service.Jobs.UpdateChildRun(record.ID, request); err == nil {
		return updated
	}
	return record
}

func (service BackgroundDeliveryService) RepairAutoContinueNotice(record domainjob.Record) error {
	// Rejections and pre-admission reservations are durable job state only.
	// The old terminal parent receives a typed lifecycle append only after the
	// normal turn pipeline has admitted the exact reserved continuation.
	if strings.TrimSpace(record.AutoContinueStatus) != "started" {
		return nil
	}
	result, err := service.persistAutoContinueItemExact(record)
	if err != nil {
		return err
	}
	if result == BackgroundDeliveryItemConflictV1 {
		return errors.New("background auto-continue item identity conflicts")
	}
	return nil
}

func (service BackgroundDeliveryService) persistAutoContinueItemExact(record domainjob.Record) (BackgroundDeliveryEnsureItemResultV1, error) {
	if service.Threads == nil {
		return "", errors.New("runtime store is unavailable")
	}
	if !record.Background || !record.AutoContinueParent || !TaskJobTerminal(record) {
		return "", errors.New("background auto-continue record is ineligible")
	}
	threadID, turnID, jobID := strings.TrimSpace(record.ParentThreadID), strings.TrimSpace(record.ParentTurnID), strings.TrimSpace(record.ID)
	status := domainjob.PublicAutoContinueStatusV1(record.AutoContinueStatus)
	reason := domainjob.NormalizeOperationalReasonV1(record.AutoContinueReason)
	timestamp := backgroundAutoContinueTimestamp(record)
	authorityTimestamp := strings.TrimSpace(record.CompletionDeliveryAt)
	if status == "" {
		return "", errors.New("background auto-continue status is unavailable")
	}
	if threadID == "" || turnID == "" || jobID == "" || timestamp == "" || authorityTimestamp == "" {
		return "", errors.New("background auto-continue identity is incomplete")
	}
	if blocker := service.securityBlockerAt(record, authorityTimestamp); blocker != "" {
		return "", errors.New(blocker)
	}
	if blocker := service.CompletionBlocker(threadID, turnID); blocker != "" {
		return "", errors.New(blocker)
	}
	details := map[string]any{
		"notificationKind": "background_job_auto_continue", "jobId": record.ID, "childRunId": record.ID,
		"status": status, "autoContinueParent": record.AutoContinueParent, "autoContinueStatus": domainjob.PublicAutoContinueStatusV1(record.AutoContinueStatus),
		"lateCompletionSuppressed": record.LateCompletionSuppressed,
	}
	if value := strings.TrimSpace(record.AutoContinueTurnID); value != "" {
		details["autoContinueTurnId"] = value
	}
	if value := strings.TrimSpace(reason); value != "" {
		details["reason"] = value
	}
	itemID := BackgroundAutoContinueItemID(turnID, jobID, status)
	if itemID == "" {
		return "", errors.New("background auto-continue item identity is invalid")
	}
	message := BackgroundAutoContinueStatusMessage(status, reason, "")
	diagnostics := map[string]any{}
	if ChildOutputRequiresWithholding(record) {
		diagnostics = SecurityBoundChildOutputProjection(record)
		diagnostics["notificationKind"] = "background_job_auto_continue"
		diagnostics["autoContinueStatus"] = status
		if reason != "" {
			diagnostics["reason"] = reason
		}
		message = securityBoundOutputMessage
	} else {
		for key, value := range details {
			diagnostics[key] = value
		}
		if value := strings.TrimSpace(record.ChildThreadID); value != "" {
			diagnostics["childThreadId"] = value
		}
		if value := strings.TrimSpace(record.ChildTurnID); value != "" {
			diagnostics["childTurnId"] = value
		}
	}
	if value := strings.TrimSpace(record.CompletionDeliveryID); value != "" {
		diagnostics["deliveryId"] = value
	}
	if value := strings.TrimSpace(record.CompletionDeliveryStatus); value != "" {
		diagnostics["deliveryStatus"] = domainjob.PublicCompletionDeliveryStatusV1(value)
	}
	if value := strings.TrimSpace(record.CompletionDeliveryReason); value != "" {
		diagnostics["deliveryReason"] = value
	}
	item := map[string]any{
		"id": itemID, "turnId": turnID, "threadId": threadID, "role": "tool", "status": "completed",
		"createdAt": timestamp, "finishedAt": timestamp, "kind": "tool_progress", "toolName": "background_auto_continue",
		"callId": BackgroundAutoContinueCallID(jobID, status), "summary": message, "message": message,
		"arguments": map[string]any{
			"runtimeStatus": "tool_progress", "stage": "background_job_auto_continue_" + status,
			"message": message, "status": status, "diagnostics": diagnostics,
		},
	}
	result, err := service.Threads.EnsureBackgroundDeliveryItemExact(threadID, turnID, item)
	if err != nil {
		return "", err
	}
	if result != BackgroundDeliveryItemInsertedV1 {
		return result, nil
	}
	child := EventChild(record, record.Status, "")
	eventDetails := details
	if ChildOutputRequiresWithholding(record) {
		child = SecurityBoundChildOutputProjection(record)
		eventDetails = diagnostics
	}
	service.recordEvent(map[string]any{
		"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID,
		"stage": "background_job_auto_continue_" + status, "label": backgroundFirstNonEmpty(domainjob.PublicKindV1(record.Kind), record.ID),
		"message": message, "child": child, "details": eventDetails,
	}, "background job auto continue")
	service.recordEvent(map[string]any{
		"kind": "item_created", "threadId": threadID, "turnId": turnID, "itemId": itemID, "timestamp": timestamp, "item": item,
	}, "background job auto continue item")
	return result, nil
}

func (service BackgroundDeliveryService) securityBlocker(record domainjob.Record) string {
	if service.SecurityBlocker == nil {
		return "job_security_authority_unavailable"
	}
	return strings.TrimSpace(service.SecurityBlocker(record))
}

func (service BackgroundDeliveryService) securityBlockerAt(record domainjob.Record, timestamp string) string {
	at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(timestamp))
	if err != nil {
		return "job_durable_timestamp_invalid"
	}
	if service.SecurityBlockerAt != nil {
		return strings.TrimSpace(service.SecurityBlockerAt(record, at.UTC()))
	}
	return service.securityBlocker(record)
}

// completionSecurityBlocker uses historical authority only for replay of an
// already-delivered terminal lifecycle. First delivery and every live
// auto-continue admission continue to require the current execution authority.
func (service BackgroundDeliveryService) completionSecurityBlocker(record domainjob.Record) string {
	if record.Background && TaskJobTerminal(record) &&
		strings.TrimSpace(record.CompletionDeliveryStatus) == "delivered" &&
		strings.TrimSpace(record.CompletionDeliveryAt) != "" {
		return service.securityBlockerAt(record, record.CompletionDeliveryAt)
	}
	return service.securityBlocker(record)
}

func (service BackgroundDeliveryService) recordEvent(event map[string]any, context string) {
	if service.RecordEvent != nil {
		service.RecordEvent(event, context)
	}
}

func backgroundCompletionTimestamp(record domainjob.Record) string {
	return strings.TrimSpace(record.FinishedAt)
}

func backgroundAutoContinueTimestamp(record domainjob.Record) string {
	return backgroundFirstNonEmpty(record.AutoContinueUpdatedAt, record.UpdatedAt, record.FinishedAt)
}

func backgroundDeliveryLedgerFields(record domainjob.Record) backgroundDeliveryLedgerFieldsV1 {
	return backgroundDeliveryLedgerFieldsV1{
		status:    strings.TrimSpace(record.CompletionDeliveryStatus),
		reason:    domainjob.NormalizeOperationalReasonV1(record.CompletionDeliveryReason),
		errorText: strings.TrimSpace(record.CompletionDeliveryError),
		timestamp: backgroundDeliveryTimestamp(record),
	}
}

func backgroundDeliveryTimestamp(record domainjob.Record) string {
	switch strings.TrimSpace(record.CompletionDeliveryStatus) {
	case "delivered", "skipped":
		if value := strings.TrimSpace(record.CompletionDeliveryAt); value != "" {
			return value
		}
	case "dead_letter":
		if value := strings.TrimSpace(record.CompletionDeadLetterAt); value != "" {
			return value
		}
	}
	return strings.TrimSpace(record.UpdatedAt)
}

func backgroundFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func backgroundStringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func backgroundBoolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func backgroundListAny(value any) []any {
	items, _ := value.([]any)
	return items
}
