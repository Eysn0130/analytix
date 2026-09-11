package subagent

import (
	"errors"
	"reflect"
	"strings"
	"time"

	eventrecordingapp "analytix.local/runtime-go/internal/app/eventrecording"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	subagentstartupport "analytix.local/runtime-go/internal/ports/subagentstartup"
)

const startupRecoveryReason = "runtime_startup_recovery"

type StartupRecoveryDependencies struct {
	Threads             subagentstartupport.ThreadStore
	Jobs                subagentstartupport.JobStore
	Security            subagentstartupport.SecurityAuthority
	CompletionAuthority BackgroundAutoContinueCompletionAuthorityV1
	AutoContinueStarter BackgroundAutoContinueStarterV1
	RecordEvent         func(map[string]any, string)
	Now                 func() time.Time
}

func NewStartupRecoveryDependenciesV1(
	threads subagentstartupport.ThreadStore,
	jobs subagentstartupport.JobStore,
	security subagentstartupport.SecurityAuthority,
	delivery BackgroundDeliveryService,
	recordEvent func(map[string]any, string),
) StartupRecoveryDependencies {
	return StartupRecoveryDependencies{
		Threads: threads, Jobs: jobs, Security: security,
		CompletionAuthority: delivery.CompletionAuthority, AutoContinueStarter: delivery.AutoContinueStarter,
		RecordEvent: recordEvent,
	}
}

func NewStartupRecoveryServiceV1(
	threads subagentstartupport.ThreadStore,
	jobs subagentstartupport.JobStore,
	security subagentstartupport.SecurityAuthority,
	delivery BackgroundDeliveryService,
	recordEvent func(map[string]any, string),
) StartupRecoveryService {
	return NewStartupRecoveryService(NewStartupRecoveryDependenciesV1(threads, jobs, security, delivery, recordEvent))
}

type BackgroundCompletionLifecyclePublisherV1 interface {
	PublishBackgroundCompletionLifecycleExact([]map[string]any) error
}

type BackgroundCompletionLifecycleExactOperationsV1 struct {
	LoadEvents          func(string) ([]map[string]any, error)
	PublicationReserved func(string) bool
	ReadThread          func(string) (map[string]any, error)
	ProjectThread       func(string, map[string]any) (map[string]any, error)
	BeforeRecord        func(map[string]any) error
	NextSequence        func(string) (int, error)
	Persist             func(string, []map[string]any, bool) error
	MissingThread       error
	SetHighest          func(string, int)
	Publish             func(string, []map[string]any)
}

func NewBackgroundCompletionLifecycleExactOperationsV1(
	loadEvents func(string) ([]map[string]any, error),
	publicationReserved func(string) bool,
	readThread func(string) (map[string]any, error),
	projectThread func(string, map[string]any) (map[string]any, error),
	beforeRecord func(map[string]any) error,
	nextSequence func(string) (int, error),
	persist func(string, []map[string]any, bool) error,
	missingThread error,
	setHighest func(string, int),
	publish func(string, []map[string]any),
) BackgroundCompletionLifecycleExactOperationsV1 {
	return BackgroundCompletionLifecycleExactOperationsV1{
		LoadEvents: loadEvents, PublicationReserved: publicationReserved, ReadThread: readThread,
		ProjectThread: projectThread, BeforeRecord: beforeRecord, NextSequence: nextSequence,
		Persist: persist, MissingThread: missingThread, SetHighest: setHighest, Publish: publish,
	}
}

type BackgroundLifecycleRecordResultV1 struct {
	Record  domainjob.Record
	Blocker string
}

func FinishBackgroundLifecycleRecordV1(
	result BackgroundLifecycleRecordResultV1,
	recordErr error,
	markDeadLetter func(domainjob.Record, string) error,
	diagnostic func(),
) {
	if recordErr != nil {
		if diagnostic != nil {
			diagnostic()
		}
		return
	}
	if result.Blocker == "" || markDeadLetter == nil {
		return
	}
	if err := markDeadLetter(result.Record, result.Blocker); err != nil && diagnostic != nil {
		diagnostic()
	}
}

func (service BackgroundDeliveryService) finishLifecycleRecordV1(result BackgroundLifecycleRecordResultV1, err error, owner string) {
	FinishBackgroundLifecycleRecordV1(result, err, service.MarkDeadLetter, func() {
		if service.LifecycleDiagnostic != nil {
			service.LifecycleDiagnostic(owner)
		}
	})
}

func (service BackgroundDeliveryService) RecordSubagentLifecycleV1(
	threadID, turnID, itemID, callID, toolName string,
	record domainjob.Record,
	status, message string,
) {
	result, err := RecordBackgroundSubagentLifecycleV1(
		service, service.LifecyclePublisher, threadID, turnID, itemID, callID, toolName, record, status, message,
	)
	service.finishLifecycleRecordV1(result, err, "subagent")
}

func (service BackgroundDeliveryService) RecordJobLifecycleV1(record domainjob.Record) {
	result, err := RecordBackgroundJobLifecycleV1(service, service.LifecyclePublisher, record)
	service.finishLifecycleRecordV1(result, err, "job")
}

