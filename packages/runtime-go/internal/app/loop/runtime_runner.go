package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	appplan "analytix.local/runtime-go/internal/app/plan"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/ports"
)

const (
	terminalToolSupersededBoundary               = "The pending terminal tool call was superseded by a newer user instruction before execution."
	providerToolArgumentValidationRecoveryPrompt = "The previous tool-call batch was rejected before persistence or execution because at least one call did not satisfy the advertised JSON Schema. Issue a fresh complete tool-call batch with valid JSON and every required field."
	providerDuplicateCreatePlanRecoveryPrompt    = "The previous Plan-mode tool-call batch was rejected before persistence or execution because it contained more than one create_plan call. Issue a fresh complete tool-call batch with exactly one create_plan call and valid JSON arguments."
)

func providerToolArgumentValidationRecoveryPromptForTool(
	toolName string,
	toolSchemas []domainmodel.ToolSchema,
) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" || !toolcatalogapp.ToolRequiresExactEmptyObjectArguments(toolName, toolSchemas) {
		return providerToolArgumentValidationRecoveryPrompt
	}
	return fmt.Sprintf(
		"The previous call to %q was rejected before persistence or execution. Call only %q with JSON arguments exactly {} and no fields.",
		toolName,
		toolName,
	)
}

func providerToolArgumentValidationRecoveryPromptForFailure(
	failure TurnFailureError,
	toolName string,
	toolSchemas []domainmodel.ToolSchema,
) string {
	if providerCorrectableToolFailureReason(failure) == duplicateCreatePlanValidationCode {
		return providerDuplicateCreatePlanRecoveryPrompt
	}
	return providerToolArgumentValidationRecoveryPromptForTool(toolName, toolSchemas)
}

func providerCorrectableToolFailureReason(failure TurnFailureError) string {
	if strings.TrimSpace(fmt.Sprint(failure.Details["code"])) == duplicateCreatePlanValidationCode {
		return duplicateCreatePlanValidationCode
	}
	return "invalid_tool_arguments"
}

// RuntimeRunnerInput is the application-owned state required to continue one
// provider/tool loop. HTTP request shapes and server persistence records are
// deliberately flattened before crossing this boundary.
// AdditionalCaseDataEffect classifies protected provider material outside the
// independent funds-tool advertisement policy.
type RuntimeRunnerInput struct {
	ThreadID                 string
	TurnID                   string
	ProviderConfig           domainmodel.TurnConfig
	ProviderID               string
	Model                    string
	Effort                   string
	Workspace                string
	Prompt                   string
	Mode                     string
	GUIPlan                  map[string]any
	DisableUserInput         bool
	MaxModelSteps            *int
	EffectiveMaxModelSteps   int
	WorkspaceCheckpointID    string
	AttachmentIDs            []string
	AttachmentPlanDigest     string
	HasAttachments           bool
	ApprovalPolicy           string
	SandboxMode              string
	NormalizedSandboxMode    string
	ToolScope                []string
	SubagentDepth            int
	UsageSource              string
	ChildRunID               string
	DelegatedToolManifest    *domainjob.DelegatedToolManifestV1
	SystemPrompt             string
	FallbackSystemPrompt     string
	SecurityContext          domainsecurity.TurnSecurityContext
	HostEntitySelection      HostCaseEntitySelectionV1
	ProviderContinuationRefs []domainsecurity.SettledToolReference
	PrivateProtocolSession   *domainmodel.PrivateProtocolSession
	AnthropicCapsule         *domainmodel.AnthropicThinkingCapsule
	PrivateProtocolCapsules  []*domainmodel.AnthropicThinkingCapsule
	ProviderCallSequence     uint64
	TerminalRecoveryKind     RuntimeTerminalRecoveryKind
	CaseFundPolicy           CaseFundAnalysisPolicy
	AdditionalCaseDataEffect bool
	// RestoredProviderStep is set only by a host-verified continuation. It
	// bypasses prompt reclassification so restart preserves the exact signed
	// steering effect that produced the pending tool call.
	RestoredProviderStep  *RuntimeProviderStep
	CaseSourceUnavailable bool
	// OrdinaryResultInputIsolated is process-local host provenance for the
	// exact provider message lane. It is never serialized or inferred merely
	// from LogicalEffectOrdinary.
	OrdinaryResultInputIsolated bool
	Messages                    []domainmodel.Message
	// OrdinaryLaneMessages is reconstructed by the host only from the base
	// system prompt and committed typed ordinary results. Protected-lane
	// transitions must return to this baseline instead of filtering and reusing
	// an unpartitioned case transcript.
	OrdinaryLaneMessages       []domainmodel.Message
	ProviderCustomRequestShape string
	RequiredFinalToolName      string
	// OutputTokenBudget is non-zero only for a host-bounded provider turn. The
	// foreground child contract uses exactly one provider response, so this is
	// both its wire limit and its independently enforced host stream limit.
	OutputTokenBudget int
}

type RuntimeRunnerToolCatalog struct {
	PromptRoute string
	// SystemPrompt is the complete provider system prompt compiled from the
	// immutable base prompt for this exact step. When non-empty, it replaces the
	// prior step's primary system message so effect-specific instructions cannot
	// survive an effect transition.
	SystemPrompt   string
	Advertisements []toolcatalogapp.MCPToolAdvertisementV1
	Schemas        []domainmodel.ToolSchema
}

// RuntimeProviderStep is the loop-owned classification for exactly one
// provider/tool step. It describes the host authority the step must acquire;
// it does not grant that authority or replace the ordinary Agent base.
type RuntimeProviderStep struct {
	Prompt                string
	LogicalEffect         domainsecurity.LogicalEffect
	OrdinaryWork          bool
	CaseSourceUnavailable bool
	HostEntitySelection   HostCaseEntitySelectionV1
}

func (step RuntimeProviderStep) OrdinaryEffect() bool {
	return step.LogicalEffect == domainsecurity.LogicalEffectOrdinary
}

func (step RuntimeProviderStep) UsesCaseDataAuthority() bool {
	return step.LogicalEffect == domainsecurity.LogicalEffectCaseData ||
		step.LogicalEffect == domainsecurity.LogicalEffectFundsData
}

func (step RuntimeProviderStep) UsesFundsDataAuthority() bool {
	return step.LogicalEffect == domainsecurity.LogicalEffectFundsData
}

// RuntimeSteeringBatch is one atomically promoted, same-effect steering
// prefix. Messages are the provider projection; Prompt is the exact host input
// used by tool selection and tool execution for the following steps.
type RuntimeSteeringBatch struct {
	Messages      []domainmodel.Message
	Prompt        string
	LogicalEffect domainsecurity.LogicalEffect
	OrdinaryWork  bool
}

type RuntimeMaterializedPlan struct {
	Output          any
	IsError         bool
	Messages        []domainmodel.Message
	ContinuationRef domainsecurity.SettledToolReference
}

type RuntimeProviderIntent struct {
	ProviderConfig domainmodel.TurnConfig
	ProviderID     string
	Model          string
	Effort         string
}

type RuntimePrepareProviderAttempt func(
	context.Context,
	int,
	domainmodel.Request,
	string,
	string,
	uint64,
) (domainmodel.Request, ProviderAttemptSettlement, error)

// RuntimeRunnerDependencies are host capabilities needed by the application
// loop. Each callback is bound to the immutable turn context by the composition
// layer; the model cannot provide or replace any of them.
type RuntimeRunnerDependencies struct {
	Provider                 ports.ProviderClient
	PendingWork              *pendingworkapp.Service
	Events                   *RuntimeEventRecorder
	ResolveToolCatalog       func(planActive bool, step RuntimeProviderStep) RuntimeRunnerToolCatalog
	PromoteSteering          func(context.Context) (RuntimeSteeringBatch, error)
	AcquireCandidateTerminal func(context.Context, RuntimeProviderStep) (context.Context, func(), error)
	PrepareProviderStep      func(context.Context, RuntimeProviderStep) (RuntimeProviderStep, error)
	ResolveProviderIntent    func(context.Context) (RuntimeProviderIntent, error)
	PauseChild               func(context.Context) error
	AcquireProviderAttempt   func(context.Context, int, RuntimeProviderStep) (context.Context, func(), error)
	PrepareProviderAttempt   RuntimePrepareProviderAttempt
	MaterializeCreatePlan    func(context.Context, []domainmodel.Message, string, bool) (RuntimeMaterializedPlan, error)
	BackgroundMemoryCount    func() int
	ToolDriver               ToolStepDriver
}

