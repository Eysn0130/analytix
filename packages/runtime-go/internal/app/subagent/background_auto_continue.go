package subagent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
	appturn "analytix.local/runtime-go/internal/app/turn"
	appusage "analytix.local/runtime-go/internal/app/usage"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const BackgroundAutoContinuePromptV1 = controlapp.InternalAutoContinuePromptV1

type BackgroundAutoContinueCompletionAuthorityV1 interface {
	Rehydrate(context.Context, domainjob.Record) (VerifiedChildCompletion, error)
}

type BackgroundAutoContinueReservationStoreV1 interface {
	AllRecords() []domainjob.Record
	ReserveBackgroundAutoContinueV1(
		string,
		string,
		func(domainjob.Record, []domainjob.Record) string,
	) (domainjob.Record, bool, error)
}

type BackgroundAutoContinueTurnStateV1 string

const (
	BackgroundAutoContinueTurnMissingV1  BackgroundAutoContinueTurnStateV1 = "missing"
	BackgroundAutoContinueTurnExactV1    BackgroundAutoContinueTurnStateV1 = "exact"
	BackgroundAutoContinueTurnConflictV1 BackgroundAutoContinueTurnStateV1 = "conflict"
)

type BackgroundAutoContinueStarterV1 interface {
	NewTurnID(domainjob.Record) (string, error)
	StartReservedTurn(context.Context, domainjob.Record) (string, error)
	InspectReservedTurn(domainjob.Record) (BackgroundAutoContinueTurnStateV1, error)
}

type BackgroundAutoContinueRuntimeStarterV1 struct {
	Allocate   func() (string, int)
	Send       func(context.Context, controlapp.StartTurnRequest) (map[string]any, error)
	LoadThread func(string) (map[string]any, error)
}

func (starter BackgroundAutoContinueRuntimeStarterV1) NewTurnID(domainjob.Record) (string, error) {
	if starter.Allocate == nil {
		return "", errors.New("background auto-continue turn allocator is unavailable")
	}
	turnID, _ := starter.Allocate()
	return turnID, nil
}