func NewBackgroundLifecycleDeliveryServiceV1(
	threads BackgroundDeliveryThreadStore,
	jobs BackgroundDeliveryJobStore,
	securityBlocker func(domainjob.Record) string,
	recordEvent func(map[string]any, string),
) BackgroundDeliveryService {
	return BackgroundDeliveryService{
		Threads: threads, Jobs: jobs, SecurityBlocker: securityBlocker, RecordEvent: recordEvent,
	}
}

func TurnCompletionFromThreadStoreV1(
	load func(string) (map[string]any, error),
	threadID, turnID string,
) (string, int, error) {
	if load == nil || !domainthread.IsCanonicalRecordID(threadID) || !domainthread.IsCanonicalRecordID(turnID) {
		return "", 0, errors.New("child completion thread reader is unavailable")
	}
	thread, err := load(threadID)
	if err != nil {
		return "", 0, err
	}
	if thread == nil || strings.TrimSpace(threadID) == "" || firstNonEmptyAnyString(thread["id"]) != threadID {
		return "", 0, errors.New("child completion thread authority is unavailable")
	}
	turns, ok := thread["turns"].([]any)
	if !ok {
		return "", 0, errors.New("child completion turn authority is unavailable")
	}
	var selected map[string]any
	for _, raw := range turns {
		turn, ok := raw.(map[string]any)
		if !ok {
			return "", 0, errors.New("child completion turn is invalid")
		}
		if firstNonEmptyAnyString(turn["id"]) != turnID {
			continue
		}
		if selected != nil || (turn["threadId"] != nil && turn["threadId"] != threadID) {
			return "", 0, errors.New("child completion turn identity is invalid")
		}
		if rawItems := turn["items"]; rawItems != nil {
			items, ok := rawItems.([]any)
			if !ok {
				return "", 0, errors.New("child completion turn items are invalid")
			}
			for _, rawItem := range items {
				if _, ok := rawItem.(map[string]any); !ok {
					return "", 0, errors.New("child completion turn item is invalid")
				}
			}
		}
		selected = turn
	}
	if selected == nil {
		return "", 0, errors.New("child completion turn authority is unavailable")
	}
	return SummaryFromThread(thread, turnID), ToolInvocationCountFromThread(thread, turnID), nil
}

// StartupRecoveryService reconciles terminal child jobs before the runtime
// accepts new work. It owns the ordering between current-authority checks,
// retry audit state, parent tool settlement, and completion delivery.
type StartupRecoveryService struct {
	threads   subagentstartupport.ThreadStore
	jobs      subagentstartupport.JobStore
	security  subagentstartupport.SecurityAuthority
	delivery  BackgroundDeliveryService
	lifecycle BackgroundCompletionLifecyclePublisherV1
}

func NewStartupRecoveryService(deps StartupRecoveryDependencies) StartupRecoveryService {
	deliveryThreads, _ := deps.Threads.(BackgroundDeliveryThreadStore)
	service := StartupRecoveryService{
		threads:  deps.Threads,
		jobs:     deps.Jobs,
		security: deps.Security,
	}
	service.lifecycle, _ = deps.Threads.(BackgroundCompletionLifecyclePublisherV1)
	service.delivery = BackgroundDeliveryService{
		Threads:             deliveryThreads,
		Jobs:                deps.Jobs,
		SecurityBlocker:     service.securityBlocker,
		SecurityBlockerAt:   service.securityBlockerAt,
		CompletionAuthority: deps.CompletionAuthority,
		AutoContinueStarter: deps.AutoContinueStarter,
		RecordEvent:         deps.RecordEvent,
	}
	if historical, ok := deps.Security.(interface {
		HistoricalBlockerAt(domainjob.Record, time.Time) string
	}); ok {
		service.delivery.SecurityBlockerAt = historical.HistoricalBlockerAt
	}
	return service
}

// RecoverInterrupted settles records that the durable job manager changed
// from running to interrupted during startup cleanup.
func (service StartupRecoveryService) RecoverInterrupted(records []domainjob.Record) error {
	for _, record := range records {
		if service.jobs == nil {
			return errors.New("job recovery store is unavailable")
		}
		latest, err := service.jobs.LoadChildRun(record.ID)
		if err != nil {
			return err
		}
		record = latest
		if held, err := service.jobs.RestartPreservesChildRunV1(record); err != nil {
			return err
		} else if held {
			continue
		}
		if completionDeliverySettled(record.CompletionDeliveryStatus) {
			continue
		}
		if blocked, err := service.deadLetterIfBlocked(record); blocked || err != nil {
			if err != nil {
				return err
			}
			continue
		}
		if blocked, err := service.deadLetterIfParentInvalid(record); blocked || err != nil {
			if err != nil {
				return err
			}
			continue
		}
		updated, err := service.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
			CompletionDeliveryStatus: "retry", CompletionDeliveryReason: startupRecoveryReason,
			RecoveryStatus: "recovering", RecoveryReason: startupRecoveryReason,
		})
		if err != nil {
			return err
		}
		record = updated
		retry := service.delivery.UpdateCompletion(
			record.ID, record.CompletionDeliveryID, record.CompletionDeliveryItemID,
			"retry", startupRecoveryReason, "",
		)
		if strings.TrimSpace(retry.ID) == "" {
			return errors.New("persist recovered job retry ledger failed")
		}
		record = retry
		message := startupLifecycleMessageV1(record)
		if err := service.settleParentTool(record, message); err != nil {
			return err
		}
		if err := service.recordLifecycle(record, record.Status, message); err != nil {
			return err
		}
	}
	return nil
}