func RunRuntimeAgentLoop(ctx context.Context, input RuntimeRunnerInput, deps RuntimeRunnerDependencies) (loopResult RuntimeAgentLoopResult, loopErr error) {
	stickyRecovery := input.TerminalRecoveryKind
	caseSourceUnavailable := input.CaseFundPolicy.SourceUnavailable || input.CaseSourceUnavailable
	defer func() {
		loopResult.CaseSourceUnavailable = loopResult.CaseSourceUnavailable || caseSourceUnavailable
		SealRuntimeAgentLoopResultForReturn(&loopResult, &stickyRecovery)
	}()
	if deps.Events == nil {
		return RuntimeAgentLoopResult{}, errors.New("runtime loop event recorder is unavailable")
	}
	if _, ok := deps.Provider.(ports.DurablePipelineProviderClient); !ok {
		return RuntimeAgentLoopResult{}, errors.New("runtime provider durable pipeline contract is unavailable")
	}
	if deps.ResolveToolCatalog == nil || deps.PromoteSteering == nil || deps.AcquireCandidateTerminal == nil ||
		deps.PrepareProviderStep == nil ||
		deps.PauseChild == nil || deps.AcquireProviderAttempt == nil {
		return RuntimeAgentLoopResult{}, errors.New("runtime loop host authority is unavailable")
	}
	if deps.ToolDriver == nil {
		return RuntimeAgentLoopResult{}, errors.New("runtime tool authority is unavailable")
	}
	if len(input.ProviderContinuationRefs) > 0 && deps.PendingWork == nil {
		return RuntimeAgentLoopResult{}, errors.New("runtime continuation authority is unavailable")
	}
	if input.HasAttachments && deps.PrepareProviderAttempt == nil {
		return RuntimeAgentLoopResult{}, errors.New("runtime attachment authority is unavailable")
	}
	if strings.TrimSpace(input.RequiredFinalToolName) != "" {
		if input.RequiredFinalToolName != toolcatalogapp.ForegroundSubmitToolName || input.OutputTokenBudget < 1 ||
			input.OutputTokenBudget > toolcatalogapp.ForegroundMaxTokenBudget {
			return RuntimeAgentLoopResult{}, errors.New("foreground child output token budget authority is invalid")
		}
	} else if input.OutputTokenBudget != 0 {
		return RuntimeAgentLoopResult{}, errors.New("output token budget is not authorized for this turn")
	}
	maxSteps := input.EffectiveMaxModelSteps
	if maxSteps <= 0 {
		// The host resolver is authoritative, but the execution boundary still
		// fails bounded if a legacy continuation or future caller bypasses it.
		maxSteps = DefaultModelStepLimit
	}
	effectiveMaxSteps := maxSteps
	currentProviderStep := initialRuntimeProviderStep(input)
	if caseSourceUnavailable {
		// Sticky source closure belongs to the frozen turn, not to whichever
		// provider-step record happened to survive a restart. It must dominate a
		// restored or freshly classified protected effect before any preparation,
		// currentness probe, catalog resolution, or transport attempt.
		currentProviderStep.CaseSourceUnavailable = true
	}
	loopEvents := deps.Events
	newStage := func(stage string, details map[string]any) domainmodel.PipelineStage {
		return domainmodel.PipelineStage{
			Stage:   stage,
			At:      time.Now().UTC(),
			Details: details,
		}
	}
	pendingStartupStages := []domainmodel.PipelineStage{
		newStage("setup", map[string]any{
			"workspaceBound": strings.TrimSpace(input.Workspace) != "",
			"caseBound":      CasePublicationRequired(input.SecurityContext),
		}),
		newStage("pre_start", map[string]any{
			"approvalPolicy": input.ApprovalPolicy,
			"sandboxMode":    input.SandboxMode,
		}),
		newStage("post_start", map[string]any{
			"maxModelSteps": float64(effectiveMaxSteps),
		}),
		newStage("input_received", map[string]any{
			"stepIndex":   float64(0),
			"promptBytes": float64(len([]byte(input.Prompt))),
		}),
		newStage("input_cached", RuntimePipelineMessageDetails(input.Messages)),
	}
	flushStartupStages := func(additional ...domainmodel.PipelineStage) error {
		if len(pendingStartupStages) == 0 && len(additional) == 0 {
			return nil
		}
		stages := make([]domainmodel.PipelineStage, 0, len(pendingStartupStages)+len(additional))
		stages = append(stages, pendingStartupStages...)
		stages = append(stages, additional...)
		pendingStartupStages = nil
		return loopEvents.PipelineStages(input.ThreadID, input.TurnID, stages)
	}
	flushStartupStagesWithCriticalStage := func(stage domainmodel.PipelineStage) error {
		if len(pendingStartupStages) == 0 {
			return loopEvents.PipelineStageAtomic(input.ThreadID, input.TurnID, stage)
		}
		stages := make([]domainmodel.PipelineStage, 0, len(pendingStartupStages)+1)
		stages = append(stages, pendingStartupStages...)
		stages = append(stages, stage)
		if err := loopEvents.PipelineStagesAtomic(input.ThreadID, input.TurnID, stages); err != nil {
			return err
		}
		pendingStartupStages = nil
		return nil
	}
	failAfterStartupStages := func(cause error) (RuntimeAgentLoopResult, error) {
		if err := flushStartupStages(); err != nil {
			return RuntimeAgentLoopResult{}, err
		}
		return RuntimeAgentLoopResult{}, cause
	}
	if input.CaseFundPolicy.MustReturnBoundaryBeforeProvider() {
		if err := flushStartupStages(); err != nil {
			return RuntimeAgentLoopResult{}, err
		}
		return RuntimeAgentLoopResult{AssistantText: input.CaseFundPolicy.BoundaryAnswer}, nil
	}
	var last domainmodel.Result
	emptyFinalRecoveries := 0
	const maxEmptyFinalRecoveries = 1
	streamRecoveries := 0
	providerToolValidationFailures := 0
	createPlanSatisfied := false
	stepLimitFinalizeNudged := false
	repeatSuccessCounts := map[string]int{}
	failureStormSignature := ""
	failureStormCount := 0
	providerContinuationRefs := append([]domainsecurity.SettledToolReference(nil), input.ProviderContinuationRefs...)
	privateProtocolSession := input.PrivateProtocolSession
	pendingPrivateProtocolCapsules := append(
		[]*domainmodel.AnthropicThinkingCapsule(nil),
		input.PrivateProtocolCapsules...,
	)
	if len(pendingPrivateProtocolCapsules) == 0 && input.AnthropicCapsule != nil {
		pendingPrivateProtocolCapsules = append(pendingPrivateProtocolCapsules, input.AnthropicCapsule)
	}
	if len(pendingPrivateProtocolCapsules) > 0 && privateProtocolSession == nil {
		return failAfterStartupStages(errors.New("anthropic private protocol session is unavailable"))
	}
	privateProtocolObserved := len(pendingPrivateProtocolCapsules) > 0
	privateProtocolKind := appmodel.PrivateProtocolReasoningKindForProvider(
		input.ProviderConfig,
		input.ProviderConfig.EndpointFormat,
		input.ProviderCustomRequestShape,
	)
	deepSeekPrivateProtocol := privateProtocolKind == "deepseek-chat-completions"
	configuredReasoningProtocol := strings.TrimSpace(input.ProviderConfig.ReasoningProtocol)
	privateProtocolSafeHistoryRequired :=
		deepSeekPrivateProtocol ||
			appmodel.PrivateProtocolRequired(configuredReasoningProtocol, input.Effort)
	providerCallSequence := input.ProviderCallSequence
	providerCallObservations := []domaincache.ProviderCallObservationV1{}
	messages := appmodel.CloneProviderMessages(input.Messages)
	ordinaryLaneMessages := appmodel.CloneProviderMessages(input.OrdinaryLaneMessages)
	if len(ordinaryLaneMessages) == 0 {
		ordinaryLaneMessages = runtimeSystemOnlyMessagesV1(messages)
	}
	currentOrdinaryInputClass := ""
	initialOrdinaryInputIsolated := input.OrdinaryResultInputIsolated ||
		(domainsecurity.TurnSecurityContextIsGeneral(input.SecurityContext) &&
			currentProviderStep.OrdinaryEffect() && currentProviderStep.OrdinaryWork)
	if initialOrdinaryInputIsolated && currentProviderStep.OrdinaryEffect() && currentProviderStep.OrdinaryWork {
		ordinaryLaneMessages = appmodel.CloneProviderMessages(messages)
		currentOrdinaryInputClass = RuntimeCandidateInputClassOrdinaryOnly
	}
	reportDeliveryCompleted := false
	compilePrivateProtocolSafeHistory := func() error {
		projected, err := privacyprojectionapp.PrivateProtocolSafeHistoryV1(
			input.SecurityContext,
			messages,
		)
		if err != nil {
			return err
		}
		projectedOrdinaryLane, err := privacyprojectionapp.PrivateProtocolSafeHistoryV1(
			input.SecurityContext,
			ordinaryLaneMessages,
		)
		if err != nil {
			return err
		}
		messages = projected
		ordinaryLaneMessages = projectedOrdinaryLane
		pendingPrivateProtocolCapsules = nil
		privateProtocolObserved = false
		return nil
	}
	applySteeringBatch := func(batch RuntimeSteeringBatch, prefix ...domainmodel.Message) (bool, error) {
		if len(batch.Messages) == 0 {
			if strings.TrimSpace(batch.Prompt) != "" || batch.LogicalEffect != "" || batch.OrdinaryWork {
				return false, errors.New("empty runtime steering batch contains provider-step metadata")
			}
			return false, nil
		}
		if strings.TrimSpace(batch.Prompt) == "" || domainsecurity.ValidateLogicalEffect(batch.LogicalEffect) != nil {
			return false, errors.New("runtime steering provider-step effect is invalid")
		}
		if caseSourceUnavailable && batch.LogicalEffect != domainsecurity.LogicalEffectOrdinary && batch.OrdinaryWork {
			ordinaryPrompt := IndependentOrdinaryPromptV1(batch.Prompt)
			if ordinaryPrompt != "" {
				// The protected lane is sticky-closed. Replace the promoted mixed
				// message with the exact case-free ordinary subrequest; never filter
				// and reuse the raw protected steering text.
				batch.Prompt = ordinaryPrompt
				batch.LogicalEffect = domainsecurity.LogicalEffectOrdinary
				batch.Messages = []domainmodel.Message{{
					Role: "user", Content: appmodel.SteeringProviderContent(ordinaryPrompt, nil),
				}}
			}
		}
		if privateProtocolObserved {
			if err := compilePrivateProtocolSafeHistory(); err != nil {
				return false, err
			}
		}
		if batch.LogicalEffect == domainsecurity.LogicalEffectOrdinary {
			// An exact signed ordinary steering batch resumes from the last
			// ordinary-only lane. Case prompts, drafts, and protected tool
			// results accumulated after that point are not replayed.
			steeringPrefix := runtimeOrdinarySteeringPrefixV1(
				input.SecurityContext, currentOrdinaryInputClass, prefix,
			)
			messages = appmodel.CloneProviderMessages(ordinaryLaneMessages)
			messages = append(messages, steeringPrefix...)
			currentOrdinaryInputClass = RuntimeCandidateInputClassOrdinaryOnly
		} else {
			messages = append(messages, appmodel.CloneProviderMessages(prefix)...)
			currentOrdinaryInputClass = ""
		}
		messages = append(messages, appmodel.CloneProviderMessages(batch.Messages)...)
		if batch.LogicalEffect == domainsecurity.LogicalEffectOrdinary {
			ordinaryLaneMessages = appmodel.CloneProviderMessages(messages)
		}
		currentProviderStep = RuntimeProviderStep{
			Prompt: batch.Prompt, LogicalEffect: batch.LogicalEffect, OrdinaryWork: batch.OrdinaryWork,
			CaseSourceUnavailable: caseSourceUnavailable,
		}
		providerContinuationRefs = nil
		pendingPrivateProtocolCapsules = nil
		privateProtocolObserved = false
		return true, nil
	}
	compileOrdinaryCandidate := func(step RuntimeProviderStep, candidateText string) (*domainordinaryresult.ResultSlotV1, error) {
		if !step.OrdinaryEffect() || !step.OrdinaryWork ||
			currentOrdinaryInputClass != RuntimeCandidateInputClassOrdinaryOnly {
			return nil, nil
		}
		// A few application-loop unit fixtures intentionally omit the host
		// security context. They can exercise provider mechanics, but cannot
		// mint a publishable ordinary result; production publication still
		// requires a valid exact context and terminal lease.
		if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(input.SecurityContext) != nil {
			return nil, nil
		}
		slot, _, err := appturn.CompileOrdinaryResultSlot(input.SecurityContext, candidateText)
		if err != nil {
			return nil, err
		}
		return &slot, nil
	}
	finishCandidateBoundary := func(
		candidateText string,
		candidateResult domainmodel.Result,
		candidateReportDeliveryCompleted bool,
		promoteBeforeTerminal bool,
	) (RuntimeAgentLoopResult, bool, error) {
		candidateTerminalContext, releaseCandidateTerminal, err := deps.AcquireCandidateTerminal(ctx, currentProviderStep)
		if err != nil {
			if ctx.Err() != nil {
				return RuntimeAgentLoopResult{AssistantText: candidateText, LastResult: candidateResult}, false, ctx.Err()
			}
			return RuntimeAgentLoopResult{AssistantText: candidateText, LastResult: candidateResult}, false,
				WrapHostBoundaryFailure(HostCandidateAuthorityFailure, err)
		}
		if candidateTerminalContext == nil || releaseCandidateTerminal == nil {
			if releaseCandidateTerminal != nil {
				releaseCandidateTerminal()
			}
			return RuntimeAgentLoopResult{AssistantText: candidateText, LastResult: candidateResult}, false,
				WrapHostBoundaryFailure(
					HostCandidateAuthorityFailure,
					errors.New("runtime candidate terminal claim is invalid"),
				)
		}
		if promoteBeforeTerminal {
			steeringBatch, promoteErr := deps.PromoteSteering(candidateTerminalContext)
			if promoteErr != nil {
				releaseCandidateTerminal()
				if ctx.Err() != nil {
					return RuntimeAgentLoopResult{AssistantText: candidateText, LastResult: candidateResult}, false, ctx.Err()
				}
				return RuntimeAgentLoopResult{AssistantText: candidateText, LastResult: candidateResult}, false,
					WrapHostBoundaryFailure(HostCandidateSteeringFailure, promoteErr)
			}
			prefix := []domainmodel.Message(nil)
			if strings.TrimSpace(candidateText) != "" {
				prefix = append(prefix, domainmodel.Message{Role: "assistant", Content: candidateText})
			}
			promoted, applyErr := applySteeringBatch(steeringBatch, prefix...)
			if applyErr != nil {
				releaseCandidateTerminal()
				return RuntimeAgentLoopResult{AssistantText: candidateText, LastResult: candidateResult}, false,
					WrapHostBoundaryFailure(HostCandidateSteeringFailure, applyErr)
			}
			if promoted {
				releaseCandidateTerminal()
				reportDeliveryCompleted = reportDeliveryCompleted || candidateReportDeliveryCompleted
				return RuntimeAgentLoopResult{}, true, nil
			}
		}
		ordinaryResult, err := compileOrdinaryCandidate(currentProviderStep, candidateText)
		if err != nil {
			releaseCandidateTerminal()
			return RuntimeAgentLoopResult{AssistantText: candidateText, LastResult: candidateResult}, false,
				WrapHostBoundaryFailure(HostCandidateProjectionFailure, err)
		}
		return RuntimeAgentLoopResult{
			AssistantText: candidateText, LastResult: candidateResult,
			ReportDeliveryCompleted:  reportDeliveryCompleted || candidateReportDeliveryCompleted,
			CandidateUsesCaseData:    currentProviderStep.UsesCaseDataAuthority(),
			CandidateOrdinaryWork:    currentProviderStep.OrdinaryWork,
			CandidateInputClass:      currentOrdinaryInputClass,
			OrdinaryResult:           ordinaryResult,
			CandidateTerminalContext: candidateTerminalContext,
			ReleaseCandidateTerminal: releaseCandidateTerminal,
		}, false, nil
	}
	for step := 0; step < maxSteps || stepLimitFinalizeNudged; step++ {
		providerCallSequence++
		stepLimitFinalizing := maxSteps > 0 && step >= maxSteps
		if err := deps.PauseChild(ctx); err != nil {
			return failAfterStartupStages(err)
		}
		stepText := ""
		stepThinking := ""
		stepThinkingSignature := ""
		streamedDeltas := false
		partialTextStarted := false
		partialToolStarted := false
		planActive := appplan.ToolActive(input.Mode, input.GUIPlan, input.Workspace)
		steeringBatch, err := deps.PromoteSteering(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return failAfterStartupStages(ctx.Err())
			}
			return failAfterStartupStages(err)
		}
		if _, err := applySteeringBatch(steeringBatch); err != nil {
			return failAfterStartupStages(err)
		}
		providerStep := currentProviderStep
		preparedProviderStep := providerStep
		if providerStep.CaseSourceUnavailable && providerStep.UsesCaseDataAuthority() {
			if !providerStep.OrdinaryWork {
				if err := flushStartupStages(); err != nil {
					return RuntimeAgentLoopResult{}, err
				}
				terminal, retry, terminalErr := finishCandidateBoundary(
					CaseFundSourceUnavailableAnswer(), domainmodel.Result{}, false, false,
				)
				if terminalErr != nil {
					return terminal, terminalErr
				}
				if retry {
					continue
				}
				return terminal, nil
			}
			preparedProviderStep = RuntimeProviderStep{
				Prompt: providerStep.Prompt, LogicalEffect: domainsecurity.LogicalEffectOrdinary,
				OrdinaryWork: true, CaseSourceUnavailable: true,
			}
		} else {
			preparedProviderStep, err = deps.PrepareProviderStep(ctx, providerStep)
			if err != nil {
				return failAfterStartupStages(err)
			}
		}
		if err := validatePreparedProviderStep(providerStep, preparedProviderStep); err != nil {
			return failAfterStartupStages(err)
		}
		if providerStep.UsesFundsDataAuthority() && preparedProviderStep.OrdinaryEffect() {
			caseSourceUnavailable = true
		}
		caseSourceUnavailable = caseSourceUnavailable || preparedProviderStep.CaseSourceUnavailable
		if providerStep.LogicalEffect != preparedProviderStep.LogicalEffect {
			if privateProtocolObserved {
				if err := compilePrivateProtocolSafeHistory(); err != nil {
					return failAfterStartupStages(err)
				}
			}
			var isolated bool
			ordinaryLaneMessages, isolated = runtimeOrdinaryLaneForPromptV1(
				ordinaryLaneMessages, providerStep.Prompt, false,
			)
			if !isolated {
				return failAfterStartupStages(TurnFailureError{
					Message: CaseFundSourceUnavailableAnswer(), Code: "tool_source_unavailable", Severity: "error",
				})
			}
			messages = appmodel.CloneProviderMessages(ordinaryLaneMessages)
			pendingPrivateProtocolCapsules = nil
			privateProtocolObserved = false
			currentOrdinaryInputClass = RuntimeCandidateInputClassOrdinaryOnly
		}
		providerStep = preparedProviderStep
		currentProviderStep = preparedProviderStep
		if deps.ResolveProviderIntent != nil {
			intent, resolveErr := deps.ResolveProviderIntent(ctx)
			if resolveErr != nil {
				return failAfterStartupStages(resolveErr)
			}
			if intent.ProviderConfig.APIKey != "" || strings.TrimSpace(intent.ProviderID) == "" ||
				strings.TrimSpace(intent.Model) == "" || strings.TrimSpace(intent.ProviderConfig.BaseURL) == "" {
				return failAfterStartupStages(errors.New("provider execution intent is invalid"))
			}
			if input.ProviderConfig.ProviderID != intent.ProviderConfig.ProviderID ||
				input.ProviderConfig.EndpointFormat != intent.ProviderConfig.EndpointFormat ||
				input.ProviderConfig.Model != intent.ProviderConfig.Model ||
				input.ProviderConfig.ReasoningProtocol != intent.ProviderConfig.ReasoningProtocol {
				if privateProtocolObserved || len(pendingPrivateProtocolCapsules) > 0 {
					if err := compilePrivateProtocolSafeHistory(); err != nil {
						return failAfterStartupStages(err)
					}
				}
			}
			input.ProviderConfig = intent.ProviderConfig
			input.ProviderID = intent.ProviderID
			input.Model = intent.Model
			input.Effort = intent.Effort
			input.ProviderCustomRequestShape = domainmodel.CustomEndpointRequestShape(intent.ProviderConfig.BaseURL)
			privateProtocolKind = appmodel.PrivateProtocolReasoningKindForProvider(
				input.ProviderConfig, input.ProviderConfig.EndpointFormat, input.ProviderCustomRequestShape,
			)
			deepSeekPrivateProtocol = privateProtocolKind == "deepseek-chat-completions"
			configuredReasoningProtocol = strings.TrimSpace(input.ProviderConfig.ReasoningProtocol)
			privateProtocolSafeHistoryRequired = deepSeekPrivateProtocol ||
				appmodel.PrivateProtocolRequired(configuredReasoningProtocol, input.Effort)
		}
		catalog := deps.ResolveToolCatalog(planActive, providerStep)
		resolvedSystemPrompt := strings.TrimSpace(catalog.SystemPrompt)
		promptRoute := catalog.PromptRoute
		mcpAdvertisements := catalog.Advertisements
		liveMCPToolNames := toolcatalogapp.MCPToolNamesFromAdvertisementsV1(mcpAdvertisements)
		knownBuiltinTools := make(map[string]bool, len(catalog.Schemas))
		for _, schema := range catalog.Schemas {
			name := strings.TrimSpace(schema.Name)
			if name == "" || strings.TrimSpace(schema.Source) == "mcp" || strings.HasPrefix(name, "mcp__") {
				continue
			}
			knownBuiltinTools[name] = true
		}
		knownMCPTools := toolcatalogapp.ToolNameSet(liveMCPToolNames)
		toolSchemas := appplan.FilterModeToolSchemas(catalog.Schemas, planActive, createPlanSatisfied, step, input.CreatePlanToolName())
		if err := toolcatalogapp.ValidateProviderVisibleToolSchemas(toolSchemas); err != nil {
			return failAfterStartupStages(err)
		}
		if err := toolcatalogapp.ValidateDelegatedToolManifestForDispatchV1(input.DelegatedToolManifest, input.ToolScope, toolSchemas, mcpAdvertisements); err != nil {
			return failAfterStartupStages(err)
		}
		promptRoute = toolcatalogapp.PromptRouteForAdvertisedTools(promptRoute, toolSchemas)
		advertisedTools := toolcatalogapp.ToolSchemaNameSet(toolSchemas)
		toolManifestHash := toolcatalogapp.ToolSchemaHash(toolSchemas)
		advertisedNameSetSortedHash := toolcatalogapp.ToolSchemaNameSetHash(toolSchemas)
		privateProtocolRequestShapeChanged := runtimeProviderSystemPromptChanged(
			messages,
			resolvedSystemPrompt,
		)
		for _, capsule := range pendingPrivateProtocolCapsules {
			changed, shapeErr := privateProtocolSession.AnthropicThinkingRequestShapeChanged(
				capsule,
				promptRoute,
				toolManifestHash,
			)
			if shapeErr != nil {
				return failAfterStartupStages(shapeErr)
			}
			privateProtocolRequestShapeChanged = privateProtocolRequestShapeChanged || changed
		}
		if len(pendingPrivateProtocolCapsules) > 0 && privateProtocolRequestShapeChanged {
			if err := compilePrivateProtocolSafeHistory(); err != nil {
				return failAfterStartupStages(err)
			}
		}
		if resolvedSystemPrompt != "" {
			messages = withRuntimeProviderSystemPrompt(messages, resolvedSystemPrompt)
		}
		advertisedToolScope := make([]string, 0, len(toolSchemas))
		for _, schema := range toolSchemas {
			if name := strings.TrimSpace(schema.Name); name != "" {
				advertisedToolScope = append(advertisedToolScope, name)
			}
		}
		if (privateProtocolSafeHistoryRequired || privateProtocolObserved) &&
			len(pendingPrivateProtocolCapsules) == 0 {
			if err := compilePrivateProtocolSafeHistory(); err != nil {
				return failAfterStartupStages(err)
			}
		}
		if step == 0 {
			memoryCount := 0
			if deps.BackgroundMemoryCount != nil {
				memoryCount = deps.BackgroundMemoryCount()
			}
			pendingStartupStages = append(pendingStartupStages,
				newStage("input_routed", map[string]any{
					"promptRoute":   promptRoute,
					"toolCount":     float64(len(toolSchemas)),
					"planActive":    planActive,
					"subagentDepth": float64(input.SubagentDepth),
				}),
				newStage("input_compressed", RuntimePipelineMessageDetails(messages)),
				newStage("input_remembered", map[string]any{
					"memoryCount":             float64(memoryCount),
					"contextInstructionCount": float64(RuntimeContextInstructionCount(messages)),
				}),
			)
		}
		var anthropicReplay *domainmodel.AnthropicThinkingReplay
		privateProtocolReplays := make([]*domainmodel.AnthropicThinkingReplay, 0, len(pendingPrivateProtocolCapsules))
		for _, capsule := range pendingPrivateProtocolCapsules {
			replay, consumeErr := appmodel.ConsumeAnthropicPrivateProtocol(appmodel.AnthropicPrivateProtocolConsumeInput{
				Session: privateProtocolSession, Capsule: capsule, ProviderConfig: input.ProviderConfig,
				EndpointFormat: input.ProviderConfig.EndpointFormat, CustomRequestShape: input.ProviderCustomRequestShape,
				ContextDigest: input.SecurityContext.ContextDigest, PromptRoute: promptRoute,
				ToolManifestHash: toolManifestHash, Sequence: providerCallSequence, Messages: messages,
			})
			if consumeErr != nil {
				return failAfterStartupStages(consumeErr)
			}
			privateProtocolReplays = append(privateProtocolReplays, replay)
			if !deepSeekPrivateProtocol && anthropicReplay == nil {
				anthropicReplay = replay
			}
		}
		systemPrompt := resolvedSystemPrompt
		if systemPrompt == "" {
			systemPrompt = strings.TrimSpace(input.SystemPrompt)
		}
		if systemPrompt == "" {
			systemPrompt = strings.TrimSpace(input.FallbackSystemPrompt)
		}
		attachmentPlanDigest := input.AttachmentPlanDigest
		prepareAttachments := input.HasAttachments && (!input.AdditionalCaseDataEffect || providerStep.UsesCaseDataAuthority())
		if !prepareAttachments {
			attachmentPlanDigest = ""
		}
		providerRequest := domainmodel.Request{
			ProviderID:                  input.ProviderID,
			Family:                      input.ProviderConfig.Family,
			EndpointFormat:              input.ProviderConfig.EndpointFormat,
			BaseURL:                     input.ProviderConfig.BaseURL,
			ProxyURL:                    input.ProviderConfig.ProxyURL,
			APIKey:                      input.ProviderConfig.APIKey,
			Model:                       input.Model,
			Route:                       promptRoute,
			ReasoningEffort:             input.Effort,
			ReasoningProtocol:           input.ProviderConfig.ReasoningProtocol,
			MaxOutputTokens:             input.OutputTokenBudget,
			SystemPrompt:                systemPrompt,
			Messages:                    messages,
			PrivateAttachmentPlanDigest: attachmentPlanDigest,
			AnthropicThinkingReplay:     anthropicReplay,
			DeepSeekReasoningReplays:    privateProtocolReplays,
			Tools:                       toolSchemas,
			Pricing:                     input.ProviderConfig.Pricing,
			PrivateProviderTelemetry: &domainmodel.ProviderTelemetryBindingV1{
				SecurityContext: input.SecurityContext, UsageSource: domaincache.NormalizeProviderUsageSourceV1(input.UsageSource),
				ChildRunID: input.ChildRunID, Channel: domaincache.ProviderChannelPrimary, LogicalSequence: providerCallSequence,
			},
		}
		providerRequestToolManifestHash := toolcatalogapp.ToolSchemaHash(providerRequest.Tools)
		providerSendPending := false
		providerSendPairs := 0
		providerAttemptPairStart := 0
		providerAttemptStarted := false
		providerAttemptClosed := false
		providerRequest.OnPipelineStage = func(stage domainmodel.PipelineStage) error {
			switch stage.Stage {
			case "provider_admission_rejected":
				return flushStartupStages(stage)
			case "pre_send":
				if providerSendPending {
					return errors.New("provider pre-send pipeline stage is already pending")
				}
				// Persist the attempt marker before the provider transport begins.
				// A crash after the HTTP request is sent must leave a durable
				// indeterminate-attempt record rather than look never-sent.
				if err := flushStartupStagesWithCriticalStage(stage); err != nil {
					return err
				}
				providerSendPending = true
				return nil
			case "post_send":
				if !providerSendPending {
					return errors.New("provider send pipeline stage pair is invalid")
				}
				if err := loopEvents.PipelineStageAtomic(input.ThreadID, input.TurnID, stage); err != nil {
					return err
				}
				providerSendPending = false
				providerSendPairs++
				return nil
			default:
				return loopEvents.PipelineStage(input.ThreadID, input.TurnID, stage.Stage, stage.Details, stage.At, stage.Trace)
			}
		}
		providerCallbacks := ProviderStreamCallbacks{
			AcquireProviderAttempt: func(attemptCtx context.Context, attempt int) (context.Context, func(), error) {
				return deps.AcquireProviderAttempt(attemptCtx, attempt, providerStep)
			},
			BeforeProviderInvocation: func(int) error {
				if providerSendPending {
					return providerPipelineContractError{reason: "provider durable send pipeline stage pair is incomplete"}
				}
				if providerAttemptStarted && !providerAttemptClosed {
					return providerPipelineContractError{reason: "provider retry followed an unclosed durable dispatch attempt"}
				}
				providerAttemptStarted = true
				providerAttemptClosed = false
				providerAttemptPairStart = providerSendPairs
				return nil
			},
			AfterProviderAttempt: func(_ int, providerErr error) error {
				if !providerAttemptStarted {
					return providerPipelineContractError{reason: "provider dispatch attempt was not opened"}
				}
				providerAttemptClosed = true
				if providerSendPending {
					return providerPipelineContractError{reason: "provider durable send pipeline stage pair is incomplete"}
				}
				if providerSendPairs != providerAttemptPairStart {
					return nil
				}
				dispatchState, known := providerDispatchStateFromErrorV1(providerErr)
				if known && dispatchState == domaincache.ProviderDispatchStateNotSent {
					return nil
				}
				return providerPipelineContractError{reason: "provider attempt lacks a durable send pipeline pair or trusted not-sent disposition"}
			},
			OnTextChunk: func(domainmodel.Chunk) error {
				// Provider prose remains a private draft until the terminal
				// publication lane validates the complete candidate.
				return nil
			},
			OnProviderRetrying: func(attempt int, maxAttempts int, cause error) error {
				if err := flushStartupStages(); err != nil {
					return err
				}
				return loopEvents.ProviderRetrying(input.ThreadID, input.TurnID, attempt, maxAttempts, cause)
			},
		}
		if deps.PrepareProviderAttempt != nil && (prepareAttachments || deps.ResolveProviderIntent != nil) {
			providerCallbacks.PrepareProviderAttempt = func(attemptCtx context.Context, attempt int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				return deps.PrepareProviderAttempt(attemptCtx, attempt, request, promptRoute, toolManifestHash, providerCallSequence)
			}
		}
		stream, err := StreamProviderWithRetry(ctx, ProviderStreamInput{
			Provider:            deps.Provider,
			Request:             providerRequest,
			SecurityContext:     input.SecurityContext,
			HostEntitySelection: providerStep.HostEntitySelection,
			OrdinaryEffect:      providerStep.OrdinaryEffect(),
			Callbacks:           providerCallbacks,
			ContinuationAuthority: ProviderContinuationAuthorityInput{
				Service: deps.PendingWork, SecurityContext: input.SecurityContext, References: providerContinuationRefs,
				ProviderConfig: input.ProviderConfig, PromptRoute: promptRoute, ToolManifestHash: toolManifestHash,
				Sequence: providerCallSequence,
			},
		})
		if providerSendPending {
			stream = sealFailedProviderStreamOutput(stream)
			err = errors.Join(err, ProviderStreamCallbackError{Err: providerPipelineContractError{reason: "provider durable send pipeline stage pair is incomplete"}})
		} else if (providerAttemptStarted && !providerAttemptClosed) || (err == nil && !providerAttemptStarted) {
			stream = sealFailedProviderStreamOutput(stream)
			err = errors.Join(err, ProviderStreamCallbackError{Err: providerPipelineContractError{reason: "provider completed without a closed durable dispatch attempt"}})
		}
		if len(pendingStartupStages) > 0 {
			if flushErr := flushStartupStages(); flushErr != nil {
				stream = sealFailedProviderStreamOutput(stream)
				err = errors.Join(err, flushErr)
			}
		}
		result := stream.Result
		providerCallObservations = append(providerCallObservations, result.CacheObservations...)
		result.CacheObservations = append([]domaincache.ProviderCallObservationV1(nil), providerCallObservations...)
		stepText = stream.Text
		stepThinking = stream.Reasoning
		stepThinkingSignature = stream.ReasoningSignature
		streamedDeltas = stream.StreamedDeltas
		partialTextStarted = stream.PartialTextStarted
		partialToolStarted = stream.PartialToolStarted
		if err != nil {
			// Provider chunks are attempt-private transport material. Failure,
			// callback failure, recovery, cancellation, and future callers may
			// retain only the already-separated public text plus bounded result
			// telemetry; raw text/reasoning chunks never cross this return edge.
			result.Chunks = nil
			if contextErr := ctx.Err(); contextErr != nil {
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: result}, errors.Join(err, contextErr)
			}
			var callbackErr ProviderStreamCallbackError
			if errors.As(err, &callbackErr) {
				if ctx.Err() != nil {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: result}, errors.Join(err, ctx.Err())
				}
				if providerStep.UsesCaseDataAuthority() && providerStep.OrdinaryWork {
					if _, recoverable := recoverableProviderPreSendProtectedLaneFailureV1(callbackErr.Err); recoverable {
						caseSourceUnavailable = true
						currentProviderStep = RuntimeProviderStep{
							Prompt: providerStep.Prompt, LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
							CaseSourceUnavailable: true,
						}
						var isolated bool
						ordinaryLaneMessages, isolated = runtimeOrdinaryLaneForPromptV1(
							ordinaryLaneMessages, providerStep.Prompt, false,
						)
						if !isolated {
							return failAfterStartupStages(TurnFailureError{
								Message: CaseFundSourceUnavailableAnswer(), Code: "tool_source_unavailable", Severity: "error",
							})
						}
						currentOrdinaryInputClass = RuntimeCandidateInputClassOrdinaryOnly
						if privateProtocolObserved {
							if err := compilePrivateProtocolSafeHistory(); err != nil {
								return failAfterStartupStages(err)
							}
						}
						messages = appmodel.CloneProviderMessages(ordinaryLaneMessages)
						providerContinuationRefs = nil
						pendingPrivateProtocolCapsules = nil
						privateProtocolObserved = false
						if maxSteps > 0 {
							step--
						}
						continue
					}
				}
				// A provider transport failure can be joined with a host callback
				// failure. The callback still owns this return path, but its closed
				// provider diagnostic must remain observable before the terminal batch.
				if len(ProviderErrorDiagnostic(err)) > 0 {
					if recordErr := loopEvents.ProviderError(input.ThreadID, input.TurnID, err); recordErr != nil {
						return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: result}, recordErr
					}
				}
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: result}, err
			}
			if streamedDeltas && !CasePublicationRequired(input.SecurityContext) && streamRecoveries < MaxInterruptedStreamRecoveries && InterruptedStreamCanRecoverWithContext(ctx, err) {
				stickyRecovery = MergeRuntimeTerminalRecoveryKind(stickyRecovery, RuntimeTerminalRecoveryApplied)
				streamRecoveries++
				if err := loopEvents.StreamInterruptedRecovery(input.ThreadID, input.TurnID, streamRecoveries, MaxInterruptedStreamRecoveries, partialToolStarted, err); err != nil {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: result}, err
				}
				messages = append(messages, domainmodel.Message{
					Role:    "user",
					Content: InterruptedStreamRecoveryPrompt(partialTextStarted, partialToolStarted),
				})
				if maxSteps > 0 {
					step--
				}
				continue
			}
			if PublicFailureForError(err).Code() != "context_window_hard_limit" {
				if recordErr := loopEvents.ProviderError(input.ThreadID, input.TurnID, err); recordErr != nil {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: result}, recordErr
				}
			}
			return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: result}, err
		}
		if err := loopEvents.PipelineStage(input.ThreadID, input.TurnID, "response_received", map[string]any{
			"stopReason":          result.Usage.FinishReason,
			"toolCallCount":       float64(ToolCallCount(result.Chunks)),
			"streamCompleted":     result.StreamCompleted,
			"durationMs":          float64(result.DurationMs),
			"firstTokenLatencyMs": float64(result.FirstTokenLatencyMs),
		}, time.Now().UTC(), nil); err != nil {
			return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: result},
				WrapHostBoundaryFailure(HostResponseEventFailure, err)
		}
		streamRecoveries = 0
		last = result
		last.Chunks = nil
		stepOutput, err := CollectStepOutput(StepOutputInput{
			ThreadID: input.ThreadID, TurnID: input.TurnID, Stream: stream, Events: loopEvents,
			SuppressAssistantTextEvents: true,
		})
		if err != nil {
			return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, err
		}
		stepText = stepOutput.Text
		stepThinking = stepOutput.Reasoning
		stepThinkingSignature = stepOutput.ReasoningSignature
		toolCalls := stepOutput.ToolCalls
		if err := deps.PauseChild(ctx); err != nil {
			return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last},
				WrapHostBoundaryFailure(HostCandidateLifecycleFailure, err)
		}
		if len(toolCalls) == 0 {
			if required := strings.TrimSpace(input.RequiredFinalToolName); required != "" {
				return RuntimeAgentLoopResult{LastResult: last}, TurnFailureError{
					Message: "foreground child completed without required " + required + " tool",
					Code:    "subagent_required_final_tool_missing", Severity: "error",
				}
			}
			if strings.TrimSpace(stepText) == "" {
				if stepLimitFinalizing {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, StepLimitExceededError(maxSteps)
				}
				if CasePublicationRequired(input.SecurityContext) {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, domainfailure.NewError(domainfailure.CodeProviderEmptyFinal, nil)
				}
				if emptyFinalRecoveries < maxEmptyFinalRecoveries {
					stickyRecovery = MergeRuntimeTerminalRecoveryKind(stickyRecovery, RuntimeTerminalRecoveryApplied)
					if err := loopEvents.EmptyFinalRecovery(input.ThreadID, input.TurnID, emptyFinalRecoveries+1, maxEmptyFinalRecoveries); err != nil {
						return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, err
					}
					emptyFinalRecoveries++
					messages = append(messages, domainmodel.Message{
						Role:    "user",
						Content: "The previous assistant response was empty. Provide a concise final answer to the user's last request.",
					})
					if maxSteps > 0 {
						step--
					}
					continue
				}
				if err := loopEvents.EmptyFinalRecoveryExhausted(input.ThreadID, input.TurnID, emptyFinalRecoveries, maxEmptyFinalRecoveries); err != nil {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, err
				}
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, domainfailure.NewError(domainfailure.CodeProviderEmptyFinal, nil)
			}
			if !stepLimitFinalizing && planActive && !createPlanSatisfied {
				if appplan.IsClarifyingQuestion(stepText) {
					candidate, steered, err := finishCandidateBoundary(stepText, last, false, true)
					if err != nil || !steered {
						return candidate, err
					}
					if maxSteps > 0 {
						step--
					}
					continue
				}
				if !appplan.CanMaterializeInSandbox(input.NormalizedSandboxMode) {
					candidate, steered, err := finishCandidateBoundary(stepText, last, false, true)
					if err != nil || !steered {
						return candidate, err
					}
					if maxSteps > 0 {
						step--
					}
					continue
				}
				if CasePublicationRequired(input.SecurityContext) {
					candidate, steered, err := finishCandidateBoundary(stepText, last, false, true)
					if err != nil || !steered {
						return candidate, err
					}
					if maxSteps > 0 {
						step--
					}
					continue
				}
				decision, guardErr := appturn.GuardGeneralOutput(input.SecurityContext, stepText)
				if guardErr != nil {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, guardErr
				}
				if decision.Blocked {
					candidate, steered, err := finishCandidateBoundary(decision.Text, last, false, true)
					if err != nil || !steered {
						return candidate, err
					}
					if maxSteps > 0 {
						step--
					}
					continue
				}
				if deps.MaterializeCreatePlan == nil {
					return RuntimeAgentLoopResult{AssistantText: decision.Text, LastResult: last}, errors.New("runtime plan materialization authority is unavailable")
				}
				responsePrivateProtocolObserved, observeErr := appmodel.ObservePrivateProtocolResponse(
					appmodel.PrivateProtocolResponseObservationInput{
						ProviderConfig: input.ProviderConfig, EndpointFormat: input.ProviderConfig.EndpointFormat,
						CustomRequestShape: input.ProviderCustomRequestShape, Thinking: stepThinking,
						Signature: stepThinkingSignature, Effort: input.Effort,
					},
				)
				if observeErr != nil {
					return RuntimeAgentLoopResult{AssistantText: decision.Text, LastResult: last}, observeErr
				}
				privateProtocolObserved = privateProtocolObserved || responsePrivateProtocolObserved
				materialized, err := deps.MaterializeCreatePlan(
					ctx,
					messages,
					decision.Text,
					privateProtocolObserved,
				)
				if err != nil {
					return RuntimeAgentLoopResult{AssistantText: decision.Text, LastResult: last}, err
				}
				messages = materialized.Messages
				providerContinuationRefs = []domainsecurity.SettledToolReference{materialized.ContinuationRef}
				if !materialized.IsError {
					createPlanSatisfied = true
				}
				messages = append(messages, domainmodel.Message{
					Role:       "tool",
					Name:       input.CreatePlanToolName(),
					ToolCallID: appplan.MaterializedCallID(input.SecurityContext.ContextDigest),
					Content:    appmodel.ToolResultContent(materialized.Output),
				})
				if privateProtocolSafeHistoryRequired || privateProtocolObserved ||
					len(pendingPrivateProtocolCapsules) > 0 {
					if err := compilePrivateProtocolSafeHistory(); err != nil {
						return RuntimeAgentLoopResult{AssistantText: decision.Text, LastResult: last}, err
					}
				}
				continue
			}
			candidate, steered, err := finishCandidateBoundary(stepText, last, false, true)
			if err != nil || !steered {
				return candidate, err
			}
			if maxSteps > 0 {
				step--
			}
			continue
		}
		if stepLimitFinalizing {
			return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, StepLimitExceededError(maxSteps)
		}
		preclaimedTerminalTool := false
		if strings.TrimSpace(input.RequiredFinalToolName) != "" {
			preclaimedTerminalTool = true
		}
		if !preclaimedTerminalTool {
			for _, call := range toolCalls {
				if strings.TrimSpace(call.Name) == toolcatalogapp.ReportDeliveryToolName {
					preclaimedTerminalTool = true
					break
				}
			}
		}
		var terminalToolContext context.Context
		var releaseTerminalTool func()
		if preclaimedTerminalTool {
			terminalToolContext, releaseTerminalTool, err = deps.AcquireCandidateTerminal(ctx, providerStep)
			if err != nil {
				if ctx.Err() != nil {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, ctx.Err()
				}
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, err
			}
			if terminalToolContext == nil || releaseTerminalTool == nil {
				if releaseTerminalTool != nil {
					releaseTerminalTool()
				}
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last},
					errors.New("runtime candidate terminal claim is invalid")
			}
			steeringBatch, promoteErr := deps.PromoteSteering(terminalToolContext)
			if promoteErr != nil {
				releaseTerminalTool()
				if ctx.Err() != nil {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, ctx.Err()
				}
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, promoteErr
			}
			promoted, applyErr := applySteeringBatch(steeringBatch, domainmodel.Message{
				Role: "assistant", Content: terminalToolSupersededBoundary,
			})
			if applyErr != nil {
				releaseTerminalTool()
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, applyErr
			}
			if promoted {
				releaseTerminalTool()
				if maxSteps > 0 {
					step--
				}
				continue
			}
		}
		terminalToolFailureResult := func(text string, result domainmodel.Result) RuntimeAgentLoopResult {
			return RuntimeAgentLoopResult{
				AssistantText: text, LastResult: result,
				CandidateTerminalContext: terminalToolContext,
				ReleaseCandidateTerminal: releaseTerminalTool,
			}
		}
		terminalToolResult := func(text string, result domainmodel.Result, reportCompleted bool) (RuntimeAgentLoopResult, error) {
			terminal := RuntimeAgentLoopResult{
				AssistantText: text, LastResult: result,
				ReportDeliveryCompleted:  reportDeliveryCompleted || reportCompleted,
				CandidateUsesCaseData:    providerStep.UsesCaseDataAuthority(),
				CandidateOrdinaryWork:    providerStep.OrdinaryWork,
				CandidateInputClass:      currentOrdinaryInputClass,
				CandidateTerminalContext: terminalToolContext,
				ReleaseCandidateTerminal: releaseTerminalTool,
			}
			ordinaryResult, err := compileOrdinaryCandidate(providerStep, text)
			if err != nil {
				return terminal, err
			}
			terminal.OrdinaryResult = ordinaryResult
			return terminal, nil
		}
		assistantMessage := domainmodel.Message{Role: "assistant", Content: stepText, ToolCalls: toolCalls}
		privateProtocolSessionBeforeToolStep := privateProtocolSession
		privateProtocolObservedBeforeToolStep := privateProtocolObserved
		var nextAnthropicCapsule *domainmodel.AnthropicThinkingCapsule
		nextPrivateProtocolCapsules := append(
			[]*domainmodel.AnthropicThinkingCapsule(nil),
			pendingPrivateProtocolCapsules...,
		)
		if len(toolCalls) > 0 {
			issued, issueErr := appmodel.IssueAnthropicPrivateProtocol(appmodel.AnthropicPrivateProtocolIssueInput{
				Session: privateProtocolSession, ProviderConfig: input.ProviderConfig,
				EndpointFormat: input.ProviderConfig.EndpointFormat, CustomRequestShape: input.ProviderCustomRequestShape,
				ContextDigest: input.SecurityContext.ContextDigest, PromptRoute: promptRoute,
				ToolManifestHash: toolManifestHash, Sequence: providerCallSequence + 1, Messages: messages,
				AssistantMessageIndex: len(messages), AssistantMessage: assistantMessage, Thinking: stepThinking,
				Signature: stepThinkingSignature, ToolCallCount: len(toolCalls), Effort: input.Effort,
			})
			if issueErr != nil {
				if preclaimedTerminalTool {
					return terminalToolFailureResult(stepText, last), issueErr
				}
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, issueErr
			}
			privateProtocolSession, nextAnthropicCapsule = issued.Session, issued.Capsule
			if nextAnthropicCapsule != nil {
				nextPrivateProtocolCapsules = append(nextPrivateProtocolCapsules, nextAnthropicCapsule)
				privateProtocolObserved = true
			}
		}
		messagesBeforeToolStep := len(messages)
		messages = append(messages, assistantMessage)
		toolStepContext := ctx
		if preclaimedTerminalTool {
			toolStepContext = terminalToolContext
		}
		toolStep, err := RunToolStep(toolStepContext, ToolStepInput{
			ThreadID:                        input.ThreadID,
			TurnID:                          input.TurnID,
			ProviderConfig:                  input.ProviderConfig,
			ProviderID:                      input.ProviderID,
			Model:                           input.Model,
			Effort:                          input.Effort,
			Workspace:                       input.Workspace,
			Prompt:                          providerStep.Prompt,
			LogicalEffect:                   providerStep.LogicalEffect,
			OrdinaryWork:                    providerStep.OrdinaryWork,
			CaseSourceUnavailable:           caseSourceUnavailable,
			OrdinaryResultInputIsolated:     currentOrdinaryInputClass == RuntimeCandidateInputClassOrdinaryOnly,
			AttachmentIDs:                   append([]string(nil), input.AttachmentIDs...),
			AttachmentPlanDigest:            input.AttachmentPlanDigest,
			WorkspaceCheckpointID:           input.WorkspaceCheckpointID,
			Mode:                            input.Mode,
			GUIPlan:                         input.GUIPlan,
			ApprovalPolicy:                  input.ApprovalPolicy,
			SandboxMode:                     input.SandboxMode,
			DisableUserInput:                input.DisableUserInput,
			MaxModelSteps:                   input.MaxModelSteps,
			EffectiveMaxModelSteps:          effectiveMaxSteps,
			Messages:                        messages,
			PrivateProtocolSession:          privateProtocolSession,
			AnthropicCapsule:                nextAnthropicCapsule,
			PrivateProtocolCapsules:         nextPrivateProtocolCapsules,
			ProviderNamespace:               appmodel.ProviderNamespace(input.UsageSource, input.ChildRunID, providerCallSequence, input.DelegatedToolManifest),
			TerminalRecoveryKind:            stickyRecovery,
			ToolCalls:                       toolCalls,
			ToolSchemas:                     toolSchemas,
			ToolScope:                       advertisedToolScope,
			SubagentDepth:                   input.SubagentDepth,
			AdvertisedTools:                 advertisedTools,
			PromptRoute:                     promptRoute,
			LoopStep:                        step,
			AdvertisedToolCount:             len(advertisedTools),
			AdvertisedToolManifestHash:      toolManifestHash,
			AdvertisedNameSetSortedHash:     advertisedNameSetSortedHash,
			ProviderRequestToolManifestHash: providerRequestToolManifestHash,
			KnownBuiltinTools:               knownBuiltinTools,
			KnownMCPTools:                   knownMCPTools,
			LiveMCPTools:                    toolcatalogapp.ToolNameSet(liveMCPToolNames),
			MCPConnectionEpochs:             toolcatalogapp.MCPConnectionEpochsFromAdvertisementsV1(mcpAdvertisements),
			MCPServerIdentities:             toolcatalogapp.MCPServerIdentitiesFromAdvertisementsV1(mcpAdvertisements),
			MCPReadOnlyPolicies:             toolcatalogapp.MCPReadOnlyPoliciesFromAdvertisementsV1(mcpAdvertisements),
			CreatePlanToolName:              input.CreatePlanToolName(),
			RequiredFinalToolName:           input.RequiredFinalToolName,
			SecurityContext:                 input.SecurityContext,
			HostEntitySelection:             providerStep.HostEntitySelection,
			State: ToolStepState{
				CreatePlanSatisfied:   createPlanSatisfied,
				RepeatSuccessCounts:   repeatSuccessCounts,
				FailureStormSignature: failureStormSignature,
				FailureStormCount:     failureStormCount,
			},
			Driver: deps.ToolDriver,
		})
		if err != nil && toolStep.providerCorrectableToolName != "" {
			var failure TurnFailureError
			if errors.As(err, &failure) && strings.TrimSpace(failure.Code) == "validation_error" {
				providerToolValidationFailures++
				if providerToolValidationFailures >= InvalidToolArgumentsHardStopThreshold {
					reason := providerCorrectableToolFailureReason(failure)
					message := "Turn stopped because the provider repeatedly issued tool calls that did not satisfy the advertised JSON Schema."
					details := map[string]any{
						"toolName":   toolStep.providerCorrectableToolName,
						"stormCount": float64(providerToolValidationFailures),
					}
					if reason == duplicateCreatePlanValidationCode {
						message = "Turn stopped because the provider repeatedly issued Plan-mode batches with more than one create_plan call."
						details["reason"] = reason
					}
					storm := TurnFailureError{
						Message:  message,
						Code:     "tool_invalid_arguments_storm",
						Severity: "error",
						Details:  details,
					}
					if preclaimedTerminalTool {
						return terminalToolFailureResult("", last), storm
					}
					return RuntimeAgentLoopResult{LastResult: last}, storm
				}
				if preclaimedTerminalTool {
					releaseTerminalTool()
				}
				messages = messages[:messagesBeforeToolStep]
				privateProtocolSession = privateProtocolSessionBeforeToolStep
				privateProtocolObserved = privateProtocolObservedBeforeToolStep
				if err := loopEvents.LoopGuard(
					input.ThreadID,
					input.TurnID,
					toolStep.providerCorrectableToolName,
					providerToolValidationFailures,
					providerCorrectableToolFailureReason(failure),
				); err != nil {
					return RuntimeAgentLoopResult{LastResult: last}, err
				}
				messages = append(messages, domainmodel.Message{
					Role: "user", Content: providerToolArgumentValidationRecoveryPromptForFailure(
						failure,
						toolStep.providerCorrectableToolName,
						toolSchemas,
					),
				})
				if maxSteps > 0 {
					step--
				}
				continue
			}
		}
		if signal := toolStep.ProtectedLaneUnavailable; signal != nil {
			code, codeOK := recoverableProtectedLaneCodeV1(signal.Code)
			errCode, errOK := "", true
			if err != nil {
				errCode, errOK = RecoverableProtectedLaneFailureCodeV1(err)
			}
			if !providerStep.UsesCaseDataAuthority() || !providerStep.OrdinaryWork || !codeOK ||
				!errOK || (err != nil && errCode != code) {
				if preclaimedTerminalTool {
					releaseTerminalTool()
				}
				if err != nil {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, err
				}
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last},
					errors.New("protected tool lane returned an invalid continuation signal")
			}
			if preclaimedTerminalTool {
				releaseTerminalTool()
			}
			caseSourceUnavailable = true
			currentProviderStep = RuntimeProviderStep{
				Prompt: providerStep.Prompt, LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
				CaseSourceUnavailable: true,
			}
			var isolated bool
			ordinaryLaneMessages, isolated = runtimeOrdinaryLaneForPromptV1(
				ordinaryLaneMessages, providerStep.Prompt, false,
			)
			if !isolated {
				return failAfterStartupStages(TurnFailureError{
					Message: CaseFundSourceUnavailableAnswer(), Code: "tool_source_unavailable", Severity: "error",
				})
			}
			currentOrdinaryInputClass = RuntimeCandidateInputClassOrdinaryOnly
			ordinaryToolMessages := appmodel.CloneProviderMessages(toolStep.Messages[messagesBeforeToolStep:])
			for index := range ordinaryToolMessages {
				if ordinaryToolMessages[index].Role == "assistant" && len(ordinaryToolMessages[index].ToolCalls) > 0 {
					// Provider prose attached to a protected/mixed tool request is
					// an untrusted draft. Only the ordinary call/result structure may
					// be replayed on the typed ordinary lane.
					ordinaryToolMessages[index].Content = ""
				}
			}
			messages = append(
				appmodel.CloneProviderMessages(ordinaryLaneMessages),
				privacyprojectionapp.OrdinaryOnlyProviderMessagesV1(ordinaryToolMessages)...,
			)
			providerContinuationRefs = append(
				[]domainsecurity.SettledToolReference(nil),
				toolStep.OrdinarySettledToolReferences...,
			)
			pendingPrivateProtocolCapsules = nil
			if privateProtocolObserved {
				if err := compilePrivateProtocolSafeHistory(); err != nil {
					return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, err
				}
			}
			ordinaryLaneMessages = appmodel.CloneProviderMessages(messages)
			createPlanSatisfied = toolStep.State.CreatePlanSatisfied
			repeatSuccessCounts = toolStep.State.RepeatSuccessCounts
			failureStormSignature = toolStep.State.FailureStormSignature
			failureStormCount = toolStep.State.FailureStormCount
			if maxSteps > 0 {
				step--
			}
			continue
		}
		if err != nil {
			if preclaimedTerminalTool {
				return terminalToolFailureResult(stepText, last), err
			}
			return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, err
		}
		if toolStep.ProviderContinuationBlocked {
			messages = messages[:messagesBeforeToolStep]
			if preclaimedTerminalTool {
				return terminalToolResult(
					strings.TrimSpace(toolStep.ProviderContinuationBoundary), last,
					toolStep.ReportDeliveryCompleted,
				)
			}
			candidate, steered, err := finishCandidateBoundary(
				strings.TrimSpace(toolStep.ProviderContinuationBoundary), last,
				toolStep.ReportDeliveryCompleted, true,
			)
			if err != nil || !steered {
				return candidate, err
			}
			if maxSteps > 0 {
				step--
			}
			continue
		}
		if strings.TrimSpace(input.RequiredFinalToolName) != "" {
			if toolStep.RequiredFinalToolRejected || !toolStep.RequiredFinalToolAccepted {
				failure := TurnFailureError{
					Message: "foreground child required final tool was rejected",
					Code:    "subagent_required_final_tool_rejected", Severity: "error",
				}
				return terminalToolFailureResult("", last), failure
			}
			messages = messages[:messagesBeforeToolStep]
			return terminalToolResult("Foreground child result submitted.", last, false)
		}
		messages = toolStep.Messages
		if providerStep.OrdinaryEffect() && providerStep.OrdinaryWork &&
			currentOrdinaryInputClass == RuntimeCandidateInputClassOrdinaryOnly {
			ordinaryLaneMessages = appmodel.CloneProviderMessages(messages)
		}
		pendingPrivateProtocolCapsules = nextPrivateProtocolCapsules
		privateProtocolObserved = len(nextPrivateProtocolCapsules) > 0
		providerContinuationRefs = append([]domainsecurity.SettledToolReference(nil), toolStep.SettledToolReferences...)
		createPlanSatisfied = toolStep.State.CreatePlanSatisfied
		repeatSuccessCounts = toolStep.State.RepeatSuccessCounts
		failureStormSignature = toolStep.State.FailureStormSignature
		failureStormCount = toolStep.State.FailureStormCount
		if toolStep.Paused {
			if preclaimedTerminalTool {
				releaseTerminalTool()
			}
			return RuntimeAgentLoopResult{LastResult: last, Paused: true, PendingKind: toolStep.PendingKind, PendingID: toolStep.PendingID}, nil
		}
		if preclaimedTerminalTool {
			releaseTerminalTool()
		}
		if err := deps.PauseChild(ctx); err != nil {
			return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, err
		}
		if maxSteps > 0 && step+1 >= maxSteps && !stepLimitFinalizeNudged {
			if CasePublicationRequired(input.SecurityContext) {
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, StepLimitExceededError(maxSteps)
			}
			stickyRecovery = MergeRuntimeTerminalRecoveryKind(stickyRecovery, RuntimeTerminalRecoveryStepLimit)
			if err := loopEvents.StepLimitFinalAnswerRecovery(input.ThreadID, input.TurnID, maxSteps); err != nil {
				return RuntimeAgentLoopResult{AssistantText: stepText, LastResult: last}, err
			}
			messages = append(messages, domainmodel.Message{
				Role:    "user",
				Content: StepLimitFinalAnswerPrompt(maxSteps),
			})
			stepLimitFinalizeNudged = true
			continue
		}
	}
	return RuntimeAgentLoopResult{LastResult: last}, StepLimitExceededError(maxSteps)
}

