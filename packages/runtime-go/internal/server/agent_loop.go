package server

import (
	"context"
	"errors"
	"strings"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	steeringapp "analytix.local/runtime-go/internal/app/steering"
	appthread "analytix.local/runtime-go/internal/app/thread"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
)

type runtimeAgentLoopInput struct {
	ThreadID                     string
	TurnID                       string
	Thread                       map[string]any
	Request                      startRuntimeTurnRequest
	ProviderConfig               provider.TurnConfig
	ProviderID                   string
	Model                        string
	Effort                       string
	Workspace                    string
	Attachments                  appturn.AttachmentPlan
	ApprovalPolicy               string
	SandboxMode                  string
	ToolScope                    []string
	SubagentDepth                int
	UsageSource                  string
	ParentThreadID               string
	ParentTurnID                 string
	ParentToolCallID             string
	ChildRunID                   string
	DelegatedToolManifest        *domainjob.DelegatedToolManifestV1
	SystemPrompt                 string
	SecurityContext              domainsecurity.TurnSecurityContext
	ContextEpoch                 domaincontextepoch.ProviderContext
	HistoryBeforeTurnID          string
	CurrentUserItemID            string
	HostChildCasePromptWitness   *turnstartapp.HostChildCasePromptFrozenWitnessV1
	CurrentChildCaseRecord       domainjob.Record
	ProviderContinuationRefs     []domainsecurity.SettledToolReference
	PrivateProtocolSession       *domainmodel.PrivateProtocolSession
	AnthropicCapsule             *domainmodel.AnthropicThinkingCapsule
	PrivateProtocolCapsules      []*domainmodel.AnthropicThinkingCapsule
	ProviderCallSequence         uint64
	TerminalRecoveryKind         apploop.RuntimeTerminalRecoveryKind
	CaseFundPolicy               *apploop.CaseFundAnalysisPolicy
	HostEntitySelection          apploop.HostCaseEntitySelectionV1
	RestoredProviderStep         *apploop.RuntimeProviderStep
	CaseSourceUnavailable        bool
	OrdinaryResultInputIsolated  bool
	OrdinaryLaneMessages         []domainmodel.Message
	TrustedCaseCompactionTurnIDs map[string]bool
	initialProviderProjection    *caseentityapp.ProviderIngressProjectionV1
	initialProviderAliases       []domaincaseentity.ModelEntityAliasV1
}

type runtimeAgentLoopResult = apploop.RuntimeAgentLoopResult

type runtimePendingToolCall = appmodel.PendingToolCall

type settledNativeFundsSourceInvalidator interface {
	InvalidateHostFundsSourceAfterSettledNativeFailure()
}

