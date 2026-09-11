package server

import (
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	attachmentauthorityapp "analytix.local/runtime-go/internal/app/attachmentauthority"
	attachmentpublicationapp "analytix.local/runtime-go/internal/app/attachmentpublication"
	attachmentuseapp "analytix.local/runtime-go/internal/app/attachmentuse"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	executionpolicy "analytix.local/runtime-go/internal/app/executionpolicy"
	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	loopapp "analytix.local/runtime-go/internal/app/loop"
	mediaexecutionapp "analytix.local/runtime-go/internal/app/mediaexecution"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	publicationauthorityapp "analytix.local/runtime-go/internal/app/publicationauthority"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	sessionapp "analytix.local/runtime-go/internal/app/session"
	steeringauthorityapp "analytix.local/runtime-go/internal/app/steeringauthority"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
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

type RuntimeReadinessStatus = runtimeReadinessStatus

type ProviderExecutionResolver interface {
	ResolveTurnExecution(context.Context, provider.TurnExecutionInput) (provider.TurnExecutionResult, error)
}

type ProviderExecutionIntentResolver interface {
	ResolveTurnIntent(context.Context, provider.TurnExecutionInput) (provider.TurnExecutionResult, error)
}

type ProviderExecutionCurrentnessValidator interface {
	ValidateTurnExecutionCurrent(context.Context, provider.TurnExecutionAuthority) error
}

type RuntimeServerComponents struct {
	AsyncTurnObserverV1 func(AsyncTurnObservationV1)
	// AsyncTurnPhaseObserverV1 is an optional in-process regression barrier.
	// It receives fixed phase labels only, never public or persisted data.
	AsyncTurnPhaseObserverV1 func(string)

	ChildIdentityFloors domainpendingwork.ChildIdentityFloorsV1
	Store               *DurableEventSessionStore
	Attachments         *filestore.PersistentAttachmentStore
	AttachmentAccess    *attachmentauthorityapp.Service
	AttachmentUses      *attachmentuseapp.Service
	Memories            *filestore.PersistentMemoryStore
	CaseFinalizer       evidenceapp.CasePublicationFinalizer
	CaseThreads         casethreadapp.Authority
	TurnSecurity        turnsecurityapp.WorkspaceSecurityAuthority
	Continuations       *continuationapp.Service
	PendingWork         *pendingworkapp.Service
	Checkpoints         checkpointapp.SnapshotAuthority
	PublicProjector     threadapp.PublicProjector
	Provider            ports.ProviderClient
	ProviderConfig      provider.RuntimeProviderConfigSet
	ProviderExecution   ProviderExecutionResolver
	ProviderRegistry    httpapi.ProviderRegistryService
	MediaExecution      *mediaexecutionapp.Executor
	InfoDataDir         string
	ModelProxyURL       string
	ApprovalPolicy      string
	SandboxMode         string
	Gate                *controlapp.ApprovalUserInputManager
	MCP                 MCPManager
	MCPSearch           runtimeinfoapp.MCPSearchConfig
	CommandProbe        ports.CommandProbe
	CommandHomeDir      string
	WorkspaceProbe      ports.WorkspaceStatusProbe
	ShellRunner         ports.ShellRunner
	GitStatusProbe      ports.GitStatusProbe
	WorktreeManager     ports.WorktreeManager
	AllowWriteRoots     []string
	ProtectedReadDirs   []string
	Jobs                *jobs.Manager
	ChildCompletions    subagentapp.ChildCompletionIssuer
	ForegroundHandoffs  *subagentapp.ForegroundHandoffAuthority
	G6Readiness         RuntimeReadinessStatus
	AutoResearch        research.AutoResearchProjectStore
	Skills              toolcatalogapp.SkillCatalog
	Subagents           subagentapp.ProfileSettings
	SubagentState       *subagentapp.RuntimeState
	NativeAuthority     *nativecomponentapp.RuntimeAuthority
	CaseEntities        *caseentityapp.Service
	ResolveCaseIngress  caseentityapp.ResolveAccountIngressCandidatesV1
	CaseAnswerSlots     func(
		context.Context,
		domainsecurity.TurnSecurityContext,
		[]domainnative.AccountFlowProviderModelOutputV1,
	) ([]domainjob.CaseDelegatedAnswerSlotBindingV1, error)
	SteeringAuthority *steeringauthorityapp.Service
	Web               runtimeinfoapp.WebConfig
	VisionBridge      runtimeinfoapp.VisionBridgeConfig
	StepLimits        loopapp.StepLimitConfig
	HeartbeatInterval time.Duration
	SkipMCPConnect    bool
}

func NewRuntimeServerHandlerFromComponents(config RuntimeServerConfig, components RuntimeServerComponents) (http.Handler, error) {
	if err := components.ChildIdentityFloors.Validate(); err != nil {
		return nil, err
	}
	if _, err := turnsecurityapp.ResolveCurrentPrincipal(context.Background(), components.TurnSecurity.Identity); err != nil {
		return nil, errors.Join(errors.New("host identity authority is required"), err)
	}
	if components.PendingWork == nil || !components.PendingWork.Available() {
		return nil, errors.New("pending work authority is required")
	}
	if components.NativeAuthority == nil || !components.NativeAuthority.Available() {
		return nil, errors.New("native runtime authority is required")
	}
	if components.SteeringAuthority == nil {
		return nil, errors.New("steering admission authority is required")
	}
	if !components.Checkpoints.Available() {
		return nil, errors.New("checkpoint snapshot authority is required")
	}
	if components.Attachments != nil && (components.AttachmentAccess == nil || !components.AttachmentAccess.UploadAvailable() ||
		components.AttachmentUses == nil || !components.AttachmentUses.Available()) {
		return nil, errors.New("attachment owner and use authority are required")
	}
	if components.Gate == nil {
		components.Gate = controlapp.NewApprovalUserInputManager()
	}
	if components.CaseFinalizer == nil {
		return nil, errors.New("case publication finalizer is required")
	}
	if components.TurnSecurity.Observer == nil {
		components.TurnSecurity.Observer = filestore.CaseBindingReader{}
	}
	components.CaseFinalizer = evidenceapp.WithCurrentPublicationAuthority(
		components.CaseFinalizer,
		evidenceapp.CurrentPublicationAuthorityFunc(func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
			return turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
				OperationContext: ctx, Identity: components.TurnSecurity.Identity,
				Observer: components.TurnSecurity.Observer, RiskAuthority: components.TurnSecurity.RiskAuthority,
				SnapshotAuthority: components.TurnSecurity.SnapshotAuthority, SnapshotAuthorityV2: components.TurnSecurity.SnapshotAuthorityV2, Context: securityContext, Workspace: securityContext.WorkspaceRealPath,
			})
		}),
	)
	if components.Attachments != nil {
		components.CaseFinalizer = attachmentpublicationapp.WithUseGuard(components.CaseFinalizer, components.AttachmentUses)
	}
	if components.SubagentState == nil {
		components.SubagentState = subagentapp.NewRuntimeState()
	}
	if components.G6Readiness.SchemaVersion == 0 {
		components.G6Readiness = defaultRuntimeReadinessStatus()
	}
	if components.HeartbeatInterval <= 0 {
		components.HeartbeatInterval = 15 * time.Second
	}
	components.ApprovalPolicy, components.SandboxMode = executionpolicy.Normalize(components.ApprovalPolicy, components.SandboxMode)
	mutationAuthority, err := filestore.OpenConditionalMutationAuthority(filepath.Join(config.DataDir, "file-mutation-quarantine-v1"))
	if err != nil {
		return nil, errors.Join(errors.New("conditional file mutation authority is unavailable"), err)
	}
	handler := &runtimeServerHandler{
		asyncTurnPhaseObserver: components.AsyncTurnPhaseObserverV1,

		turnSeq:            components.ChildIdentityFloors.TurnSequence,
		runtimeToken:       config.RuntimeToken,
		insecure:           config.Insecure,
		startedAt:          config.StartedAt,
		host:               config.Host,
		port:               config.Port,
		dataDir:            config.DataDir,
		infoDataDir:        firstNonEmptyString(components.InfoDataDir, config.DataDir),
		mutationAuthority:  mutationAuthority,
		store:              components.Store,
		attachments:        components.Attachments,
		attachmentAccess:   components.AttachmentAccess,
		attachmentUses:     components.AttachmentUses,
		memories:           components.Memories,
		caseFinalizer:      components.CaseFinalizer,
		asyncTurnObserver:  components.AsyncTurnObserverV1,
		caseThreads:        components.CaseThreads,
		turnSecurity:       components.TurnSecurity,
		continuations:      components.Continuations,
		pendingWork:        components.PendingWork,
		checkpoints:        components.Checkpoints,
		workspaceMutations: workspacemutationapp.NewCoordinator(),
		publicProjector:    threadapp.EnsurePublicProjector(components.PublicProjector),
		provider:           components.Provider,
		providerConfig:     components.ProviderConfig,
		providerExecution:  components.ProviderExecution,
		providerRegistry:   components.ProviderRegistry,
		mediaExecution:     components.MediaExecution,
		modelProxyURL:      components.ModelProxyURL,
		approvalPolicy:     components.ApprovalPolicy,
		sandboxMode:        components.SandboxMode,
		gate:               components.Gate,
		gates:              controlapp.NewGateRegistry[runtimePendingToolCall](),
		mcp:                components.MCP,
		mcpSearch:          components.MCPSearch,
		commandProbe:       components.CommandProbe,
		commandHomeDir:     components.CommandHomeDir,
		workspaceProbe:     components.WorkspaceProbe,
		shellRunner:        components.ShellRunner,
		gitStatusProbe:     components.GitStatusProbe,
		worktreeManager:    components.WorktreeManager,
		allowWriteRoots:    components.AllowWriteRoots,
		protectedReadDirs:  components.ProtectedReadDirs,
		jobs:               components.Jobs,
		childCompletions:   components.ChildCompletions,
		foregroundHandoffs: components.ForegroundHandoffs,
		g6Readiness:        components.G6Readiness,
		autoResearch:       components.AutoResearch,
		skills:             components.Skills,
		subagents:          components.Subagents,
		subagentState:      components.SubagentState,
		nativeAuthority:    components.NativeAuthority,
		caseEntities:       components.CaseEntities,
		resolveCaseIngress: components.ResolveCaseIngress,
		caseAnswerSlots:    components.CaseAnswerSlots,
		steeringAuthority:  components.SteeringAuthority,
		web:                components.Web,
		visionBridge:       components.VisionBridge,
		stepLimits:         components.StepLimits,
		heartbeatInterval:  components.HeartbeatInterval,
		readRegistry:       filetoolsapp.NewReadRegistry(),
		cachePrefixShapes:  map[string]appusage.PrefixBaseline{},
	}
	if handler.store != nil {
		handler.store.SetCaseThreadAuthority(handler.caseThreads)
		handler.store.SetSteeringAuthority(handler.steeringAuthority)
	}
	handler.control = controlapp.NewController(runtimeControlDriver{handler: handler})
	handler.sessions = sessionapp.NewService(sessionapp.Dependencies{Repository: handler.store})
	handler.threads = threadapp.NewService(threadapp.Dependencies{
		Repository:               handler.store,
		DataDir:                  handler.dataDir,
		DefaultApprovalPolicy:    handler.approvalPolicy,
		DefaultSandboxMode:       handler.sandboxMode,
		UsageSnapshot:            handler.threadUsageSnapshot,
		PublicProjector:          handler.publicProjector,
		CaseThreads:              handler.caseThreads,
		WorkspaceReader:          filestore.CaseBindingReader{},
		WorkspaceSecurity:        handler.turnSecurity,
		BeginTransition:          handler.beginRuntimeThreadContextTransition,
		BeginScopeTransition:     handler.beginRuntimeThreadScopeTransition,
		BeginWorkspaceTransition: handler.beginRuntimeWorkspaceScopeTransition,
		BeginMetadataRead:        handler.subagentState.AcquireSecurityScopeRead,
		QuiesceThreadTurns:       handler.quiesceRuntimeThreadForMutation,
	})
	if err := handler.restoreRuntimeState(); err != nil {
		return nil, err
	}
	if handler.jobs != nil {
		cleaned, err := handler.jobs.CleanupStaleRunningRecords()
		if err != nil {
			return nil, err
		}
		securityAuthority := handler.runtimeJobSecurityAuthorizer()
		startupRecovery := subagentapp.NewStartupRecoveryServiceV1(
			handler.store, handler.jobs, securityAuthority,
			handler.runtimeBackgroundDeliveryService(), handler.recordRuntimeBestEffortEvent,
		)
		if err := startupRecovery.RecoverInterrupted(cleaned); err != nil {
			return nil, err
		}
		if err := startupRecovery.RecoverPendingDeliveries(); err != nil {
			return nil, err
		}
	}
	if handler.mcp != nil && !components.SkipMCPConnect {
		handler.mcp.Connect()
	}
	return handler, nil
}