func (input RuntimeRunnerInput) CreatePlanToolName() string {
	return toolcatalogapp.ToolCreatePlanName
}

func initialRuntimeProviderStep(input RuntimeRunnerInput) RuntimeProviderStep {
	if input.RestoredProviderStep != nil {
		step := *input.RestoredProviderStep
		if step.Prompt != input.Prompt || strings.TrimSpace(step.Prompt) == "" ||
			domainsecurity.ValidateLogicalEffect(step.LogicalEffect) != nil ||
			(step.CaseSourceUnavailable && (!step.OrdinaryEffect() || !step.OrdinaryWork)) {
			return RuntimeProviderStep{}
		}
		return step
	}
	step := RuntimeProviderStep{
		Prompt: input.Prompt, LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
		HostEntitySelection: input.HostEntitySelection,
	}
	if input.CaseFundPolicy.SourceUnavailable {
		step.CaseSourceUnavailable = true
	}
	if input.CaseFundPolicy.ProviderUsesCaseDataAuthority() {
		step.LogicalEffect = domainsecurity.LogicalEffectFundsData
		step.OrdinaryWork = input.CaseFundPolicy.OrdinaryWorkRequested
		return step
	}
	if input.AdditionalCaseDataEffect {
		step.LogicalEffect = domainsecurity.LogicalEffectCaseData
		step.OrdinaryWork = IndependentOrdinaryPromptV1(input.Prompt) != ""
	}
	return step
}