func (h *runtimeServerHandler) runRuntimeAgentLoop(ctx context.Context, input runtimeAgentLoopInput) (runtimeAgentLoopResult, error) {
	var err error
	input, err = h.prepareRuntimeChildCaseDelegationV1(ctx, input)
	if err != nil {
		return runtimeAgentLoopResult{}, err
	}
	h.clearRuntimeReadRecords(input.ThreadID)
	resolvedIngress := apploop.ResolveInitialProviderIngressV1(apploop.ResolveInitialProviderIngressInputV1{
		Projection: input.initialProviderProjection, Fallback: input.Request.Prompt,
		Policy: input.CaseFundPolicy, SourceUnavailable: input.CaseSourceUnavailable,
		HostEntitySelection: input.HostEntitySelection,
	})
	consumedIngress := resolvedIngress.Consumption
	if len(input.initialProviderAliases) != 0 {
		if !resolvedIngress.HostEntitySelection.AvailableV1() {
			return runtimeAgentLoopResult{}, errors.New("case delegation host selection is unavailable")
		}
		consumedIngress.Aliases = append([]domaincaseentity.ModelEntityAliasV1(nil), input.initialProviderAliases...)
	}
	initialProviderText := consumedIngress.Text
	input.initialProviderProjection = nil
	input.CaseFundPolicy = resolvedIngress.Policy
	input.CaseSourceUnavailable = resolvedIngress.SourceUnavailable
	input.HostEntitySelection = resolvedIngress.HostEntitySelection
	caseFundPolicy, boundary := h.resolveRuntimeCaseFundAnalysisPolicy(&input)
	ordinaryHistory := appthread.TypedOrdinaryProviderHistoryBeforeTurnV1(
		h.publicProjector, input.Thread, input.HistoryBeforeTurnID, input.TrustedCaseCompactionTurnIDs,
	)
	caseTaskContinuationHistory := appmodel.CaseTaskContinuationProviderHistoryBeforeTurnV1(
		input.Thread, input.HistoryBeforeTurnID, input.TrustedCaseCompactionTurnIDs,
	)
	initial := apploop.PrepareInitialProviderMessagesV1(apploop.InitialProviderMessagesInputV1{
		SystemPrompt: input.SystemPrompt, ProviderID: input.ProviderID, Model: input.Model,
		Policy: caseFundPolicy, Boundary: boundary,
		History: appmodel.ProviderHistoryBeforeTurnWithSteeringAuthority(
			input.Thread, input.HistoryBeforeTurnID, h.steeringAuthority,
		),
		OrdinaryHistory:             ordinaryHistory,
		CaseTaskContinuationHistory: caseTaskContinuationHistory,
		Background:                  h.runtimeBackgroundJobContextMessages(input.ThreadID, input.SubagentDepth, input.SecurityContext),
		UserPrompt:                  initialProviderText, AttachmentPlanDigest: input.Attachments.PlanDigest,
		ContextEpoch: input.ContextEpoch, CaseSensitive: domainsecurity.TurnSecurityContextIsCaseSensitive(input.SecurityContext),
	})
	if initial.Boundary != "" {
		return runtimeAgentLoopResult{
			AssistantText: initial.Boundary, CaseSourceUnavailable: caseFundPolicy.SourceUnavailable,
		}, nil
	}
	if input.HostChildCasePromptWitness != nil {
		if err := privacyprojectionapp.BindHostChildCasePromptCurrentAttemptV1(
			privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1{
				Witness: input.HostChildCasePromptWitness, SecurityContext: input.SecurityContext,
				CurrentRecord: input.CurrentChildCaseRecord, Thread: input.Thread,
				ThreadID: input.ThreadID, TurnID: input.TurnID, UserItemID: input.CurrentUserItemID,
				Aliases: consumedIngress.Aliases, Messages: &initial.Messages,
			},
		); err != nil {
			return runtimeAgentLoopResult{}, err
		}
	} else if err := apploop.BindInitialProviderIngressAliasesV1(input.SecurityContext, consumedIngress, &initial); err != nil {
		return runtimeAgentLoopResult{}, err
	}
	input.SystemPrompt = initial.BaseSystemPrompt
	input.OrdinaryResultInputIsolated = initial.OrdinaryResultInputIsolated
	input.OrdinaryLaneMessages = initial.OrdinaryLaneMessages
	return h.runRuntimeAgentLoopWithMessages(ctx, input, initial.Messages)
}

