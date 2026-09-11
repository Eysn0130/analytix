package server

import (
	"context"
	"errors"
	"sync"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	attachmentauthorityapp "analytix.local/runtime-go/internal/app/attachmentauthority"
	attachmentuseapp "analytix.local/runtime-go/internal/app/attachmentuse"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	loopapp "analytix.local/runtime-go/internal/app/loop"
	mediaexecutionapp "analytix.local/runtime-go/internal/app/mediaexecution"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	sessionapp "analytix.local/runtime-go/internal/app/session"
	steeringauthorityapp "analytix.local/runtime-go/internal/app/steeringauthority"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	threadsummaryapp "analytix.local/runtime-go/internal/app/threadsummary"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	appusage "analytix.local/runtime-go/internal/app/usage"
	workspacemutationapp "analytix.local/runtime-go/internal/app/workspacemutation"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	jobs "analytix.local/runtime-go/internal/jobs"
	"analytix.local/runtime-go/internal/ports"
	provider "analytix.local/runtime-go/internal/provider"
	research "analytix.local/runtime-go/internal/research"
)

const DefaultRuntimeToken = httpapi.DefaultRuntimeToken

type runtimeServerHandler struct {
	runtimeToken       string
	insecure           bool
	startedAt          string
	host               string
	port               int
	dataDir            string
	infoDataDir        string
	mutationAuthority  filestore.ConditionalMutationAuthority
	store              *DurableEventSessionStore
	attachments        *filestore.PersistentAttachmentStore
	attachmentAccess   *attachmentauthorityapp.Service
	attachmentUses     *attachmentuseapp.Service
	memories           *filestore.PersistentMemoryStore
	caseFinalizer      evidenceapp.CasePublicationFinalizer
	asyncTurnObserver  func(AsyncTurnObservationV1)
	caseThreads        casethreadapp.Authority
	turnSecurity       turnsecurityapp.WorkspaceSecurityAuthority
	continuations      *continuationapp.Service
	pendingWork        *pendingworkapp.Service
	checkpoints        checkpointapp.SnapshotAuthority
	workspaceMutations *workspacemutationapp.Coordinator
	publicProjector    threadapp.PublicProjector
	control            *controlapp.Controller
	sessions           *sessionapp.Service
	threads            *threadapp.Service
	threadSummaries    *threadsummaryapp.Service
	provider           ports.ProviderClient
	providerConfig     provider.RuntimeProviderConfigSet
	providerExecution  ProviderExecutionResolver
	providerRegistry   httpapi.ProviderRegistryService
	mediaExecution     *mediaexecutionapp.Executor
	modelProxyURL      string
	approvalPolicy     string
	sandboxMode        string
	gate               *controlapp.ApprovalUserInputManager
	gates              *controlapp.GateRegistry[runtimePendingToolCall]
	mcp                MCPManager
	mcpSearch          runtimeMCPSearchSettings
	commandProbe       ports.CommandProbe
	commandHomeDir     string
	workspaceProbe     ports.WorkspaceStatusProbe
	shellRunner        ports.ShellRunner
	gitStatusProbe     ports.GitStatusProbe
	worktreeManager    ports.WorktreeManager
	allowWriteRoots    []string
	protectedReadDirs  []string
	jobs               *jobs.Manager
	childCompletions   subagentapp.ChildCompletionIssuer
	foregroundHandoffs *subagentapp.ForegroundHandoffAuthority
	g6Readiness        runtimeReadinessStatus
	autoResearch       research.AutoResearchProjectStore
	skills             runtimeSkillCatalog
	subagents          runtimeSubagentConfig
	subagentState      *subagentapp.RuntimeState
	nativeAuthority    *nativecomponentapp.RuntimeAuthority
	caseEntities       *caseentityapp.Service
	resolveCaseIngress caseentityapp.ResolveAccountIngressCandidatesV1
	caseAnswerSlots    func(
		context.Context,
		domainsecurity.TurnSecurityContext,
		[]domainnative.AccountFlowProviderModelOutputV1,
	) ([]domainjob.CaseDelegatedAnswerSlotBindingV1, error)
	steeringAuthority *steeringauthorityapp.Service
	web               runtimeinfoapp.WebConfig
	visionBridge      runtimeVisionBridgeConfig
	stepLimits        loopapp.StepLimitConfig
	heartbeatInterval time.Duration

	asyncTurnPhaseObserver func(string)

	mu                 sync.Mutex
	gateCancellationMu sync.Mutex
	turnSeq            int
	readRegistry       *filetoolsapp.ReadRegistry
	cachePrefixShapes  map[string]appusage.PrefixBaseline
}

func (h *runtimeServerHandler) resolveRuntimeTurnExecution(
	ctx context.Context,
	input provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	if h == nil || ctx == nil || ctx.Err() != nil {
		return provider.TurnExecutionResult{}, errors.New("provider execution authority is unavailable")
	}
	if h.providerExecution == nil {
		return provider.TurnExecutionResult{}, errors.New("provider execution authority is unavailable")
	}
	return h.providerExecution.ResolveTurnExecution(ctx, input)
}

func (h *runtimeServerHandler) resolveRuntimeTurnIntent(
	ctx context.Context,
	input provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	if h == nil || ctx == nil || ctx.Err() != nil {
		return provider.TurnExecutionResult{}, errors.New("provider execution authority is unavailable")
	}
	resolver, ok := h.providerExecution.(ProviderExecutionIntentResolver)
	if !ok || resolver == nil {
		return provider.TurnExecutionResult{}, errors.New("provider execution intent authority is unavailable")
	}
	result, err := resolver.ResolveTurnIntent(ctx, input)
	if err != nil {
		return provider.TurnExecutionResult{}, err
	}
	if result.Config.APIKey != "" {
		return provider.TurnExecutionResult{}, errors.New("provider execution intent exposed a credential")
	}
	return result, nil
}

func (h *runtimeServerHandler) validateRuntimeTurnExecutionCurrent(
	ctx context.Context,
	authority provider.TurnExecutionAuthority,
) error {
	if h == nil || ctx == nil || ctx.Err() != nil {
		return errors.New("provider execution authority is unavailable")
	}
	if err := h.requireRuntimeTurnExecutionCurrentness(); err != nil {
		return err
	}
	validator, ok := h.providerExecution.(ProviderExecutionCurrentnessValidator)
	if !ok || validator == nil {
		return nil
	}
	return validator.ValidateTurnExecutionCurrent(ctx, authority)
}

func (h *runtimeServerHandler) requireRuntimeTurnExecutionCurrentness() error {
	if h == nil {
		return errors.New("provider execution authority is unavailable")
	}
	if _, lateBound := h.providerExecution.(ProviderExecutionIntentResolver); !lateBound {
		return nil
	}
	if validator, ok := h.providerExecution.(ProviderExecutionCurrentnessValidator); !ok || validator == nil {
		return errors.New("provider execution currentness authority is unavailable")
	}
	return nil
}

type runtimeSkillCatalog = toolcatalogapp.SkillCatalog

type runtimeSubagentConfig = subagentapp.ProfileSettings

type runtimeMCPSearchSettings = runtimeinfoapp.MCPSearchConfig

type runtimeVisionBridgeConfig = runtimeinfoapp.VisionBridgeConfig

type MCPManager = ports.RuntimeMCPManager