func recoverableProviderPreSendProtectedLaneFailureV1(err error) (string, bool) {
	var acquireErr providerAttemptAcquireError
	if errors.As(err, &acquireErr) {
		return RecoverableProtectedLaneFailureCodeV1(acquireErr.Err)
	}
	var prepareErr providerAttemptPrepareError
	if errors.As(err, &prepareErr) {
		return RecoverableProtectedLaneFailureCodeV1(prepareErr.Err)
	}
	return "", false
}

func validatePreparedProviderStep(current, prepared RuntimeProviderStep) error {
	if strings.TrimSpace(prepared.Prompt) == "" || prepared.Prompt != current.Prompt ||
		domainsecurity.ValidateLogicalEffect(prepared.LogicalEffect) != nil ||
		prepared.OrdinaryWork != current.OrdinaryWork {
		return errors.New("runtime provider step preparation is invalid")
	}
	if prepared.LogicalEffect == current.LogicalEffect {
		if prepared.CaseSourceUnavailable != current.CaseSourceUnavailable {
			return errors.New("runtime provider step preparation changed the source boundary")
		}
		if !sameHostCaseEntitySelectionV1(
			prepared.HostEntitySelection,
			current.HostEntitySelection,
		) {
			return errors.New("runtime provider step preparation changed the host entity selection")
		}
		return nil
	}
	if current.LogicalEffect == domainsecurity.LogicalEffectFundsData && current.OrdinaryWork &&
		prepared.LogicalEffect == domainsecurity.LogicalEffectOrdinary && prepared.CaseSourceUnavailable &&
		!prepared.HostEntitySelection.AvailableV1() {
		return nil
	}
	return errors.New("runtime provider step preparation changed the authorized effect")
}

