package loop

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type ToolStepDriver interface {
	AcquireToolCallAdmission(context.Context, appmodel.PendingToolCall) (context.Context, func(), error)
	PersistToolCallReady(context.Context, string, string, domainmodel.ToolCall, int, domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant) (string, error)
	PersistToolResultAndMessage(context.Context, appmodel.PendingToolCall, any, bool) (domainmodel.Message, error)
	RequestUserInput(context.Context, appmodel.PendingToolCall) (string, error)
	RequestApproval(context.Context, appmodel.PendingToolCall) (string, error)
	ToolPolicy(string) (readOnly bool, parallel bool)
	Preflight(appmodel.PendingToolCall) (any, bool, bool)
	ExecuteAndSettle(context.Context, appmodel.PendingToolCall, any, ToolOutputTransform) (SettledToolExecution, error)
	ExecuteAuthorizedReadOnlyBatch(context.Context, []appmodel.PendingToolCall) (string, []domainmodel.Message, error)
	SettleToolBatchAuthority(context.Context, string, domainsecurity.TurnSecurityContext, []appmodel.PendingToolCall, string, string) error
	RecordLoopGuard(string, string, string, int, string) error
}

type ToolStepInput struct {
	ThreadID                        string
	TurnID                          string
	ProviderConfig                  domainmodel.TurnConfig
	ProviderID                      string
	Model                           string
	Effort                          string
	Workspace                       string
	Prompt                          string
	LogicalEffect                   domainsecurity.LogicalEffect
	OrdinaryWork                    bool
	CaseSourceUnavailable           bool
	OrdinaryResultInputIsolated     bool
	AttachmentIDs                   []string
	AttachmentPlanDigest            string
	WorkspaceCheckpointID           string
	Mode                            string
	GUIPlan                         map[string]any
	ApprovalPolicy                  string
	SandboxMode                     string
	DisableUserInput                bool
	MaxModelSteps                   *int
	EffectiveMaxModelSteps          int
	Messages                        []domainmodel.Message
	PrivateProtocolSession          *domainmodel.PrivateProtocolSession
	AnthropicCapsule                *domainmodel.AnthropicThinkingCapsule
	PrivateProtocolCapsules         []*domainmodel.AnthropicThinkingCapsule
	ProviderNamespace               domaincontinuation.ProviderContinuationNamespaceV1
	TerminalRecoveryKind            RuntimeTerminalRecoveryKind
	ToolCalls                       []domainmodel.ToolCall
	ToolSchemas                     []domainmodel.ToolSchema
	ToolScope                       []string
	SubagentDepth                   int
	AdvertisedTools                 map[string]bool
	PromptRoute                     string
	LoopStep                        int
	AdvertisedToolCount             int
	AdvertisedToolManifestHash      string
	AdvertisedNameSetSortedHash     string
	ProviderRequestToolManifestHash string
	KnownBuiltinTools               map[string]bool
	KnownMCPTools                   map[string]bool
	LiveMCPTools                    map[string]bool
	MCPConnectionEpochs             map[string]uint64
	MCPServerIdentities             map[string]string
	MCPReadOnlyPolicies             map[string]bool
	CreatePlanToolName              string
	RequiredFinalToolName           string
	SecurityContext                 domainsecurity.TurnSecurityContext
	HostEntitySelection             HostCaseEntitySelectionV1
	State                           ToolStepState
	Driver                          ToolStepDriver
}

type ToolStepState struct {
	CreatePlanSatisfied   bool
	RepeatSuccessCounts   map[string]int
	FailureStormSignature string
	FailureStormCount     int
}

type ToolStepResult struct {
	Messages                      []domainmodel.Message
	SettledToolReferences         []domainsecurity.SettledToolReference
	OrdinarySettledToolReferences []domainsecurity.SettledToolReference
	providerCorrectableToolName   string
	State                         ToolStepState
	ProtectedLaneUnavailable      *ProtectedLaneUnavailableV1
	Paused                        bool
	PendingKind                   string
	PendingID                     string
	ProviderContinuationBlocked   bool
	ProviderContinuationBoundary  string
	RequiredFinalToolAccepted     bool
	RequiredFinalToolRejected     bool
	ReportDeliveryCompleted       bool
}

const duplicateCreatePlanValidationCode = "duplicate_create_plan_calls"