func (h *runtimeServerHandler) runRuntimeAgentLoopWithMessages(ctx context.Context, input runtimeAgentLoopInput, messages []provider.Message) (runtimeAgentLoopResult, error) {
	if strings.TrimSpace(input.ChildRunID) != "" && !input.HostEntitySelection.AvailableV1() {
		var err error
		input, err = h.prepareRuntimeChildCaseDelegationV1(ctx, input)
		if err != nil {
			return runtimeAgentLoopResult{}, err
		}
	}
	loopEvents := apploop.NewRuntimeEventRecorderWithTrace(h.store, runtimeThreadTraceEnabled())
	providerAttempts := h.runtimeProviderAttempts(input)
	caseFundPolicy, boundary := h.resolveRuntimeCaseFundAnalysisPolicy(&input)
	additionalCaseDataEffect := input.Attachments.UsesCaseDataAuthority() || len(input.initialProviderAliases) != 0
	if boundary != "" {
		return runtimeAgentLoopResult{
			AssistantText: boundary, CaseSourceUnavailable: caseFundPolicy.SourceUnavailable,
		}, nil
	}
	baseSystemPrompt := strings.TrimSpace(input.SystemPrompt)
	if baseSystemPrompt == "" {
		baseSystemPrompt = apploop.RuntimeProviderBaseSystemPromptV1(messages, caseFundPolicy)
	}
	if baseSystemPrompt == "" {
		baseSystemPrompt = apploop.DefaultRuntimeSystemPrompt(input.ProviderID, input.Model)
	}
	input.SystemPrompt = baseSystemPrompt
	maxSteps := h.resolveRuntimeModelStepLimit(input.Thread, input.Request)
	return apploop.RunRuntimeAgentLoop(ctx, apploop.RuntimeRunnerInput{
		ThreadID: input.ThreadID, TurnID: input.TurnID,
		ProviderConfig: input.ProviderConfig, ProviderID: input.ProviderID, Model: input.Model, Effort: input.Effort,
		Workspace: input.Workspace, Prompt: input.Request.Prompt, Mode: input.Request.Mode, GUIPlan: cloneMap(input.Request.GUIPlan),
		DisableUserInput: input.Request.DisableUserInput, MaxModelSteps: input.Request.MaxModelSteps,
		EffectiveMaxModelSteps: maxSteps, WorkspaceCheckpointID: input.Request.WorkspaceCheckpointID,
		AttachmentIDs: append([]string(nil), input.Attachments.IDs...), AttachmentPlanDigest: input.Attachments.PlanDigest,
		HasAttachments: !input.Attachments.Empty(), ApprovalPolicy: input.ApprovalPolicy, SandboxMode: input.SandboxMode,
		NormalizedSandboxMode: normalizeSandboxMode(input.SandboxMode),
		ToolScope:             append([]string(nil), input.ToolScope...), SubagentDepth: input.SubagentDepth,
		UsageSource: input.UsageSource, ChildRunID: input.ChildRunID, DelegatedToolManifest: input.DelegatedToolManifest,
		SystemPrompt: input.SystemPrompt, FallbackSystemPrompt: apploop.DefaultRuntimeSystemPrompt(input.ProviderID, input.Model),
		SecurityContext: input.SecurityContext, HostEntitySelection: input.HostEntitySelection,
		ProviderContinuationRefs: input.ProviderContinuationRefs,
		PrivateProtocolSession:   input.PrivateProtocolSession, AnthropicCapsule: input.AnthropicCapsule,
		PrivateProtocolCapsules: append([]*domainmodel.AnthropicThinkingCapsule(nil), input.PrivateProtocolCapsules...),
		ProviderCallSequence:    input.ProviderCallSequence, TerminalRecoveryKind: input.TerminalRecoveryKind,
		CaseFundPolicy: caseFundPolicy, AdditionalCaseDataEffect: additionalCaseDataEffect, Messages: messages,
		RestoredProviderStep:        input.RestoredProviderStep,
		CaseSourceUnavailable:       input.CaseSourceUnavailable,
		OrdinaryResultInputIsolated: input.OrdinaryResultInputIsolated && !additionalCaseDataEffect,
		OrdinaryLaneMessages:        input.OrdinaryLaneMessages,
		OutputTokenBudget:           input.Request.InternalOutputTokenBudget,
		ProviderCustomRequestShape:  provider.CustomEndpointRequestShape(input.ProviderConfig.BaseURL),
		RequiredFinalToolName: func() string {
			if input.SubagentDepth == 1 && toolcatalogapp.IsForegroundSubmitOnlyScope(input.ToolScope) {
				return toolcatalogapp.ForegroundSubmitToolName
			}
			return ""
		}(),
	}, apploop.RuntimeRunnerDependencies{
		Provider: h.provider, PendingWork: h.pendingWork, Events: loopEvents,
		ResolveProviderIntent: providerAttempts.CurrentIntent,
		PrepareProviderStep: func(stepCtx context.Context, step apploop.RuntimeProviderStep) (apploop.RuntimeProviderStep, error) {
			if !step.UsesFundsDataAuthority() {
				return step, nil
			}
			if err := h.nativeAuthority.PrepareCaseTurn(stepCtx, input.SecurityContext); err != nil {
				if errors.Is(err, nativecomponentapp.ErrHealthUnavailableSettled) {
					if invalidator, ok := h.mcp.(settledNativeFundsSourceInvalidator); ok {
						invalidator.InvalidateHostFundsSourceAfterSettledNativeFailure()
					}
					if step.OrdinaryWork {
						return apploop.RuntimeProviderStep{
							Prompt: step.Prompt, LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
							CaseSourceUnavailable: true,
						}, nil
					}
					return apploop.RuntimeProviderStep{}, apploop.TurnFailureError{
						Message: apploop.CaseFundSourceUnavailableAnswer(), Code: "tool_source_unavailable", Severity: "error",
					}
				}
				return apploop.RuntimeProviderStep{}, err
			}
			return step, nil
		},
		ResolveToolCatalog: func(planActive bool, step apploop.RuntimeProviderStep) apploop.RuntimeRunnerToolCatalog {
			stepPolicy, stepScope := apploop.ResolveRuntimeProviderStepPolicyV1(apploop.RuntimeProviderStepPolicyInputV1{
				Step: step, HasCaseBinding: filestore.WorkspaceHasAnalytixCaseBinding(input.Workspace),
				ToolSource: h.mcp, SecurityContext: input.SecurityContext, RequestedScope: input.ToolScope,
				Delegated: input.SubagentDepth > 0 || input.DelegatedToolManifest != nil,
			})
			if step.CaseSourceUnavailable && step.OrdinaryEffect() {
				stepPolicy = apploop.CaseFundSourceUnavailablePolicyV1(true)
			}
			promptRoute := h.runtimePromptRouteForPrompt(step.Prompt, stepScope, input.SubagentDepth > 0, planActive)
			advertisements := toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(h.mcp, input.SecurityContext)
			schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPAdvertisements(
				input.Request.DisableUserInput, stepScope, input.SubagentDepth > 0, planActive, step.Prompt,
				h.runtimeGoalToolsActive(input.ThreadID, step.Prompt, stepScope), advertisements,
			)
			if len(schemas) == 0 && input.SubagentDepth == 0 && input.DelegatedToolManifest == nil &&
				step.OrdinaryEffect() && appmodel.ContinuationSourceReferencesRequiredV1(input.Thread) {
				// Do not widen a delegated or explicit tool scope to make history
				// convenient. This narrow read is still guarded at execution.
				for _, schema := range toolcatalogapp.BuiltinToolSchemas(toolcatalogapp.BuiltinToolSchemaInput{}) {
					if schema.Name == "read_task_history" {
						schemas = toolcatalogapp.FilterToolSchemas([]domainmodel.ToolSchema{schema}, stepScope)
						break
					}
				}
			}
			schemas = toolcatalogapp.SecurityScopedToolSchemas(input.SecurityContext, input.SubagentDepth, schemas)
			advertisements, schemas, _ = privacyprojectionapp.ProviderCatalogForLogicalEffectV1(
				step.LogicalEffect, advertisements, schemas,
			)
			return apploop.RuntimeRunnerToolCatalog{
				PromptRoute: promptRoute,
				SystemPrompt: apploop.RuntimeProviderSystemPromptV1(
					baseSystemPrompt, stepPolicy,
				),
				Advertisements: advertisements,
				Schemas:        schemas,
			}
		},
		PromoteSteering: func(promotionCtx context.Context) (apploop.RuntimeSteeringBatch, error) {
			return h.promoteRuntimeSteeringForProvider(promotionCtx, input.SecurityContext)
		},
		AcquireCandidateTerminal: func(candidateCtx context.Context, step apploop.RuntimeProviderStep) (context.Context, func(), error) {
			return h.runtimePublicationAuthority().AcquireCurrentCandidateTerminalForLoop(
				candidateCtx, input.SecurityContext, step.UsesCaseDataAuthority(),
			)
		},
		PauseChild: func(pauseCtx context.Context) error {
			return h.pauseRuntimeBackgroundChildIfRequested(pauseCtx, input.ChildRunID)
		},
		AcquireProviderAttempt: func(attemptCtx context.Context, _ int, step apploop.RuntimeProviderStep) (context.Context, func(), error) {
			return apploop.AcquireProviderAttemptEffect(
				attemptCtx, input.SecurityContext, step.UsesCaseDataAuthority(),
				h.runtimeSubagentState().AcquireContextEffectForAuthority,
				func(effectCtx context.Context) error {
					return h.validateCurrentProviderAttempt(
						effectCtx,
						input,
						step.UsesCaseDataAuthority(),
						step.UsesFundsDataAuthority(),
					)
				},
			)
		},
		PrepareProviderAttempt: providerAttempts.Prepare,
		MaterializeCreatePlan: func(materializeCtx context.Context, currentMessages []domainmodel.Message, planText string, privateProtocolObserved bool) (apploop.RuntimeMaterializedPlan, error) {
			output, isError, nextMessages, continuationRef, err := h.materializeRuntimeCreatePlanText(
				materializeCtx,
				input,
				currentMessages,
				planText,
				privateProtocolObserved,
			)
			return apploop.RuntimeMaterializedPlan{Output: output, IsError: isError, Messages: nextMessages, ContinuationRef: continuationRef}, err
		},
		BackgroundMemoryCount: func() int {
			return len(h.runtimeBackgroundJobContextMessages(input.ThreadID, input.SubagentDepth, input.SecurityContext))
		},
		ToolDriver: h.runtimeToolStepDriver(loopEvents),
	})
}