func runtimeOrdinaryLaneForPromptV1(
	messages []domainmodel.Message,
	prompt string,
	steering bool,
) ([]domainmodel.Message, bool) {
	ordinaryPrompt := IndependentOrdinaryPromptV1(prompt)
	if ordinaryPrompt == "" {
		return nil, false
	}
	projected := privacyprojectionapp.OrdinaryOnlyProviderMessagesV1(messages)
	content := ordinaryPrompt
	if steering {
		content = appmodel.SteeringProviderContent(ordinaryPrompt, nil)
	}
	for index := len(projected) - 1; index >= 0; index-- {
		if projected[index].Role != "user" {
			continue
		}
		if projected[index].Content == content || projected[index].Content == ordinaryPrompt {
			return projected, true
		}
		break
	}
	projected = append(projected, domainmodel.Message{Role: "user", Content: content})
	return projected, true
}

func withRuntimeProviderSystemPrompt(messages []domainmodel.Message, systemPrompt string) []domainmodel.Message {
	projected := appmodel.CloneProviderMessages(messages)
	for index := range projected {
		if projected[index].Role == "system" {
			projected[index].Content = systemPrompt
			return projected
		}
	}
	return append([]domainmodel.Message{{Role: "system", Content: systemPrompt}}, projected...)
}