// RecoverPendingDeliveries retries completion projection for terminal
// background jobs whose delivery ledger was not durably settled.
func (service StartupRecoveryService) RecoverPendingDeliveries() error {
	if service.jobs == nil {
		return nil
	}
	for _, record := range service.jobs.AllRecords() {
		if held, err := service.jobs.RestartPreservesChildRunV1(record); err != nil {
			return err
		} else if held {
			continue
		}
		if !record.Background || !TaskJobTerminal(record) {
			continue
		}
		if completionDeliverySettled(record.CompletionDeliveryStatus) {
			if err := service.delivery.RepairSettledCompletionLedger(record); err != nil {
				return err
			}
			if strings.TrimSpace(record.CompletionDeliveryStatus) == "delivered" {
				if err := service.reconcileSettledDeliveredLifecycle(record); err != nil {
					return err
				}
				if err := service.delivery.RecoverAutoContinue(record); err != nil {
					return err
				}
			}
			continue
		}
		if blocked, err := service.deadLetterIfBlocked(record); blocked || err != nil {
			if err != nil {
				return err
			}
			continue
		}
		if blocked, err := service.deadLetterIfParentInvalid(record); blocked || err != nil {
			if err != nil {
				return err
			}
			continue
		}
		deliveryID := strings.TrimSpace(record.CompletionDeliveryID)
		if deliveryID == "" {
			deliveryID = taskJobDeliveryID(record)
		}
		updated, err := service.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
			CompletionDeliveryID: deliveryID, CompletionDeliveryItemID: record.CompletionDeliveryItemID,
			CompletionDeliveryStatus: "retry", CompletionDeliveryReason: startupRecoveryReason,
			RecoveryStatus: "recovering", RecoveryReason: startupRecoveryReason,
		})
		if err != nil {
			return err
		}
		record = updated
		retry := service.delivery.UpdateCompletion(
			record.ID, deliveryID, record.CompletionDeliveryItemID,
			"retry", startupRecoveryReason, "",
		)
		if strings.TrimSpace(retry.ID) == "" {
			return errors.New("persist background job retry ledger failed")
		}
		record = retry
		message := startupLifecycleMessageV1(record)
		if strings.TrimSpace(record.Status) == string(domainjob.StatusInterrupted) {
			if err := service.settleParentTool(record, message); err != nil {
				return err
			}
		}
		if err := service.recordLifecycle(record, record.Status, message); err != nil {
			return err
		}
	}
	return nil
}

func (service StartupRecoveryService) reconcileSettledDeliveredLifecycle(record domainjob.Record) error {
	if strings.TrimSpace(record.CompletionDeliveryID) == "" || strings.TrimSpace(record.CompletionDeliveryItemID) == "" {
		return errors.New("settled background completion lifecycle identity is incomplete")
	}
	identity, blocker, err := service.parentToolIdentity(record)
	if err != nil {
		return err
	}
	if blocker != "" {
		return errors.New("settled background completion lifecycle parent identity is invalid")
	}
	events := BuildDurableJobLifecycleEventsV1(
		identity.threadID, identity.turnID, identity.itemID, identity.callID, identity.toolName, record,
	)
	var completion map[string]any
	for _, event := range events {
		if BackgroundJobIDFromEvent(event) == strings.TrimSpace(record.ID) {
			completion = event
			break
		}
	}
	expectedItemID := BackgroundJobCompletionItemID(identity.turnID, completion)
	if completion == nil || expectedItemID == "" || expectedItemID != strings.TrimSpace(record.CompletionDeliveryItemID) {
		return errors.New("settled background completion lifecycle item identity is invalid")
	}
	expected, ok := BuildBackgroundJobCompletionNotificationItem(BackgroundJobCompletionNotificationItemInput{
		ThreadID: identity.threadID, TurnID: identity.turnID, ItemID: expectedItemID,
		CallID: identity.callID, ToolName: identity.toolName,
		CreatedAt: backgroundCompletionTimestamp(record), Event: completion,
	})
	if !ok {
		return errors.New("settled background completion lifecycle item is invalid")
	}
	turn, ok := FindTurn(identity.thread, identity.turnID)
	if !ok {
		return errors.New("settled background completion lifecycle parent turn is missing")
	}
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if backgroundStringField(item, "id") != expectedItemID {
			continue
		}
		if !reflect.DeepEqual(cloneMap(expected), cloneMap(item)) {
			return errors.New("settled background completion lifecycle item conflicts")
		}
		return service.recordLifecycle(record, record.Status, startupLifecycleMessageV1(record))
	}
	return errors.New("settled background completion lifecycle item is missing")
}