func RunToolStep(ctx context.Context, input ToolStepInput) (ToolStepResult, error) {
	if len(input.ToolScope) == 0 && len(input.ToolSchemas) > 0 {
		input.ToolScope = make([]string, 0, len(input.ToolSchemas))
		for _, schema := range input.ToolSchemas {
			input.ToolScope = append(input.ToolScope, schema.Name)
		}
	}
	state := input.State
	if state.RepeatSuccessCounts == nil {
		state.RepeatSuccessCounts = map[string]int{}
	}
	result := ToolStepResult{
		Messages: appmodel.CloneProviderMessages(input.Messages),
		State:    state,
	}
	if len(input.ToolCalls) > 0 {
		if err := domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(input.SecurityContext); err != nil || input.SecurityContext.ThreadID != input.ThreadID || input.SecurityContext.TurnID != input.TurnID {
			return result, TurnFailureError{Message: "turn security context is missing or invalid", Code: "turn_security_context_invalid", Severity: "error"}
		}
	}
	if required := strings.TrimSpace(input.RequiredFinalToolName); required != "" {
		if len(input.ToolCalls) != 1 || strings.TrimSpace(input.ToolCalls[0].Name) != required {
			return result, TurnFailureError{
				Message: "foreground child must terminate with exactly one " + required + " call",
				Code:    "subagent_required_final_tool_invalid", Severity: "error",
			}
		}
	}
	seenCallIDs := map[string]struct{}{}
	for index := range input.ToolCalls {
		if len(bytes.TrimSpace(input.ToolCalls[index].Arguments)) == 0 {
			input.ToolCalls[index].Arguments = []byte("{}")
		}
		call := input.ToolCalls[index]
		callID := strings.TrimSpace(call.ID)
		if !domainmodel.IsHostToolCallIDV1(callID) {
			return result, TurnFailureError{
				Message:  "provider tool call lacks a host-issued identity",
				Code:     "tool_call_identity_invalid",
				Details:  map[string]any{"toolName": call.Name},
				Severity: "error",
			}
		}
		if _, duplicate := seenCallIDs[callID]; duplicate {
			return result, TurnFailureError{
				Message:  call.Name + " reused provider tool call identity " + callID,
				Code:     "tool_call_identity_invalid",
				Details:  map[string]any{"toolName": call.Name, "toolCallId": callID},
				Severity: "error",
			}
		}
		seenCallIDs[callID] = struct{}{}
		if !input.AdvertisedTools[call.Name] {
			return result, TurnFailureError{
				Message:  domainfailure.New("tool_not_advertised", nil).Message(),
				Code:     "tool_not_advertised",
				Details:  unadvertisedToolDiagnosticV1(input, call.Name),
				Severity: "error",
			}
		}
		if validationOutput, blocked := toolcatalogapp.ValidateToolCallArguments(call, input.ToolSchemas); blocked {
			code := strings.TrimSpace(fmt.Sprint(validationOutput["code"]))
			if code == "" {
				code = "validation_error"
			}
			if code == "validation_error" {
				result.providerCorrectableToolName = strings.TrimSpace(call.Name)
			}
			return result, TurnFailureError{
				Message:  strings.TrimSpace(fmt.Sprint(validationOutput["error"])),
				Code:     code,
				Details:  validationOutput,
				Severity: "error",
			}
		}
		if toolcatalogapp.IsCaseBoundSecurityContext(input.SecurityContext) && toolcatalogapp.CaseArtifactCallRequiresAuthority(call) {
			return result, TurnFailureError{
				Message:  "case artifact execution requires host publication authority",
				Code:     "publication_receipt_required",
				Details:  map[string]any{"toolName": call.Name, "executed": false},
				Severity: "error",
			}
		}
		if err := validateAttemptPrivateToolArgumentsV1(call); err != nil {
			return result, TurnFailureError{
				Message:  call.Name + " arguments contain private material and were rejected before persistence",
				Code:     "tool_private_arguments",
				Details:  map[string]any{"toolName": call.Name},
				Severity: "error",
			}
		}
	}
	createPlanToolName := strings.TrimSpace(input.CreatePlanToolName)
	if createPlanToolName == "" {
		createPlanToolName = toolcatalogapp.ToolCreatePlanName
	}
	if input.AdvertisedTools[createPlanToolName] {
		createPlanCalls := 0
		for _, call := range input.ToolCalls {
			if strings.TrimSpace(call.Name) == createPlanToolName {
				createPlanCalls++
			}
		}
		if createPlanCalls > 1 {
			result.providerCorrectableToolName = createPlanToolName
			return result, TurnFailureError{
				Message: "Plan mode requires exactly one create_plan call per provider response",
				Code:    "validation_error",
				Details: map[string]any{
					"code":      duplicateCreatePlanValidationCode,
					"toolName":  createPlanToolName,
					"callCount": float64(createPlanCalls),
					"executed":  false,
				},
				Severity: "error",
			}
		}
	}
	markProtectedLaneUnavailable := func(code string) {
		if !input.OrdinaryWork || result.ProtectedLaneUnavailable != nil {
			return
		}
		if exact, ok := recoverableProtectedLaneCodeV1(code); ok {
			result.ProtectedLaneUnavailable = &ProtectedLaneUnavailableV1{Code: exact}
		}
	}
	markProtectedLaneError := func(err error) bool {
		code, ok := RecoverableProtectedLaneFailureCodeV1(err)
		if ok {
			markProtectedLaneUnavailable(code)
		}
		return ok && result.ProtectedLaneUnavailable != nil
	}
	clearProtectedLaneUnavailable := func() {
		result.ProtectedLaneUnavailable = nil
	}
	appendSettledToolReference := func(pending appmodel.PendingToolCall) {
		reference := settledToolReference(pending)
		result.SettledToolReferences = append(result.SettledToolReferences, reference)
		if !executiongrantapp.CallUsesCaseDataAuthority(pending.Call) {
			result.OrdinarySettledToolReferences = append(result.OrdinarySettledToolReferences, reference)
		}
	}
	readOnlyBatch := []toolStepScheduledCall{}
	flushReadOnlyBatch := func() error {
		if len(readOnlyBatch) == 0 {
			return nil
		}
		calls := readOnlyBatch
		readOnlyBatch = nil
		ordinary, protected := partitionScheduledReadOnlyBatchByAuthority(calls)
		partitions := []struct {
			calls     []toolStepScheduledCall
			protected bool
		}{
			{calls: ordinary},
			{calls: protected, protected: true},
		}
		messagesByCallID := make(map[string]domainmodel.Message, len(calls))
		settledByCallID := make(map[string]appmodel.PendingToolCall, len(calls))
		batchSettled := false
		appended := false
		appendCompleted := func() {
			if appended {
				return
			}
			appended = true
			for _, scheduled := range calls {
				callID := strings.TrimSpace(scheduled.Call.ID)
				if message, ok := messagesByCallID[callID]; ok {
					result.Messages = append(result.Messages, message)
				}
				if pending, ok := settledByCallID[callID]; ok {
					appendSettledToolReference(pending)
				}
			}
			if batchSettled {
				result.State.FailureStormSignature = ""
				result.State.FailureStormCount = 0
			}
		}
		for _, partition := range partitions {
			if len(partition.calls) == 0 {
				continue
			}
			pendingCalls := make([]appmodel.PendingToolCall, 0, len(partition.calls))
			protectedReady := false
			for _, scheduled := range partition.calls {
				call := scheduled.Call
				if err := ctx.Err(); err != nil {
					clearProtectedLaneUnavailable()
					appendCompleted()
					return err
				}
				if scheduled.PreparationFailure != nil {
					if !partition.protected || protectedReady || !markProtectedLaneError(scheduled.PreparationFailure) {
						clearProtectedLaneUnavailable()
					}
					appendCompleted()
					return scheduled.PreparationFailure
				}
				grant, err := executiongrantapp.IssueProvider(
					input.SecurityContext, input.ProviderID, call, input.ToolSchemas, input.ToolScope,
					scheduled.ReadOnly, scheduled.ApprovalState,
					input.MCPConnectionEpochs[call.Name], input.MCPServerIdentities[call.Name], time.Now().UTC(),
				)
				if err != nil {
					failure := TurnFailureError{
						Message: "tool execution grant could not be issued", Code: "execution_grant_rejected",
						Details: executiongrantapp.ErrorDetails(err, call.Name), Severity: "error",
					}
					if !partition.protected || protectedReady || !markProtectedLaneError(failure) {
						clearProtectedLaneUnavailable()
					}
					appendCompleted()
					return failure
				}
				pending := pendingToolCallForStep(input, call, "", result.Messages, grant)
				pending.PriorSettledToolRefs = append([]domainsecurity.SettledToolReference(nil), result.SettledToolReferences...)
				admissionBaseCtx, cancelAdmission := toolCallAdmissionContext(ctx)
				admissionCtx, releaseAdmissionLease, err := input.Driver.AcquireToolCallAdmission(admissionBaseCtx, pending)
				if err != nil {
					cancelAdmission()
					failure := toolCallAdmissionFailure(call.Name, err)
					if !partition.protected || protectedReady || !markProtectedLaneError(err) {
						clearProtectedLaneUnavailable()
					}
					appendCompleted()
					return failure
				}
				if admissionCtx == nil || releaseAdmissionLease == nil {
					if releaseAdmissionLease != nil {
						releaseAdmissionLease()
					}
					cancelAdmission()
					clearProtectedLaneUnavailable()
					appendCompleted()
					return toolCallAdmissionFailure(call.Name, errors.New("tool call admission returned an invalid authority lease"))
				}
				var released bool
				releaseAdmission := func() {
					if released {
						return
					}
					released = true
					releaseAdmissionLease()
					cancelAdmission()
				}
				itemID, err := input.Driver.PersistToolCallReady(
					admissionCtx, input.ThreadID, input.TurnID, call, len(input.ToolCalls), input.SecurityContext, grant,
				)
				if err != nil {
					releaseAdmission()
					clearProtectedLaneUnavailable()
					appendCompleted()
					return err
				}
				pending.ToolCallItemID = itemID
				if partition.protected {
					protectedReady = true
				}
				if cancelErr := ctx.Err(); cancelErr != nil {
					releaseAdmission()
					output := toolcatalogapp.CancelledOutput(call.Name, call.ID, cancelErr)
					message, err := input.Driver.PersistToolResultAndMessage(ctx, pending, output, true)
					if err != nil {
						clearProtectedLaneUnavailable()
						appendCompleted()
						return err
					}
					messagesByCallID[call.ID] = message
					settledByCallID[call.ID] = pending
					continue
				}
				if output, blocked := RepeatSuccessBlock(call, result.State.RepeatSuccessCounts); blocked {
					releaseAdmission()
					message, err := input.Driver.PersistToolResultAndMessage(ctx, pending, output, true)
					if err != nil {
						clearProtectedLaneUnavailable()
						appendCompleted()
						return err
					}
					messagesByCallID[call.ID] = message
					settledByCallID[call.ID] = pending
					if partition.protected {
						if code, ok := RecoverableProtectedLaneToolMessageCodeV1(message); ok {
							markProtectedLaneUnavailable(code)
						}
					}
					continue
				}
				if output, isError, blocked := input.Driver.Preflight(pending); blocked {
					var guarded bool
					output, guarded = ApplyFailureStormGuard(
						call, output, isError, &result.State.FailureStormSignature, &result.State.FailureStormCount,
					)
					if guarded {
						if err := input.Driver.RecordLoopGuard(input.ThreadID, input.TurnID, call.Name, result.State.FailureStormCount, "tool_failure"); err != nil {
							releaseAdmission()
							clearProtectedLaneUnavailable()
							appendCompleted()
							return err
						}
					}
					releaseAdmission()
					message, err := input.Driver.PersistToolResultAndMessage(ctx, pending, output, isError)
					if err != nil {
						clearProtectedLaneUnavailable()
						appendCompleted()
						return err
					}
					messagesByCallID[call.ID] = message
					settledByCallID[call.ID] = pending
					if partition.protected {
						if code, ok := RecoverableProtectedLaneToolMessageCodeV1(message); ok {
							markProtectedLaneUnavailable(code)
						}
					}
					if failure, terminal := FailureStormTurnFailure(call, output, isError, result.State.FailureStormCount); terminal {
						clearProtectedLaneUnavailable()
						appendCompleted()
						return failure
					}
					continue
				}
				releaseAdmission()
				pendingCalls = append(pendingCalls, pending)
			}
			if len(pendingCalls) == 0 {
				continue
			}
			batchID, toolMessages, err := input.Driver.ExecuteAuthorizedReadOnlyBatch(ctx, pendingCalls)
			if err != nil {
				if batchID == "" {
					if !partition.protected || !markProtectedLaneError(err) {
						clearProtectedLaneUnavailable()
					}
					appendCompleted()
					return err
				}
				closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
				closeErr := input.Driver.SettleToolBatchAuthority(closeCtx, batchID, input.SecurityContext, pendingCalls, "failed", "batch_execution_failed")
				cancel()
				clearProtectedLaneUnavailable()
				appendCompleted()
				return errors.Join(err, closeErr)
			}
			partitionMessages, err := indexReadOnlyBatchMessages(pendingCalls, toolMessages)
			if err != nil {
				closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
				closeErr := input.Driver.SettleToolBatchAuthority(closeCtx, batchID, input.SecurityContext, pendingCalls, "failed", "batch_execution_failed")
				cancel()
				clearProtectedLaneUnavailable()
				appendCompleted()
				return errors.Join(err, closeErr)
			}
			status, reason := "completed", "batch_completed"
			if ctx.Err() != nil {
				status, reason = "cancelled", "batch_cancelled"
			}
			closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			closeErr := input.Driver.SettleToolBatchAuthority(closeCtx, batchID, input.SecurityContext, pendingCalls, status, reason)
			cancel()
			if closeErr != nil || status != "completed" {
				clearProtectedLaneUnavailable()
				appendCompleted()
				return errors.Join(ctx.Err(), closeErr)
			}
			batchSettled = true
			for _, pending := range pendingCalls {
				callID := strings.TrimSpace(pending.Call.ID)
				message := partitionMessages[callID]
				messagesByCallID[callID] = message
				settledByCallID[callID] = pending
				if partition.protected {
					if code, ok := RecoverableProtectedLaneToolMessageCodeV1(message); ok {
						markProtectedLaneUnavailable(code)
					}
				}
			}
		}
		appendCompleted()
		return nil
	}
	for _, call := range input.ToolCalls {
		if err := ctx.Err(); err != nil {
			clearProtectedLaneUnavailable()
			return result, err
		}
		scheduled, err := scheduleToolStepCall(input, call)
		if err != nil {
			clearProtectedLaneUnavailable()
			return result, err
		}
		if scheduled.Batchable {
			readOnlyBatch = append(readOnlyBatch, scheduled)
			continue
		}
		if err := flushReadOnlyBatch(); err != nil {
			return result, err
		}
		if scheduled.PreparationFailure != nil {
			if !executiongrantapp.CallUsesCaseDataAuthority(call) || !markProtectedLaneError(scheduled.PreparationFailure) {
				clearProtectedLaneUnavailable()
			}
			return result, scheduled.PreparationFailure
		}
		readOnly := scheduled.ReadOnly
		approvalState := scheduled.ApprovalState
		grantSchemas := input.ToolSchemas
		if approvalState == "pending" {
			if preparer, ok := input.Driver.(interface {
				PrepareReportApproval(
					context.Context,
					ReportApprovalPreparationInputV1,
				) (ReportApprovalPreparationV1, error)
			}); ok {
				prepared, err := preparer.PrepareReportApproval(
					ctx,
					ReportApprovalPreparationInputV1{
						Prompt: input.Prompt, Call: call,
						ToolSchemas:         input.ToolSchemas,
						ToolScope:           input.ToolScope,
						SecurityContext:     input.SecurityContext,
						HostEntitySelection: input.HostEntitySelection,
					},
				)
				if err != nil {
					return result, TurnFailureError{
						Message: "report approval could not be prepared",
						Code:    "report_publication_fallback",
						Details: map[string]any{
							"toolName": call.Name,
						},
						Severity: "error",
					}
				}
				call = prepared.Call
				grantSchemas = prepared.ToolSchemas
			}
		}
		grant, err := executiongrantapp.IssueProvider(input.SecurityContext, input.ProviderID, call, grantSchemas, input.ToolScope, readOnly, approvalState,
			input.MCPConnectionEpochs[call.Name], input.MCPServerIdentities[call.Name], time.Now().UTC())
		if err != nil {
			failure := TurnFailureError{Message: "tool execution grant could not be issued", Code: "execution_grant_rejected", Details: executiongrantapp.ErrorDetails(err, call.Name), Severity: "error"}
			if !executiongrantapp.CallUsesCaseDataAuthority(call) || !markProtectedLaneError(err) {
				clearProtectedLaneUnavailable()
			}
			return result, failure
		}
		pending := pendingToolCallForStep(input, call, "", result.Messages, grant)
		pending.PriorSettledToolRefs = append([]domainsecurity.SettledToolReference(nil), result.SettledToolReferences...)
		admissionBaseCtx, cancelAdmission := toolCallAdmissionContext(ctx)
		admissionCtx, releaseAdmissionLease, err := input.Driver.AcquireToolCallAdmission(admissionBaseCtx, pending)
		if err != nil {
			cancelAdmission()
			failure := toolCallAdmissionFailure(call.Name, err)
			if !executiongrantapp.CallUsesCaseDataAuthority(call) || !markProtectedLaneError(err) {
				clearProtectedLaneUnavailable()
			}
			return result, failure
		}
		if admissionCtx == nil || releaseAdmissionLease == nil {
			if releaseAdmissionLease != nil {
				releaseAdmissionLease()
			}
			cancelAdmission()
			return result, toolCallAdmissionFailure(call.Name, errors.New("tool call admission returned an invalid authority lease"))
		}
		var releaseAdmissionOnce bool
		releaseAdmission := func() {
			if releaseAdmissionOnce {
				return
			}
			releaseAdmissionOnce = true
			releaseAdmissionLease()
			cancelAdmission()
		}
		itemID, err := input.Driver.PersistToolCallReady(admissionCtx, input.ThreadID, input.TurnID, call, len(input.ToolCalls), input.SecurityContext, grant)
		if err != nil {
			releaseAdmission()
			return ToolStepResult{}, err
		}
		pending.ToolCallItemID = itemID
		if cancelErr := ctx.Err(); cancelErr != nil {
			releaseAdmission()
			output := toolcatalogapp.CancelledOutput(call.Name, call.ID, cancelErr)
			message, err := input.Driver.PersistToolResultAndMessage(ctx, pending, output, true)
			if err != nil {
				return ToolStepResult{}, err
			}
			result.Messages = append(result.Messages, message)
			appendSettledToolReference(pending)
			if executiongrantapp.CallUsesCaseDataAuthority(call) {
				if code, ok := RecoverableProtectedLaneToolMessageCodeV1(message); ok {
					markProtectedLaneUnavailable(code)
				}
			}
			continue
		}
		if toolcatalogapp.IsUserInputTool(call.Name) {
			pending.Messages = appmodel.CloneProviderMessages(result.Messages)
			inputID, err := input.Driver.RequestUserInput(admissionCtx, pending)
			releaseAdmission()
			if err != nil {
				return ToolStepResult{}, err
			}
			result.Paused = true
			result.PendingKind = "user_input"
			result.PendingID = inputID
			return result, nil
		}
		if output, blocked := RepeatSuccessBlock(call, result.State.RepeatSuccessCounts); blocked {
			releaseAdmission()
			message, err := input.Driver.PersistToolResultAndMessage(ctx, pending, output, true)
			if err != nil {
				return ToolStepResult{}, err
			}
			result.Messages = append(result.Messages, message)
			appendSettledToolReference(pending)
			if executiongrantapp.CallUsesCaseDataAuthority(call) {
				if code, ok := RecoverableProtectedLaneToolMessageCodeV1(message); ok {
					markProtectedLaneUnavailable(code)
				}
			}
			continue
		}
		if output, isError, blocked := input.Driver.Preflight(pending); blocked {
			var guarded bool
			output, guarded = ApplyFailureStormGuard(call, output, isError, &result.State.FailureStormSignature, &result.State.FailureStormCount)
			if guarded {
				if err := input.Driver.RecordLoopGuard(input.ThreadID, input.TurnID, call.Name, result.State.FailureStormCount, "tool_failure"); err != nil {
					releaseAdmission()
					return ToolStepResult{}, err
				}
			}
			releaseAdmission()
			message, err := input.Driver.PersistToolResultAndMessage(ctx, pending, output, isError)
			if err != nil {
				return ToolStepResult{}, err
			}
			result.Messages = append(result.Messages, message)
			appendSettledToolReference(pending)
			if executiongrantapp.CallUsesCaseDataAuthority(call) {
				if code, ok := RecoverableProtectedLaneToolMessageCodeV1(message); ok {
					markProtectedLaneUnavailable(code)
				}
			}
			if failure, terminal := FailureStormTurnFailure(call, output, isError, result.State.FailureStormCount); terminal {
				clearProtectedLaneUnavailable()
				return result, failure
			}
			continue
		}
		if approvalState == "pending" {
			pending.Messages = appmodel.CloneProviderMessages(result.Messages)
			approvalID, err := input.Driver.RequestApproval(admissionCtx, pending)
			releaseAdmission()
			if err != nil {
				return ToolStepResult{}, err
			}
			result.Paused = true
			result.PendingKind = "approval"
			result.PendingID = approvalID
			return result, nil
		}
		releaseAdmission()
		settled, err := input.Driver.ExecuteAndSettle(ctx, pending, nil, func(output any, isError bool) (any, bool, error) {
			var guarded bool
			output, guarded = ApplyFailureStormGuard(call, output, isError, &result.State.FailureStormSignature, &result.State.FailureStormCount)
			if guarded {
				if err := input.Driver.RecordLoopGuard(input.ThreadID, input.TurnID, call.Name, result.State.FailureStormCount, "tool_failure"); err != nil {
					return output, isError, err
				}
			}
			return output, isError, nil
		})
		if err != nil {
			return ToolStepResult{}, err
		}
		output, isError, message := settled.Output, settled.IsError, settled.Message
		if !isError {
			RecordRepeatSuccess(call, result.State.RepeatSuccessCounts)
		}
		if call.Name == input.CreatePlanToolName && !isError {
			result.State.CreatePlanSatisfied = true
		}
		result.Messages = append(result.Messages, message)
		appendSettledToolReference(pending)
		if executiongrantapp.CallUsesCaseDataAuthority(call) {
			if code, ok := RecoverableProtectedLaneToolMessageCodeV1(message); ok {
				markProtectedLaneUnavailable(code)
			}
		}
		if strings.TrimSpace(input.RequiredFinalToolName) != "" && call.Name == input.RequiredFinalToolName {
			result.RequiredFinalToolAccepted = !isError
			result.RequiredFinalToolRejected = isError
		}
		if reportTerminal := EvaluateReportTerminalV1(
			input.SecurityContext, pending.ExecutionGrant, call, settled,
		); reportTerminal.Invalid {
			clearProtectedLaneUnavailable()
			return result, TurnFailureError{
				Message:  "report publication terminal authority is unavailable",
				Code:     "report_publication_fallback",
				Details:  map[string]any{"toolName": call.Name},
				Severity: "error",
			}
		} else if reportTerminal.Completed {
			result.ReportDeliveryCompleted = true
			result.ProviderContinuationBlocked = true
			result.ProviderContinuationBoundary = reportTerminal.Boundary
			return result, nil
		}
		if decision := EvaluateProviderContinuation(input.SecurityContext, pending.ExecutionGrant, call, settled); decision.Blocked {
			result.ProviderContinuationBlocked = true
			result.ProviderContinuationBoundary = decision.Boundary
			return result, nil
		}
		if failure, terminal := FailureStormTurnFailure(call, output, isError, result.State.FailureStormCount); terminal {
			clearProtectedLaneUnavailable()
			return result, failure
		}
	}
	if err := flushReadOnlyBatch(); err != nil {
		return result, err
	}
	return result, nil
}