func runtimeProviderSystemPromptChanged(messages []domainmodel.Message, systemPrompt string) bool {
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		return false
	}
	for _, message := range messages {
		if message.Role == "system" {
			return message.Content != systemPrompt
		}
	}
	return true
}

func runtimeSystemOnlyMessagesV1(messages []domainmodel.Message) []domainmodel.Message {
	for _, message := range messages {
		if message.Role == "system" {
			return []domainmodel.Message{appmodel.CloneProviderMessages([]domainmodel.Message{message})[0]}
		}
	}
	return nil
}

func runtimeOrdinarySteeringPrefixV1(
	securityContext domainsecurity.TurnSecurityContext,
	inputClass string,
	prefix []domainmodel.Message,
) []domainmodel.Message {
	projected := make([]domainmodel.Message, 0, len(prefix))
	for _, message := range prefix {
		if message.Role != "assistant" || len(message.ToolCalls) != 0 || len(message.Parts) != 0 {
			continue
		}
		text := strings.TrimSpace(message.Content)
		if inputClass == RuntimeCandidateInputClassOrdinaryOnly {
			slot, _, err := appturn.CompileOrdinaryResultSlot(securityContext, text)
			if err == nil {
				message.Content = slot.Text
				projected = append(projected, message)
			}
			continue
		}
		if text == terminalToolSupersededBoundary || text == childCompletionReceiptBoundary {
			message.Content = text
			projected = append(projected, message)
		}
	}
	return appmodel.CloneProviderMessages(projected)
}