func (starter BackgroundAutoContinueRuntimeStarterV1) StartReservedTurn(ctx context.Context, record domainjob.Record) (string, error) {
	if ctx == nil || starter.Send == nil {
		return "", errors.New("background auto-continue starter is unavailable")
	}
	response, err := starter.Send(ctx, controlapp.StartTurnRequest{
		ThreadID: record.ParentThreadID, Prompt: BackgroundAutoContinuePromptV1, Async: true,
		InternalUsageSource: appusage.SourceTurn, InternalTurnID: record.AutoContinueTurnID,
		InternalAutoContinueJobID: record.ID, InternalAutoContinueParentTurnID: record.ParentTurnID,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(mapString(response, "turnId")), nil
}

func (starter BackgroundAutoContinueRuntimeStarterV1) InspectReservedTurn(record domainjob.Record) (BackgroundAutoContinueTurnStateV1, error) {
	if starter.LoadThread == nil {
		return BackgroundAutoContinueTurnConflictV1, errors.New("background auto-continue turn store is unavailable")
	}
	thread, err := starter.LoadThread(record.ParentThreadID)
	if err != nil {
		return BackgroundAutoContinueTurnConflictV1, err
	}
	return InspectBackgroundAutoContinueTurnV1(record, thread), nil
}

func (service BackgroundDeliveryService) ReservedAutoContinueTurnIdentityV1(
	request controlapp.StartTurnRequest,
	threadID string,
) (string, int, error) {
	turnID := strings.TrimSpace(request.InternalTurnID)
	jobID := strings.TrimSpace(request.InternalAutoContinueJobID)
	parentTurnID := strings.TrimSpace(request.InternalAutoContinueParentTurnID)
	if turnID == "" {
		if jobID != "" || parentTurnID != "" {
			return "", 0, errors.New("background auto-continue reservation identity is incomplete")
		}
		return "", 0, nil
	}
	turnNumber, ok := appturn.SequenceID(turnID)
	if carrierErr := controlapp.ValidateInternalReservedTurnRequestV1(
		request, BackgroundAutoContinuePromptV1, appusage.SourceTurn,
	); !ok || carrierErr != nil || service.Jobs == nil || service.AutoContinueStarter == nil ||
		threadID == "" || request.ThreadID != threadID || jobID == "" || parentTurnID == "" {
		return "", 0, errors.New("background auto-continue reservation identity is invalid")
	}
	record, err := service.Jobs.LoadChildRun(jobID)
	if err != nil || !BackgroundAutoContinueReservationMatchesV1(record, threadID, parentTurnID, turnID) {
		return "", 0, errors.New("background auto-continue durable reservation is invalid")
	}
	state, err := service.AutoContinueStarter.InspectReservedTurn(record)
	if err != nil || state != BackgroundAutoContinueTurnMissingV1 {
		return "", 0, errors.New("background auto-continue reserved turn already exists or conflicts")
	}
	return turnID, turnNumber, nil
}

func (service BackgroundDeliveryService) ValidateReservedAutoContinueFrozenV1(
	request controlapp.StartTurnRequest,
	thread map[string]any,
	turnID string,
) error {
	jobID := strings.TrimSpace(request.InternalAutoContinueJobID)
	if strings.TrimSpace(request.InternalTurnID) == "" {
		if jobID != "" || strings.TrimSpace(request.InternalAutoContinueParentTurnID) != "" {
			return errors.New("background auto-continue frozen reservation is incomplete")
		}
		return nil
	}
	reservations, ok := service.Jobs.(BackgroundAutoContinueReservationStoreV1)
	if !ok || jobID == "" {
		return errors.New("background auto-continue frozen reservation is unavailable")
	}
	record, err := service.Jobs.LoadChildRun(jobID)
	if err != nil || !BackgroundAutoContinueReservationMatchesV1(
		record, request.ThreadID, request.InternalAutoContinueParentTurnID, turnID,
	) {
		return errors.New("background auto-continue frozen reservation changed")
	}
	if reason := service.AutoContinueGate(record); reason != "" {
		return fmt.Errorf("background auto-continue authority rejected: %s", reason)
	}
	if reason := BackgroundAutoContinueGate(record, thread, reservations.AllRecords()); reason != "" {
		return fmt.Errorf("background auto-continue parent gate rejected: %s", reason)
	}
	if InspectBackgroundAutoContinueTurnV1(record, thread) != BackgroundAutoContinueTurnMissingV1 {
		return errors.New("background auto-continue reserved turn conflicts before append")
	}
	return nil
}

func BackgroundAutoContinueReservationMatchesV1(record domainjob.Record, threadID, parentTurnID, turnID string) bool {
	return record.Background && record.AutoContinueParent && strings.TrimSpace(record.Status) == string(domainjob.StatusCompleted) &&
		strings.TrimSpace(record.CompletionDeliveryStatus) == "delivered" && strings.TrimSpace(record.CompletionDeliveryID) != "" &&
		strings.TrimSpace(record.CompletionDeliveryItemID) != "" && strings.TrimSpace(record.AutoContinueStatus) == "starting" &&
		strings.TrimSpace(record.AutoContinueTurnID) == strings.TrimSpace(turnID) && strings.TrimSpace(record.ParentThreadID) == strings.TrimSpace(threadID) &&
		strings.TrimSpace(record.ParentTurnID) == strings.TrimSpace(parentTurnID)
}

func (service BackgroundDeliveryService) autoContinueAuthorityGate(record domainjob.Record) string {
	if record.SecurityBinding == nil {
		return "job_security_binding_missing"
	}
	if reason := service.securityBlocker(record); reason != "" {
		return reason
	}
	caseBound := strings.TrimSpace(record.SecurityBinding.ParentCaseID) != domainsecurity.UnboundCaseID
	if !caseBound {
		if record.ChildCompletionReceipt != nil {
			return "child_completion_receipt_unexpected"
		}
		return ""
	}
	if record.ChildCompletionReceipt == nil {
		return "child_completion_receipt_required"
	}
	if service.CompletionAuthority == nil {
		return "child_completion_receipt_invalid"
	}
	verified, err := service.CompletionAuthority.Rehydrate(context.Background(), record)
	if err != nil {
		return "child_completion_receipt_invalid"
	}
	receipt, err := verified.ReceiptForPersistence()
	if err != nil || receipt == nil || !receipt.CanContinueParent {
		return "child_completion_not_continuable"
	}
	return ""
}

func (service BackgroundDeliveryService) maybeAutoContinue(record domainjob.Record, _ string) error {
	if service.Jobs == nil || service.Threads == nil || !record.Background || !record.AutoContinueParent {
		return nil
	}
	latest, err := service.Jobs.LoadChildRun(record.ID)
	if err != nil {
		return err
	}
	record = latest
	if strings.TrimSpace(record.AutoContinueStatus) != "" {
		return service.RepairAutoContinueNotice(record)
	}
	terminalStatus := strings.TrimSpace(record.Status)
	if !TaskJobTerminal(record) && !BackgroundAutoContinueTerminalStatus(terminalStatus) {
		return nil
	}
	if terminalStatus != string(domainjob.StatusCompleted) || strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) {
		return service.skipAutoContinue(record, "job_not_completed")
	}
	if record.LateCompletionSuppressed {
		return service.skipAutoContinue(record, backgroundFirstNonEmpty(record.LateCompletionReason, "late_completion_suppressed"))
	}
	if strings.TrimSpace(record.CompletionDeliveryStatus) != "delivered" ||
		strings.TrimSpace(record.CompletionDeliveryID) == "" || strings.TrimSpace(record.CompletionDeliveryItemID) == "" {
		return service.skipAutoContinue(record, "completion_delivery_not_delivered")
	}
	if reason := service.AutoContinueGate(record); reason != "" {
		return service.skipAutoContinue(record, reason)
	}
	thread, err := service.Threads.GetThread(record.ParentThreadID)
	if err != nil || thread == nil {
		return service.skipAutoContinue(record, "parent_thread_missing")
	}
	reservations, ok := service.Jobs.(BackgroundAutoContinueReservationStoreV1)
	if !ok {
		return service.skipAutoContinue(record, "auto_continue_reservation_unavailable")
	}
	if reason := BackgroundAutoContinueGate(record, thread, reservations.AllRecords()); reason != "" {
		return service.skipAutoContinue(record, reason)
	}
	if service.AutoContinueStarter == nil {
		return service.skipAutoContinue(record, "auto_continue_starter_unavailable")
	}
	turnID, err := service.AutoContinueStarter.NewTurnID(record)
	if err != nil || strings.TrimSpace(turnID) == "" {
		return service.skipAutoContinue(record, "auto_continue_starter_unavailable")
	}
	reserved, won, err := reservations.ReserveBackgroundAutoContinueV1(
		record.ID,
		turnID,
		func(current domainjob.Record, records []domainjob.Record) string {
			if !current.Background || !current.AutoContinueParent ||
				strings.TrimSpace(current.Status) != string(domainjob.StatusCompleted) ||
				strings.TrimSpace(current.CompletionDeliveryStatus) != "delivered" {
				return "job_auto_continue_state_changed"
			}
			return BackgroundAutoContinueSiblingGate(current, records)
		},
	)
	if err != nil {
		return err
	}
	if !won {
		return nil
	}
	return service.startReservedAutoContinue(reserved)
}

func (service BackgroundDeliveryService) skipAutoContinue(record domainjob.Record, reason string) error {
	updated := service.updateAutoContinue(record, "skipped", "", reason, "", false)
	if strings.TrimSpace(updated.AutoContinueStatus) != "skipped" {
		return errors.New("persist background auto-continue skip failed")
	}
	return nil
}

func (service BackgroundDeliveryService) startReservedAutoContinue(record domainjob.Record) error {
	if service.AutoContinueStarter == nil || strings.TrimSpace(record.AutoContinueStatus) != "starting" ||
		strings.TrimSpace(record.AutoContinueTurnID) == "" {
		return errors.New("background auto-continue start authority is unavailable")
	}
	turnID, err := service.AutoContinueStarter.StartReservedTurn(context.Background(), record)
	if err != nil || strings.TrimSpace(turnID) != strings.TrimSpace(record.AutoContinueTurnID) {
		return service.reconcileReservedAutoContinueStart(record)
	}
	return service.finishStartedAutoContinue(record)
}

func (service BackgroundDeliveryService) reconcileReservedAutoContinueStart(record domainjob.Record) error {
	state, err := service.AutoContinueStarter.InspectReservedTurn(record)
	if err != nil {
		return errors.New("background auto-continue reserved turn inspection is unavailable")
	}
	switch state {
	case BackgroundAutoContinueTurnExactV1:
		return service.finishStartedAutoContinue(record)
	case BackgroundAutoContinueTurnConflictV1:
		_ = service.updateAutoContinue(record, "failed", record.AutoContinueTurnID, "auto_continue_turn_conflict", "", false)
	case BackgroundAutoContinueTurnMissingV1:
		_ = service.updateAutoContinue(record, "failed", record.AutoContinueTurnID, "auto_continue_start_failed", "", false)
	default:
		return errors.New("background auto-continue reserved turn inspection is invalid")
	}
	return errors.New("background auto-continue reserved turn did not start exactly")
}

func (service BackgroundDeliveryService) finishStartedAutoContinue(record domainjob.Record) error {
	updated := service.updateAutoContinue(record, "started", record.AutoContinueTurnID, "", "", false)
	if strings.TrimSpace(updated.AutoContinueStatus) != "started" {
		return errors.New("persist background auto-continue started status failed")
	}
	_, err := service.persistAutoContinueItemExact(updated)
	return err
}

func (service BackgroundDeliveryService) RecoverAutoContinue(record domainjob.Record) error {
	if service.Jobs == nil || !record.Background || !record.AutoContinueParent {
		return nil
	}
	latest, err := service.Jobs.LoadChildRun(record.ID)
	if err != nil {
		return err
	}
	record = latest
	switch strings.TrimSpace(record.AutoContinueStatus) {
	case "":
		return service.maybeAutoContinue(record, record.Status)
	case "starting":
		if service.AutoContinueStarter == nil {
			return service.settleRecoveredAutoContinue(record, "failed", "auto_continue_starter_unavailable")
		}
		state, inspectErr := service.AutoContinueStarter.InspectReservedTurn(record)
		if inspectErr != nil {
			return inspectErr
		}
		switch state {
		case BackgroundAutoContinueTurnExactV1:
			return service.settleRecoveredAutoContinue(record, "started", "")
		case BackgroundAutoContinueTurnConflictV1:
			return service.settleRecoveredAutoContinue(record, "failed", "auto_continue_turn_conflict")
		case BackgroundAutoContinueTurnMissingV1:
			if reason := service.AutoContinueGate(record); reason != "" {
				return service.settleRecoveredAutoContinue(record, "skipped", reason)
			}
			thread, loadErr := service.Threads.GetThread(record.ParentThreadID)
			reservations, ok := service.Jobs.(BackgroundAutoContinueReservationStoreV1)
			if loadErr != nil || thread == nil {
				return service.settleRecoveredAutoContinue(record, "skipped", "parent_thread_missing")
			}
			if !ok {
				return service.settleRecoveredAutoContinue(record, "failed", "auto_continue_reservation_unavailable")
			}
			if reason := BackgroundAutoContinueGate(record, thread, reservations.AllRecords()); reason != "" {
				return service.settleRecoveredAutoContinue(record, "skipped", reason)
			}
			return service.startReservedAutoContinue(record)
		default:
			return service.settleRecoveredAutoContinue(record, "failed", "auto_continue_turn_conflict")
		}
	default:
		return service.RepairAutoContinueNotice(record)
	}
}

func (service BackgroundDeliveryService) settleRecoveredAutoContinue(record domainjob.Record, status, reason string) error {
	updated := service.updateAutoContinue(record, status, record.AutoContinueTurnID, reason, "", false)
	if strings.TrimSpace(updated.AutoContinueStatus) != strings.TrimSpace(status) {
		return errors.New("persist recovered background auto-continue status failed")
	}
	return service.RepairAutoContinueNotice(updated)
}

func InspectBackgroundAutoContinueTurnV1(record domainjob.Record, thread map[string]any) BackgroundAutoContinueTurnStateV1 {
	threadID := strings.TrimSpace(record.ParentThreadID)
	parentTurnID := strings.TrimSpace(record.ParentTurnID)
	turnID := strings.TrimSpace(record.AutoContinueTurnID)
	if thread == nil || threadID == "" || parentTurnID == "" || turnID == "" || mapString(thread, "id") != threadID {
		return BackgroundAutoContinueTurnConflictV1
	}
	turns := autoContinueList(thread["turns"])
	parentIndex := -1
	continuationIndex := -1
	var continuation map[string]any
	for index, raw := range turns {
		turn, _ := raw.(map[string]any)
		switch mapString(turn, "id") {
		case parentTurnID:
			parentIndex = index
		case turnID:
			continuationIndex = index
			continuation = turn
		}
	}
	if continuationIndex < 0 {
		if parentIndex < 0 || parentIndex != len(turns)-1 {
			return BackgroundAutoContinueTurnConflictV1
		}
		return BackgroundAutoContinueTurnMissingV1
	}
	if parentIndex < 0 || continuationIndex != parentIndex+1 || continuation == nil ||
		mapString(continuation, "threadId") != threadID || mapString(continuation, "prompt") != BackgroundAutoContinuePromptV1 {
		return BackgroundAutoContinueTurnConflictV1
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(continuation["securityContext"])
	if err != nil || securityContext.ThreadID != threadID || securityContext.TurnID != turnID {
		return BackgroundAutoContinueTurnConflictV1
	}
	wantedItemID := "item_" + turnID + "_user"
	for _, raw := range autoContinueList(continuation["items"]) {
		item, _ := raw.(map[string]any)
		if mapString(item, "id") != wantedItemID {
			continue
		}
		if mapString(item, "threadId") == threadID && mapString(item, "turnId") == turnID &&
			mapString(item, "kind") == "user_message" && mapString(item, "role") == "user" &&
			mapString(item, "text") == BackgroundAutoContinuePromptV1 {
			return BackgroundAutoContinueTurnExactV1
		}
		return BackgroundAutoContinueTurnConflictV1
	}
	return BackgroundAutoContinueTurnConflictV1
}