func (service StartupRecoveryService) settleParentTool(record domainjob.Record, message string) error {
	if service.threads == nil {
		return errors.New("recovered job parent store is unavailable")
	}
	if blocked, err := service.deadLetterIfBlocked(record); blocked || err != nil {
		return err
	}
	identity, blocker, err := service.parentToolIdentity(record)
	if err != nil {
		return err
	}
	if blocker != "" {
		return service.markDeadLetter(record, blocker)
	}
	records, err := RecoveredJobToolResult(RecoveredJobToolResultInput{
		ThreadID: identity.threadID,
		TurnID:   identity.turnID,
		Record:   record,
		Message:  message,
		CallID:   identity.callID,
		ToolName: identity.toolName,
	})
	if err != nil {
		return service.markDeadLetter(record, "parent_tool_result_invalid")
	}
	result, err := service.threads.EnsureRecoveredParentToolSettlementExact(
		subagentstartupport.RecoveredParentToolSettlementInput{
			Record: record, ToolCallItemID: identity.itemID, CallID: identity.callID, ToolName: identity.toolName,
			Status: records.Status, Timestamp: RecoveredJobSettlementTimestamp(record), ResultItem: records.ResultItem,
		},
	)
	if err != nil {
		return err
	}
	switch result {
	case subagentstartupport.RecoveredParentToolSettlementInserted:
		service.recordEvent(records.Event, "recovered job tool result")
	case subagentstartupport.RecoveredParentToolSettlementExistingExact:
		return nil
	case subagentstartupport.RecoveredParentToolSettlementConflict:
		return service.markDeadLetter(record, "parent_tool_result_invalid")
	default:
		return errors.New("recovered parent tool settlement result is invalid")
	}
	return nil
}

func (service StartupRecoveryService) recordLifecycle(record domainjob.Record, status, message string) error {
	if service.jobs == nil {
		return errors.New("job recovery store is unavailable")
	}
	latest, err := service.jobs.LoadChildRun(record.ID)
	if err != nil {
		return err
	}
	record = latest
	status = strings.TrimSpace(record.Status)
	message = startupLifecycleMessageV1(record)
	threadID := strings.TrimSpace(record.ParentThreadID)
	turnID := strings.TrimSpace(record.ParentTurnID)
	if threadID == "" || turnID == "" {
		return nil
	}
	if blocked, err := service.deadLetterIfBlocked(record); blocked || err != nil {
		return err
	}
	identity, blocker, err := service.parentToolIdentity(record)
	if err != nil {
		return err
	}
	if blocker != "" {
		return service.markDeadLetter(record, blocker)
	}
	admitted, ok, err := AdmitAndPublishBackgroundCompletionLifecycleV1(
		service.delivery, service.lifecycle, threadID, turnID, identity.callID, identity.toolName, record,
		func(current domainjob.Record) []map[string]any {
			return BuildJobLifecycleEvents(JobLifecycleEventInput{
				ThreadID: threadID, TurnID: turnID, ItemID: identity.itemID, CallID: identity.callID,
				ToolName: identity.toolName, Record: current, Status: strings.TrimSpace(current.Status),
				ProgressStatus: toolcatalogapp.ToolProgressStatus(strings.TrimSpace(current.Status)),
				Message:        startupLifecycleMessageV1(current),
			})
		},
	)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	record = admitted
	status = strings.TrimSpace(record.Status)
	message = startupLifecycleMessageV1(record)
	service.delivery.MaybeAutoContinue(record, status, message)
	return nil
}

func AdmitAndPublishBackgroundCompletionLifecycleV1(
	service BackgroundDeliveryService,
	publisher BackgroundCompletionLifecyclePublisherV1,
	threadID, turnID, callID, toolName string,
	record domainjob.Record,
	buildEvents func(domainjob.Record) []map[string]any,
) (domainjob.Record, bool, error) {
	events := buildEvents(record)
	admitted, ok, err := AdmitBackgroundCompletionLifecycleV1(service, threadID, turnID, callID, toolName, events)
	if err != nil || !ok {
		return admitted, false, err
	}
	if publisher == nil {
		return admitted, false, errors.New("background completion lifecycle publisher is unavailable")
	}
	events = CanonicalBackgroundCompletionLifecycleEventsV1(buildEvents(admitted), admitted)
	if err := publisher.PublishBackgroundCompletionLifecycleExact(events); err != nil {
		return admitted, false, err
	}
	return admitted, true, nil
}

func BuildDurableSubagentLifecycleEventsV1(
	threadID, turnID, itemID, callID, toolName string,
	record domainjob.Record,
) []map[string]any {
	status := strings.TrimSpace(record.Status)
	return BuildSubagentProgressEvents(SubagentProgressEventInput{
		ThreadID: threadID, TurnID: turnID, ItemID: itemID, CallID: callID, ToolName: toolName,
		Record: record, Status: status, ProgressStatus: toolcatalogapp.ToolProgressStatus(status), Message: "",
	})
}

func BuildDurableJobLifecycleEventsV1(
	threadID, turnID, itemID, callID, toolName string,
	record domainjob.Record,
) []map[string]any {
	status := strings.TrimSpace(record.Status)
	return BuildJobLifecycleEvents(JobLifecycleEventInput{
		ThreadID: threadID, TurnID: turnID, ItemID: itemID, CallID: callID, ToolName: toolName,
		Record: record, Status: status, ProgressStatus: toolcatalogapp.ToolProgressStatus(status), Message: "",
	})
}

