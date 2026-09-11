package subagent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	usageapp "analytix.local/runtime-go/internal/app/usage"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/ports"
)

type CompletionDriver interface {
	StartTurn(context.Context, controlapp.StartTurnRequest) (map[string]any, error)
	UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error)
	LoadChildRun(id string) (domainjob.Record, error)
	TurnCompletion(threadID string, turnID string) (string, int, error)
	UsageSnapshot(threadID string) (map[string]any, error)
	CacheDiagnostics(threadID string, turnID string) (map[string]any, error)
	RecordProgress(record domainjob.Record, status string, message string)
}

// ChildTurnStartAuthority is the shared linearization boundary for the
// pending-steer queue, durable child-run claim and first child effect.
// Cancellation closes this authority even when it races after the durable
// claim has succeeded.
type ChildTurnStartAuthority interface {
	StartIfActive(context.Context, func() error) error
}

type ChildCompletionIssuer interface {
	Issue(context.Context, domainjob.Record, string) (VerifiedChildCompletion, error)
	Rehydrate(context.Context, domainjob.Record) (VerifiedChildCompletion, error)
}

type ForegroundHandoffIssuer interface {
	Prepare(context.Context, domainjob.Record, string) (PreparedForegroundHandoff, error)
	Consume(context.Context, domainjob.Record, PreparedForegroundHandoff) (VerifiedForegroundHandoff, error)
	Delete(string)
}

type CompleteTaskInput struct {
	Request              TaskRequest
	Record               domainjob.Record
	ProviderID           string
	Model                string
	Effort               string
	ParentMaxModelSteps  *int
	ParentApprovalPolicy string
	ParentSandboxMode    string
	ParentSubagentDepth  int
	ToolScope            []string
	WorktreeManager      ports.WorktreeManager
	Driver               CompletionDriver
	AuthorizeStart       func(domainjob.Record) (domainjob.Record, error)
	StartAuthority       ChildTurnStartAuthority
	CompletionAuthority  ChildCompletionIssuer
	ForegroundAuthority  ForegroundHandoffIssuer
}

type CompleteTaskResult struct {
	Output  map[string]any
	Record  domainjob.Record
	IsError bool
	Err     error
}