func runtimeProviderExecutionInputV1(input runtimeAgentLoopInput) provider.TurnExecutionInput {
	return provider.TurnExecutionInput{
		RequestProviderID: strings.TrimSpace(input.Request.ProviderID), RequestModel: strings.TrimSpace(input.Request.Model),
		RequestEndpointFormat: strings.TrimSpace(input.Request.EndpointFormat), RequestEffort: input.Effort,
		ThreadProviderID: stringField(input.Thread, "providerId"), ThreadModel: stringField(input.Thread, "model"),
		InternalUsageSource: input.UsageSource, InternalChildRunID: input.ChildRunID,
	}
}

func runtimeProviderExecutionMatchesIntentV1(config provider.TurnConfig, request domainmodel.Request) bool {
	return strings.TrimSpace(config.ProviderID) == strings.TrimSpace(request.ProviderID) &&
		strings.TrimSpace(config.Family) == strings.TrimSpace(request.Family) &&
		strings.TrimSpace(config.EndpointFormat) == strings.TrimSpace(request.EndpointFormat) &&
		strings.TrimSpace(config.BaseURL) == strings.TrimSpace(request.BaseURL) &&
		strings.TrimSpace(config.ProxyURL) == strings.TrimSpace(request.ProxyURL) &&
		strings.TrimSpace(config.Model) == strings.TrimSpace(request.Model) &&
		config.ReasoningEffort == request.ReasoningEffort &&
		strings.TrimSpace(config.ReasoningProtocol) == strings.TrimSpace(request.ReasoningProtocol)
}