func RecordBackgroundSubagentLifecycleV1(
	service BackgroundDeliveryService,
	publisher BackgroundCompletionLifecyclePublisherV1,
	threadID, turnID, itemID, callID, toolName string,
	record domainjob.Record,
	status, message string,
) (BackgroundLifecycleRecordResultV1, error) {
	if service.Jobs == nil {
		return BackgroundLifecycleRecordResultV1{Record: record}, errors.New("background completion job store is unavailable")
	}
	if record.Background {
		latest, err := service.Jobs.LoadChildRun(record.ID)
		if err != nil {
			return BackgroundLifecycleRecordResultV1{Record: record}, err
		}
		record = latest
		if service.Threads == nil {
			return BackgroundLifecycleRecordResultV1{Record: record, Blocker: "runtime_store_unavailable"}, nil
		}
		thread, err := service.Threads.GetThread(strings.TrimSpace(record.ParentThreadID))
		if err != nil {
			return BackgroundLifecycleRecordResultV1{Record: record}, err
		}
		expectedItemID, expectedCallID, expectedToolName, ok := JobToolIdentity(thread, record)
		if !ok || threadID != strings.TrimSpace(record.ParentThreadID) ||
			turnID != strings.TrimSpace(record.ParentTurnID) || itemID != expectedItemID ||
			callID != expectedCallID || toolName != expectedToolName {
			return BackgroundLifecycleRecordResultV1{Record: record, Blocker: "parent_tool_identity_invalid"}, nil
		}
		threadID, turnID, itemID, callID, toolName = strings.TrimSpace(record.ParentThreadID),
			strings.TrimSpace(record.ParentTurnID), expectedItemID, expectedCallID, expectedToolName
	}
	if blocker := service.completionSecurityBlocker(record); blocker != "" {
		return BackgroundLifecycleRecordResultV1{Record: record, Blocker: blocker}, nil
	}
	if record.Background {
		buildEvents := func(current domainjob.Record) []map[string]any {
			return BuildDurableSubagentLifecycleEventsV1(threadID, turnID, itemID, callID, toolName, current)
		}
		if TaskJobTerminal(record) {
			admitted, ok, err := AdmitAndPublishBackgroundCompletionLifecycleV1(
				service, publisher, threadID, turnID, callID, toolName, record, buildEvents,
			)
			if err != nil || !ok {
				return BackgroundLifecycleRecordResultV1{Record: admitted}, err
			}
			service.MaybeAutoContinue(admitted, admitted.Status, "")
			return BackgroundLifecycleRecordResultV1{Record: admitted}, nil
		}
		for _, event := range buildEvents(record) {
			service.recordEvent(cloneMap(event), "subagent progress")
		}
		service.MaybeAutoContinue(record, record.Status, "")
		return BackgroundLifecycleRecordResultV1{Record: record}, nil
	}
	events := BuildSubagentProgressEvents(SubagentProgressEventInput{
		ThreadID: threadID, TurnID: turnID, ItemID: itemID, CallID: callID, ToolName: toolName,
		Record: record, Status: status, ProgressStatus: toolcatalogapp.ToolProgressStatus(status), Message: message,
	})
	for _, event := range events {
		service.recordEvent(cloneMap(event), "subagent progress")
	}
	service.MaybeAutoContinue(record, status, message)
	return BackgroundLifecycleRecordResultV1{Record: record}, nil
}

func RecordBackgroundJobLifecycleV1(
	service BackgroundDeliveryService,
	publisher BackgroundCompletionLifecyclePublisherV1,
	record domainjob.Record,
) (BackgroundLifecycleRecordResultV1, error) {
	if service.Jobs == nil || strings.TrimSpace(record.ID) == "" {
		return BackgroundLifecycleRecordResultV1{Record: record}, errors.New("background completion job store is unavailable")
	}
	latest, err := service.Jobs.LoadChildRun(record.ID)
	if err != nil {
		return BackgroundLifecycleRecordResultV1{Record: record}, err
	}
	record = latest
	if blocker := service.completionSecurityBlocker(record); blocker != "" {
		return BackgroundLifecycleRecordResultV1{Record: record, Blocker: blocker}, nil
	}
	threadID, turnID := strings.TrimSpace(record.ParentThreadID), strings.TrimSpace(record.ParentTurnID)
	if threadID == "" || turnID == "" {
		return BackgroundLifecycleRecordResultV1{Record: record}, nil
	}
	if service.Threads == nil {
		return BackgroundLifecycleRecordResultV1{Record: record, Blocker: "runtime_store_unavailable"}, nil
	}
	thread, err := service.Threads.GetThread(threadID)
	if err != nil {
		thread = nil
	}
	itemID, callID, toolName, ok := JobToolIdentity(thread, record)
	if !ok {
		return BackgroundLifecycleRecordResultV1{Record: record, Blocker: "parent_tool_identity_invalid"}, nil
	}
	buildEvents := func(current domainjob.Record) []map[string]any {
		return BuildDurableJobLifecycleEventsV1(threadID, turnID, itemID, callID, toolName, current)
	}
	if record.Background && TaskJobTerminal(record) {
		admitted, ok, err := AdmitAndPublishBackgroundCompletionLifecycleV1(
			service, publisher, threadID, turnID, callID, toolName, record, buildEvents,
		)
		if err != nil || !ok {
			return BackgroundLifecycleRecordResultV1{Record: admitted}, err
		}
		record = admitted
	} else {
		for _, event := range buildEvents(record) {
			service.recordEvent(cloneMap(event), "job lifecycle")
		}
	}
	service.MaybeAutoContinue(record, record.Status, "")
	return BackgroundLifecycleRecordResultV1{Record: record}, nil
}