func unadvertisedToolDiagnosticV1(input ToolStepInput, rawName string) map[string]any {
	if input.LoopStep < 0 || input.LoopStep > 1_000_000 ||
		input.AdvertisedToolCount < 0 || input.AdvertisedToolCount > 1_000_000 ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(input.AdvertisedToolManifestHash)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(input.AdvertisedNameSetSortedHash)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(input.ProviderRequestToolManifestHash)) {
		return nil
	}
	switch strings.TrimSpace(input.PromptRoute) {
	case toolcatalogapp.RouteDirectAnswer, toolcatalogapp.RouteLightAgent, toolcatalogapp.RouteToolAgent, toolcatalogapp.RouteSubagent:
	default:
		return nil
	}
	normalizedName := strings.TrimSpace(rawName)
	if runes := []rune(normalizedName); len(runes) > 256 {
		normalizedName = string(runes[:256])
	}
	category := "unknown_provider_name"
	switch {
	case input.KnownBuiltinTools[normalizedName] && toolcatalogapp.IsCompatibilityToolAlias(normalizedName):
		category = "known_alias_not_advertised"
	case input.KnownMCPTools[normalizedName]:
		category = "known_mcp_not_advertised"
	case input.KnownBuiltinTools[normalizedName]:
		category = "known_builtin_not_advertised"
	}
	runToolStepManifestHash := toolcatalogapp.ToolSchemaHash(input.ToolSchemas)
	providerRequestManifestHash := strings.TrimSpace(input.ProviderRequestToolManifestHash)
	details := map[string]any{
		"rejectedToolNormalizedNameSha256":       domainmodel.BytesHash([]byte(normalizedName)),
		"rejectedToolCategory":                   category,
		"promptRoute":                            strings.TrimSpace(input.PromptRoute),
		"loopStep":                               float64(input.LoopStep),
		"advertisedToolCount":                    float64(input.AdvertisedToolCount),
		"advertisedToolManifestHash":             strings.TrimSpace(input.AdvertisedToolManifestHash),
		"advertisedNameSetSortedHash":            strings.TrimSpace(input.AdvertisedNameSetSortedHash),
		"providerRequestToolManifestHash":        providerRequestManifestHash,
		"runToolStepManifestHash":                runToolStepManifestHash,
		"providerRequestRunToolStepManifestSame": providerRequestManifestHash == runToolStepManifestHash,
	}
	if !domainfailure.ValidateToolNotAdvertisedDetails(details) {
		return nil
	}
	return details
}

