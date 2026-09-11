package turn

import (
	"errors"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

type FailureStore interface {
	FinishTurnIfActiveWithItemsAndFields(threadID, turnID, status string, items []map[string]any, fields map[string]any) (bool, string, error)
	RecordGeneralTerminalEventBundle(threadID, turnID string) ([]map[string]any, error)
}

type PersistFailureInput struct {
	Store            FailureStore
	SecurityContext  domainsecurity.TurnSecurityContext
	TerminalReason   string
	ThreadID         string
	TurnID           string
	Model            string
	Failure          domainfailure.Record
	FinishedAt       string
	Events           []map[string]any
	Result           domainmodel.Result
	CacheDiagnostics map[string]any
	UsageSource      string
	ChildRunID       string
	Interrupt        *GeneralTerminalInterruptMetadata
}

type GeneralTerminalInterruptMetadata struct {
	Discard               bool
	Cancelled             bool
	CancelledPendingGates int
}

type PersistFailureResult struct {
	Changed     bool
	Status      string
	Publication domainturnterminal.GeneralTerminalPublicationCommitV1
}

func PersistFailure(input PersistFailureInput) error {
	_, err := CommitGeneralFailureTerminal(input)
	return err
}

func CommitGeneralFailureTerminal(input PersistFailureInput) (PersistFailureResult, error) {
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := strings.TrimSpace(input.TurnID)
	terminalReason := strings.TrimSpace(input.TerminalReason)
	terminalStatus, ok := domainturnterminal.GeneralTerminalStatusForReasonV1(terminalReason)
	hostFailure, failureOK := domainterminal.GeneralFailureRecordForCauseV1(terminalReason, input.Failure)
	if input.Store == nil || domainsecurity.ValidateTurnSecurityContextForExecution(input.SecurityContext) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(input.SecurityContext) || input.SecurityContext.ThreadID != threadID ||
		input.SecurityContext.TurnID != turnID || !ok || !failureOK {
		return PersistFailureResult{}, errors.New("general failure terminal authority is invalid")
	}
	if (terminalReason == "cancel") != (input.Interrupt != nil) ||
		input.Interrupt != nil && (input.Interrupt.CancelledPendingGates < 0 ||
			input.Interrupt.CancelledPendingGates > 0 && !input.Interrupt.Cancelled) {
		return PersistFailureResult{}, errors.New("general failure interrupt metadata is invalid")
	}
	failure := BuildFailureRecord(FailureRecordInput{
		ThreadID:         threadID,
		TurnID:           turnID,
		Failure:          hostFailure,
		FinishedAt:       input.FinishedAt,
		Events:           input.Events,
		Model:            input.Model,
		Result:           input.Result,
		CacheDiagnostics: input.CacheDiagnostics,
		UsageSource:      input.UsageSource,
		ChildRunID:       input.ChildRunID,
	})
	finishedAt := strings.TrimSpace(input.FinishedAt)
	terminalItem := contracts.CloneMap(failure.Items[0])
	if terminalStatus == "completed" {
		terminalItem = BuildAssistantTextItem(AssistantTextItemInput{
			ThreadID: threadID, TurnID: turnID, ItemID: "item_" + turnID + "_boundary",
			Status: "completed", Text: GeneralProviderFinalQuarantinedText,
			CreatedAt: finishedAt, FinishedAt: finishedAt,
		})
	} else {
		terminalItem["status"] = terminalStatus
	}
	terminalText, textOK := domainturnterminal.GeneralTerminalItemTextV1(terminalItem)
	if !textOK {
		return PersistFailureResult{}, errors.New("general failure terminal item is invalid")
	}
	binding, err := domainturnterminal.NewGeneralTerminalCASBindingForOutcomeV1(
		input.SecurityContext, terminalReason, terminalStatus, terminalText,
	)
	if err != nil {
		return PersistFailureResult{}, err
	}
	bindingRecord := domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	terminalItem["generalTerminalCASBinding"] = bindingRecord
	itemEvent := itemCompletedEvent(terminalItem)
	itemEvent["timestamp"] = finishedAt
	usageEvent := contracts.CloneMap(failure.UsageEvent)
	usageEvent["timestamp"] = finishedAt
	usageEvent["usageFinalStatus"] = terminalStatus
	terminalEvent := contracts.CloneMap(failure.TurnFailedEvent)
	terminalKind, _ := domainturnterminal.GeneralTerminalLifecycleEventKindV1(terminalStatus)
	terminalEvent["kind"] = terminalKind
	terminalEvent["status"] = terminalStatus
	terminalEvent["itemId"] = stringField(terminalItem, "id")
	terminalEvent["timestamp"] = finishedAt
	terminalEvent["terminalReason"] = terminalReason
	terminalEvent["generalTerminalCASBindingDigest"] = binding.BindingDigest
	if input.Interrupt != nil {
		terminalEvent["discard"] = input.Interrupt.Discard
		terminalEvent["cancelled"] = input.Interrupt.Cancelled
		terminalEvent["cancelledPendingGates"] = input.Interrupt.CancelledPendingGates
	}
	publication, err := BuildGeneralTerminalPublicationCommitForEventsV1(
		input.SecurityContext, binding, itemEvent, usageEvent, terminalEvent, finishedAt, true,
	)
	if err != nil {
		return PersistFailureResult{}, err
	}
	fields := map[string]any{
		"generalTerminalCASBinding":  bindingRecord,
		"generalTerminalPublication": domainturnterminal.GeneralTerminalPublicationCommitV1Map(publication),
	}
	if input.Interrupt != nil {
		fields["discard"] = input.Interrupt.Discard
		fields["cancelled"] = input.Interrupt.Cancelled
		fields["cancelledPendingGates"] = input.Interrupt.CancelledPendingGates
	}
	changed, status, finishErr := input.Store.FinishTurnIfActiveWithItemsAndFields(
		threadID, turnID, terminalStatus, []map[string]any{terminalItem}, fields,
	)
	result := PersistFailureResult{Changed: changed, Status: status, Publication: publication}
	if finishErr != nil {
		return result, WithGeneralTerminalDetailV1(finishErr, GeneralTerminalDetailFinishV1)
	}
	if _, err := input.Store.RecordGeneralTerminalEventBundle(threadID, turnID); err != nil {
		return result, WithGeneralTerminalDetailV1(err, GeneralTerminalDetailOutboxV1)
	}
	return result, nil
}