// AdmitBackgroundCompletionLifecycleV1 is the shared durable admission owner
// for live and startup background lifecycle publication. It returns only a
// fresh durable child whose exact completion item has reached delivered state.
func AdmitBackgroundCompletionLifecycleV1(
	service BackgroundDeliveryService,
	threadID, turnID, callID, toolName string,
	events []map[string]any,
) (domainjob.Record, bool, error) {
	jobID := ""
	expectedItemID := ""
	for _, event := range events {
		candidateJobID := strings.TrimSpace(BackgroundJobIDFromEvent(event))
		if candidateJobID == "" {
			continue
		}
		candidateItemID := strings.TrimSpace(BackgroundJobCompletionItemID(turnID, event))
		if candidateItemID == "" || (jobID != "" && candidateJobID != jobID) ||
			(expectedItemID != "" && candidateItemID != expectedItemID) {
			return domainjob.Record{}, false, errors.New("background completion lifecycle identity is invalid")
		}
		jobID = candidateJobID
		expectedItemID = candidateItemID
	}
	if jobID == "" || expectedItemID == "" {
		return domainjob.Record{}, false, errors.New("background completion lifecycle event is missing")
	}
	if service.Jobs == nil {
		return domainjob.Record{}, false, errors.New("background completion job store is unavailable")
	}
	service.PersistCompletionItems(threadID, turnID, callID, toolName, events)
	latest, err := service.Jobs.LoadChildRun(jobID)
	if err != nil {
		return domainjob.Record{}, false, err
	}
	if !latest.Background || !TaskJobTerminal(latest) ||
		strings.TrimSpace(latest.CompletionDeliveryStatus) != "delivered" ||
		strings.TrimSpace(latest.CompletionDeliveryItemID) != expectedItemID ||
		strings.TrimSpace(latest.CompletionDeliveryID) == "" {
		return latest, false, nil
	}
	return latest, true, nil
}

// CanonicalBackgroundCompletionLifecycleEventsV1 projects delivery state only
// from the durable post-admission record, including security-bound children.
func CanonicalBackgroundCompletionLifecycleEventsV1(events []map[string]any, record domainjob.Record) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, source := range events {
		event := cloneMap(source)
		if backgroundLifecycleEventJobIDV1(event) != strings.TrimSpace(record.ID) {
			out = append(out, event)
			continue
		}
		if strings.TrimSpace(BackgroundJobIDFromEvent(event)) == strings.TrimSpace(record.ID) {
			details, _ := event["details"].(map[string]any)
			details = cloneMap(details)
			details["deliveryId"] = strings.TrimSpace(record.CompletionDeliveryID)
			details["deliveryItemId"] = strings.TrimSpace(record.CompletionDeliveryItemID)
			details["deliveryStatus"] = domainjob.PublicCompletionDeliveryStatusV1(record.CompletionDeliveryStatus)
			if reason := domainjob.NormalizeOperationalReasonV1(record.CompletionDeliveryReason); reason != "" {
				details["deliveryReason"] = reason
			} else {
				delete(details, "deliveryReason")
			}
			event["details"] = details
		}
		if timestamp := firstNonEmptyAnyString(record.CompletionDeliveryAt, record.FinishedAt, record.UpdatedAt); timestamp != "" {
			event["timestamp"] = timestamp
		}
		out = append(out, event)
	}
	return out
}

func backgroundLifecycleEventJobIDV1(event map[string]any) string {
	if jobID := strings.TrimSpace(BackgroundJobIDFromEvent(event)); jobID != "" {
		return jobID
	}
	child, _ := event["child"].(map[string]any)
	return strings.TrimSpace(firstNonEmptyAnyString(child["jobId"], child["childRunId"], child["childId"]))
}

func backgroundLifecycleEventIdentityV1(event map[string]any) string {
	jobID := backgroundLifecycleEventJobIDV1(event)
	if jobID == "" {
		return ""
	}
	base := []string{
		jobID, backgroundStringField(event, "kind"), backgroundStringField(event, "threadId"),
		backgroundStringField(event, "turnId"),
	}
	switch backgroundStringField(event, "kind") {
	case "pipeline_stage":
		base = append(base, backgroundStringField(event, "stage"))
	case "tool_progress":
		base = append(base, backgroundStringField(event, "itemId"), backgroundStringField(event, "callId"),
			backgroundStringField(event, "status"))
	default:
		return ""
	}
	return strings.Join(base, "\x00")
}

func backgroundLifecycleEventClaimsBundleMemberV1(
	event map[string]any,
	jobID string,
	expected map[string]map[string]any,
) bool {
	if backgroundLifecycleEventJobIDV1(event) != jobID {
		return false
	}
	kind := backgroundStringField(event, "kind")
	for _, candidate := range expected {
		if backgroundStringField(candidate, "kind") != kind {
			continue
		}
		switch kind {
		case "pipeline_stage":
			if backgroundStringField(event, "stage") == backgroundStringField(candidate, "stage") {
				return true
			}
		case "tool_progress":
			if backgroundStringField(event, "status") == backgroundStringField(candidate, "status") {
				return true
			}
		}
	}
	return false
}