func RuntimePipelineMessageDetails(messages []domainmodel.Message) map[string]any {
	return map[string]any{
		"historyItems":            float64(RuntimeHistoryItemCount(messages)),
		"messageCount":            float64(len(messages)),
		"contextInstructionCount": float64(RuntimeContextInstructionCount(messages)),
		"systemBytes":             float64(RuntimeMessageRoleBytes(messages, "system")),
		"userBytes":               float64(RuntimeMessageRoleBytes(messages, "user")),
		"assistantBytes":          float64(RuntimeMessageRoleBytes(messages, "assistant")),
		"toolBytes":               float64(RuntimeMessageRoleBytes(messages, "tool")),
	}
}

func RuntimeHistoryItemCount(messages []domainmodel.Message) int {
	count := 0
	for index, message := range messages {
		if index == 0 && message.Role == "system" {
			continue
		}
		count++
	}
	return count
}

func RuntimeContextInstructionCount(messages []domainmodel.Message) int {
	count := 0
	for _, message := range messages {
		if message.Role == "system" && strings.TrimSpace(message.Content) != "" {
			count++
		}
	}
	return count
}

func RuntimeMessageRoleBytes(messages []domainmodel.Message, role string) int {
	total := 0
	for _, message := range messages {
		if message.Role != role {
			continue
		}
		total += len([]byte(message.Content))
		for _, part := range message.Parts {
			total += len([]byte(part.Text))
			total += len([]byte(part.ImageURL))
			total += len([]byte(part.Data))
		}
	}
	return total
}
