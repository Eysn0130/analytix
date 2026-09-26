package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	appgoal "analytix.local/runtime-go/internal/app/goal"
	apploop "analytix.local/runtime-go/internal/app/loop"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	appthread "analytix.local/runtime-go/internal/app/thread"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	appusage "analytix.local/runtime-go/internal/app/usage"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
	"analytix.local/runtime-go/internal/research"
)

func (h *runtimeServerHandler) handleThreadTurns(w http.ResponseWriter, r *http.Request, threadID string) {
	httpapi.StartTurnHandlers{Control: h.runtimeControl()}.HandleThreadTurns(w, r, threadID)
}

type startRuntimeTurnRequest = controlapp.StartTurnRequest

func (h *runtimeServerHandler) startRuntimeTurn(ctx context.Context, threadID string, request startRuntimeTurnRequest) (response map[string]any, err error) {
	if h.caseThreads != nil && h.caseThreads.RestartPreservesThreadV1(threadID) {
		return nil, casethreadapp.ErrRestartPreserved
	}
	if err := domainmodel.ValidateReasoningEffortV1(request.ReasoningEffort); err != nil {
		return nil, err
	}
	autoContinueService := h.runtimeBackgroundDeliveryService()
	reservedTurnID, reservedTurnNumber, err := autoContinueService.ReservedAutoContinueTurnIdentityV1(request, threadID)
	if err != nil {
		return nil, err
	}
	childTransitionAuthority, err := h.runtimeChildTransitionAuthority(request, threadID)
	if err != nil {
		return nil, err
	}
	if childTransitionAuthority != nil {
		if reservedTurnID != "" {
			return nil, errors.New("child turn cannot consume a parent auto-continue allocation")
		}
		reservedTurnID, reservedTurnNumber, err = h.consumeRuntimeChildTurnV1(ctx, childTransitionAuthority)
		if err != nil {
			return nil, err
		}
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil {
		return nil, err
	}
	finishOperation, admitted := h.runtimeControl().BeginTurnOperation()
	if !admitted {
		return nil, controlapp.ErrRuntimeShuttingDown
	}
	operationOwned := true
	defer func() {
		if operationOwned {
			finishOperation()
		}
	}()
	postAppendGuardArmed := false
	var postAppendFailureInput runtimeAgentLoopInput
	var securityTransition *subagentapp.SecurityContextTransition
	var releaseThreadTransition func()
	turnCancelRegistered := false
	turnCancelTransferred := false
	registeredTurnID := ""
	var cancelTurn context.CancelFunc
	defer func() {
		// A failed post-append turn is terminally closed while its transition
		// writer is still held. Only then may another context transition start.
		if err != nil && postAppendGuardArmed {
			if recordErr := h.recordRuntimeTurnFailureForInput(postAppendFailureInput, err); recordErr != nil {
				err = errors.Join(err, fmt.Errorf("post-append terminal closure failed: %w", recordErr))
			}
		}
		if securityTransition != nil {
			securityTransition.Abort()
		}
		if releaseThreadTransition != nil {
			releaseThreadTransition()
		}
		if turnCancelRegistered && !turnCancelTransferred {
			h.runtimeControl().UnregisterTurnCancel(threadID, registeredTurnID)
			if cancelTurn != nil {
				cancelTurn()
			}
		}
	}()
	releasePreparation, err := h.runtimeControl().PrepareForeground(ctx, threadID)
	if err != nil {
		return nil, err
	}
	defer releasePreparation()
	thread, startBaselineDigest, err := turnstartapp.PrepareStartBaselineV1(turnstartapp.PrepareStartBaselineInputV1{
		Context: ctx, Store: h.store, Compactor: h.runtimeThreadService(), ThreadID: threadID,
		Prompt: request.Prompt, MainThread: request.InternalSubagentDepth == 0 && strings.TrimSpace(request.InternalChildRunID) == "",
		Reserved: reservedTurnID != "",
		ValidateReserved: func(thread map[string]any) error {
			if childTransitionAuthority != nil {
				if stringField(thread, "id") != childTransitionAuthority.ChildThreadID {
					return turnstartapp.ErrBaselineConflict
				}
				return turnstartapp.ValidateUnoccupiedTurnIDV1(thread, reservedTurnID)
			}
			return autoContinueService.ValidateReservedAutoContinueFrozenV1(request, thread, reservedTurnID)
		},
		ContextWindowTokens: func(thread map[string]any) int {
			return h.providerConfig.TurnConfig(firstNonEmptyString(request.ProviderID, stringField(thread, "providerId")), firstNonEmptyString(request.Model, stringField(thread, "model"))).ContextWindowTokens
		},
	})
	if err != nil {
		return nil, err
	}
	trustedCaseCompactionTurnIDs := appthread.TrustedCaseCompactionTurnIDsV1(thread, h.caseThreads, h.store)
	turnID := reservedTurnID
	turnNumber := reservedTurnNumber
	if turnID == "" {
		turnID, turnNumber = h.nextRuntimeTurnIdentity()
	}
	if turnID == "" || turnNumber <= 0 {
		return nil, errors.New("runtime turn identity is unavailable")
	}

	threadWorkspace := strings.TrimSpace(stringField(thread, "workspace"))
	turnContext := ctx
	if request.Async {
		turnContext = context.Background()
	}
	turnContext, cancelTurn = context.WithCancel(turnContext)
	if registerErr := h.runtimeControl().RegisterTurnCancelWithError(threadID, turnID, cancelTurn); registerErr != nil {
		cancelTurn()
		return nil, registerErr
	}
	turnCancelRegistered = true
	registeredTurnID = turnID
	issuedAt := time.Now().UTC()
	var startPlan appturn.StartPlan
	var providerConfig provider.TurnConfig
	var attachments appturn.AttachmentPlan
	providerID := strings.TrimSpace(request.ProviderID)
	model := strings.TrimSpace(request.Model)
	effort := request.ReasoningEffort
	approvalPolicy := ""
	sandboxMode := ""
	var admissionErr error
	var preProviderCaseFundPolicy *apploop.CaseFundAnalysisPolicy
	now := issuedAt.Format(time.RFC3339Nano)
	caseBindingReader := filestore.CaseBindingReader{}
	queuedLexicalCaseRisk, queuedProtectedCaseData := runtimeTaskJobQueuedSteerRiskSignals(childTransitionAuthority)
	securityAuthority := h.turnSecurity
	securityAuthority.RiskIntent = request.RiskIntent
	caseAdmissionText := apploop.CaseAdmissionTextV1(request.Prompt, request.DisplayText, request.FileReferences)
	securityAuthority.LexicalCaseRisk = queuedLexicalCaseRisk || apploop.PromptRequiresCaseRiskAdmission(caseAdmissionText)
	securityAuthority.ProtectedCaseData = queuedProtectedCaseData || domainsecurity.ContainsProtectedCaseFactCandidate(caseAdmissionText)
	securityAuthority.TrustedCaseThread = h.caseThreads != nil && h.caseThreads.IsCaseThread(threadID)
	attachmentCarriesCaseBinding, err := h.runtimeAttachmentPipeline().Planner.PreflightCaseRisk(
		turnContext,
		request.AttachmentIDs,
		strings.TrimSpace(request.RiskIntent) == domainsecurity.RiskClassCase ||
			securityAuthority.LexicalCaseRisk ||
			securityAuthority.ProtectedCaseData ||
			securityAuthority.TrustedCaseThread,
	)
	if err != nil {
		return nil, err
	}
	securityAuthority.ContextChangingInput = attachmentCarriesCaseBinding
	casePolicyPrompt := request.Prompt
	casePolicyDisplayText := request.DisplayText
	casePolicyFileReferences, _ := contracts.CloneValue(request.FileReferences).([]any)
	hostChildCasePrompt := turnstartapp.HostChildCasePromptCandidateV1(childTransitionAuthority, request.Prompt)
	validatedHostChildCasePrompt := ""
	var hostChildCasePromptWitness *turnstartapp.HostChildCasePromptFrozenWitnessV1
	request.Prompt, request.DisplayText, request.FileReferences, request.GUIPlan = privacyprojectionapp.ProjectTurnContent(
		request.Prompt, request.DisplayText, request.FileReferences, request.GUIPlan,
	)
	delegatedScope, delegatedToolManifest, err := turnstartapp.ResolveDelegatedToolAuthority(childTransitionAuthority, request.InternalToolScope)
	if err != nil {
		return nil, err
	}
	request.InternalToolScope = delegatedScope
	if childTransitionAuthority == nil {
		releaseThreadTransition, err = h.runtimeControl().ReserveThreadTransition(threadID)
		if err == nil {
			securityTransition, err = turnstartapp.BeginSecurityTransitionWithBarrier(
				turnContext, h.runtimeSubagentState(), securityAuthority.Identity, principal,
				caseBindingReader, threadID, threadWorkspace,
				func() error { return h.quiesceRuntimeThreadForTurnStart(turnContext, threadID, turnID) }, 5*time.Second,
			)
		}
	} else {
		securityTransition, err = turnstartapp.BeginSecurityTransition(
			turnContext, h.runtimeSubagentState(), securityAuthority.Identity, principal,
			caseBindingReader, h.mcp, thread, threadID, turnID, threadWorkspace, time.Now().UTC(),
			childTransitionAuthority,
		)
	}
	if err != nil {
		return nil, err
	}
	if releaseThreadTransition != nil {
		latestThread, latestDigest, readErr := h.store.ReadThreadStartBaseline(threadID)
		if readErr != nil {
			return nil, readErr
		}
		if latestThread == nil || strings.TrimSpace(stringField(latestThread, "id")) != threadID {
			return nil, errors.New("turn start post-quiescence thread authority is unavailable")
		}
		latestWorkspace := strings.TrimSpace(stringField(latestThread, "workspace"))
		latestWorkspaceRealPath, workspaceErr := caseBindingReader.WorkspaceRealPath(latestWorkspace)
		expectedWorkspaceRealPath, expectedErr := securityTransition.ScopeWorkspaceRealPath()
		if workspaceErr != nil || expectedErr != nil || latestWorkspaceRealPath != expectedWorkspaceRealPath {
			return nil, errors.New("turn start workspace changed during authority quiescence")
		}
		thread, startBaselineDigest, threadWorkspace = latestThread, latestDigest, latestWorkspace
	}
	previousTitle := stringField(thread, "title")
	hostContext, cancelContextCommit := newHostAuthorityContext()
	preFreezeResearchPrompt := apploop.PreFreezeHostEffectPromptV1(
		casePolicyPrompt, casePolicyDisplayText, casePolicyFileReferences, filestore.WorkspaceHasAnalytixCaseBinding(threadWorkspace) ||
			strings.TrimSpace(securityAuthority.RiskIntent) == domainsecurity.RiskClassCase || securityAuthority.LexicalCaseRisk || securityAuthority.ProtectedCaseData || securityAuthority.ContextChangingInput || securityAuthority.TrustedCaseThread,
	)
	prepareWorkspaceBeforeFreeze := research.WorkspacePreparationBeforeFreezeV1(preFreezeResearchPrompt)
	commitResult, commitErr := turnstartapp.CommitPreparedTurn(turnstartapp.PreparedCommitInput{
		Context: turnContext, HostContext: hostContext, Store: h.store, CaseAuthority: h.caseThreads, Transition: securityTransition,
		Principal: principal, Reader: caseBindingReader, SecurityAuthority: securityAuthority, Source: h.mcp,
		InitialBaselineDigest: startBaselineDigest, PostBarrierThread: thread, PostBarrierDigest: startBaselineDigest, ReusePostBarrierBaseline: childTransitionAuthority == nil, ThreadID: threadID, TurnID: turnID,
		Workspace: threadWorkspace, IssuedAt: issuedAt, CommitAt: time.Now().UTC(), Timeout: 5 * time.Second,
		PrepareWorkspaceBeforeFreeze: prepareWorkspaceBeforeFreeze,
		ValidateFrozen: func(validateCtx context.Context, frozen domainsecurity.TurnSecurityContext) error {
			if validateErr := h.validateRuntimeChildTransitionFrozen(validateCtx, childTransitionAuthority, frozen); validateErr != nil {
				return validateErr
			}
			if validateErr := autoContinueService.ValidateReservedAutoContinueFrozenV1(request, thread, turnID); validateErr != nil {
				return validateErr
			}
			if hostChildCasePrompt == "" {
				return nil
			}
			var witnessErr error
			validatedHostChildCasePrompt, hostChildCasePromptWitness, witnessErr = turnstartapp.NewHostChildCasePromptFrozenWitnessV1(
				childTransitionAuthority, frozen, turnID, hostChildCasePrompt,
			)
			return witnessErr
		},
		RecordFactory: func(currentThread map[string]any, preparation turnstartapp.SecurityPreparation) (turnstartapp.SecurityRecords, string, error) {
			if hostChildCasePrompt != "" {
				if validatedHostChildCasePrompt == "" || validatedHostChildCasePrompt != hostChildCasePrompt {
					return turnstartapp.SecurityRecords{}, "", errors.New("current host child case prompt authority is invalid")
				}
			}
			providerID = strings.TrimSpace(request.ProviderID)
			model = strings.TrimSpace(request.Model)
			effort = request.ReasoningEffort
			endpointFormat := strings.TrimSpace(request.EndpointFormat)
			attachments = appturn.AttachmentPlan{}
			preProviderCaseFundPolicy = nil
			if preparation.ExecutionReady {
				candidate := apploop.CaseFundAnalysisPolicyForTurnV1(
					filestore.WorkspaceHasAnalytixCaseBinding(threadWorkspace),
					casePolicyPrompt, casePolicyDisplayText, casePolicyFileReferences,
					h.mcp, preparation.SecurityContext, false,
				)
				if candidate.Active &&
					domainsecurity.TurnSecurityContextAllowsCaseEvidence(preparation.SecurityContext) &&
					!preparation.SourceReady {
					ordinaryPrompt := candidate.OrdinaryPrompt
					candidate = apploop.CaseFundSourceUnavailablePolicyV1(candidate.OrdinaryWorkRequested)
					candidate.OrdinaryPrompt = ordinaryPrompt
				}
				candidate = apploop.CaseFundBoundaryAuthorityPolicyForCurrentTurnV1(candidate,
					domainsecurity.TurnSecurityContextIsBoundaryOnly(preparation.SecurityContext), strings.TrimSpace(securityAuthority.RiskIntent) == domainsecurity.RiskClassCase || securityAuthority.LexicalCaseRisk || securityAuthority.ProtectedCaseData || securityAuthority.ContextChangingInput, len(request.AttachmentIDs) > 0, filestore.WorkspaceHasAnalytixCaseBinding(threadWorkspace), request.Prompt, request.DisplayText)
				candidate, scopedTools, _ := apploop.ResolveCaseFundToolScopeV1(
					candidate,
					request.InternalToolScope,
					request.InternalSubagentDepth > 0 || delegatedToolManifest != nil,
				)
				request.InternalToolScope = scopedTools
				if candidate.Active {
					preProviderCaseFundPolicy = &candidate
				}
				if !candidate.MustReturnBoundaryBeforeProvider() {
					execution, resolveErr := h.resolveRuntimeTurnIntent(turnContext, provider.TurnExecutionInput{
						RequestProviderID: providerID, RequestModel: model, RequestEndpointFormat: request.EndpointFormat,
						RequestEffort: effort, ThreadProviderID: stringField(currentThread, "providerId"),
						ThreadModel: stringField(currentThread, "model"), InternalUsageSource: request.InternalUsageSource,
						InternalChildRunID: request.InternalChildRunID,
					})
					if resolveErr != nil {
						admissionErr = fmt.Errorf("resolve turn provider: %w", resolveErr)
					} else {
						providerConfig = execution.Config
						providerID, model, effort = execution.ProviderID, execution.Model, execution.Effort
						endpointFormat = providerConfig.EndpointFormat
						if !domainsecurity.TurnSecurityContextIsBoundaryOnly(preparation.SecurityContext) {
							planned, attachmentErr := (appturn.AttachmentPlanner{
								Store: h.attachments, Owners: h.attachmentAccess.Owners,
							}).Plan(turnContext, appturn.AttachmentPlanInput{
								IDs: request.AttachmentIDs, SecurityContext: preparation.SecurityContext,
								ModelInputModalities: providerConfig.InputModalities, ModelMessageParts: providerConfig.MessageParts,
							})
							if attachmentErr != nil {
								admissionErr = fmt.Errorf("resolve turn attachments: %w", attachmentErr)
							} else {
								attachments = planned
							}
						}
					}
				}
				if admissionErr != nil {
					providerID = firstNonEmptyString(providerID, stringField(currentThread, "providerId"))
					model = firstNonEmptyString(model, stringField(currentThread, "model"))
					effort = firstNonEmptyString(effort, stringField(currentThread, "reasoningEffort"))
				}
			} else {
				providerID = firstNonEmptyString(providerID, stringField(currentThread, "providerId"))
				model = firstNonEmptyString(model, stringField(currentThread, "model"))
				effort = firstNonEmptyString(effort, stringField(currentThread, "reasoningEffort"))
			}
			executionPolicyVersion, threadPolicyMigrationAllowed := appturn.ThreadPolicyMigrationState(threadID, currentThread, h.pendingGateIDsFromReplay)
			startPlan = appturn.BuildStartPlan(appturn.StartPlanInput{
				ThreadID: threadID, TurnNumber: turnNumber, CreatedAt: now, Prompt: request.Prompt, DisplayText: request.DisplayText,
				Model: model, ProviderID: providerID, ReasoningEffort: effort, EndpointFormat: endpointFormat,
				RequestApprovalPolicy: request.ApprovalPolicy, ThreadApprovalPolicy: stringField(currentThread, "approvalPolicy"), RuntimeApprovalPolicy: h.approvalPolicy,
				RequestSandboxMode: request.SandboxMode, ThreadSandboxMode: stringField(currentThread, "sandboxMode"), RuntimeSandboxMode: h.sandboxMode,
				ThreadExecutionPolicyVersion: executionPolicyVersion, ThreadPolicyMigrationAllowed: threadPolicyMigrationAllowed,
				Mode: request.Mode, AttachmentIDs: request.AttachmentIDs, Attachments: attachments, FileReferences: request.FileReferences,
				WorkspaceCheckpointID: request.WorkspaceCheckpointID, GUIPlan: request.GUIPlan, DisableUserInput: request.DisableUserInput,
				DisableUserInputSet: request.DisableUserInputSet, MaxModelSteps: request.MaxModelSteps,
			})
			if startPlan.TurnID != turnID {
				return turnstartapp.SecurityRecords{}, "", errors.New("turn start record factory changed the admitted turn identity")
			}
			approvalPolicy, sandboxMode = startPlan.ApprovalPolicy, startPlan.SandboxMode
			return turnstartapp.SecurityRecords{
				Turn: startPlan.Record.Turn, TurnStartedEvent: startPlan.Record.TurnStartedEvent, ThreadPatch: startPlan.ThreadPatch,
			}, providerID, nil
		},
	})
	cancelContextCommit()
	if admissionErr != nil {
		// Registration precedes the live probe. A later provider/attachment
		// admission failure therefore must still append and commit this exact
		// turn before the host closes it; returning from RecordFactory would
		// strand signed staged authority and quarantine the thread on restart.
		commitResult.Preparation.ExecutionReady = false
	}
	securityContext, providerContext := commitResult.Preparation.SecurityContext, commitResult.Preparation.ProviderContext
	providerRequest := request
	if hostChildCasePrompt != "" {
		if validatedHostChildCasePrompt == "" || validatedHostChildCasePrompt != hostChildCasePrompt {
			return nil, errors.New("current host child case prompt authority is invalid")
		}
		// The exact prompt is a process-local provider-attempt sidecar. The
		// durable turn keeps the ordinary privacy projection because numeric
		// commitment digests can otherwise resemble account identifiers.
		providerRequest.Prompt = validatedHostChildCasePrompt
	}
	if commitResult.Appended {
		postAppendFailureInput = runtimeAgentLoopInput{
			ThreadID: threadID, TurnID: turnID, Thread: thread, Request: providerRequest, ProviderConfig: providerConfig,
			ProviderID: providerID, Model: model, Effort: effort, Workspace: threadWorkspace, Attachments: attachments,
			ApprovalPolicy: approvalPolicy, SandboxMode: sandboxMode, ToolScope: append([]string(nil), request.InternalToolScope...),
			SubagentDepth: request.InternalSubagentDepth, UsageSource: request.InternalUsageSource, ChildRunID: request.InternalChildRunID,
			DelegatedToolManifest: delegatedToolManifest,
			SystemPrompt:          request.InternalSystemPrompt, SecurityContext: securityContext, ContextEpoch: providerContext, HistoryBeforeTurnID: turnID,
			CurrentUserItemID: startPlan.UserItemID, HostChildCasePromptWitness: hostChildCasePromptWitness,
			CaseFundPolicy: preProviderCaseFundPolicy, TrustedCaseCompactionTurnIDs: trustedCaseCompactionTurnIDs,
		}
		postAppendGuardArmed = true
	}
	if commitErr != nil {
		return nil, commitErr
	}
	thread = commitResult.Thread
	if title := strings.TrimSpace(stringField(thread, "title")); title != "" && title != previousTitle {
		if _, _, err := h.store.RecordEvent(appthread.BuildUpdatedEvent(threadID, title, stringField(thread, "status"))); err != nil {
			return nil, err
		}
	}
	turnStartedEvent := apploop.WithTurnStartedTrace(startPlan.Record.TurnStartedEvent, now, time.Now().UTC(), runtimeThreadTraceEnabled())
	if _, _, err := h.store.RecordEventsAtomic([]map[string]any{
		turnStartedEvent,
		startPlan.Record.UserItemCreatedEvent,
	}); err != nil {
		return nil, err
	}
	if !commitResult.Preparation.ExecutionReady {
		if err := securityTransition.Commit(); err != nil {
			return nil, err
		}
		response = startPlan.Response()
		if admissionErr != nil && !apploop.CasePublicationRequired(securityContext) {
			return nil, admissionErr
		}
		reasonOverride := evidenceapp.TerminalReason("")
		if admissionErr != nil {
			reasonOverride = evidenceapp.TerminalProviderFailure
		}
		completion, boundaryErr := h.completeHostBoundaryTurn(postAppendFailureInput, commitResult.Preparation, now, reasonOverride)
		if boundaryErr != nil {
			return nil, boundaryErr
		}
		postAppendGuardArmed = false
		for key, value := range completion {
			response[key] = value
		}
		return response, nil
	}
	loopInput := postAppendFailureInput
	loopInput.Thread = thread
	if err := turnContext.Err(); err != nil {
		return nil, err
	}
	if err := securityTransition.Commit(); err != nil {
		h.runtimeControl().UnregisterTurnCancel(threadID, turnID)
		turnCancelRegistered = false
		cancelTurn()
		return nil, err
	}
	securityTransition = nil
	if releaseThreadTransition != nil {
		releaseThreadTransition()
		releaseThreadTransition = nil
	}
	initialIngress := apploop.PrepareInitialCaseAccountIngressV1(
		turnContext,
		apploop.NewInitialCaseAccountIngressInputV1(
			loopInput.CaseFundPolicy,
			loopInput.SecurityContext,
			loopInput.Request.Prompt,
			casePolicyPrompt,
			loopInput.SubagentDepth > 0 || strings.TrimSpace(loopInput.ChildRunID) != "",
			stringField(thread, "relation"),
			stringField(thread, "forkedFromThreadId"),
			h.caseEntities,
			h.resolveCaseIngress,
		),
	)
	loopInput.CaseFundPolicy = initialIngress.Policy
	loopInput.CaseSourceUnavailable = initialIngress.SourceUnavailable
	loopInput.initialProviderProjection = initialIngress.ProviderProjection
	loopInput.HostEntitySelection = initialIngress.HostEntitySelection
	researchPrompt := apploop.HostEffectPromptV1(loopInput.CaseFundPolicy, request.Prompt)
	casePolicyPrompt = ""
	if err := turnContext.Err(); err != nil {
		return nil, err
	}
	if err := subagentapp.PrepareRuntimeTaskJobSteeringForTurn(subagentapp.TaskJobSteerRuntimeDeps{
		Context: turnContext, Jobs: h.jobs, Turns: h.store,
		RecordEvent: h.recordRuntimeBestEffortEvent, BeginAuthority: h.beginRuntimeTaskJobSteerAuthority,
	}, request.InternalChildRunID, threadID, turnID); err != nil {
		return nil, err
	}
	if research.IsResearchGoalPrompt(researchPrompt) {
		if err := turnstartapp.RunResearchStateEffect(
			turnContext, securityContext, h.runtimeSubagentState().AcquireContextEffect,
			func(effectCtx context.Context) error {
				return h.validateCurrentProviderAttempt(effectCtx, loopInput, false, false)
			},
			func(effectCtx context.Context) error {
				return research.PrepareTurnState(effectCtx, h.autoResearch, h.store, researchPrompt, securityContext.WorkspaceRealPath, threadID, turnID)
			},
		); err != nil {
			return nil, err
		}
	}
	response = startPlan.Response()
	if request.Async {
		postAppendGuardArmed = false
		operationOwned = false
		turnCancelTransferred = true
		go func() {
			defer cancelTurn()
			h.completeRuntimeTurnAsync(turnContext, loopInput, turnNumber, now, finishOperation)
		}()
		return response, nil
	}
	completion, err := h.completeStartedRuntimeTurn(turnContext, loopInput, turnNumber, now)
	if err != nil {
		return nil, err
	}
	postAppendGuardArmed = false
	for key, value := range completion {
		response[key] = value
	}
	return response, nil
}

func (h *runtimeServerHandler) runtimeChildTransitionAuthority(request startRuntimeTurnRequest, threadID string) (*turnstartapp.ChildTransitionAuthority, error) {
	if h == nil {
		return nil, errors.New("durable child turn authority is unavailable")
	}
	return turnstartapp.ResolveChildTransitionAuthority(turnstartapp.ChildTransitionResolveInput{
		ChildRunID: request.InternalChildRunID, ChildDepth: request.InternalSubagentDepth,
		ChildThreadID: threadID, Store: h.jobs, Blocker: h.runtimeJobSecurityAuthorizer().Blocker,
	})
}

func runtimeTaskJobQueuedSteerRiskSignals(authority *turnstartapp.ChildTransitionAuthority) (bool, bool) {
	if authority == nil {
		return false, false
	}
	lexicalCaseRisk := false
	protectedCaseData := false
	for _, message := range authority.ExpectedRecord.Steers {
		if strings.TrimSpace(message.Status) != "queued" {
			continue
		}
		lexicalCaseRisk = lexicalCaseRisk || apploop.PromptRequiresCaseRiskAdmission(message.Text)
		protectedCaseData = protectedCaseData || domainsecurity.ContainsProtectedCaseFactCandidate(message.Text)
	}
	return lexicalCaseRisk, protectedCaseData
}

func (h *runtimeServerHandler) validateRuntimeChildTransitionFrozen(
	ctx context.Context,
	authority *turnstartapp.ChildTransitionAuthority,
	frozen domainsecurity.TurnSecurityContext,
) error {
	if h == nil {
		return errors.New("frozen durable child turn authority is invalid")
	}
	return turnstartapp.ValidateChildTransitionFrozen(
		ctx, authority, h.jobs, h.runtimeJobSecurityAuthorizer().Blocker, frozen,
	)
}

func (h *runtimeServerHandler) completeHostBoundaryTurn(input runtimeAgentLoopInput, preparation turnstartapp.SecurityPreparation, startedAt string, reasonOverride evidenceapp.TerminalReason) (map[string]any, error) {
	securityContext := preparation.SecurityContext
	if domainsecurity.ValidateTurnSecurityContext(securityContext) != nil || preparation.ExecutionReady {
		return nil, errors.New("host boundary turn authority is invalid")
	}
	sourceUnavailable := domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) && !preparation.SourceReady
	if domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) {
		switch securityContext.PublicationPolicy.BlockerCode {
		case domainsecurity.PublicationBlockerCaseBindingMissing,
			domainsecurity.PublicationBlockerDatasetSnapshotUnavailable:
			sourceUnavailable = true
		}
	}
	reason := evidenceapp.TerminalSuccess
	if sourceUnavailable {
		reason = evidenceapp.TerminalSourceUnavailable
	}
	if reasonOverride != "" {
		reason = reasonOverride
		sourceUnavailable = false
	}
	hostContext, cancelHostAuthority := newHostAuthorityContext()
	accepted, err := h.runtimePublicationAuthority().PersistCurrentCaseFixed(hostContext, evidenceapp.PersistCaseBoundaryInput{
		Store: h.store, Context: securityContext, TerminalReason: reason, SourceUnavailable: sourceUnavailable,
		ReportRequested: apploop.PromptLooksLikeCaseFundReportDelivery(input.Request.Prompt),
		ThreadID:        input.ThreadID, TurnID: input.TurnID, Model: input.Model, CreatedAt: startedAt,
		AcceptedAt: time.Now().UTC(), UsageSource: input.Request.InternalUsageSource,
		ChildRunID: input.Request.InternalChildRunID,
	})
	cancelHostAuthority()
	if err != nil {
		return nil, err
	}
	if !accepted.Persistence.Changed {
		return map[string]any{"status": accepted.Persistence.Status}, nil
	}
	return map[string]any{"status": "completed"}, nil
}