func ReconcileBackgroundCompletionLifecycleExactV1(
	drafts []map[string]any,
	operations BackgroundCompletionLifecycleExactOperationsV1,
) error {
	if len(drafts) == 0 || operations.LoadEvents == nil || operations.PublicationReserved == nil ||
		operations.ReadThread == nil || operations.ProjectThread == nil || operations.NextSequence == nil ||
		operations.Persist == nil || operations.MissingThread == nil || operations.SetHighest == nil || operations.Publish == nil {
		return errors.New("background completion lifecycle event store is unavailable")
	}
	var completion map[string]any
	jobID := ""
	threadID := strings.TrimSpace(backgroundStringField(drafts[0], "threadId"))
	for _, draft := range drafts {
		if strings.TrimSpace(backgroundStringField(draft, "threadId")) != threadID {
			return errors.New("background completion lifecycle thread identity is invalid")
		}
		candidateJobID := strings.TrimSpace(BackgroundJobIDFromEvent(draft))
		if candidateJobID == "" {
			continue
		}
		if completion != nil || (jobID != "" && jobID != candidateJobID) {
			return errors.New("background completion lifecycle event identity is duplicated")
		}
		completion, jobID = draft, candidateJobID
	}
	if threadID == "" || completion == nil || jobID == "" {
		return errors.New("background completion lifecycle event identity is missing")
	}
	thread, err := operations.ReadThread(threadID)
	if err != nil || thread == nil {
		if err == nil {
			err = operations.MissingThread
		}
		return err
	}
	thread, err = operations.ProjectThread(threadID, thread)
	if err != nil {
		return err
	}
	if _, err := domainsecurity.ClassifyCaseSensitiveThread(thread); err != nil {
		return err
	}
	replay, err := operations.LoadEvents(threadID)
	if err != nil {
		return err
	}
	// A completed GENERAL parent can later acquire case lineage. Observe its
	// already committed ordinary bundle without re-publishing old child metadata.
	// The same exact matcher checks every member, payload and contiguous sequence.
	turnID := backgroundStringField(completion, "turnId")
	if parent, found := FindTurn(thread, turnID); found {
		frozen, parseErr := domainsecurity.ParseTurnSecurityContext(parent["securityContext"])
		general := parseErr == nil && domainsecurity.TurnSecurityContextIsGeneral(frozen) &&
			backgroundStringField(thread, "id") == threadID && backgroundStringField(parent, "threadId") == threadID &&
			frozen.ThreadID == threadID && frozen.TurnID == turnID
		for _, draft := range drafts {
			general = general && backgroundStringField(draft, "turnId") == turnID
		}
		if general {
			historical := map[string]any{"id": threadID, "turns": []any{map[string]any{
				"id": turnID, "threadId": threadID, "securityContext": parent["securityContext"],
			}}}
			if matched, matchErr := matchBackgroundCompletionLifecycleReplayV1(historical, drafts, replay, jobID); matchErr == nil && matched {
				return nil
			}
		}
	}
	matched, err := matchBackgroundCompletionLifecycleReplayV1(thread, drafts, replay, jobID)
	if err != nil || matched {
		return err
	}
	result, _, err := eventrecordingapp.RecordBatch(eventrecordingapp.Input{
		Drafts: drafts, Atomic: true, PublicationReserved: operations.PublicationReserved,
		ReadThread: operations.ReadThread, ProjectThread: operations.ProjectThread,
		BeforeRecord: operations.BeforeRecord, NextSequence: operations.NextSequence,
		Persist: operations.Persist, MissingThread: operations.MissingThread,
	})
	if err != nil {
		return err
	}
	operations.SetHighest(result.ThreadID, result.HighestSequence)
	operations.Publish(result.ThreadID, result.Events)
	return nil
}

func matchBackgroundCompletionLifecycleReplayV1(thread map[string]any, drafts, replay []map[string]any, jobID string) (bool, error) {
	expectedByIdentity := make(map[string]map[string]any, len(drafts))
	expectedOrder := make([]string, 0, len(drafts))
	for _, draft := range drafts {
		if backgroundLifecycleEventJobIDV1(draft) != jobID {
			return false, errors.New("background completion lifecycle bundle job identity is invalid")
		}
		projected, err := turnapp.SanitizeCaseEventPublication(thread, draft)
		if err != nil {
			return false, err
		}
		identity := backgroundLifecycleEventIdentityV1(projected)
		if identity == "" || expectedByIdentity[identity] != nil {
			return false, errors.New("background completion lifecycle bundle identity is invalid")
		}
		expectedByIdentity[identity] = cloneMap(projected)
		expectedOrder = append(expectedOrder, identity)
	}
	existingByIdentity := map[string]map[string]any{}
	for _, event := range replay {
		identity := backgroundLifecycleEventIdentityV1(event)
		if expectedByIdentity[identity] == nil {
			if backgroundLifecycleEventClaimsBundleMemberV1(event, jobID, expectedByIdentity) {
				return false, errors.New("background completion lifecycle replay identity conflicts")
			}
			continue
		}
		if existingByIdentity[identity] != nil {
			return false, errors.New("background completion lifecycle replay identity is duplicated")
		}
		existingByIdentity[identity] = event
	}
	if len(existingByIdentity) != 0 {
		if len(existingByIdentity) != len(expectedByIdentity) {
			return false, errors.New("background completion lifecycle replay bundle is incomplete")
		}
		previousSequence := -1
		for _, identity := range expectedOrder {
			expected, observed := cloneMap(expectedByIdentity[identity]), cloneMap(existingByIdentity[identity])
			sequence, ok := contracts.NumericSeq(observed["seq"])
			delete(expected, "seq")
			delete(observed, "seq")
			if !ok || previousSequence >= 0 && sequence != previousSequence+1 || !reflect.DeepEqual(expected, observed) {
				return false, errors.New("background completion lifecycle replay bundle conflicts")
			}
			previousSequence = sequence
		}
		return true, nil
	}
	return false, nil
}