func DefaultRuntimeReadinessStatus() RuntimeReadinessStatus { return defaultRuntimeReadinessStatus() }

func (h *runtimeServerHandler) runtimePublicationAuthority() *publicationauthorityapp.Service {
	return publicationauthorityapp.NewService(publicationauthorityapp.Dependencies{
		AcquireContextEffect:             h.runtimeSubagentState().AcquireContextEffect,
		AcquireContextEffectForAuthority: h.runtimeSubagentState().AcquireContextEffectForAuthority,
		ValidateCurrent: func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
				OperationContext: ctx, Identity: h.turnSecurity.Identity,
				Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority,
				SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: securityContext,
				Workspace: securityContext.WorkspaceRealPath,
			})
		},
		ValidateCurrentForAuthority: func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, caseDataEffect bool) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return turnsecurityapp.ValidateCurrentForEffect(turnsecurityapp.CurrentValidationInput{
				OperationContext: ctx, Identity: h.turnSecurity.Identity,
				Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority,
				SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: securityContext,
				Workspace: securityContext.WorkspaceRealPath,
			}, caseDataEffect)
		},
		CaseLineage: h.caseThreads, TerminalArbitrator: h.runtimeControl(), CaseFinalizer: h.caseFinalizer,
		HostContext: newHostAuthorityContextFrom,
	})
}