func (h *runtimeServerHandler) completeRuntimeTurnAsync(ctx context.Context, input runtimeAgentLoopInput, turnNumber int, startedAt string, finishOperation func()) {
	var completionErr, recordErr error
	if h.asyncTurnPhaseObserver != nil {
		ctx = context.WithValue(ctx, asyncTurnPhaseObserverContextKeyV1{}, h.asyncTurnPhaseObserver)
	}
	defer func() {
		h.runtimeControl().UnregisterTurnCancel(input.ThreadID, input.TurnID)
		if finishOperation != nil {
			finishOperation()
		}
	}()
	if observe := h.asyncTurnObserver; observe != nil {
		phase := "starting"
		ctx = context.WithValue(ctx, asyncTurnPhaseContextKeyV1{}, &phase)
		defer func() {
			observe(AsyncTurnObservationV1{
				ThreadID: input.ThreadID, TurnID: input.TurnID, Stage: "finished",
				CompletionPhase: phase, CompletionErrorClass: asyncTurnErrorClassV1(completionErr),
				CompletionDetailClass:    appturn.GeneralTerminalDetailClassV1(completionErr),
				FailureRecordDetailClass: appturn.GeneralTerminalDetailClassV1(recordErr),
				FailureRecordErrorClass:  asyncTurnErrorClassV1(recordErr), TerminalStatus: h.asyncTurnTerminalStatusV1(input),
			})
		}()
		observe(AsyncTurnObservationV1{ThreadID: input.ThreadID, TurnID: input.TurnID, Stage: "started", CompletionPhase: phase})
	}
	if _, completionErr = h.completeStartedRuntimeTurn(ctx, input, turnNumber, startedAt); completionErr != nil {
		recordErr = h.recordRuntimeTurnFailureForInput(input, completionErr)
		if recordErr != nil {
			fmt.Fprintln(os.Stderr, "[analytix] event=ANALYTIX_RUNTIME_ASYNC_TURN_FAILURE_RECORD_FAILED")
		}
	}
}

