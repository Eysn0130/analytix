package server

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	attachmentauthority "analytix.local/runtime-go/internal/adapters/outbound/attachmentauthority"
	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	continuationstore "analytix.local/runtime-go/internal/adapters/outbound/continuationstore"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingworkstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	processadapter "analytix.local/runtime-go/internal/adapters/outbound/process"
	attachmentauthorityapp "analytix.local/runtime-go/internal/app/attachmentauthority"
	attachmentuseapp "analytix.local/runtime-go/internal/app/attachmentuse"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	executionpolicy "analytix.local/runtime-go/internal/app/executionpolicy"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	steeringauthorityapp "analytix.local/runtime-go/internal/app/steeringauthority"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	jobs "analytix.local/runtime-go/internal/jobs"
	mcp "analytix.local/runtime-go/internal/mcp"
	provider "analytix.local/runtime-go/internal/provider"
	research "analytix.local/runtime-go/internal/research"
	casepublicationtest "analytix.local/runtime-go/internal/testsupport/casepublication"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func newCompatibilityRuntimeServerHandler(config RuntimeServerConfig) http.Handler {
	if strings.TrimSpace(config.RuntimeToken) == "" && !config.Insecure {
		config.RuntimeToken = DefaultRuntimeToken
	}
	if strings.TrimSpace(config.StartedAt) == "" {
		config.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(config.Host) == "" {
		config.Host = "127.0.0.1"
	}
	if strings.TrimSpace(config.DataDir) == "" {
		config.DataDir = "/tmp/analytix"
	}
	store, err := newRuntimeEventSessionStore(config)
	if err != nil {
		panic(err)
	}
	if err := store.SeedFromG2Routes(config.Routes); err != nil {
		panic(err)
	}
	attachmentStore, err := filestore.NewPersistentAttachmentStore(config.DataDir)
	if err != nil {
		panic(err)
	}
	attachmentAuthorityRoot := filepath.Join(config.DataDir, "private", "attachment-authority")
	attachmentPrivateAccess, err := privatecastest.NewAccessAuthority(attachmentAuthorityRoot)
	if err != nil {
		panic(err)
	}
	attachmentAuthorityStore, err := attachmentauthority.NewStore(attachmentAuthorityRoot, attachmentPrivateAccess)
	if err != nil {
		panic(err)
	}
	attachmentAccess := &attachmentauthorityapp.Service{
		Threads: store, Bindings: filestore.CaseBindingReader{}, Owners: attachmentAuthorityStore, Uploads: attachmentAuthorityStore,
	}
	memoryStore, err := filestore.NewPersistentMemoryStore(config.DataDir)
	if err != nil {
		panic(err)
	}
	continuationRoot := filepath.Join(config.DataDir, "private", "gate-continuations")
	continuationPrivateAccess, err := privatecastest.NewAccessAuthority(continuationRoot)
	if err != nil {
		panic(err)
	}
	continuationStore, err := continuationstore.NewStore(continuationRoot, continuationPrivateAccess)
	if err != nil {
		panic(err)
	}
	pendingWorkRoot := filepath.Join(config.DataDir, "private", "pending-work")
	pendingWorkAuthority, err := privatecastest.NewAccessAuthority(pendingWorkRoot)
	if err != nil {
		panic(err)
	}
	pendingWorkStore, err := pendingworkstore.NewStore(pendingWorkRoot, pendingWorkAuthority)
	if err != nil {
		panic(err)
	}
	checkpointAuthorityRoot := filepath.Join(config.DataDir, "private", "checkpoint-authority")
	checkpointPrivateAccess, err := privatecastest.NewAccessAuthority(checkpointAuthorityRoot)
	if err != nil {
		panic(err)
	}
	checkpointAuthorityStore, err := checkpointauthority.NewStoreContext(context.Background(), checkpointAuthorityRoot, checkpointPrivateAccess)
	if err != nil {
		panic(err)
	}
	continuationStateExists, err := continuationStore.HasRecords(nil)
	if err != nil {
		panic(err)
	}
	pendingWorkStateExists, err := pendingWorkStore.HasRecords(nil)
	if err != nil {
		panic(err)
	}
	continuationAuthority, err := finalauthority.OpenOrCreateFileAuthority(
		filepath.Join(config.DataDir, "private", "authority", "continuation-ed25519-v1.json"), continuationStateExists || pendingWorkStateExists,
	)
	if err != nil {
		panic(err)
	}
	jobManager, err := jobs.NewManager(filepath.Join(config.DataDir, "child-runs"))
	if err != nil {
		panic(err)
	}
	caseFinalizer, err := casepublicationtest.New(filepath.Join(config.DataDir, "private", "test-case-publication"))
	if err != nil {
		panic(err)
	}
	if config.G6Readiness.SchemaVersion == 0 {
		config.G6Readiness = defaultRuntimeReadinessStatus()
	}
	mcpSpecs, err := loadMCPServerSpecs(config)
	if err != nil {
		panic(err)
	}
	mcpSearch, err := loadRuntimeMCPSearchSettings(config)
	if err != nil {
		panic(err)
	}
	skillCatalog, err := loadRuntimeSkillCatalog(config)
	if err != nil {
		panic(err)
	}
	subagentConfig, err := loadRuntimeSubagentConfig(config)
	if err != nil {
		panic(err)
	}
	sandboxSettings, err := loadRuntimeSandboxSettings(config)
	if err != nil {
		panic(err)
	}
	webConfig, err := loadRuntimeWebConfig(config)
	if err != nil {
		panic(err)
	}
	visionBridgeConfig, err := loadRuntimeVisionBridgeConfig(config)
	if err != nil {
		panic(err)
	}
	stepLimits, err := loadRuntimeStepLimitConfig(config)
	if err != nil {
		panic(err)
	}
	approvalPolicy, sandboxMode := executionpolicy.Normalize(config.ApprovalPolicy, config.SandboxMode)
	mcpProxyURL := strings.TrimSpace(config.MCPProxyURL)
	if mcpProxyURL == "" {
		mcpProxyURL = strings.TrimSpace(config.ModelProxyURL)
	}
	subagentState := subagentapp.NewRuntimeState()
	nativeAuthority, err := nativecomponentapp.NewUnavailableRuntimeAuthority(nativecomponentapp.HealthDependencies{
		Store: store, DurableAuthority: compatibilityNativeDurableAuthority{},
		AcquireEffect: subagentState.AcquireContextEffect,
		Now:           time.Now,
	})
	if err != nil {
		panic(err)
	}
	handler, err := NewRuntimeServerHandlerFromComponents(config, RuntimeServerComponents{
		Store:            store,
		Attachments:      attachmentStore,
		AttachmentAccess: attachmentAccess,
		AttachmentUses: attachmentuseapp.NewService(
			continuationAuthority, attachmentAuthorityStore,
			func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
		),
		Memories:      memoryStore,
		CaseFinalizer: caseFinalizer,
		TurnSecurity: turnsecurityapp.WorkspaceSecurityAuthority{
			Identity: testIdentityAuthority(),
		},
		Continuations: continuationapp.NewService(continuationAuthority, continuationStore),
		PendingWork:   pendingworkapp.NewService(continuationAuthority, pendingWorkStore, store),
		Checkpoints:   checkpointapp.SnapshotAuthority{Store: checkpointAuthorityStore},
		Provider:      provider.NewHTTPProviderClient(provider.NewDefaultHTTPClientWithProxy(config.ModelProxyURL)),
		ProviderConfig: provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
			DefaultProviderID:     config.ProviderID,
			DefaultBaseURL:        config.BaseURL,
			DefaultAPIKey:         config.APIKey,
			DefaultEndpointFormat: config.EndpointFormat,
			DefaultModel:          config.Model,
			ModelProvidersJSON:    config.ModelProvidersJSON,
		}),
		ModelProxyURL:  strings.TrimSpace(config.ModelProxyURL),
		ApprovalPolicy: approvalPolicy,
		SandboxMode:    sandboxMode,
		Gate:           controlapp.NewApprovalUserInputManager(),
		MCP: mcp.NewProductionManagerWithOptions(mcpSpecs, mcp.ProductionManagerOptions{
			CacheDir: filepath.Join(config.DataDir, "mcp-schema-cache"),
			ProxyURL: mcpProxyURL,
		}),
		CommandProbe:      processadapter.NewCommandProbe(),
		CommandHomeDir:    processadapter.HomeDir(),
		WorkspaceProbe:    processadapter.NewWorkspaceStatusProbe(),
		ShellRunner:       processadapter.NewShellRunner(),
		GitStatusProbe:    processadapter.NewGitStatusProbe(),
		WorktreeManager:   processadapter.NewWorktreeManager(filepath.Join(config.DataDir, "subagent-worktrees")),
		MCPSearch:         mcpSearch,
		AllowWriteRoots:   sandboxSettings.AllowWriteRoots,
		ProtectedReadDirs: sandboxSettings.ProtectedReadDirs,
		Jobs:              jobManager,
		G6Readiness:       config.G6Readiness,
		AutoResearch:      research.NewAutoResearchProjectStore(nil),
		Skills:            skillCatalog,
		Subagents:         subagentConfig,
		SubagentState:     subagentState,
		NativeAuthority:   nativeAuthority,
		SteeringAuthority: steeringauthorityapp.NewService(continuationAuthority),
		Web:               webConfig,
		VisionBridge:      visionBridgeConfig,
		StepLimits:        stepLimits,
		HeartbeatInterval: 15 * time.Second,
	})
	if err != nil {
		panic(err)
	}
	runtimeHandler, ok := handler.(*runtimeServerHandler)
	if !ok {
		panic("test runtime handler has an unexpected type")
	}
	primaryReader, err := finalauthority.NewAcceptedFinalCASReader(runtimeHandler.store.root)
	if err != nil {
		panic(err)
	}
	if err := runtimeHandler.store.BindPrimaryThreadReaderV1(primaryReader); err != nil {
		panic(err)
	}
	installHandlerProviderExecutionResolverForTest(runtimeHandler)
	return runtimeHandler
}