type toolStepScheduledCall struct {
	Call               domainmodel.ToolCall
	ReadOnly           bool
	ApprovalState      string
	Batchable          bool
	PreparationFailure error
}

func scheduleToolStepCall(input ToolStepInput, call domainmodel.ToolCall) (toolStepScheduledCall, error) {
	readOnly, parallel := input.Driver.ToolPolicy(call.Name)
	if toolcatalogapp.MCPToolServerID(call.Name) != "" {
		// The current-run live source is the first MCP authority seam. Do not
		// consult a cached/frozen policy when that source is absent. Keep a
		// parallel read-only failure queued so the batch can execute its
		// independent ordinary partition before surfacing the protected error.
		if !input.LiveMCPTools[call.Name] {
			return toolStepScheduledCall{
				Call: call, ReadOnly: readOnly, ApprovalState: "not_required",
				Batchable: !toolcatalogapp.IsUserInputTool(call.Name) && parallel && readOnly,
				PreparationFailure: TurnFailureError{
					Message: call.Name + " has no live MCP source for the current run",
					Code:    "tool_source_unavailable", Details: map[string]any{"toolName": call.Name}, Severity: "error",
				},
			}, nil
		}
		frozenReadOnly, frozen := input.MCPReadOnlyPolicies[call.Name]
		if !frozen {
			return toolStepScheduledCall{
				Call: call, ReadOnly: readOnly, ApprovalState: "not_required",
				Batchable: !toolcatalogapp.IsUserInputTool(call.Name) && parallel && readOnly,
				PreparationFailure: TurnFailureError{
					Message: call.Name + " has no frozen MCP policy for the provider request",
					Code:    "tool_advertisement_authority_missing", Details: map[string]any{"toolName": call.Name}, Severity: "error",
				},
			}, nil
		}
		readOnly = frozenReadOnly
		parallel = parallel && frozenReadOnly
	}
	if provenanceErr := validateFundsSubjectReferenceProvenanceV1(input, call); provenanceErr != nil {
		return toolStepScheduledCall{
			Call: call, ReadOnly: readOnly, ApprovalState: "not_required",
			Batchable:          !toolcatalogapp.IsUserInputTool(call.Name) && parallel && readOnly,
			PreparationFailure: provenanceErr,
		}, nil
	}
	approvalState := "not_required"
	if toolcatalogapp.RequiresApprovalWithHostPolicy(call.Name, input.ApprovalPolicy, input.SandboxMode, readOnly) {
		approvalState = "pending"
	}
	return toolStepScheduledCall{
		Call: call, ReadOnly: readOnly, ApprovalState: approvalState,
		Batchable: !toolcatalogapp.IsUserInputTool(call.Name) && parallel && approvalState != "pending",
	}, nil
}