func (h *runtimeServerHandler) completeStartedRuntimeTurn(ctx context.Context, input runtimeAgentLoopInput, turnNumber int, startedAt string) (map[string]any, error) {
	threadID := input.ThreadID
	turnID := input.TurnID
	request := input.Request
	model := input.Model
	effort := input.Effort
	caseFundPolicy, _ := h.resolveRuntimeCaseFundAnalysisPolicy(&input)
	setAsyncTurnCompletionPhaseV1(ctx, "loop")
	loopResult, err := h.runRuntimeAgentLoop(ctx, input)
	setAsyncTurnCompletionPhaseV1(ctx, "candidate_handoff")
	terminalOperationContext, releaseCandidateTerminal, handoffErr := takeRuntimeCandidateTerminal(ctx, &loopResult)
	if releaseCandidateTerminal != nil {
		defer releaseCandidateTerminal()
	}
	if handoffErr != nil {
		return nil, apploop.WrapHostBoundaryFailure(apploop.HostCandidateAuthorityFailure, handoffErr)
	}
	loopResult, err = apploop.RejectCancelledRuntimeResult(ctx, loopResult, err)
	if err != nil {
		setAsyncTurnCompletionPhaseV1(ctx, "loop")
		failure := apploop.RuntimeAgentLoopFailure{Cause: err, Result: loopResult}
		if releaseCandidateTerminal != nil {
			fixedTerminalContext, cancelTerminalPublication := newHostAuthorityContextFrom(terminalOperationContext)
			defer cancelTerminalPublication()
			setAsyncTurnCompletionPhaseV1(ctx, "claimed_failure_persist")
			if persistErr := h.persistRuntimeTurnFailureForInput(fixedTerminalContext, input, failure); persistErr != nil {
				return nil, errors.Join(failure, fmt.Errorf("persist claimed runtime failure: %w", persistErr))
			}
			if status, terminal, statusErr := h.runtimeTurnTerminalStatus(threadID, turnID); statusErr == nil && terminal {
				return map[string]any{"status": status}, nil
			}
		}
		if ctx.Err() != nil {
			if status, terminal, statusErr := h.runtimeTurnTerminalStatus(threadID, turnID); statusErr == nil && terminal {
				return map[string]any{"status": status}, nil
			}
		}
		return nil, failure
	}
	if status, terminal, statusErr := h.runtimeTurnTerminalStatus(threadID, turnID); statusErr == nil && terminal {
		return map[string]any{"status": status}, nil
	}
	if loopResult.Paused {
		response := map[string]any{
			"threadId":    threadID,
			"turnId":      turnID,
			"status":      "waiting",
			"pendingKind": loopResult.PendingKind,
			"pendingId":   loopResult.PendingID,
		}
		return response, nil
	}
	terminalEnteredAt := time.Now()
	sourceUnavailable := caseFundPolicy.SourceUnavailable || loopResult.CaseSourceUnavailable
	terminalReason := evidenceapp.TerminalReasonForRuntimeCompletion(loopResult, sourceUnavailable)
	fixedTerminalContext, cancelTerminalPublication := h.newRuntimeTerminalContextV1(
		terminalOperationContext, terminalEnteredAt, input.SecurityContext, loopResult, terminalReason, releaseCandidateTerminal,
	)
	defer cancelTerminalPublication()
	candidateTerminalContext := terminalOperationContext
	if releaseCandidateTerminal != nil {
		candidateTerminalContext = fixedTerminalContext
	}
	result := loopResult.LastResult
	cacheDiagnostics := h.runtimeCacheDiagnostics(threadID, input.SecurityContext, result)
	var completionRecord appturn.CompletionRecord
	var publicationTiming appturn.PublicationTiming
	var changed bool
	status := ""
	terminalReferenceKind := ""
	terminalReferenceDigest := ""
	if err := requireRuntimeCandidateTerminalHandoffForResult(
		terminalReason, loopResult, terminalOperationContext, releaseCandidateTerminal,
	); err != nil {
		setAsyncTurnCompletionPhaseV1(ctx, "candidate_handoff")
		return nil, apploop.WrapHostBoundaryFailure(apploop.HostCandidateAuthorityFailure, err)
	}
	if apploop.CasePublicationRequired(input.SecurityContext) {
		caseInput := evidenceapp.PersistCaseBoundaryInput{
			Store: h.store, Context: input.SecurityContext, TerminalReason: terminalReason,
			OrdinaryResult: loopResult.OrdinaryResult, CaseSlotIntent: evidenceapp.CaseSlotPublicationIntentForTurnV1(apploop.RuntimeResultCarriesOrdinaryCandidate(loopResult), caseFundPolicy.Active, apploop.PromptLooksLikeCaseFundReportDelivery(input.Request.Prompt)),
			SourceUnavailable: sourceUnavailable, ReportRequested: apploop.PromptLooksLikeCaseFundReportDelivery(input.Request.Prompt),
			ThreadID: threadID, TurnID: turnID, Model: model, CreatedAt: startedAt, AcceptedAt: time.Now().UTC(),
			Telemetry:   appusage.NewTerminalTelemetryV1(result.Usage, cacheDiagnostics),
			UsageSource: request.InternalUsageSource, ChildRunID: request.InternalChildRunID,
		}
		var accepted evidenceapp.PersistCaseBoundaryResult
		var err error
		if appturn.GeneralTerminalReasonAllowsCandidateV1(string(terminalReason)) ||
			apploop.RuntimeResultCarriesOrdinaryCandidate(loopResult) {
			setAsyncTurnCompletionPhaseV1(ctx, "case_candidate_persist")
			accepted, err = h.runtimePublicationAuthority().PersistCurrentCaseCandidate(
				candidateTerminalContext, caseInput, loopResult.CandidateUsesCaseData,
			)
		} else {
			setAsyncTurnCompletionPhaseV1(ctx, "case_fixed_persist")
			accepted, err = h.runtimePublicationAuthority().PersistCurrentCaseFixed(fixedTerminalContext, caseInput)
		}
		if err != nil {
			return nil, err
		}
		setAsyncTurnCompletionPhaseV1(ctx, "case_longitudinal_append")
		if err := caseentityapp.AppendFinalizedCaseLongitudinalStateV1(
			fixedTerminalContext,
			h.caseEntities,
			h.store,
			threadID,
			input.SecurityContext,
			&accepted,
		); err != nil {
			return nil, err
		}
		publicationTiming = accepted.Persistence.Timing
		completionRecord = accepted.Persistence.CompletionRecord
		changed, status = accepted.Persistence.Changed, accepted.Persistence.Status
		terminalReferenceKind = appgoal.TerminalReferenceAcceptedFinal
		terminalReferenceDigest = accepted.Persistence.AcceptedFinal.RecordDigest
		if !changed {
			return map[string]any{"status": status}, nil
		}
	} else {
		if appturn.GeneralTerminalReasonAllowsCandidateV1(string(terminalReason)) {
			setAsyncTurnCompletionPhaseV1(ctx, "general_candidate_commit")
			committed, commitErr := h.runtimePublicationAuthority().CommitCurrentGeneralCompletion(candidateTerminalContext, appturn.CommitCompletionInput{
				Store: h.store, SecurityContext: input.SecurityContext,
				ThreadID: threadID, TurnID: turnID, Model: model,
				CreatedAt: startedAt, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
				Telemetry: appusage.NewTerminalTelemetryV1(result.Usage, cacheDiagnostics), UsageSource: request.InternalUsageSource, ChildRunID: request.InternalChildRunID,
				TerminalReason: string(terminalReason),
				OrdinaryResult: loopResult.OrdinaryResult,
			})
			if commitErr != nil {
				return nil, apploop.WrapHostBoundaryFailure(apploop.HostCandidatePublicationFailure, commitErr)
			}
			publicationTiming = committed.Timing
			completionRecord, changed, status = committed.CompletionRecord, committed.Changed, committed.Status
			terminalReferenceDigest = committed.CASBinding.BindingDigest
		} else {
			setAsyncTurnCompletionPhaseV1(ctx, "general_fixed_commit")
			failure := evidenceapp.GeneralTerminalHostFailure(terminalReason)
			var committed appturn.PersistFailureResult
			commitErr := h.runtimePublicationAuthority().WithCurrentGeneralFixedTerminal(fixedTerminalContext, input.SecurityContext, func(context.Context) error {
				var err error
				committed, err = appturn.CommitGeneralFailureTerminal(appturn.PersistFailureInput{
					Store: h.store, SecurityContext: input.SecurityContext, TerminalReason: string(terminalReason),
					ThreadID: threadID, TurnID: turnID, Model: model, Failure: failure,
					FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), Result: result,
					CacheDiagnostics: cacheDiagnostics, UsageSource: request.InternalUsageSource, ChildRunID: request.InternalChildRunID,
				})
				return err
			})
			if commitErr != nil {
				return nil, commitErr
			}
			completionRecord.UsageEvent = committed.Publication.UsageEvent
			changed, status = committed.Changed, committed.Status
			terminalReferenceDigest = committed.Publication.AuthorityDigest
		}
		terminalReferenceKind = appgoal.TerminalReferenceGeneralCAS
		if !changed {
			return map[string]any{"status": status}, nil
		}
	}
	recordRuntimePublicationTrace(threadID, turnID, loopResult, terminalEnteredAt, publicationTiming)
	if releaseCandidateTerminal != nil {
		releaseCandidateTerminal()
	}
	if goal, goalErr := h.store.GetGoal(threadID); goalErr == nil && goal != nil {
		if _, _, err := appgoal.RecordCompletedTurnChildRun(appgoal.CompletedTurnChildRunInput{
			Starter:                 h.jobs,
			Events:                  h.store,
			Goal:                    goal,
			ThreadID:                threadID,
			TurnID:                  turnID,
			TurnNumber:              turnNumber,
			Model:                   model,
			Effort:                  effort,
			RequestedModel:          request.Model,
			TerminalReferenceKind:   terminalReferenceKind,
			TerminalReferenceDigest: terminalReferenceDigest,
			SecurityContext:         input.SecurityContext,
			TotalTokens:             result.Usage.TotalTokens,
			CacheHitRate:            result.Usage.CacheHitRate,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "[analytix] event=ANALYTIX_RUNTIME_POST_TERMINAL_GOAL_LINEAGE_FAILED")
		}
	}
	if !changed {
		return map[string]any{"status": status}, nil
	}
	response := map[string]any{"status": status}
	if usage, ok := completionRecord.UsageEvent["usage"].(map[string]any); ok {
		response["usage"] = contracts.CloneMap(usage)
	}
	if cacheDiagnostics, ok := completionRecord.UsageEvent["cacheDiagnostics"].(map[string]any); ok {
		response["cacheDiagnostics"] = contracts.CloneMap(cacheDiagnostics)
	}
	return response, nil
}