func bindRuntimeProviderExecutionV1(request domainmodel.Request, execution provider.TurnExecutionResult) domainmodel.Request {
	request.ProviderID = execution.ProviderID
	request.Family = execution.Config.Family
	request.EndpointFormat = execution.Config.EndpointFormat
	request.BaseURL = execution.Config.BaseURL
	request.ProxyURL = execution.Config.ProxyURL
	request.APIKey = execution.Config.APIKey
	request.Model = execution.Model
	request.ReasoningEffort = execution.Effort
	request.ReasoningProtocol = execution.Config.ReasoningProtocol
	request.Pricing = execution.Config.Pricing
	return request
}

func (h *runtimeServerHandler) resolveRuntimeCaseFundAnalysisPolicy(
	input *runtimeAgentLoopInput,
) (apploop.CaseFundAnalysisPolicy, string) {
	if input == nil {
		return apploop.CaseFundAnalysisPolicy{}, ""
	}
	policy, scope, boundary := apploop.ResolveFrozenCaseFundPolicyV1(
		&input.CaseFundPolicy,
		func() apploop.CaseFundAnalysisPolicy {
			return apploop.CaseFundAnalysisPolicyForTurnV1(
				filestore.WorkspaceHasAnalytixCaseBinding(input.Workspace),
				input.Request.Prompt, input.Request.DisplayText, input.Request.FileReferences,
				h.mcp, input.SecurityContext, false,
			)
		},
		input.ToolScope,
		input.SubagentDepth > 0 || input.DelegatedToolManifest != nil,
	)
	input.ToolScope = scope
	return policy, boundary
}

