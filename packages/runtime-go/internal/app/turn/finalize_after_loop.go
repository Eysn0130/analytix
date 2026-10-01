package turn

import (
	"errors"
	"strings"
	"time"

	appusage "analytix.local/runtime-go/internal/app/usage"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

type CompletionStore interface {
	TurnStatus(threadID, turnID string) (string, error)
	FinishTurnIfActiveWithItemsAndFields(threadID, turnID, status string, items []map[string]any, fields map[string]any) (bool, string, error)
	RecordGeneralTerminalEventBundle(threadID, turnID string) ([]map[string]any, error)
}

type FinalizeAfterLoopInput struct {
	Timing          *PublicationTiming // Optional process-local observation; never authority.
	Store           CompletionStore
	SecurityContext domainsecurity.TurnSecurityContext
	ThreadID        string
	TurnID          string
	Model           string
	Telemetry       appusage.TerminalTelemetryV1
	Now             string
	UsageSource     string
	ChildRunID      string
	TerminalReason  string
	OrdinaryResult  *domainordinaryresult.ResultSlotV1
}

type CommitCompletionInput struct {
	Store           CompletionStore
	SecurityContext domainsecurity.TurnSecurityContext
	ThreadID        string
	TurnID          string
	Model           string
	CreatedAt       string
	FinishedAt      string
	Telemetry       appusage.TerminalTelemetryV1
	UsageSource     string
	ChildRunID      string
	TerminalReason  string
	OrdinaryResult  *domainordinaryresult.ResultSlotV1
}

type CommitCompletionResult struct {
	Timing           PublicationTiming `json:"-"`
	CompletionRecord CompletionRecord
	PublishedText    string
	CASBinding       domainturnterminal.GeneralTerminalCASBindingV1
	Publication      domainturnterminal.GeneralTerminalPublicationCommitV1
	Changed          bool
	Status           string
}

// CommitCompletedTurn atomically publishes either a typed ordinary result or
// the legacy host-fixed boundary and makes the turn terminal before publishing
// any replay event. Raw provider prose is intentionally absent from the input;
// only ResultSlotV1 may cross the terminal CAS, history, SSE, or export path.
func CommitCompletedTurn(input CommitCompletionInput) (CommitCompletionResult, error) {
	if input.Store == nil {
		return CommitCompletionResult{}, errors.New("completion store is unavailable")
	}
	terminalReason := strings.TrimSpace(input.TerminalReason)
	if terminalReason == "" {
		terminalReason = "success"
	}
	if !GeneralTerminalReasonAllowsCandidateV1(terminalReason) {
		return CommitCompletionResult{}, errors.New("general completion terminal reason cannot publish a candidate")
	}
	if domainsecurity.ValidateTurnSecurityContextForExecution(input.SecurityContext) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(input.SecurityContext) ||
		input.SecurityContext.ThreadID != strings.TrimSpace(input.ThreadID) || input.SecurityContext.TurnID != strings.TrimSpace(input.TurnID) {
		return CommitCompletionResult{}, errors.New("general completion security context is invalid")
	}
	finishedAt := strings.TrimSpace(input.FinishedAt)
	if finishedAt == "" {
		finishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	createdAt := strings.TrimSpace(input.CreatedAt)
	if createdAt == "" {
		createdAt = finishedAt
	}
	publishedText := GeneralProviderFinalQuarantinedText
	if input.OrdinaryResult != nil {
		if domainordinaryresult.ValidateResultSlotV1(*input.OrdinaryResult) != nil {
			return CommitCompletionResult{}, errors.New("general completion ordinary result is invalid")
		}
		publishedText = input.OrdinaryResult.Text
	}
	record := BuildCompletionRecord(CompletionRecordInput{
		ThreadID: input.ThreadID, TurnID: input.TurnID, Model: input.Model, AssistantText: publishedText,
		CreatedAt: createdAt, FinishedAt: finishedAt, Usage: input.Telemetry.PublicUsageMap(),
		CacheDiagnostics: input.Telemetry.PublicCacheDiagnosticsMap(), UsageSource: input.UsageSource, ChildRunID: input.ChildRunID,
	})
	if input.OrdinaryResult != nil {
		record.AssistantItem["ordinaryResult"] = domainordinaryresult.ResultSlotV1Map(*input.OrdinaryResult)
	}
	binding, err := domainturnterminal.NewGeneralTerminalCASBindingForOutcomeV1(
		input.SecurityContext, terminalReason, "completed", publishedText,
	)
	if err != nil {
		return CommitCompletionResult{}, err
	}
	bindingRecord := domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	record.AssistantItem["generalTerminalCASBinding"] = bindingRecord
	record.ItemCompletedEvent = AssistantItemCompletedEvent(record.AssistantItem)
	record.ItemCompletedEvent["timestamp"] = finishedAt
	record.UsageEvent["timestamp"] = finishedAt
	record.TurnCompletedEvent["timestamp"] = finishedAt
	record.TurnCompletedEvent["generalTerminalCASBindingDigest"] = binding.BindingDigest
	record.TurnCompletedEvent["terminalReason"] = binding.TerminalReason
	items := []map[string]any{record.AssistantItem}
	publication, err := BuildGeneralTerminalPublicationCommitV1(
		input.SecurityContext, binding, record, finishedAt, len(items) > 0,
	)
	if err != nil {
		return CommitCompletionResult{}, err
	}
	timing := PublicationTiming{ProjectionReadyAt: time.Now()}
	changed, status, err := input.Store.FinishTurnIfActiveWithItemsAndFields(input.ThreadID, input.TurnID, "completed", items, map[string]any{
		"generalTerminalCASBinding":  bindingRecord,
		"generalTerminalPublication": domainturnterminal.GeneralTerminalPublicationCommitV1Map(publication),
	})
	if err == nil && changed {
		timing.CommittedAt = time.Now()
	}
	result := CommitCompletionResult{
		Timing:           timing,
		CompletionRecord: record, PublishedText: publishedText, CASBinding: binding,
		Publication: publication, Changed: changed, Status: status,
	}
	if err != nil || !changed {
		return result, WithGeneralTerminalDetailV1(err, GeneralTerminalDetailFinishV1)
	}
	if _, err := input.Store.RecordGeneralTerminalEventBundle(input.ThreadID, input.TurnID); err != nil {
		return result, WithGeneralTerminalDetailV1(err, GeneralTerminalDetailOutboxV1)
	}
	result.Timing.DeliverableAt = time.Now()
	return result, nil
}

func FinalizeAfterLoop(input FinalizeAfterLoopInput) error {
	status, err := input.Store.TurnStatus(input.ThreadID, input.TurnID)
	if err != nil {
		return err
	}
	if IsTerminalStatus(status) {
		return nil
	}
	now := strings.TrimSpace(input.Now)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339Nano)
	}
	result, err := CommitCompletedTurn(CommitCompletionInput{
		Store:           input.Store,
		SecurityContext: input.SecurityContext,
		ThreadID:        input.ThreadID,
		TurnID:          input.TurnID,
		Model:           input.Model,
		CreatedAt:       now,
		FinishedAt:      now,
		Telemetry:       input.Telemetry,
		UsageSource:     input.UsageSource,
		ChildRunID:      input.ChildRunID,
		TerminalReason:  input.TerminalReason,
		OrdinaryResult:  input.OrdinaryResult,
	})
	if err != nil {
		return err
	}
	if input.Timing != nil && result.Changed {
		*input.Timing = result.Timing
	}
	return nil
}

func GeneralTerminalReasonAllowsCandidateV1(reason string) bool {
	return domainterminal.CandidateAllowedV1(reason)
}