func CompleteTask(ctx context.Context, input CompleteTaskInput) CompleteTaskResult {
	record := input.Record
	if foregroundSubmitOnlyRecord(record) && input.ForegroundAuthority != nil {
		// The live submission body is parent-call scoped. Every terminal path,
		// including failures after the child submitted, destroys that capability.
		defer input.ForegroundAuthority.Delete(record.ID)
	}
	if err := domainmodel.ValidateReasoningEffortV1(input.Effort); err != nil {
		return CompleteTaskResult{Output: RunOutput(record, "", nil, err), Record: record, IsError: true}
	}
	if err := domainmodel.ValidateReasoningEffortV1(record.Effort); err != nil || input.Effort != record.Effort {
		err := errors.New("child reasoning effort authority is invalid")
		return CompleteTaskResult{Output: RunOutput(record, "", nil, err), Record: record, IsError: true}
	}
	if err := domainjob.ValidateExecutableCaseDelegationV1(
		record.Kind, record.Name, record.Label, record.Prompt, record.SecurityBinding, record.CaseDelegation,
	); err != nil {
		return CompleteTaskResult{Output: RunOutput(record, "", nil, err), Record: record, IsError: true}
	}
	driver := input.Driver
	if driver == nil {
		err := errors.New("subagent completion driver is required")
		return CompleteTaskResult{Output: RunOutput(record, "", nil, err), Record: record, IsError: true}
	}
	if input.StartAuthority == nil {
		err := errors.New("child turn start authority is required")
		if updated, updateErr := driver.UpdateChildRun(record.ID, domainjob.UpdateRequest{
			Status: string(domainjob.StatusInterrupted), FailureCode: domainjob.FailureChildInterrupted,
		}); updateErr == nil {
			record = updated
		}
		driver.RecordProgress(record, string(domainjob.StatusInterrupted), domainjob.FailureChildInterrupted)
		return CompleteTaskResult{Output: RunOutput(record, "", nil, err), Record: record, IsError: true}
	}
	if input.Request.TimeBudgetMSSet && input.Request.TimeBudgetMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(input.Request.TimeBudgetMS)*time.Millisecond)
		defer cancel()
	}
	effectiveToolScope := append([]string(nil), input.ToolScope...)
	if SecurityBoundChildOutput(record) {
		effectiveToolScope = append([]string(nil), record.ToolScope...)
	}
	maxSteps := ResolvedMaxSteps(input.Request, input.ParentMaxModelSteps)
	childPrompt := strings.TrimSpace(record.Prompt)
	if childPrompt == "" && (record.SecurityBinding == nil || record.SecurityBinding.ParentCaseID == domainsecurity.UnboundCaseID) {
		childPrompt = input.Request.Prompt
	}
	turnRequest := controlapp.StartTurnRequest{
		ThreadID:                  record.ChildThreadID,
		Prompt:                    childPrompt,
		Model:                     input.Model,
		ProviderID:                input.ProviderID,
		EndpointFormat:            "",
		ReasoningEffort:           input.Effort,
		ApprovalPolicy:            ApprovalPolicy(input.Request.ToolPolicy, input.ParentApprovalPolicy),
		SandboxMode:               input.ParentSandboxMode,
		DisableUserInput:          true,
		DisableUserInputSet:       true,
		MaxModelSteps:             &maxSteps,
		InternalToolScope:         effectiveToolScope,
		InternalSubagentDepth:     input.ParentSubagentDepth + 1,
		InternalUsageSource:       usageapp.SourceSubagent,
		InternalSystemPrompt:      SystemPromptForRequest(input.Request),
		InternalChildRunID:        record.ID,
		InternalOutputTokenBudget: input.Request.TokenBudget,
	}
	var response map[string]any
	var authorizeErr, validationErr error
	err := input.StartAuthority.StartIfActive(ctx, func() error {
		if input.AuthorizeStart != nil {
			var authorizedRecord domainjob.Record
			authorizedRecord, authorizeErr = input.AuthorizeStart(record)
			if strings.TrimSpace(authorizedRecord.ID) != "" {
				record = authorizedRecord
			}
			if authorizeErr != nil {
				return authorizeErr
			}
		}
		// Cancellation can win while the durable claim is in flight.
		if err := ctx.Err(); err != nil {
			return err
		}
		validationErr = domainjob.ValidateExecutableCaseDelegationV1(
			record.Kind, record.Name, record.Label, record.Prompt, record.SecurityBinding, record.CaseDelegation,
		)
		if validationErr != nil {
			return validationErr
		}
		turnRequest.Prompt = strings.TrimSpace(record.Prompt)
		if turnRequest.Prompt == "" && (record.SecurityBinding == nil || record.SecurityBinding.ParentCaseID == domainsecurity.UnboundCaseID) {
			turnRequest.Prompt = input.Request.Prompt
		}
		if SecurityBoundChildOutput(record) {
			if domainjob.ValidateDelegatedToolManifestV1(record.DelegatedToolManifest, record.ToolScope, record.ToolSchemaHash) != nil {
				validationErr = errors.New("subagent_tool_manifest_invalid")
				return validationErr
			}
			turnRequest.InternalToolScope = append([]string(nil), record.ToolScope...)
		}
		var startErr error
		response, startErr = driver.StartTurn(ctx, turnRequest)
		return startErr
	})
	// Settle failure after releasing the start barrier. A lost claim belongs
	// to another lifecycle operation and must never enter generic cleanup.
	if authorizeErr != nil {
		if IsJobStartClaimError(authorizeErr) {
			status := firstNonEmptyAnyString(record.Status, string(domainjob.StatusInterrupted))
			driver.RecordProgress(record, status, childFailureCodeV1(record, status))
			return completeTaskResult(ctx, input, record, RunOutput(record, "", nil, authorizeErr), true)
		}
		if updated, updateErr := driver.UpdateChildRun(record.ID, domainjob.UpdateRequest{
			Status: string(domainjob.StatusInterrupted), FailureCode: domainjob.FailureChildInterrupted,
		}); updateErr == nil {
			record = updated
		}
		driver.RecordProgress(record, string(domainjob.StatusInterrupted), domainjob.FailureChildInterrupted)
		return completeTaskResult(ctx, input, record, RunOutput(record, "", nil, authorizeErr), true)
	}
	if validationErr != nil {
		if updated, updateErr := driver.UpdateChildRun(record.ID, domainjob.UpdateRequest{
			Status: string(domainjob.StatusFailed), FailureCode: domainjob.FailureChildExecutionFailed,
		}); updateErr == nil {
			record = updated
		}
		driver.RecordProgress(record, string(domainjob.StatusFailed), domainjob.FailureChildExecutionFailed)
		return completeTaskResult(ctx, input, record, RunOutput(record, "", nil, validationErr), true)
	}
	if err != nil {
		status := string(domainjob.StatusFailed)
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			status = string(domainjob.StatusKilled)
		}
		failureCode := domainjob.NormalizeFailureCode("", status, true)
		if updated, updateErr := driver.UpdateChildRun(record.ID, domainjob.UpdateRequest{Status: status, FailureCode: failureCode}); updateErr == nil {
			record = updated
		}
		status = firstNonEmptyAnyString(record.Status, status)
		driver.RecordProgress(record, status, childFailureCodeV1(record, status))
		return completeTaskResult(ctx, input, record, RunOutput(record, "", nil, err), true)
	}
	childTurnID := firstNonEmptyAnyString(response["turnId"])
	if firstNonEmptyAnyString(response["status"]) == "waiting" {
		err := errors.New("subagent paused waiting for an interactive gate")
		if updated, updateErr := driver.UpdateChildRun(record.ID, domainjob.UpdateRequest{
			Status: string(domainjob.StatusFailed), ChildTurnID: childTurnID, FailureCode: domainjob.FailureChildExecutionFailed,
		}); updateErr == nil {
			record = updated
		}
		status := firstNonEmptyAnyString(record.Status, string(domainjob.StatusFailed))
		driver.RecordProgress(record, status, childFailureCodeV1(record, status))
		return completeTaskResult(ctx, input, record, RunOutput(record, "", nil, err), true)
	}
	latest, loadErr := driver.LoadChildRun(record.ID)
	if loadErr != nil {
		return CompleteTaskResult{Output: RunOutput(record, "", nil, loadErr), Record: record, IsError: true, Err: loadErr}
	}
	if latest.ID != record.ID || latest.ChildThreadID != record.ChildThreadID ||
		latest.ParentThreadID != record.ParentThreadID || latest.ParentTurnID != record.ParentTurnID || latest.ParentToolCallID != record.ParentToolCallID ||
		!reflect.DeepEqual(latest.SecurityBinding, record.SecurityBinding) || !reflect.DeepEqual(latest.CaseDelegation, record.CaseDelegation) {
		err := errors.New("child completion job identity changed")
		return CompleteTaskResult{Output: RunOutput(record, "", nil, err), Record: record, IsError: true, Err: err}
	}
	record = latest
	if latest.Status == string(domainjob.StatusKilled) {
		driver.RecordProgress(record, string(domainjob.StatusKilled), childFailureCodeV1(record, record.Status))
		return completeTaskResult(ctx, input, record, RunOutput(record, record.Output, record.Usage, ErrorForTerminalRecord(record, "subagent killed")), true)
	}
	summary, toolInvocations, readErr := driver.TurnCompletion(record.ChildThreadID, childTurnID)
	if readErr != nil {
		return CompleteTaskResult{Output: RunOutput(record, "", nil, readErr), Record: record, IsError: true, Err: readErr}
	}
	if strings.TrimSpace(summary) == "" {
		err := errors.New("subagent completed without a final response")
		if updated, updateErr := driver.UpdateChildRun(record.ID, domainjob.UpdateRequest{
			Status:          string(domainjob.StatusFailed),
			ChildTurnID:     childTurnID,
			FailureCode:     domainjob.FailureChildExecutionFailed,
			ToolInvocations: &toolInvocations,
		}); updateErr == nil {
			record = updated
		}
		status := firstNonEmptyAnyString(record.Status, string(domainjob.StatusFailed))
		driver.RecordProgress(record, status, childFailureCodeV1(record, status))
		return completeTaskResult(ctx, input, record, RunOutput(record, "", nil, err), true)
	}
	usage := mapFromStartTurnResponse(response, "usage")
	if usage == nil {
		usage, readErr = driver.UsageSnapshot(record.ChildThreadID)
		if readErr != nil {
			return CompleteTaskResult{Output: RunOutput(record, "", nil, readErr), Record: record, IsError: true, Err: readErr}
		}
	}
	if usage == nil {
		usage = map[string]any{}
	}
	usage["usageSource"] = usageapp.SourceSubagent
	usage["childRunId"] = record.ID
	cacheDiagnostics := mapFromStartTurnResponse(response, "cacheDiagnostics")
	if cacheDiagnostics == nil {
		cacheDiagnostics, readErr = driver.CacheDiagnostics(record.ChildThreadID, childTurnID)
		if readErr != nil {
			return CompleteTaskResult{Output: RunOutput(record, "", nil, readErr), Record: record, IsError: true, Err: readErr}
		}
	}
	if len(cacheDiagnostics) > 0 {
		usage["cacheDiagnostics"] = cacheDiagnostics
	}
	persistedSummary := summary
	persistedUsage := usage
	if SecurityBoundChildOutput(record) {
		persistedSummary = ""
		persistedUsage = nil
	}
	var verifiedCompletion VerifiedChildCompletion
	var completionReceipt *domainjob.ChildCompletionReceiptV1
	var preparedForeground PreparedForegroundHandoff
	var foregroundReceipt *domainjob.ForegroundChildHandoffReceiptV1
	if caseChildCompletionRequired(record) {
		if input.CompletionAuthority == nil {
			err := errors.New("child completion receipt authority is required")
			return CompleteTaskResult{Output: RunOutput(record, "", nil, err), Record: record, IsError: true, Err: err}
		}
		var receiptErr error
		verifiedCompletion, receiptErr = input.CompletionAuthority.Issue(ctx, record, childTurnID)
		if receiptErr == nil {
			completionReceipt, receiptErr = verifiedCompletion.ReceiptForPersistence()
		}
		if receiptErr != nil {
			err := errors.Join(errors.New("child completion receipt was not issued"), receiptErr)
			return CompleteTaskResult{Output: RunOutput(record, "", nil, err), Record: record, IsError: true, Err: err}
		}
		record.ChildCompletionReceipt = domainjob.CloneChildCompletionReceiptV1(completionReceipt)
	}
	if foregroundSubmitOnlyRecord(record) {
		if input.ForegroundAuthority == nil {
			err := errors.New("foreground child handoff authority is required")
			return CompleteTaskResult{Output: RunOutput(record, "", nil, err), Record: record, IsError: true, Err: err}
		}
		var handoffErr error
		preparedForeground, handoffErr = input.ForegroundAuthority.Prepare(ctx, record, childTurnID)
		if handoffErr == nil {
			foregroundReceipt, handoffErr = preparedForeground.ReceiptForPersistence()
		}
		if handoffErr != nil {
			err := errors.Join(errors.New("foreground child handoff receipt was not prepared"), handoffErr)
			return CompleteTaskResult{Output: RunOutput(record, "", nil, err), Record: record, IsError: true, Err: err}
		}
	}
	if updated, updateErr := driver.UpdateChildRun(record.ID, domainjob.UpdateRequest{
		Status:                        string(domainjob.StatusCompleted),
		ChildTurnID:                   childTurnID,
		ChildCompletionReceipt:        completionReceipt,
		ForegroundChildHandoffReceipt: foregroundReceipt,
		Output:                        persistedSummary,
		Usage:                         persistedUsage,
		ToolInvocations:               &toolInvocations,
	}); updateErr == nil {
		record = updated
	}
	if strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) {
		if input.ForegroundAuthority != nil {
			input.ForegroundAuthority.Delete(record.ID)
		}
		status := firstNonEmptyAnyString(record.Status, string(domainjob.StatusFailed))
		message := childFailureCodeV1(record, status)
		driver.RecordProgress(record, status, message)
		return completeTaskResult(ctx, input, record, RunOutput(record, record.Output, record.Usage, ErrorForTerminalRecord(record, message)), true)
	}
	driver.RecordProgress(record, string(domainjob.StatusCompleted), "")
	if foregroundReceipt != nil {
		verified, consumeErr := input.ForegroundAuthority.Consume(ctx, record, preparedForeground)
		if consumeErr != nil {
			input.ForegroundAuthority.Delete(record.ID)
			driver.RecordProgress(record, string(domainjob.StatusCompleted), "foreground child handoff capability could not be consumed")
			return CompleteTaskResult{Output: SecurityBoundChildOutputProjection(record), Record: record, IsError: true}
		}
		return CompleteTaskResult{Output: verified.OutputProjection(record), Record: record}
	}
	if completionReceipt != nil {
		rehydrated, rehydrateErr := input.CompletionAuthority.Rehydrate(ctx, record)
		if rehydrateErr != nil {
			driver.RecordProgress(record, string(domainjob.StatusCompleted), "child completion receipt could not be rehydrated")
			return CompleteTaskResult{Output: SecurityBoundChildOutputProjection(record), Record: record, IsError: true}
		}
		return CompleteTaskResult{Output: rehydrated.OutputProjection(record), Record: record}
	}
	return completeTaskResult(ctx, input, record, RunOutput(record, persistedSummary, persistedUsage, nil), false)
}

func completeTaskResult(ctx context.Context, input CompleteTaskInput, record domainjob.Record, output map[string]any, isError bool) CompleteTaskResult {
	if SecurityBoundChildOutput(record) {
		return CompleteTaskResult{Output: SecurityBoundChildOutputProjection(record), Record: record, IsError: isError}
	}
	if updated, ok := SummarizeWorktreeIsolation(ctx, WorktreeIsolationSummaryInput{Manager: input.WorktreeManager, Store: input.Driver, Record: record}); ok {
		record = updated
		AddIsolationMetadata(output, updated)
		input.Driver.RecordProgress(updated, updated.Status, "")
	}
	return CompleteTaskResult{Output: output, Record: record, IsError: isError}
}

func mapFromStartTurnResponse(response map[string]any, key string) map[string]any {
	if response == nil {
		return nil
	}
	value, ok := response[key].(map[string]any)
	if !ok {
		return nil
	}
	return contracts.CloneMap(value)
}

func childFailureCodeV1(record domainjob.Record, status string) string {
	return domainjob.NormalizeFailureCode(record.FailureCode, status, true)
}