func partitionScheduledReadOnlyBatchByAuthority(calls []toolStepScheduledCall) (ordinary, protected []toolStepScheduledCall) {
	ordinary = make([]toolStepScheduledCall, 0, len(calls))
	protected = make([]toolStepScheduledCall, 0, len(calls))
	for _, scheduled := range calls {
		if executiongrantapp.CallUsesCaseDataAuthority(scheduled.Call) {
			protected = append(protected, scheduled)
			continue
		}
		ordinary = append(ordinary, scheduled)
	}
	return ordinary, protected
}

func indexReadOnlyBatchMessages(calls []appmodel.PendingToolCall, messages []domainmodel.Message) (map[string]domainmodel.Message, error) {
	if len(messages) != len(calls) {
		return nil, errors.New("read-only batch result count does not match its calls")
	}
	wanted := make(map[string]bool, len(calls))
	for _, pending := range calls {
		wanted[strings.TrimSpace(pending.Call.ID)] = true
	}
	indexed := make(map[string]domainmodel.Message, len(messages))
	for _, message := range messages {
		callID := strings.TrimSpace(message.ToolCallID)
		if !wanted[callID] || callID == "" || indexed[callID].ToolCallID != "" {
			return nil, errors.New("read-only batch result identity does not match its calls")
		}
		indexed[callID] = message
	}
	return indexed, nil
}

func toolCallAdmissionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx != nil && ctx.Err() == nil {
		return ctx, func() {}
	}
	return detachedToolSettlementContext(ctx)
}

func toolCallAdmissionFailure(toolName string, err error) error {
	code := strings.TrimSpace(err.Error())
	if !strings.HasPrefix(code, "execution_grant_") && !strings.HasPrefix(code, "turn_security_") {
		code = "execution_grant_admission_rejected"
	}
	return TurnFailureError{
		Message:  strings.TrimSpace(toolName) + " could not be admitted under current host authority",
		Code:     code,
		Details:  map[string]any{"toolName": strings.TrimSpace(toolName)},
		Severity: "error",
	}
}

func settledToolReference(pending appmodel.PendingToolCall) domainsecurity.SettledToolReference {
	return domainsecurity.SettledToolReference{
		GrantID:      pending.ExecutionGrant.GrantID,
		ResultItemID: toolcatalogapp.ToolResultItemID(pending.TurnID, pending.Call.ID),
	}
}

func pendingToolCallForStep(input ToolStepInput, call domainmodel.ToolCall, itemID string, messages []domainmodel.Message, grant domainsecurity.ExecutionGrant) appmodel.PendingToolCall {
	effectiveMaxModelSteps := input.EffectiveMaxModelSteps
	prompt := strings.TrimSpace(input.Prompt)
	providerStepExact := prompt != "" && domainsecurity.ValidateLogicalEffect(input.LogicalEffect) == nil
	return appmodel.PendingToolCall{
		ThreadID:              input.ThreadID,
		TurnID:                input.TurnID,
		ProviderConfig:        input.ProviderConfig,
		ProviderID:            input.ProviderID,
		Model:                 input.Model,
		Effort:                input.Effort,
		Workspace:             input.Workspace,
		Prompt:                prompt,
		LogicalEffect:         input.LogicalEffect,
		OrdinaryWork:          input.OrdinaryWork,
		ProviderStepExact:     providerStepExact,
		CaseSourceUnavailable: input.CaseSourceUnavailable,
		OrdinaryResultInputIsolated: input.OrdinaryResultInputIsolated && providerStepExact &&
			input.LogicalEffect == domainsecurity.LogicalEffectOrdinary && input.OrdinaryWork,
		AttachmentIDs:          append([]string(nil), input.AttachmentIDs...),
		AttachmentPlanDigest:   strings.TrimSpace(input.AttachmentPlanDigest),
		WorkspaceCheckpointID:  input.WorkspaceCheckpointID,
		Mode:                   input.Mode,
		GUIPlan:                contracts.CloneMap(input.GUIPlan),
		ApprovalPolicy:         input.ApprovalPolicy,
		SandboxMode:            input.SandboxMode,
		DisableUserInput:       input.DisableUserInput,
		MaxModelSteps:          cloneOptionalStepLimit(input.MaxModelSteps),
		EffectiveMaxModelSteps: &effectiveMaxModelSteps,
		Messages:               appmodel.CloneProviderMessages(messages),
		PrivateProtocolSession: input.PrivateProtocolSession,
		AnthropicCapsule:       input.AnthropicCapsule,
		PrivateProtocolCapsules: append(
			[]*domainmodel.AnthropicThinkingCapsule(nil),
			input.PrivateProtocolCapsules...,
		),
		PrivateProtocolObserved: len(input.PrivateProtocolCapsules) > 0 || input.AnthropicCapsule != nil,
		ProviderNamespace:       domaincontinuation.CloneProviderContinuationNamespaceV1(input.ProviderNamespace),
		TerminalRecoveryKind:    input.TerminalRecoveryKind,
		Call:                    call,
		ToolCallItemID:          itemID,
		ToolScope:               append([]string(nil), input.ToolScope...),
		SubagentDepth:           input.SubagentDepth,
		SecurityContext:         input.SecurityContext,
		ExecutionGrant:          grant,
	}
}

func cloneOptionalStepLimit(input *int) *int {
	if input == nil {
		return nil
	}
	value := *input
	return &value
}