func (service StartupRecoveryService) deadLetterIfBlocked(record domainjob.Record) (bool, error) {
	reason := service.delivery.completionSecurityBlocker(record)
	if reason == "" {
		return false, nil
	}
	return true, service.markDeadLetter(record, reason)
}

type startupParentToolIdentity struct {
	thread   map[string]any
	threadID string
	turnID   string
	itemID   string
	callID   string
	toolName string
}

func (service StartupRecoveryService) deadLetterIfParentInvalid(record domainjob.Record) (bool, error) {
	_, blocker, err := service.parentToolIdentity(record)
	if err != nil {
		return false, err
	}
	if blocker == "" {
		return false, nil
	}
	return true, service.markDeadLetter(record, blocker)
}

func (service StartupRecoveryService) parentToolIdentity(record domainjob.Record) (startupParentToolIdentity, string, error) {
	identity := startupParentToolIdentity{
		threadID: strings.TrimSpace(record.ParentThreadID),
		turnID:   strings.TrimSpace(record.ParentTurnID),
	}
	if identity.threadID == "" || identity.turnID == "" {
		return identity, "job_security_binding_missing", nil
	}
	if service.threads == nil {
		return identity, "", errors.New("recovered job parent store is unavailable")
	}
	thread, err := service.threads.GetThread(identity.threadID)
	if err != nil {
		return identity, "", err
	}
	if thread == nil {
		return identity, "parent_thread_missing", nil
	}
	identity.thread = thread
	if recoveredParentThreadArchived(thread) {
		return identity, "parent_thread_archived", nil
	}
	if _, ok := FindTurn(thread, identity.turnID); !ok {
		return identity, "parent_turn_missing", nil
	}
	identity.itemID, identity.callID, identity.toolName, _ = JobToolIdentity(thread, record)
	if identity.itemID == "" || identity.callID == "" || identity.toolName == "" {
		return identity, "parent_tool_identity_invalid", nil
	}
	return identity, "", nil
}

func (service StartupRecoveryService) securityBlocker(record domainjob.Record) string {
	if service.security == nil {
		return "job_security_authority_unavailable"
	}
	return strings.TrimSpace(service.security.Blocker(record))
}

func (service StartupRecoveryService) securityBlockerAt(record domainjob.Record, at time.Time) string {
	if service.security == nil {
		return "job_security_authority_unavailable"
	}
	if authority, ok := service.security.(interface {
		BlockerAt(domainjob.Record, time.Time) string
	}); ok {
		return strings.TrimSpace(authority.BlockerAt(record, at.UTC()))
	}
	return strings.TrimSpace(service.security.Blocker(record))
}

// RecoveredParentSettlementBlockerAt revalidates durable recovery authority
// against the latest thread while the store mutex is held.
func RecoveredParentSettlementBlockerAt(record domainjob.Record, thread map[string]any, at time.Time) string {
	if strings.TrimSpace(record.ParentThreadID) == "" || strings.TrimSpace(record.ParentTurnID) == "" || record.SecurityBinding == nil {
		return "job_security_binding_missing"
	}
	if thread == nil {
		return "parent_thread_missing"
	}
	if recoveredParentThreadArchived(thread) {
		return "parent_thread_archived"
	}
	if _, ok := FindTurn(thread, record.ParentTurnID); !ok {
		return "parent_turn_missing"
	}
	if thread["securityState"] == nil || !RecordMatchesThreadSecurity(record, thread) {
		return "parent_security_context_mismatch"
	}
	return durableParentGrantBlocker(record, thread, at.UTC())
}

func recoveredParentThreadArchived(thread map[string]any) bool {
	status, _ := thread["status"].(string)
	archived, _ := thread["archived"].(bool)
	return strings.TrimSpace(status) == "archived" || archived
}

func (service StartupRecoveryService) markDeadLetter(record domainjob.Record, reason string) error {
	if service.security == nil {
		return errors.New("job dead-letter authority is unavailable")
	}
	return service.security.MarkDeadLetter(record, reason)
}

func (service StartupRecoveryService) recordEvent(event map[string]any, context string) {
	if service.delivery.RecordEvent != nil {
		service.delivery.RecordEvent(event, context)
	}
}

func completionDeliverySettled(status string) bool {
	switch strings.TrimSpace(status) {
	case "delivered", "skipped", "dead_letter":
		return true
	default:
		return false
	}
}

func startupLifecycleMessageV1(record domainjob.Record) string {
	status := strings.TrimSpace(record.Status)
	if status == string(domainjob.StatusCompleted) {
		return "background_job_completed"
	}
	if code := domainjob.NormalizeFailureCode(record.FailureCode, status, true); code != "" {
		return code
	}
	return domainjob.FailureChildUnknown
}