func loadMCPServerSpecs(config RuntimeServerConfig) ([]mcp.ServerSpec, error) {
	if strings.TrimSpace(config.MCPConfigJSON) != "" {
		return mcp.LoadMCPJSONDocument([]byte(config.MCPConfigJSON), config.DataDir)
	}
	if strings.TrimSpace(config.MCPConfigPath) != "" {
		return mcp.LoadMCPJSON(config.MCPConfigPath, config.DataDir)
	}
	return nil, nil
}

func loadRuntimeMCPSearchSettings(config RuntimeServerConfig) (runtimeMCPSearchSettings, error) {
	document, ok, err := runtimeConfigDocument(config)
	if err != nil {
		return runtimeinfoapp.LoadMCPSearchConfigFromDocument(nil, false), err
	}
	return runtimeinfoapp.LoadMCPSearchConfigFromDocument(document, ok), nil
}

type compatibilityNativeDurableAuthority struct{}

func (compatibilityNativeDurableAuthority) ValidateCurrent(
	threadID string,
	thread map[string]any,
) (domainsecurity.TurnSecurityContext, error) {
	securityContext, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || securityContext.ThreadID != threadID {
		return domainsecurity.TurnSecurityContext{}, errors.New("test native durable authority is invalid")
	}
	return securityContext, nil
}