func (h *runtimeServerHandler) validateCurrentProviderAttempt(
	ctx context.Context,
	input runtimeAgentLoopInput,
	caseDataEffect, fundsDataEffect bool,
) error {
	if h == nil {
		return apploop.TurnFailureError{Message: "provider authority is unavailable", Code: "turn_security_authority_unavailable", Severity: "error"}
	}
	return apploop.ValidateProviderAttemptAuthority(apploop.ProviderAttemptAuthorityInput{
		OperationContext: ctx, Authority: h.turnSecurity, SecurityContext: input.SecurityContext,
		Workspace: input.Workspace, CaseDataEffect: caseDataEffect,
		FundsDataEffect: fundsDataEffect, CaseLineage: h.caseThreads, Source: h.mcp,
	})
}

func (h *runtimeServerHandler) promoteRuntimeSteeringForProvider(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) (apploop.RuntimeSteeringBatch, error) {
	if h == nil {
		return apploop.RuntimeSteeringBatch{}, apploop.TurnFailureError{Message: "steering promotion authority is unavailable", Code: "turn_security_authority_unavailable", Severity: "error"}
	}
	pending, err := h.store.PendingSteeringEntriesForContext(
		securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest,
	)
	if err != nil {
		return apploop.RuntimeSteeringBatch{}, err
	}
	// No steering effect exists to promote at this observation point. A steer
	// admitted after this read races exactly like one admitted after a completed
	// promotion and is consumed at the next loop boundary.
	if len(pending) == 0 {
		return apploop.RuntimeSteeringBatch{}, nil
	}
	batch, err := steeringapp.PromoteCurrentForProviderEffectBatch(ctx, securityContext, steeringapp.EffectAwareDependencies{
		Store: h.store, Jobs: h.jobs, AcquireContextEffect: h.runtimeSubagentState().AcquireContextEffectForAuthority,
		Authority: h.turnSecurity, VerifyAuthority: h.steeringAuthority.Verify,
		RecordEvent: h.recordRuntimeBestEffortEvent,
	})
	if message, code, projected := steeringapp.ProviderFailureProjection(err); projected {
		return apploop.RuntimeSteeringBatch{}, apploop.TurnFailureError{Message: message, Code: code, Severity: "error"}
	}
	return apploop.RuntimeSteeringBatch{
		Messages: batch.Messages, Prompt: batch.Prompt,
		LogicalEffect: batch.LogicalEffect, OrdinaryWork: batch.OrdinaryWork,
	}, err
}

func (h *runtimeServerHandler) runtimeToolStepDriver(events *apploop.RuntimeEventRecorder) apploop.ToolStepDriverFuncs {
	return apploop.ToolStepDriverFuncs{
		AcquireToolCallAdmissionFunc: h.acquireRuntimeToolCallAdmission,
		PersistToolCallReadyFunc:     h.persistToolCallReady, PersistToolResultAndMessageFunc: h.persistRuntimeToolResultAndMessage,
		RequestUserInputFunc: h.requestRuntimeUserInput,
		RequestApprovalFunc:  h.requestRuntimeApproval, ToolPolicyFunc: h.runtimeToolPolicy, PreflightFunc: h.runtimeToolPreflight,
		ExecuteAndSettleFunc: h.executeAndSettleRuntimeTool, ExecuteAuthorizedBatchFunc: h.executeRuntimeAuthorizedReadOnlyToolBatch,
		SettleToolBatchAuthorityFunc: func(ctx context.Context, workID string, securityContext domainsecurity.TurnSecurityContext, calls []runtimePendingToolCall, status, reason string) error {
			_, err := h.pendingWork.CompleteToolBatchWithGate(ctx, h.runtimeSubagentState().EffectGate(), workID, securityContext, calls, status, reason)
			return err
		},
		RecordLoopGuardFunc: events.LoopGuard,
	}
}
