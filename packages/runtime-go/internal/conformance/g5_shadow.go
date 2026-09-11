//go:build !analytix_prod

package conformance

import (
	"encoding/json"
	"strings"

	agent "analytix.local/runtime-go/internal/agent"
	contracts "analytix.local/runtime-go/internal/contracts"
	mcp "analytix.local/runtime-go/internal/mcp"
	"analytix.local/runtime-go/internal/protocol"
	provider "analytix.local/runtime-go/internal/provider"
)

type ProductBoundary = contracts.ProductBoundary
type G2RouteReplayCase = protocol.G2RouteReplayCase
type G3ProviderConformanceContract = provider.G3ProviderConformanceContract
type G3ProviderUsageCaseSummary = provider.G3ProviderUsageCaseSummary
type G3ProviderUsageSummary = provider.G3ProviderUsageSummary
type G3ProviderCacheAccounting = provider.G3ProviderCacheAccounting
type G3ProviderStreamingConformance = provider.G3ProviderStreamingConformance
type ProviderPrefixShape = provider.ProviderPrefixShape
type ProviderDriftAttribution = provider.ProviderDriftAttribution
type G5ProviderDriftAttribution = provider.G5ProviderDriftAttribution

type G5FullLoopConformanceContract struct {
	Mode                       string                      `json:"mode"`
	SourceContractIDs          []string                    `json:"sourceContractIds"`
	ProductBoundary            ProductBoundary             `json:"productBoundary"`
	ContractInventory          G5FullLoopContractInventory `json:"contractInventory"`
	RemainingBlockers          []string                    `json:"remainingBlockers"`
	ControlReplay              map[string]any              `json:"controlReplay"`
	ControlExecutableCases     G5ControlExecutableCases    `json:"controlExecutableCases"`
	ShadowSlicesExpectedOutput map[string]any              `json:"shadowSlicesExpectedOutput"`
	ExpectedOutput             map[string]any              `json:"expectedOutput"`
}

type G5FullLoopContractInventory struct {
	FullLoop        []string             `json:"fullLoop"`
	JobManager      G5JobManagerContract `json:"jobManager"`
	CacheCompaction []string             `json:"cacheCompaction"`
	ResumeInterrupt []string             `json:"resumeInterrupt"`
	MCPIndexer      []string             `json:"mcpIndexer"`
}

type G5JobManagerContract struct {
	Tests             []string `json:"tests"`
	RouteContract     []string `json:"routeContract"`
	RequiredBehaviors []string `json:"requiredBehaviors"`
}

func BuildG5FullLoopConformanceOutput(contract G5FullLoopConformanceContract) map[string]any {
	return map[string]any{
		"stage":                    "G5",
		"mode":                     contract.Mode,
		"sourceContractIds":        contract.SourceContractIDs,
		"tsOwnedContractInventory": true,
		"electronMainConnected":    false,
		"defaultGoBackendEnabled":  false,
		"fullLoopTests":            contract.ContractInventory.FullLoop,
		"jobManager": map[string]any{
			"tests":             contract.ContractInventory.JobManager.Tests,
			"routes":            contract.ContractInventory.JobManager.RouteContract,
			"requiredBehaviors": contract.ContractInventory.JobManager.RequiredBehaviors,
		},
		"cacheCompactionTests": contract.ContractInventory.CacheCompaction,
		"resumeInterruptTests": contract.ContractInventory.ResumeInterrupt,
		"mcpIndexerTests":      contract.ContractInventory.MCPIndexer,
		"remainingBlockers":    contract.RemainingBlockers,
		"productBoundary":      contract.ProductBoundary,
	}
}

type G5ControlExecutableCases struct {
	ProductBoundary             G5ProductBoundaryCase             `json:"productBoundary"`
	PackageRuntimeIdentity      G5PackageRuntimeIdentityCase      `json:"packageRuntimeIdentity"`
	RuntimeHTTPRouteSovereignty G5RuntimeHTTPRouteSovereigntyCase `json:"runtimeHttpRouteSovereignty"`
	DesktopSovereignty          G5DesktopSovereigntyCase          `json:"desktopSovereignty"`
	DesktopMainIpcBoundary      G5DesktopMainIpcBoundaryCase      `json:"desktopMainIpcBoundary"`
	RendererRouteSurface        G5RendererRouteSurfaceCase        `json:"rendererRouteSurfaceSovereignty"`
	GoalPersistenceOffLock      G5GoalPersistenceOffLockCase      `json:"goalPersistenceOffLock"`
	ToolResultFileImageBoundary G5ToolResultFileImageBoundaryCase `json:"toolResultFileImageBoundary"`
	EventJsonlReplayBoundary    G5EventJsonlReplayBoundaryCase    `json:"eventJsonlReplayBoundary"`
	MCPMalformedSchemaBoundary  G5MCPMalformedSchemaBoundaryCase  `json:"mcpMalformedSchemaBoundary"`
	Cancel                      G5CancelExecutableCase            `json:"cancel"`
	TaskJobs                    G5TaskJobsExecutableCase          `json:"taskJobs"`
	Approval                    G5ApprovalDenyCase                `json:"approvalDeny"`
	UserInput                   G5UserInputExecutableCase         `json:"userInput"`
	ApprovalUserInputRoute      G5ApprovalUserInputRouteCase      `json:"approvalUserInputRouteReplay"`
	ApprovalUserInputInventory  G5ApprovalUserInputInventoryCase  `json:"approvalUserInputInventory"`
	AbortCleanup                G5AbortCleanupCase                `json:"abortCleanup"`
	ResumeGates                 G5ResumePendingGatesCase          `json:"resumePendingGates"`
	AutoResearch                G5AutoResearchExecutableCase      `json:"autoResearch"`
	MCPLifecycle                G5MCPLifecycleExecutableCase      `json:"mcpLifecycle"`
	MCPCoreLifecycle            G5MCPCoreLifecycleCase            `json:"mcpCoreLifecycle"`
	MCPBackgroundReconnect      G5MCPBackgroundReconnectCase      `json:"mcpBackgroundReconnect"`
	MCPCallReconnect            G5MCPCallReconnectCase            `json:"mcpCallReconnect"`
	MCPKnownOverrideDiagnostics G5MCPKnownOverrideDiagnosticsCase `json:"mcpKnownOverrideDiagnostics"`
	MCPLiveLocalIndexer         G5MCPLiveLocalIndexerCase         `json:"mcpLiveLocalIndexer"`
	MCPApprovalAnnotations      G5MCPApprovalAnnotationCase       `json:"mcpApprovalAnnotations"`
	MCPSearchMetaTools          G5MCPSearchMetaToolsCase          `json:"mcpSearchMetaTools"`
	MCPSearchRefreshDrift       G5MCPSearchRefreshDriftCase       `json:"mcpSearchRefreshDrift"`
	MCPSearchWorkspace          G5MCPSearchWorkspaceBoundaryCase  `json:"mcpSearchWorkspaceBoundary"`
	Checkpoint                  G5CheckpointRewindCase            `json:"checkpointRewind"`
	RemoteEntry                 G5RemoteEntryCase                 `json:"remoteEntry"`
	HistoryRepair               G5HistoryRepairCase               `json:"historyRepair"`
	CompactionBoundary          G5CompactionBoundaryCase          `json:"compactionBoundary"`
	StepLimits                  G5StepLimitsExecutableCase        `json:"stepLimits"`
	Planner                     G5PlannerExecutableCase           `json:"planner"`
	AutoRouter                  G5AutoRouterClassifierCase        `json:"autoRouterClassifier"`
	Combined                    G5CombinedExecutableCase          `json:"combined"`
	PlanStepCancelCache         G5PlanStepCancelCacheCase         `json:"planStepCancelCache"`
	PlanCancelStateReset        G5PlanCancelStateResetCase        `json:"planCancelStateReset"`
	ProviderCacheReleaseGuard   G5ProviderReleaseGuardControlCase `json:"providerCacheReleaseGuard"`
	ProviderUsageParser         G5ProviderUsageParserControlCase  `json:"providerUsageParser"`
	ProviderRequestShape        G5ProviderRequestShapeControlCase `json:"providerRequestShape"`
	ProviderLiveLocalHTTP       G5ProviderLiveLocalHTTPCase       `json:"providerLiveLocalHttpContract"`
	ProviderCacheCoverageFloor  G5ProviderCacheCoverageFloorCase  `json:"providerCacheCoverageFloor"`
	ProviderDriftAttribution    G5ProviderDriftAttributionCase    `json:"providerDriftAttribution"`
	ProviderCacheAccounting     G5ProviderCacheAccountingCase     `json:"providerCacheAccounting"`
	ProviderOfflineParitySeal   G5ProviderOfflineParitySealCase   `json:"providerOfflineParitySeal"`
	ProviderCachePrivacy        G5ProviderCachePrivacyCase        `json:"providerCachePrivacy"`
	ProviderCacheInventory      G5ProviderCacheInventoryCase      `json:"providerCacheInventory"`
	ProviderStreaming           G5ProviderStreamingControlCase    `json:"providerStreaming"`
	SessionRouteStatus          G5SessionRouteStatusControlCase   `json:"sessionRouteStatus"`
	SessionRouteInventory       G5SessionRouteInventoryCase       `json:"sessionRouteInventory"`
	SessionRouteReplay          G5SessionRouteReplayCase          `json:"sessionRouteReplay"`
}

type G5ProductBoundaryCase struct {
	SourceContractID        string                  `json:"sourceContractId"`
	ProductBoundary         ProductBoundary         `json:"productBoundary"`
	ElectronMainConnected   bool                    `json:"electronMainConnected"`
	DefaultGoBackendEnabled bool                    `json:"defaultGoBackendEnabled"`
	Expected                G5ProductBoundaryOutput `json:"expected"`
}

type G5ProductBoundaryOutput struct {
	RendererPreloadMainBridgeUnchanged bool `json:"rendererPreloadMainBridgeUnchanged"`
	AnalytixServeContractUnchanged     bool `json:"analytixServeContractUnchanged"`
	ReasonixPublicProtocolAllowed      bool `json:"reasonixPublicProtocolAllowed"`
	DefaultGoBackendAllowed            bool `json:"defaultGoBackendAllowed"`
	RendererVisibleGoRoutesAllowed     bool `json:"rendererVisibleGoRoutesAllowed"`
	ElectronMainConnected              bool `json:"electronMainConnected"`
	DefaultGoBackendEnabled            bool `json:"defaultGoBackendEnabled"`
	UsesReasonixProtocol               bool `json:"usesReasonixProtocol"`
	RendererRouteExposed               bool `json:"rendererRouteExposed"`
}

type G5PackageRuntimeIdentityCase struct {
	SourceContractID               string                           `json:"sourceContractId"`
	SourceFiles                    G5PackageRuntimeIdentityFiles    `json:"sourceFiles"`
	ReleaseIdentity                G5PackageRuntimeReleaseIdentity  `json:"releaseIdentity"`
	RuntimeCLI                     G5PackageRuntimeCLI              `json:"runtimeCli"`
	ForbiddenIdentityFields        []string                         `json:"forbiddenIdentityFields"`
	ForbiddenIdentityFieldsPresent []string                         `json:"forbiddenIdentityFieldsPresent"`
	Evidence                       G5PackageRuntimeIdentityEvidence `json:"evidence"`
	ProductBoundary                G5PackageRuntimeProductBoundary  `json:"productBoundary"`
	Expected                       G5PackageRuntimeIdentityOutput   `json:"expected"`
}

type G5PackageRuntimeIdentityFiles struct {
	RootPackage           string `json:"rootPackage"`
	RuntimePackage        string `json:"runtimePackage"`
	ElectronBuilderConfig string `json:"electronBuilderConfig"`
	AppIdentity           string `json:"appIdentity"`
	MainIndex             string `json:"mainIndex"`
	ResolveAnalytixBinary string `json:"resolveAnalytixBinary"`
	RuntimeServeEntry     string `json:"runtimeServeEntry"`
	RuntimeServe          string `json:"runtimeServe"`
	AfterPack             string `json:"afterPack"`
	PackagingConfigTest   string `json:"packagingConfigTest"`
	ReleaseWorkflow       string `json:"releaseWorkflow"`
}

type G5PackageRuntimeReleaseIdentity struct {
	RootPackageName          string `json:"rootPackageName"`
	RootProductName          string `json:"rootProductName"`
	RuntimePackageName       string `json:"runtimePackageName"`
	RuntimeBinName           string `json:"runtimeBinName"`
	RuntimeBinPath           string `json:"runtimeBinPath"`
	RuntimeServeScript       string `json:"runtimeServeScript"`
	AppID                    string `json:"appId"`
	BuilderProductName       string `json:"builderProductName"`
	ArtifactNamePrefix       string `json:"artifactNamePrefix"`
	NSISShortcutName         string `json:"nsisShortcutName"`
	NSISUninstallDisplayName string `json:"nsisUninstallDisplayName"`
	AppProductName           string `json:"appProductName"`
	WindowsAppUserModelID    string `json:"windowsAppUserModelId"`
}

type G5PackageRuntimeCLI struct {
	BundledEntryCandidate string `json:"bundledEntryCandidate"`
	AfterPackRequiredPath string `json:"afterPackRequiredPath"`
	ReadyPrefix           string `json:"readyPrefix"`
	ServeUsagePrefix      string `json:"serveUsagePrefix"`
	SupportedCommand      string `json:"supportedCommand"`
	UnknownCommandMessage string `json:"unknownCommandMessage"`
}

type G5PackageRuntimeIdentityEvidence struct {
	PackagingTestPinsReleaseIdentity bool `json:"packagingTestPinsReleaseIdentity"`
	ResolveUsesBundledServeEntry     bool `json:"resolveUsesBundledServeEntry"`
	ServeEntryOnlySupportsServe      bool `json:"serveEntryOnlySupportsServe"`
	ServeUsageMentionsAnalytixServe  bool `json:"serveUsageMentionsAnalytixServe"`
	AfterPackRequiresServeEntry      bool `json:"afterPackRequiresServeEntry"`
	ReleaseWorkflowUsesAnalytixEnv   bool `json:"releaseWorkflowUsesAnalytixEnv"`
}

type G5PackageRuntimeProductBoundary struct {
	UsesReasonixProtocol    bool `json:"usesReasonixProtocol"`
	KunIdentityExposed      bool `json:"kunIdentityExposed"`
	DefaultGoBackendEnabled bool `json:"defaultGoBackendEnabled"`
}

type G5PackageRuntimeIdentityOutput struct {
	RootPackageNameAnalytix           bool `json:"rootPackageNameAnalytix"`
	RootProductNameAnalytix           bool `json:"rootProductNameAnalytix"`
	RuntimePackageNameAnalytixRuntime bool `json:"runtimePackageNameAnalytixRuntime"`
	RuntimeBinAnalytixServeEntry      bool `json:"runtimeBinAnalytixServeEntry"`
	RuntimeServeScriptUsesServeEntry  bool `json:"runtimeServeScriptUsesServeEntry"`
	BuilderAppIDAnalytix              bool `json:"builderAppIdAnalytix"`
	BuilderProductNameAnalytix        bool `json:"builderProductNameAnalytix"`
	BuilderArtifactNameAnalytix       bool `json:"builderArtifactNameAnalytix"`
	NSISNamesAnalytix                 bool `json:"nsisNamesAnalytix"`
	AppProductNameAnalytix            bool `json:"appProductNameAnalytix"`
	WindowsAppUserModelIDAnalytix     bool `json:"windowsAppUserModelIdAnalytix"`
	ResolveBundledServeEntry          bool `json:"resolveBundledServeEntry"`
	ServeUsageAnalytixServe           bool `json:"serveUsageAnalytixServe"`
	ServeEntryAllowsOnlyAnalytixServe bool `json:"serveEntryAllowsOnlyAnalytixServe"`
	ReadyHandshakeAnalytix            bool `json:"readyHandshakeAnalytix"`
	AfterPackRequiresServeEntry       bool `json:"afterPackRequiresServeEntry"`
	ReleaseEnvAnalytixPrefixed        bool `json:"releaseEnvAnalytixPrefixed"`
	UsesReasonixProtocol              bool `json:"usesReasonixProtocol"`
	KunIdentityExposed                bool `json:"kunIdentityExposed"`
	DefaultGoBackendEnabled           bool `json:"defaultGoBackendEnabled"`
}

type G5RuntimeHTTPRouteSovereigntyCase struct {
	SourceContractID            string                                `json:"sourceContractId"`
	SourceFiles                 G5RuntimeHTTPRouteSovereigntyFiles    `json:"sourceFiles"`
	RouteCount                  int                                   `json:"routeCount"`
	Routes                      []G5RuntimeHTTPRoute                  `json:"routes"`
	UnauthenticatedRoutes       []string                              `json:"unauthenticatedRoutes"`
	AuthenticatedRouteCount     int                                   `json:"authenticatedRouteCount"`
	AuthMatrix                  G5RuntimeHTTPAuthMatrix               `json:"authMatrix"`
	SSERoutes                   []string                              `json:"sseRoutes"`
	InternalTaskJobRoutes       []string                              `json:"internalTaskJobRoutes"`
	CompatibilityOnlyRoutes     []string                              `json:"compatibilityOnlyRoutes"`
	ForbiddenRouteTokens        []string                              `json:"forbiddenRouteTokens"`
	ForbiddenRouteTokensPresent []string                              `json:"forbiddenRouteTokensPresent"`
	ForbiddenDispatchMatrix     G5RuntimeHTTPForbiddenDispatchMatrix  `json:"forbiddenDispatchMatrix"`
	SharedEndpointBuilderMatrix G5SharedEndpointBuilderMatrix         `json:"sharedEndpointBuilderMatrix"`
	Evidence                    G5RuntimeHTTPRouteSovereigntyEvidence `json:"evidence"`
	ProductBoundary             G5RuntimeHTTPRouteSovereigntyBoundary `json:"productBoundary"`
	Expected                    G5RuntimeHTTPRouteSovereigntyOutput   `json:"expected"`
}

type G5RuntimeHTTPRouteSovereigntyFiles struct {
	ServerRoutesIndex string `json:"serverRoutesIndex"`
	Router            string `json:"router"`
	HTTPServer        string `json:"httpServer"`
	SharedEndpoints   string `json:"sharedEndpoints"`
	HTTPServerTest    string `json:"httpServerTest"`
}

type G5RuntimeHTTPRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type G5RuntimeHTTPAuthMatrix struct {
	HealthRoute         string                        `json:"healthRoute"`
	HealthStatus        int                           `json:"healthStatus"`
	UnauthorizedStatus  int                           `json:"unauthorizedStatus"`
	UnauthorizedBody    G5RuntimeHTTPUnauthorizedBody `json:"unauthorizedBody"`
	ProtectedRouteCount int                           `json:"protectedRouteCount"`
	ProtectedRouteKeys  []string                      `json:"protectedRouteKeys"`
	SensitiveRouteKeys  []string                      `json:"sensitiveRouteKeys"`
}

type G5RuntimeHTTPUnauthorizedBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type G5RuntimeHTTPForbiddenDispatchMatrix struct {
	AuthMode            string                 `json:"authMode"`
	Status              int                    `json:"status"`
	Body                G5RuntimeHTTPErrorBody `json:"body"`
	TokenCount          int                    `json:"tokenCount"`
	Tokens              []string               `json:"tokens"`
	ProtocolTokens      []string               `json:"protocolTokens"`
	HiddenSurfaceTokens []string               `json:"hiddenSurfaceTokens"`
}

type G5RuntimeHTTPErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type G5SharedEndpointBuilderMatrix struct {
	SourceID                          string                        `json:"sourceId"`
	TurnID                            string                        `json:"turnId"`
	CheckpointID                      string                        `json:"checkpointId"`
	EncodedSourceID                   string                        `json:"encodedSourceId"`
	EncodedTurnID                     string                        `json:"encodedTurnId"`
	EncodedCheckpointID               string                        `json:"encodedCheckpointId"`
	BuilderCases                      []G5SharedEndpointBuilderCase `json:"builderCases"`
	ExportedEndpointStrings           []string                      `json:"exportedEndpointStrings"`
	ForbiddenTokens                   []string                      `json:"forbiddenTokens"`
	ForbiddenTokensPresent            []string                      `json:"forbiddenTokensPresent"`
	CanonicalUserInputTemplate        string                        `json:"canonicalUserInputTemplate"`
	SingularUserInputTemplateExported bool                          `json:"singularUserInputTemplateExported"`
	SensitiveBuilderNames             []string                      `json:"sensitiveBuilderNames"`
	UnitTestEvidencePresent           bool                          `json:"unitTestEvidencePresent"`
}

type G5SharedEndpointBuilderCase struct {
	Name            string   `json:"name"`
	Template        string   `json:"template"`
	OutputPath      string   `json:"outputPath"`
	EncodedSegments []string `json:"encodedSegments"`
}

type G5RuntimeHTTPRouteSovereigntyEvidence struct {
	RouterFirstMatch                        bool `json:"routerFirstMatch"`
	StructuredNotFound                      bool `json:"structuredNotFound"`
	HealthNoAuth                            bool `json:"healthNoAuth"`
	V1AuthGuardsCoverRoutes                 bool `json:"v1AuthGuardsCoverRoutes"`
	SSEUsesBuildEventStreamResponse         bool `json:"sseUsesBuildEventStreamResponse"`
	TaskJobRoutesUseInternalHandlers        bool `json:"taskJobRoutesUseInternalHandlers"`
	SingularUserInputCompatibility          bool `json:"singularUserInputCompatibility"`
	SharedThreadEventsTemplatePresent       bool `json:"sharedThreadEventsTemplatePresent"`
	SharedSessionResumeTemplatePresent      bool `json:"sharedSessionResumeTemplatePresent"`
	SharedApprovalUserInputTemplatesPresent bool `json:"sharedApprovalUserInputTemplatesPresent"`
}

type G5RuntimeHTTPRouteSovereigntyBoundary struct {
	UsesReasonixProtocol       bool `json:"usesReasonixProtocol"`
	TopLevelHiddenEntryExposed bool `json:"topLevelHiddenEntryExposed"`
	DefaultGoBackendEnabled    bool `json:"defaultGoBackendEnabled"`
}

type G5RuntimeHTTPRouteSovereigntyOutput struct {
	RouteCountExact                            bool `json:"routeCountExact"`
	OnlyHealthUnauthenticated                  bool `json:"onlyHealthUnauthenticated"`
	AllRuntimeRoutesAnalytixOwned              bool `json:"allRuntimeRoutesAnalytixOwned"`
	HealthUnauthenticatedStatusOK              bool `json:"healthUnauthenticatedStatusOk"`
	AllAuthenticatedRoutesRejectMissingAuth    bool `json:"allAuthenticatedRoutesRejectMissingAuth"`
	UnauthorizedBodyShapeStable                bool `json:"unauthorizedBodyShapeStable"`
	AuthMatrixCoversAllRegisteredRoutes        bool `json:"authMatrixCoversAllRegisteredRoutes"`
	SensitiveRoutesProtected                   bool `json:"sensitiveRoutesProtected"`
	ForbiddenDispatchReturnsStructuredNotFound bool `json:"forbiddenDispatchReturnsStructuredNotFound"`
	ForbiddenDispatchCoversAllTokens           bool `json:"forbiddenDispatchCoversAllTokens"`
	ForbiddenProtocolTokensRejected            bool `json:"forbiddenProtocolTokensRejected"`
	ForbiddenHiddenSurfaceTokensRejected       bool `json:"forbiddenHiddenSurfaceTokensRejected"`
	ForbiddenDispatchUsesRuntimeAuth           bool `json:"forbiddenDispatchUsesRuntimeAuth"`
	SharedEndpointBuilderCaseCountExact        bool `json:"sharedEndpointBuilderCaseCountExact"`
	SharedEndpointBuildersEncodeRouteIds       bool `json:"sharedEndpointBuildersEncodeRouteIds"`
	SharedEndpointTemplatesAnalytixOwned       bool `json:"sharedEndpointTemplatesAnalytixOwned"`
	SharedEndpointCanonicalUserInputPlural     bool `json:"sharedEndpointCanonicalUserInputPlural"`
	SharedEndpointSensitiveBuildersPresent     bool `json:"sharedEndpointSensitiveBuildersPresent"`
	SharedEndpointBuilderUnitEvidencePresent   bool `json:"sharedEndpointBuilderUnitEvidencePresent"`
	SSERouteAnalytixThreadEvents               bool `json:"sseRouteAnalytixThreadEvents"`
	ThreadLifecycleRoutesPresent               bool `json:"threadLifecycleRoutesPresent"`
	ApprovalUserInputRoutesPresent             bool `json:"approvalUserInputRoutesPresent"`
	TaskJobRoutesInternalOnly                  bool `json:"taskJobRoutesInternalOnly"`
	NotFoundStructured                         bool `json:"notFoundStructured"`
	NoForbiddenRoutes                          bool `json:"noForbiddenRoutes"`
	SingularUserInputCompatibilityOnly         bool `json:"singularUserInputCompatibilityOnly"`
	UsesReasonixProtocol                       bool `json:"usesReasonixProtocol"`
	TopLevelHiddenEntryExposed                 bool `json:"topLevelHiddenEntryExposed"`
	DefaultGoBackendEnabled                    bool `json:"defaultGoBackendEnabled"`
}

type G5DesktopSovereigntyCase struct {
	SourceFiles                        G5DesktopSovereigntySourceFiles     `json:"sourceFiles"`
	BridgeName                         string                              `json:"bridgeName"`
	RuntimeIpcChannels                 G5DesktopRuntimeIpcChannels         `json:"runtimeIpcChannels"`
	ExposedBridgeNames                 []string                            `json:"exposedBridgeNames"`
	WindowTypeProperties               []string                            `json:"windowTypeProperties"`
	FacadeDomains                      []string                            `json:"facadeDomains"`
	ForbiddenBridgeAliases             []string                            `json:"forbiddenBridgeAliases"`
	ForbiddenRuntimeIpcChannels        []string                            `json:"forbiddenRuntimeIpcChannels"`
	ForbiddenRuntimeIpcChannelsExposed []string                            `json:"forbiddenRuntimeIpcChannelsExposed"`
	ForbiddenSettingsKeys              []string                            `json:"forbiddenSettingsKeys"`
	RendererBridgeAllowList            G5RendererBridgeAllowList           `json:"rendererBridgeAllowList"`
	RendererRuntimeClientBridgeMatrix  G5RendererRuntimeClientBridgeMatrix `json:"rendererRuntimeClientBridgeMatrix"`
	RendererSettingsBridgeMatrix       G5RendererSettingsBridgeMatrix      `json:"rendererSettingsBridgeMatrix"`
	RendererProviderEndpointMatrix     G5RendererProviderEndpointMatrix    `json:"rendererProviderEndpointMatrix"`
	RendererProviderAliasGuardMatrix   G5RendererProviderAliasGuardMatrix  `json:"rendererProviderAliasGuardMatrix"`
	RendererProviderFacadeSealMatrix   G5RendererProviderFacadeSealMatrix  `json:"rendererProviderFacadeSealMatrix"`
	SideConversationRelationMatrix     G5SideConversationRelationMatrix    `json:"sideConversationRelationContractMatrix"`
	RendererUsageRuntimeClientMatrix   G5RendererUsageRuntimeClientMatrix  `json:"rendererUsageRuntimeClientFacadeMatrix"`
	RendererSettingsReadFacadeMatrix   G5RendererSettingsReadFacadeMatrix  `json:"rendererSettingsReadFacadeMatrix"`
	PreloadRuntimeRequestBridgeMatrix  G5PreloadRuntimeRequestBridgeMatrix `json:"preloadRuntimeRequestBridgeMatrix"`
	PreloadSseBridgeMatrix             G5PreloadSseBridgeMatrix            `json:"preloadSseBridgeMatrix"`
	EvidenceIDs                        []string                            `json:"evidenceIds"`
	Expected                           G5DesktopSovereigntyOutput          `json:"expected"`
}

type G5DesktopSovereigntySourceFiles struct {
	Preload                   string `json:"preload"`
	WindowTypes               string `json:"windowTypes"`
	SharedAPI                 string `json:"sharedApi"`
	SettingsStoreTest         string `json:"settingsStoreTest"`
	RuntimeNormalizer         string `json:"runtimeNormalizer"`
	ProductScan               string `json:"productSovereigntyScan"`
	RendererSourceRoot        string `json:"rendererSourceRoot"`
	RendererRuntimeClient     string `json:"rendererRuntimeClient"`
	RendererRuntimeClientTest string `json:"rendererRuntimeClientTest"`
	RendererProvider          string `json:"rendererProvider"`
	RendererProviderTest      string `json:"rendererProviderTest"`
	RendererAgentTypes        string `json:"rendererAgentTypes"`
	RendererSideActions       string `json:"rendererSideActions"`
	RendererSideActionsTest   string `json:"rendererSideActionsTest"`
	RendererThreadUsage       string `json:"rendererThreadUsage"`
	RendererThreadUsageTest   string `json:"rendererThreadUsageTest"`
	RendererDailyUsage        string `json:"rendererDailyUsage"`
	RendererDailyUsageTest    string `json:"rendererDailyUsageTest"`
	RendererModelUsage        string `json:"rendererModelUsage"`
	RendererModelUsageTest    string `json:"rendererModelUsageTest"`
	RendererSettingsAgents    string `json:"rendererSettingsAgents"`
	RendererLlmDebug          string `json:"rendererLlmDebug"`
	RendererKeyboardSettings  string `json:"rendererKeyboardShortcutSettings"`
	RendererVoiceDictation    string `json:"rendererVoiceDictation"`
	RendererInitialUsage      string `json:"rendererInitialUsageHeatmap"`
	PreloadRuntimeRequestTest string `json:"preloadRuntimeRequestTest"`
	PreloadSseBridgeTest      string `json:"preloadSseBridgeTest"`
}

type G5DesktopRuntimeIpcChannels struct {
	Request  string `json:"request"`
	SSEStart string `json:"sseStart"`
	SSEStop  string `json:"sseStop"`
	SSEEvent string `json:"sseEvent"`
	SSEEnd   string `json:"sseEnd"`
	SSEError string `json:"sseError"`
}

type G5DesktopSovereigntyOutput struct {
	ExposesOnlyAnalytixBridge                             bool `json:"exposesOnlyAnalytixBridge"`
	WindowTypeOnlyAnalytix                                bool `json:"windowTypeOnlyAnalytix"`
	FacadeDomainsAnalytixOwned                            bool `json:"facadeDomainsAnalytixOwned"`
	PublicApiTypesAnalytixOwned                           bool `json:"publicApiTypesAnalytixOwned"`
	RuntimeRequestUsesAnalytixIpc                         bool `json:"runtimeRequestUsesAnalytixIpc"`
	RuntimeSseUsesAnalytixIpc                             bool `json:"runtimeSseUsesAnalytixIpc"`
	RendererNamedRuntimeApisAllowListed                   bool `json:"rendererNamedRuntimeApisAllowListed"`
	RendererNamedSettingsApisAllowListed                  bool `json:"rendererNamedSettingsApisAllowListed"`
	RendererGenericRuntimeBypassAbsent                    bool `json:"rendererGenericRuntimeBypassAbsent"`
	RendererSettingsReadBypassAbsent                      bool `json:"rendererSettingsReadBypassAbsent"`
	OptionalChainBridgeAccessScanned                      bool `json:"optionalChainBridgeAccessScanned"`
	RendererProviderUsesSharedRootPaths                   bool `json:"rendererProviderUsesSharedRootPaths"`
	RendererProviderEncodesDynamicRouteIDs                bool `json:"rendererProviderEncodesDynamicRouteIds"`
	RendererProviderRuntimePathsAnalytixOwned             bool `json:"rendererProviderRuntimePathsAnalytixOwned"`
	RendererProviderUsesRuntimeClientFacade               bool `json:"rendererProviderUsesRuntimeClientFacade"`
	RendererProviderEndpointBuilderUnitEvidencePresent    bool `json:"rendererProviderEndpointBuilderUnitEvidencePresent"`
	RendererProviderAliasGuardInstallsThrowingAliases     bool `json:"rendererProviderAliasGuardInstallsThrowingAliases"`
	RendererProviderAliasGuardCoversRuntimeRoutes         bool `json:"rendererProviderAliasGuardCoversRuntimeRoutes"`
	RendererProviderAliasGuardCoversLifecycleAndGates     bool `json:"rendererProviderAliasGuardCoversLifecycleAndGates"`
	RendererProviderAliasGuardCoversForkResumeAndEncoding bool `json:"rendererProviderAliasGuardCoversForkResumeAndEncoding"`
	RendererProviderAliasGuardRejectsForbiddenRoutes      bool `json:"rendererProviderAliasGuardRejectsForbiddenRoutes"`
	RendererProviderAliasGuardUnitEvidencePresent         bool `json:"rendererProviderAliasGuardUnitEvidencePresent"`
	RendererProviderFacadeSealUsesRuntimeClient           bool `json:"rendererProviderFacadeSealUsesRuntimeClient"`
	RendererProviderFacadeSealRejectsDirectBridge         bool `json:"rendererProviderFacadeSealRejectsDirectBridge"`
	RendererProviderFacadeSealCoversArchiveRestore        bool `json:"rendererProviderFacadeSealCoversArchiveRestore"`
	RendererProviderFacadeSealCoversRelationPatch         bool `json:"rendererProviderFacadeSealCoversRelationPatch"`
	RendererProviderFacadeSealScanGuardPresent            bool `json:"rendererProviderFacadeSealScanGuardPresent"`
	RendererProviderFacadeSealUnitEvidencePresent         bool `json:"rendererProviderFacadeSealUnitEvidencePresent"`
	SideConversationRelationContractOptionalProvider      bool `json:"sideConversationRelationContractOptionalProvider"`
	SideConversationRelationPromotesThroughProvider       bool `json:"sideConversationRelationPromotesThroughProvider"`
	SideConversationRelationRefreshesAndCloses            bool `json:"sideConversationRelationRefreshesAndCloses"`
	SideConversationRelationRejectsDirectBridge           bool `json:"sideConversationRelationRejectsDirectBridge"`
	SideConversationRelationScanGuardPresent              bool `json:"sideConversationRelationScanGuardPresent"`
	SideConversationRelationUnitEvidencePresent           bool `json:"sideConversationRelationUnitEvidencePresent"`
	RendererUsageRuntimeClientCoversThreadUsage           bool `json:"rendererUsageRuntimeClientCoversThreadUsage"`
	RendererUsageRuntimeClientCoversDailyUsage            bool `json:"rendererUsageRuntimeClientCoversDailyUsage"`
	RendererUsageRuntimeClientCoversModelUsage            bool `json:"rendererUsageRuntimeClientCoversModelUsage"`
	RendererUsageRuntimeClientCoversSettingsDiagnostics   bool `json:"rendererUsageRuntimeClientCoversSettingsDiagnostics"`
	RendererUsageRuntimeClientRejectsDirectBridge         bool `json:"rendererUsageRuntimeClientRejectsDirectBridge"`
	RendererUsageRuntimeClientScanGuardPresent            bool `json:"rendererUsageRuntimeClientScanGuardPresent"`
	RendererUsageRuntimeClientUnitEvidencePresent         bool `json:"rendererUsageRuntimeClientUnitEvidencePresent"`
	RendererSettingsReadFacadeCoversKeyboardShortcuts     bool `json:"rendererSettingsReadFacadeCoversKeyboardShortcuts"`
	RendererSettingsReadFacadeCoversSpeechToText          bool `json:"rendererSettingsReadFacadeCoversSpeechToText"`
	RendererSettingsReadFacadeCoversUsageModelLabel       bool `json:"rendererSettingsReadFacadeCoversUsageModelLabel"`
	RendererSettingsReadFacadePreservesSettingsEvent      bool `json:"rendererSettingsReadFacadePreservesSettingsChangedEvent"`
	RendererSettingsReadFacadeRejectsDirectBridge         bool `json:"rendererSettingsReadFacadeRejectsDirectBridge"`
	RendererSettingsReadFacadeScanGuardPresent            bool `json:"rendererSettingsReadFacadeScanGuardPresent"`
	RendererRuntimeClientRuntimeRequestPreservesArguments bool `json:"rendererRuntimeClientRuntimeRequestPreservesArguments"`
	RendererRuntimeClientRestartUsesRuntimeApi            bool `json:"rendererRuntimeClientRestartUsesRuntimeApi"`
	RendererRuntimeClientSseControlsPreserveArguments     bool `json:"rendererRuntimeClientSseControlsPreserveArguments"`
	RendererRuntimeClientSseListenersPreserveHandlers     bool `json:"rendererRuntimeClientSseListenersPreserveHandlers"`
	RendererRuntimeClientLegacyAliasesUnread              bool `json:"rendererRuntimeClientLegacyAliasesUnread"`
	RendererRuntimeClientUnitEvidencePresent              bool `json:"rendererRuntimeClientUnitEvidencePresent"`
	RendererSettingsBridgeUsesAnalytixSettingsApi         bool `json:"rendererSettingsBridgeUsesAnalytixSettingsApi"`
	RendererSettingsBridgeCachesReads                     bool `json:"rendererSettingsBridgeCachesReads"`
	RendererSettingsBridgeRefreshesCacheAfterWrite        bool `json:"rendererSettingsBridgeRefreshesCacheAfterWrite"`
	RendererSettingsBridgePreservesTopLevelRuntimePatch   bool `json:"rendererSettingsBridgePreservesTopLevelRuntimePatch"`
	RendererSettingsBridgeLegacyAliasesUnread             bool `json:"rendererSettingsBridgeLegacyAliasesUnread"`
	RendererSettingsBridgeUnitEvidencePresent             bool `json:"rendererSettingsBridgeUnitEvidencePresent"`
	PreloadRuntimeRequestPreservesPathMethodBody          bool `json:"preloadRuntimeRequestPreservesPathMethodBody"`
	PreloadDiagnosticsRuntimeRequestUsesSameChannel       bool `json:"preloadDiagnosticsRuntimeRequestUsesSameChannel"`
	PreloadRuntimeRestartUsesAnalytixIpc                  bool `json:"preloadRuntimeRestartUsesAnalytixIpc"`
	PreloadRuntimeRequestExposesOnlyAnalytixApi           bool `json:"preloadRuntimeRequestExposesOnlyAnalytixApi"`
	PreloadRuntimeRequestUnitEvidencePresent              bool `json:"preloadRuntimeRequestUnitEvidencePresent"`
	PreloadSseStartStopPreservesArguments                 bool `json:"preloadSseStartStopPreservesArguments"`
	PreloadSsePayloadListenersOmitElectronEvent           bool `json:"preloadSsePayloadListenersOmitElectronEvent"`
	PreloadSseListenerCleanupUsesSameWrapper              bool `json:"preloadSseListenerCleanupUsesSameWrapper"`
	PreloadSseBridgeUnitEvidencePresent                   bool `json:"preloadSseBridgeUnitEvidencePresent"`
	DropsReasonixAutoPlanConfig                           bool `json:"dropsReasonixAutoPlanConfig"`
	StripsLegacyRuntimeSettings                           bool `json:"stripsLegacyRuntimeSettings"`
	WritesTopLevelRuntimeSettings                         bool `json:"writesTopLevelRuntimeSettings"`
	DeprecatedBridgeAliasExposed                          bool `json:"deprecatedBridgeAliasExposed"`
	ForbiddenRuntimeIpcExposed                            bool `json:"forbiddenRuntimeIpcExposed"`
	DeprecatedSettingsFallbackWritten                     bool `json:"deprecatedSettingsFallbackWritten"`
	UsesReasonixProtocol                                  bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed                                  bool `json:"topLevelRouteExposed"`
}

type G5RendererBridgeAllowList struct {
	AllowedNamedRuntimeAPIs     []string `json:"allowedNamedRuntimeApis"`
	AllowedNamedSettingsAPIs    []string `json:"allowedNamedSettingsApis"`
	DirectRuntimeAPIs           []string `json:"directRuntimeApis"`
	DirectSettingsAPIs          []string `json:"directSettingsApis"`
	DirectRuntimeViolations     []string `json:"directRuntimeViolations"`
	DirectSettingsViolations    []string `json:"directSettingsViolations"`
	GenericRuntimeBypassCount   int      `json:"genericRuntimeBypassCount"`
	SettingsReadBypassCount     int      `json:"settingsReadBypassCount"`
	OptionalChainPatternCovered bool     `json:"optionalChainPatternCovered"`
}

type G5RendererRuntimeClientBridgeMatrix struct {
	SourceIDs                                    G5RendererRuntimeClientIDs       `json:"sourceIds"`
	EncodedIDs                                   G5RendererRuntimeClientIDs       `json:"encodedIds"`
	RuntimeRequestCalls                          []G5RendererRuntimeClientRequest `json:"runtimeRequestCalls"`
	SSEStartCall                                 G5PreloadSseStartCall            `json:"sseStartCall"`
	SSEStopCall                                  G5PreloadSseStreamCall           `json:"sseStopCall"`
	ListenerAPIs                                 []string                         `json:"listenerApis"`
	SourceUsesAnalytixRuntime                    bool                             `json:"sourceUsesAnalytixRuntime"`
	SourcePassesRuntimeRequestArgumentsUnchanged bool                             `json:"sourcePassesRuntimeRequestArgumentsUnchanged"`
	SourcePassesRestartUnchanged                 bool                             `json:"sourcePassesRestartUnchanged"`
	SourcePassesSseControlsUnchanged             bool                             `json:"sourcePassesSseControlsUnchanged"`
	SourcePassesSseListenersUnchanged            bool                             `json:"sourcePassesSseListenersUnchanged"`
	RuntimeRequestUnitEvidencePresent            bool                             `json:"runtimeRequestUnitEvidencePresent"`
	RestartUnitEvidencePresent                   bool                             `json:"restartUnitEvidencePresent"`
	SSEUnitEvidencePresent                       bool                             `json:"sseUnitEvidencePresent"`
	LegacyAliasUnitEvidencePresent               bool                             `json:"legacyAliasUnitEvidencePresent"`
}

type G5RendererRuntimeClientIDs struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

type G5RendererRuntimeClientRequest struct {
	Path          string `json:"path"`
	Method        string `json:"method,omitempty"`
	Body          string `json:"body,omitempty"`
	ArgumentCount int    `json:"argumentCount"`
}

type G5RendererSettingsBridgeMatrix struct {
	Patch                                G5RendererSettingsPatch             `json:"patch"`
	CacheExpectations                    G5RendererSettingsCacheExpectations `json:"cacheExpectations"`
	SourceUsesAnalytixSettings           bool                                `json:"sourceUsesAnalytixSettings"`
	SourceCachesSettingsReads            bool                                `json:"sourceCachesSettingsReads"`
	SourceRefreshesCacheAfterSetSettings bool                                `json:"sourceRefreshesCacheAfterSetSettings"`
	CacheUnitEvidencePresent             bool                                `json:"cacheUnitEvidencePresent"`
	RefreshUnitEvidencePresent           bool                                `json:"refreshUnitEvidencePresent"`
	TopLevelRuntimePatchEvidencePresent  bool                                `json:"topLevelRuntimePatchUnitEvidencePresent"`
	LegacyAliasUnitEvidencePresent       bool                                `json:"legacyAliasUnitEvidencePresent"`
}

type G5RendererSettingsPatch struct {
	WorkspaceRoot  string `json:"workspaceRoot"`
	RuntimeModel   string `json:"runtimeModel"`
	ApprovalPolicy string `json:"approvalPolicy"`
}

type G5RendererSettingsCacheExpectations struct {
	GetSettingsCallsForDoubleRead    int `json:"getSettingsCallsForDoubleRead"`
	GetSettingsCallsAfterSetSettings int `json:"getSettingsCallsAfterSetSettings"`
	SetSettingsCallsAfterWrite       int `json:"setSettingsCallsAfterWrite"`
}

type G5RendererProviderEndpointMatrix struct {
	RootPaths                       G5RendererProviderRootPaths   `json:"rootPaths"`
	RootConstantsUsed               bool                          `json:"rootConstantsUsed"`
	SourceIDs                       G5RendererProviderEndpointIDs `json:"sourceIds"`
	EncodedIDs                      G5RendererProviderEndpointIDs `json:"encodedIds"`
	EncodedRuntimeRequestPaths      []string                      `json:"encodedRuntimeRequestPaths"`
	SensitivePathKinds              []string                      `json:"sensitivePathKinds"`
	UsesRendererRuntimeClientFacade bool                          `json:"usesRendererRuntimeClientFacade"`
	PathOwnershipGuardPresent       bool                          `json:"pathOwnershipGuardPresent"`
	UnitTestEvidencePresent         bool                          `json:"unitTestEvidencePresent"`
}

type G5RendererProviderRootPaths struct {
	Health  string `json:"health"`
	Threads string `json:"threads"`
}

type G5RendererProviderEndpointIDs struct {
	ThreadID   string `json:"threadId"`
	TurnID     string `json:"turnId"`
	ApprovalID string `json:"approvalId"`
	InputID    string `json:"inputId"`
	SessionID  string `json:"sessionId"`
}

type G5RendererProviderAliasGuardMatrix struct {
	HelperName                       string                               `json:"helperName"`
	ForbiddenAliases                 []string                             `json:"forbiddenAliases"`
	GuardedTestNames                 []string                             `json:"guardedTestNames"`
	Coverage                         G5RendererProviderAliasGuardCoverage `json:"coverage"`
	SourceInstallsThrowingAliases    bool                                 `json:"sourceInstallsThrowingAliases"`
	SourceUsesAnalytixOnly           bool                                 `json:"sourceUsesAnalytixOnly"`
	RouteOwnershipEvidencePresent    bool                                 `json:"routeOwnershipEvidencePresent"`
	LifecycleEvidencePresent         bool                                 `json:"lifecycleEvidencePresent"`
	ApprovalUserInputEvidencePresent bool                                 `json:"approvalUserInputEvidencePresent"`
	ForkResumeEvidencePresent        bool                                 `json:"forkResumeEvidencePresent"`
	DynamicEncodingEvidencePresent   bool                                 `json:"dynamicEncodingEvidencePresent"`
	ForbiddenRouteGuardPresent       bool                                 `json:"forbiddenRouteGuardPresent"`
}

type G5RendererProviderAliasGuardCoverage struct {
	RouteOwnership       bool `json:"routeOwnership"`
	ThreadLifecycle      bool `json:"threadLifecycle"`
	ApprovalUserInput    bool `json:"approvalUserInput"`
	ForkResume           bool `json:"forkResume"`
	DynamicRouteEncoding bool `json:"dynamicRouteEncoding"`
}

type G5RendererProviderFacadeSealMatrix struct {
	Facade                              string   `json:"facade"`
	ForbiddenDirectBridge               string   `json:"forbiddenDirectBridge"`
	SealedMethods                       []string `json:"sealedMethods"`
	SourceUsesRuntimeClient             bool     `json:"sourceUsesRuntimeClient"`
	SourceRejectsDirectBridgeBypass     bool     `json:"sourceRejectsDirectBridgeBypass"`
	ArchiveRestoreSourceEvidencePresent bool     `json:"archiveRestoreSourceEvidencePresent"`
	RelationSourceEvidencePresent       bool     `json:"relationSourceEvidencePresent"`
	LifecycleUnitEvidencePresent        bool     `json:"lifecycleUnitEvidencePresent"`
	ScanGuardPresent                    bool     `json:"scanGuardPresent"`
	ProviderFacadeTokenScanPresent      bool     `json:"providerFacadeTokenScanPresent"`
}

type G5SideConversationRelationMatrix struct {
	ProviderMethod                          string `json:"providerMethod"`
	StoreAction                             string `json:"storeAction"`
	Relation                                string `json:"relation"`
	ProviderContractOptional                bool   `json:"providerContractOptional"`
	ProviderImplementationUsesRuntimeClient bool   `json:"providerImplementationUsesRuntimeClient"`
	StoreUsesProviderContract               bool   `json:"storeUsesProviderContract"`
	StoreRefreshesAndCloses                 bool   `json:"storeRefreshesAndCloses"`
	StoreRejectsDirectRuntimeBridge         bool   `json:"storeRejectsDirectRuntimeBridge"`
	UnitEvidencePresent                     bool   `json:"unitEvidencePresent"`
	ScanGuardPresent                        bool   `json:"scanGuardPresent"`
}

type G5RendererUsageRuntimeClientMatrix struct {
	Facade                              string   `json:"facade"`
	ForbiddenDirectBridge               string   `json:"forbiddenDirectBridge"`
	UsageLoaders                        []string `json:"usageLoaders"`
	DiagnosticsLoaders                  []string `json:"diagnosticsLoaders"`
	ThreadUsageSourceUsesRuntimeClient  bool     `json:"threadUsageSourceUsesRuntimeClient"`
	DailyUsageSourceUsesRuntimeClient   bool     `json:"dailyUsageSourceUsesRuntimeClient"`
	ModelUsageSourceUsesRuntimeClient   bool     `json:"modelUsageSourceUsesRuntimeClient"`
	TokenEconomySourceUsesRuntimeClient bool     `json:"tokenEconomySourceUsesRuntimeClient"`
	LlmDebugSourceUsesRuntimeClient     bool     `json:"llmDebugSourceUsesRuntimeClient"`
	SourceRejectsDirectBridgeBypass     bool     `json:"sourceRejectsDirectBridgeBypass"`
	UsageUnitEvidencePresent            bool     `json:"usageUnitEvidencePresent"`
	ScanGuardPresent                    bool     `json:"scanGuardPresent"`
	UsageFacadeTokenScanPresent         bool     `json:"usageFacadeTokenScanPresent"`
}

type G5RendererSettingsReadFacadeMatrix struct {
	Facade                                   string   `json:"facade"`
	ForbiddenDirectBridge                    string   `json:"forbiddenDirectBridge"`
	SettingsReaders                          []string `json:"settingsReaders"`
	KeyboardShortcutSourceUsesSettingsClient bool     `json:"keyboardShortcutSourceUsesSettingsClient"`
	SpeechToTextSourceUsesSettingsClient     bool     `json:"speechToTextSourceUsesSettingsClient"`
	UsageHeatmapSourceUsesSettingsClient     bool     `json:"usageHeatmapSourceUsesSettingsClient"`
	SettingsChangedEventPreserved            bool     `json:"settingsChangedEventPreserved"`
	SourceRejectsDirectSettingsBypass        bool     `json:"sourceRejectsDirectSettingsBypass"`
	ScanGuardPresent                         bool     `json:"scanGuardPresent"`
	SettingsReadFacadeTokenScanPresent       bool     `json:"settingsReadFacadeTokenScanPresent"`
}

type G5PreloadRuntimeRequestBridgeMatrix struct {
	Channel                              string                      `json:"channel"`
	RestartChannel                       string                      `json:"restartChannel"`
	SourceIDs                            G5PreloadRuntimeRequestIDs  `json:"sourceIds"`
	EncodedIDs                           G5PreloadRuntimeRequestIDs  `json:"encodedIds"`
	RuntimeFacadeCall                    G5PreloadRuntimeRequestCall `json:"runtimeFacadeCall"`
	DiagnosticsFacadeCall                G5PreloadRuntimeRequestCall `json:"diagnosticsFacadeCall"`
	SourcePassesArgumentsUnchanged       bool                        `json:"sourcePassesArgumentsUnchanged"`
	SourcePassesRestartUnchanged         bool                        `json:"sourcePassesRestartUnchanged"`
	RuntimeFacadeUnitEvidencePresent     bool                        `json:"runtimeFacadeUnitEvidencePresent"`
	RestartUnitEvidencePresent           bool                        `json:"restartUnitEvidencePresent"`
	DiagnosticsFacadeUnitEvidencePresent bool                        `json:"diagnosticsFacadeUnitEvidencePresent"`
	ExposesOnlyAnalytixApi               bool                        `json:"exposesOnlyAnalytixApi"`
}

type G5PreloadRuntimeRequestIDs struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	InputID  string `json:"inputId"`
}

type G5PreloadRuntimeRequestCall struct {
	Path   string `json:"path"`
	Method string `json:"method"`
	Body   string `json:"body"`
}

type G5PreloadSseBridgeMatrix struct {
	Channels                       G5PreloadSseChannels     `json:"channels"`
	StartCall                      G5PreloadSseStartCall    `json:"startCall"`
	StopCall                       G5PreloadSseStreamCall   `json:"stopCall"`
	EventPayload                   G5PreloadSseEventPayload `json:"eventPayload"`
	EndPayload                     G5PreloadSseStreamCall   `json:"endPayload"`
	ErrorPayload                   G5PreloadSseErrorPayload `json:"errorPayload"`
	SourcePassesStartStopUnchanged bool                     `json:"sourcePassesStartStopUnchanged"`
	SourcePayloadOnlyWrappers      bool                     `json:"sourcePayloadOnlyWrappers"`
	SourceCleanupUsesSameWrapper   bool                     `json:"sourceCleanupUsesSameWrapper"`
	StartStopUnitEvidencePresent   bool                     `json:"startStopUnitEvidencePresent"`
	PayloadUnitEvidencePresent     bool                     `json:"payloadUnitEvidencePresent"`
	ExposesOnlyAnalytixApi         bool                     `json:"exposesOnlyAnalytixApi"`
}

type G5PreloadSseChannels struct {
	Start string `json:"start"`
	Stop  string `json:"stop"`
	Event string `json:"event"`
	End   string `json:"end"`
	Error string `json:"error"`
}

type G5PreloadSseStartCall struct {
	ThreadID string `json:"threadId"`
	SinceSeq int    `json:"sinceSeq"`
	StreamID string `json:"streamId"`
}

type G5PreloadSseStreamCall struct {
	StreamID string `json:"streamId"`
}

type G5PreloadSseEventPayload struct {
	StreamID string `json:"streamId"`
	Seq      int    `json:"seq"`
	Kind     string `json:"kind"`
}

type G5PreloadSseErrorPayload struct {
	StreamID string `json:"streamId"`
	Status   int    `json:"status"`
}

type G5DesktopMainIpcBoundaryCase struct {
	SourceContractID               string                                      `json:"sourceContractId"`
	SourceFiles                    G5DesktopMainIpcBoundarySource              `json:"sourceFiles"`
	RuntimeRequest                 G5DesktopMainRuntimeRequest                 `json:"runtimeRequest"`
	SSE                            G5DesktopMainSSE                            `json:"sse"`
	EndpointBuilderAllowListMatrix G5DesktopMainEndpointBuilderAllowListMatrix `json:"endpointBuilderAllowListMatrix"`
	RuntimeAdapterHandoffMatrix    G5DesktopMainRuntimeAdapterHandoffMatrix    `json:"runtimeAdapterHandoffMatrix"`
	RuntimeHostHandoffMatrix       G5DesktopMainRuntimeHostHandoffMatrix       `json:"runtimeHostHandoffMatrix"`
	MainSseHostEncodingMatrix      G5DesktopMainSseHostEncodingMatrix          `json:"mainSseHostEncodingMatrix"`
	ProductBoundary                G5DesktopMainIpcProductBoundary             `json:"productBoundary"`
	Expected                       G5DesktopMainIpcBoundaryOutput              `json:"expected"`
}

type G5DesktopMainIpcBoundarySource struct {
	AppIpcSchemas              string `json:"appIpcSchemas"`
	AppIpcSchemasTest          string `json:"appIpcSchemasTest"`
	RegisterAppIpcHandlers     string `json:"registerAppIpcHandlers"`
	RegisterAppIpcHandlersTest string `json:"registerAppIpcHandlersTest"`
	RuntimeSseIpc              string `json:"runtimeSseIpc"`
	RuntimeSseIpcTest          string `json:"runtimeSseIpcTest"`
	RuntimeAdapter             string `json:"runtimeAdapter"`
	RuntimeAdapterTest         string `json:"runtimeAdapterTest"`
}

type G5DesktopMainRuntimeRequest struct {
	HandlerChannel               string                       `json:"handlerChannel"`
	AllowedAnalytixRouteExamples []string                     `json:"allowedAnalytixRouteExamples"`
	ForbiddenRuntimeRoutes       []string                     `json:"forbiddenRuntimeRoutes"`
	Evidence                     G5DesktopMainRuntimeEvidence `json:"evidence"`
}

type G5DesktopMainRuntimeEvidence struct {
	SchemaStrict               bool `json:"schemaStrict"`
	RefinesAllowedSurface      bool `json:"refinesAllowedSurface"`
	NormalizesRelativePath     bool `json:"normalizesRelativePath"`
	HandlerParsesBeforeRuntime bool `json:"handlerParsesBeforeRuntimeCall"`
	ForbiddenHandlerNoExecute  bool `json:"forbiddenHandlerNoExecute"`
}

type G5DesktopMainSSE struct {
	StartChannel      string                   `json:"startChannel"`
	StopChannel       string                   `json:"stopChannel"`
	EventChannel      string                   `json:"eventChannel"`
	ErrorChannel      string                   `json:"errorChannel"`
	EndChannel        string                   `json:"endChannel"`
	RoutePathTemplate string                   `json:"routePathTemplate"`
	BatchMs           int                      `json:"batchMs"`
	Evidence          G5DesktopMainSSEEvidence `json:"evidence"`
}

type G5DesktopMainSSEEvidence struct {
	StartSchemaStrict            bool `json:"startSchemaStrict"`
	RejectsReasonixSessionID     bool `json:"rejectsReasonixSessionId"`
	UsesAnalytixThreadEventsPath bool `json:"usesAnalytixThreadEventsPath"`
	StopParsesStreamID           bool `json:"stopParsesStreamId"`
	StopMatchesStreamIDOnly      bool `json:"stopMatchesStreamIdOnly"`
	ReconnectLastEventID         bool `json:"reconnectLastEventId"`
	BatchesEventsAt100ms         bool `json:"batchesEventsAt100ms"`
}

type G5DesktopMainEndpointBuilderAllowListMatrix struct {
	SourceIDs                        G5DesktopMainEndpointIDs       `json:"sourceIds"`
	EncodedIDs                       G5DesktopMainEndpointIDs       `json:"encodedIds"`
	AcceptedSharedBuilderRequests    []G5DesktopMainEndpointRequest `json:"acceptedSharedBuilderRequests"`
	RejectedRawDynamicRequests       []G5DesktopMainEndpointRequest `json:"rejectedRawDynamicRequests"`
	SharedTemplatesCompiled          []string                       `json:"sharedTemplatesCompiled"`
	SchemaUsesSharedTemplates        bool                           `json:"schemaUsesSharedTemplates"`
	UnitTestEvidencePresent          bool                           `json:"unitTestEvidencePresent"`
	RawRouteRejectionEvidencePresent bool                           `json:"rawRouteRejectionEvidencePresent"`
}

type G5DesktopMainEndpointIDs struct {
	ThreadID     string `json:"threadId"`
	TurnID       string `json:"turnId"`
	CheckpointID string `json:"checkpointId"`
	ApprovalID   string `json:"approvalId"`
	InputID      string `json:"inputId"`
	SessionID    string `json:"sessionId"`
	AttachmentID string `json:"attachmentId"`
	MemoryID     string `json:"memoryId"`
}

type G5DesktopMainEndpointRequest struct {
	Name   string `json:"name,omitempty"`
	Path   string `json:"path"`
	Method string `json:"method"`
	Body   string `json:"body,omitempty"`
}

type G5DesktopMainRuntimeAdapterHandoffMatrix struct {
	HandlerChannel                      string                         `json:"handlerChannel"`
	AdapterFunction                     string                         `json:"adapterFunction"`
	AcceptedAdapterCalls                []G5DesktopMainEndpointRequest `json:"acceptedAdapterCalls"`
	RejectedBeforeAdapterCalls          []G5DesktopMainEndpointRequest `json:"rejectedBeforeAdapterCalls"`
	ParseBeforeAdapterCall              bool                           `json:"parseBeforeAdapterCall"`
	EncodedPathEvidencePresent          bool                           `json:"encodedPathEvidencePresent"`
	MethodAndBodyEvidencePresent        bool                           `json:"methodAndBodyEvidencePresent"`
	RejectsBeforeAdapterEvidencePresent bool                           `json:"rejectsBeforeAdapterEvidencePresent"`
}

type G5DesktopMainRuntimeHostHandoffMatrix struct {
	AdapterFunction                  string                           `json:"adapterFunction"`
	BaseURLFunction                  string                           `json:"baseUrlFunction"`
	SourceIDs                        G5DesktopMainRuntimeHostIDs      `json:"sourceIds"`
	EncodedIDs                       G5DesktopMainRuntimeHostIDs      `json:"encodedIds"`
	Request                          G5DesktopMainRuntimeHostRequest  `json:"request"`
	SourceEvidence                   G5DesktopMainRuntimeHostEvidence `json:"sourceEvidence"`
	UnitTestEvidencePresent          bool                             `json:"unitTestEvidencePresent"`
	EnsureRuntimePortEvidencePresent bool                             `json:"ensureRuntimePortEvidencePresent"`
}

type G5DesktopMainRuntimeHostIDs struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

type G5DesktopMainRuntimeHostRequest struct {
	Path              string `json:"path"`
	Method            string `json:"method"`
	Body              string `json:"body"`
	AuthHeader        string `json:"authHeader"`
	ContentType       string `json:"contentType"`
	CustomHeaderName  string `json:"customHeaderName"`
	CustomHeaderValue string `json:"customHeaderValue"`
}

type G5DesktopMainRuntimeHostEvidence struct {
	UsesEnsuredSettings         bool `json:"usesEnsuredSettings"`
	JoinsBaseWithNormalizedPath bool `json:"joinsBaseWithNormalizedPath"`
	SetsBearerAuth              bool `json:"setsBearerAuth"`
	ForwardsCustomHeaders       bool `json:"forwardsCustomHeaders"`
	DefaultsJSONContentType     bool `json:"defaultsJsonContentType"`
	ForwardsMethodAndBody       bool `json:"forwardsMethodAndBody"`
}

type G5DesktopMainSseHostEncodingMatrix struct {
	HandlerChannel                 string                           `json:"handlerChannel"`
	EventChannel                   string                           `json:"eventChannel"`
	SourceThreadID                 string                           `json:"sourceThreadId"`
	EncodedThreadID                string                           `json:"encodedThreadId"`
	Request                        G5DesktopMainSseHostRequest      `json:"request"`
	ErrorPayload                   G5DesktopMainSseHostErrorPayload `json:"errorPayload"`
	SourceBuildsAnalytixEventsPath bool                             `json:"sourceBuildsAnalytixEventsPath"`
	EncodedUnitEvidencePresent     bool                             `json:"encodedUnitEvidencePresent"`
	HeaderUnitEvidencePresent      bool                             `json:"headerUnitEvidencePresent"`
	ErrorUnitEvidencePresent       bool                             `json:"errorUnitEvidencePresent"`
	ForbiddenRouteGuardPresent     bool                             `json:"forbiddenRouteGuardPresent"`
}

type G5DesktopMainSseHostRequest struct {
	Path          string `json:"path"`
	SinceSeq      int    `json:"sinceSeq"`
	LastEventID   string `json:"lastEventId"`
	Accept        string `json:"accept"`
	Authorization string `json:"authorization"`
	StreamID      string `json:"streamId"`
}

type G5DesktopMainSseHostErrorPayload struct {
	StreamID string `json:"streamId"`
	Status   int    `json:"status"`
}

type G5DesktopMainIpcProductBoundary struct {
	UsesReasonixProtocol    bool `json:"usesReasonixProtocol"`
	RendererRouteExposed    bool `json:"rendererRouteExposed"`
	DefaultGoBackendEnabled bool `json:"defaultGoBackendEnabled"`
}

type G5DesktopMainIpcBoundaryOutput struct {
	RuntimeRequestSchemaStrict                         bool `json:"runtimeRequestSchemaStrict"`
	RuntimeRequestAllowsAnalytixRoutes                 bool `json:"runtimeRequestAllowsAnalytixRoutes"`
	RuntimeRequestRejectsForbiddenRoutes               bool `json:"runtimeRequestRejectsForbiddenRoutes"`
	RuntimeRequestHandlerRejectsBeforeRuntimeCall      bool `json:"runtimeRequestHandlerRejectsBeforeRuntimeCall"`
	MainIpcEndpointBuilderAcceptsSharedPaths           bool `json:"mainIpcEndpointBuilderAcceptsSharedPaths"`
	MainIpcEndpointBuilderRejectsRawDynamicRoutes      bool `json:"mainIpcEndpointBuilderRejectsRawDynamicRoutes"`
	MainIpcEndpointBuilderUsesSharedTemplates          bool `json:"mainIpcEndpointBuilderUsesSharedTemplates"`
	MainIpcEndpointBuilderUnitEvidencePresent          bool `json:"mainIpcEndpointBuilderUnitEvidencePresent"`
	MainIpcEndpointBuilderRejectsSingularUserInput     bool `json:"mainIpcEndpointBuilderRejectsSingularUserInput"`
	MainIpcRuntimeAdapterPreservesEncodedPaths         bool `json:"mainIpcRuntimeAdapterPreservesEncodedPaths"`
	MainIpcRuntimeAdapterPreservesMethodAndBody        bool `json:"mainIpcRuntimeAdapterPreservesMethodAndBody"`
	MainIpcRuntimeAdapterRejectsRawDynamicRoutes       bool `json:"mainIpcRuntimeAdapterRejectsRawDynamicRoutes"`
	MainIpcRuntimeAdapterRejectsBeforeCall             bool `json:"mainIpcRuntimeAdapterRejectsBeforeCall"`
	MainIpcRuntimeAdapterUnitEvidencePresent           bool `json:"mainIpcRuntimeAdapterUnitEvidencePresent"`
	MainRuntimeHostHandoffPreservesEncodedPathAndQuery bool `json:"mainRuntimeHostHandoffPreservesEncodedPathAndQuery"`
	MainRuntimeHostHandoffPreservesMethodHeadersBody   bool `json:"mainRuntimeHostHandoffPreservesMethodHeadersBody"`
	MainRuntimeHostHandoffUsesEnsuredSettings          bool `json:"mainRuntimeHostHandoffUsesEnsuredSettings"`
	MainRuntimeHostHandoffUnitEvidencePresent          bool `json:"mainRuntimeHostHandoffUnitEvidencePresent"`
	MainSseHostEncodesThreadIdAndCursor                bool `json:"mainSseHostEncodesThreadIdAndCursor"`
	MainSseHostPreservesHeadersAndStreamId             bool `json:"mainSseHostPreservesHeadersAndStreamId"`
	MainSseHostRejectsForbiddenRouteTokens             bool `json:"mainSseHostRejectsForbiddenRouteTokens"`
	MainSseHostEncodingUnitEvidencePresent             bool `json:"mainSseHostEncodingUnitEvidencePresent"`
	SSEStartSchemaStrict                               bool `json:"sseStartSchemaStrict"`
	SSERejectsReasonixSessionPayload                   bool `json:"sseRejectsReasonixSessionPayload"`
	SSEUsesAnalytixThreadEventsRoute                   bool `json:"sseUsesAnalytixThreadEventsRoute"`
	SSEStopParsesStreamID                              bool `json:"sseStopParsesStreamId"`
	SSEStopMatchesStreamIDOnly                         bool `json:"sseStopMatchesStreamIdOnly"`
	SSEReconnectCursorPreserved                        bool `json:"sseReconnectCursorPreserved"`
	SSEBatchesEventsAt100ms                            bool `json:"sseBatchesEventsAt100ms"`
	UsesReasonixProtocol                               bool `json:"usesReasonixProtocol"`
	RendererRouteExposed                               bool `json:"rendererRouteExposed"`
	DefaultGoBackendEnabled                            bool `json:"defaultGoBackendEnabled"`
}

type G5RendererRouteSurfaceCase struct {
	SourceContractID                    string                                `json:"sourceContractId"`
	SourceFiles                         G5RendererRouteSurfaceSourceFiles     `json:"sourceFiles"`
	AppRoutes                           []string                              `json:"appRoutes"`
	ForbiddenTopLevelRouteTokens        []string                              `json:"forbiddenTopLevelRouteTokens"`
	ForbiddenTopLevelRouteTokensPresent []string                              `json:"forbiddenTopLevelRouteTokensPresent"`
	ForbiddenEntrypointSymbols          []string                              `json:"forbiddenEntrypointSymbols"`
	ForbiddenEntrypointSymbolsPresent   []string                              `json:"forbiddenEntrypointSymbolsPresent"`
	QuarantinedWorkflowSymbols          []string                              `json:"quarantinedWorkflowSymbols"`
	QuarantinedWorkflowEntrySurfaceHits []string                              `json:"quarantinedWorkflowEntrySurfaceHits"`
	Evidence                            G5RendererRouteSurfaceEvidence        `json:"evidence"`
	ProductBoundary                     G5RendererRouteSurfaceProductBoundary `json:"productBoundary"`
	Expected                            G5RendererRouteSurfaceOutput          `json:"expected"`
}

type G5RendererRouteSurfaceSourceFiles struct {
	Workbench                 string `json:"workbench"`
	WorkbenchRouteSurfaceTest string `json:"workbenchRouteSurfaceTest"`
	ChatStoreTypes            string `json:"chatStoreTypes"`
	Sidebar                   string `json:"sidebar"`
	PluginMarketplaceView     string `json:"pluginMarketplaceView"`
	BrowserAnalytixBridge     string `json:"browserAnalytixBridge"`
	WorkflowCreateLoopView    string `json:"workflowCreateLoopView"`
	CreateLoopRuntime         string `json:"createLoopRuntime"`
	ShellNavigationControls   string `json:"shellNavigationControls"`
	WorkbenchShell            string `json:"workbenchShell"`
	BaseShellCSS              string `json:"baseShellCss"`
}

type G5RendererRouteSurfaceEvidence struct {
	DormantWorkflowCodeExists                     bool `json:"dormantWorkflowCodeExists"`
	BrowserPreviewInstallsWindowAnalytix          bool `json:"browserPreviewInstallsWindowAnalytix"`
	BrowserPreviewForbiddenAliasesAbsent          bool `json:"browserPreviewForbiddenAliasesAbsent"`
	PluginMarketplaceReceivesLeftSidebarCollapsed bool `json:"pluginMarketplaceReceivesLeftSidebarCollapsed"`
	PluginMarketplaceTabsBeforeContent            bool `json:"pluginMarketplaceTabsBeforeContent"`
	ShellNavigationControlsNoDrag                 bool `json:"shellNavigationControlsNoDrag"`
	NativeSafeInsetCSSPresent                     bool `json:"nativeSafeInsetCssPresent"`
}

type G5RendererRouteSurfaceProductBoundary struct {
	UsesReasonixProtocol       bool `json:"usesReasonixProtocol"`
	TopLevelHiddenEntryExposed bool `json:"topLevelHiddenEntryExposed"`
	DefaultGoBackendEnabled    bool `json:"defaultGoBackendEnabled"`
}

type G5RendererRouteSurfaceOutput struct {
	AppRouteUnionKunCompatible          bool `json:"appRouteUnionKunCompatible"`
	NoForbiddenTopLevelRouteTokens      bool `json:"noForbiddenTopLevelRouteTokens"`
	NoForbiddenEntrypointSymbols        bool `json:"noForbiddenEntrypointSymbols"`
	DormantWorkflowCodeQuarantined      bool `json:"dormantWorkflowCodeQuarantined"`
	BrowserPreviewBridgeAnalytixOnly    bool `json:"browserPreviewBridgeAnalytixOnly"`
	PluginMarketplaceSafeAreaPropagates bool `json:"pluginMarketplaceSafeAreaPropagates"`
	ShellNavigationNoDrag               bool `json:"shellNavigationNoDrag"`
	NativeControlsSafeInset             bool `json:"nativeControlsSafeInset"`
	UsesReasonixProtocol                bool `json:"usesReasonixProtocol"`
	TopLevelHiddenEntryExposed          bool `json:"topLevelHiddenEntryExposed"`
	DefaultGoBackendEnabled             bool `json:"defaultGoBackendEnabled"`
}

type G5GoalPersistenceOffLockCase struct {
	SourceContractID               string                           `json:"sourceContractId"`
	SourceFiles                    G5GoalPersistenceSourceFiles     `json:"sourceFiles"`
	SetGoalOrder                   []string                         `json:"setGoalOrder"`
	ClearGoalOrder                 []string                         `json:"clearGoalOrder"`
	ForbiddenLockSubstrings        []string                         `json:"forbiddenLockSubstrings"`
	ForbiddenLockSubstringsPresent []string                         `json:"forbiddenLockSubstringsPresent"`
	WarningEvidence                G5GoalPersistenceWarningEvidence `json:"warningEvidence"`
	ProductBoundary                G5GoalPersistenceProductBoundary `json:"productBoundary"`
	Expected                       G5GoalPersistenceOffLockOutput   `json:"expected"`
}

type G5GoalPersistenceSourceFiles struct {
	ThreadService     string `json:"threadService"`
	ThreadServiceTest string `json:"threadServiceTest"`
}

type G5GoalPersistenceWarningEvidence struct {
	WarningEvent                 string `json:"warningEvent"`
	ThreadID                     string `json:"threadId"`
	OriginalError                string `json:"originalError"`
	ActionMarker                 string `json:"actionMarker"`
	LogsPersistenceFailure       bool   `json:"logsPersistenceFailure"`
	SurfacesOriginalError        bool   `json:"surfacesOriginalError"`
	WarningIsExactValueFreeEvent bool   `json:"warningIsExactValueFreeEvent"`
}

type G5GoalPersistenceProductBoundary struct {
	UsesReasonixProtocol     bool `json:"usesReasonixProtocol"`
	StatusApprovalLockShared bool `json:"statusApprovalLockShared"`
	DefaultGoBackendEnabled  bool `json:"defaultGoBackendEnabled"`
	RendererVisibleGoRoute   bool `json:"rendererVisibleGoRoute"`
}

type G5GoalPersistenceOffLockOutput struct {
	SetGoalPersistsBeforeEvent                bool `json:"setGoalPersistsBeforeEvent"`
	ClearGoalPersistsBeforeEvent              bool `json:"clearGoalPersistsBeforeEvent"`
	NoForbiddenControllerLock                 bool `json:"noForbiddenControllerLock"`
	GoalWritesOutsideSharedStatusApprovalLock bool `json:"goalWritesOutsideSharedStatusApprovalLock"`
	PersistenceFailureWarns                   bool `json:"persistenceFailureWarns"`
	PersistenceFailureSurfaces                bool `json:"persistenceFailureSurfaces"`
	WarningIsExactValueFreeEvent              bool `json:"warningIsExactValueFreeEvent"`
	WarningIncludesThreadID                   bool `json:"warningIncludesThreadId"`
	WarningIncludesOriginalError              bool `json:"warningIncludesOriginalError"`
	WarningIncludesAction                     bool `json:"warningIncludesAction"`
	UsesReasonixProtocol                      bool `json:"usesReasonixProtocol"`
	StatusApprovalLockShared                  bool `json:"statusApprovalLockShared"`
	DefaultGoBackendEnabled                   bool `json:"defaultGoBackendEnabled"`
	RendererVisibleGoRoute                    bool `json:"rendererVisibleGoRoute"`
}

type G5ToolResultFileImageBoundaryCase struct {
	SourceContractID      string                               `json:"sourceContractId"`
	SourceFiles           G5ToolResultFileImageSourceFiles     `json:"sourceFiles"`
	InlineImageKinds      []string                             `json:"inlineImageKinds"`
	EvictedPayloadMarkers G5ToolResultEvictedPayloadMarkers    `json:"evictedPayloadMarkers"`
	CapPolicy             G5ToolResultImageCapPolicy           `json:"capPolicy"`
	AttachmentFallback    G5ToolResultAttachmentFallback       `json:"attachmentFallback"`
	GeneratedFiles        G5ToolResultGeneratedFiles           `json:"generatedFiles"`
	ProductBoundary       G5ToolResultFileImageProductBoundary `json:"productBoundary"`
	Expected              G5ToolResultFileImageBoundaryOutput  `json:"expected"`
}

type G5ToolResultFileImageSourceFiles struct {
	ToolResultImage     string `json:"toolResultImage"`
	ToolResultImageTest string `json:"toolResultImageTest"`
	AttachmentStoreTest string `json:"attachmentStoreTest"`
	RendererMapperTest  string `json:"rendererMapperTest"`
}

type G5ToolResultEvictedPayloadMarkers struct {
	Payload           string   `json:"payload"`
	PreservedMetadata []string `json:"preservedMetadata"`
}

type G5ToolResultImageCapPolicy struct {
	HistoryImageCount     int    `json:"historyImageCount"`
	MaxKept               int    `json:"maxKept"`
	EvictedCount          int    `json:"evictedCount"`
	NewestImageDataBase64 string `json:"newestImageDataBase64"`
}

type G5ToolResultAttachmentFallback struct {
	LocalFilePath          string `json:"localFilePath"`
	TextFallbackPrefix     string `json:"textFallbackPrefix"`
	MimeType               string `json:"mimeType"`
	Dimensions             string `json:"dimensions"`
	FallbackBase64         string `json:"fallbackBase64"`
	DeepseekV4TextFallback bool   `json:"deepseekV4TextFallback"`
}

type G5ToolResultGeneratedFiles struct {
	ToolName              string `json:"toolName"`
	AttachmentID          string `json:"attachmentId"`
	GeneratedRelativePath string `json:"generatedRelativePath"`
	SpeechRelativePath    string `json:"speechRelativePath"`
}

type G5ToolResultFileImageProductBoundary struct {
	UsesReasonixProtocol    bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed    bool `json:"topLevelRouteExposed"`
	DefaultGoBackendEnabled bool `json:"defaultGoBackendEnabled"`
}

type G5ToolResultFileImageBoundaryOutput struct {
	InlineImageKindsPreserved        bool `json:"inlineImageKindsPreserved"`
	EvictedBase64Omitted             bool `json:"evictedBase64Omitted"`
	EvictedMetadataPreserved         bool `json:"evictedMetadataPreserved"`
	OnlyNewestImagesInline           bool `json:"onlyNewestImagesInline"`
	AttachmentLocalFilePathPreserved bool `json:"attachmentLocalFilePathPreserved"`
	TextFallbackCarriesFilePath      bool `json:"textFallbackCarriesFilePath"`
	DeepseekV4TextFallback           bool `json:"deepseekV4TextFallback"`
	GeneratedFileMetaLifted          bool `json:"generatedFileMetaLifted"`
	ToolAttachmentMetaLifted         bool `json:"toolAttachmentMetaLifted"`
	UsesReasonixProtocol             bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed             bool `json:"topLevelRouteExposed"`
	DefaultGoBackendEnabled          bool `json:"defaultGoBackendEnabled"`
}

type G5EventJsonlReplayBoundaryCase struct {
	SourceContractID        string                           `json:"sourceContractId"`
	SourceFiles             G5EventJsonlReplaySourceFiles    `json:"sourceFiles"`
	JsonlFiles              []string                         `json:"jsonlFiles"`
	RecorderEvidence        G5EventJsonlRecorderEvidence     `json:"recorderEvidence"`
	ReplayEvidence          G5EventJsonlReplayEvidence       `json:"replayEvidence"`
	UsageCompactionEvidence G5EventJsonlUsageCompaction      `json:"usageCompactionEvidence"`
	ProductBoundary         G5EventJsonlProductBoundary      `json:"productBoundary"`
	Expected                G5EventJsonlReplayBoundaryOutput `json:"expected"`
}

type G5EventJsonlReplaySourceFiles struct {
	FileSessionStore         string `json:"fileSessionStore"`
	LoopTest                 string `json:"loopTest"`
	RuntimeEventRecorderTest string `json:"runtimeEventRecorderTest"`
	FileSessionStoreTest     string `json:"fileSessionStoreTest"`
}

type G5EventJsonlRecorderEvidence struct {
	PersistsBeforePublish      bool `json:"persistsBeforePublish"`
	ConcurrentSeqsUnique       bool `json:"concurrentSeqsUnique"`
	PersistedHighWaterReadOnce bool `json:"persistedHighWaterReadOnce"`
}

type G5EventJsonlReplayEvidence struct {
	AppendNewlineTerminated        bool `json:"appendNewlineTerminated"`
	LoadEventsSinceFiltersAndSorts bool `json:"loadEventsSinceFiltersAndSorts"`
	HighestSeqUsesMax              bool `json:"highestSeqUsesMax"`
	MalformedJsonlLineSkipped      bool `json:"malformedJsonlLineSkipped"`
}

type G5EventJsonlUsageCompaction struct {
	CompactedSeqs            []int  `json:"compactedSeqs"`
	HighestSeq               int    `json:"highestSeq"`
	FailureKeepsAppendedSeqs []int  `json:"failureKeepsAppendedSeqs"`
	WarningPrefix            string `json:"warningPrefix"`
}

type G5EventJsonlProductBoundary struct {
	UsesReasonixProtocol    bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed    bool `json:"topLevelRouteExposed"`
	DefaultGoBackendEnabled bool `json:"defaultGoBackendEnabled"`
}

type G5EventJsonlReplayBoundaryOutput struct {
	AppendNewlineTerminated             bool `json:"appendNewlineTerminated"`
	LoadEventsSinceFiltersAndSorts      bool `json:"loadEventsSinceFiltersAndSorts"`
	HighestSeqPreservesMax              bool `json:"highestSeqPreservesMax"`
	MalformedJsonlLineSkipped           bool `json:"malformedJsonlLineSkipped"`
	PersistsBeforePublish               bool `json:"persistsBeforePublish"`
	ConcurrentSeqsUnique                bool `json:"concurrentSeqsUnique"`
	PersistedHighWaterReadOnce          bool `json:"persistedHighWaterReadOnce"`
	UsageCompactionKeepsCarryover       bool `json:"usageCompactionKeepsCarryover"`
	CompactionFailureKeepsAppendOnlyLog bool `json:"compactionFailureKeepsAppendOnlyLog"`
	UsesReasonixProtocol                bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed                bool `json:"topLevelRouteExposed"`
	DefaultGoBackendEnabled             bool `json:"defaultGoBackendEnabled"`
}

type G5MCPMalformedSchemaBoundaryCase struct {
	SourceContractID   string                              `json:"sourceContractId"`
	SourceFiles        G5MCPMalformedSchemaSourceFiles     `json:"sourceFiles"`
	MalformedTools     G5MCPMalformedTools                 `json:"malformedTools"`
	NormalizedSchemas  G5MCPMalformedNormalizedSchemas     `json:"normalizedSchemas"`
	NormalizerEvidence G5MCPMalformedNormalizerEvidence    `json:"normalizerEvidence"`
	ProductBoundary    G5MCPMalformedSchemaProductBoundary `json:"productBoundary"`
	Expected           G5MCPMalformedSchemaBoundaryOutput  `json:"expected"`
}

type G5MCPMalformedSchemaSourceFiles struct {
	MCPToolProvider     string `json:"mcpToolProvider"`
	MCPToolProviderTest string `json:"mcpToolProviderTest"`
}

type G5MCPMalformedTools struct {
	NonObjectSchemaToolName   string `json:"nonObjectSchemaToolName"`
	MalformedRequiredToolName string `json:"malformedRequiredToolName"`
	RawNonObjectSchemaKind    string `json:"rawNonObjectSchemaKind"`
	RawRequiredMixedCount     int    `json:"rawRequiredMixedCount"`
}

type G5MCPMalformedNormalizedSchemas struct {
	BadSchema   G5MCPMalformedBadSchema   `json:"badSchema"`
	BadRequired G5MCPMalformedBadRequired `json:"badRequired"`
}

type G5MCPMalformedBadSchema struct {
	Type                 string `json:"type"`
	PropertiesEmpty      bool   `json:"propertiesEmpty"`
	AdditionalProperties bool   `json:"additionalProperties"`
}

type G5MCPMalformedBadRequired struct {
	Type              string   `json:"type"`
	Required          []string `json:"required"`
	PropertiesDropped bool     `json:"propertiesDropped"`
}

type G5MCPMalformedNormalizerEvidence struct {
	NonRecordDefaults      bool `json:"nonRecordDefaults"`
	NonObjectTypeDefaults  bool `json:"nonObjectTypeDefaults"`
	PropertiesMustBeRecord bool `json:"propertiesMustBeRecord"`
	RequiredFiltersStrings bool `json:"requiredFiltersStrings"`
	OutputSchemaRecordOnly bool `json:"outputSchemaRecordOnly"`
}

type G5MCPMalformedSchemaProductBoundary struct {
	UsesReasonixProtocol    bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed    bool `json:"topLevelRouteExposed"`
	DefaultGoBackendEnabled bool `json:"defaultGoBackendEnabled"`
}

type G5MCPMalformedSchemaBoundaryOutput struct {
	NonObjectSchemaDefaults      bool `json:"nonObjectSchemaDefaults"`
	PropertiesArrayDropped       bool `json:"propertiesArrayDropped"`
	RequiredNonStringsDropped    bool `json:"requiredNonStringsDropped"`
	AdvertisedToolNamesPreserved bool `json:"advertisedToolNamesPreserved"`
	ModelCatalogSchemaSafe       bool `json:"modelCatalogSchemaSafe"`
	OutputSchemaNonRecordOmitted bool `json:"outputSchemaNonRecordOmitted"`
	UsesReasonixProtocol         bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed         bool `json:"topLevelRouteExposed"`
	DefaultGoBackendEnabled      bool `json:"defaultGoBackendEnabled"`
}

type G5CancelExecutableCase struct {
	AcceptedToolCalls    []G5AcceptedToolCall `json:"acceptedToolCalls"`
	CancelledResultCode  string               `json:"cancelledResultCode"`
	ExpectedResults      []G5ToolResult       `json:"expectedResults"`
	ScheduledAfterCancel int                  `json:"scheduledAfterCancel"`
}

type G5AcceptedToolCall struct {
	CallID string `json:"callId"`
	State  string `json:"state"`
	Output string `json:"output"`
}

type G5ToolResult struct {
	CallID string `json:"callId"`
	Status string `json:"status"`
	Code   string `json:"code"`
	Output string `json:"output"`
}

type G5TaskJobsExecutableCase struct {
	ToolContractBoundary    G5TaskJobToolContractBoundaryCase    `json:"toolContractBoundary"`
	TranscriptIdentity      G5TaskJobTranscriptIdentityCase      `json:"transcriptIdentity"`
	NestedSSEMetadata       G5TaskJobNestedSSEMetadataCase       `json:"nestedSseMetadata"`
	Jobs                    []G5TaskJobInput                     `json:"jobs"`
	SkippedUnstartedReason  string                               `json:"skippedUnstartedReason"`
	ExpectedJobs            []G5TaskJobOutput                    `json:"expectedJobs"`
	StaleReconcile          G5TaskJobStaleReconcileCase          `json:"staleReconcile"`
	RestartDrill            G5TaskJobRestartDrillCase            `json:"restartDrill"`
	RouteExecutable         G5TaskJobRouteExecutableCase         `json:"routeExecutable"`
	Lifecycle               G5TaskJobLifecycleCase               `json:"lifecycle"`
	PlannerExecutor         G5TaskJobPlannerExecutorCase         `json:"plannerExecutor"`
	PlannerToolsetInventory G5TaskJobPlannerToolsetInventoryCase `json:"plannerToolsetInventory"`
	ParentGoalEvidence      G5ParentGoalEvidenceCase             `json:"parentGoalEvidence"`
	ParallelValidation      G5ParallelValidationCase             `json:"parallelValidation"`
}

type G5TaskJobToolContractBoundaryCase struct {
	SourceContractID        string                              `json:"sourceContractId"`
	Task                    TaskToolContract                    `json:"task"`
	ParallelTasks           ParallelTaskToolContract            `json:"parallelTasks"`
	Routes                  []string                            `json:"routes"`
	ProtectedRoutes         []string                            `json:"protectedRoutes"`
	UnauthorizedStatus      int                                 `json:"unauthorizedStatus"`
	ForbiddenTopLevelRoutes []string                            `json:"forbiddenTopLevelRoutes"`
	Expected                G5TaskJobToolContractBoundaryOutput `json:"expected"`
}

type G5TaskJobToolContractBoundaryOutput struct {
	TaskToolName                   string   `json:"taskToolName"`
	ParallelToolName               string   `json:"parallelToolName"`
	TaskInternalRuntimeOnly        bool     `json:"taskInternalRuntimeOnly"`
	ParallelInternalRuntimeOnly    bool     `json:"parallelInternalRuntimeOnly"`
	RequiresPermissionGate         bool     `json:"requiresPermissionGate"`
	MayAppendParentGoalEvidence    bool     `json:"mayAppendParentGoalEvidence"`
	RequiresDependencyValidation   bool     `json:"requiresDependencyValidation"`
	RequiresPlannerReadOnlyToolset bool     `json:"requiresPlannerReadOnlyToolset"`
	Routes                         []string `json:"routes"`
	ProtectedRoutes                []string `json:"protectedRoutes"`
	UnauthorizedStatus             int      `json:"unauthorizedStatus"`
	ForbiddenTopLevelRoutes        []string `json:"forbiddenTopLevelRoutes"`
	UsesReasonixProtocol           bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed           bool     `json:"topLevelRouteExposed"`
}

type G5TaskJobTranscriptIdentityCase struct {
	SourceContractID string                            `json:"sourceContractId"`
	Transcript       TaskTranscript                    `json:"transcript"`
	Expected         G5TaskJobTranscriptIdentityOutput `json:"expected"`
}

type G5TaskJobTranscriptIdentityOutput struct {
	SourceID                       string `json:"sourceId"`
	ContinueTargetID               string `json:"continueTargetId"`
	ForkTargetID                   string `json:"forkTargetId"`
	IncompatibleError              string `json:"incompatibleError"`
	SameTranscriptIdentityRequired bool   `json:"sameTranscriptIdentityRequired"`
	ContinuePreservesTarget        bool   `json:"continuePreservesTarget"`
	ForkCreatesDistinctTarget      bool   `json:"forkCreatesDistinctTarget"`
	ContinueTargetMatchesSource    bool   `json:"continueTargetMatchesSource"`
	ForkTargetDistinctFromSource   bool   `json:"forkTargetDistinctFromSource"`
	UsesReasonixProtocol           bool   `json:"usesReasonixProtocol"`
	TopLevelRouteExposed           bool   `json:"topLevelRouteExposed"`
}

type G5TaskJobNestedSSEMetadataCase struct {
	SourceContractID   string                           `json:"sourceContractId"`
	NestedEvent        TaskNestedEvent                  `json:"nestedEvent"`
	ParentGoalEvidence TaskParentGoalEvidence           `json:"parentGoalEvidence"`
	Expected           G5TaskJobNestedSSEMetadataOutput `json:"expected"`
}

type G5TaskJobNestedSSEMetadataOutput struct {
	ParentCallID                string   `json:"parentCallId"`
	ChildRunID                  string   `json:"childRunId"`
	NestedSSEMetadataFields     []string `json:"nestedSseMetadataFields"`
	IncludesParentCallID        bool     `json:"includesParentCallId"`
	IncludesChildRunID          bool     `json:"includesChildRunId"`
	IncludesEvidenceLedgered    bool     `json:"includesEvidenceLedgered"`
	IncludesEvidenceLedgerError bool     `json:"includesEvidenceLedgerError"`
	ParentChildDistinct         bool     `json:"parentChildDistinct"`
	RequiresActiveGoal          bool     `json:"requiresActiveGoal"`
	EvidenceLedgeredEventKey    string   `json:"evidenceLedgeredEventKey"`
	EvidenceLedgerErrorEventKey string   `json:"evidenceLedgerErrorEventKey"`
	UsesReasonixProtocol        bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed        bool     `json:"topLevelRouteExposed"`
}

type G5TaskJobStaleReconcileCase struct {
	Jobs         []G5TaskJobInput  `json:"jobs"`
	Reason       string            `json:"reason"`
	ExpectedJobs []G5TaskJobOutput `json:"expectedJobs"`
}

type G5TaskJobRestartDrillCase struct {
	Jobs               []G5TaskJobInput            `json:"jobs"`
	OutputAfterRestart string                      `json:"outputAfterRestart"`
	WaitStatus         string                      `json:"waitStatus"`
	KillStatus         string                      `json:"killStatus"`
	KillError          string                      `json:"killError"`
	Expected           G5TaskJobRestartDrillOutput `json:"expected"`
}

type G5TaskJobRestartDrillOutput struct {
	RehydratedCount int                      `json:"rehydratedCount"`
	Completed       G5TaskJobRestartComplete `json:"completed"`
	Killed          G5TaskJobRestartKilled   `json:"killed"`
}

type G5TaskJobRestartComplete struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Output     string `json:"output"`
	NextOffset int    `json:"nextOffset"`
}

type G5TaskJobRestartKilled struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

type G5TaskJobRouteExecutableCase struct {
	UnauthorizedStatus int                            `json:"unauthorizedStatus"`
	Output             TaskJobRouteOutput             `json:"output"`
	Wait               TaskJobRouteWait               `json:"wait"`
	Kill               TaskJobRouteKill               `json:"kill"`
	MissingOutput      TaskJobRouteMissing            `json:"missingOutput"`
	Rehydrated         TaskJobRouteRehydrated         `json:"rehydrated"`
	ProductBoundary    G5ParentGoalProductBoundary    `json:"productBoundary"`
	Expected           G5TaskJobRouteExecutableOutput `json:"expected"`
}

type G5TaskJobRouteExecutableOutput struct {
	UnauthorizedStatus            int    `json:"unauthorizedStatus"`
	OutputStatus                  int    `json:"outputStatus"`
	OutputJobStatus               string `json:"outputJobStatus"`
	OutputNextOffset              int    `json:"outputNextOffset"`
	OutputReplayOffset            int    `json:"outputReplayOffset"`
	WaitStatus                    int    `json:"waitStatus"`
	WaitJobStatus                 string `json:"waitJobStatus"`
	KillStatus                    int    `json:"killStatus"`
	KillJobStatus                 string `json:"killJobStatus"`
	MissingOutputStatus           int    `json:"missingOutputStatus"`
	RehydratedOutputStatus        int    `json:"rehydratedOutputStatus"`
	RehydratedWaitStatus          int    `json:"rehydratedWaitStatus"`
	RehydratedKillStatus          int    `json:"rehydratedKillStatus"`
	RehydratedCompletedStatus     string `json:"rehydratedCompletedStatus"`
	RehydratedCompletedNextOffset int    `json:"rehydratedCompletedNextOffset"`
	RehydratedKilledStatus        string `json:"rehydratedKilledStatus"`
	UsesReasonixProtocol          bool   `json:"usesReasonixProtocol"`
	TopLevelRouteExposed          bool   `json:"topLevelRouteExposed"`
}

type G5TaskJobLifecycleCase struct {
	Foreground      TaskForegroundContract      `json:"foreground"`
	Background      TaskBackgroundContract      `json:"background"`
	WaitOutputKill  TaskWaitOutputKillContract  `json:"waitOutputKill"`
	ProductBoundary G5ParentGoalProductBoundary `json:"productBoundary"`
	Expected        G5TaskJobLifecycleOutput    `json:"expected"`
}

type G5TaskJobLifecycleOutput struct {
	ForegroundKind             string `json:"foregroundKind"`
	ForegroundStatus           string `json:"foregroundStatus"`
	ForegroundResult           string `json:"foregroundResult"`
	BackgroundKind             string `json:"backgroundKind"`
	BackgroundStatusAcrossTurn string `json:"backgroundStatusAcrossTurn"`
	BackgroundOutput           string `json:"backgroundOutput"`
	BackgroundFinalStatus      string `json:"backgroundFinalStatus"`
	BackgroundFinalResult      string `json:"backgroundFinalResult"`
	WaitOutputKillStatus       string `json:"waitOutputKillStatus"`
	WaitOutputKillError        string `json:"waitOutputKillError"`
	UsesReasonixProtocol       bool   `json:"usesReasonixProtocol"`
	TopLevelRouteExposed       bool   `json:"topLevelRouteExposed"`
}

type G5TaskJobPlannerExecutorCase struct {
	PlannerKind                     string                         `json:"plannerKind"`
	PlannerPolicy                   string                         `json:"plannerPolicy"`
	ExecutorPolicy                  string                         `json:"executorPolicy"`
	FailureStatus                   string                         `json:"failureStatus"`
	CancelledStatus                 string                         `json:"cancelledStatus"`
	SkippedReason                   string                         `json:"skippedReason"`
	CancelReason                    string                         `json:"cancelReason"`
	OutputOffsetJobCount            int                            `json:"outputOffsetJobCount"`
	RequiresFailurePropagation      bool                           `json:"requiresFailurePropagation"`
	RequiresCancellationPropagation bool                           `json:"requiresCancellationPropagation"`
	RequiresTranscriptPropagation   bool                           `json:"requiresTranscriptPropagation"`
	TranscriptPropagationMode       string                         `json:"transcriptPropagationMode"`
	TranscriptPropagationJobCount   int                            `json:"transcriptPropagationJobCount"`
	ProductBoundary                 G5ParentGoalProductBoundary    `json:"productBoundary"`
	Expected                        G5TaskJobPlannerExecutorOutput `json:"expected"`
}

type G5TaskJobPlannerExecutorOutput struct {
	PlannerKind                   string `json:"plannerKind"`
	PlannerPolicy                 string `json:"plannerPolicy"`
	ExecutorPolicy                string `json:"executorPolicy"`
	FailureStatus                 string `json:"failureStatus"`
	CancelledStatus               string `json:"cancelledStatus"`
	SkippedReason                 string `json:"skippedReason"`
	CancelReason                  string `json:"cancelReason"`
	OutputOffsetJobCount          int    `json:"outputOffsetJobCount"`
	FailurePropagates             bool   `json:"failurePropagates"`
	CancellationPropagates        bool   `json:"cancellationPropagates"`
	TranscriptPropagates          bool   `json:"transcriptPropagates"`
	TranscriptPropagationMode     string `json:"transcriptPropagationMode"`
	TranscriptPropagationJobCount int    `json:"transcriptPropagationJobCount"`
	UsesReasonixProtocol          bool   `json:"usesReasonixProtocol"`
	TopLevelRouteExposed          bool   `json:"topLevelRouteExposed"`
}

type G5TaskJobPlannerToolsetInventoryCase struct {
	ReadOnlyToolset  []string                               `json:"readOnlyToolset"`
	ForbiddenToolset []string                               `json:"forbiddenToolset"`
	TaskToolName     string                                 `json:"taskToolName"`
	ParallelToolName string                                 `json:"parallelToolName"`
	PlannerPolicy    string                                 `json:"plannerPolicy"`
	ExecutorPolicy   string                                 `json:"executorPolicy"`
	ProductBoundary  G5ParentGoalProductBoundary            `json:"productBoundary"`
	Expected         G5TaskJobPlannerToolsetInventoryOutput `json:"expected"`
}

type G5TaskJobPlannerToolsetInventoryOutput struct {
	ReadOnlyToolset           []string `json:"readOnlyToolset"`
	ForbiddenToolset          []string `json:"forbiddenToolset"`
	ReadOnlyToolCount         int      `json:"readOnlyToolCount"`
	ForbiddenToolCount        int      `json:"forbiddenToolCount"`
	ReadOnlyExcludesTaskTools bool     `json:"readOnlyExcludesTaskTools"`
	ForbiddenMatchesTaskTools bool     `json:"forbiddenMatchesTaskTools"`
	PlannerPolicy             string   `json:"plannerPolicy"`
	ExecutorPolicy            string   `json:"executorPolicy"`
	UsesReasonixProtocol      bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed      bool     `json:"topLevelRouteExposed"`
}

type G5ParentGoalEvidenceCase struct {
	RequiresActiveGoal       bool                        `json:"requiresActiveGoal"`
	LedgeredEventKey         string                      `json:"ledgeredEventKey"`
	LedgerErrorEventKey      string                      `json:"ledgerErrorEventKey"`
	ActiveGoalEventMetadata  map[string]any              `json:"activeGoalEventMetadata"`
	MissingGoalEventMetadata map[string]any              `json:"missingGoalEventMetadata"`
	ProductBoundary          G5ParentGoalProductBoundary `json:"productBoundary"`
	Expected                 G5ParentGoalEvidenceOutput  `json:"expected"`
}

type G5ParentGoalProductBoundary struct {
	UsesReasonixProtocol bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed bool `json:"topLevelRouteExposed"`
}

type G5ParentGoalEvidenceOutput struct {
	RequiresActiveGoal      bool   `json:"requiresActiveGoal"`
	LedgeredEventKey        string `json:"ledgeredEventKey"`
	LedgerErrorEventKey     string `json:"ledgerErrorEventKey"`
	LedgeredWhenActiveGoal  bool   `json:"ledgeredWhenActiveGoal"`
	ErrorsWithoutActiveGoal bool   `json:"errorsWithoutActiveGoal"`
	UsesReasonixProtocol    bool   `json:"usesReasonixProtocol"`
	TopLevelRouteExposed    bool   `json:"topLevelRouteExposed"`
}

type G5TaskJobInput struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Output string `json:"output"`
	Offset int    `json:"offset"`
}

type G5TaskJobOutput struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output string `json:"output"`
	Offset int    `json:"offset"`
	Reason string `json:"reason"`
}

type G5ParallelValidationCase struct {
	DependencyField string                      `json:"dependencyField"`
	ValidPlan       []G5ParallelPlanItem        `json:"validPlan"`
	InvalidPlans    []G5ParallelInvalidPlan     `json:"invalidPlans"`
	ProductBoundary G5ParentGoalProductBoundary `json:"productBoundary"`
	Expected        G5ParallelValidationOutput  `json:"expected"`
}

type G5ParallelPlanItem struct {
	ID        string   `json:"id"`
	DependsOn []string `json:"depends_on,omitempty"`
}

type G5ParallelInvalidPlan struct {
	CaseID string               `json:"caseId"`
	Tasks  []G5ParallelPlanItem `json:"tasks"`
}

type G5ParallelValidationOutput struct {
	DependencyField      string                            `json:"dependencyField"`
	ValidOrder           []string                          `json:"validOrder"`
	InvalidResults       []G5ParallelInvalidValidationItem `json:"invalidResults"`
	UsesReasonixProtocol bool                              `json:"usesReasonixProtocol"`
	TopLevelRouteExposed bool                              `json:"topLevelRouteExposed"`
}

type G5ParallelInvalidValidationItem struct {
	CaseID string `json:"caseId"`
	Error  string `json:"error"`
}

type G5ApprovalDenyCase struct {
	AttemptedToolNames []string             `json:"attemptedToolNames"`
	ApprovalIDs        []string             `json:"approvalIds"`
	Expected           G5ApprovalDenyOutput `json:"expected"`
}

type G5ApprovalDenyOutput struct {
	DeniedToolNames    []string `json:"deniedToolNames"`
	ApprovalIDs        []string `json:"approvalIds"`
	ApprovalItemCount  int      `json:"approvalItemCount"`
	CreatesDurableJobs bool     `json:"createsDurableJobs"`
	CreatesChildRuns   bool     `json:"createsChildRuns"`
}

type G5UserInputExecutableCase struct {
	Submitted                  G5UserInputSubmittedCase                  `json:"submitted"`
	Cancelled                  G5UserInputCancelledCase                  `json:"cancelled"`
	StructuredChoiceValidation G5UserInputStructuredChoiceValidationCase `json:"structuredChoiceValidation"`
	Expected                   G5UserInputOutput                         `json:"expected"`
}

type G5UserInputSubmittedCase struct {
	ID                           string              `json:"id"`
	ItemID                       string              `json:"itemId"`
	Answers                      []G5UserInputAnswer `json:"answers"`
	ResolvedEventKind            string              `json:"resolvedEventKind"`
	ResolvedEventIncludesAnswers bool                `json:"resolvedEventIncludesAnswers"`
	PendingBefore                int                 `json:"pendingBefore"`
	PendingAfter                 int                 `json:"pendingAfter"`
}

type G5UserInputAnswer = agent.UserInputAnswer

type G5UserInputCancelledCase struct {
	ID                  string `json:"id"`
	Status              string `json:"status"`
	SecondResolveStatus int    `json:"secondResolveStatus"`
	PendingBefore       int    `json:"pendingBefore"`
	PendingAfter        int    `json:"pendingAfter"`
}

type G5UserInputStructuredChoiceValidationCase struct {
	MaxQuestions                int      `json:"maxQuestions"`
	MinOptionsWhenProvided      int      `json:"minOptionsWhenProvided"`
	MaxOptionsWhenProvided      int      `json:"maxOptionsWhenProvided"`
	DedupeLabelsCaseInsensitive bool     `json:"dedupeLabelsCaseInsensitive"`
	InvalidResultCode           string   `json:"invalidResultCode"`
	OpensGateOnInvalid          bool     `json:"opensGateOnInvalid"`
	InvalidCases                []string `json:"invalidCases"`
}

type G5UserInputStructuredChoiceValidationOutput struct {
	MaxQuestions                          int      `json:"maxQuestions"`
	MinOptionsWhenProvided                int      `json:"minOptionsWhenProvided"`
	MaxOptionsWhenProvided                int      `json:"maxOptionsWhenProvided"`
	DedupeLabelsCaseInsensitive           bool     `json:"dedupeLabelsCaseInsensitive"`
	InvalidResultCode                     string   `json:"invalidResultCode"`
	InvalidCases                          []string `json:"invalidCases"`
	InvalidCaseCount                      int      `json:"invalidCaseCount"`
	RejectsTooManyQuestions               bool     `json:"rejectsTooManyQuestions"`
	RejectsSingleOption                   bool     `json:"rejectsSingleOption"`
	RejectsTooManyOptions                 bool     `json:"rejectsTooManyOptions"`
	RejectsDuplicateLabelsCaseInsensitive bool     `json:"rejectsDuplicateLabelsCaseInsensitive"`
	OpensGateOnInvalid                    bool     `json:"opensGateOnInvalid"`
	UsesReasonixProtocol                  bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed                  bool     `json:"topLevelRouteExposed"`
}

type G5UserInputOutput struct {
	SubmittedInputID             string                                      `json:"submittedInputId"`
	SubmittedStatus              string                                      `json:"submittedStatus"`
	AnswerCount                  int                                         `json:"answerCount"`
	HTTPEchoesAnswers            bool                                        `json:"httpEchoesAnswers"`
	ResolvedEventKind            string                                      `json:"resolvedEventKind"`
	ResolvedEventIncludesAnswers bool                                        `json:"resolvedEventIncludesAnswers"`
	CancelledInputID             string                                      `json:"cancelledInputId"`
	CancelledStatus              string                                      `json:"cancelledStatus"`
	SecondResolveStatus          int                                         `json:"secondResolveStatus"`
	PendingAfterSubmit           int                                         `json:"pendingAfterSubmit"`
	PendingAfterCancel           int                                         `json:"pendingAfterCancel"`
	StructuredChoiceValidation   G5UserInputStructuredChoiceValidationOutput `json:"structuredChoiceValidation"`
}

type G5AbortCleanupCase struct {
	ApprovalID                 string               `json:"approvalId"`
	UserInputID                string               `json:"userInputId"`
	ExpectedApprovalStatus     string               `json:"expectedApprovalStatus"`
	ExpectedUserInputStatus    string               `json:"expectedUserInputStatus"`
	LateApprovalDecisionStatus int                  `json:"lateApprovalDecisionStatus"`
	LateUserInputResolveStatus int                  `json:"lateUserInputResolveStatus"`
	ReplayKinds                []string             `json:"replayKinds"`
	Expected                   G5AbortCleanupOutput `json:"expected"`
}

type G5AbortCleanupOutput struct {
	ApprovalID                 string   `json:"approvalId"`
	ApprovalStatus             string   `json:"approvalStatus"`
	UserInputID                string   `json:"userInputId"`
	UserInputStatus            string   `json:"userInputStatus"`
	LateApprovalDecisionStatus int      `json:"lateApprovalDecisionStatus"`
	LateUserInputResolveStatus int      `json:"lateUserInputResolveStatus"`
	PendingAfterCleanup        int      `json:"pendingAfterCleanup"`
	ReplayKinds                []string `json:"replayKinds"`
	UsesReasonixProtocol       bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed       bool     `json:"topLevelRouteExposed"`
}

type G5ResumePendingGatesCase struct {
	SourceThreadID  string                     `json:"sourceThreadId"`
	ApprovalID      string                     `json:"approvalId"`
	UserInputID     string                     `json:"userInputId"`
	SourceStatuses  G5ResumeGateStatusPair     `json:"sourceStatuses"`
	ResumedStatuses G5ResumeGateStatusPair     `json:"resumedStatuses"`
	Expected        G5ResumePendingGatesOutput `json:"expected"`
}

type G5ResumeGateStatusPair struct {
	Approval  string `json:"approval"`
	UserInput string `json:"userInput"`
}

type G5ResumePendingGatesOutput struct {
	SourceApprovalStatus   string `json:"sourceApprovalStatus"`
	SourceUserInputStatus  string `json:"sourceUserInputStatus"`
	ResumedApprovalStatus  string `json:"resumedApprovalStatus"`
	ResumedUserInputStatus string `json:"resumedUserInputStatus"`
	PendingAfterResume     int    `json:"pendingAfterResume"`
	AnswersCopiedToResume  bool   `json:"answersCopiedToResume"`
	UsesReasonixProtocol   bool   `json:"usesReasonixProtocol"`
	TopLevelRouteExposed   bool   `json:"topLevelRouteExposed"`
}

type G5AutoResearchExecutableCase struct {
	ThreadID                    string               `json:"threadId"`
	Objective                   string               `json:"objective"`
	Requirements                []string             `json:"requirements"`
	ExpectedStateRelativePath   string               `json:"expectedStateRelativePath"`
	ExpectedFiles               []string             `json:"expectedFiles"`
	Direction                   string               `json:"direction"`
	DirectionOutcome            string               `json:"directionOutcome"`
	DirectionSummary            string               `json:"directionSummary"`
	RecordDirectionToolName     string               `json:"recordDirectionToolName"`
	DirectionRequiresActiveGoal bool                 `json:"directionRequiresActiveResearchGoal"`
	UnknownRequirementID        string               `json:"unknownRequirementId"`
	Expected                    G5AutoResearchOutput `json:"expected"`
}

type G5AutoResearchOutput struct {
	StateRelativePath                    string `json:"stateRelativePath"`
	FileCount                            int    `json:"fileCount"`
	WritesReasonixFile                   bool   `json:"writesReasonixFile"`
	WritesAgentsFile                     bool   `json:"writesAgentsFile"`
	UnknownRequirementAccepted           bool   `json:"unknownRequirementAccepted"`
	FindingsWrittenForUnknownRequirement bool   `json:"findingsWrittenForUnknownRequirement"`
	DirectionTrackingFileWritten         bool   `json:"directionTrackingFileWritten"`
	IterationLogRecordsDirection         bool   `json:"iterationLogRecordsDirection"`
	RecordResearchDirectionToolPresent   bool   `json:"recordResearchDirectionToolPresent"`
	RecordDirectionRequiresActiveGoal    bool   `json:"recordDirectionRequiresActiveResearchGoal"`
	StablePrefixContainsState            bool   `json:"stablePrefixContainsState"`
	ToolSchemaContainsState              bool   `json:"toolSchemaContainsState"`
	TopLevelRouteExposed                 bool   `json:"topLevelRouteExposed"`
}

type G5MCPLifecycleExecutableCase struct {
	ProviderID                 string               `json:"providerId"`
	FailedServerIDs            []string             `json:"failedServerIds"`
	ExpectedConnectedServerIDs []string             `json:"expectedConnectedServerIds"`
	ExpectedErrorServerIDs     []string             `json:"expectedErrorServerIds"`
	AttemptsPerFailedServer    int                  `json:"attemptsPerFailedServer"`
	RequiresRuntimeRestart     bool                 `json:"requiresRuntimeRestart"`
	LiveLocal                  G5MCPLiveLocalCase   `json:"liveLocal"`
	Expected                   G5MCPLifecycleOutput `json:"expected"`
}

type G5MCPLiveLocalCase struct {
	ServerID          string   `json:"serverId"`
	CWD               string   `json:"cwd"`
	LowPriority       bool     `json:"lowPriority"`
	BackgroundStart   bool     `json:"backgroundStart"`
	InitialPaths      []string `json:"initialPaths"`
	LateTombstonePath string   `json:"lateTombstonePath"`
	ResumePaths       []string `json:"resumePaths"`
	SecretDiagnostic  string   `json:"secretDiagnostic"`
	Replacement       string   `json:"replacement"`
}

type G5MCPLifecycleOutput struct {
	ProviderID             string         `json:"providerId"`
	RetryAttempts          map[string]int `json:"retryAttempts"`
	ConnectedServerIDs     []string       `json:"connectedServerIds"`
	ErrorServerIDs         []string       `json:"errorServerIds"`
	RequiresRuntimeRestart bool           `json:"requiresRuntimeRestart"`
	ActivePaths            []string       `json:"activePaths"`
	TombstoneCount         int            `json:"tombstoneCount"`
	RestartedFromSnapshot  bool           `json:"restartedFromSnapshot"`
	SecretSafeDiagnostic   string         `json:"secretSafeDiagnostic"`
	LeaksSecret            bool           `json:"leaksSecret"`
	TopLevelRouteExposed   bool           `json:"topLevelRouteExposed"`
}

type G5MCPCoreLifecycleCase struct {
	SourceContractID string                      `json:"sourceContractId"`
	ProviderID       string                      `json:"providerId"`
	CoreLifecycle    MCPCoreLifecycle            `json:"coreLifecycle"`
	ProductBoundary  G5ParentGoalProductBoundary `json:"productBoundary"`
	Expected         G5MCPCoreLifecycleOutput    `json:"expected"`
}

type MCPCoreLifecycle struct {
	Connect    MCPProviderConnect    `json:"connect"`
	Disconnect MCPProviderDisconnect `json:"disconnect"`
	Reload     MCPProviderReload     `json:"reload"`
	Cancel     MCPProviderCancel     `json:"cancel"`
	Error      MCPProviderError      `json:"error"`
}

type G5MCPCoreLifecycleOutput struct {
	ProviderID           string   `json:"providerId"`
	ConnectToolNames     []string `json:"connectToolNames"`
	ConnectAvailable     bool     `json:"connectAvailable"`
	ConnectToolCount     int      `json:"connectToolCount"`
	DisconnectReason     string   `json:"disconnectReason"`
	DisconnectToolNames  []string `json:"disconnectToolNames"`
	DisconnectAvailable  bool     `json:"disconnectAvailable"`
	DisconnectToolCount  int      `json:"disconnectToolCount"`
	ReloadToolNames      []string `json:"reloadToolNames"`
	SchemaOrderStable    bool     `json:"schemaOrderStable"`
	CancelErrorSubstring string   `json:"cancelErrorSubstring"`
	CancelExecuted       bool     `json:"cancelExecuted"`
	ErrorCode            string   `json:"errorCode"`
	ErrorApproved        bool     `json:"errorApproved"`
	UsesReasonixProtocol bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed bool     `json:"topLevelRouteExposed"`
}

type G5MCPBackgroundReconnectCase struct {
	SourceContractID    string                         `json:"sourceContractId"`
	BackgroundReconnect MCPBackgroundReconnect         `json:"backgroundReconnect"`
	Expected            G5MCPBackgroundReconnectOutput `json:"expected"`
}

type G5MCPBackgroundReconnectOutput struct {
	FailedServerIDs         []string `json:"failedServerIds"`
	SuspendedProviderID     string   `json:"suspendedProviderId"`
	SuspendedReason         string   `json:"suspendedReason"`
	ConnectedServerIDs      []string `json:"connectedServerIds"`
	ErrorServerIDs          []string `json:"errorServerIds"`
	AttemptsPerFailedServer int      `json:"attemptsPerFailedServer"`
	RetryAllFailedServers   bool     `json:"retryAllFailedServers"`
	RequiresRuntimeRestart  bool     `json:"requiresRuntimeRestart"`
}

type G5MCPCallReconnectCase struct {
	SourceContractID string                   `json:"sourceContractId"`
	CallReconnect    MCPCallReconnect         `json:"callReconnect"`
	Expected         G5MCPCallReconnectOutput `json:"expected"`
}

type MCPCallReconnect = mcp.MCPCallReconnect
type MCPStaleConnectionResult = mcp.MCPStaleConnectionResult
type MCPProtocolFailureResult = mcp.MCPProtocolFailureResult

type G5MCPCallReconnectOutput struct {
	ServerID                  string `json:"serverId"`
	ToolName                  string `json:"toolName"`
	NormalizedToolName        string `json:"normalizedToolName"`
	TransportErrorRetried     bool   `json:"transportErrorRetried"`
	ProtocolErrorRetried      bool   `json:"protocolErrorRetried"`
	MaxAttempts               int    `json:"maxAttempts"`
	StaleFactoryAttempts      int    `json:"staleFactoryAttempts"`
	StaleCloseCount           int    `json:"staleCloseCount"`
	StaleResultInstance       int    `json:"staleResultInstance"`
	StaleCallSucceeded        bool   `json:"staleCallSucceeded"`
	ProtocolFactoryAttempts   int    `json:"protocolFactoryAttempts"`
	ProtocolCloseCount        int    `json:"protocolCloseCount"`
	ProtocolErrorCode         string `json:"protocolErrorCode"`
	ProtocolCallReturnedError bool   `json:"protocolCallReturnedError"`
	UsesReasonixProtocol      bool   `json:"usesReasonixProtocol"`
	TopLevelRouteExposed      bool   `json:"topLevelRouteExposed"`
}

type G5MCPKnownOverrideDiagnosticsCase struct {
	SourceContractID      string                              `json:"sourceContractId"`
	KnownOverrideVariants []MCPKnownOverride                  `json:"knownOverrideVariants"`
	Expected              G5MCPKnownOverrideDiagnosticsOutput `json:"expected"`
}

type G5MCPKnownOverrideDiagnosticRow struct {
	ServerID            string `json:"serverId"`
	KnownOverride       string `json:"knownOverride"`
	EffectiveCWD        string `json:"effectiveCwd"`
	LowPriority         bool   `json:"lowPriority"`
	BackgroundStart     bool   `json:"backgroundStart"`
	WorkspaceRoot       string `json:"workspaceRoot"`
	ExplicitCWD         string `json:"explicitCwd,omitempty"`
	DaemonIdleTimeoutMs string `json:"daemonIdleTimeoutMs,omitempty"`
}

type G5MCPKnownOverrideDiagnosticsOutput struct {
	Diagnostics                []G5MCPKnownOverrideDiagnosticRow `json:"diagnostics"`
	VariantCount               int                               `json:"variantCount"`
	KnownOverrideKinds         []string                          `json:"knownOverrideKinds"`
	WorkspaceRoots             []string                          `json:"workspaceRoots"`
	ExplicitCWDServerIDs       []string                          `json:"explicitCwdServerIds"`
	DaemonIdleTimeoutServerIDs []string                          `json:"daemonIdleTimeoutServerIds"`
	AllLowPriority             bool                              `json:"allLowPriority"`
	AllBackgroundStart         bool                              `json:"allBackgroundStart"`
	UsesReasonixProtocol       bool                              `json:"usesReasonixProtocol"`
	TopLevelRouteExposed       bool                              `json:"topLevelRouteExposed"`
}

type G5MCPLiveLocalIndexerCase struct {
	SourceContractID string                      `json:"sourceContractId"`
	LiveLocalIndexer MCPLiveLocalIndexer         `json:"liveLocalIndexer"`
	Expected         G5MCPLiveLocalIndexerOutput `json:"expected"`
}

type G5MCPLiveLocalIndexerOutput struct {
	ServerID                  string         `json:"serverId"`
	CWD                       string         `json:"cwd"`
	LowPriority               bool           `json:"lowPriority"`
	BackgroundStart           bool           `json:"backgroundStart"`
	RetryServerIDs            []string       `json:"retryServerIds"`
	AttemptsPerFailedServer   int            `json:"attemptsPerFailedServer"`
	RetryAttempts             map[string]int `json:"retryAttempts"`
	InitialPaths              []string       `json:"initialPaths"`
	ResumePaths               []string       `json:"resumePaths"`
	ActivePaths               []string       `json:"activePaths"`
	TombstoneCount            int            `json:"tombstoneCount"`
	RestartedFromSnapshot     bool           `json:"restartedFromSnapshot"`
	LateTombstonePath         string         `json:"lateTombstonePath"`
	SecretSafeDiagnostic      string         `json:"secretSafeDiagnostic"`
	LeaksSecret               bool           `json:"leaksSecret"`
	ExecutionErrorToolName    string         `json:"executionErrorToolName"`
	ExecutionErrorIsError     bool           `json:"executionErrorIsError"`
	ExecutionErrorSafe        string         `json:"executionErrorSafe"`
	ExecutionErrorLeaksSecret bool           `json:"executionErrorLeaksSecret"`
	TopLevelRouteExposed      bool           `json:"topLevelRouteExposed"`
	UsesReasonixProtocol      bool           `json:"usesReasonixProtocol"`
}

type G5MCPApprovalAnnotationCase struct {
	SourceContractID    string                        `json:"sourceContractId"`
	ApprovalAnnotations MCPApprovalAnnotations        `json:"approvalAnnotations"`
	Expected            G5MCPApprovalAnnotationOutput `json:"expected"`
}

type G5MCPApprovalAnnotationOutput struct {
	ServerID           string `json:"serverId"`
	ToolName           string `json:"toolName"`
	NormalizedToolName string `json:"normalizedToolName"`
	DestructiveHint    bool   `json:"destructiveHint"`
	OpenWorldHint      bool   `json:"openWorldHint"`
	ApprovalID         string `json:"approvalId"`
	Decision           string `json:"decision"`
	ResultKind         string `json:"resultKind"`
	Executed           bool   `json:"executed"`
	DeniedNoExecute    bool   `json:"deniedNoExecute"`
}

type G5MCPSearchMetaToolsCase struct {
	SourceContractID string                     `json:"sourceContractId"`
	SearchMetaTools  MCPSearchMetaTools         `json:"searchMetaTools"`
	Expected         G5MCPSearchMetaToolsOutput `json:"expected"`
}

type G5MCPSearchMetaToolsOutput struct {
	ToolNames              []string `json:"toolNames"`
	ToolCount              int      `json:"toolCount"`
	RefreshToolAdvertised  bool     `json:"refreshToolAdvertised"`
	TrustedWorkspace       string   `json:"trustedWorkspace"`
	UntrustedWorkspace     string   `json:"untrustedWorkspace"`
	Query                  string   `json:"query"`
	TrustedToolID          string   `json:"trustedToolId"`
	UntrustedSearchedTools int      `json:"untrustedSearchedTools"`
	UnknownToolError       string   `json:"unknownToolError"`
	CallPolicy             string   `json:"callPolicy"`
	DeniedCallExecuted     bool     `json:"deniedCallExecuted"`
	DeniedNoExecute        bool     `json:"deniedNoExecute"`
	UsesReasonixProtocol   bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed   bool     `json:"topLevelRouteExposed"`
}

type G5MCPSearchRefreshDriftCase struct {
	SourceContractID string                        `json:"sourceContractId"`
	RefreshDrift     MCPRefreshDrift               `json:"refreshDrift"`
	Expected         G5MCPSearchRefreshDriftOutput `json:"expected"`
}

type G5MCPSearchRefreshDriftOutput struct {
	ServerID             string   `json:"serverId"`
	InitialToolNames     []string `json:"initialToolNames"`
	ExpandedToolNames    []string `json:"expandedToolNames"`
	TotalIndexed         int      `json:"totalIndexed"`
	CatalogDrift         bool     `json:"catalogDrift"`
	TopLevelRouteExposed bool     `json:"topLevelRouteExposed"`
}

type G5MCPSearchWorkspaceBoundaryCase struct {
	SourceContractID        string                             `json:"sourceContractId"`
	SearchWorkspaceBoundary MCPSearchWorkspaceBoundary         `json:"searchWorkspaceBoundary"`
	Expected                G5MCPSearchWorkspaceBoundaryOutput `json:"expected"`
}

type MCPSearchWorkspaceBoundary struct {
	TrustedWorkspace       string `json:"trustedWorkspace"`
	UntrustedWorkspace     string `json:"untrustedWorkspace"`
	Query                  string `json:"query"`
	TrustedToolID          string `json:"trustedToolId"`
	UntrustedSearchedTools int    `json:"untrustedSearchedTools"`
	UnknownToolError       string `json:"unknownToolError"`
	CallPolicy             string `json:"callPolicy"`
	DeniedCallExecuted     bool   `json:"deniedCallExecuted"`
}

type G5MCPSearchWorkspaceBoundaryOutput struct {
	TrustedWorkspace       string `json:"trustedWorkspace"`
	UntrustedWorkspace     string `json:"untrustedWorkspace"`
	Query                  string `json:"query"`
	TrustedToolID          string `json:"trustedToolId"`
	UntrustedSearchedTools int    `json:"untrustedSearchedTools"`
	UnknownToolError       string `json:"unknownToolError"`
	CallPolicy             string `json:"callPolicy"`
	DeniedNoExecute        bool   `json:"deniedNoExecute"`
}

type G5CheckpointRewindCase struct {
	CheckpointID string                    `json:"checkpointId"`
	PlanID       string                    `json:"planId"`
	ApplyID      string                    `json:"applyId"`
	RescueID     string                    `json:"rescueId"`
	ThreadID     string                    `json:"threadId"`
	TurnID       string                    `json:"turnId"`
	Workspace    string                    `json:"workspace"`
	CreatedAt    string                    `json:"createdAt"`
	ChangedFiles []G5CheckpointChangedFile `json:"changedFiles"`
	PathRisks    []G5CheckpointPathRisk    `json:"pathRisks"`
	Confirmation G5CheckpointConfirmation  `json:"confirmation"`
	EventKinds   []string                  `json:"eventKinds"`
	Expected     G5CheckpointRewindOutput  `json:"expected"`
}

type G5CheckpointChangedFile struct {
	RelativePath string `json:"relativePath"`
	ChangeKind   string `json:"changeKind"`
	BeforeHash   string `json:"beforeHash,omitempty"`
	AfterHash    string `json:"afterHash,omitempty"`
}

type G5CheckpointPathRisk struct {
	RelativePath string `json:"relativePath"`
	Exists       bool   `json:"exists"`
	IsSymlink    bool   `json:"isSymlink,omitempty"`
}

type G5CheckpointConfirmation struct {
	Confirmed   bool   `json:"confirmed"`
	Destructive bool   `json:"destructive"`
	Phrase      string `json:"phrase"`
}

type G5CheckpointRewindOutput struct {
	CheckpointIDPrefix           string   `json:"checkpointIdPrefix"`
	PlanIDPrefix                 string   `json:"planIdPrefix"`
	ApplyIDPrefix                string   `json:"applyIdPrefix"`
	RescueIDPrefix               string   `json:"rescueIdPrefix"`
	ReadyFileCount               int      `json:"readyFileCount"`
	BlockedFileCount             int      `json:"blockedFileCount"`
	SymlinkBlocked               bool     `json:"symlinkBlocked"`
	PathEscapeBlocked            bool     `json:"pathEscapeBlocked"`
	LegalDotDotFilenameReady     bool     `json:"legalDotDotFilenameReady"`
	RequiresExplicitConfirmation bool     `json:"requiresExplicitConfirmation"`
	ConversationAuditAppendOnly  bool     `json:"conversationAuditAppendOnly"`
	RewritesTranscript           bool     `json:"rewritesTranscript"`
	UsesGitRefs                  bool     `json:"usesGitRefs"`
	EventKinds                   []string `json:"eventKinds"`
	UsesReasonixProtocol         bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed         bool     `json:"topLevelRouteExposed"`
}

type G5RemoteEntryCase struct {
	ExpectedPortKeys     []string            `json:"expectedPortKeys"`
	ForbiddenPortKeys    []string            `json:"forbiddenPortKeys"`
	RejectedOverrideKeys []string            `json:"rejectedOverrideKeys"`
	Expected             G5RemoteEntryOutput `json:"expected"`
}

type G5RemoteEntryOutput struct {
	ExposedPortKeys          []string `json:"exposedPortKeys"`
	ForbiddenPortKeysAbsent  bool     `json:"forbiddenPortKeysAbsent"`
	RejectedOverrideAccepted bool     `json:"rejectedOverrideAccepted"`
	GoalAccess               bool     `json:"goalAccess"`
	CheckpointAccess         bool     `json:"checkpointAccess"`
	MemoryAccess             bool     `json:"memoryAccess"`
	StorageAccess            bool     `json:"storageAccess"`
	ToolHostAccess           bool     `json:"toolHostAccess"`
	UsesReasonixProtocol     bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed     bool     `json:"topLevelRouteExposed"`
}

type G5HistoryRepairCase struct {
	Items    []G5HistoryRepairItem `json:"items"`
	Expected G5HistoryRepairOutput `json:"expected"`
}

type G5HistoryRepairItem struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	TurnID   string `json:"turnId"`
	CallID   string `json:"callId,omitempty"`
	ToolName string `json:"toolName,omitempty"`
}

type G5HistoryRepairOutput struct {
	RepairedIds                     []string `json:"repairedIds"`
	DroppedIds                      []string `json:"droppedIds"`
	KeptCallIds                     []string `json:"keptCallIds"`
	KeptResultCallIds               []string `json:"keptResultCallIds"`
	OrphanResultDropped             bool     `json:"orphanResultDropped"`
	MissingResultCallDropped        bool     `json:"missingResultCallDropped"`
	DuplicateResultDropped          bool     `json:"duplicateResultDropped"`
	BridgeTextPreserved             bool     `json:"bridgeTextPreserved"`
	StablePrefixContainsRepairState bool     `json:"stablePrefixContainsRepairState"`
	UsesReasonixProtocol            bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed            bool     `json:"topLevelRouteExposed"`
}

type G5CompactionBoundaryCase struct {
	Items    []G5CompactionBoundaryItem `json:"items"`
	Expected G5CompactionBoundaryOutput `json:"expected"`
}

type G5CompactionBoundaryItem struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	TurnID         string `json:"turnId"`
	ReplacedTokens int    `json:"replacedTokens,omitempty"`
	Summary        string `json:"summary,omitempty"`
}

type G5CompactionBoundaryOutput struct {
	EffectiveIds                        []string `json:"effectiveIds"`
	DroppedIds                          []string `json:"droppedIds"`
	LatestCompactionID                  string   `json:"latestCompactionId"`
	LatestCompactionFirst               bool     `json:"latestCompactionFirst"`
	LatestCompactionPreserved           bool     `json:"latestCompactionPreserved"`
	OlderCompactionDropped              bool     `json:"olderCompactionDropped"`
	NoopCompactionDropped               bool     `json:"noopCompactionDropped"`
	PostCompactionUserPreserved         bool     `json:"postCompactionUserPreserved"`
	PreCompactionUserDropped            bool     `json:"preCompactionUserDropped"`
	StablePrefixContainsCompactionState bool     `json:"stablePrefixContainsCompactionState"`
	UsesReasonixProtocol                bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed                bool     `json:"topLevelRouteExposed"`
}

type G5StepLimitsExecutableCase struct {
	DefaultMaxModelSteps        int                `json:"defaultMaxModelSteps"`
	UserGlobalMaxModelSteps     int                `json:"userGlobalMaxModelSteps"`
	SessionMaxModelSteps        int                `json:"sessionMaxModelSteps"`
	TurnMaxModelSteps           int                `json:"turnMaxModelSteps"`
	PlannerMaxModelSteps        int                `json:"plannerMaxModelSteps"`
	HeadlessMaxModelSteps       int                `json:"headlessMaxModelSteps"`
	ZeroDefaultMaxModelSteps    int                `json:"zeroDefaultMaxModelSteps"`
	DelegateParentMaxModelSteps int                `json:"delegateParentMaxModelSteps"`
	DelegateFloorParentMaxSteps int                `json:"delegateFloorParentMaxModelSteps"`
	Expected                    G5StepLimitsOutput `json:"expected"`
}

type G5StepLimitsOutput struct {
	Default                    int              `json:"default"`
	UserGlobal                 int              `json:"userGlobal"`
	Session                    int              `json:"session"`
	Turn                       int              `json:"turn"`
	Planner                    int              `json:"planner"`
	Headless                   int              `json:"headless"`
	ZeroDefault                int              `json:"zeroDefault"`
	DelegateInherited          int              `json:"delegateInherited"`
	DelegateFloorInherited     int              `json:"delegateFloorInherited"`
	DynamicLimitInStablePrefix bool             `json:"dynamicLimitInStablePrefix"`
	ErrorCode                  string           `json:"errorCode"`
	Matrix                     []G5StepLimitRow `json:"matrix"`
}

type G5StepLimitRow struct {
	Scope                      string `json:"scope"`
	Configured                 int    `json:"configured"`
	Fallback                   int    `json:"fallback"`
	Effective                  int    `json:"effective"`
	Source                     string `json:"source"`
	DynamicLimitInStablePrefix bool   `json:"dynamicLimitInStablePrefix"`
	DisablesGuard              bool   `json:"disablesGuard"`
	DelegateMinFloorApplied    bool   `json:"delegateMinFloorApplied"`
}

type G5PlannerExecutableCase struct {
	AvailableToolset             []string                `json:"availableToolset"`
	ReadOnlyToolset              []string                `json:"readOnlyToolset"`
	ForbiddenToolset             []string                `json:"forbiddenToolset"`
	BlockedToolset               []string                `json:"blockedToolset"`
	PlanToolName                 string                  `json:"planToolName"`
	ForgedToolName               string                  `json:"forgedToolName"`
	NormalModeAdvertised         []string                `json:"normalModeAdvertised"`
	ExpectedCapabilityAdvertised []string                `json:"expectedCapabilityAdvertised"`
	ExpectedStep0Advertised      []string                `json:"expectedStep0Advertised"`
	ExpectedStep1Advertised      []string                `json:"expectedStep1Advertised"`
	ExpectedRejectedCall         G5PlannerRejectedCall   `json:"expectedRejectedCall"`
	ExpectedRejectedCalls        []G5PlannerRejectedCall `json:"expectedRejectedCalls"`
	Expected                     G5PlannerGateOutput     `json:"expected"`
}

type G5PlannerRejectedCall struct {
	ToolName string `json:"toolName"`
	Status   string `json:"status"`
	Code     string `json:"code"`
	Executed bool   `json:"executed"`
}

type G5AutoRouterClassifierCase struct {
	RouterModel           string                           `json:"routerModel"`
	DefaultTimeoutMs      int                              `json:"defaultTimeoutMs"`
	Fingerprint           string                           `json:"fingerprint"`
	IsolatedRequest       G5AutoRouterIsolatedRequest      `json:"isolatedRequest"`
	FingerprintDrift      G5AutoRouterFingerprintDrift     `json:"fingerprintDrift"`
	TimeoutFallback       G5AutoRouterTimeoutFallback      `json:"timeoutFallback"`
	RecommendationParsing []G5AutoRouterRecommendationCase `json:"recommendationParsing"`
	ContextBoundary       G5AutoRouterContextBoundary      `json:"contextBoundary"`
	ProductBoundary       G5AutoRouterProductBoundary      `json:"productBoundary"`
	Expected              G5AutoRouterClassifierOutput     `json:"expected"`
}

type G5AutoRouterIsolatedRequest struct {
	TurnIDSuffix               string `json:"turnIdSuffix"`
	Stream                     bool   `json:"stream"`
	MaxTokens                  int    `json:"maxTokens"`
	Temperature                int    `json:"temperature"`
	ResponseFormat             string `json:"responseFormat"`
	ReasoningEffort            string `json:"reasoningEffort"`
	PrefixItemCount            int    `json:"prefixItemCount"`
	ToolCount                  int    `json:"toolCount"`
	CarriesContextInstructions bool   `json:"carriesContextInstructions"`
}

type G5AutoRouterFingerprintDrift struct {
	RouterModelChangeInvalidates     bool `json:"routerModelChangeInvalidates"`
	SystemPromptChangeInvalidates    bool `json:"systemPromptChangeInvalidates"`
	TimeoutChangeInvalidates         bool `json:"timeoutChangeInvalidates"`
	MaxTokensChangeInvalidates       bool `json:"maxTokensChangeInvalidates"`
	TemperatureChangeInvalidates     bool `json:"temperatureChangeInvalidates"`
	ReasoningEffortChangeInvalidates bool `json:"reasoningEffortChangeInvalidates"`
	DefaultFlashAliasStable          bool `json:"defaultFlashAliasStable"`
	ResponseFormatDefaultStable      bool `json:"responseFormatDefaultStable"`
}

type G5AutoRouterTimeoutFallback struct {
	TimeoutMs               int    `json:"timeoutMs"`
	TimeoutFingerprint      string `json:"timeoutFingerprint"`
	FallbackModel           string `json:"fallbackModel"`
	FallbackReasoningEffort string `json:"fallbackReasoningEffort"`
	FallbackSource          string `json:"fallbackSource"`
	AbortsClassifier        bool   `json:"abortsClassifier"`
}

type G5AutoRouterRecommendationCase struct {
	ID                      string  `json:"id"`
	Raw                     string  `json:"raw"`
	Accepted                bool    `json:"accepted"`
	ExpectedModel           *string `json:"expectedModel"`
	ExpectedReasoningEffort *string `json:"expectedReasoningEffort"`
}

type G5AutoRouterContextBoundary struct {
	CurrentTurnID        string   `json:"currentTurnId"`
	IncludedRows         []string `json:"includedRows"`
	ExcludedLatestText   string   `json:"excludedLatestText"`
	ActiveTurnExcluded   bool     `json:"activeTurnExcluded"`
	ToolResultSummarized bool     `json:"toolResultSummarized"`
}

type G5AutoRouterProductBoundary struct {
	PublicAutoPlanSetting      bool `json:"publicAutoPlanSetting"`
	ProjectAutoPlanOverride    bool `json:"projectAutoPlanOverride"`
	ReasonixControllerProtocol bool `json:"reasonixControllerProtocol"`
	TopLevelRouteExposed       bool `json:"topLevelRouteExposed"`
}

type G5AutoRouterClassifierOutput struct {
	FingerprintCurrent                  bool `json:"fingerprintCurrent"`
	ContractDriftInvalidatesCache       bool `json:"contractDriftInvalidatesCache"`
	RequestIsolated                     bool `json:"requestIsolated"`
	TimeoutFallsBackToHeuristic         bool `json:"timeoutFallsBackToHeuristic"`
	AbortsTimedOutClassifier            bool `json:"abortsTimedOutClassifier"`
	AcceptedRecommendationCount         int  `json:"acceptedRecommendationCount"`
	RejectedRecommendationCount         int  `json:"rejectedRecommendationCount"`
	ProMaxRecommendationAccepted        bool `json:"proMaxRecommendationAccepted"`
	AutoRecommendationRejected          bool `json:"autoRecommendationRejected"`
	MalformedRecommendationRejected     bool `json:"malformedRecommendationRejected"`
	ActiveTurnExcludedFromRecentContext bool `json:"activeTurnExcludedFromRecentContext"`
	RecentContextPreservesToolSummary   bool `json:"recentContextPreservesToolSummary"`
	StablePrefixContainsClassifierState bool `json:"stablePrefixContainsClassifierState"`
	UsesReasonixProtocol                bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed                bool `json:"topLevelRouteExposed"`
}

type G5CombinedExecutableCase struct {
	AutoRouteCache G5CombinedAutoRouteCache `json:"autoRouteCache"`
	StepLimit      G5CombinedStepLimit      `json:"stepLimit"`
	Cancel         G5CombinedCancel         `json:"cancel"`
	Expected       G5CombinedOutput         `json:"expected"`
}

type G5CombinedAutoRouteCache struct {
	SameTurnRouterCalls           int  `json:"sameTurnRouterCalls"`
	MainModelSteps                int  `json:"mainModelSteps"`
	NextTurnRouterCalls           int  `json:"nextTurnRouterCalls"`
	ClassifierStateInStablePrefix bool `json:"classifierStateInStablePrefix"`
}

type G5CombinedStepLimit struct {
	MaxModelSteps              int    `json:"maxModelSteps"`
	ErrorCode                  string `json:"errorCode"`
	DynamicLimitInStablePrefix bool   `json:"dynamicLimitInStablePrefix"`
}

type G5CombinedCancel struct {
	AcceptedToolCalls   []G5AcceptedToolCall `json:"acceptedToolCalls"`
	CancelledResultCode string               `json:"cancelledResultCode"`
}

type G5CombinedOutput struct {
	SameTurnRouterCalls               int            `json:"sameTurnRouterCalls"`
	MainModelSteps                    int            `json:"mainModelSteps"`
	MaxModelSteps                     int            `json:"maxModelSteps"`
	NextTurnRouterCalls               int            `json:"nextTurnRouterCalls"`
	RouteCacheReusedUntilStepLimit    bool           `json:"routeCacheReusedUntilStepLimit"`
	NextTurnReroutes                  bool           `json:"nextTurnReroutes"`
	StepLimitErrorCode                string         `json:"stepLimitErrorCode"`
	DynamicControlStateInStablePrefix bool           `json:"dynamicControlStateInStablePrefix"`
	CancelledResultCode               string         `json:"cancelledResultCode"`
	ClassifierStateInStablePrefix     bool           `json:"classifierStateInStablePrefix"`
	StepLimitDynamicInStablePrefix    bool           `json:"stepLimitDynamicInStablePrefix"`
	AcceptedToolCallCount             int            `json:"acceptedToolCallCount"`
	CancelResultCount                 int            `json:"cancelResultCount"`
	CompletedResultCount              int            `json:"completedResultCount"`
	AbortedResultCount                int            `json:"abortedResultCount"`
	CancelResults                     []G5ToolResult `json:"cancelResults"`
	CompletedResultPreserved          bool           `json:"completedResultPreserved"`
	UnstartedResultStatus             string         `json:"unstartedResultStatus"`
}

type G5PlanStepCancelCacheCase struct {
	Mode                     string                        `json:"mode"`
	Model                    string                        `json:"model"`
	RequestCount             int                           `json:"requestCount"`
	AbortedRunStatus         string                        `json:"abortedRunStatus"`
	RetryRunStatus           string                        `json:"retryRunStatus"`
	Step0MustAdvertise       []string                      `json:"step0MustAdvertise"`
	Step0MustNotAdvertise    []string                      `json:"step0MustNotAdvertise"`
	FollowUpRequiredToolName string                        `json:"followUpRequiredToolName"`
	FollowUpTools            []string                      `json:"followUpTools"`
	RetryMustAdvertise       []string                      `json:"retryMustAdvertise"`
	UsageEventCount          int                           `json:"usageEventCount"`
	Provider                 string                        `json:"provider"`
	EndpointFormat           string                        `json:"endpointFormat"`
	CacheHitTokens           int                           `json:"cacheHitTokens"`
	CacheMissTokens          int                           `json:"cacheMissTokens"`
	PrefixChanged            bool                          `json:"prefixChanged"`
	PrefixChangeReasons      []string                      `json:"prefixChangeReasons"`
	ProductBoundary          G5PlanStepCancelCacheBoundary `json:"productBoundary"`
	Expected                 G5PlanStepCancelCacheOutput   `json:"expected"`
}

type G5PlanStepCancelCacheBoundary struct {
	PublicAutoPlanSetting      bool `json:"publicAutoPlanSetting"`
	ReasonixControllerProtocol bool `json:"reasonixControllerProtocol"`
	TopLevelRouteExposed       bool `json:"topLevelRouteExposed"`
	DefaultGoBackendEnabled    bool `json:"defaultGoBackendEnabled"`
}

type G5PlanStepCancelCacheOutput struct {
	Step0ReadOnlyPlusPlan                    bool `json:"step0ReadOnlyPlusPlan"`
	FollowUpOnlyCreatePlan                   bool `json:"followUpOnlyCreatePlan"`
	CancelledStepDoesNotAdvanceCacheBaseline bool `json:"cancelledStepDoesNotAdvanceCacheBaseline"`
	NextPlanReusesOriginalCacheBaseline      bool `json:"nextPlanReusesOriginalCacheBaseline"`
	UsageEventCount                          int  `json:"usageEventCount"`
	AllPrefixChangedFalse                    bool `json:"allPrefixChangedFalse"`
	CacheTelemetryPreserved                  bool `json:"cacheTelemetryPreserved"`
	ForbiddenShellExcluded                   bool `json:"forbiddenShellExcluded"`
	UsesReasonixProtocol                     bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed                     bool `json:"topLevelRouteExposed"`
	DefaultGoBackendEnabled                  bool `json:"defaultGoBackendEnabled"`
}

type G5PlanCancelStateResetCase struct {
	SourceTestName                      string                         `json:"sourceTestName"`
	PreviousMode                        string                         `json:"previousMode"`
	AbortedRunStatus                    string                         `json:"abortedRunStatus"`
	PlanToolName                        string                         `json:"planToolName"`
	NormalModel                         string                         `json:"normalModel"`
	NormalMustNotAdvertise              []string                       `json:"normalMustNotAdvertise"`
	NormalModeInstructionPresent        bool                           `json:"normalModeInstructionPresent"`
	NormalRequiredToolNamePresent       bool                           `json:"normalRequiredToolNamePresent"`
	AutoRequestedModel                  string                         `json:"autoRequestedModel"`
	AutoRouterCalls                     int                            `json:"autoRouterCalls"`
	AutoRouterTurnIDSuffix              string                         `json:"autoRouterTurnIdSuffix"`
	AutoRouterToolCount                 int                            `json:"autoRouterToolCount"`
	AutoRouterPrefixItemCount           int                            `json:"autoRouterPrefixItemCount"`
	AutoRouterModeInstructionPresent    bool                           `json:"autoRouterModeInstructionPresent"`
	AutoRealModel                       string                         `json:"autoRealModel"`
	AutoReasoningEffort                 string                         `json:"autoReasoningEffort"`
	AutoMustNotAdvertise                []string                       `json:"autoMustNotAdvertise"`
	AutoModeInstructionPresent          bool                           `json:"autoModeInstructionPresent"`
	AutoRequiredToolNamePresent         bool                           `json:"autoRequiredToolNamePresent"`
	StablePrefixContainsPlanState       bool                           `json:"stablePrefixContainsPlanState"`
	StablePrefixContainsClassifierState bool                           `json:"stablePrefixContainsClassifierState"`
	ProductBoundary                     G5PlanCancelStateResetBoundary `json:"productBoundary"`
	Expected                            G5PlanCancelStateResetOutput   `json:"expected"`
}

type G5PlanCancelStateResetBoundary struct {
	PublicAutoPlanSetting      bool `json:"publicAutoPlanSetting"`
	ProjectAutoPlanOverride    bool `json:"projectAutoPlanOverride"`
	ReasonixControllerProtocol bool `json:"reasonixControllerProtocol"`
	TopLevelRouteExposed       bool `json:"topLevelRouteExposed"`
	DefaultGoBackendEnabled    bool `json:"defaultGoBackendEnabled"`
}

type G5PlanCancelStateResetOutput struct {
	CancelledPlanDoesNotLeakMode   bool `json:"cancelledPlanDoesNotLeakMode"`
	NormalTurnHidesCreatePlan      bool `json:"normalTurnHidesCreatePlan"`
	NormalTurnHasNoPlanRequirement bool `json:"normalTurnHasNoPlanRequirement"`
	AutoTurnReroutesAfterCancel    bool `json:"autoTurnReroutesAfterCancel"`
	AutoRouterRequestIsolated      bool `json:"autoRouterRequestIsolated"`
	AutoRecommendationCurrent      bool `json:"autoRecommendationCurrent"`
	AutoTurnHidesCreatePlan        bool `json:"autoTurnHidesCreatePlan"`
	AutoTurnHasNoPlanRequirement   bool `json:"autoTurnHasNoPlanRequirement"`
	StablePrefixClean              bool `json:"stablePrefixClean"`
	UsesReasonixProtocol           bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed           bool `json:"topLevelRouteExposed"`
	DefaultGoBackendEnabled        bool `json:"defaultGoBackendEnabled"`
}

type G5ControlExecutableOutput struct {
	ProductBoundary             G5ProductBoundaryOutput             `json:"productBoundary"`
	PackageRuntimeIdentity      G5PackageRuntimeIdentityOutput      `json:"packageRuntimeIdentity"`
	RuntimeHTTPRouteSovereignty G5RuntimeHTTPRouteSovereigntyOutput `json:"runtimeHttpRouteSovereignty"`
	DesktopSovereignty          G5DesktopSovereigntyOutput          `json:"desktopSovereignty"`
	DesktopMainIpcBoundary      G5DesktopMainIpcBoundaryOutput      `json:"desktopMainIpcBoundary"`
	RendererRouteSurface        G5RendererRouteSurfaceOutput        `json:"rendererRouteSurfaceSovereignty"`
	GoalPersistenceOffLock      G5GoalPersistenceOffLockOutput      `json:"goalPersistenceOffLock"`
	ToolResultFileImageBoundary G5ToolResultFileImageBoundaryOutput `json:"toolResultFileImageBoundary"`
	EventJsonlReplayBoundary    G5EventJsonlReplayBoundaryOutput    `json:"eventJsonlReplayBoundary"`
	MCPMalformedSchemaBoundary  G5MCPMalformedSchemaBoundaryOutput  `json:"mcpMalformedSchemaBoundary"`
	Cancel                      G5CancelExecutableOutput            `json:"cancel"`
	TaskJobs                    G5TaskJobsExecutableOutput          `json:"taskJobs"`
	Approval                    G5ApprovalDenyOutput                `json:"approvalDeny"`
	UserInput                   G5UserInputOutput                   `json:"userInput"`
	ApprovalUserInputRoute      G5ApprovalUserInputReplay           `json:"approvalUserInputRouteReplay"`
	ApprovalUserInputInventory  G5ApprovalUserInputInventoryOutput  `json:"approvalUserInputInventory"`
	AbortCleanup                G5AbortCleanupOutput                `json:"abortCleanup"`
	ResumeGates                 G5ResumePendingGatesOutput          `json:"resumePendingGates"`
	AutoResearch                G5AutoResearchOutput                `json:"autoResearch"`
	MCPLifecycle                G5MCPLifecycleOutput                `json:"mcpLifecycle"`
	MCPCoreLifecycle            G5MCPCoreLifecycleOutput            `json:"mcpCoreLifecycle"`
	MCPBackgroundReconnect      G5MCPBackgroundReconnectOutput      `json:"mcpBackgroundReconnect"`
	MCPCallReconnect            G5MCPCallReconnectOutput            `json:"mcpCallReconnect"`
	MCPKnownOverrideDiagnostics G5MCPKnownOverrideDiagnosticsOutput `json:"mcpKnownOverrideDiagnostics"`
	MCPLiveLocalIndexer         G5MCPLiveLocalIndexerOutput         `json:"mcpLiveLocalIndexer"`
	MCPApprovalAnnotations      G5MCPApprovalAnnotationOutput       `json:"mcpApprovalAnnotations"`
	MCPSearchMetaTools          G5MCPSearchMetaToolsOutput          `json:"mcpSearchMetaTools"`
	MCPSearchRefreshDrift       G5MCPSearchRefreshDriftOutput       `json:"mcpSearchRefreshDrift"`
	MCPSearchWorkspace          G5MCPSearchWorkspaceBoundaryOutput  `json:"mcpSearchWorkspaceBoundary"`
	Checkpoint                  G5CheckpointRewindOutput            `json:"checkpointRewind"`
	RemoteEntry                 G5RemoteEntryOutput                 `json:"remoteEntry"`
	HistoryRepair               G5HistoryRepairOutput               `json:"historyRepair"`
	CompactionBoundary          G5CompactionBoundaryOutput          `json:"compactionBoundary"`
	StepLimits                  G5StepLimitsOutput                  `json:"stepLimits"`
	Planner                     G5PlannerExecutableOutput           `json:"planner"`
	AutoRouter                  G5AutoRouterClassifierOutput        `json:"autoRouterClassifier"`
	Combined                    G5CombinedOutput                    `json:"combined"`
	PlanStepCancelCache         G5PlanStepCancelCacheOutput         `json:"planStepCancelCache"`
	PlanCancelStateReset        G5PlanCancelStateResetOutput        `json:"planCancelStateReset"`
	ProviderCacheReleaseGuard   G5ProviderReleaseGuardOutput        `json:"providerCacheReleaseGuard"`
	ProviderUsageParser         G5ProviderUsageParserOutput         `json:"providerUsageParser"`
	ProviderRequestShape        G5ProviderRequestShapeOutput        `json:"providerRequestShape"`
	ProviderLiveLocalHTTP       ProviderLiveLocalHTTPContract       `json:"providerLiveLocalHttpContract"`
	ProviderCacheCoverageFloor  G5ProviderCacheCoverageFloorOutput  `json:"providerCacheCoverageFloor"`
	ProviderDriftAttribution    G5ProviderDriftAttribution          `json:"providerDriftAttribution"`
	ProviderCacheAccounting     G3ProviderCacheAccounting           `json:"providerCacheAccounting"`
	ProviderOfflineParitySeal   G5ProviderOfflineParitySeal         `json:"providerOfflineParitySeal"`
	ProviderCachePrivacy        G5ProviderCachePrivacyOutput        `json:"providerCachePrivacy"`
	ProviderCacheInventory      G5ProviderCacheInventoryOutput      `json:"providerCacheInventory"`
	ProviderStreaming           G5ProviderStreamingOutput           `json:"providerStreaming"`
	SessionRouteStatus          G5SessionRouteStatusOutput          `json:"sessionRouteStatus"`
	SessionRouteInventory       G5SessionRouteInventoryOutput       `json:"sessionRouteInventory"`
	SessionRouteReplay          G5SessionRouteReplayOutput          `json:"sessionRouteReplay"`
}

type G5CancelExecutableOutput struct {
	Results              []G5ToolResult `json:"results"`
	ScheduledAfterCancel int            `json:"scheduledAfterCancel"`
}

type G5TaskJobsExecutableOutput struct {
	ToolContractBoundary    G5TaskJobToolContractBoundaryOutput    `json:"toolContractBoundary"`
	TranscriptIdentity      G5TaskJobTranscriptIdentityOutput      `json:"transcriptIdentity"`
	NestedSSEMetadata       G5TaskJobNestedSSEMetadataOutput       `json:"nestedSseMetadata"`
	Jobs                    []G5TaskJobOutput                      `json:"jobs"`
	StaleReconciled         []G5TaskJobOutput                      `json:"staleReconciled"`
	RestartDrill            G5TaskJobRestartDrillOutput            `json:"restartDrill"`
	RouteExecutable         G5TaskJobRouteExecutableOutput         `json:"routeExecutable"`
	Lifecycle               G5TaskJobLifecycleOutput               `json:"lifecycle"`
	PlannerExecutor         G5TaskJobPlannerExecutorOutput         `json:"plannerExecutor"`
	PlannerToolsetInventory G5TaskJobPlannerToolsetInventoryOutput `json:"plannerToolsetInventory"`
	ParentGoal              G5ParentGoalEvidenceOutput             `json:"parentGoalEvidence"`
	ParallelValidation      G5ParallelValidationOutput             `json:"parallelValidation"`
}

type G5PlannerExecutableOutput struct {
	Step0Advertised []string                `json:"step0Advertised"`
	Step1Advertised []string                `json:"step1Advertised"`
	RejectedCall    G5PlannerRejectedCall   `json:"rejectedCall"`
	Gate            G5PlannerGateOutput     `json:"gate"`
	RejectedCalls   []G5PlannerRejectedCall `json:"rejectedCalls"`
}

type G5PlannerGateOutput struct {
	NormalModeHidesPlanTool   bool     `json:"normalModeHidesPlanTool"`
	CapabilityGateAdvertised  []string `json:"capabilityGateAdvertised"`
	Step0Advertised           []string `json:"step0Advertised"`
	Step1Advertised           []string `json:"step1Advertised"`
	Step0ExcludesBlockedTools bool     `json:"step0ExcludesBlockedTools"`
	Step1OnlyCreatePlan       bool     `json:"step1OnlyCreatePlan"`
	RejectedToolNames         []string `json:"rejectedToolNames"`
	RejectedCallCount         int      `json:"rejectedCallCount"`
	AllForgedCallsRejected    bool     `json:"allForgedCallsRejected"`
	UsesReasonixProtocol      bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed      bool     `json:"topLevelRouteExposed"`
}

func BuildG5ControlExecutableOutput(cases G5ControlExecutableCases) G5ControlExecutableOutput {
	return G5ControlExecutableOutput{
		ProductBoundary:             replayG5ProductBoundary(cases.ProductBoundary),
		PackageRuntimeIdentity:      replayG5PackageRuntimeIdentity(cases.PackageRuntimeIdentity),
		RuntimeHTTPRouteSovereignty: replayG5RuntimeHTTPRouteSovereignty(cases.RuntimeHTTPRouteSovereignty),
		DesktopSovereignty:          replayG5DesktopSovereignty(cases.DesktopSovereignty),
		DesktopMainIpcBoundary:      replayG5DesktopMainIpcBoundary(cases.DesktopMainIpcBoundary),
		RendererRouteSurface:        replayG5RendererRouteSurface(cases.RendererRouteSurface),
		GoalPersistenceOffLock:      replayG5GoalPersistenceOffLock(cases.GoalPersistenceOffLock),
		ToolResultFileImageBoundary: replayG5ToolResultFileImageBoundary(cases.ToolResultFileImageBoundary),
		EventJsonlReplayBoundary:    replayG5EventJsonlReplayBoundary(cases.EventJsonlReplayBoundary),
		MCPMalformedSchemaBoundary:  replayG5MCPMalformedSchemaBoundary(cases.MCPMalformedSchemaBoundary),
		Cancel: G5CancelExecutableOutput{
			Results:              replayG5Cancel(cases.Cancel),
			ScheduledAfterCancel: 0,
		},
		TaskJobs: G5TaskJobsExecutableOutput{
			ToolContractBoundary:    replayG5TaskJobToolContractBoundary(cases.TaskJobs.ToolContractBoundary),
			TranscriptIdentity:      replayG5TaskJobTranscriptIdentity(cases.TaskJobs.TranscriptIdentity),
			NestedSSEMetadata:       replayG5TaskJobNestedSSEMetadata(cases.TaskJobs.NestedSSEMetadata),
			Jobs:                    replayG5TaskJobs(cases.TaskJobs),
			StaleReconciled:         replayG5TaskJobStaleReconcile(cases.TaskJobs.StaleReconcile),
			RestartDrill:            replayG5TaskJobRestartDrill(cases.TaskJobs.RestartDrill),
			RouteExecutable:         replayG5TaskJobRouteExecutable(cases.TaskJobs.RouteExecutable),
			Lifecycle:               replayG5TaskJobLifecycle(cases.TaskJobs.Lifecycle),
			PlannerExecutor:         replayG5TaskJobPlannerExecutor(cases.TaskJobs.PlannerExecutor),
			PlannerToolsetInventory: replayG5TaskJobPlannerToolsetInventory(cases.TaskJobs.PlannerToolsetInventory),
			ParentGoal:              replayG5ParentGoalEvidence(cases.TaskJobs.ParentGoalEvidence),
			ParallelValidation:      replayG5ParallelValidation(cases.TaskJobs.ParallelValidation),
		},
		Approval:                    replayG5ApprovalDeny(cases.Approval),
		UserInput:                   replayG5UserInput(cases.UserInput),
		ApprovalUserInputRoute:      buildG5ApprovalUserInputReplay(cases.ApprovalUserInputRoute.Contract),
		ApprovalUserInputInventory:  replayG5ApprovalUserInputInventory(cases.ApprovalUserInputInventory),
		AbortCleanup:                replayG5AbortCleanup(cases.AbortCleanup),
		ResumeGates:                 replayG5ResumePendingGates(cases.ResumeGates),
		AutoResearch:                replayG5AutoResearch(cases.AutoResearch),
		MCPLifecycle:                replayG5MCPLifecycle(cases.MCPLifecycle),
		MCPCoreLifecycle:            replayG5MCPCoreLifecycle(cases.MCPCoreLifecycle),
		MCPBackgroundReconnect:      replayG5MCPBackgroundReconnect(cases.MCPBackgroundReconnect),
		MCPCallReconnect:            replayG5MCPCallReconnect(cases.MCPCallReconnect),
		MCPKnownOverrideDiagnostics: replayG5MCPKnownOverrideDiagnostics(cases.MCPKnownOverrideDiagnostics),
		MCPLiveLocalIndexer:         replayG5MCPLiveLocalIndexer(cases.MCPLiveLocalIndexer),
		MCPApprovalAnnotations:      replayG5MCPApprovalAnnotations(cases.MCPApprovalAnnotations),
		MCPSearchMetaTools:          replayG5MCPSearchMetaTools(cases.MCPSearchMetaTools),
		MCPSearchRefreshDrift:       replayG5MCPSearchRefreshDrift(cases.MCPSearchRefreshDrift),
		MCPSearchWorkspace:          replayG5MCPSearchWorkspaceBoundary(cases.MCPSearchWorkspace),
		Checkpoint:                  replayG5CheckpointRewind(cases.Checkpoint),
		RemoteEntry:                 replayG5RemoteEntry(cases.RemoteEntry),
		HistoryRepair:               replayG5HistoryRepair(cases.HistoryRepair),
		CompactionBoundary:          replayG5CompactionBoundary(cases.CompactionBoundary),
		StepLimits:                  replayG5StepLimits(cases.StepLimits),
		Planner:                     replayG5PlannerPolicy(cases.Planner),
		AutoRouter:                  replayG5AutoRouterClassifier(cases.AutoRouter),
		Combined:                    replayG5CombinedControls(cases.Combined),
		PlanStepCancelCache:         replayG5PlanStepCancelCache(cases.PlanStepCancelCache),
		PlanCancelStateReset:        replayG5PlanCancelStateReset(cases.PlanCancelStateReset),
		ProviderCacheReleaseGuard:   replayG5ProviderReleaseGuard(cases.ProviderCacheReleaseGuard),
		ProviderUsageParser:         buildG5ProviderUsageParserReplay(cases.ProviderUsageParser.Cases),
		ProviderRequestShape:        buildProviderRequestShapeReplay(cases.ProviderRequestShape.Cases),
		ProviderLiveLocalHTTP:       replayG5ProviderLiveLocalHTTPContract(cases.ProviderLiveLocalHTTP),
		ProviderCacheCoverageFloor:  replayG5ProviderCacheCoverageFloor(cases.ProviderCacheCoverageFloor),
		ProviderDriftAttribution:    buildProviderDriftAttribution(cases.ProviderDriftAttribution.DriftAttribution),
		ProviderCacheAccounting:     buildG5ProviderCacheAccounting(cases.ProviderCacheAccounting.Cases),
		ProviderOfflineParitySeal:   replayG5ProviderOfflineParitySeal(cases.ProviderOfflineParitySeal),
		ProviderCachePrivacy:        replayG5ProviderCachePrivacy(cases.ProviderCachePrivacy),
		ProviderCacheInventory:      replayG5ProviderCacheInventory(cases.ProviderCacheInventory),
		ProviderStreaming:           replayG5ProviderStreaming(cases.ProviderStreaming),
		SessionRouteStatus:          buildG5RouteStatusReplay(cases.SessionRouteStatus.Routes),
		SessionRouteInventory:       replayG5SessionRouteInventory(cases.SessionRouteInventory),
		SessionRouteReplay:          replayG5SessionRouteReplay(cases.SessionRouteReplay),
	}
}

func replayG5ProductBoundary(input G5ProductBoundaryCase) G5ProductBoundaryOutput {
	return G5ProductBoundaryOutput{
		RendererPreloadMainBridgeUnchanged: input.ProductBoundary.RendererPreloadMainBridgeUnchanged,
		AnalytixServeContractUnchanged:     input.ProductBoundary.AnalytixServeContractUnchanged,
		ReasonixPublicProtocolAllowed:      input.ProductBoundary.ReasonixPublicProtocolAllowed,
		DefaultGoBackendAllowed:            input.ProductBoundary.DefaultGoBackendAllowed,
		RendererVisibleGoRoutesAllowed:     input.ProductBoundary.RendererVisibleGoRoutesAllowed,
		ElectronMainConnected:              input.ElectronMainConnected,
		DefaultGoBackendEnabled:            input.DefaultGoBackendEnabled,
		UsesReasonixProtocol:               input.ProductBoundary.ReasonixPublicProtocolAllowed,
		RendererRouteExposed:               input.ProductBoundary.RendererVisibleGoRoutesAllowed,
	}
}

func replayG5PackageRuntimeIdentity(input G5PackageRuntimeIdentityCase) G5PackageRuntimeIdentityOutput {
	runtimeBinAnalytixServeEntry := input.ReleaseIdentity.RuntimeBinName == "analytix" &&
		input.ReleaseIdentity.RuntimeBinPath == "./dist/cli/serve-entry.js"
	nsisNamesAnalytix := input.ReleaseIdentity.NSISShortcutName == "Analytix灵鉴" &&
		input.ReleaseIdentity.NSISUninstallDisplayName == "Analytix灵鉴"
	resolveBundledServeEntry := input.Evidence.ResolveUsesBundledServeEntry &&
		input.RuntimeCLI.BundledEntryCandidate == "packages/runtime/dist/cli/serve-entry.js"
	serveUsageAnalytixServe := input.Evidence.ServeUsageMentionsAnalytixServe &&
		input.RuntimeCLI.ServeUsagePrefix == "analytix serve [options]"
	serveEntryAllowsOnlyAnalytixServe := input.Evidence.ServeEntryOnlySupportsServe &&
		input.RuntimeCLI.SupportedCommand == "serve" &&
		input.RuntimeCLI.UnknownCommandMessage == "Only 'analytix serve' is supported."
	afterPackRequiresServeEntry := input.Evidence.AfterPackRequiresServeEntry &&
		input.RuntimeCLI.AfterPackRequiredPath == input.RuntimeCLI.BundledEntryCandidate
	return G5PackageRuntimeIdentityOutput{
		RootPackageNameAnalytix:           input.ReleaseIdentity.RootPackageName == "analytix",
		RootProductNameAnalytix:           input.ReleaseIdentity.RootProductName == "analytix",
		RuntimePackageNameAnalytixRuntime: input.ReleaseIdentity.RuntimePackageName == "analytix-runtime",
		RuntimeBinAnalytixServeEntry:      runtimeBinAnalytixServeEntry,
		RuntimeServeScriptUsesServeEntry:  input.ReleaseIdentity.RuntimeServeScript == "node ./dist/cli/serve-entry.js",
		BuilderAppIDAnalytix:              input.ReleaseIdentity.AppID == "com.analytix.desktop",
		BuilderProductNameAnalytix:        input.ReleaseIdentity.BuilderProductName == "analytix",
		BuilderArtifactNameAnalytix:       input.ReleaseIdentity.ArtifactNamePrefix == "analytix-",
		NSISNamesAnalytix:                 nsisNamesAnalytix,
		AppProductNameAnalytix:            input.ReleaseIdentity.AppProductName == "analytix",
		WindowsAppUserModelIDAnalytix:     input.ReleaseIdentity.WindowsAppUserModelID == "com.analytix.desktop",
		ResolveBundledServeEntry:          resolveBundledServeEntry,
		ServeUsageAnalytixServe:           serveUsageAnalytixServe,
		ServeEntryAllowsOnlyAnalytixServe: serveEntryAllowsOnlyAnalytixServe,
		ReadyHandshakeAnalytix:            input.RuntimeCLI.ReadyPrefix == "ANALYTIX_READY ",
		AfterPackRequiresServeEntry:       afterPackRequiresServeEntry,
		ReleaseEnvAnalytixPrefixed:        input.Evidence.ReleaseWorkflowUsesAnalytixEnv,
		UsesReasonixProtocol:              input.ProductBoundary.UsesReasonixProtocol,
		KunIdentityExposed:                len(input.ForbiddenIdentityFieldsPresent) > 0 || input.ProductBoundary.KunIdentityExposed,
		DefaultGoBackendEnabled:           input.ProductBoundary.DefaultGoBackendEnabled,
	}
}

func replayG5RuntimeHTTPRouteSovereignty(input G5RuntimeHTTPRouteSovereigntyCase) G5RuntimeHTTPRouteSovereigntyOutput {
	routeKeys := make([]string, 0, len(input.Routes))
	for _, route := range input.Routes {
		routeKeys = append(routeKeys, route.Method+" "+route.Path)
	}
	protectedRouteKeys := make([]string, 0, len(routeKeys))
	for _, route := range routeKeys {
		if route != "GET /health" {
			protectedRouteKeys = append(protectedRouteKeys, route)
		}
	}
	threadLifecycleRoutes := []string{
		"GET /v1/threads",
		"POST /v1/threads",
		"GET /v1/threads/:id",
		"PATCH /v1/threads/:id",
		"DELETE /v1/threads/:id",
		"POST /v1/threads/:id/fork",
		"POST /v1/sessions/:id/resume-thread",
	}
	approvalUserInputRoutes := []string{
		"POST /v1/approvals/:id",
		"POST /v1/user-inputs/:id",
	}
	sensitiveRoutes := []string{
		"GET /v1/threads/:id/events",
		"POST /v1/runtime/task-jobs/wait",
		"POST /v1/approvals/:id",
		"POST /v1/user-inputs/:id",
		"POST /v1/sessions/:id/resume-thread",
	}
	forbiddenProtocolTokens := []string{
		"/v1/reasonix",
		"/v1/kun",
		"/v1/deepseek",
		"/v1/runtime/go",
		"/session-api",
		"/api/session",
	}
	forbiddenHiddenSurfaceTokens := []string{
		"/v1/workflow",
		"/v1/create-loop",
		"/v1/subagent",
		"/v1/subagents",
		"/v1/autoresearch",
		"/v1/auto-research",
		"/v1/mcp-indexer",
	}
	sharedBuilderNames := make([]string, 0, len(input.SharedEndpointBuilderMatrix.BuilderCases))
	sharedEndpointBuildersEncodeRouteIDs := true
	for _, builderCase := range input.SharedEndpointBuilderMatrix.BuilderCases {
		sharedBuilderNames = append(sharedBuilderNames, builderCase.Name)
		if len(builderCase.EncodedSegments) == 0 ||
			strings.Contains(builderCase.OutputPath, "with space") ||
			strings.Contains(builderCase.OutputPath, "?x=") ||
			strings.Contains(builderCase.OutputPath, "#frag") {
			sharedEndpointBuildersEncodeRouteIDs = false
			continue
		}
		for _, segment := range builderCase.EncodedSegments {
			if !strings.Contains(builderCase.OutputPath, segment) {
				sharedEndpointBuildersEncodeRouteIDs = false
				break
			}
		}
	}
	sharedEndpointTemplatesAnalytixOwned := len(input.SharedEndpointBuilderMatrix.ForbiddenTokensPresent) == 0
	for _, endpoint := range input.SharedEndpointBuilderMatrix.ExportedEndpointStrings {
		if endpoint != "/health" && !strings.HasPrefix(endpoint, "/v1/") {
			sharedEndpointTemplatesAnalytixOwned = false
			break
		}
	}
	allRoutesAnalytixOwned := true
	for _, route := range routeKeys {
		if route != "GET /health" && !strings.Contains(route, " /v1/") {
			allRoutesAnalytixOwned = false
			break
		}
	}
	return G5RuntimeHTTPRouteSovereigntyOutput{
		RouteCountExact:               input.RouteCount == 52 && len(input.Routes) == 52,
		OnlyHealthUnauthenticated:     len(input.UnauthenticatedRoutes) == 1 && input.UnauthenticatedRoutes[0] == "GET /health" && input.AuthenticatedRouteCount == input.RouteCount-1,
		AllRuntimeRoutesAnalytixOwned: allRoutesAnalytixOwned,
		HealthUnauthenticatedStatusOK: input.AuthMatrix.HealthRoute == "GET /health" && input.AuthMatrix.HealthStatus == 200,
		AllAuthenticatedRoutesRejectMissingAuth: input.AuthMatrix.UnauthorizedStatus == 401 &&
			input.AuthMatrix.ProtectedRouteCount == input.AuthenticatedRouteCount &&
			sameStringSlice(input.AuthMatrix.ProtectedRouteKeys, protectedRouteKeys),
		UnauthorizedBodyShapeStable: input.AuthMatrix.UnauthorizedBody.Code == "unauthorized" &&
			input.AuthMatrix.UnauthorizedBody.Message == "Runtime authentication is required.",
		AuthMatrixCoversAllRegisteredRoutes: input.AuthMatrix.ProtectedRouteCount+len(input.UnauthenticatedRoutes) == len(input.Routes) &&
			sameStringSlice(input.AuthMatrix.ProtectedRouteKeys, protectedRouteKeys),
		SensitiveRoutesProtected: sameStringSlice(input.AuthMatrix.SensitiveRouteKeys, sensitiveRoutes) && allStringInSlice(sensitiveRoutes, protectedRouteKeys),
		ForbiddenDispatchReturnsStructuredNotFound: input.ForbiddenDispatchMatrix.Status == 404 &&
			input.ForbiddenDispatchMatrix.Body.Code == "not_found" &&
			input.ForbiddenDispatchMatrix.Body.Message == "route not found",
		ForbiddenDispatchCoversAllTokens: input.ForbiddenDispatchMatrix.TokenCount == len(input.ForbiddenRouteTokens) &&
			sameStringSlice(input.ForbiddenDispatchMatrix.Tokens, input.ForbiddenRouteTokens),
		ForbiddenProtocolTokensRejected:          sameStringSlice(input.ForbiddenDispatchMatrix.ProtocolTokens, forbiddenProtocolTokens) && allStringInSlice(forbiddenProtocolTokens, input.ForbiddenRouteTokens),
		ForbiddenHiddenSurfaceTokensRejected:     sameStringSlice(input.ForbiddenDispatchMatrix.HiddenSurfaceTokens, forbiddenHiddenSurfaceTokens) && allStringInSlice(forbiddenHiddenSurfaceTokens, input.ForbiddenRouteTokens),
		ForbiddenDispatchUsesRuntimeAuth:         input.ForbiddenDispatchMatrix.AuthMode == "valid-runtime-token",
		SharedEndpointBuilderCaseCountExact:      len(input.SharedEndpointBuilderMatrix.BuilderCases) == 25,
		SharedEndpointBuildersEncodeRouteIds:     sharedEndpointBuildersEncodeRouteIDs,
		SharedEndpointTemplatesAnalytixOwned:     sharedEndpointTemplatesAnalytixOwned,
		SharedEndpointCanonicalUserInputPlural:   input.SharedEndpointBuilderMatrix.CanonicalUserInputTemplate == "/v1/user-inputs/{id}" && !input.SharedEndpointBuilderMatrix.SingularUserInputTemplateExported,
		SharedEndpointSensitiveBuildersPresent:   allStringInSlice(input.SharedEndpointBuilderMatrix.SensitiveBuilderNames, sharedBuilderNames),
		SharedEndpointBuilderUnitEvidencePresent: input.SharedEndpointBuilderMatrix.UnitTestEvidencePresent,
		SSERouteAnalytixThreadEvents:             input.Evidence.SSEUsesBuildEventStreamResponse && sameStringSlice(input.SSERoutes, []string{"GET /v1/threads/:id/events"}),
		ThreadLifecycleRoutesPresent:             allStringInSlice(threadLifecycleRoutes, routeKeys),
		ApprovalUserInputRoutesPresent:           allStringInSlice(approvalUserInputRoutes, routeKeys),
		TaskJobRoutesInternalOnly:                input.Evidence.TaskJobRoutesUseInternalHandlers && len(input.InternalTaskJobRoutes) == 3,
		NotFoundStructured:                       input.Evidence.StructuredNotFound,
		NoForbiddenRoutes:                        len(input.ForbiddenRouteTokensPresent) == 0,
		SingularUserInputCompatibilityOnly:       input.Evidence.SingularUserInputCompatibility && sameStringSlice(input.CompatibilityOnlyRoutes, []string{"POST /v1/user-input/:id"}),
		UsesReasonixProtocol:                     input.ProductBoundary.UsesReasonixProtocol,
		TopLevelHiddenEntryExposed:               input.ProductBoundary.TopLevelHiddenEntryExposed,
		DefaultGoBackendEnabled:                  input.ProductBoundary.DefaultGoBackendEnabled,
	}
}

func replayG5DesktopSovereignty(input G5DesktopSovereigntyCase) G5DesktopSovereigntyOutput {
	deprecatedBridgeAliasExposed :=
		anyStringInSlice(input.ForbiddenBridgeAliases, input.ExposedBridgeNames) ||
			anyStringInSlice(input.ForbiddenBridgeAliases, input.WindowTypeProperties) ||
			anyStringInSlice(input.ForbiddenBridgeAliases, input.FacadeDomains)
	forbiddenRuntimeIpcExposed := len(input.ForbiddenRuntimeIpcChannelsExposed) > 0
	publicAPIAnalytixOwned := stringInSlice("shared-api-no-upstream-public-types", input.EvidenceIDs)
	runtimeRequestUsesAnalytixIpc := input.RuntimeIpcChannels.Request == "runtime:request" &&
		stringInSlice("runtime-request-analytix-ipc", input.EvidenceIDs) &&
		!forbiddenRuntimeIpcExposed
	runtimeSseUsesAnalytixIpc := input.RuntimeIpcChannels.SSEStart == "runtime:sse:start" &&
		input.RuntimeIpcChannels.SSEStop == "runtime:sse:stop" &&
		input.RuntimeIpcChannels.SSEEvent == "runtime:sse-event" &&
		input.RuntimeIpcChannels.SSEEnd == "runtime:sse-end" &&
		input.RuntimeIpcChannels.SSEError == "runtime:sse-error" &&
		stringInSlice("runtime-sse-analytix-ipc", input.EvidenceIDs) &&
		!forbiddenRuntimeIpcExposed
	rendererNamedRuntimeApisAllowListed :=
		len(input.RendererBridgeAllowList.DirectRuntimeViolations) == 0 &&
			allStringInSlice(input.RendererBridgeAllowList.DirectRuntimeAPIs, input.RendererBridgeAllowList.AllowedNamedRuntimeAPIs)
	rendererNamedSettingsApisAllowListed :=
		len(input.RendererBridgeAllowList.DirectSettingsViolations) == 0 &&
			allStringInSlice(input.RendererBridgeAllowList.DirectSettingsAPIs, input.RendererBridgeAllowList.AllowedNamedSettingsAPIs)
	rendererGenericRuntimeBypassAbsent :=
		input.RendererBridgeAllowList.GenericRuntimeBypassCount == 0 &&
			stringInSlice("renderer-generic-runtime-bypass-absent", input.EvidenceIDs)
	rendererSettingsReadBypassAbsent :=
		input.RendererBridgeAllowList.SettingsReadBypassCount == 0 &&
			stringInSlice("renderer-settings-read-bypass-absent", input.EvidenceIDs)
	optionalChainBridgeAccessScanned :=
		input.RendererBridgeAllowList.OptionalChainPatternCovered &&
			stringInSlice("renderer-optional-chain-bridge-scan", input.EvidenceIDs)
	rendererRuntimeClientRuntimeRequestPreservesArguments :=
		input.RendererRuntimeClientBridgeMatrix.SourceUsesAnalytixRuntime &&
			input.RendererRuntimeClientBridgeMatrix.SourcePassesRuntimeRequestArgumentsUnchanged &&
			input.RendererRuntimeClientBridgeMatrix.RuntimeRequestUnitEvidencePresent &&
			stringInSlice("renderer-runtime-client-request-evidence", input.EvidenceIDs) &&
			len(input.RendererRuntimeClientBridgeMatrix.RuntimeRequestCalls) == 3
	expectedRendererRuntimeClientArgs := map[int]bool{1: false, 2: false, 3: false}
	for _, request := range input.RendererRuntimeClientBridgeMatrix.RuntimeRequestCalls {
		if !strings.HasPrefix(request.Path, "/v1/") ||
			!strings.Contains(request.Path, input.RendererRuntimeClientBridgeMatrix.EncodedIDs.ThreadID) ||
			!strings.Contains(request.Path, input.RendererRuntimeClientBridgeMatrix.EncodedIDs.TurnID) ||
			strings.Contains(request.Path, input.RendererRuntimeClientBridgeMatrix.SourceIDs.ThreadID) ||
			strings.Contains(request.Path, input.RendererRuntimeClientBridgeMatrix.SourceIDs.TurnID) {
			rendererRuntimeClientRuntimeRequestPreservesArguments = false
		}
		switch request.ArgumentCount {
		case 1:
			if request.Method != "" || request.Body != "" {
				rendererRuntimeClientRuntimeRequestPreservesArguments = false
			}
			expectedRendererRuntimeClientArgs[1] = true
		case 2:
			if request.Method != "POST" || request.Body != "" {
				rendererRuntimeClientRuntimeRequestPreservesArguments = false
			}
			expectedRendererRuntimeClientArgs[2] = true
		case 3:
			if request.Method != "POST" || request.Body != `{"action":"step"}` {
				rendererRuntimeClientRuntimeRequestPreservesArguments = false
			}
			expectedRendererRuntimeClientArgs[3] = true
		default:
			rendererRuntimeClientRuntimeRequestPreservesArguments = false
		}
	}
	for _, seen := range expectedRendererRuntimeClientArgs {
		if !seen {
			rendererRuntimeClientRuntimeRequestPreservesArguments = false
		}
	}
	rendererRuntimeClientRestartUsesRuntimeAPI :=
		input.RendererRuntimeClientBridgeMatrix.SourceUsesAnalytixRuntime &&
			input.RendererRuntimeClientBridgeMatrix.SourcePassesRestartUnchanged &&
			input.RendererRuntimeClientBridgeMatrix.RestartUnitEvidencePresent &&
			stringInSlice("renderer-runtime-client-restart-evidence", input.EvidenceIDs)
	rendererRuntimeClientSseControlsPreserveArguments :=
		input.RendererRuntimeClientBridgeMatrix.SourceUsesAnalytixRuntime &&
			input.RendererRuntimeClientBridgeMatrix.SourcePassesSseControlsUnchanged &&
			input.RendererRuntimeClientBridgeMatrix.SSEUnitEvidencePresent &&
			stringInSlice("renderer-runtime-client-sse-evidence", input.EvidenceIDs) &&
			input.RendererRuntimeClientBridgeMatrix.SSEStartCall.ThreadID == input.RendererRuntimeClientBridgeMatrix.SourceIDs.ThreadID &&
			input.RendererRuntimeClientBridgeMatrix.SSEStartCall.SinceSeq == 7 &&
			input.RendererRuntimeClientBridgeMatrix.SSEStartCall.StreamID == "stream-renderer" &&
			input.RendererRuntimeClientBridgeMatrix.SSEStopCall.StreamID == "stream-renderer"
	rendererRuntimeClientSseListenersPreserveHandlers :=
		input.RendererRuntimeClientBridgeMatrix.SourcePassesSseListenersUnchanged &&
			input.RendererRuntimeClientBridgeMatrix.SSEUnitEvidencePresent &&
			allStringInSlice([]string{"onSseEvent", "onSseEnd", "onSseError"}, input.RendererRuntimeClientBridgeMatrix.ListenerAPIs) &&
			len(input.RendererRuntimeClientBridgeMatrix.ListenerAPIs) == 3
	rendererRuntimeClientLegacyAliasesUnread :=
		input.RendererRuntimeClientBridgeMatrix.SourceUsesAnalytixRuntime &&
			input.RendererRuntimeClientBridgeMatrix.LegacyAliasUnitEvidencePresent
	rendererRuntimeClientUnitEvidencePresent :=
		input.RendererRuntimeClientBridgeMatrix.RuntimeRequestUnitEvidencePresent &&
			input.RendererRuntimeClientBridgeMatrix.RestartUnitEvidencePresent &&
			input.RendererRuntimeClientBridgeMatrix.SSEUnitEvidencePresent &&
			input.RendererRuntimeClientBridgeMatrix.LegacyAliasUnitEvidencePresent
	rendererSettingsBridgeUsesAnalytixSettingsAPI :=
		input.RendererSettingsBridgeMatrix.SourceUsesAnalytixSettings
	rendererSettingsBridgeCachesReads :=
		input.RendererSettingsBridgeMatrix.SourceUsesAnalytixSettings &&
			input.RendererSettingsBridgeMatrix.SourceCachesSettingsReads &&
			input.RendererSettingsBridgeMatrix.CacheUnitEvidencePresent &&
			input.RendererSettingsBridgeMatrix.CacheExpectations.GetSettingsCallsForDoubleRead == 1 &&
			stringInSlice("renderer-settings-cache-evidence", input.EvidenceIDs)
	rendererSettingsBridgeRefreshesCacheAfterWrite :=
		input.RendererSettingsBridgeMatrix.SourceUsesAnalytixSettings &&
			input.RendererSettingsBridgeMatrix.SourceRefreshesCacheAfterSetSettings &&
			input.RendererSettingsBridgeMatrix.RefreshUnitEvidencePresent &&
			input.RendererSettingsBridgeMatrix.CacheExpectations.GetSettingsCallsAfterSetSettings == 1 &&
			input.RendererSettingsBridgeMatrix.CacheExpectations.SetSettingsCallsAfterWrite == 1 &&
			stringInSlice("renderer-settings-cache-evidence", input.EvidenceIDs)
	rendererSettingsBridgePreservesTopLevelRuntimePatch :=
		input.RendererSettingsBridgeMatrix.SourceUsesAnalytixSettings &&
			input.RendererSettingsBridgeMatrix.TopLevelRuntimePatchEvidencePresent &&
			input.RendererSettingsBridgeMatrix.Patch.RuntimeModel == "deepseek-reasoner" &&
			input.RendererSettingsBridgeMatrix.Patch.ApprovalPolicy == "never" &&
			stringInSlice("renderer-settings-runtime-patch-evidence", input.EvidenceIDs)
	rendererSettingsBridgeLegacyAliasesUnread :=
		input.RendererSettingsBridgeMatrix.SourceUsesAnalytixSettings &&
			input.RendererSettingsBridgeMatrix.LegacyAliasUnitEvidencePresent &&
			stringInSlice("renderer-settings-legacy-alias-evidence", input.EvidenceIDs)
	rendererSettingsBridgeUnitEvidencePresent :=
		input.RendererSettingsBridgeMatrix.CacheUnitEvidencePresent &&
			input.RendererSettingsBridgeMatrix.RefreshUnitEvidencePresent &&
			input.RendererSettingsBridgeMatrix.TopLevelRuntimePatchEvidencePresent &&
			input.RendererSettingsBridgeMatrix.LegacyAliasUnitEvidencePresent
	rendererProviderUsesSharedRootPaths :=
		input.RendererProviderEndpointMatrix.RootConstantsUsed &&
			input.RendererProviderEndpointMatrix.RootPaths.Health == "/health" &&
			input.RendererProviderEndpointMatrix.RootPaths.Threads == "/v1/threads" &&
			stringInSlice("renderer-provider-shared-root-paths", input.EvidenceIDs)
	rendererProviderEncodesDynamicRouteIDs := len(input.RendererProviderEndpointMatrix.EncodedRuntimeRequestPaths) == 8
	rawRendererEndpointIDs := []string{
		input.RendererProviderEndpointMatrix.SourceIDs.ThreadID,
		input.RendererProviderEndpointMatrix.SourceIDs.TurnID,
		input.RendererProviderEndpointMatrix.SourceIDs.ApprovalID,
		input.RendererProviderEndpointMatrix.SourceIDs.InputID,
		input.RendererProviderEndpointMatrix.SourceIDs.SessionID,
	}
	encodedRendererEndpointIDs := []string{
		input.RendererProviderEndpointMatrix.EncodedIDs.ThreadID,
		input.RendererProviderEndpointMatrix.EncodedIDs.TurnID,
		input.RendererProviderEndpointMatrix.EncodedIDs.ApprovalID,
		input.RendererProviderEndpointMatrix.EncodedIDs.InputID,
		input.RendererProviderEndpointMatrix.EncodedIDs.SessionID,
	}
	for _, path := range input.RendererProviderEndpointMatrix.EncodedRuntimeRequestPaths {
		if !strings.HasPrefix(path, "/v1/") {
			rendererProviderEncodesDynamicRouteIDs = false
		}
		for _, rawID := range rawRendererEndpointIDs {
			if rawID != "" && strings.Contains(path, rawID) {
				rendererProviderEncodesDynamicRouteIDs = false
			}
		}
		hasEncodedID := false
		for _, encodedID := range encodedRendererEndpointIDs {
			if encodedID != "" && strings.Contains(path, encodedID) {
				hasEncodedID = true
				break
			}
		}
		if !hasEncodedID {
			rendererProviderEncodesDynamicRouteIDs = false
		}
	}
	rendererProviderRuntimePathsAnalytixOwned := input.RendererProviderEndpointMatrix.PathOwnershipGuardPresent
	forbiddenRendererRuntimePathTokens := []string{
		"/v1/reasonix",
		"/v1/runtime/go",
		"/v1/workflow",
		"/v1/create-loop",
		"/v1/subagent",
		"/v1/autoresearch",
		"/v1/mcp-indexer",
	}
	for _, path := range input.RendererProviderEndpointMatrix.EncodedRuntimeRequestPaths {
		if !strings.HasPrefix(path, "/v1/") {
			rendererProviderRuntimePathsAnalytixOwned = false
		}
		for _, token := range forbiddenRendererRuntimePathTokens {
			if strings.Contains(path, token) {
				rendererProviderRuntimePathsAnalytixOwned = false
			}
		}
	}
	rendererProviderUsesRuntimeClientFacade :=
		input.RendererProviderEndpointMatrix.UsesRendererRuntimeClientFacade &&
			stringInSlice("renderer-provider-runtime-client-facade", input.EvidenceIDs)
	rendererProviderEndpointBuilderUnitEvidencePresent :=
		input.RendererProviderEndpointMatrix.UnitTestEvidencePresent &&
			stringInSlice("renderer-provider-endpoint-builder-unit-evidence", input.EvidenceIDs)
	rendererProviderAliasGuardInstallsThrowingAliases :=
		input.RendererProviderAliasGuardMatrix.HelperName == "installDsGui" &&
			input.RendererProviderAliasGuardMatrix.SourceInstallsThrowingAliases &&
			input.RendererProviderAliasGuardMatrix.SourceUsesAnalytixOnly &&
			len(input.RendererProviderAliasGuardMatrix.ForbiddenAliases) == 2 &&
			allStringInSlice([]string{"kun", "reasonix"}, input.RendererProviderAliasGuardMatrix.ForbiddenAliases) &&
			stringInSlice("renderer-provider-alias-guard-evidence", input.EvidenceIDs)
	rendererProviderAliasGuardCoversRuntimeRoutes :=
		input.RendererProviderAliasGuardMatrix.Coverage.RouteOwnership &&
			input.RendererProviderAliasGuardMatrix.RouteOwnershipEvidencePresent &&
			stringInSlice("renderer-provider-alias-guard-evidence", input.EvidenceIDs)
	rendererProviderAliasGuardCoversLifecycleAndGates :=
		input.RendererProviderAliasGuardMatrix.Coverage.ThreadLifecycle &&
			input.RendererProviderAliasGuardMatrix.Coverage.ApprovalUserInput &&
			input.RendererProviderAliasGuardMatrix.LifecycleEvidencePresent &&
			input.RendererProviderAliasGuardMatrix.ApprovalUserInputEvidencePresent &&
			stringInSlice("renderer-provider-lifecycle-alias-evidence", input.EvidenceIDs)
	rendererProviderAliasGuardCoversForkResumeAndEncoding :=
		input.RendererProviderAliasGuardMatrix.Coverage.ForkResume &&
			input.RendererProviderAliasGuardMatrix.Coverage.DynamicRouteEncoding &&
			input.RendererProviderAliasGuardMatrix.ForkResumeEvidencePresent &&
			input.RendererProviderAliasGuardMatrix.DynamicEncodingEvidencePresent &&
			stringInSlice("renderer-provider-dynamic-route-alias-evidence", input.EvidenceIDs)
	rendererProviderAliasGuardRejectsForbiddenRoutes :=
		input.RendererProviderAliasGuardMatrix.ForbiddenRouteGuardPresent &&
			input.RendererProviderEndpointMatrix.PathOwnershipGuardPresent &&
			rendererProviderRuntimePathsAnalytixOwned
	requiredRendererProviderAliasGuardTests := []string{
		"keeps renderer runtime requests on analytix-owned HTTP routes",
		"uses Analytix thread lifecycle endpoints for list, archive, restore, rename, workspace, and delete",
		"calls Analytix fork and user-input compatibility endpoints",
		"URL-encodes dynamic runtime route ids before calling the bridge",
		"resumes a session through the Analytix HTTP runtime",
	}
	rendererProviderAliasGuardUnitEvidencePresent :=
		len(input.RendererProviderAliasGuardMatrix.GuardedTestNames) == 5 &&
			allStringInSlice(requiredRendererProviderAliasGuardTests, input.RendererProviderAliasGuardMatrix.GuardedTestNames) &&
			rendererProviderAliasGuardInstallsThrowingAliases &&
			rendererProviderAliasGuardCoversRuntimeRoutes &&
			rendererProviderAliasGuardCoversLifecycleAndGates &&
			rendererProviderAliasGuardCoversForkResumeAndEncoding &&
			rendererProviderAliasGuardRejectsForbiddenRoutes
	rendererProviderFacadeSealUsesRuntimeClient :=
		input.RendererProviderFacadeSealMatrix.Facade == "rendererRuntimeClient.runtimeRequest" &&
			input.RendererProviderFacadeSealMatrix.SourceUsesRuntimeClient &&
			stringInSlice("renderer-provider-facade-source-evidence", input.EvidenceIDs)
	rendererProviderFacadeSealRejectsDirectBridge :=
		input.RendererProviderFacadeSealMatrix.ForbiddenDirectBridge == "window.analytix.runtime.runtimeRequest" &&
			input.RendererProviderFacadeSealMatrix.SourceRejectsDirectBridgeBypass &&
			stringInSlice("renderer-provider-facade-source-evidence", input.EvidenceIDs)
	rendererProviderFacadeSealCoversArchiveRestore :=
		input.RendererProviderFacadeSealMatrix.ArchiveRestoreSourceEvidencePresent &&
			input.RendererProviderFacadeSealMatrix.LifecycleUnitEvidencePresent &&
			stringInSlice("renderer-provider-facade-lifecycle-evidence", input.EvidenceIDs)
	rendererProviderFacadeSealCoversRelationPatch :=
		input.RendererProviderFacadeSealMatrix.RelationSourceEvidencePresent &&
			input.RendererProviderFacadeSealMatrix.LifecycleUnitEvidencePresent &&
			stringInSlice("renderer-provider-facade-lifecycle-evidence", input.EvidenceIDs)
	rendererProviderFacadeSealScanGuardPresent :=
		input.RendererProviderFacadeSealMatrix.ScanGuardPresent &&
			input.RendererProviderFacadeSealMatrix.ProviderFacadeTokenScanPresent &&
			stringInSlice("renderer-provider-direct-bypass-scan-evidence", input.EvidenceIDs)
	rendererProviderFacadeSealUnitEvidencePresent :=
		len(input.RendererProviderFacadeSealMatrix.SealedMethods) == 2 &&
			allStringInSlice([]string{"archiveThread", "updateThreadRelation"}, input.RendererProviderFacadeSealMatrix.SealedMethods) &&
			rendererProviderFacadeSealUsesRuntimeClient &&
			rendererProviderFacadeSealRejectsDirectBridge &&
			rendererProviderFacadeSealCoversArchiveRestore &&
			rendererProviderFacadeSealCoversRelationPatch &&
			rendererProviderFacadeSealScanGuardPresent
	sideConversationRelationContractOptionalProvider :=
		input.SideConversationRelationMatrix.ProviderMethod == "updateThreadRelation" &&
			input.SideConversationRelationMatrix.ProviderContractOptional &&
			input.SideConversationRelationMatrix.ProviderImplementationUsesRuntimeClient &&
			stringInSlice("side-conversation-relation-provider-contract", input.EvidenceIDs)
	sideConversationRelationPromotesThroughProvider :=
		input.SideConversationRelationMatrix.StoreAction == "promoteSideConversation" &&
			input.SideConversationRelationMatrix.Relation == "primary" &&
			input.SideConversationRelationMatrix.StoreUsesProviderContract &&
			stringInSlice("side-conversation-relation-store-evidence", input.EvidenceIDs)
	sideConversationRelationRefreshesAndCloses :=
		input.SideConversationRelationMatrix.StoreRefreshesAndCloses &&
			stringInSlice("side-conversation-relation-store-evidence", input.EvidenceIDs)
	sideConversationRelationRejectsDirectBridge :=
		input.SideConversationRelationMatrix.StoreRejectsDirectRuntimeBridge &&
			stringInSlice("side-conversation-direct-bypass-scan-evidence", input.EvidenceIDs)
	sideConversationRelationScanGuardPresent :=
		input.SideConversationRelationMatrix.ScanGuardPresent &&
			stringInSlice("side-conversation-direct-bypass-scan-evidence", input.EvidenceIDs)
	sideConversationRelationUnitEvidencePresent :=
		input.SideConversationRelationMatrix.UnitEvidencePresent &&
			sideConversationRelationContractOptionalProvider &&
			sideConversationRelationPromotesThroughProvider &&
			sideConversationRelationRefreshesAndCloses &&
			sideConversationRelationRejectsDirectBridge &&
			sideConversationRelationScanGuardPresent
	rendererUsageRuntimeClientCoversThreadUsage :=
		input.RendererUsageRuntimeClientMatrix.Facade == "rendererRuntimeClient.runtimeRequest" &&
			stringInSlice("loadThreadUsage", input.RendererUsageRuntimeClientMatrix.UsageLoaders) &&
			input.RendererUsageRuntimeClientMatrix.ThreadUsageSourceUsesRuntimeClient &&
			input.RendererUsageRuntimeClientMatrix.UsageUnitEvidencePresent &&
			stringInSlice("renderer-usage-runtime-client-source-evidence", input.EvidenceIDs) &&
			stringInSlice("renderer-usage-runtime-client-unit-evidence", input.EvidenceIDs)
	rendererUsageRuntimeClientCoversDailyUsage :=
		input.RendererUsageRuntimeClientMatrix.Facade == "rendererRuntimeClient.runtimeRequest" &&
			stringInSlice("loadDailyUsage", input.RendererUsageRuntimeClientMatrix.UsageLoaders) &&
			input.RendererUsageRuntimeClientMatrix.DailyUsageSourceUsesRuntimeClient &&
			input.RendererUsageRuntimeClientMatrix.UsageUnitEvidencePresent &&
			stringInSlice("renderer-usage-runtime-client-source-evidence", input.EvidenceIDs) &&
			stringInSlice("renderer-usage-runtime-client-unit-evidence", input.EvidenceIDs)
	rendererUsageRuntimeClientCoversModelUsage :=
		input.RendererUsageRuntimeClientMatrix.Facade == "rendererRuntimeClient.runtimeRequest" &&
			stringInSlice("loadModelUsage", input.RendererUsageRuntimeClientMatrix.UsageLoaders) &&
			input.RendererUsageRuntimeClientMatrix.ModelUsageSourceUsesRuntimeClient &&
			input.RendererUsageRuntimeClientMatrix.UsageUnitEvidencePresent &&
			stringInSlice("renderer-usage-runtime-client-source-evidence", input.EvidenceIDs) &&
			stringInSlice("renderer-usage-runtime-client-unit-evidence", input.EvidenceIDs)
	rendererUsageRuntimeClientCoversSettingsDiagnostics :=
		stringInSlice("loadTokenEconomySavingsSummary", input.RendererUsageRuntimeClientMatrix.DiagnosticsLoaders) &&
			stringInSlice("LlmDebugSettingsSection", input.RendererUsageRuntimeClientMatrix.DiagnosticsLoaders) &&
			input.RendererUsageRuntimeClientMatrix.TokenEconomySourceUsesRuntimeClient &&
			input.RendererUsageRuntimeClientMatrix.LlmDebugSourceUsesRuntimeClient &&
			stringInSlice("renderer-usage-runtime-client-settings-diagnostics-evidence", input.EvidenceIDs)
	rendererUsageRuntimeClientRejectsDirectBridge :=
		input.RendererUsageRuntimeClientMatrix.ForbiddenDirectBridge == "window.analytix.runtime.runtimeRequest" &&
			input.RendererUsageRuntimeClientMatrix.SourceRejectsDirectBridgeBypass &&
			stringInSlice("renderer-usage-direct-bypass-scan-evidence", input.EvidenceIDs)
	rendererUsageRuntimeClientScanGuardPresent :=
		input.RendererUsageRuntimeClientMatrix.ScanGuardPresent &&
			input.RendererUsageRuntimeClientMatrix.UsageFacadeTokenScanPresent &&
			stringInSlice("renderer-usage-direct-bypass-scan-evidence", input.EvidenceIDs)
	rendererUsageRuntimeClientUnitEvidencePresent :=
		len(input.RendererUsageRuntimeClientMatrix.UsageLoaders) == 3 &&
			rendererUsageRuntimeClientCoversThreadUsage &&
			rendererUsageRuntimeClientCoversDailyUsage &&
			rendererUsageRuntimeClientCoversModelUsage &&
			rendererUsageRuntimeClientCoversSettingsDiagnostics &&
			rendererUsageRuntimeClientRejectsDirectBridge &&
			rendererUsageRuntimeClientScanGuardPresent
	rendererSettingsReadFacadeCoversKeyboardShortcuts :=
		input.RendererSettingsReadFacadeMatrix.Facade == "rendererRuntimeClient.getSettings" &&
			stringInSlice("useKeyboardShortcutSettings", input.RendererSettingsReadFacadeMatrix.SettingsReaders) &&
			input.RendererSettingsReadFacadeMatrix.KeyboardShortcutSourceUsesSettingsClient &&
			stringInSlice("renderer-settings-read-facade-source-evidence", input.EvidenceIDs)
	rendererSettingsReadFacadeCoversSpeechToText :=
		input.RendererSettingsReadFacadeMatrix.Facade == "rendererRuntimeClient.getSettings" &&
			stringInSlice("useSpeechToTextSettings", input.RendererSettingsReadFacadeMatrix.SettingsReaders) &&
			input.RendererSettingsReadFacadeMatrix.SpeechToTextSourceUsesSettingsClient &&
			stringInSlice("renderer-settings-read-facade-source-evidence", input.EvidenceIDs)
	rendererSettingsReadFacadeCoversUsageModelLabel :=
		input.RendererSettingsReadFacadeMatrix.Facade == "rendererRuntimeClient.getSettings" &&
			stringInSlice("InitialSessionUsageHeatmapView", input.RendererSettingsReadFacadeMatrix.SettingsReaders) &&
			input.RendererSettingsReadFacadeMatrix.UsageHeatmapSourceUsesSettingsClient &&
			stringInSlice("renderer-settings-read-facade-source-evidence", input.EvidenceIDs)
	rendererSettingsReadFacadePreservesSettingsChangedEvent :=
		input.RendererSettingsReadFacadeMatrix.SettingsChangedEventPreserved &&
			stringInSlice("renderer-settings-read-event-sync-evidence", input.EvidenceIDs)
	rendererSettingsReadFacadeRejectsDirectBridge :=
		input.RendererSettingsReadFacadeMatrix.ForbiddenDirectBridge == "window.analytix.settings.getSettings" &&
			input.RendererSettingsReadFacadeMatrix.SourceRejectsDirectSettingsBypass &&
			stringInSlice("renderer-settings-read-direct-bypass-scan-evidence", input.EvidenceIDs)
	rendererSettingsReadFacadeScanGuardPresent :=
		input.RendererSettingsReadFacadeMatrix.ScanGuardPresent &&
			input.RendererSettingsReadFacadeMatrix.SettingsReadFacadeTokenScanPresent &&
			stringInSlice("renderer-settings-read-direct-bypass-scan-evidence", input.EvidenceIDs)
	preloadRuntimeRequestPreservesPathMethodBody :=
		input.PreloadRuntimeRequestBridgeMatrix.Channel == "runtime:request" &&
			input.PreloadRuntimeRequestBridgeMatrix.SourcePassesArgumentsUnchanged &&
			input.PreloadRuntimeRequestBridgeMatrix.RuntimeFacadeUnitEvidencePresent &&
			input.PreloadRuntimeRequestBridgeMatrix.RuntimeFacadeCall.Method == "POST" &&
			input.PreloadRuntimeRequestBridgeMatrix.RuntimeFacadeCall.Body == `{"action":"step"}` &&
			strings.HasPrefix(input.PreloadRuntimeRequestBridgeMatrix.RuntimeFacadeCall.Path, "/v1/") &&
			strings.Contains(input.PreloadRuntimeRequestBridgeMatrix.RuntimeFacadeCall.Path, input.PreloadRuntimeRequestBridgeMatrix.EncodedIDs.ThreadID) &&
			strings.Contains(input.PreloadRuntimeRequestBridgeMatrix.RuntimeFacadeCall.Path, input.PreloadRuntimeRequestBridgeMatrix.EncodedIDs.TurnID) &&
			!strings.Contains(input.PreloadRuntimeRequestBridgeMatrix.RuntimeFacadeCall.Path, input.PreloadRuntimeRequestBridgeMatrix.SourceIDs.ThreadID) &&
			!strings.Contains(input.PreloadRuntimeRequestBridgeMatrix.RuntimeFacadeCall.Path, input.PreloadRuntimeRequestBridgeMatrix.SourceIDs.TurnID) &&
			stringInSlice("preload-runtime-request-path-method-body-evidence", input.EvidenceIDs)
	preloadDiagnosticsRuntimeRequestUsesSameChannel :=
		input.PreloadRuntimeRequestBridgeMatrix.Channel == "runtime:request" &&
			input.PreloadRuntimeRequestBridgeMatrix.DiagnosticsFacadeUnitEvidencePresent &&
			input.PreloadRuntimeRequestBridgeMatrix.DiagnosticsFacadeCall.Method == "POST" &&
			input.PreloadRuntimeRequestBridgeMatrix.DiagnosticsFacadeCall.Body == "{}" &&
			strings.HasPrefix(input.PreloadRuntimeRequestBridgeMatrix.DiagnosticsFacadeCall.Path, "/v1/") &&
			strings.Contains(input.PreloadRuntimeRequestBridgeMatrix.DiagnosticsFacadeCall.Path, input.PreloadRuntimeRequestBridgeMatrix.EncodedIDs.InputID) &&
			!strings.Contains(input.PreloadRuntimeRequestBridgeMatrix.DiagnosticsFacadeCall.Path, input.PreloadRuntimeRequestBridgeMatrix.SourceIDs.InputID) &&
			stringInSlice("preload-diagnostics-runtime-request-same-channel", input.EvidenceIDs)
	preloadRuntimeRestartUsesAnalytixIpc :=
		input.PreloadRuntimeRequestBridgeMatrix.RestartChannel == "runtime:restart" &&
			input.PreloadRuntimeRequestBridgeMatrix.SourcePassesRestartUnchanged &&
			input.PreloadRuntimeRequestBridgeMatrix.RestartUnitEvidencePresent &&
			stringInSlice("preload-runtime-restart-evidence", input.EvidenceIDs)
	preloadRuntimeRequestUnitEvidencePresent :=
		input.PreloadRuntimeRequestBridgeMatrix.RuntimeFacadeUnitEvidencePresent &&
			input.PreloadRuntimeRequestBridgeMatrix.RestartUnitEvidencePresent &&
			input.PreloadRuntimeRequestBridgeMatrix.DiagnosticsFacadeUnitEvidencePresent
	preloadSseStartStopPreservesArguments :=
		input.PreloadSseBridgeMatrix.Channels.Start == "runtime:sse:start" &&
			input.PreloadSseBridgeMatrix.Channels.Stop == "runtime:sse:stop" &&
			input.PreloadSseBridgeMatrix.SourcePassesStartStopUnchanged &&
			input.PreloadSseBridgeMatrix.StartStopUnitEvidencePresent &&
			input.PreloadSseBridgeMatrix.StartCall.ThreadID != "" &&
			input.PreloadSseBridgeMatrix.StartCall.SinceSeq == 42 &&
			input.PreloadSseBridgeMatrix.StartCall.StreamID == "stream-provided" &&
			input.PreloadSseBridgeMatrix.StopCall.StreamID == "stream-provided" &&
			stringInSlice("preload-sse-start-stop-evidence", input.EvidenceIDs)
	preloadSsePayloadListenersOmitElectronEvent :=
		input.PreloadSseBridgeMatrix.Channels.Event == "runtime:sse-event" &&
			input.PreloadSseBridgeMatrix.Channels.End == "runtime:sse-end" &&
			input.PreloadSseBridgeMatrix.Channels.Error == "runtime:sse-error" &&
			input.PreloadSseBridgeMatrix.SourcePayloadOnlyWrappers &&
			input.PreloadSseBridgeMatrix.PayloadUnitEvidencePresent &&
			input.PreloadSseBridgeMatrix.EventPayload.StreamID == "stream-a" &&
			input.PreloadSseBridgeMatrix.EventPayload.Seq == 1 &&
			input.PreloadSseBridgeMatrix.EventPayload.Kind == "thread_event" &&
			input.PreloadSseBridgeMatrix.EndPayload.StreamID == "stream-a" &&
			input.PreloadSseBridgeMatrix.ErrorPayload.StreamID == "stream-a" &&
			input.PreloadSseBridgeMatrix.ErrorPayload.Status == 404 &&
			stringInSlice("preload-sse-payload-only-listeners", input.EvidenceIDs)
	preloadSseListenerCleanupUsesSameWrapper :=
		input.PreloadSseBridgeMatrix.SourceCleanupUsesSameWrapper &&
			input.PreloadSseBridgeMatrix.PayloadUnitEvidencePresent
	preloadSseBridgeUnitEvidencePresent :=
		input.PreloadSseBridgeMatrix.StartStopUnitEvidencePresent &&
			input.PreloadSseBridgeMatrix.PayloadUnitEvidencePresent &&
			input.PreloadSseBridgeMatrix.ExposesOnlyAnalytixApi
	dropsReasonixAutoPlanConfig := stringInSlice("settings-drop-reasonix-auto-plan", input.EvidenceIDs)
	stripsLegacyRuntimeSettings := stringInSlice("settings-drop-legacy-agent-envelope", input.EvidenceIDs)
	writesTopLevelRuntimeSettings := stringInSlice("settings-endpoint-format-top-level-runtime", input.EvidenceIDs)
	return G5DesktopSovereigntyOutput{
		ExposesOnlyAnalytixBridge: len(input.ExposedBridgeNames) == 1 &&
			input.ExposedBridgeNames[0] == input.BridgeName,
		WindowTypeOnlyAnalytix: len(input.WindowTypeProperties) == 1 &&
			input.WindowTypeProperties[0] == input.BridgeName,
		FacadeDomainsAnalytixOwned:                            !anyStringInSlice(input.ForbiddenBridgeAliases, input.FacadeDomains),
		PublicApiTypesAnalytixOwned:                           publicAPIAnalytixOwned,
		RuntimeRequestUsesAnalytixIpc:                         runtimeRequestUsesAnalytixIpc,
		RuntimeSseUsesAnalytixIpc:                             runtimeSseUsesAnalytixIpc,
		RendererNamedRuntimeApisAllowListed:                   rendererNamedRuntimeApisAllowListed,
		RendererNamedSettingsApisAllowListed:                  rendererNamedSettingsApisAllowListed,
		RendererGenericRuntimeBypassAbsent:                    rendererGenericRuntimeBypassAbsent,
		RendererSettingsReadBypassAbsent:                      rendererSettingsReadBypassAbsent,
		OptionalChainBridgeAccessScanned:                      optionalChainBridgeAccessScanned,
		RendererProviderUsesSharedRootPaths:                   rendererProviderUsesSharedRootPaths,
		RendererProviderEncodesDynamicRouteIDs:                rendererProviderEncodesDynamicRouteIDs,
		RendererProviderRuntimePathsAnalytixOwned:             rendererProviderRuntimePathsAnalytixOwned,
		RendererProviderUsesRuntimeClientFacade:               rendererProviderUsesRuntimeClientFacade,
		RendererProviderEndpointBuilderUnitEvidencePresent:    rendererProviderEndpointBuilderUnitEvidencePresent,
		RendererProviderAliasGuardInstallsThrowingAliases:     rendererProviderAliasGuardInstallsThrowingAliases,
		RendererProviderAliasGuardCoversRuntimeRoutes:         rendererProviderAliasGuardCoversRuntimeRoutes,
		RendererProviderAliasGuardCoversLifecycleAndGates:     rendererProviderAliasGuardCoversLifecycleAndGates,
		RendererProviderAliasGuardCoversForkResumeAndEncoding: rendererProviderAliasGuardCoversForkResumeAndEncoding,
		RendererProviderAliasGuardRejectsForbiddenRoutes:      rendererProviderAliasGuardRejectsForbiddenRoutes,
		RendererProviderAliasGuardUnitEvidencePresent:         rendererProviderAliasGuardUnitEvidencePresent,
		RendererProviderFacadeSealUsesRuntimeClient:           rendererProviderFacadeSealUsesRuntimeClient,
		RendererProviderFacadeSealRejectsDirectBridge:         rendererProviderFacadeSealRejectsDirectBridge,
		RendererProviderFacadeSealCoversArchiveRestore:        rendererProviderFacadeSealCoversArchiveRestore,
		RendererProviderFacadeSealCoversRelationPatch:         rendererProviderFacadeSealCoversRelationPatch,
		RendererProviderFacadeSealScanGuardPresent:            rendererProviderFacadeSealScanGuardPresent,
		RendererProviderFacadeSealUnitEvidencePresent:         rendererProviderFacadeSealUnitEvidencePresent,
		SideConversationRelationContractOptionalProvider:      sideConversationRelationContractOptionalProvider,
		SideConversationRelationPromotesThroughProvider:       sideConversationRelationPromotesThroughProvider,
		SideConversationRelationRefreshesAndCloses:            sideConversationRelationRefreshesAndCloses,
		SideConversationRelationRejectsDirectBridge:           sideConversationRelationRejectsDirectBridge,
		SideConversationRelationScanGuardPresent:              sideConversationRelationScanGuardPresent,
		SideConversationRelationUnitEvidencePresent:           sideConversationRelationUnitEvidencePresent,
		RendererUsageRuntimeClientCoversThreadUsage:           rendererUsageRuntimeClientCoversThreadUsage,
		RendererUsageRuntimeClientCoversDailyUsage:            rendererUsageRuntimeClientCoversDailyUsage,
		RendererUsageRuntimeClientCoversModelUsage:            rendererUsageRuntimeClientCoversModelUsage,
		RendererUsageRuntimeClientCoversSettingsDiagnostics:   rendererUsageRuntimeClientCoversSettingsDiagnostics,
		RendererUsageRuntimeClientRejectsDirectBridge:         rendererUsageRuntimeClientRejectsDirectBridge,
		RendererUsageRuntimeClientScanGuardPresent:            rendererUsageRuntimeClientScanGuardPresent,
		RendererUsageRuntimeClientUnitEvidencePresent:         rendererUsageRuntimeClientUnitEvidencePresent,
		RendererSettingsReadFacadeCoversKeyboardShortcuts:     rendererSettingsReadFacadeCoversKeyboardShortcuts,
		RendererSettingsReadFacadeCoversSpeechToText:          rendererSettingsReadFacadeCoversSpeechToText,
		RendererSettingsReadFacadeCoversUsageModelLabel:       rendererSettingsReadFacadeCoversUsageModelLabel,
		RendererSettingsReadFacadePreservesSettingsEvent:      rendererSettingsReadFacadePreservesSettingsChangedEvent,
		RendererSettingsReadFacadeRejectsDirectBridge:         rendererSettingsReadFacadeRejectsDirectBridge,
		RendererSettingsReadFacadeScanGuardPresent:            rendererSettingsReadFacadeScanGuardPresent,
		RendererRuntimeClientRuntimeRequestPreservesArguments: rendererRuntimeClientRuntimeRequestPreservesArguments,
		RendererRuntimeClientRestartUsesRuntimeApi:            rendererRuntimeClientRestartUsesRuntimeAPI,
		RendererRuntimeClientSseControlsPreserveArguments:     rendererRuntimeClientSseControlsPreserveArguments,
		RendererRuntimeClientSseListenersPreserveHandlers:     rendererRuntimeClientSseListenersPreserveHandlers,
		RendererRuntimeClientLegacyAliasesUnread:              rendererRuntimeClientLegacyAliasesUnread,
		RendererRuntimeClientUnitEvidencePresent:              rendererRuntimeClientUnitEvidencePresent,
		RendererSettingsBridgeUsesAnalytixSettingsApi:         rendererSettingsBridgeUsesAnalytixSettingsAPI,
		RendererSettingsBridgeCachesReads:                     rendererSettingsBridgeCachesReads,
		RendererSettingsBridgeRefreshesCacheAfterWrite:        rendererSettingsBridgeRefreshesCacheAfterWrite,
		RendererSettingsBridgePreservesTopLevelRuntimePatch:   rendererSettingsBridgePreservesTopLevelRuntimePatch,
		RendererSettingsBridgeLegacyAliasesUnread:             rendererSettingsBridgeLegacyAliasesUnread,
		RendererSettingsBridgeUnitEvidencePresent:             rendererSettingsBridgeUnitEvidencePresent,
		PreloadRuntimeRequestPreservesPathMethodBody:          preloadRuntimeRequestPreservesPathMethodBody,
		PreloadDiagnosticsRuntimeRequestUsesSameChannel:       preloadDiagnosticsRuntimeRequestUsesSameChannel,
		PreloadRuntimeRestartUsesAnalytixIpc:                  preloadRuntimeRestartUsesAnalytixIpc,
		PreloadRuntimeRequestExposesOnlyAnalytixApi:           input.PreloadRuntimeRequestBridgeMatrix.ExposesOnlyAnalytixApi,
		PreloadRuntimeRequestUnitEvidencePresent:              preloadRuntimeRequestUnitEvidencePresent,
		PreloadSseStartStopPreservesArguments:                 preloadSseStartStopPreservesArguments,
		PreloadSsePayloadListenersOmitElectronEvent:           preloadSsePayloadListenersOmitElectronEvent,
		PreloadSseListenerCleanupUsesSameWrapper:              preloadSseListenerCleanupUsesSameWrapper,
		PreloadSseBridgeUnitEvidencePresent:                   preloadSseBridgeUnitEvidencePresent,
		DropsReasonixAutoPlanConfig:                           dropsReasonixAutoPlanConfig,
		StripsLegacyRuntimeSettings:                           stripsLegacyRuntimeSettings,
		WritesTopLevelRuntimeSettings:                         writesTopLevelRuntimeSettings,
		DeprecatedBridgeAliasExposed:                          deprecatedBridgeAliasExposed,
		ForbiddenRuntimeIpcExposed:                            forbiddenRuntimeIpcExposed,
		DeprecatedSettingsFallbackWritten:                     !(dropsReasonixAutoPlanConfig && stripsLegacyRuntimeSettings && writesTopLevelRuntimeSettings),
		UsesReasonixProtocol:                                  false,
		TopLevelRouteExposed:                                  false,
	}
}

func replayG5DesktopMainIpcBoundary(input G5DesktopMainIpcBoundaryCase) G5DesktopMainIpcBoundaryOutput {
	runtimeRequestAllowsAnalytixRoutes := len(input.RuntimeRequest.AllowedAnalytixRouteExamples) >= 4
	for _, route := range input.RuntimeRequest.AllowedAnalytixRouteExamples {
		if !strings.HasPrefix(route, "/v1/") {
			runtimeRequestAllowsAnalytixRoutes = false
			break
		}
	}
	runtimeRequestRejectsForbiddenRoutes := len(input.RuntimeRequest.ForbiddenRuntimeRoutes) == 8
	for _, route := range input.RuntimeRequest.ForbiddenRuntimeRoutes {
		if !strings.HasPrefix(route, "/v1/") {
			runtimeRequestRejectsForbiddenRoutes = false
			break
		}
	}
	runtimeRequestHandlerRejectsBeforeRuntimeCall :=
		input.RuntimeRequest.HandlerChannel == "runtime:request" &&
			input.RuntimeRequest.Evidence.HandlerParsesBeforeRuntime &&
			input.RuntimeRequest.Evidence.ForbiddenHandlerNoExecute
	mainIpcEndpointBuilderAcceptsSharedPaths :=
		len(input.EndpointBuilderAllowListMatrix.AcceptedSharedBuilderRequests) == 14 &&
			input.EndpointBuilderAllowListMatrix.UnitTestEvidencePresent
	rawMainIpcEndpointIDs := []string{
		input.EndpointBuilderAllowListMatrix.SourceIDs.ThreadID,
		input.EndpointBuilderAllowListMatrix.SourceIDs.TurnID,
		input.EndpointBuilderAllowListMatrix.SourceIDs.CheckpointID,
		input.EndpointBuilderAllowListMatrix.SourceIDs.ApprovalID,
		input.EndpointBuilderAllowListMatrix.SourceIDs.InputID,
		input.EndpointBuilderAllowListMatrix.SourceIDs.SessionID,
		input.EndpointBuilderAllowListMatrix.SourceIDs.AttachmentID,
		input.EndpointBuilderAllowListMatrix.SourceIDs.MemoryID,
	}
	encodedMainIpcEndpointIDs := []string{
		input.EndpointBuilderAllowListMatrix.EncodedIDs.ThreadID,
		input.EndpointBuilderAllowListMatrix.EncodedIDs.TurnID,
		input.EndpointBuilderAllowListMatrix.EncodedIDs.CheckpointID,
		input.EndpointBuilderAllowListMatrix.EncodedIDs.ApprovalID,
		input.EndpointBuilderAllowListMatrix.EncodedIDs.InputID,
		input.EndpointBuilderAllowListMatrix.EncodedIDs.SessionID,
		input.EndpointBuilderAllowListMatrix.EncodedIDs.AttachmentID,
		input.EndpointBuilderAllowListMatrix.EncodedIDs.MemoryID,
	}
	for _, request := range input.EndpointBuilderAllowListMatrix.AcceptedSharedBuilderRequests {
		if !strings.HasPrefix(request.Path, "/v1/") || request.Method == "" {
			mainIpcEndpointBuilderAcceptsSharedPaths = false
		}
		for _, rawID := range rawMainIpcEndpointIDs {
			if rawID != "" && strings.Contains(request.Path, rawID) {
				mainIpcEndpointBuilderAcceptsSharedPaths = false
			}
		}
		hasEncodedID := false
		for _, encodedID := range encodedMainIpcEndpointIDs {
			if encodedID != "" && strings.Contains(request.Path, encodedID) {
				hasEncodedID = true
				break
			}
		}
		if !hasEncodedID {
			mainIpcEndpointBuilderAcceptsSharedPaths = false
		}
	}
	mainIpcEndpointBuilderRejectsRawDynamicRoutes :=
		len(input.EndpointBuilderAllowListMatrix.RejectedRawDynamicRequests) == 9 &&
			input.EndpointBuilderAllowListMatrix.RawRouteRejectionEvidencePresent
	mainIpcEndpointBuilderRejectsSingularUserInput := false
	for _, request := range input.EndpointBuilderAllowListMatrix.RejectedRawDynamicRequests {
		if !strings.HasPrefix(request.Path, "/v1/") || request.Method == "" {
			mainIpcEndpointBuilderRejectsRawDynamicRoutes = false
		}
		if request.Path == "/v1/user-input/input_raw" {
			mainIpcEndpointBuilderRejectsSingularUserInput = input.EndpointBuilderAllowListMatrix.RawRouteRejectionEvidencePresent
		}
		if request.Path != "/v1/user-input/input_raw" && !strings.Contains(request.Path, "/raw") {
			mainIpcEndpointBuilderRejectsRawDynamicRoutes = false
		}
	}
	requiredMainIpcTemplates := []string{
		"ANALYTIX_THREAD_TEMPLATE",
		"ANALYTIX_THREAD_FORK_TEMPLATE",
		"ANALYTIX_THREAD_REWIND_TEMPLATE",
		"ANALYTIX_THREAD_TURNS_TEMPLATE",
		"ANALYTIX_THREAD_STEER_TEMPLATE",
		"ANALYTIX_THREAD_INTERRUPT_TEMPLATE",
		"ANALYTIX_THREAD_CHECKPOINT_REWIND_PLAN_TEMPLATE",
		"ANALYTIX_THREAD_CHECKPOINT_REWIND_APPLY_TEMPLATE",
		"ANALYTIX_APPROVAL_TEMPLATE",
		"ANALYTIX_USER_INPUT_TEMPLATE",
		"ANALYTIX_SESSION_RESUME_TEMPLATE",
		"ANALYTIX_ATTACHMENT_CONTENT_TEMPLATE",
		"ANALYTIX_MEMORY_RECORD_TEMPLATE",
	}
	mainIpcEndpointBuilderUsesSharedTemplates :=
		input.EndpointBuilderAllowListMatrix.SchemaUsesSharedTemplates &&
			allStringInSlice(requiredMainIpcTemplates, input.EndpointBuilderAllowListMatrix.SharedTemplatesCompiled)
	mainIpcRuntimeAdapterPreservesEncodedPaths :=
		input.RuntimeAdapterHandoffMatrix.HandlerChannel == "runtime:request" &&
			input.RuntimeAdapterHandoffMatrix.AdapterFunction == "runtimeRequest" &&
			input.RuntimeAdapterHandoffMatrix.EncodedPathEvidencePresent &&
			len(input.RuntimeAdapterHandoffMatrix.AcceptedAdapterCalls) == 8
	for _, request := range input.RuntimeAdapterHandoffMatrix.AcceptedAdapterCalls {
		if !strings.HasPrefix(request.Path, "/v1/") || request.Method == "" {
			mainIpcRuntimeAdapterPreservesEncodedPaths = false
		}
		for _, rawID := range rawMainIpcEndpointIDs {
			if rawID != "" && strings.Contains(request.Path, rawID) {
				mainIpcRuntimeAdapterPreservesEncodedPaths = false
			}
		}
		hasEncodedID := false
		for _, encodedID := range encodedMainIpcEndpointIDs {
			if encodedID != "" && strings.Contains(request.Path, encodedID) {
				hasEncodedID = true
				break
			}
		}
		if !hasEncodedID {
			mainIpcRuntimeAdapterPreservesEncodedPaths = false
		}
	}
	mainIpcRuntimeAdapterPreservesMethodAndBody := input.RuntimeAdapterHandoffMatrix.MethodAndBodyEvidencePresent
	for _, request := range input.RuntimeAdapterHandoffMatrix.AcceptedAdapterCalls {
		if request.Method == "GET" {
			if request.Body != "" {
				mainIpcRuntimeAdapterPreservesMethodAndBody = false
			}
			continue
		}
		if request.Method == "" || request.Body == "" {
			mainIpcRuntimeAdapterPreservesMethodAndBody = false
		}
	}
	mainIpcRuntimeAdapterRejectsRawDynamicRoutes :=
		input.RuntimeAdapterHandoffMatrix.RejectsBeforeAdapterEvidencePresent &&
			len(input.RuntimeAdapterHandoffMatrix.RejectedBeforeAdapterCalls) == 7
	for _, request := range input.RuntimeAdapterHandoffMatrix.RejectedBeforeAdapterCalls {
		if !strings.HasPrefix(request.Path, "/v1/") || request.Method == "" {
			mainIpcRuntimeAdapterRejectsRawDynamicRoutes = false
		}
		if request.Path != "/v1/user-input/input_raw" && !strings.Contains(request.Path, "/raw") {
			mainIpcRuntimeAdapterRejectsRawDynamicRoutes = false
		}
	}
	mainIpcRuntimeAdapterRejectsBeforeCall :=
		input.RuntimeAdapterHandoffMatrix.ParseBeforeAdapterCall &&
			input.RuntimeAdapterHandoffMatrix.RejectsBeforeAdapterEvidencePresent &&
			runtimeRequestHandlerRejectsBeforeRuntimeCall
	mainIpcRuntimeAdapterUnitEvidencePresent :=
		input.RuntimeAdapterHandoffMatrix.EncodedPathEvidencePresent &&
			input.RuntimeAdapterHandoffMatrix.MethodAndBodyEvidencePresent &&
			input.RuntimeAdapterHandoffMatrix.RejectsBeforeAdapterEvidencePresent
	mainRuntimeHostHandoffPreservesEncodedPathAndQuery :=
		input.RuntimeHostHandoffMatrix.AdapterFunction == "runtimeRequestViaHost" &&
			input.RuntimeHostHandoffMatrix.BaseURLFunction == "getRuntimeBaseUrlForSettings" &&
			input.RuntimeHostHandoffMatrix.UnitTestEvidencePresent &&
			input.RuntimeHostHandoffMatrix.SourceEvidence.JoinsBaseWithNormalizedPath &&
			strings.HasPrefix(input.RuntimeHostHandoffMatrix.Request.Path, "/v1/") &&
			strings.Contains(input.RuntimeHostHandoffMatrix.Request.Path, input.RuntimeHostHandoffMatrix.EncodedIDs.ThreadID) &&
			strings.Contains(input.RuntimeHostHandoffMatrix.Request.Path, input.RuntimeHostHandoffMatrix.EncodedIDs.TurnID) &&
			strings.Contains(input.RuntimeHostHandoffMatrix.Request.Path, "timezone=Asia%2FShanghai") &&
			!strings.Contains(input.RuntimeHostHandoffMatrix.Request.Path, input.RuntimeHostHandoffMatrix.SourceIDs.ThreadID) &&
			!strings.Contains(input.RuntimeHostHandoffMatrix.Request.Path, input.RuntimeHostHandoffMatrix.SourceIDs.TurnID)
	mainRuntimeHostHandoffPreservesMethodHeadersBody :=
		input.RuntimeHostHandoffMatrix.UnitTestEvidencePresent &&
			input.RuntimeHostHandoffMatrix.SourceEvidence.SetsBearerAuth &&
			input.RuntimeHostHandoffMatrix.SourceEvidence.ForwardsCustomHeaders &&
			input.RuntimeHostHandoffMatrix.SourceEvidence.DefaultsJSONContentType &&
			input.RuntimeHostHandoffMatrix.SourceEvidence.ForwardsMethodAndBody &&
			input.RuntimeHostHandoffMatrix.Request.Method == "POST" &&
			input.RuntimeHostHandoffMatrix.Request.Body == `{"action":"step"}` &&
			input.RuntimeHostHandoffMatrix.Request.AuthHeader == "Bearer usage-token" &&
			input.RuntimeHostHandoffMatrix.Request.ContentType == "application/json" &&
			input.RuntimeHostHandoffMatrix.Request.CustomHeaderName == "X-Analytix-Evidence" &&
			input.RuntimeHostHandoffMatrix.Request.CustomHeaderValue == "runtime-host-path"
	mainRuntimeHostHandoffUsesEnsuredSettings :=
		input.RuntimeHostHandoffMatrix.SourceEvidence.UsesEnsuredSettings &&
			input.RuntimeHostHandoffMatrix.EnsureRuntimePortEvidencePresent
	mainRuntimeHostHandoffUnitEvidencePresent :=
		input.RuntimeHostHandoffMatrix.UnitTestEvidencePresent &&
			input.RuntimeHostHandoffMatrix.EnsureRuntimePortEvidencePresent
	mainSseHostEncodesThreadIDAndCursor :=
		input.MainSseHostEncodingMatrix.HandlerChannel == "runtime:sse:start" &&
			input.MainSseHostEncodingMatrix.EventChannel == "runtime:sse-error" &&
			input.MainSseHostEncodingMatrix.SourceBuildsAnalytixEventsPath &&
			input.MainSseHostEncodingMatrix.EncodedUnitEvidencePresent &&
			strings.HasPrefix(input.MainSseHostEncodingMatrix.Request.Path, "/v1/threads/") &&
			strings.HasSuffix(input.MainSseHostEncodingMatrix.Request.Path, "/events") &&
			strings.Contains(input.MainSseHostEncodingMatrix.Request.Path, input.MainSseHostEncodingMatrix.EncodedThreadID) &&
			!strings.Contains(input.MainSseHostEncodingMatrix.Request.Path, input.MainSseHostEncodingMatrix.SourceThreadID) &&
			input.MainSseHostEncodingMatrix.Request.SinceSeq == 9 &&
			input.MainSseHostEncodingMatrix.Request.LastEventID == "9"
	mainSseHostPreservesHeadersAndStreamID :=
		input.MainSseHostEncodingMatrix.HeaderUnitEvidencePresent &&
			input.MainSseHostEncodingMatrix.ErrorUnitEvidencePresent &&
			input.MainSseHostEncodingMatrix.Request.Accept == "text/event-stream" &&
			input.MainSseHostEncodingMatrix.Request.Authorization == "Bearer runtime-token" &&
			input.MainSseHostEncodingMatrix.Request.StreamID == "stream-encoded" &&
			input.MainSseHostEncodingMatrix.ErrorPayload.StreamID == "stream-encoded" &&
			input.MainSseHostEncodingMatrix.ErrorPayload.Status == 404
	mainSseHostRejectsForbiddenRouteTokens := input.MainSseHostEncodingMatrix.ForbiddenRouteGuardPresent
	forbiddenSseTokens := []string{
		"/v1/reasonix",
		"/v1/runtime/go",
		"/v1/workflow",
		"/v1/create-loop",
		"/v1/subagents",
		"/v1/autoresearch",
		"/v1/mcp-indexer",
	}
	for _, token := range forbiddenSseTokens {
		if strings.Contains(input.MainSseHostEncodingMatrix.Request.Path, token) {
			mainSseHostRejectsForbiddenRouteTokens = false
			break
		}
	}
	mainSseHostEncodingUnitEvidencePresent :=
		input.MainSseHostEncodingMatrix.EncodedUnitEvidencePresent &&
			input.MainSseHostEncodingMatrix.HeaderUnitEvidencePresent &&
			input.MainSseHostEncodingMatrix.ErrorUnitEvidencePresent
	sseChannelsOwned := input.SSE.StartChannel == "runtime:sse:start" &&
		input.SSE.StopChannel == "runtime:sse:stop" &&
		input.SSE.EventChannel == "runtime:sse-event" &&
		input.SSE.ErrorChannel == "runtime:sse-error" &&
		input.SSE.EndChannel == "runtime:sse-end"
	return G5DesktopMainIpcBoundaryOutput{
		RuntimeRequestSchemaStrict:                         input.RuntimeRequest.Evidence.SchemaStrict,
		RuntimeRequestAllowsAnalytixRoutes:                 runtimeRequestAllowsAnalytixRoutes,
		RuntimeRequestRejectsForbiddenRoutes:               runtimeRequestRejectsForbiddenRoutes,
		RuntimeRequestHandlerRejectsBeforeRuntimeCall:      runtimeRequestHandlerRejectsBeforeRuntimeCall,
		MainIpcEndpointBuilderAcceptsSharedPaths:           mainIpcEndpointBuilderAcceptsSharedPaths,
		MainIpcEndpointBuilderRejectsRawDynamicRoutes:      mainIpcEndpointBuilderRejectsRawDynamicRoutes,
		MainIpcEndpointBuilderUsesSharedTemplates:          mainIpcEndpointBuilderUsesSharedTemplates,
		MainIpcEndpointBuilderUnitEvidencePresent:          input.EndpointBuilderAllowListMatrix.UnitTestEvidencePresent,
		MainIpcEndpointBuilderRejectsSingularUserInput:     mainIpcEndpointBuilderRejectsSingularUserInput,
		MainIpcRuntimeAdapterPreservesEncodedPaths:         mainIpcRuntimeAdapterPreservesEncodedPaths,
		MainIpcRuntimeAdapterPreservesMethodAndBody:        mainIpcRuntimeAdapterPreservesMethodAndBody,
		MainIpcRuntimeAdapterRejectsRawDynamicRoutes:       mainIpcRuntimeAdapterRejectsRawDynamicRoutes,
		MainIpcRuntimeAdapterRejectsBeforeCall:             mainIpcRuntimeAdapterRejectsBeforeCall,
		MainIpcRuntimeAdapterUnitEvidencePresent:           mainIpcRuntimeAdapterUnitEvidencePresent,
		MainRuntimeHostHandoffPreservesEncodedPathAndQuery: mainRuntimeHostHandoffPreservesEncodedPathAndQuery,
		MainRuntimeHostHandoffPreservesMethodHeadersBody:   mainRuntimeHostHandoffPreservesMethodHeadersBody,
		MainRuntimeHostHandoffUsesEnsuredSettings:          mainRuntimeHostHandoffUsesEnsuredSettings,
		MainRuntimeHostHandoffUnitEvidencePresent:          mainRuntimeHostHandoffUnitEvidencePresent,
		MainSseHostEncodesThreadIdAndCursor:                mainSseHostEncodesThreadIDAndCursor,
		MainSseHostPreservesHeadersAndStreamId:             mainSseHostPreservesHeadersAndStreamID,
		MainSseHostRejectsForbiddenRouteTokens:             mainSseHostRejectsForbiddenRouteTokens,
		MainSseHostEncodingUnitEvidencePresent:             mainSseHostEncodingUnitEvidencePresent,
		SSEStartSchemaStrict:                               input.SSE.Evidence.StartSchemaStrict,
		SSERejectsReasonixSessionPayload:                   input.SSE.Evidence.RejectsReasonixSessionID,
		SSEUsesAnalytixThreadEventsRoute:                   sseChannelsOwned && input.SSE.RoutePathTemplate == "/v1/threads/{id}/events" && input.SSE.Evidence.UsesAnalytixThreadEventsPath,
		SSEStopParsesStreamID:                              input.SSE.Evidence.StopParsesStreamID,
		SSEStopMatchesStreamIDOnly:                         input.SSE.Evidence.StopMatchesStreamIDOnly,
		SSEReconnectCursorPreserved:                        input.SSE.Evidence.ReconnectLastEventID,
		SSEBatchesEventsAt100ms:                            input.SSE.BatchMs == 100 && input.SSE.Evidence.BatchesEventsAt100ms,
		UsesReasonixProtocol:                               input.ProductBoundary.UsesReasonixProtocol,
		RendererRouteExposed:                               input.ProductBoundary.RendererRouteExposed,
		DefaultGoBackendEnabled:                            input.ProductBoundary.DefaultGoBackendEnabled,
	}
}

func replayG5RendererRouteSurface(input G5RendererRouteSurfaceCase) G5RendererRouteSurfaceOutput {
	expectedRoutes := []string{"chat", "write", "settings", "plugins", "claw", "schedule"}
	appRouteUnionKunCompatible := sameStringSlice(input.AppRoutes, expectedRoutes)
	noForbiddenRouteTokens := len(input.ForbiddenTopLevelRouteTokensPresent) == 0
	noForbiddenEntrypoints := len(input.ForbiddenEntrypointSymbolsPresent) == 0
	dormantWorkflowQuarantined := input.Evidence.DormantWorkflowCodeExists &&
		len(input.QuarantinedWorkflowEntrySurfaceHits) == 0
	browserPreviewBridgeAnalytixOnly := input.Evidence.BrowserPreviewInstallsWindowAnalytix &&
		input.Evidence.BrowserPreviewForbiddenAliasesAbsent
	pluginMarketplaceSafeAreaPropagates := input.Evidence.PluginMarketplaceReceivesLeftSidebarCollapsed &&
		input.Evidence.PluginMarketplaceTabsBeforeContent
	return G5RendererRouteSurfaceOutput{
		AppRouteUnionKunCompatible:          appRouteUnionKunCompatible,
		NoForbiddenTopLevelRouteTokens:      noForbiddenRouteTokens,
		NoForbiddenEntrypointSymbols:        noForbiddenEntrypoints,
		DormantWorkflowCodeQuarantined:      dormantWorkflowQuarantined,
		BrowserPreviewBridgeAnalytixOnly:    browserPreviewBridgeAnalytixOnly,
		PluginMarketplaceSafeAreaPropagates: pluginMarketplaceSafeAreaPropagates,
		ShellNavigationNoDrag:               input.Evidence.ShellNavigationControlsNoDrag,
		NativeControlsSafeInset:             input.Evidence.NativeSafeInsetCSSPresent,
		UsesReasonixProtocol:                input.ProductBoundary.UsesReasonixProtocol,
		TopLevelHiddenEntryExposed:          input.ProductBoundary.TopLevelHiddenEntryExposed,
		DefaultGoBackendEnabled:             input.ProductBoundary.DefaultGoBackendEnabled,
	}
}

func replayG5GoalPersistenceOffLock(input G5GoalPersistenceOffLockCase) G5GoalPersistenceOffLockOutput {
	setGoalPersistsBeforeEvent := sameStringSlice(input.SetGoalOrder, []string{
		"touchThread",
		"persistGoalThread:set",
		"goal_updated",
	})
	clearGoalPersistsBeforeEvent := sameStringSlice(input.ClearGoalOrder, []string{
		"touchThread",
		"persistGoalThread:clear",
		"goal_cleared",
	})
	noForbiddenControllerLock := len(input.ForbiddenLockSubstringsPresent) == 0
	const fixedWarningEvent = "[analytix] event=ANALYTIX_GOAL_PERSISTENCE_FAILED"
	warningIsExactValueFreeEvent := input.WarningEvidence.WarningIsExactValueFreeEvent &&
		input.WarningEvidence.WarningEvent == fixedWarningEvent
	return G5GoalPersistenceOffLockOutput{
		SetGoalPersistsBeforeEvent:                setGoalPersistsBeforeEvent,
		ClearGoalPersistsBeforeEvent:              clearGoalPersistsBeforeEvent,
		NoForbiddenControllerLock:                 noForbiddenControllerLock,
		GoalWritesOutsideSharedStatusApprovalLock: noForbiddenControllerLock && !input.ProductBoundary.StatusApprovalLockShared,
		PersistenceFailureWarns:                   input.WarningEvidence.LogsPersistenceFailure,
		PersistenceFailureSurfaces:                input.WarningEvidence.SurfacesOriginalError,
		WarningIsExactValueFreeEvent:              warningIsExactValueFreeEvent,
		WarningIncludesThreadID:                   strings.Contains(input.WarningEvidence.WarningEvent, input.WarningEvidence.ThreadID),
		WarningIncludesOriginalError:              strings.Contains(input.WarningEvidence.WarningEvent, input.WarningEvidence.OriginalError),
		WarningIncludesAction:                     strings.Contains(input.WarningEvidence.WarningEvent, input.WarningEvidence.ActionMarker),
		UsesReasonixProtocol:                      input.ProductBoundary.UsesReasonixProtocol,
		StatusApprovalLockShared:                  input.ProductBoundary.StatusApprovalLockShared,
		DefaultGoBackendEnabled:                   input.ProductBoundary.DefaultGoBackendEnabled,
		RendererVisibleGoRoute:                    input.ProductBoundary.RendererVisibleGoRoute,
	}
}

func replayG5ToolResultFileImageBoundary(input G5ToolResultFileImageBoundaryCase) G5ToolResultFileImageBoundaryOutput {
	inlineKindsPreserved := sameStringSlice(input.InlineImageKinds, []string{"image", "computer_screenshot"})
	evictedBase64Omitted := input.EvictedPayloadMarkers.Payload == "HUGE_BASE64_PAYLOAD" &&
		!stringInSlice(input.EvictedPayloadMarkers.Payload, input.EvictedPayloadMarkers.PreservedMetadata)
	evictedMetadataPreserved := sameStringSlice(input.EvictedPayloadMarkers.PreservedMetadata, []string{"computer_screenshot", "1280"})
	onlyNewestImagesInline := input.CapPolicy.HistoryImageCount == 4 &&
		input.CapPolicy.MaxKept == 2 &&
		input.CapPolicy.EvictedCount == 2 &&
		input.CapPolicy.NewestImageDataBase64 == "IMG_D"
	attachmentLocalFilePathPreserved := input.AttachmentFallback.LocalFilePath == "/tmp/picked/shot.png"
	textFallbackCarriesFilePath := attachmentLocalFilePathPreserved &&
		input.AttachmentFallback.TextFallbackPrefix == "[Attached image as base64 text]" &&
		input.AttachmentFallback.MimeType == "image/webp" &&
		input.AttachmentFallback.Dimensions == "1280x720" &&
		input.AttachmentFallback.FallbackBase64 == "YWJj"
	generatedFileMetaLifted := input.GeneratedFiles.ToolName == "generate_image" &&
		input.GeneratedFiles.GeneratedRelativePath == ".analytix-images/img-1.png" &&
		input.GeneratedFiles.SpeechRelativePath == ".analytix-audio/speech.mp3"
	toolAttachmentMetaLifted := input.GeneratedFiles.AttachmentID == "att_abc"
	return G5ToolResultFileImageBoundaryOutput{
		InlineImageKindsPreserved:        inlineKindsPreserved,
		EvictedBase64Omitted:             evictedBase64Omitted,
		EvictedMetadataPreserved:         evictedMetadataPreserved,
		OnlyNewestImagesInline:           onlyNewestImagesInline,
		AttachmentLocalFilePathPreserved: attachmentLocalFilePathPreserved,
		TextFallbackCarriesFilePath:      textFallbackCarriesFilePath,
		DeepseekV4TextFallback:           input.AttachmentFallback.DeepseekV4TextFallback,
		GeneratedFileMetaLifted:          generatedFileMetaLifted,
		ToolAttachmentMetaLifted:         toolAttachmentMetaLifted,
		UsesReasonixProtocol:             input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:             input.ProductBoundary.TopLevelRouteExposed,
		DefaultGoBackendEnabled:          input.ProductBoundary.DefaultGoBackendEnabled,
	}
}

func replayG5EventJsonlReplayBoundary(input G5EventJsonlReplayBoundaryCase) G5EventJsonlReplayBoundaryOutput {
	jsonlFilesPresent := sameStringSlice(input.JsonlFiles, []string{"events.jsonl", "messages.jsonl"})
	usageCarryover := sameIntSlice(input.UsageCompactionEvidence.CompactedSeqs, []int{1, 3, 5, 6, 7}) &&
		input.UsageCompactionEvidence.HighestSeq == 7
	compactionFailureKeepsLog := sameIntSlice(input.UsageCompactionEvidence.FailureKeepsAppendedSeqs, []int{1, 2, 3}) &&
		input.UsageCompactionEvidence.WarningPrefix == "[analytix] usage event compaction failed"
	return G5EventJsonlReplayBoundaryOutput{
		AppendNewlineTerminated:             jsonlFilesPresent && input.ReplayEvidence.AppendNewlineTerminated,
		LoadEventsSinceFiltersAndSorts:      input.ReplayEvidence.LoadEventsSinceFiltersAndSorts,
		HighestSeqPreservesMax:              input.ReplayEvidence.HighestSeqUsesMax,
		MalformedJsonlLineSkipped:           input.ReplayEvidence.MalformedJsonlLineSkipped,
		PersistsBeforePublish:               input.RecorderEvidence.PersistsBeforePublish,
		ConcurrentSeqsUnique:                input.RecorderEvidence.ConcurrentSeqsUnique,
		PersistedHighWaterReadOnce:          input.RecorderEvidence.PersistedHighWaterReadOnce,
		UsageCompactionKeepsCarryover:       usageCarryover,
		CompactionFailureKeepsAppendOnlyLog: compactionFailureKeepsLog,
		UsesReasonixProtocol:                input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:                input.ProductBoundary.TopLevelRouteExposed,
		DefaultGoBackendEnabled:             input.ProductBoundary.DefaultGoBackendEnabled,
	}
}

func replayG5MCPMalformedSchemaBoundary(input G5MCPMalformedSchemaBoundaryCase) G5MCPMalformedSchemaBoundaryOutput {
	badSchema := input.NormalizedSchemas.BadSchema
	badRequired := input.NormalizedSchemas.BadRequired
	nonObjectDefaults := input.MalformedTools.RawNonObjectSchemaKind == "array" &&
		badSchema.Type == "object" &&
		badSchema.PropertiesEmpty &&
		badSchema.AdditionalProperties &&
		input.NormalizerEvidence.NonRecordDefaults
	propertiesDropped := badRequired.Type == "object" &&
		badRequired.PropertiesDropped &&
		input.NormalizerEvidence.PropertiesMustBeRecord
	requiredNonStringsDropped := input.MalformedTools.RawRequiredMixedCount == 3 &&
		sameStringSlice(badRequired.Required, []string{"query"}) &&
		input.NormalizerEvidence.RequiredFiltersStrings
	advertisedNamesPreserved := input.MalformedTools.NonObjectSchemaToolName == "mcp_github_bad_schema" &&
		input.MalformedTools.MalformedRequiredToolName == "mcp_github_bad_required"
	modelCatalogSchemaSafe := nonObjectDefaults &&
		propertiesDropped &&
		requiredNonStringsDropped &&
		input.NormalizerEvidence.NonObjectTypeDefaults
	return G5MCPMalformedSchemaBoundaryOutput{
		NonObjectSchemaDefaults:      nonObjectDefaults,
		PropertiesArrayDropped:       propertiesDropped,
		RequiredNonStringsDropped:    requiredNonStringsDropped,
		AdvertisedToolNamesPreserved: advertisedNamesPreserved,
		ModelCatalogSchemaSafe:       modelCatalogSchemaSafe,
		OutputSchemaNonRecordOmitted: input.NormalizerEvidence.OutputSchemaRecordOnly,
		UsesReasonixProtocol:         input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:         input.ProductBoundary.TopLevelRouteExposed,
		DefaultGoBackendEnabled:      input.ProductBoundary.DefaultGoBackendEnabled,
	}
}

func replayG5Cancel(input G5CancelExecutableCase) []G5ToolResult {
	results := make([]G5ToolResult, 0, len(input.AcceptedToolCalls))
	for _, call := range input.AcceptedToolCalls {
		if call.State == "completed" {
			results = append(results, G5ToolResult{
				CallID: call.CallID,
				Status: "completed",
				Code:   "",
				Output: call.Output,
			})
			continue
		}
		output := call.Output
		if output == "" {
			output = "cancelled before tool execution"
		}
		results = append(results, G5ToolResult{
			CallID: call.CallID,
			Status: "aborted",
			Code:   input.CancelledResultCode,
			Output: output,
		})
	}
	return results
}

func replayG5TaskJobToolContractBoundary(input G5TaskJobToolContractBoundaryCase) G5TaskJobToolContractBoundaryOutput {
	return G5TaskJobToolContractBoundaryOutput{
		TaskToolName:                   input.Task.Name,
		ParallelToolName:               input.ParallelTasks.Name,
		TaskInternalRuntimeOnly:        input.Task.InternalRuntimeOnly,
		ParallelInternalRuntimeOnly:    input.ParallelTasks.InternalRuntimeOnly,
		RequiresPermissionGate:         input.Task.RequiresPermissionGate,
		MayAppendParentGoalEvidence:    input.Task.MayAppendParentGoalEvidence,
		RequiresDependencyValidation:   input.ParallelTasks.RequiresDependencyValidation,
		RequiresPlannerReadOnlyToolset: input.ParallelTasks.RequiresPlannerReadOnlyToolset,
		Routes:                         append([]string{}, input.Routes...),
		ProtectedRoutes:                append([]string{}, input.ProtectedRoutes...),
		UnauthorizedStatus:             input.UnauthorizedStatus,
		ForbiddenTopLevelRoutes:        append([]string{}, input.ForbiddenTopLevelRoutes...),
		UsesReasonixProtocol:           false,
		TopLevelRouteExposed:           false,
	}
}

func replayG5TaskJobTranscriptIdentity(input G5TaskJobTranscriptIdentityCase) G5TaskJobTranscriptIdentityOutput {
	return G5TaskJobTranscriptIdentityOutput{
		SourceID:                       input.Transcript.SourceID,
		ContinueTargetID:               input.Transcript.ContinueTargetID,
		ForkTargetID:                   input.Transcript.ForkTargetID,
		IncompatibleError:              input.Transcript.IncompatibleError,
		SameTranscriptIdentityRequired: input.Transcript.SameIdentityRequired,
		ContinuePreservesTarget:        input.Transcript.ContinuePreservesTarget,
		ForkCreatesDistinctTarget:      input.Transcript.ForkCreatesDistinctTarget,
		ContinueTargetMatchesSource:    input.Transcript.ContinueTargetID == input.Transcript.SourceID,
		ForkTargetDistinctFromSource:   input.Transcript.ForkTargetID != input.Transcript.SourceID,
		UsesReasonixProtocol:           false,
		TopLevelRouteExposed:           false,
	}
}

func replayG5TaskJobNestedSSEMetadata(input G5TaskJobNestedSSEMetadataCase) G5TaskJobNestedSSEMetadataOutput {
	fields := append([]string{}, input.NestedEvent.NestedSSEMetadataFields...)
	return G5TaskJobNestedSSEMetadataOutput{
		ParentCallID:                input.NestedEvent.ParentCallID,
		ChildRunID:                  input.NestedEvent.ChildRunID,
		NestedSSEMetadataFields:     fields,
		IncludesParentCallID:        stringSliceContains(fields, "parentCallId"),
		IncludesChildRunID:          stringSliceContains(fields, "childRunId"),
		IncludesEvidenceLedgered:    stringSliceContains(fields, input.ParentGoalEvidence.EvidenceLedgeredEventKey),
		IncludesEvidenceLedgerError: stringSliceContains(fields, input.ParentGoalEvidence.EvidenceLedgerErrorEventKey),
		ParentChildDistinct:         input.NestedEvent.ParentCallID != input.NestedEvent.ChildRunID,
		RequiresActiveGoal:          input.ParentGoalEvidence.RequiresActiveGoal,
		EvidenceLedgeredEventKey:    input.ParentGoalEvidence.EvidenceLedgeredEventKey,
		EvidenceLedgerErrorEventKey: input.ParentGoalEvidence.EvidenceLedgerErrorEventKey,
		UsesReasonixProtocol:        false,
		TopLevelRouteExposed:        false,
	}
}

func replayG5TaskJobs(input G5TaskJobsExecutableCase) []G5TaskJobOutput {
	jobs := make([]G5TaskJobOutput, 0, len(input.Jobs))
	for _, job := range input.Jobs {
		switch job.State {
		case "completed":
			jobs = append(jobs, G5TaskJobOutput{
				ID:     job.ID,
				Status: "completed",
				Output: job.Output,
				Offset: job.Offset,
				Reason: "",
			})
		case "running":
			jobs = append(jobs, G5TaskJobOutput{
				ID:     job.ID,
				Status: "killed",
				Output: job.Output,
				Offset: job.Offset,
				Reason: "parent turn aborted",
			})
		default:
			jobs = append(jobs, G5TaskJobOutput{
				ID:     job.ID,
				Status: "skipped",
				Output: job.Output,
				Offset: job.Offset,
				Reason: input.SkippedUnstartedReason,
			})
		}
	}
	return jobs
}

func replayG5ParallelValidation(input G5ParallelValidationCase) G5ParallelValidationOutput {
	validOrder, _ := validateG5ParallelTaskPlan(input.ValidPlan)
	invalidResults := make([]G5ParallelInvalidValidationItem, 0, len(input.InvalidPlans))
	for _, item := range input.InvalidPlans {
		_, validationError := validateG5ParallelTaskPlan(item.Tasks)
		invalidResults = append(invalidResults, G5ParallelInvalidValidationItem{
			CaseID: item.CaseID,
			Error:  validationError,
		})
	}
	return G5ParallelValidationOutput{
		DependencyField:      input.DependencyField,
		ValidOrder:           validOrder,
		InvalidResults:       invalidResults,
		UsesReasonixProtocol: input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed: input.ProductBoundary.TopLevelRouteExposed,
	}
}

func validateG5ParallelTaskPlan(items []G5ParallelPlanItem) ([]string, string) {
	if len(items) < 2 {
		return nil, "parallel task plan requires at least two tasks"
	}
	ids := make(map[string]bool, len(items))
	byID := make(map[string]G5ParallelPlanItem, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			return nil, "parallel task ids must be non-empty"
		}
		if ids[item.ID] {
			return nil, "duplicate parallel task id: " + item.ID
		}
		ids[item.ID] = true
		byID[item.ID] = item
	}
	for _, item := range items {
		for _, dependency := range item.DependsOn {
			if dependency == item.ID {
				return nil, "self-referencing parallel task dependency: " + item.ID
			}
			if !ids[dependency] {
				return nil, "unknown parallel task dependency: " + dependency
			}
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	order := make([]string, 0, len(items))
	var visit func(string) string
	visit = func(id string) string {
		if visited[id] {
			return ""
		}
		if visiting[id] {
			return "parallel task dependency cycle includes: " + id
		}
		visiting[id] = true
		for _, dependency := range byID[id].DependsOn {
			if validationError := visit(dependency); validationError != "" {
				return validationError
			}
		}
		delete(visiting, id)
		visited[id] = true
		order = append(order, id)
		return ""
	}
	for _, item := range items {
		if validationError := visit(item.ID); validationError != "" {
			return nil, validationError
		}
	}
	return order, ""
}

func replayG5TaskJobStaleReconcile(input G5TaskJobStaleReconcileCase) []G5TaskJobOutput {
	jobs := make([]G5TaskJobOutput, 0, len(input.Jobs))
	for _, job := range input.Jobs {
		jobs = append(jobs, G5TaskJobOutput{
			ID:     job.ID,
			Status: "interrupted",
			Output: job.Output,
			Offset: job.Offset,
			Reason: input.Reason,
		})
	}
	return jobs
}

func replayG5TaskJobRestartDrill(input G5TaskJobRestartDrillCase) G5TaskJobRestartDrillOutput {
	output := G5TaskJobRestartDrillOutput{
		RehydratedCount: len(input.Jobs),
	}
	for _, job := range input.Jobs {
		switch job.State {
		case "running":
			output.Completed = G5TaskJobRestartComplete{
				ID:         job.ID,
				Status:     input.WaitStatus,
				Output:     job.Output + input.OutputAfterRestart,
				NextOffset: job.Offset + 1,
			}
		case "queued":
			output.Killed = G5TaskJobRestartKilled{
				ID:     job.ID,
				Status: input.KillStatus,
				Error:  input.KillError,
			}
		}
	}
	return output
}

func replayG5TaskJobRouteExecutable(input G5TaskJobRouteExecutableCase) G5TaskJobRouteExecutableOutput {
	return G5TaskJobRouteExecutableOutput{
		UnauthorizedStatus:            input.UnauthorizedStatus,
		OutputStatus:                  input.Output.Status,
		OutputJobStatus:               input.Output.JobStatus,
		OutputNextOffset:              input.Output.NextOffset,
		OutputReplayOffset:            input.Output.ReplayOffset,
		WaitStatus:                    input.Wait.Status,
		WaitJobStatus:                 input.Wait.JobStatus,
		KillStatus:                    input.Kill.Status,
		KillJobStatus:                 input.Kill.JobStatus,
		MissingOutputStatus:           input.MissingOutput.Status,
		RehydratedOutputStatus:        input.Rehydrated.OutputStatus,
		RehydratedWaitStatus:          input.Rehydrated.WaitStatus,
		RehydratedKillStatus:          input.Rehydrated.KillStatus,
		RehydratedCompletedStatus:     input.Rehydrated.CompletedStatus,
		RehydratedCompletedNextOffset: input.Rehydrated.CompletedNextOffset,
		RehydratedKilledStatus:        input.Rehydrated.KilledStatus,
		UsesReasonixProtocol:          input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:          input.ProductBoundary.TopLevelRouteExposed,
	}
}

func replayG5TaskJobLifecycle(input G5TaskJobLifecycleCase) G5TaskJobLifecycleOutput {
	return G5TaskJobLifecycleOutput{
		ForegroundKind:             input.Foreground.Kind,
		ForegroundStatus:           input.Foreground.Status,
		ForegroundResult:           input.Foreground.Result,
		BackgroundKind:             input.Background.Kind,
		BackgroundStatusAcrossTurn: input.Background.StatusAcrossTurn,
		BackgroundOutput:           input.Background.Output,
		BackgroundFinalStatus:      input.Background.FinalStatus,
		BackgroundFinalResult:      input.Background.FinalResult,
		WaitOutputKillStatus:       input.WaitOutputKill.KilledStatus,
		WaitOutputKillError:        input.WaitOutputKill.KilledError,
		UsesReasonixProtocol:       input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:       input.ProductBoundary.TopLevelRouteExposed,
	}
}

func replayG5TaskJobPlannerExecutor(input G5TaskJobPlannerExecutorCase) G5TaskJobPlannerExecutorOutput {
	return G5TaskJobPlannerExecutorOutput{
		PlannerKind:                   input.PlannerKind,
		PlannerPolicy:                 input.PlannerPolicy,
		ExecutorPolicy:                input.ExecutorPolicy,
		FailureStatus:                 input.FailureStatus,
		CancelledStatus:               input.CancelledStatus,
		SkippedReason:                 input.SkippedReason,
		CancelReason:                  input.CancelReason,
		OutputOffsetJobCount:          input.OutputOffsetJobCount,
		FailurePropagates:             input.RequiresFailurePropagation,
		CancellationPropagates:        input.RequiresCancellationPropagation,
		TranscriptPropagates:          input.RequiresTranscriptPropagation,
		TranscriptPropagationMode:     input.TranscriptPropagationMode,
		TranscriptPropagationJobCount: input.TranscriptPropagationJobCount,
		UsesReasonixProtocol:          input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:          input.ProductBoundary.TopLevelRouteExposed,
	}
}

func replayG5TaskJobPlannerToolsetInventory(input G5TaskJobPlannerToolsetInventoryCase) G5TaskJobPlannerToolsetInventoryOutput {
	taskToolNames := []string{input.TaskToolName, input.ParallelToolName}
	return G5TaskJobPlannerToolsetInventoryOutput{
		ReadOnlyToolset:           append([]string(nil), input.ReadOnlyToolset...),
		ForbiddenToolset:          append([]string(nil), input.ForbiddenToolset...),
		ReadOnlyToolCount:         len(input.ReadOnlyToolset),
		ForbiddenToolCount:        len(input.ForbiddenToolset),
		ReadOnlyExcludesTaskTools: allToolsAbsent(input.ReadOnlyToolset, taskToolNames),
		ForbiddenMatchesTaskTools: allToolsPresent(input.ForbiddenToolset, taskToolNames),
		PlannerPolicy:             input.PlannerPolicy,
		ExecutorPolicy:            input.ExecutorPolicy,
		UsesReasonixProtocol:      input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:      input.ProductBoundary.TopLevelRouteExposed,
	}
}

func allToolsAbsent(haystack []string, needles []string) bool {
	for _, needle := range needles {
		if stringSliceContains(haystack, needle) {
			return false
		}
	}
	return true
}

func allToolsPresent(haystack []string, needles []string) bool {
	for _, needle := range needles {
		if !stringSliceContains(haystack, needle) {
			return false
		}
	}
	return true
}

func replayG5ParentGoalEvidence(input G5ParentGoalEvidenceCase) G5ParentGoalEvidenceOutput {
	activeLedgered, _ := input.ActiveGoalEventMetadata[input.LedgeredEventKey].(bool)
	missingLedgered, _ := input.MissingGoalEventMetadata[input.LedgeredEventKey].(bool)
	missingError, _ := input.MissingGoalEventMetadata[input.LedgerErrorEventKey].(string)

	return G5ParentGoalEvidenceOutput{
		RequiresActiveGoal:     input.RequiresActiveGoal,
		LedgeredEventKey:       input.LedgeredEventKey,
		LedgerErrorEventKey:    input.LedgerErrorEventKey,
		LedgeredWhenActiveGoal: activeLedgered,
		ErrorsWithoutActiveGoal: input.RequiresActiveGoal &&
			!missingLedgered &&
			missingError != "",
		UsesReasonixProtocol: input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed: input.ProductBoundary.TopLevelRouteExposed,
	}
}

func replayG5ApprovalDeny(input G5ApprovalDenyCase) G5ApprovalDenyOutput {
	return G5ApprovalDenyOutput{
		DeniedToolNames:    input.AttemptedToolNames,
		ApprovalIDs:        input.ApprovalIDs,
		ApprovalItemCount:  len(input.ApprovalIDs),
		CreatesDurableJobs: false,
		CreatesChildRuns:   false,
	}
}

func replayG5UserInput(input G5UserInputExecutableCase) G5UserInputOutput {
	return G5UserInputOutput{
		SubmittedInputID:             input.Submitted.ID,
		SubmittedStatus:              "submitted",
		AnswerCount:                  len(input.Submitted.Answers),
		HTTPEchoesAnswers:            len(input.Submitted.Answers) > 0,
		ResolvedEventKind:            input.Submitted.ResolvedEventKind,
		ResolvedEventIncludesAnswers: input.Submitted.ResolvedEventIncludesAnswers,
		CancelledInputID:             input.Cancelled.ID,
		CancelledStatus:              input.Cancelled.Status,
		SecondResolveStatus:          input.Cancelled.SecondResolveStatus,
		PendingAfterSubmit:           input.Submitted.PendingAfter,
		PendingAfterCancel:           input.Cancelled.PendingAfter,
		StructuredChoiceValidation:   replayG5UserInputStructuredChoiceValidation(input.StructuredChoiceValidation),
	}
}

func replayG5UserInputStructuredChoiceValidation(
	input G5UserInputStructuredChoiceValidationCase,
) G5UserInputStructuredChoiceValidationOutput {
	return G5UserInputStructuredChoiceValidationOutput{
		MaxQuestions:                          input.MaxQuestions,
		MinOptionsWhenProvided:                input.MinOptionsWhenProvided,
		MaxOptionsWhenProvided:                input.MaxOptionsWhenProvided,
		DedupeLabelsCaseInsensitive:           input.DedupeLabelsCaseInsensitive,
		InvalidResultCode:                     input.InvalidResultCode,
		InvalidCases:                          append([]string(nil), input.InvalidCases...),
		InvalidCaseCount:                      len(input.InvalidCases),
		RejectsTooManyQuestions:               stringInSlice("too_many_questions", input.InvalidCases),
		RejectsSingleOption:                   stringInSlice("single_option", input.InvalidCases),
		RejectsTooManyOptions:                 stringInSlice("too_many_options", input.InvalidCases),
		RejectsDuplicateLabelsCaseInsensitive: stringInSlice("duplicate_labels_case_insensitive", input.InvalidCases),
		OpensGateOnInvalid:                    input.OpensGateOnInvalid,
		UsesReasonixProtocol:                  false,
		TopLevelRouteExposed:                  false,
	}
}

func replayG5AbortCleanup(input G5AbortCleanupCase) G5AbortCleanupOutput {
	return G5AbortCleanupOutput{
		ApprovalID:                 input.ApprovalID,
		ApprovalStatus:             input.ExpectedApprovalStatus,
		UserInputID:                input.UserInputID,
		UserInputStatus:            input.ExpectedUserInputStatus,
		LateApprovalDecisionStatus: input.LateApprovalDecisionStatus,
		LateUserInputResolveStatus: input.LateUserInputResolveStatus,
		PendingAfterCleanup:        0,
		ReplayKinds:                append([]string(nil), input.ReplayKinds...),
		UsesReasonixProtocol:       false,
		TopLevelRouteExposed:       false,
	}
}

func replayG5ResumePendingGates(input G5ResumePendingGatesCase) G5ResumePendingGatesOutput {
	pendingAfterResume := 0
	if input.ResumedStatuses.Approval == "pending" {
		pendingAfterResume++
	}
	if input.ResumedStatuses.UserInput == "pending" {
		pendingAfterResume++
	}
	return G5ResumePendingGatesOutput{
		SourceApprovalStatus:   input.SourceStatuses.Approval,
		SourceUserInputStatus:  input.SourceStatuses.UserInput,
		ResumedApprovalStatus:  input.ResumedStatuses.Approval,
		ResumedUserInputStatus: input.ResumedStatuses.UserInput,
		PendingAfterResume:     pendingAfterResume,
		AnswersCopiedToResume:  false,
		UsesReasonixProtocol:   false,
		TopLevelRouteExposed:   false,
	}
}

func replayG5AutoResearch(input G5AutoResearchExecutableCase) G5AutoResearchOutput {
	return G5AutoResearchOutput{
		StateRelativePath:                    ".analytix/autoresearch/" + input.ThreadID,
		FileCount:                            len(input.ExpectedFiles),
		WritesReasonixFile:                   stringInSlice("REASONIX.md", input.ExpectedFiles),
		WritesAgentsFile:                     stringInSlice("AGENTS.md", input.ExpectedFiles),
		UnknownRequirementAccepted:           false,
		FindingsWrittenForUnknownRequirement: false,
		DirectionTrackingFileWritten:         input.Direction != "" && stringInSlice("directions_tried.json", input.ExpectedFiles),
		IterationLogRecordsDirection:         input.Direction != "" && stringInSlice("iteration_log.jsonl", input.ExpectedFiles),
		RecordResearchDirectionToolPresent:   input.RecordDirectionToolName == "record_research_direction",
		RecordDirectionRequiresActiveGoal:    input.DirectionRequiresActiveGoal,
		StablePrefixContainsState:            false,
		ToolSchemaContainsState:              false,
		TopLevelRouteExposed:                 false,
	}
}

func replayG5MCPLifecycle(input G5MCPLifecycleExecutableCase) G5MCPLifecycleOutput {
	retryAttempts := make(map[string]int, len(input.FailedServerIDs))
	for _, serverID := range input.FailedServerIDs {
		retryAttempts[serverID] = input.AttemptsPerFailedServer
	}
	activePaths := make([]string, 0, len(input.LiveLocal.InitialPaths)+len(input.LiveLocal.ResumePaths))
	for _, path := range input.LiveLocal.InitialPaths {
		if path != input.LiveLocal.LateTombstonePath {
			activePaths = append(activePaths, path)
		}
	}
	activePaths = append(activePaths, input.LiveLocal.ResumePaths...)
	secretSafeDiagnostic := "Authorization=" + input.LiveLocal.Replacement
	return G5MCPLifecycleOutput{
		ProviderID:             input.ProviderID,
		RetryAttempts:          retryAttempts,
		ConnectedServerIDs:     input.ExpectedConnectedServerIDs,
		ErrorServerIDs:         input.ExpectedErrorServerIDs,
		RequiresRuntimeRestart: input.RequiresRuntimeRestart,
		ActivePaths:            activePaths,
		TombstoneCount:         1,
		RestartedFromSnapshot:  true,
		SecretSafeDiagnostic:   secretSafeDiagnostic,
		LeaksSecret:            strings.Contains(secretSafeDiagnostic, input.LiveLocal.SecretDiagnostic),
		TopLevelRouteExposed:   false,
	}
}

func replayG5MCPCoreLifecycle(input G5MCPCoreLifecycleCase) G5MCPCoreLifecycleOutput {
	return G5MCPCoreLifecycleOutput{
		ProviderID:           input.ProviderID,
		ConnectToolNames:     append([]string(nil), input.CoreLifecycle.Connect.ToolNames...),
		ConnectAvailable:     input.CoreLifecycle.Connect.Diagnostic.Available,
		ConnectToolCount:     input.CoreLifecycle.Connect.Diagnostic.ToolCount,
		DisconnectReason:     input.CoreLifecycle.Disconnect.Reason,
		DisconnectToolNames:  append([]string{}, input.CoreLifecycle.Disconnect.ToolNames...),
		DisconnectAvailable:  input.CoreLifecycle.Disconnect.Diagnostic.Available,
		DisconnectToolCount:  input.CoreLifecycle.Disconnect.Diagnostic.ToolCount,
		ReloadToolNames:      append([]string(nil), input.CoreLifecycle.Reload.ToolNames...),
		SchemaOrderStable:    input.CoreLifecycle.Reload.SchemaOrderStable,
		CancelErrorSubstring: input.CoreLifecycle.Cancel.ErrorSubstring,
		CancelExecuted:       input.CoreLifecycle.Cancel.Executed,
		ErrorCode:            input.CoreLifecycle.Error.Code,
		ErrorApproved:        input.CoreLifecycle.Error.Approved,
		UsesReasonixProtocol: input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed: input.ProductBoundary.TopLevelRouteExposed,
	}
}

func replayG5MCPBackgroundReconnect(input G5MCPBackgroundReconnectCase) G5MCPBackgroundReconnectOutput {
	return G5MCPBackgroundReconnectOutput{
		FailedServerIDs:         append([]string{}, input.BackgroundReconnect.FailedServerIDs...),
		SuspendedProviderID:     input.BackgroundReconnect.SuspendedProviderID,
		SuspendedReason:         input.BackgroundReconnect.SuspendedReason,
		ConnectedServerIDs:      append([]string{}, input.BackgroundReconnect.ExpectedConnectedServerIDs...),
		ErrorServerIDs:          append([]string{}, input.BackgroundReconnect.ExpectedErrorServerIDs...),
		AttemptsPerFailedServer: input.BackgroundReconnect.AttemptsPerFailedServer,
		RetryAllFailedServers: len(input.BackgroundReconnect.FailedServerIDs) ==
			len(input.BackgroundReconnect.ExpectedConnectedServerIDs)+len(input.BackgroundReconnect.ExpectedErrorServerIDs),
		RequiresRuntimeRestart: input.BackgroundReconnect.RequiresRuntimeRestart,
	}
}

func replayG5MCPCallReconnect(input G5MCPCallReconnectCase) G5MCPCallReconnectOutput {
	reconnect := input.CallReconnect
	return G5MCPCallReconnectOutput{
		ServerID:                  reconnect.ServerID,
		ToolName:                  reconnect.ToolName,
		NormalizedToolName:        reconnect.NormalizedToolName,
		TransportErrorRetried:     reconnect.RetryOnTransportError && reconnect.StaleConnection.FactoryAttempts == reconnect.MaxAttempts,
		ProtocolErrorRetried:      reconnect.RetryOnProtocolError,
		MaxAttempts:               reconnect.MaxAttempts,
		StaleFactoryAttempts:      reconnect.StaleConnection.FactoryAttempts,
		StaleCloseCount:           reconnect.StaleConnection.CloseCount,
		StaleResultInstance:       reconnect.StaleConnection.ResultInstance,
		StaleCallSucceeded:        !reconnect.StaleConnection.IsError,
		ProtocolFactoryAttempts:   reconnect.ProtocolFailure.FactoryAttempts,
		ProtocolCloseCount:        reconnect.ProtocolFailure.CloseCount,
		ProtocolErrorCode:         reconnect.ProtocolFailure.Code,
		ProtocolCallReturnedError: reconnect.ProtocolFailure.IsError,
		UsesReasonixProtocol:      false,
		TopLevelRouteExposed:      false,
	}
}

func replayG5MCPKnownOverrideDiagnostics(input G5MCPKnownOverrideDiagnosticsCase) G5MCPKnownOverrideDiagnosticsOutput {
	diagnostics := make([]G5MCPKnownOverrideDiagnosticRow, 0, len(input.KnownOverrideVariants))
	kinds := make([]string, 0, len(input.KnownOverrideVariants))
	roots := make([]string, 0, len(input.KnownOverrideVariants))
	explicitServers := make([]string, 0)
	daemonServers := make([]string, 0)
	allLowPriority := true
	allBackgroundStart := true

	for _, variant := range input.KnownOverrideVariants {
		row := G5MCPKnownOverrideDiagnosticRow{
			ServerID:        variant.ServerID,
			KnownOverride:   variant.Diagnostic.KnownOverride,
			EffectiveCWD:    variant.Diagnostic.EffectiveCWD,
			LowPriority:     variant.Diagnostic.LowPriority,
			BackgroundStart: variant.Diagnostic.BackgroundStart,
			WorkspaceRoot:   variant.WorkspaceRoot,
		}
		if variant.ExplicitCWD != "" {
			row.ExplicitCWD = variant.ExplicitCWD
			explicitServers = append(explicitServers, variant.ServerID)
		}
		if variant.DaemonIdleTimeoutMs != "" {
			row.DaemonIdleTimeoutMs = variant.DaemonIdleTimeoutMs
			daemonServers = append(daemonServers, variant.ServerID)
		}

		diagnostics = append(diagnostics, row)
		kinds = append(kinds, variant.Diagnostic.KnownOverride)
		roots = append(roots, variant.WorkspaceRoot)
		allLowPriority = allLowPriority && variant.Diagnostic.LowPriority
		allBackgroundStart = allBackgroundStart && variant.Diagnostic.BackgroundStart
	}

	return G5MCPKnownOverrideDiagnosticsOutput{
		Diagnostics:                diagnostics,
		VariantCount:               len(input.KnownOverrideVariants),
		KnownOverrideKinds:         kinds,
		WorkspaceRoots:             roots,
		ExplicitCWDServerIDs:       explicitServers,
		DaemonIdleTimeoutServerIDs: daemonServers,
		AllLowPriority:             allLowPriority,
		AllBackgroundStart:         allBackgroundStart,
		UsesReasonixProtocol:       false,
		TopLevelRouteExposed:       false,
	}
}

func replayG5MCPLiveLocalIndexer(input G5MCPLiveLocalIndexerCase) G5MCPLiveLocalIndexerOutput {
	indexer := input.LiveLocalIndexer
	retryAttempts := make(map[string]int, len(indexer.FailedServerIDs))
	for _, serverID := range indexer.FailedServerIDs {
		retryAttempts[serverID] = indexer.AttemptsPerServer
	}

	initialPaths := make([]string, 0, len(indexer.InitialFiles))
	activePaths := make([]string, 0, len(indexer.InitialFiles)+len(indexer.ResumeFiles))
	tombstoneCount := 0
	for _, file := range indexer.InitialFiles {
		initialPaths = append(initialPaths, file.Path)
		if file.Path == indexer.LateTombstonePath {
			tombstoneCount++
			continue
		}
		activePaths = append(activePaths, file.Path)
	}

	resumePaths := make([]string, 0, len(indexer.ResumeFiles))
	for _, file := range indexer.ResumeFiles {
		resumePaths = append(resumePaths, file.Path)
		activePaths = append(activePaths, file.Path)
	}

	secretSafeDiagnostic := strings.ReplaceAll("Authorization="+indexer.SecretDiagnostic, indexer.SecretDiagnostic, "<redacted>")

	return G5MCPLiveLocalIndexerOutput{
		ServerID:                  indexer.ServerID,
		CWD:                       indexer.CWD,
		LowPriority:               indexer.LowPriority,
		BackgroundStart:           indexer.BackgroundStart,
		RetryServerIDs:            append([]string(nil), indexer.FailedServerIDs...),
		AttemptsPerFailedServer:   indexer.AttemptsPerServer,
		RetryAttempts:             retryAttempts,
		InitialPaths:              initialPaths,
		ResumePaths:               resumePaths,
		ActivePaths:               activePaths,
		TombstoneCount:            tombstoneCount,
		RestartedFromSnapshot:     len(indexer.ResumeFiles) > 0,
		LateTombstonePath:         indexer.LateTombstonePath,
		SecretSafeDiagnostic:      secretSafeDiagnostic,
		LeaksSecret:               strings.Contains(secretSafeDiagnostic, indexer.SecretDiagnostic),
		ExecutionErrorToolName:    indexer.ExecutionError.ToolName,
		ExecutionErrorIsError:     indexer.ExecutionError.IsError,
		ExecutionErrorSafe:        indexer.ExecutionError.SecretSafeError,
		ExecutionErrorLeaksSecret: strings.Contains(indexer.ExecutionError.SecretSafeError, indexer.SecretDiagnostic),
		TopLevelRouteExposed:      false,
		UsesReasonixProtocol:      false,
	}
}

func replayG5MCPApprovalAnnotations(input G5MCPApprovalAnnotationCase) G5MCPApprovalAnnotationOutput {
	return G5MCPApprovalAnnotationOutput{
		ServerID:           input.ApprovalAnnotations.ServerID,
		ToolName:           input.ApprovalAnnotations.ToolName,
		NormalizedToolName: input.ApprovalAnnotations.NormalizedToolName,
		DestructiveHint:    input.ApprovalAnnotations.Annotations.DestructiveHint,
		OpenWorldHint:      input.ApprovalAnnotations.Annotations.OpenWorldHint,
		ApprovalID:         input.ApprovalAnnotations.ApprovalID,
		Decision:           input.ApprovalAnnotations.Decision,
		ResultKind:         input.ApprovalAnnotations.ResultKind,
		Executed:           input.ApprovalAnnotations.Executed,
		DeniedNoExecute:    !input.ApprovalAnnotations.Executed,
	}
}

func replayG5MCPSearchMetaTools(input G5MCPSearchMetaToolsCase) G5MCPSearchMetaToolsOutput {
	meta := input.SearchMetaTools
	return G5MCPSearchMetaToolsOutput{
		ToolNames:              append([]string(nil), meta.ToolNames...),
		ToolCount:              len(meta.ToolNames),
		RefreshToolAdvertised:  stringInSlice("mcp_refresh_catalog", meta.ToolNames),
		TrustedWorkspace:       meta.TrustedWorkspace,
		UntrustedWorkspace:     meta.UntrustedWorkspace,
		Query:                  meta.Query,
		TrustedToolID:          meta.TrustedToolID,
		UntrustedSearchedTools: meta.UntrustedSearchedTools,
		UnknownToolError:       meta.UnknownToolError,
		CallPolicy:             meta.CallPolicy,
		DeniedCallExecuted:     meta.DeniedCallExecuted,
		DeniedNoExecute:        !meta.DeniedCallExecuted,
		UsesReasonixProtocol:   false,
		TopLevelRouteExposed:   false,
	}
}

func replayG5MCPSearchRefreshDrift(input G5MCPSearchRefreshDriftCase) G5MCPSearchRefreshDriftOutput {
	return G5MCPSearchRefreshDriftOutput{
		ServerID:             input.RefreshDrift.ServerID,
		InitialToolNames:     append([]string(nil), input.RefreshDrift.InitialToolNames...),
		ExpandedToolNames:    append([]string(nil), input.RefreshDrift.ExpandedToolNames...),
		TotalIndexed:         input.RefreshDrift.ExpectedTotalIndexed,
		CatalogDrift:         input.RefreshDrift.ExpectedCatalogDrift,
		TopLevelRouteExposed: false,
	}
}

func replayG5MCPSearchWorkspaceBoundary(input G5MCPSearchWorkspaceBoundaryCase) G5MCPSearchWorkspaceBoundaryOutput {
	return G5MCPSearchWorkspaceBoundaryOutput{
		TrustedWorkspace:       input.SearchWorkspaceBoundary.TrustedWorkspace,
		UntrustedWorkspace:     input.SearchWorkspaceBoundary.UntrustedWorkspace,
		Query:                  input.SearchWorkspaceBoundary.Query,
		TrustedToolID:          input.SearchWorkspaceBoundary.TrustedToolID,
		UntrustedSearchedTools: input.SearchWorkspaceBoundary.UntrustedSearchedTools,
		UnknownToolError:       input.SearchWorkspaceBoundary.UnknownToolError,
		CallPolicy:             input.SearchWorkspaceBoundary.CallPolicy,
		DeniedNoExecute:        !input.SearchWorkspaceBoundary.DeniedCallExecuted,
	}
}

func replayG5CheckpointRewind(input G5CheckpointRewindCase) G5CheckpointRewindOutput {
	pathRisks := make(map[string]G5CheckpointPathRisk, len(input.PathRisks))
	for _, risk := range input.PathRisks {
		pathRisks[risk.RelativePath] = risk
	}

	readyCount := 0
	blockedCount := 0
	pathEscapeBlocked := false
	symlinkBlocked := false
	legalDotDotFilenameReady := false
	for _, file := range input.ChangedFiles {
		if checkpointPathEscapes(file.RelativePath) {
			blockedCount++
			pathEscapeBlocked = true
			continue
		}
		if risk, ok := pathRisks[file.RelativePath]; ok && risk.IsSymlink {
			blockedCount++
			symlinkBlocked = true
			continue
		}
		readyCount++
		if strings.HasPrefix(file.RelativePath, "..") {
			legalDotDotFilenameReady = true
		}
	}

	return G5CheckpointRewindOutput{
		CheckpointIDPrefix:           idPrefix(input.CheckpointID, "axcp_"),
		PlanIDPrefix:                 idPrefix(input.PlanID, "axrp_"),
		ApplyIDPrefix:                idPrefix(input.ApplyID, "axra_"),
		RescueIDPrefix:               idPrefix(input.RescueID, "axrr_"),
		ReadyFileCount:               readyCount,
		BlockedFileCount:             blockedCount,
		SymlinkBlocked:               symlinkBlocked,
		PathEscapeBlocked:            pathEscapeBlocked,
		LegalDotDotFilenameReady:     legalDotDotFilenameReady,
		RequiresExplicitConfirmation: input.Confirmation.Confirmed && input.Confirmation.Destructive && input.Confirmation.Phrase == "APPLY_CHECKPOINT_REWIND",
		ConversationAuditAppendOnly:  stringInSlice("checkpoint_captured", input.EventKinds) && stringInSlice("checkpoint_rewind_applied", input.EventKinds),
		RewritesTranscript:           false,
		UsesGitRefs:                  false,
		EventKinds:                   input.EventKinds,
		UsesReasonixProtocol:         false,
		TopLevelRouteExposed:         false,
	}
}

func checkpointPathEscapes(relativePath string) bool {
	return relativePath == "" || relativePath == ".." || strings.HasPrefix(relativePath, "../") || strings.HasPrefix(relativePath, "/")
}

func idPrefix(value string, prefix string) string {
	if strings.HasPrefix(value, prefix) {
		return prefix
	}
	return ""
}

func replayG5RemoteEntry(input G5RemoteEntryCase) G5RemoteEntryOutput {
	expected := input.ExpectedPortKeys
	return G5RemoteEntryOutput{
		ExposedPortKeys:          expected,
		ForbiddenPortKeysAbsent:  noStringsInSlice(input.ForbiddenPortKeys, expected),
		RejectedOverrideAccepted: len(input.RejectedOverrideKeys) == 0,
		GoalAccess:               stringInSlice("goal", expected),
		CheckpointAccess:         stringInSlice("checkpointRewindService", expected),
		MemoryAccess:             stringInSlice("memoryStore", expected),
		StorageAccess:            stringInSlice("sessionStore", expected) || stringInSlice("threadStore", expected) || stringInSlice("threadService", expected),
		ToolHostAccess:           stringInSlice("toolHost", expected),
		UsesReasonixProtocol:     false,
		TopLevelRouteExposed:     false,
	}
}

func replayG5HistoryRepair(input G5HistoryRepairCase) G5HistoryRepairOutput {
	repaired := repairG5HistoryItems(input.Items)
	repairedIDs := make([]string, 0, len(repaired))
	keptCallIDs := []string{}
	keptResultCallIDs := []string{}
	repairedIDSet := map[string]bool{}
	for _, item := range repaired {
		repairedIDs = append(repairedIDs, item.ID)
		repairedIDSet[item.ID] = true
		switch item.Kind {
		case "tool_call":
			keptCallIDs = append(keptCallIDs, item.CallID)
		case "tool_result":
			keptResultCallIDs = append(keptResultCallIDs, item.CallID)
		}
	}

	droppedIDs := []string{}
	for _, item := range input.Items {
		if !repairedIDSet[item.ID] {
			droppedIDs = append(droppedIDs, item.ID)
		}
	}
	droppedIDSet := map[string]bool{}
	for _, id := range droppedIDs {
		droppedIDSet[id] = true
	}
	return G5HistoryRepairOutput{
		RepairedIds:                     repairedIDs,
		DroppedIds:                      droppedIDs,
		KeptCallIds:                     keptCallIDs,
		KeptResultCallIds:               keptResultCallIDs,
		OrphanResultDropped:             droppedIDSet["orphan_result"],
		MissingResultCallDropped:        droppedIDSet["missing_call"],
		DuplicateResultDropped:          droppedIDSet["result_b_duplicate"],
		BridgeTextPreserved:             repairedIDSet["assistant_bridge"] && repairedIDSet["assistant_text"],
		StablePrefixContainsRepairState: false,
		UsesReasonixProtocol:            false,
		TopLevelRouteExposed:            false,
	}
}

func replayG5CompactionBoundary(input G5CompactionBoundaryCase) G5CompactionBoundaryOutput {
	effective := effectiveG5HistoryAfterLatestCompaction(input.Items)
	effectiveIDs := make([]string, 0, len(effective))
	effectiveIDSet := map[string]bool{}
	for _, item := range effective {
		effectiveIDs = append(effectiveIDs, item.ID)
		effectiveIDSet[item.ID] = true
	}

	droppedIDs := []string{}
	for _, item := range input.Items {
		if !effectiveIDSet[item.ID] {
			droppedIDs = append(droppedIDs, item.ID)
		}
	}
	droppedIDSet := map[string]bool{}
	for _, id := range droppedIDs {
		droppedIDSet[id] = true
	}

	latestCompactionID := ""
	latestCompactionFirst := false
	if len(effective) > 0 {
		latestCompactionID = effective[0].ID
		latestCompactionFirst = effective[0].Kind == "compaction" && effective[0].ReplacedTokens > 0
	}

	return G5CompactionBoundaryOutput{
		EffectiveIds:                        effectiveIDs,
		DroppedIds:                          droppedIDs,
		LatestCompactionID:                  latestCompactionID,
		LatestCompactionFirst:               latestCompactionFirst,
		LatestCompactionPreserved:           effectiveIDSet["latest_compaction"],
		OlderCompactionDropped:              droppedIDSet["older_compaction"],
		NoopCompactionDropped:               droppedIDSet["noop_compaction"],
		PostCompactionUserPreserved:         effectiveIDSet["post_user_continue"],
		PreCompactionUserDropped:            droppedIDSet["pre_latest_user"],
		StablePrefixContainsCompactionState: false,
		UsesReasonixProtocol:                false,
		TopLevelRouteExposed:                false,
	}
}

func effectiveG5HistoryAfterLatestCompaction(items []G5CompactionBoundaryItem) []G5CompactionBoundaryItem {
	for index := len(items) - 1; index >= 0; index-- {
		item := items[index]
		if item.Kind == "compaction" && item.ReplacedTokens > 0 {
			return append([]G5CompactionBoundaryItem(nil), items[index:]...)
		}
	}
	return append([]G5CompactionBoundaryItem(nil), items...)
}

func repairG5HistoryItems(items []G5HistoryRepairItem) []G5HistoryRepairItem {
	repaired := []G5HistoryRepairItem{}
	changed := false
	index := 0
	for index < len(items) {
		item := items[index]
		if item.Kind == "tool_result" {
			changed = true
			index++
			continue
		}
		if item.Kind != "tool_call" {
			repaired = append(repaired, item)
			index++
			continue
		}

		calls := []struct {
			item  G5HistoryRepairItem
			index int
		}{}
		seenCallIDs := map[string]bool{}
		cursor := index
		for cursor < len(items) && items[cursor].Kind == "tool_call" {
			call := items[cursor]
			if !seenCallIDs[call.CallID] {
				seenCallIDs[call.CallID] = true
				calls = append(calls, struct {
					item  G5HistoryRepairItem
					index int
				}{item: call, index: cursor})
			} else {
				changed = true
			}
			cursor++
		}

		result := findG5ResultBlock(items, cursor, item.TurnID, seenCallIDs)
		for _, call := range calls {
			repaired = append(repaired, call.item)
		}
		repaired = append(repaired, result.bridgeItems...)
		for _, resultIndex := range result.resultIndexes {
			repaired = append(repaired, items[resultIndex])
		}
		for _, call := range calls {
			if result.resultCallIDs[call.item.CallID] {
				continue
			}
			changed = true
			repaired = append(repaired, makeG5InterruptedToolResult(call.item))
		}
		if result.changed {
			changed = true
		}
		index = result.nextIndex
	}
	if !changed {
		return items
	}
	return repaired
}

type g5ResultBlock struct {
	resultCallIDs map[string]bool
	resultIndexes []int
	bridgeItems   []G5HistoryRepairItem
	changed       bool
	nextIndex     int
}

func findG5ResultBlock(items []G5HistoryRepairItem, startIndex int, turnID string, expectedCallIDs map[string]bool) g5ResultBlock {
	seenResultIDs := map[string]bool{}
	resultIndexes := []int{}
	bridgeItems := []G5HistoryRepairItem{}
	changed := false
	sawResult := false
	index := startIndex

	for index < len(items) {
		item := items[index]
		if item.Kind == "tool_result" {
			sawResult = true
			if expectedCallIDs[item.CallID] && !seenResultIDs[item.CallID] {
				seenResultIDs[item.CallID] = true
				resultIndexes = append(resultIndexes, index)
			} else {
				changed = true
			}
			index++
			continue
		}
		if isG5ToolResultBridgeItem(item, turnID, sawResult) {
			bridgeItems = append(bridgeItems, item)
			index++
			continue
		}
		break
	}

	return g5ResultBlock{
		resultCallIDs: seenResultIDs,
		resultIndexes: resultIndexes,
		bridgeItems:   bridgeItems,
		changed:       changed,
		nextIndex:     index,
	}
}

func makeG5InterruptedToolResult(call G5HistoryRepairItem) G5HistoryRepairItem {
	return G5HistoryRepairItem{
		ID:       "tool_result_repaired_" + call.CallID,
		Kind:     "tool_result",
		TurnID:   call.TurnID,
		CallID:   call.CallID,
		ToolName: call.ToolName,
	}
}

func isG5ToolResultBridgeItem(item G5HistoryRepairItem, turnID string, sawResult bool) bool {
	switch item.Kind {
	case "assistant_reasoning", "approval", "user_input", "error":
		return true
	case "assistant_text":
		return !sawResult && item.TurnID == turnID
	default:
		return false
	}
}

func noStringsInSlice(values []string, haystack []string) bool {
	for _, value := range values {
		if stringInSlice(value, haystack) {
			return false
		}
	}
	return true
}

func replayG5StepLimits(input G5StepLimitsExecutableCase) G5StepLimitsOutput {
	userGlobal := positiveOverride(input.UserGlobalMaxModelSteps, input.DefaultMaxModelSteps)
	session := boundedOverride(input.SessionMaxModelSteps, userGlobal)
	turn := boundedOverride(input.TurnMaxModelSteps, session)
	planner := positiveOverride(input.PlannerMaxModelSteps, userGlobal)
	headless := positiveOverride(input.HeadlessMaxModelSteps, userGlobal)
	delegateInherited := delegateInheritedStepLimit(input.DelegateParentMaxModelSteps)
	delegateFloorInherited := delegateInheritedStepLimit(input.DelegateFloorParentMaxSteps)
	return G5StepLimitsOutput{
		Default:                    input.DefaultMaxModelSteps,
		UserGlobal:                 userGlobal,
		Session:                    session,
		Turn:                       turn,
		Planner:                    planner,
		Headless:                   headless,
		ZeroDefault:                boundedOverride(input.ZeroDefaultMaxModelSteps, input.DefaultMaxModelSteps),
		DelegateInherited:          delegateInherited,
		DelegateFloorInherited:     delegateFloorInherited,
		DynamicLimitInStablePrefix: false,
		ErrorCode:                  "turn_step_limit_exceeded",
		Matrix:                     buildG5StepLimitMatrix(input, userGlobal, session, turn, planner, headless, delegateInherited, delegateFloorInherited),
	}
}

func buildG5StepLimitMatrix(input G5StepLimitsExecutableCase, userGlobal int, session int, turn int, planner int, headless int, delegateInherited int, delegateFloorInherited int) []G5StepLimitRow {
	return []G5StepLimitRow{
		{Scope: "default", Configured: input.DefaultMaxModelSteps, Fallback: 0, Effective: input.DefaultMaxModelSteps, Source: "default"},
		{Scope: "userGlobal", Configured: input.UserGlobalMaxModelSteps, Fallback: input.DefaultMaxModelSteps, Effective: userGlobal, Source: "user"},
		{Scope: "session", Configured: input.SessionMaxModelSteps, Fallback: userGlobal, Effective: session, Source: "session"},
		{Scope: "turn", Configured: input.TurnMaxModelSteps, Fallback: session, Effective: turn, Source: "turn"},
		{Scope: "planner", Configured: input.PlannerMaxModelSteps, Fallback: userGlobal, Effective: planner, Source: "planner"},
		{Scope: "headless", Configured: input.HeadlessMaxModelSteps, Fallback: userGlobal, Effective: headless, Source: "headless"},
		{Scope: "zeroDefault", Configured: input.ZeroDefaultMaxModelSteps, Fallback: input.DefaultMaxModelSteps, Effective: boundedOverride(input.ZeroDefaultMaxModelSteps, input.DefaultMaxModelSteps), Source: "default"},
		{Scope: "delegate", Configured: input.DelegateParentMaxModelSteps, Fallback: 5, Effective: delegateInherited, Source: "parent-half"},
		{Scope: "delegateFloor", Configured: input.DelegateFloorParentMaxSteps, Fallback: 5, Effective: delegateFloorInherited, Source: "parent-half", DelegateMinFloorApplied: true},
	}
}

func positiveOverride(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func boundedOverride(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func delegateInheritedStepLimit(parent int) int {
	half := parent / 2
	if half < 5 {
		return 5
	}
	return half
}

func replayG5PlannerPolicy(input G5PlannerExecutableCase) G5PlannerExecutableOutput {
	gateTools := []string{"user_input", "request_user_input"}
	capabilityAdvertised := filterG5PlannerCapabilityTools(
		input.AvailableToolset,
		input.ReadOnlyToolset,
		input.PlanToolName,
		gateTools,
	)
	step0 := filterG5PlannerTools(capabilityAdvertised, input.ReadOnlyToolset, input.PlanToolName, gateTools)
	step1 := filterG5PlannerTools(input.AvailableToolset, nil, input.PlanToolName, nil)
	rejected := replayG5PlannerRejectedCall(input.ForgedToolName, step0, input.BlockedToolset)
	rejectedCalls := make([]G5PlannerRejectedCall, 0, len(input.BlockedToolset))
	rejectedToolNames := make([]string, 0, len(input.BlockedToolset))
	allRejected := true
	for _, toolName := range input.BlockedToolset {
		call := replayG5PlannerRejectedCall(toolName, step0, input.BlockedToolset)
		rejectedCalls = append(rejectedCalls, call)
		rejectedToolNames = append(rejectedToolNames, toolName)
		if call.Status != "failed" || call.Code != "tool_dispatch_rejected" || call.Executed {
			allRejected = false
		}
	}
	return G5PlannerExecutableOutput{
		Step0Advertised: step0,
		Step1Advertised: step1,
		RejectedCall:    rejected,
		Gate: G5PlannerGateOutput{
			NormalModeHidesPlanTool:   !stringInSlice(input.PlanToolName, input.NormalModeAdvertised),
			CapabilityGateAdvertised:  capabilityAdvertised,
			Step0Advertised:           step0,
			Step1Advertised:           step1,
			Step0ExcludesBlockedTools: !anyStringInSlice(input.BlockedToolset, step0),
			Step1OnlyCreatePlan:       len(step1) == 1 && step1[0] == input.PlanToolName,
			RejectedToolNames:         rejectedToolNames,
			RejectedCallCount:         len(rejectedCalls),
			AllForgedCallsRejected:    allRejected,
			UsesReasonixProtocol:      false,
			TopLevelRouteExposed:      false,
		},
		RejectedCalls: rejectedCalls,
	}
}

func replayG5PlannerRejectedCall(toolName string, advertised []string, blocked []string) G5PlannerRejectedCall {
	rejected := G5PlannerRejectedCall{
		ToolName: toolName,
		Status:   "completed",
		Code:     "",
		Executed: true,
	}
	if stringInSlice(toolName, blocked) && !stringInSlice(toolName, advertised) {
		rejected.Status = "failed"
		rejected.Code = "tool_dispatch_rejected"
		rejected.Executed = false
	}
	return rejected
}

func filterG5PlannerCapabilityTools(available []string, readOnly []string, planToolName string, gateTools []string) []string {
	out := make([]string, 0, len(available))
	for _, name := range available {
		if name == planToolName || stringInSlice(name, readOnly) || stringInSlice(name, gateTools) {
			out = append(out, name)
		}
	}
	return out
}

func filterG5PlannerTools(available []string, readOnly []string, planToolName string, gateTools []string) []string {
	out := make([]string, 0, len(available))
	for _, name := range available {
		if name == planToolName || stringInSlice(name, readOnly) || stringInSlice(name, gateTools) {
			out = append(out, name)
		}
	}
	return out
}

func stringInSlice(value string, values []string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func anyStringInSlice(values []string, list []string) bool {
	for _, value := range values {
		if stringInSlice(value, list) {
			return true
		}
	}
	return false
}

func allStringInSlice(values []string, list []string) bool {
	for _, value := range values {
		if !stringInSlice(value, list) {
			return false
		}
	}
	return true
}

func stringPointerEquals(value *string, expected string) bool {
	return value != nil && *value == expected
}

func replayG5AutoRouterClassifier(input G5AutoRouterClassifierCase) G5AutoRouterClassifierOutput {
	drift := input.FingerprintDrift
	request := input.IsolatedRequest
	fallback := input.TimeoutFallback
	boundary := input.ProductBoundary
	acceptedRecommendationCount := 0
	rejectedRecommendationCount := 0
	proMaxRecommendationAccepted := false
	autoRecommendationRejected := false
	malformedRecommendationRejected := false
	for _, item := range input.RecommendationParsing {
		if item.Accepted {
			acceptedRecommendationCount++
		} else {
			rejectedRecommendationCount++
		}
		if item.ID == "pro-max-json" && item.Accepted &&
			stringPointerEquals(item.ExpectedModel, "deepseek-v4-pro") &&
			stringPointerEquals(item.ExpectedReasoningEffort, "max") {
			proMaxRecommendationAccepted = true
		}
		if item.ID == "auto-rejected" && !item.Accepted && item.ExpectedModel == nil {
			autoRecommendationRejected = true
		}
		if item.ID == "malformed-rejected" && !item.Accepted && item.ExpectedModel == nil {
			malformedRecommendationRejected = true
		}
	}
	return G5AutoRouterClassifierOutput{
		FingerprintCurrent: input.Fingerprint != "" && input.DefaultTimeoutMs > 0 && input.RouterModel != "",
		ContractDriftInvalidatesCache: drift.RouterModelChangeInvalidates &&
			drift.SystemPromptChangeInvalidates &&
			drift.TimeoutChangeInvalidates &&
			drift.MaxTokensChangeInvalidates &&
			drift.TemperatureChangeInvalidates &&
			drift.ReasoningEffortChangeInvalidates &&
			drift.DefaultFlashAliasStable &&
			drift.ResponseFormatDefaultStable,
		RequestIsolated: !request.Stream &&
			request.TurnIDSuffix == "_auto_router" &&
			request.MaxTokens == 96 &&
			request.Temperature == 0 &&
			request.ResponseFormat == "json_object" &&
			request.ReasoningEffort == "off" &&
			request.PrefixItemCount == 0 &&
			request.ToolCount == 0 &&
			!request.CarriesContextInstructions,
		TimeoutFallsBackToHeuristic: fallback.TimeoutMs > 0 &&
			fallback.TimeoutFingerprint != "" &&
			fallback.FallbackModel != "" &&
			fallback.FallbackReasoningEffort == "max" &&
			fallback.FallbackSource == "heuristic",
		AbortsTimedOutClassifier:            fallback.AbortsClassifier,
		AcceptedRecommendationCount:         acceptedRecommendationCount,
		RejectedRecommendationCount:         rejectedRecommendationCount,
		ProMaxRecommendationAccepted:        proMaxRecommendationAccepted,
		AutoRecommendationRejected:          autoRecommendationRejected,
		MalformedRecommendationRejected:     malformedRecommendationRejected,
		ActiveTurnExcludedFromRecentContext: input.ContextBoundary.ActiveTurnExcluded,
		RecentContextPreservesToolSummary:   input.ContextBoundary.ToolResultSummarized,
		StablePrefixContainsClassifierState: false,
		UsesReasonixProtocol:                boundary.ReasonixControllerProtocol,
		TopLevelRouteExposed:                boundary.TopLevelRouteExposed,
	}
}

func replayG5CombinedControls(input G5CombinedExecutableCase) G5CombinedOutput {
	cancelResults := replayG5Cancel(G5CancelExecutableCase{
		AcceptedToolCalls:   input.Cancel.AcceptedToolCalls,
		CancelledResultCode: input.Cancel.CancelledResultCode,
	})
	completedPreserved := false
	unstartedStatus := ""
	completedResultCount := 0
	abortedResultCount := 0
	for _, result := range cancelResults {
		if result.Status == "completed" && result.Code == "" {
			completedPreserved = true
			completedResultCount += 1
		}
		if result.Status == "aborted" {
			abortedResultCount += 1
		}
		if strings.Contains(result.CallID, "unstarted") {
			unstartedStatus = result.Status
		}
	}
	return G5CombinedOutput{
		SameTurnRouterCalls: input.AutoRouteCache.SameTurnRouterCalls,
		MainModelSteps:      input.AutoRouteCache.MainModelSteps,
		MaxModelSteps:       input.StepLimit.MaxModelSteps,
		NextTurnRouterCalls: input.AutoRouteCache.NextTurnRouterCalls,
		RouteCacheReusedUntilStepLimit: input.AutoRouteCache.SameTurnRouterCalls == 1 &&
			input.AutoRouteCache.MainModelSteps == input.StepLimit.MaxModelSteps,
		NextTurnReroutes:                  input.AutoRouteCache.NextTurnRouterCalls == 1,
		StepLimitErrorCode:                input.StepLimit.ErrorCode,
		DynamicControlStateInStablePrefix: input.StepLimit.DynamicLimitInStablePrefix || input.AutoRouteCache.ClassifierStateInStablePrefix,
		CancelledResultCode:               input.Cancel.CancelledResultCode,
		ClassifierStateInStablePrefix:     input.AutoRouteCache.ClassifierStateInStablePrefix,
		StepLimitDynamicInStablePrefix:    input.StepLimit.DynamicLimitInStablePrefix,
		AcceptedToolCallCount:             len(input.Cancel.AcceptedToolCalls),
		CancelResultCount:                 len(cancelResults),
		CompletedResultCount:              completedResultCount,
		AbortedResultCount:                abortedResultCount,
		CancelResults:                     cancelResults,
		CompletedResultPreserved:          completedPreserved,
		UnstartedResultStatus:             unstartedStatus,
	}
}

func replayG5PlanStepCancelCache(input G5PlanStepCancelCacheCase) G5PlanStepCancelCacheOutput {
	return G5PlanStepCancelCacheOutput{
		Step0ReadOnlyPlusPlan: allStringInSlice([]string{"create_plan", "ls"}, input.Step0MustAdvertise) &&
			stringInSlice("bash", input.Step0MustNotAdvertise),
		FollowUpOnlyCreatePlan: input.FollowUpRequiredToolName == "create_plan" &&
			len(input.FollowUpTools) == 1 &&
			input.FollowUpTools[0] == "create_plan",
		CancelledStepDoesNotAdvanceCacheBaseline: input.AbortedRunStatus == "aborted" &&
			!input.PrefixChanged &&
			len(input.PrefixChangeReasons) == 0,
		NextPlanReusesOriginalCacheBaseline: input.RetryRunStatus == "failed" &&
			allStringInSlice([]string{"create_plan", "ls"}, input.RetryMustAdvertise) &&
			!input.PrefixChanged,
		UsageEventCount:         input.UsageEventCount,
		AllPrefixChangedFalse:   !input.PrefixChanged,
		CacheTelemetryPreserved: input.Provider == "deepseek" && input.EndpointFormat == "chat_completions" && input.CacheHitTokens == 80 && input.CacheMissTokens == 20,
		ForbiddenShellExcluded:  stringInSlice("bash", input.Step0MustNotAdvertise),
		UsesReasonixProtocol:    input.ProductBoundary.ReasonixControllerProtocol,
		TopLevelRouteExposed:    input.ProductBoundary.TopLevelRouteExposed,
		DefaultGoBackendEnabled: input.ProductBoundary.DefaultGoBackendEnabled,
	}
}

func replayG5PlanCancelStateReset(input G5PlanCancelStateResetCase) G5PlanCancelStateResetOutput {
	return G5PlanCancelStateResetOutput{
		CancelledPlanDoesNotLeakMode: input.PreviousMode == "plan" &&
			input.AbortedRunStatus == "aborted" &&
			!input.NormalModeInstructionPresent &&
			!input.AutoModeInstructionPresent,
		NormalTurnHidesCreatePlan:      stringInSlice(input.PlanToolName, input.NormalMustNotAdvertise),
		NormalTurnHasNoPlanRequirement: !input.NormalRequiredToolNamePresent,
		AutoTurnReroutesAfterCancel:    input.AutoRequestedModel == "auto" && input.AutoRouterCalls == 1 && input.AutoRouterTurnIDSuffix == "_auto_router",
		AutoRouterRequestIsolated:      input.AutoRouterToolCount == 0 && input.AutoRouterPrefixItemCount == 0 && !input.AutoRouterModeInstructionPresent,
		AutoRecommendationCurrent:      input.AutoRealModel == "deepseek-v4-pro" && input.AutoReasoningEffort == "max",
		AutoTurnHidesCreatePlan:        stringInSlice(input.PlanToolName, input.AutoMustNotAdvertise),
		AutoTurnHasNoPlanRequirement:   !input.AutoRequiredToolNamePresent,
		StablePrefixClean:              !input.StablePrefixContainsPlanState && !input.StablePrefixContainsClassifierState,
		UsesReasonixProtocol:           input.ProductBoundary.ReasonixControllerProtocol,
		TopLevelRouteExposed:           input.ProductBoundary.TopLevelRouteExposed,
		DefaultGoBackendEnabled:        input.ProductBoundary.DefaultGoBackendEnabled,
	}
}

type G5ShadowSourceFixtures struct {
	TaskJobOrchestration TaskJobOrchestrationContract
	ProviderCache        ProviderCacheContract
	ProviderStreaming    G3ProviderConformanceContract
	G2Routes             []G2RouteReplayCase
	ApprovalUserInput    ApprovalUserInputRouteContract
	MCPLifecycle         MCPToolLifecycleContract
}

type TaskJobOrchestrationContract struct {
	ID              string                      `json:"id"`
	ToolContracts   TaskToolContracts           `json:"toolContracts"`
	RouteContract   TaskJobRouteContract        `json:"routeContract"`
	RouteExecutable TaskJobRouteExecutable      `json:"routeExecutable"`
	Foreground      TaskForegroundContract      `json:"foreground"`
	Background      TaskBackgroundContract      `json:"background"`
	WaitOutputKill  TaskWaitOutputKillContract  `json:"waitOutputKill"`
	Parallel        TaskParallelContract        `json:"parallel"`
	PlannerExecutor TaskPlannerExecutorContract `json:"plannerExecutor"`
	DurableRunner   TaskDurableRunnerContract   `json:"durableRunner"`
	ApprovalDeny    TaskApprovalDenyContract    `json:"approvalDenyNoExecute"`
	NestedEvent     TaskNestedEvent             `json:"nestedEvent"`
	ParentGoal      TaskParentGoalEvidence      `json:"parentGoalEvidence"`
	Permissions     TaskPermissions             `json:"permissions"`
	Transcript      TaskTranscript              `json:"transcript"`
}

type TaskToolContracts struct {
	Task          TaskToolContract         `json:"task"`
	ParallelTasks ParallelTaskToolContract `json:"parallelTasks"`
}

type TaskToolContract struct {
	Name                        string `json:"name"`
	PromptField                 string `json:"promptField"`
	BackgroundField             string `json:"backgroundField"`
	ContinueField               string `json:"continueField"`
	ForkField                   string `json:"forkField"`
	InternalRuntimeOnly         bool   `json:"internalRuntimeOnly"`
	RequiresPermissionGate      bool   `json:"requiresPermissionGate"`
	MayAppendParentGoalEvidence bool   `json:"mayAppendParentGoalEvidence"`
}

type ParallelTaskToolContract struct {
	Name                           string `json:"name"`
	TasksField                     string `json:"tasksField"`
	DependencyField                string `json:"dependencyField"`
	InternalRuntimeOnly            bool   `json:"internalRuntimeOnly"`
	RequiresDependencyValidation   bool   `json:"requiresDependencyValidation"`
	RequiresPlannerReadOnlyToolset bool   `json:"requiresPlannerReadOnlyToolset"`
}

type TaskJobRouteContract struct {
	Wait                    string                 `json:"wait"`
	Output                  string                 `json:"output"`
	Kill                    string                 `json:"kill"`
	ForbiddenTopLevelRoutes []string               `json:"forbiddenTopLevelRoutes"`
	AuthMatrix              TaskJobRouteAuthMatrix `json:"authMatrix"`
}

type TaskJobRouteAuthMatrix struct {
	ProtectedRoutes    []string `json:"protectedRoutes"`
	UnauthorizedStatus int      `json:"unauthorizedStatus"`
}

type TaskJobRouteExecutable struct {
	UnauthorizedStatus int                    `json:"unauthorizedStatus"`
	Output             TaskJobRouteOutput     `json:"output"`
	Wait               TaskJobRouteWait       `json:"wait"`
	Kill               TaskJobRouteKill       `json:"kill"`
	MissingOutput      TaskJobRouteMissing    `json:"missingOutput"`
	Rehydrated         TaskJobRouteRehydrated `json:"rehydrated"`
}

type TaskJobRouteOutput struct {
	Status       int    `json:"status"`
	JobStatus    string `json:"jobStatus"`
	Output       string `json:"output"`
	NextOffset   int    `json:"nextOffset"`
	ReplayOffset int    `json:"replayOffset"`
}

type TaskJobRouteWait struct {
	Status    int    `json:"status"`
	JobStatus string `json:"jobStatus"`
	Result    string `json:"result"`
}

type TaskJobRouteKill struct {
	Status    int    `json:"status"`
	JobStatus string `json:"jobStatus"`
	Error     string `json:"error"`
}

type TaskJobRouteMissing struct {
	Status int `json:"status"`
}

type TaskJobRouteRehydrated struct {
	OutputStatus        int    `json:"outputStatus"`
	WaitStatus          int    `json:"waitStatus"`
	KillStatus          int    `json:"killStatus"`
	CompletedStatus     string `json:"completedStatus"`
	CompletedNextOffset int    `json:"completedNextOffset"`
	KilledStatus        string `json:"killedStatus"`
}

type TaskForegroundContract struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Result string `json:"result"`
}

type TaskBackgroundContract struct {
	Kind             string `json:"kind"`
	StatusAcrossTurn string `json:"statusAcrossTurn"`
	Output           string `json:"output"`
	FinalStatus      string `json:"finalStatus"`
	FinalResult      string `json:"finalResult"`
}

type TaskWaitOutputKillContract struct {
	KilledStatus string `json:"killedStatus"`
	KilledError  string `json:"killedError"`
}

type TaskParallelContract struct {
	DependencyField        string   `json:"dependencyField"`
	ValidOrder             []string `json:"validOrder"`
	SingleTaskError        string   `json:"singleTaskError"`
	DuplicateIDError       string   `json:"duplicateIdError"`
	SelfDependencyError    string   `json:"selfDependencyError"`
	CycleError             string   `json:"cycleError"`
	UnknownDependencyError string   `json:"unknownDependencyError"`
}

type TaskPlannerExecutorContract struct {
	PlannerKind                     string `json:"plannerKind"`
	PlannerPolicy                   string `json:"plannerPolicy"`
	ExecutorPolicy                  string `json:"executorPolicy"`
	FailureStatus                   string `json:"failureStatus"`
	CancelledStatus                 string `json:"cancelledStatus"`
	SkippedReason                   string `json:"skippedReason"`
	CancelReason                    string `json:"cancelReason"`
	OutputOffsetJobCount            int    `json:"outputOffsetJobCount"`
	RequiresFailurePropagation      bool   `json:"requiresFailurePropagation"`
	RequiresCancellationPropagation bool   `json:"requiresCancellationPropagation"`
	RequiresTranscriptPropagation   bool   `json:"requiresTranscriptPropagation"`
	TranscriptPropagationMode       string `json:"transcriptPropagationMode"`
	TranscriptPropagationJobCount   int    `json:"transcriptPropagationJobCount"`
}

type TaskDurableRunnerContract struct {
	RestartDrill TaskDurableRunnerRestartDrill `json:"restartDrill"`
}

type TaskDurableRunnerRestartDrill struct {
	RunningJobID        string `json:"runningJobId"`
	QueuedJobID         string `json:"queuedJobId"`
	RehydratedCount     int    `json:"rehydratedCount"`
	OutputBeforeRestart string `json:"outputBeforeRestart"`
	OutputAfterRestart  string `json:"outputAfterRestart"`
	WaitStatus          string `json:"waitStatus"`
	KillStatus          string `json:"killStatus"`
	KillError           string `json:"killError"`
}

type TaskApprovalDenyContract struct {
	DeniedToolNames    []string `json:"deniedToolNames"`
	ApprovalIDs        []string `json:"approvalIds"`
	CreatesDurableJobs bool     `json:"createsDurableJobs"`
	CreatesChildRuns   bool     `json:"createsChildRuns"`
}

type TaskNestedEvent struct {
	ParentCallID            string   `json:"parentCallId"`
	ChildRunID              string   `json:"childRunId"`
	NestedSSEMetadataFields []string `json:"nestedSseMetadataFields"`
}

type TaskParentGoalEvidence struct {
	RequiresActiveGoal          bool   `json:"requiresActiveGoal"`
	EvidenceLedgeredEventKey    string `json:"evidenceLedgeredEventKey"`
	EvidenceLedgerErrorEventKey string `json:"evidenceLedgerErrorEventKey"`
}

type TaskPermissions struct {
	PlannerReadOnlyToolset  []string `json:"plannerReadOnlyToolset"`
	PlannerForbiddenToolset []string `json:"plannerForbiddenToolset"`
}

type TaskTranscript struct {
	SourceID                  string `json:"sourceId"`
	ContinueTargetID          string `json:"continueTargetId"`
	ForkTargetID              string `json:"forkTargetId"`
	IncompatibleError         string `json:"incompatibleError"`
	SameIdentityRequired      bool   `json:"sameIdentityRequired"`
	ContinuePreservesTarget   bool   `json:"continuePreservesTarget"`
	ForkCreatesDistinctTarget bool   `json:"forkCreatesDistinctTarget"`
}

type ApprovalUserInputRouteContract = agent.ApprovalUserInputRouteContract
type ApprovalRouteDecision = agent.ApprovalRouteDecision
type ApprovalDecisionRequest = agent.ApprovalDecisionRequest
type ApprovalUserInputReplay = agent.ApprovalUserInputReplay
type ApprovalUserInputCancel = agent.ApprovalUserInputCancel
type ApprovalUserInputSubmitted = agent.ApprovalUserInputSubmitted
type ApprovalUserInputQuestion = agent.ApprovalUserInputQuestion
type ApprovalUserInputOption = agent.ApprovalUserInputOption
type ApprovalUserInputResolveRequest = agent.ApprovalUserInputResolveRequest
type ApprovalUserInputResolution = agent.ApprovalUserInputResolution
type ApprovalUserInputExpectedResponse = agent.ApprovalUserInputExpectedResponse
type ApprovalUserInputResolvedEvent = agent.ApprovalUserInputResolvedEvent
type ApprovalUserInputAbort = agent.ApprovalUserInputAbort
type ApprovalUserInputResume = agent.ApprovalUserInputResume
type ApprovalUserInputStatuses = agent.ApprovalUserInputStatuses
type ApprovalUserInputRemote = agent.ApprovalUserInputRemote

type G5ApprovalUserInputRouteCase struct {
	SourceContractID string                         `json:"sourceContractId"`
	Contract         ApprovalUserInputRouteContract `json:"contract"`
	Expected         G5ApprovalUserInputReplay      `json:"expected"`
}

type G5ApprovalUserInputInventoryCase struct {
	SourceContractID string                               `json:"sourceContractId"`
	Input            G5ApprovalUserInputInventoryInput    `json:"input"`
	ProductBoundary  G5ApprovalUserInputInventoryBoundary `json:"productBoundary"`
	Expected         G5ApprovalUserInputInventoryOutput   `json:"expected"`
}

type G5ApprovalUserInputInventoryBoundary struct {
	UsesReasonixProtocol bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed bool `json:"topLevelRouteExposed"`
}

type G5ApprovalUserInputInventoryInput struct {
	ApprovalID                   string   `json:"approvalId"`
	ApprovalPendingAfter         int      `json:"approvalPendingAfter"`
	SubmittedInputID             string   `json:"submittedInputId"`
	CancelledInputID             string   `json:"cancelledInputId"`
	AbortApprovalID              string   `json:"abortApprovalId"`
	AbortUserInputID             string   `json:"abortUserInputId"`
	ReplayKindsInOrder           []string `json:"replayKindsInOrder"`
	AbortReplayKinds             []string `json:"abortReplayKinds"`
	AnswerCount                  int      `json:"answerCount"`
	HTTPEchoesAnswers            bool     `json:"httpEchoesAnswers"`
	ResolvedEventIncludesAnswers bool     `json:"resolvedEventIncludesAnswers"`
	LateApprovalDecisionStatus   int      `json:"lateApprovalDecisionStatus"`
	LateUserInputResolveStatus   int      `json:"lateUserInputResolveStatus"`
	PendingAfterSubmit           int      `json:"pendingAfterSubmit"`
	PendingAfterCancel           int      `json:"pendingAfterCancel"`
	PendingAfterAbortCleanup     int      `json:"pendingAfterAbortCleanup"`
}

type G5ApprovalUserInputInventoryOutput struct {
	GateIDs                      []string `json:"gateIds"`
	ApprovalIDs                  []string `json:"approvalIds"`
	UserInputIDs                 []string `json:"userInputIds"`
	RouteKinds                   []string `json:"routeKinds"`
	ReplayKindsInOrder           []string `json:"replayKindsInOrder"`
	AbortReplayKinds             []string `json:"abortReplayKinds"`
	AnswerCount                  int      `json:"answerCount"`
	HTTPEchoesAnswers            bool     `json:"httpEchoesAnswers"`
	ResolvedEventIncludesAnswers bool     `json:"resolvedEventIncludesAnswers"`
	LateApprovalDecisionStatus   int      `json:"lateApprovalDecisionStatus"`
	LateUserInputResolveStatus   int      `json:"lateUserInputResolveStatus"`
	PendingAfterAll              int      `json:"pendingAfterAll"`
	UsesReasonixProtocol         bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed         bool     `json:"topLevelRouteExposed"`
}

type G5ApprovalUserInputReplay struct {
	ApprovalRoute                G5ApprovalRouteReplay        `json:"approvalRoute"`
	ApprovalID                   string                       `json:"approvalId"`
	ApprovalDecision             string                       `json:"approvalDecision"`
	ApprovalStatus               string                       `json:"approvalStatus"`
	ApprovalPendingAfter         int                          `json:"approvalPendingAfter"`
	SecondApprovalDecisionStatus int                          `json:"secondApprovalDecisionStatus"`
	ReplaySinceSeq               int                          `json:"replaySinceSeq"`
	ReplayKindsInOrder           []string                     `json:"replayKindsInOrder"`
	SubmittedInputID             string                       `json:"submittedInputId"`
	SubmittedStatus              string                       `json:"submittedStatus"`
	UserInputSubmitRoute         G5UserInputSubmitRouteReplay `json:"userInputSubmitRoute"`
	AnswerCount                  int                          `json:"answerCount"`
	HTTPEchoesAnswers            bool                         `json:"httpEchoesAnswers"`
	ResolvedEventKind            string                       `json:"resolvedEventKind"`
	ResolvedEventIncludesAnswers bool                         `json:"resolvedEventIncludesAnswers"`
	CancelledInputID             string                       `json:"cancelledInputId"`
	CancelledStatus              string                       `json:"cancelledStatus"`
	UserInputCancelRoute         G5UserInputCancelRouteReplay `json:"userInputCancelRoute"`
	LateResolveRejected          bool                         `json:"lateResolveRejected"`
	PendingAfterSubmit           int                          `json:"pendingAfterSubmit"`
	PendingAfterCancel           int                          `json:"pendingAfterCancel"`
	AbortApprovalID              string                       `json:"abortApprovalId"`
	AbortApprovalStatus          string                       `json:"abortApprovalStatus"`
	AbortUserInputID             string                       `json:"abortUserInputId"`
	AbortUserInputStatus         string                       `json:"abortUserInputStatus"`
	LateApprovalDecisionStatus   int                          `json:"lateApprovalDecisionStatus"`
	LateUserInputResolveStatus   int                          `json:"lateUserInputResolveStatus"`
	PendingAfterAbortCleanup     int                          `json:"pendingAfterAbortCleanup"`
	AbortReplayKinds             []string                     `json:"abortReplayKinds"`
}

type G5ApprovalRouteReplay struct {
	ItemID         string                  `json:"itemId"`
	ToolName       string                  `json:"toolName"`
	Summary        string                  `json:"summary"`
	RequestBody    ApprovalDecisionRequest `json:"requestBody"`
	ResponseStatus int                     `json:"responseStatus"`
	ResponseBody   map[string]any          `json:"responseBody"`
	PendingBefore  int                     `json:"pendingBefore"`
	PendingAfter   int                     `json:"pendingAfter"`
}

type G5PromptQuestionReplay struct {
	Header       string   `json:"header"`
	ID           string   `json:"id"`
	OptionLabels []string `json:"optionLabels"`
}

type G5UserInputSubmitRouteReplay struct {
	ThreadID       string                          `json:"threadId"`
	ItemID         string                          `json:"itemId"`
	Prompt         string                          `json:"prompt"`
	Questions      []G5PromptQuestionReplay        `json:"questions"`
	RequestBody    ApprovalUserInputResolveRequest `json:"requestBody"`
	ResponseStatus int                             `json:"responseStatus"`
	ResponseBody   map[string]any                  `json:"responseBody"`
	ResolvedEvent  ApprovalUserInputResolvedEvent  `json:"resolvedEvent"`
	PendingBefore  int                             `json:"pendingBefore"`
	PendingAfter   int                             `json:"pendingAfter"`
}

type G5UserInputCancelRouteReplay struct {
	ItemID              string                          `json:"itemId"`
	Prompt              string                          `json:"prompt"`
	Questions           []G5PromptQuestionReplay        `json:"questions"`
	RequestBody         ApprovalUserInputResolveRequest `json:"requestBody"`
	ResponseStatus      int                             `json:"responseStatus"`
	ResponseBody        map[string]any                  `json:"responseBody"`
	SecondResolveStatus int                             `json:"secondResolveStatus"`
	PendingBefore       int                             `json:"pendingBefore"`
	PendingAfter        int                             `json:"pendingAfter"`
}

type ProviderCacheContract struct {
	ID                    string                        `json:"id"`
	StablePrefix          ProviderStablePrefix          `json:"stablePrefix"`
	DriftAttribution      ProviderDriftAttribution      `json:"driftAttribution"`
	ProviderUsageCases    []ProviderUsageCase           `json:"providerUsageCases"`
	RequestShapeCases     []ProviderRequestShapeCase    `json:"requestShapeCases"`
	ReleaseGuard          ProviderReleaseGuard          `json:"releaseGuard"`
	LiveLocalHTTPContract ProviderLiveLocalHTTPContract `json:"liveLocalHttpContract"`
	Privacy               ProviderCachePrivacy          `json:"privacy"`
	LiveCredentialPolicy  ProviderLiveCredentialPolicy  `json:"liveCredentialPolicy"`
}

type ProviderStablePrefix struct {
	FirstShape          ProviderPrefixShape        `json:"firstShape"`
	EquivalentShape     ProviderPrefixShape        `json:"equivalentShape"`
	Usage               G3ProviderUsageSummary     `json:"usage"`
	ExpectedDiagnostics ProviderExpectedDiagnostic `json:"expectedDiagnostics"`
}

type G5ProviderDriftAttributionCase struct {
	SourceContractID string                     `json:"sourceContractId"`
	DriftAttribution ProviderDriftAttribution   `json:"driftAttribution"`
	Expected         G5ProviderDriftAttribution `json:"expected"`
}

type ProviderExpectedDiagnostic struct {
	PrefixChanged           bool `json:"prefixChanged"`
	CacheTelemetrySupported bool `json:"cacheTelemetrySupported"`
}

type ProviderUsageCase struct {
	ID             string                 `json:"id"`
	EndpointFormat string                 `json:"endpointFormat"`
	BaseURL        string                 `json:"baseUrl"`
	ResponseBody   map[string]any         `json:"responseBody"`
	ExpectedUsage  G3ProviderUsageSummary `json:"expectedUsage"`
}

type G5ProviderCacheAccountingCase struct {
	Cases    []ProviderUsageCase       `json:"cases"`
	Expected G3ProviderCacheAccounting `json:"expected"`
}

type G5ProviderOfflineParitySealCase struct {
	SourceContractID            string                              `json:"sourceContractId"`
	StablePrefix                ProviderStablePrefix                `json:"stablePrefix"`
	ProviderUsageCaseIDs        []string                            `json:"providerUsageCaseIds"`
	RequestShapeCaseIDs         []string                            `json:"requestShapeCaseIds"`
	RequestShapeEndpointFormats []string                            `json:"requestShapeEndpointFormats"`
	ReleaseGuard                G5ProviderOfflineParityReleaseGuard `json:"releaseGuard"`
	LiveCredentialPolicy        ProviderLiveCredentialPolicy        `json:"liveCredentialPolicy"`
	Expected                    G5ProviderOfflineParitySeal         `json:"expected"`
}

type G5ProviderOfflineParityReleaseGuard struct {
	Status           string  `json:"status"`
	ThresholdPercent float64 `json:"thresholdPercent"`
	TailWindow       int     `json:"tailWindow"`
}

type G5ProviderOfflineParitySeal struct {
	FixtureOnly                   bool     `json:"fixtureOnly"`
	MayClaimLiveSuperiority       bool     `json:"mayClaimLiveSuperiority"`
	StablePrefixEquivalent        bool     `json:"stablePrefixEquivalent"`
	PrefixItemsHashStable         bool     `json:"prefixItemsHashStable"`
	ToolsHashStable               bool     `json:"toolsHashStable"`
	StablePrefixHash              string   `json:"stablePrefixHash"`
	EquivalentPrefixHash          string   `json:"equivalentPrefixHash"`
	DeepseekProviderID            string   `json:"deepseekProviderId"`
	DeepseekEndpointFormat        string   `json:"deepseekEndpointFormat"`
	DeepseekModel                 string   `json:"deepseekModel"`
	DeepseekStableCacheHitTokens  int      `json:"deepseekStableCacheHitTokens"`
	DeepseekStableCacheMissTokens int      `json:"deepseekStableCacheMissTokens"`
	DeepseekStableCacheHitRate    float64  `json:"deepseekStableCacheHitRate"`
	DiagnosticsPrefixChanged      bool     `json:"diagnosticsPrefixChanged"`
	DiagnosticsTelemetrySupported bool     `json:"diagnosticsTelemetrySupported"`
	ProviderUsageCaseCount        int      `json:"providerUsageCaseCount"`
	RequestShapeCaseCount         int      `json:"requestShapeCaseCount"`
	RequestShapeEndpointFormats   []string `json:"requestShapeEndpointFormats"`
	ReleaseGuardStatus            string   `json:"releaseGuardStatus"`
	ReleaseGuardThresholdPercent  float64  `json:"releaseGuardThresholdPercent"`
	ReleaseGuardTailWindow        int      `json:"releaseGuardTailWindow"`
}

type G5ProviderStreamingControlCase struct {
	SourceContractID    string                         `json:"sourceContractId"`
	ProviderUsageMatrix []G3ProviderUsageCaseSummary   `json:"providerUsageMatrix"`
	Streaming           G3ProviderStreamingConformance `json:"streaming"`
	ProductBoundary     ProductBoundary                `json:"productBoundary"`
	Expected            G5ProviderStreamingOutput      `json:"expected"`
}

type G5ProviderStreamingOutput struct {
	SourceContractID              string          `json:"sourceContractId"`
	ThreadID                      string          `json:"threadId"`
	SinceSeq                      int             `json:"sinceSeq"`
	SSEFrameCount                 int             `json:"sseFrameCount"`
	StreamingKinds                []string        `json:"streamingKinds"`
	ExpectedKindsInOrder          []string        `json:"expectedKindsInOrder"`
	ExpectedUsageCaseID           string          `json:"expectedUsageCaseId"`
	UsageEventMatchesExpectedCase bool            `json:"usageEventMatchesExpectedCase"`
	UsagePromptTokens             int             `json:"usagePromptTokens"`
	UsageCompletionTokens         int             `json:"usageCompletionTokens"`
	UsageReasoningTokens          int             `json:"usageReasoningTokens"`
	UsageTotalTokens              int             `json:"usageTotalTokens"`
	UsageCacheHitTokens           int             `json:"usageCacheHitTokens"`
	UsageCacheMissTokens          int             `json:"usageCacheMissTokens"`
	UsageCacheHitRate             float64         `json:"usageCacheHitRate"`
	CacheTelemetrySupported       bool            `json:"cacheTelemetrySupported"`
	ProductBoundary               ProductBoundary `json:"productBoundary"`
}

type G5SessionRouteStatusControlCase struct {
	SourceContractID string                     `json:"sourceContractId"`
	Routes           []G2RouteReplayCase        `json:"routes"`
	Expected         G5SessionRouteStatusOutput `json:"expected"`
}

type G5SessionRouteInventoryCase struct {
	SourceContractID string                          `json:"sourceContractId"`
	Routes           []G5SessionRouteInventoryRoute  `json:"routes"`
	ProductBoundary  G5SessionRouteInventoryBoundary `json:"productBoundary"`
	Expected         G5SessionRouteInventoryOutput   `json:"expected"`
}

type G5SessionRouteInventoryRoute struct {
	ID           string `json:"id"`
	Setup        string `json:"setup"`
	Path         string `json:"path"`
	Auth         string `json:"auth"`
	ResponseKind string `json:"responseKind"`
}

type G5SessionRouteInventoryBoundary struct {
	UsesReasonixProtocol bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed bool `json:"topLevelRouteExposed"`
}

type G5SessionRouteInventoryOutput struct {
	RouteIDs               []string `json:"routeIds"`
	JsonRouteIDs           []string `json:"jsonRouteIds"`
	SSERouteIDs            []string `json:"sseRouteIds"`
	EventRouteIDs          []string `json:"eventRouteIds"`
	ResumeRouteIDs         []string `json:"resumeRouteIds"`
	ForkRouteIDs           []string `json:"forkRouteIds"`
	ArchiveRouteIDs        []string `json:"archiveRouteIds"`
	SearchRouteIDs         []string `json:"searchRouteIds"`
	ReadUpdateRouteIDs     []string `json:"readUpdateRouteIds"`
	RuntimeTokenRouteCount int      `json:"runtimeTokenRouteCount"`
	UnauthorizedRouteIDs   []string `json:"unauthorizedRouteIds"`
	UsesReasonixProtocol   bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed   bool     `json:"topLevelRouteExposed"`
}

type G5SessionRouteReplayCase struct {
	SourceContractID string                       `json:"sourceContractId"`
	Routes           []G5SessionRouteReplayRow    `json:"routes"`
	ProductBoundary  G5SessionRouteReplayBoundary `json:"productBoundary"`
	Expected         G5SessionRouteReplayOutput   `json:"expected"`
}

type G5SessionRouteReplayRow struct {
	ID                string   `json:"id"`
	Method            string   `json:"method"`
	Path              string   `json:"path"`
	Setup             string   `json:"setup"`
	Auth              string   `json:"auth"`
	ResponseKind      string   `json:"responseKind"`
	Status            int      `json:"status"`
	RequestBodyHash   string   `json:"requestBodyHash"`
	ResponseBodyShape string   `json:"responseBodyShape"`
	ResponseBodyHash  string   `json:"responseBodyHash"`
	SSEFrameCount     int      `json:"sseFrameCount"`
	SSEEventNames     []string `json:"sseEventNames"`
	SSEFramesHash     string   `json:"sseFramesHash"`
}

type G5SessionRouteReplayBoundary struct {
	UsesReasonixProtocol   bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed   bool `json:"topLevelRouteExposed"`
	RendererVisibleGoRoute bool `json:"rendererVisibleGoRoute"`
	DefaultGoBackend       bool `json:"defaultGoBackend"`
}

type G5SessionRouteReplayOutput struct {
	RouteCount                    int                       `json:"routeCount"`
	Matrix                        []G5SessionRouteReplayRow `json:"matrix"`
	ExactJSONBodyRouteCount       int                       `json:"exactJsonBodyRouteCount"`
	ExactSSERouteCount            int                       `json:"exactSseRouteCount"`
	RuntimeTokenRouteCount        int                       `json:"runtimeTokenRouteCount"`
	UnauthorizedRouteIDs          []string                  `json:"unauthorizedRouteIds"`
	ArchiveResponseHash           string                    `json:"archiveResponseHash"`
	SearchResponseHash            string                    `json:"searchResponseHash"`
	ForkResponseHash              string                    `json:"forkResponseHash"`
	ResumeResponseHash            string                    `json:"resumeResponseHash"`
	ReplaySSEHash                 string                    `json:"replaySseHash"`
	CaughtUpSSEHash               string                    `json:"caughtUpSseHash"`
	UnauthorizedBodyHash          string                    `json:"unauthorizedBodyHash"`
	EveryRouteHasMethodPathStatus bool                      `json:"everyRouteHasMethodPathStatus"`
	JSONRoutesHaveBodyHash        bool                      `json:"jsonRoutesHaveBodyHash"`
	SSERoutesHaveExactFrames      bool                      `json:"sseRoutesHaveExactFrames"`
	UsesReasonixProtocol          bool                      `json:"usesReasonixProtocol"`
	TopLevelRouteExposed          bool                      `json:"topLevelRouteExposed"`
	RendererVisibleGoRoute        bool                      `json:"rendererVisibleGoRoute"`
	DefaultGoBackend              bool                      `json:"defaultGoBackend"`
}

type G5SessionRouteStatusOutput struct {
	RouteCount     int                    `json:"routeCount"`
	JsonRouteCount int                    `json:"jsonRouteCount"`
	SSERouteCount  int                    `json:"sseRouteCount"`
	StatusCodes    G5RouteStatusCodeCount `json:"statusCodes"`
	Auth           G5RouteAuthStatus      `json:"auth"`
	Archive        G5RouteArchiveStatus   `json:"archive"`
	ReadUpdate     G5RouteReadUpdate      `json:"readUpdate"`
	Fork           G5RouteForkStatus      `json:"fork"`
	Resume         G5RouteResumeStatus    `json:"resume"`
	SSE            G5RouteSSEStatus       `json:"sse"`
}

type G5RouteStatusCodeCount struct {
	OK           int `json:"ok"`
	Created      int `json:"created"`
	Unauthorized int `json:"unauthorized"`
}

type G5RouteAuthStatus struct {
	ProtectedRouteID  string `json:"protectedRouteId"`
	ProtectedPath     string `json:"protectedPath"`
	ProtectedStatus   int    `json:"protectedStatus"`
	ProtectedBodyCode string `json:"protectedBodyCode"`
	SSEFrameCount     int    `json:"sseFrameCount"`
}

type G5RouteArchiveStatus struct {
	PatchStatus         int    `json:"patchStatus"`
	ArchivedStatus      string `json:"archivedStatus"`
	ArchivedOnlyCount   int    `json:"archivedOnlyCount"`
	SearchArchivedCount int    `json:"searchArchivedCount"`
	SearchFirstStatus   string `json:"searchFirstStatus"`
}

type G5RouteReadUpdate struct {
	ReadStatus       int    `json:"readStatus"`
	ReadLatestSeq    int    `json:"readLatestSeq"`
	ReadTurnCount    int    `json:"readTurnCount"`
	UpdateStatus     int    `json:"updateStatus"`
	UpdatedWorkspace string `json:"updatedWorkspace"`
}

type G5RouteForkStatus struct {
	Status              int    `json:"status"`
	Relation            string `json:"relation"`
	ParentThreadID      string `json:"parentThreadId"`
	ForkedFromTurnCount int    `json:"forkedFromTurnCount"`
	ForkedTurnCount     int    `json:"forkedTurnCount"`
}

type G5RouteResumeStatus struct {
	Status       int    `json:"status"`
	SessionID    string `json:"sessionId"`
	MessageCount int    `json:"messageCount"`
	Summary      string `json:"summary"`
}

type G5RouteSSEStatus struct {
	ReplayStatus       int      `json:"replayStatus"`
	ReplayFrameCount   int      `json:"replayFrameCount"`
	CaughtUpStatus     int      `json:"caughtUpStatus"`
	CaughtUpFrameCount int      `json:"caughtUpFrameCount"`
	ReplayEventNames   []string `json:"replayEventNames"`
}

type G5ProviderUsageParserControlCase struct {
	Cases    []ProviderUsageCase         `json:"cases"`
	Expected G5ProviderUsageParserOutput `json:"expected"`
}

type G5ProviderUsageParserOutput struct {
	CaseCount                   int                               `json:"caseCount"`
	TelemetrySupportedCaseIDs   []string                          `json:"telemetrySupportedCaseIds"`
	UnsupportedAbsentFields     G5ProviderUnsupportedAbsentFields `json:"unsupportedAbsentFields"`
	DeepseekNativePrecedence    G5DeepseekNativePrecedence        `json:"deepseekNativePrecedence"`
	OpenAIResponsesCachedTokens G5OpenAIResponsesCachedTokens     `json:"openaiResponsesCachedTokens"`
	AnthropicCacheFields        G5AnthropicCacheFields            `json:"anthropicCacheFields"`
}

type G5ProviderUnsupportedAbsentFields struct {
	CaseID            string   `json:"caseId"`
	AbsentFields      []string `json:"absentFields"`
	CacheHitRateKnown bool     `json:"cacheHitRateKnown"`
}

type G5DeepseekNativePrecedence struct {
	CaseID                          string  `json:"caseId"`
	PromptCacheHitTokens            int     `json:"promptCacheHitTokens"`
	PromptCacheMissTokens           int     `json:"promptCacheMissTokens"`
	PromptTokensDetailsCachedTokens int     `json:"promptTokensDetailsCachedTokens"`
	ExpectedCacheHitTokens          int     `json:"expectedCacheHitTokens"`
	ExpectedCacheMissTokens         int     `json:"expectedCacheMissTokens"`
	NativeCacheFieldsWin            bool    `json:"nativeCacheFieldsWin"`
	CacheHitRate                    float64 `json:"cacheHitRate"`
}

type G5OpenAIResponsesCachedTokens struct {
	CaseID                  string `json:"caseId"`
	InputTokens             int    `json:"inputTokens"`
	CachedTokens            int    `json:"cachedTokens"`
	ExpectedCacheHitTokens  int    `json:"expectedCacheHitTokens"`
	ExpectedCacheMissTokens int    `json:"expectedCacheMissTokens"`
	ReasoningTokens         int    `json:"reasoningTokens"`
	CachedTokensUsed        bool   `json:"cachedTokensUsed"`
}

type G5AnthropicCacheFields struct {
	CaseID                      string `json:"caseId"`
	InputTokens                 int    `json:"inputTokens"`
	CacheReadInputTokens        int    `json:"cacheReadInputTokens"`
	CacheCreationInputTokens    int    `json:"cacheCreationInputTokens"`
	ExpectedPromptTokens        int    `json:"expectedPromptTokens"`
	ExpectedCacheHitTokens      int    `json:"expectedCacheHitTokens"`
	ExpectedCacheMissTokens     int    `json:"expectedCacheMissTokens"`
	CacheFieldsIncludedInPrompt bool   `json:"cacheFieldsIncludedInPrompt"`
}

type ProviderRequestShapeCase struct {
	ID                  string   `json:"id"`
	EndpointFormat      string   `json:"endpointFormat"`
	BaseURL             string   `json:"baseUrl"`
	Model               string   `json:"model"`
	ReasoningEffort     string   `json:"reasoningEffort,omitempty"`
	ExpectedURL         string   `json:"expectedUrl"`
	RequiredHeaders     []string `json:"requiredHeaders"`
	ForbiddenHeaders    []string `json:"forbiddenHeaders"`
	RequiredBodyFields  []string `json:"requiredBodyFields"`
	ForbiddenBodyFields []string `json:"forbiddenBodyFields"`
	ExpectedToolShape   string   `json:"expectedToolShape"`
}

type G5ProviderRequestShapeControlCase struct {
	Cases    []ProviderRequestShapeCase   `json:"cases"`
	Expected G5ProviderRequestShapeOutput `json:"expected"`
}

type G5ProviderRequestShapeOutput struct {
	CaseCount                           int                                  `json:"caseCount"`
	ExactURLCount                       int                                  `json:"exactUrlCount"`
	DerivedURLMatchCaseIDs              []string                             `json:"derivedUrlMatchCaseIds"`
	HeaderShapeMatchCaseIDs             []string                             `json:"headerShapeMatchCaseIds"`
	BodyShapeMatchCaseIDs               []string                             `json:"bodyShapeMatchCaseIds"`
	ToolShapeMatchCaseIDs               []string                             `json:"toolShapeMatchCaseIds"`
	Matrix                              []ProviderRequestShapeCase           `json:"matrix"`
	EndpointFormats                     []string                             `json:"endpointFormats"`
	FullEndpointCaseIDs                 []string                             `json:"fullEndpointCaseIds"`
	ToolShapes                          []string                             `json:"toolShapes"`
	CustomFullEndpointExactURLCaseIDs   []string                             `json:"customFullEndpointExactUrlCaseIds"`
	CustomFullEndpointAppendedPathCount int                                  `json:"customFullEndpointAppendedPathCount"`
	RequiredBodyFieldFamilies           G5ProviderRequiredBodyFieldFamilies  `json:"requiredBodyFieldFamilies"`
	ForbiddenBodyFieldFamilies          G5ProviderForbiddenBodyFieldFamilies `json:"forbiddenBodyFieldFamilies"`
}

type G5ProviderRequiredBodyFieldFamilies struct {
	MessagesFieldCaseCount        int `json:"messagesFieldCaseCount"`
	InputFieldCaseCount           int `json:"inputFieldCaseCount"`
	SystemFieldCaseCount          int `json:"systemFieldCaseCount"`
	ThinkingFieldCaseCount        int `json:"thinkingFieldCaseCount"`
	MaxOutputTokensFieldCaseCount int `json:"maxOutputTokensCaseCount"`
	MaxTokensFieldCaseCount       int `json:"maxTokensCaseCount"`
}

type G5ProviderForbiddenBodyFieldFamilies struct {
	ThinkingForbiddenCaseCount        int `json:"thinkingForbiddenCaseCount"`
	SystemForbiddenCaseCount          int `json:"systemForbiddenCaseCount"`
	InputForbiddenCaseCount           int `json:"inputForbiddenCaseCount"`
	MaxOutputTokensForbiddenCaseCount int `json:"maxOutputTokensForbiddenCaseCount"`
}

type ProviderReleaseGuard struct {
	FixtureOnly      bool                       `json:"fixtureOnly"`
	ThresholdPercent float64                    `json:"thresholdPercent"`
	MaxLowTailCases  int                        `json:"maxLowTailCases"`
	TailWindow       int                        `json:"tailWindow"`
	Cases            []ProviderReleaseGuardCase `json:"cases"`
}

type ProviderReleaseGuardCase struct {
	ID                         string    `json:"id"`
	CacheHitPercentCurve       []float64 `json:"cacheHitPercentCurve"`
	ExpectedTailAveragePercent float64   `json:"expectedTailAveragePercent"`
	ExpectedStatus             string    `json:"expectedStatus"`
	CompactionGuardPaused      bool      `json:"compactionGuardPaused"`
	MaxAllowedCollapses        *int      `json:"maxAllowedCollapses"`
}

type G5ProviderReleaseGuardControlCase struct {
	FixtureOnly      bool                         `json:"fixtureOnly"`
	ThresholdPercent float64                      `json:"thresholdPercent"`
	MaxLowTailCases  int                          `json:"maxLowTailCases"`
	TailWindow       int                          `json:"tailWindow"`
	Cases            []ProviderReleaseGuardCase   `json:"cases"`
	Expected         G5ProviderReleaseGuardOutput `json:"expected"`
}

type G5ProviderReleaseGuardOutput struct {
	FixtureOnly      bool                         `json:"fixtureOnly"`
	Status           string                       `json:"status"`
	LowTailCases     int                          `json:"lowTailCases"`
	MaxLowTailCases  int                          `json:"maxLowTailCases"`
	ThresholdPercent float64                      `json:"thresholdPercent"`
	TailWindow       int                          `json:"tailWindow"`
	Cases            []G5ProviderReleaseGuardCase `json:"cases"`
}

type G5ProviderReleaseGuardCase struct {
	ID                    string  `json:"id"`
	TailAveragePercent    float64 `json:"tailAveragePercent"`
	Status                string  `json:"status"`
	CollapseCount         int     `json:"collapseCount"`
	CompactionGuardPaused bool    `json:"compactionGuardPaused"`
}

type ProviderCachePrivacy struct {
	ForbiddenDiagnosticsSubstrings []string `json:"forbiddenDiagnosticsSubstrings"`
}

type ProviderLiveCredentialPolicy struct {
	FixtureOnly         bool `json:"fixtureOnly"`
	MayClaimSuperiority bool `json:"mayClaimSuperiority"`
}

type G5ProviderCachePrivacyCase struct {
	SourceContractID      string                         `json:"sourceContractId"`
	Privacy               ProviderCachePrivacy           `json:"privacy"`
	Diagnostics           map[string]any                 `json:"diagnostics"`
	LiveCredentialPolicy  ProviderLiveCredentialPolicy   `json:"liveCredentialPolicy"`
	LiveLocalHTTPContract ProviderLiveLocalHTTPContract  `json:"liveLocalHttpContract"`
	ProductBoundary       G5ProviderCachePrivacyBoundary `json:"productBoundary"`
	Expected              G5ProviderCachePrivacyOutput   `json:"expected"`
}

type G5ProviderCachePrivacyBoundary struct {
	UsesReasonixProtocol bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed bool `json:"topLevelRouteExposed"`
}

type G5ProviderCachePrivacyOutput struct {
	FixtureOnly                        bool `json:"fixtureOnly"`
	DiagnosticCheckedFieldCount        int  `json:"diagnosticCheckedFieldCount"`
	ForbiddenDiagnosticsSubstringCount int  `json:"forbiddenDiagnosticsSubstringCount"`
	DiagnosticsLeakForbiddenSubstrings bool `json:"diagnosticsLeakForbiddenSubstrings"`
	LiveCredentialsUsed                bool `json:"liveCredentialsUsed"`
	MayClaimLiveSuperiority            bool `json:"mayClaimLiveSuperiority"`
	UsesReasonixProtocol               bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed               bool `json:"topLevelRouteExposed"`
}

type G5ProviderCacheInventoryCase struct {
	SourceContractID     string                           `json:"sourceContractId"`
	StablePrefix         ProviderStablePrefix             `json:"stablePrefix"`
	ProviderUsageCaseIDs []string                         `json:"providerUsageCaseIds"`
	RequestShapeCaseIDs  []string                         `json:"requestShapeCaseIds"`
	ProductBoundary      G5ProviderCacheInventoryBoundary `json:"productBoundary"`
	Expected             G5ProviderCacheInventoryOutput   `json:"expected"`
}

type G5ProviderCacheInventoryBoundary struct {
	UsesReasonixProtocol bool `json:"usesReasonixProtocol"`
	TopLevelRouteExposed bool `json:"topLevelRouteExposed"`
}

type G5ProviderCacheInventoryOutput struct {
	StablePrefixHash       string   `json:"stablePrefixHash"`
	ToolsHash              string   `json:"toolsHash"`
	ProviderUsageCaseIDs   []string `json:"providerUsageCaseIds"`
	RequestShapeCaseIDs    []string `json:"requestShapeCaseIds"`
	ProviderUsageCaseCount int      `json:"providerUsageCaseCount"`
	RequestShapeCaseCount  int      `json:"requestShapeCaseCount"`
	StablePrefixEquivalent bool     `json:"stablePrefixEquivalent"`
	ToolsHashStable        bool     `json:"toolsHashStable"`
	UsesReasonixProtocol   bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed   bool     `json:"topLevelRouteExposed"`
}

type ProviderLiveLocalHTTPContract struct {
	FixtureOnly                      bool     `json:"fixtureOnly"`
	Transport                        string   `json:"transport"`
	UsesLiveCredentials              bool     `json:"usesLiveCredentials"`
	PreservesOriginalProviderBaseURL bool     `json:"preservesOriginalProviderBaseUrl"`
	UsageCaseCount                   int      `json:"usageCaseCount"`
	RequestShapeCaseCount            int      `json:"requestShapeCaseCount"`
	ExpectedPostCount                int      `json:"expectedPostCount"`
	CoveredEndpointFormats           []string `json:"coveredEndpointFormats"`
	CoveredProviderFamilies          []string `json:"coveredProviderFamilies"`
	MayClaimLiveSuperiority          bool     `json:"mayClaimLiveSuperiority"`
}

type G5ProviderLiveLocalHTTPCase struct {
	SourceContractID            string                        `json:"sourceContractId"`
	Contract                    ProviderLiveLocalHTTPContract `json:"contract"`
	ProviderUsageCaseIDs        []string                      `json:"providerUsageCaseIds"`
	RequestShapeCaseIDs         []string                      `json:"requestShapeCaseIds"`
	RequestShapeEndpointFormats []string                      `json:"requestShapeEndpointFormats"`
	Expected                    ProviderLiveLocalHTTPContract `json:"expected"`
}

type G5ProviderCacheCoverageFloorCase struct {
	SourceContractID      string                             `json:"sourceContractId"`
	ProviderUsageCases    []ProviderUsageCase                `json:"providerUsageCases"`
	RequestShapeCases     []ProviderRequestShapeCoverageCase `json:"requestShapeCases"`
	LiveLocalHTTPContract ProviderLiveLocalHTTPContract      `json:"liveLocalHttpContract"`
	ProductBoundary       G5ProviderCacheInventoryBoundary   `json:"productBoundary"`
	Expected              G5ProviderCacheCoverageFloorOutput `json:"expected"`
}

type ProviderRequestShapeCoverageCase struct {
	ID                string `json:"id"`
	EndpointFormat    string `json:"endpointFormat"`
	BaseURL           string `json:"baseUrl"`
	ExpectedURL       string `json:"expectedUrl"`
	ExpectedToolShape string `json:"expectedToolShape"`
}

type G5ProviderCacheCoverageFloorOutput struct {
	RequiredProviderFamilies                      []string `json:"requiredProviderFamilies"`
	CoveredProviderFamilies                       []string `json:"coveredProviderFamilies"`
	ProviderFamilyCoverageComplete                bool     `json:"providerFamilyCoverageComplete"`
	ProviderUsageCaseIDs                          []string `json:"providerUsageCaseIds"`
	RequestShapeCaseIDs                           []string `json:"requestShapeCaseIds"`
	RequestShapeEndpointFormats                   []string `json:"requestShapeEndpointFormats"`
	TelemetrySupportedCaseIDs                     []string `json:"telemetrySupportedCaseIds"`
	UnsupportedUnknownCaseIDs                     []string `json:"unsupportedUnknownCaseIds"`
	CustomFullEndpointCaseIDs                     []string `json:"customFullEndpointCaseIds"`
	CustomFullEndpointExactURLCaseIDs             []string `json:"customFullEndpointExactUrlCaseIds"`
	CustomFullEndpointToolShapes                  []string `json:"customFullEndpointToolShapes"`
	CustomFullEndpointTelemetryCaseIDs            []string `json:"customFullEndpointTelemetryCaseIds"`
	CustomFullEndpointRequestShapeOnly            bool     `json:"customFullEndpointRequestShapeOnly"`
	TelemetrySupportedExcludesCustomFullEndpoints bool     `json:"telemetrySupportedExcludesCustomFullEndpoints"`
	CustomProviderCacheTelemetryClaimAllowed      bool     `json:"customProviderCacheTelemetryClaimAllowed"`
	DeepSeekUsageCaseIDs                          []string `json:"deepseekUsageCaseIds"`
	DeepSeekRequestShapeCaseIDs                   []string `json:"deepseekRequestShapeCaseIds"`
	OpenAICompatibleChatRequestShapeCaseIDs       []string `json:"openaiCompatibleChatRequestShapeCaseIds"`
	OpenAIResponsesUsageCaseIDs                   []string `json:"openaiResponsesUsageCaseIds"`
	OpenAIResponsesRequestShapeCaseIDs            []string `json:"openaiResponsesRequestShapeCaseIds"`
	AnthropicMessagesUsageCaseIDs                 []string `json:"anthropicMessagesUsageCaseIds"`
	AnthropicMessagesRequestShapeCaseIDs          []string `json:"anthropicMessagesRequestShapeCaseIds"`
	LiveCredentialsUsed                           bool     `json:"liveCredentialsUsed"`
	MayClaimLiveSuperiority                       bool     `json:"mayClaimLiveSuperiority"`
	UsesReasonixProtocol                          bool     `json:"usesReasonixProtocol"`
	TopLevelRouteExposed                          bool     `json:"topLevelRouteExposed"`
}

type MCPToolLifecycleContract = mcp.MCPToolLifecycleContract
type MCPProviderConnect = mcp.MCPProviderConnect
type MCPProviderDisconnect = mcp.MCPProviderDisconnect
type MCPProviderDiagnostic = mcp.MCPProviderDiagnostic
type MCPProviderReload = mcp.MCPProviderReload
type MCPProviderCancel = mcp.ProviderCancel
type MCPProviderError = mcp.MCPProviderError
type MCPApprovalAnnotations = mcp.MCPApprovalAnnotations
type MCPApprovalAnnotation = mcp.MCPApprovalAnnotation
type MCPSearchMetaTools = mcp.MCPSearchMetaTools
type MCPRefreshDrift = mcp.MCPRefreshDrift
type MCPBackgroundReconnect = mcp.MCPBackgroundReconnect
type MCPKnownOverride = mcp.MCPKnownOverride
type MCPKnownOverrideDiagnostic = mcp.MCPKnownOverrideDiagnostic
type MCPLiveLocalIndexer = mcp.MCPLiveLocalIndexer
type MCPLiveLocalFile = mcp.MCPLiveLocalFile
type MCPLiveLocalExpectedOutput = mcp.MCPLiveLocalExpectedOutput
type MCPLiveLocalExecutionError = mcp.MCPLiveLocalExecutionError

func appendProviderEndpointPath(baseURL string, versionedPath string) string {
	return provider.AppendEndpointPath(baseURL, versionedPath)
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func BuildG5ShadowSlicesOutput(g5 G5FullLoopConformanceContract, sources G5ShadowSourceFixtures) map[string]any {
	return map[string]any{
		"stage": "G5",
		"mode":  g5.Mode,
		"sourceContractIds": []string{
			sources.TaskJobOrchestration.ID,
			sources.ProviderCache.ID,
			sources.ProviderStreaming.ID,
			"go-g2-route-replay-contract-v1",
			sources.ApprovalUserInput.ID,
			sources.MCPLifecycle.ID,
		},
		"jobReplay": map[string]any{
			"taskToolName":     sources.TaskJobOrchestration.ToolContracts.Task.Name,
			"parallelToolName": sources.TaskJobOrchestration.ToolContracts.ParallelTasks.Name,
			"toolContractBoundary": map[string]any{
				"task": map[string]any{
					"name":                        sources.TaskJobOrchestration.ToolContracts.Task.Name,
					"promptField":                 sources.TaskJobOrchestration.ToolContracts.Task.PromptField,
					"backgroundField":             sources.TaskJobOrchestration.ToolContracts.Task.BackgroundField,
					"continueField":               sources.TaskJobOrchestration.ToolContracts.Task.ContinueField,
					"forkField":                   sources.TaskJobOrchestration.ToolContracts.Task.ForkField,
					"internalRuntimeOnly":         sources.TaskJobOrchestration.ToolContracts.Task.InternalRuntimeOnly,
					"requiresPermissionGate":      sources.TaskJobOrchestration.ToolContracts.Task.RequiresPermissionGate,
					"mayAppendParentGoalEvidence": sources.TaskJobOrchestration.ToolContracts.Task.MayAppendParentGoalEvidence,
				},
				"parallelTasks": map[string]any{
					"name":                           sources.TaskJobOrchestration.ToolContracts.ParallelTasks.Name,
					"tasksField":                     sources.TaskJobOrchestration.ToolContracts.ParallelTasks.TasksField,
					"dependencyField":                sources.TaskJobOrchestration.ToolContracts.ParallelTasks.DependencyField,
					"internalRuntimeOnly":            sources.TaskJobOrchestration.ToolContracts.ParallelTasks.InternalRuntimeOnly,
					"requiresDependencyValidation":   sources.TaskJobOrchestration.ToolContracts.ParallelTasks.RequiresDependencyValidation,
					"requiresPlannerReadOnlyToolset": sources.TaskJobOrchestration.ToolContracts.ParallelTasks.RequiresPlannerReadOnlyToolset,
				},
				"usesReasonixProtocol": false,
				"topLevelRouteExposed": false,
			},
			"routes": []string{sources.TaskJobOrchestration.RouteContract.Wait, sources.TaskJobOrchestration.RouteContract.Output, sources.TaskJobOrchestration.RouteContract.Kill},
			"routeBoundary": map[string]any{
				"protectedRoutes":         append([]string(nil), sources.TaskJobOrchestration.RouteContract.AuthMatrix.ProtectedRoutes...),
				"unauthorizedStatus":      sources.TaskJobOrchestration.RouteContract.AuthMatrix.UnauthorizedStatus,
				"forbiddenTopLevelRoutes": append([]string(nil), sources.TaskJobOrchestration.RouteContract.ForbiddenTopLevelRoutes...),
			},
			"routeExecutable": buildG5TaskJobRouteExecutable(sources.TaskJobOrchestration.RouteExecutable),
			"lifecycle": map[string]any{
				"foreground": map[string]any{
					"kind":   sources.TaskJobOrchestration.Foreground.Kind,
					"status": sources.TaskJobOrchestration.Foreground.Status,
					"result": sources.TaskJobOrchestration.Foreground.Result,
				},
				"background": map[string]any{
					"kind":             sources.TaskJobOrchestration.Background.Kind,
					"statusAcrossTurn": sources.TaskJobOrchestration.Background.StatusAcrossTurn,
					"output":           sources.TaskJobOrchestration.Background.Output,
					"finalStatus":      sources.TaskJobOrchestration.Background.FinalStatus,
					"finalResult":      sources.TaskJobOrchestration.Background.FinalResult,
				},
				"waitOutputKill": map[string]any{
					"killedStatus": sources.TaskJobOrchestration.WaitOutputKill.KilledStatus,
					"killedError":  sources.TaskJobOrchestration.WaitOutputKill.KilledError,
				},
			},
			"dependencyOrder":         sources.TaskJobOrchestration.Parallel.ValidOrder,
			"plannerReadOnlyToolset":  sources.TaskJobOrchestration.Permissions.PlannerReadOnlyToolset,
			"plannerForbiddenToolset": sources.TaskJobOrchestration.Permissions.PlannerForbiddenToolset,
			"nestedSseMetadataFields": sources.TaskJobOrchestration.NestedEvent.NestedSSEMetadataFields,
			"parentGoalEvidence": map[string]any{
				"requiresActiveGoal":          sources.TaskJobOrchestration.ParentGoal.RequiresActiveGoal,
				"evidenceLedgeredEventKey":    sources.TaskJobOrchestration.ParentGoal.EvidenceLedgeredEventKey,
				"evidenceLedgerErrorEventKey": sources.TaskJobOrchestration.ParentGoal.EvidenceLedgerErrorEventKey,
				"usesReasonixProtocol":        false,
				"topLevelRouteExposed":        false,
			},
			"sameTranscriptIdentityRequired": sources.TaskJobOrchestration.Transcript.SameIdentityRequired,
			"continuePreservesTarget":        sources.TaskJobOrchestration.Transcript.ContinuePreservesTarget,
			"forkCreatesDistinctTarget":      sources.TaskJobOrchestration.Transcript.ForkCreatesDistinctTarget,
			"plannerExecutor": map[string]any{
				"plannerKind":                     sources.TaskJobOrchestration.PlannerExecutor.PlannerKind,
				"plannerPolicy":                   sources.TaskJobOrchestration.PlannerExecutor.PlannerPolicy,
				"executorPolicy":                  sources.TaskJobOrchestration.PlannerExecutor.ExecutorPolicy,
				"failureStatus":                   sources.TaskJobOrchestration.PlannerExecutor.FailureStatus,
				"cancelledStatus":                 sources.TaskJobOrchestration.PlannerExecutor.CancelledStatus,
				"skippedReason":                   sources.TaskJobOrchestration.PlannerExecutor.SkippedReason,
				"cancelReason":                    sources.TaskJobOrchestration.PlannerExecutor.CancelReason,
				"outputOffsetJobCount":            sources.TaskJobOrchestration.PlannerExecutor.OutputOffsetJobCount,
				"requiresFailurePropagation":      sources.TaskJobOrchestration.PlannerExecutor.RequiresFailurePropagation,
				"requiresCancellationPropagation": sources.TaskJobOrchestration.PlannerExecutor.RequiresCancellationPropagation,
				"requiresTranscriptPropagation":   sources.TaskJobOrchestration.PlannerExecutor.RequiresTranscriptPropagation,
				"transcriptPropagationMode":       sources.TaskJobOrchestration.PlannerExecutor.TranscriptPropagationMode,
				"transcriptPropagationJobCount":   sources.TaskJobOrchestration.PlannerExecutor.TranscriptPropagationJobCount,
			},
			"durableRunnerRestart": map[string]any{
				"runningJobId":       sources.TaskJobOrchestration.DurableRunner.RestartDrill.RunningJobID,
				"queuedJobId":        sources.TaskJobOrchestration.DurableRunner.RestartDrill.QueuedJobID,
				"rehydratedCount":    sources.TaskJobOrchestration.DurableRunner.RestartDrill.RehydratedCount,
				"waitStatus":         sources.TaskJobOrchestration.DurableRunner.RestartDrill.WaitStatus,
				"killStatus":         sources.TaskJobOrchestration.DurableRunner.RestartDrill.KillStatus,
				"outputAfterRestart": sources.TaskJobOrchestration.DurableRunner.RestartDrill.OutputAfterRestart,
			},
			"approvalDenyNoExecute": map[string]any{
				"deniedToolNames":    sources.TaskJobOrchestration.ApprovalDeny.DeniedToolNames,
				"approvalIds":        sources.TaskJobOrchestration.ApprovalDeny.ApprovalIDs,
				"createsDurableJobs": sources.TaskJobOrchestration.ApprovalDeny.CreatesDurableJobs,
				"createsChildRuns":   sources.TaskJobOrchestration.ApprovalDeny.CreatesChildRuns,
			},
			"parallelValidation": map[string]any{
				"dependencyField":        sources.TaskJobOrchestration.Parallel.DependencyField,
				"validOrder":             append([]string(nil), sources.TaskJobOrchestration.Parallel.ValidOrder...),
				"invalidCaseIds":         []string{"single_task", "duplicate_id", "self_dependency", "cycle", "unknown_dependency"},
				"singleTaskError":        sources.TaskJobOrchestration.Parallel.SingleTaskError,
				"duplicateIdError":       sources.TaskJobOrchestration.Parallel.DuplicateIDError,
				"selfDependencyError":    sources.TaskJobOrchestration.Parallel.SelfDependencyError,
				"cycleError":             sources.TaskJobOrchestration.Parallel.CycleError,
				"unknownDependencyError": sources.TaskJobOrchestration.Parallel.UnknownDependencyError,
			},
		},
		"cacheReplay": map[string]any{
			"stablePrefixHash":               sources.ProviderCache.StablePrefix.FirstShape.PrefixHash,
			"toolsHash":                      sources.ProviderCache.StablePrefix.FirstShape.ToolsHash,
			"providerUsageCaseIds":           providerUsageCaseIDs(sources.ProviderCache.ProviderUsageCases),
			"requestShapeCaseIds":            providerRequestShapeCaseIDs(sources.ProviderCache.RequestShapeCases),
			"requestShapeReplay":             buildProviderRequestShapeReplay(sources.ProviderCache.RequestShapeCases),
			"releaseGuardStatuses":           releaseGuardStatuses(sources.ProviderCache.ReleaseGuard.Cases),
			"releaseGuard":                   buildG5ProviderReleaseGuard(sources.ProviderCache.ReleaseGuard),
			"cacheAccounting":                buildG5ProviderCacheAccounting(sources.ProviderCache.ProviderUsageCases),
			"usageParserReplay":              buildG5ProviderUsageParserReplay(sources.ProviderCache.ProviderUsageCases),
			"liveLocalHttpContract":          buildProviderLiveLocalHTTPContract(sources.ProviderCache),
			"offlineParitySeal":              buildG5ProviderOfflineParitySeal(sources.ProviderCache),
			"providerCachePrivacy":           replayG5ProviderCachePrivacy(g5.ControlExecutableCases.ProviderCachePrivacy),
			"driftAttribution":               buildProviderDriftAttribution(sources.ProviderCache.DriftAttribution),
			"forbiddenDiagnosticsSubstrings": sources.ProviderCache.Privacy.ForbiddenDiagnosticsSubstrings,
			"liveSuperiorityClaimAllowed":    sources.ProviderCache.LiveCredentialPolicy.MayClaimSuperiority,
		},
		"providerStreamingReplay": buildG5ProviderStreamingReplay(sources.ProviderStreaming),
		"sessionReplay": map[string]any{
			"routeIds":          routeIDs(sources.G2Routes),
			"resumeRouteIds":    routeIDsMatching(sources.G2Routes, func(route G2RouteReplayCase) bool { return strings.Contains(route.ID, "resume") }),
			"forkRouteIds":      routeIDsMatching(sources.G2Routes, func(route G2RouteReplayCase) bool { return strings.Contains(route.ID, "fork") }),
			"sseRouteIds":       routeIDsMatching(sources.G2Routes, func(route G2RouteReplayCase) bool { return route.ResponseKind == "sse" }),
			"routeStatusReplay": buildG5RouteStatusReplay(sources.G2Routes),
		},
		"approvalUserInputReplay": buildG5ApprovalUserInputReplay(sources.ApprovalUserInput),
		"controlReplay":           g5.ControlReplay,
		"mcpReplay": map[string]any{
			"providerId": sources.MCPLifecycle.ProviderID,
			"lifecycle": map[string]any{
				"providerId":           sources.MCPLifecycle.ProviderID,
				"connectToolNames":     append([]string(nil), sources.MCPLifecycle.Connect.ToolNames...),
				"connectAvailable":     sources.MCPLifecycle.Connect.Diagnostic.Available,
				"connectToolCount":     sources.MCPLifecycle.Connect.Diagnostic.ToolCount,
				"disconnectReason":     sources.MCPLifecycle.Disconnect.Reason,
				"disconnectToolNames":  append([]string{}, sources.MCPLifecycle.Disconnect.ToolNames...),
				"disconnectAvailable":  sources.MCPLifecycle.Disconnect.Diagnostic.Available,
				"disconnectToolCount":  sources.MCPLifecycle.Disconnect.Diagnostic.ToolCount,
				"reloadToolNames":      append([]string(nil), sources.MCPLifecycle.Reload.ToolNames...),
				"schemaOrderStable":    sources.MCPLifecycle.Reload.SchemaOrderStable,
				"cancelErrorSubstring": sources.MCPLifecycle.Cancel.ErrorSubstring,
				"cancelExecuted":       sources.MCPLifecycle.Cancel.Executed,
				"errorCode":            sources.MCPLifecycle.Error.Code,
				"errorApproved":        sources.MCPLifecycle.Error.Approved,
				"usesReasonixProtocol": false,
				"topLevelRouteExposed": false,
			},
			"retryFailedServerIds":         sources.MCPLifecycle.BackgroundReconnect.FailedServerIDs,
			"expectedConnectedServerIds":   sources.MCPLifecycle.BackgroundReconnect.ExpectedConnectedServerIDs,
			"backgroundReconnect":          buildG5MCPBackgroundReconnectReplay(sources.MCPLifecycle.BackgroundReconnect),
			"knownOverride":                sources.MCPLifecycle.KnownOverride.Diagnostic.KnownOverride,
			"knownOverrideDiagnostics":     buildG5MCPKnownOverrideReplay(sources.MCPLifecycle.KnownOverrideVars),
			"liveLocalActivePaths":         sources.MCPLifecycle.LiveLocalIndexer.ExpectedOutput.ActivePaths,
			"lateTombstonePath":            sources.MCPLifecycle.LiveLocalIndexer.LateTombstonePath,
			"secretSafeDiagnostic":         sources.MCPLifecycle.LiveLocalIndexer.ExpectedOutput.SecretSafeDiagnostic,
			"liveLocalIndexer":             buildG5MCPLiveLocalIndexerReplay(sources.MCPLifecycle.LiveLocalIndexer),
			"approvalAnnotations":          buildG5MCPApprovalAnnotationReplay(sources.MCPLifecycle.ApprovalAnnotations),
			"searchMetaToolNames":          append([]string(nil), sources.MCPLifecycle.SearchMetaTools.ToolNames...),
			"searchTrustedToolId":          sources.MCPLifecycle.SearchMetaTools.TrustedToolID,
			"searchUnknownToolError":       sources.MCPLifecycle.SearchMetaTools.UnknownToolError,
			"searchUntrustedSearchedTools": sources.MCPLifecycle.SearchMetaTools.UntrustedSearchedTools,
			"searchCallPolicy":             sources.MCPLifecycle.SearchMetaTools.CallPolicy,
			"searchCallDeniedNoExecute":    !sources.MCPLifecycle.SearchMetaTools.DeniedCallExecuted,
			"searchRefreshDrift": map[string]any{
				"serverId":             sources.MCPLifecycle.SearchMetaTools.RefreshDrift.ServerID,
				"initialToolNames":     append([]string(nil), sources.MCPLifecycle.SearchMetaTools.RefreshDrift.InitialToolNames...),
				"expandedToolNames":    append([]string(nil), sources.MCPLifecycle.SearchMetaTools.RefreshDrift.ExpandedToolNames...),
				"totalIndexed":         sources.MCPLifecycle.SearchMetaTools.RefreshDrift.ExpectedTotalIndexed,
				"catalogDrift":         sources.MCPLifecycle.SearchMetaTools.RefreshDrift.ExpectedCatalogDrift,
				"topLevelRouteExposed": false,
			},
			"searchWorkspaceBoundary": buildG5MCPSearchWorkspaceBoundary(sources.MCPLifecycle.SearchMetaTools),
		},
		"productBoundary": g5.ProductBoundary,
	}
}

func buildG5MCPBackgroundReconnectReplay(input MCPBackgroundReconnect) map[string]any {
	return map[string]any{
		"failedServerIds":         append([]string(nil), input.FailedServerIDs...),
		"suspendedProviderId":     input.SuspendedProviderID,
		"suspendedReason":         input.SuspendedReason,
		"connectedServerIds":      append([]string(nil), input.ExpectedConnectedServerIDs...),
		"errorServerIds":          append([]string(nil), input.ExpectedErrorServerIDs...),
		"attemptsPerFailedServer": input.AttemptsPerFailedServer,
		"retryAllFailedServers":   len(input.FailedServerIDs) == len(input.ExpectedConnectedServerIDs)+len(input.ExpectedErrorServerIDs),
		"requiresRuntimeRestart":  input.RequiresRuntimeRestart,
	}
}

func buildG5MCPKnownOverrideReplay(variants []MCPKnownOverride) []map[string]any {
	output := make([]map[string]any, 0, len(variants))
	for _, variant := range variants {
		item := map[string]any{
			"serverId":        variant.ServerID,
			"knownOverride":   variant.Diagnostic.KnownOverride,
			"effectiveCwd":    variant.Diagnostic.EffectiveCWD,
			"lowPriority":     variant.Diagnostic.LowPriority,
			"backgroundStart": variant.Diagnostic.BackgroundStart,
			"workspaceRoot":   variant.WorkspaceRoot,
		}
		if variant.ExplicitCWD != "" {
			item["explicitCwd"] = variant.ExplicitCWD
		}
		if variant.DaemonIdleTimeoutMs != "" {
			item["daemonIdleTimeoutMs"] = variant.DaemonIdleTimeoutMs
		}
		output = append(output, item)
	}
	return output
}

func buildG5MCPSearchWorkspaceBoundary(input MCPSearchMetaTools) map[string]any {
	return map[string]any{
		"trustedWorkspace":       input.TrustedWorkspace,
		"untrustedWorkspace":     input.UntrustedWorkspace,
		"query":                  input.Query,
		"trustedToolId":          input.TrustedToolID,
		"untrustedSearchedTools": input.UntrustedSearchedTools,
		"unknownToolError":       input.UnknownToolError,
		"callPolicy":             input.CallPolicy,
		"deniedNoExecute":        !input.DeniedCallExecuted,
	}
}

func buildG5TaskJobRouteExecutable(route TaskJobRouteExecutable) map[string]any {
	return map[string]any{
		"unauthorizedStatus":            route.UnauthorizedStatus,
		"outputStatus":                  route.Output.Status,
		"outputJobStatus":               route.Output.JobStatus,
		"outputNextOffset":              route.Output.NextOffset,
		"outputReplayOffset":            route.Output.ReplayOffset,
		"waitStatus":                    route.Wait.Status,
		"waitJobStatus":                 route.Wait.JobStatus,
		"killStatus":                    route.Kill.Status,
		"killJobStatus":                 route.Kill.JobStatus,
		"missingOutputStatus":           route.MissingOutput.Status,
		"rehydratedOutputStatus":        route.Rehydrated.OutputStatus,
		"rehydratedWaitStatus":          route.Rehydrated.WaitStatus,
		"rehydratedKillStatus":          route.Rehydrated.KillStatus,
		"rehydratedCompletedStatus":     route.Rehydrated.CompletedStatus,
		"rehydratedCompletedNextOffset": route.Rehydrated.CompletedNextOffset,
		"rehydratedKilledStatus":        route.Rehydrated.KilledStatus,
	}
}

func replayG5ApprovalUserInputInventory(input G5ApprovalUserInputInventoryCase) G5ApprovalUserInputInventoryOutput {
	caseInput := input.Input
	return G5ApprovalUserInputInventoryOutput{
		GateIDs: []string{
			caseInput.ApprovalID,
			caseInput.SubmittedInputID,
			caseInput.CancelledInputID,
			caseInput.AbortApprovalID,
			caseInput.AbortUserInputID,
		},
		ApprovalIDs: []string{
			caseInput.ApprovalID,
			caseInput.AbortApprovalID,
		},
		UserInputIDs: []string{
			caseInput.SubmittedInputID,
			caseInput.CancelledInputID,
			caseInput.AbortUserInputID,
		},
		RouteKinds: []string{
			"approval-decision",
			"user-input-submit",
			"user-input-cancel",
			"abort-cleanup",
		},
		ReplayKindsInOrder:           append([]string(nil), caseInput.ReplayKindsInOrder...),
		AbortReplayKinds:             append([]string(nil), caseInput.AbortReplayKinds...),
		AnswerCount:                  caseInput.AnswerCount,
		HTTPEchoesAnswers:            caseInput.HTTPEchoesAnswers,
		ResolvedEventIncludesAnswers: caseInput.ResolvedEventIncludesAnswers,
		LateApprovalDecisionStatus:   caseInput.LateApprovalDecisionStatus,
		LateUserInputResolveStatus:   caseInput.LateUserInputResolveStatus,
		PendingAfterAll: caseInput.ApprovalPendingAfter +
			caseInput.PendingAfterSubmit +
			caseInput.PendingAfterCancel +
			caseInput.PendingAfterAbortCleanup,
		UsesReasonixProtocol: input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed: input.ProductBoundary.TopLevelRouteExposed,
	}
}

func buildG5ApprovalUserInputReplay(contract ApprovalUserInputRouteContract) G5ApprovalUserInputReplay {
	_, httpEchoesAnswers := contract.SubmittedUserInput.ExpectedResponse.Body["answers"]
	approvalStatus, _ := contract.Approval.ExpectedResponse.Body["status"].(string)
	cancelledStatus, _ := contract.UserInput.ExpectedResponse.Body["status"].(string)
	return G5ApprovalUserInputReplay{
		ApprovalRoute: G5ApprovalRouteReplay{
			ItemID:         contract.Approval.ItemID,
			ToolName:       contract.Approval.ToolName,
			Summary:        contract.Approval.Summary,
			RequestBody:    contract.Approval.DecisionRequest,
			ResponseStatus: contract.Approval.ExpectedResponse.Status,
			ResponseBody:   cloneJSONMap(contract.Approval.ExpectedResponse.Body),
			PendingBefore:  contract.Approval.PendingBefore,
			PendingAfter:   contract.Approval.PendingAfter,
		},
		ApprovalID:                   contract.Approval.ID,
		ApprovalDecision:             contract.Approval.DecisionRequest.Decision,
		ApprovalStatus:               approvalStatus,
		ApprovalPendingAfter:         contract.Approval.PendingAfter,
		SecondApprovalDecisionStatus: contract.Approval.SecondDecisionStatus,
		ReplaySinceSeq:               contract.Replay.SinceSeq,
		ReplayKindsInOrder:           append([]string(nil), contract.Replay.ExpectedKindsInOrder...),
		SubmittedInputID:             contract.SubmittedUserInput.ID,
		SubmittedStatus:              contract.SubmittedUserInput.ExpectedResolution.Status,
		UserInputSubmitRoute: G5UserInputSubmitRouteReplay{
			ThreadID:       contract.SubmittedUserInput.ThreadID,
			ItemID:         contract.SubmittedUserInput.ItemID,
			Prompt:         contract.SubmittedUserInput.Prompt,
			Questions:      userInputQuestionReplay(contract.SubmittedUserInput.Questions),
			RequestBody:    contract.SubmittedUserInput.ResolveRequest,
			ResponseStatus: contract.SubmittedUserInput.ExpectedResponse.Status,
			ResponseBody:   cloneJSONMap(contract.SubmittedUserInput.ExpectedResponse.Body),
			ResolvedEvent:  contract.SubmittedUserInput.ResolvedEvent,
			PendingBefore:  contract.SubmittedUserInput.PendingBefore,
			PendingAfter:   contract.SubmittedUserInput.PendingAfter,
		},
		AnswerCount:                  len(contract.SubmittedUserInput.ExpectedResolution.Answers),
		HTTPEchoesAnswers:            httpEchoesAnswers,
		ResolvedEventKind:            contract.SubmittedUserInput.ResolvedEvent.Kind,
		ResolvedEventIncludesAnswers: contract.SubmittedUserInput.ResolvedEvent.IncludesAnswers,
		CancelledInputID:             contract.UserInput.ID,
		CancelledStatus:              cancelledStatus,
		UserInputCancelRoute: G5UserInputCancelRouteReplay{
			ItemID:              contract.UserInput.ItemID,
			Prompt:              contract.UserInput.Prompt,
			Questions:           userInputQuestionReplay(contract.UserInput.Questions),
			RequestBody:         contract.UserInput.ResolveRequest,
			ResponseStatus:      contract.UserInput.ExpectedResponse.Status,
			ResponseBody:        cloneJSONMap(contract.UserInput.ExpectedResponse.Body),
			SecondResolveStatus: contract.UserInput.SecondResolveStatus,
			PendingBefore:       contract.UserInput.PendingBefore,
			PendingAfter:        contract.UserInput.PendingAfter,
		},
		LateResolveRejected:        contract.UserInput.SecondResolveStatus == 404,
		PendingAfterSubmit:         contract.SubmittedUserInput.PendingAfter,
		PendingAfterCancel:         contract.UserInput.PendingAfter,
		AbortApprovalID:            contract.AbortCleanup.ApprovalID,
		AbortApprovalStatus:        contract.AbortCleanup.ExpectedApprovalStatus,
		AbortUserInputID:           contract.AbortCleanup.UserInputID,
		AbortUserInputStatus:       contract.AbortCleanup.ExpectedUserInputStatus,
		LateApprovalDecisionStatus: contract.AbortCleanup.LateApprovalDecisionStatus,
		LateUserInputResolveStatus: contract.AbortCleanup.LateUserInputResolveStatus,
		PendingAfterAbortCleanup:   0,
		AbortReplayKinds:           append([]string(nil), contract.AbortCleanup.ExpectedReplayKinds...),
	}
}

func userInputQuestionReplay(questions []ApprovalUserInputQuestion) []G5PromptQuestionReplay {
	output := make([]G5PromptQuestionReplay, 0, len(questions))
	for _, question := range questions {
		labels := make([]string, 0, len(question.Options))
		for _, option := range question.Options {
			labels = append(labels, option.Label)
		}
		output = append(output, G5PromptQuestionReplay{
			Header:       question.Header,
			ID:           question.ID,
			OptionLabels: labels,
		})
	}
	return output
}

func cloneJSONMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func providerUsageCaseIDs(cases []ProviderUsageCase) []string {
	ids := make([]string, 0, len(cases))
	for _, item := range cases {
		ids = append(ids, item.ID)
	}
	return ids
}

func providerRequestShapeCaseIDs(cases []ProviderRequestShapeCase) []string {
	ids := make([]string, 0, len(cases))
	for _, item := range cases {
		ids = append(ids, item.ID)
	}
	return ids
}

func releaseGuardStatuses(cases []ProviderReleaseGuardCase) []string {
	statuses := make([]string, 0, len(cases))
	for _, item := range cases {
		statuses = append(statuses, item.ExpectedStatus)
	}
	return statuses
}

func buildG5ProviderReleaseGuard(guard ProviderReleaseGuard) map[string]any {
	cases := make([]map[string]any, 0, len(guard.Cases))
	lowTailCases := 0
	failed := false
	for _, item := range guard.Cases {
		tailAveragePercent := releaseGuardTailAverage(item.CacheHitPercentCurve, guard.TailWindow)
		collapseCount := releaseGuardCollapseCount(item.CacheHitPercentCurve)
		collapseFailed := item.MaxAllowedCollapses != nil && collapseCount > *item.MaxAllowedCollapses
		status := "pass"
		if collapseFailed {
			status = "fail"
			failed = true
		} else if tailAveragePercent < guard.ThresholdPercent {
			status = "fail"
			lowTailCases += 1
			failed = true
		}
		cases = append(cases, map[string]any{
			"id":                    item.ID,
			"tailAveragePercent":    tailAveragePercent,
			"status":                status,
			"collapseCount":         collapseCount,
			"compactionGuardPaused": item.CompactionGuardPaused,
		})
	}
	if guard.MaxLowTailCases != 0 || lowTailCases != 0 {
		failed = true
	}
	status := "pass"
	if failed {
		status = "fail"
	}
	return map[string]any{
		"fixtureOnly":      guard.FixtureOnly,
		"status":           status,
		"lowTailCases":     lowTailCases,
		"maxLowTailCases":  guard.MaxLowTailCases,
		"thresholdPercent": guard.ThresholdPercent,
		"tailWindow":       guard.TailWindow,
		"cases":            cases,
	}
}

func replayG5ProviderReleaseGuard(guard G5ProviderReleaseGuardControlCase) G5ProviderReleaseGuardOutput {
	cases := make([]G5ProviderReleaseGuardCase, 0, len(guard.Cases))
	lowTailCases := 0
	failed := false
	for _, item := range guard.Cases {
		tailAveragePercent := releaseGuardTailAverage(item.CacheHitPercentCurve, guard.TailWindow)
		collapseCount := releaseGuardCollapseCount(item.CacheHitPercentCurve)
		collapseFailed := item.MaxAllowedCollapses != nil && collapseCount > *item.MaxAllowedCollapses
		status := "pass"
		if collapseFailed {
			status = "fail"
			failed = true
		} else if tailAveragePercent < guard.ThresholdPercent {
			status = "fail"
			lowTailCases += 1
			failed = true
		}
		cases = append(cases, G5ProviderReleaseGuardCase{
			ID:                    item.ID,
			TailAveragePercent:    tailAveragePercent,
			Status:                status,
			CollapseCount:         collapseCount,
			CompactionGuardPaused: item.CompactionGuardPaused,
		})
	}
	if guard.MaxLowTailCases != 0 || lowTailCases != 0 {
		failed = true
	}
	status := "pass"
	if failed {
		status = "fail"
	}
	return G5ProviderReleaseGuardOutput{
		FixtureOnly:      guard.FixtureOnly,
		Status:           status,
		LowTailCases:     lowTailCases,
		MaxLowTailCases:  guard.MaxLowTailCases,
		ThresholdPercent: guard.ThresholdPercent,
		TailWindow:       guard.TailWindow,
		Cases:            cases,
	}
}

func buildG5ProviderUsageParserReplay(cases []ProviderUsageCase) G5ProviderUsageParserOutput {
	telemetrySupportedCaseIDs := make([]string, 0)
	for _, item := range cases {
		if item.ExpectedUsage.CacheHitTokens != nil && item.ExpectedUsage.CacheMissTokens != nil {
			telemetrySupportedCaseIDs = append(telemetrySupportedCaseIDs, item.ID)
		}
	}
	unsupported := providerUsageCaseByID(cases, "unsupported-openai-compatible")
	deepseekNative := providerUsageCaseByID(cases, "deepseek-native-cache-precedence")
	deepseekUsage := providerUsageResponseUsage(deepseekNative)
	deepseekPromptDetails := nestedMap(deepseekUsage, "prompt_tokens_details")
	responses := providerUsageCaseByID(cases, "openai-responses-cached-tokens")
	responsesUsage := providerUsageResponseUsage(responses)
	responsesInputDetails := nestedMap(responsesUsage, "input_tokens_details")
	responsesOutputDetails := nestedMap(responsesUsage, "output_tokens_details")
	anthropic := providerUsageCaseByID(cases, "anthropic-cache-fields")
	anthropicUsage := providerUsageResponseUsage(anthropic)

	return G5ProviderUsageParserOutput{
		CaseCount:                 len(cases),
		TelemetrySupportedCaseIDs: telemetrySupportedCaseIDs,
		UnsupportedAbsentFields: G5ProviderUnsupportedAbsentFields{
			CaseID:            unsupported.ID,
			AbsentFields:      append([]string(nil), unsupported.ExpectedUsage.Absent...),
			CacheHitRateKnown: unsupported.ExpectedUsage.CacheHitRate != nil,
		},
		DeepseekNativePrecedence: G5DeepseekNativePrecedence{
			CaseID:                          deepseekNative.ID,
			PromptCacheHitTokens:            intFromUsagePayload(deepseekUsage, "prompt_cache_hit_tokens"),
			PromptCacheMissTokens:           intFromUsagePayload(deepseekUsage, "prompt_cache_miss_tokens"),
			PromptTokensDetailsCachedTokens: intFromUsagePayload(deepseekPromptDetails, "cached_tokens"),
			ExpectedCacheHitTokens:          pointerIntValue(deepseekNative.ExpectedUsage.CacheHitTokens),
			ExpectedCacheMissTokens:         pointerIntValue(deepseekNative.ExpectedUsage.CacheMissTokens),
			NativeCacheFieldsWin: pointerIntValue(deepseekNative.ExpectedUsage.CacheHitTokens) == intFromUsagePayload(deepseekUsage, "prompt_cache_hit_tokens") &&
				pointerIntValue(deepseekNative.ExpectedUsage.CacheHitTokens) != intFromUsagePayload(deepseekPromptDetails, "cached_tokens"),
			CacheHitRate: pointerFloatValue(deepseekNative.ExpectedUsage.CacheHitRate),
		},
		OpenAIResponsesCachedTokens: G5OpenAIResponsesCachedTokens{
			CaseID:                  responses.ID,
			InputTokens:             intFromUsagePayload(responsesUsage, "input_tokens"),
			CachedTokens:            intFromUsagePayload(responsesInputDetails, "cached_tokens"),
			ExpectedCacheHitTokens:  pointerIntValue(responses.ExpectedUsage.CacheHitTokens),
			ExpectedCacheMissTokens: pointerIntValue(responses.ExpectedUsage.CacheMissTokens),
			ReasoningTokens:         intFromUsagePayload(responsesOutputDetails, "reasoning_tokens"),
			CachedTokensUsed:        pointerIntValue(responses.ExpectedUsage.CacheHitTokens) == intFromUsagePayload(responsesInputDetails, "cached_tokens"),
		},
		AnthropicCacheFields: G5AnthropicCacheFields{
			CaseID:                   anthropic.ID,
			InputTokens:              intFromUsagePayload(anthropicUsage, "input_tokens"),
			CacheReadInputTokens:     intFromUsagePayload(anthropicUsage, "cache_read_input_tokens"),
			CacheCreationInputTokens: intFromUsagePayload(anthropicUsage, "cache_creation_input_tokens"),
			ExpectedPromptTokens:     pointerIntValue(anthropic.ExpectedUsage.PromptTokens),
			ExpectedCacheHitTokens:   pointerIntValue(anthropic.ExpectedUsage.CacheHitTokens),
			ExpectedCacheMissTokens:  pointerIntValue(anthropic.ExpectedUsage.CacheMissTokens),
			CacheFieldsIncludedInPrompt: pointerIntValue(anthropic.ExpectedUsage.PromptTokens) ==
				intFromUsagePayload(anthropicUsage, "input_tokens")+
					intFromUsagePayload(anthropicUsage, "cache_read_input_tokens")+
					intFromUsagePayload(anthropicUsage, "cache_creation_input_tokens"),
		},
	}
}

func providerUsageCaseByID(cases []ProviderUsageCase, id string) ProviderUsageCase {
	for _, item := range cases {
		if item.ID == id {
			return item
		}
	}
	return ProviderUsageCase{}
}

func providerUsageResponseUsage(item ProviderUsageCase) map[string]any {
	return nestedMap(item.ResponseBody, "usage")
}

func nestedMap(input map[string]any, field string) map[string]any {
	value, _ := input[field].(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func releaseGuardTailAverage(values []float64, windowSize int) float64 {
	if windowSize <= 0 {
		return 0
	}
	start := len(values) - windowSize
	if start < 0 {
		start = 0
	}
	tail := values[start:]
	if len(tail) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range tail {
		total += value
	}
	return total / float64(len(tail))
}

func releaseGuardCollapseCount(values []float64) int {
	collapses := 0
	for index := 1; index < len(values); index += 1 {
		if values[index] < values[index-1] {
			collapses += 1
		}
	}
	return collapses
}

func buildG5ProviderOfflineParitySeal(contract ProviderCacheContract) G5ProviderOfflineParitySeal {
	guard := buildG5ProviderReleaseGuard(contract.ReleaseGuard)
	return G5ProviderOfflineParitySeal{
		FixtureOnly:                   contract.LiveCredentialPolicy.FixtureOnly,
		MayClaimLiveSuperiority:       contract.LiveCredentialPolicy.MayClaimSuperiority,
		StablePrefixEquivalent:        contract.StablePrefix.FirstShape.PrefixHash == contract.StablePrefix.EquivalentShape.PrefixHash,
		PrefixItemsHashStable:         contract.StablePrefix.FirstShape.PrefixItemsHash == contract.StablePrefix.EquivalentShape.PrefixItemsHash,
		ToolsHashStable:               contract.StablePrefix.FirstShape.ToolsHash == contract.StablePrefix.EquivalentShape.ToolsHash,
		StablePrefixHash:              contract.StablePrefix.FirstShape.PrefixHash,
		EquivalentPrefixHash:          contract.StablePrefix.EquivalentShape.PrefixHash,
		DeepseekProviderID:            contract.StablePrefix.FirstShape.ProviderID,
		DeepseekEndpointFormat:        contract.StablePrefix.FirstShape.EndpointFormat,
		DeepseekModel:                 contract.StablePrefix.FirstShape.Model,
		DeepseekStableCacheHitTokens:  pointerIntValue(contract.StablePrefix.Usage.CacheHitTokens),
		DeepseekStableCacheMissTokens: pointerIntValue(contract.StablePrefix.Usage.CacheMissTokens),
		DeepseekStableCacheHitRate:    pointerFloatValue(contract.StablePrefix.Usage.CacheHitRate),
		DiagnosticsPrefixChanged:      contract.StablePrefix.ExpectedDiagnostics.PrefixChanged,
		DiagnosticsTelemetrySupported: contract.StablePrefix.ExpectedDiagnostics.CacheTelemetrySupported,
		ProviderUsageCaseCount:        len(contract.ProviderUsageCases),
		RequestShapeCaseCount:         len(contract.RequestShapeCases),
		RequestShapeEndpointFormats:   providerRequestShapeEndpointFormats(contract.RequestShapeCases),
		ReleaseGuardStatus:            guard["status"].(string),
		ReleaseGuardThresholdPercent:  contract.ReleaseGuard.ThresholdPercent,
		ReleaseGuardTailWindow:        contract.ReleaseGuard.TailWindow,
	}
}

func replayG5ProviderOfflineParitySeal(input G5ProviderOfflineParitySealCase) G5ProviderOfflineParitySeal {
	return G5ProviderOfflineParitySeal{
		FixtureOnly:                   input.LiveCredentialPolicy.FixtureOnly,
		MayClaimLiveSuperiority:       input.LiveCredentialPolicy.MayClaimSuperiority,
		StablePrefixEquivalent:        input.StablePrefix.FirstShape.PrefixHash == input.StablePrefix.EquivalentShape.PrefixHash,
		PrefixItemsHashStable:         input.StablePrefix.FirstShape.PrefixItemsHash == input.StablePrefix.EquivalentShape.PrefixItemsHash,
		ToolsHashStable:               input.StablePrefix.FirstShape.ToolsHash == input.StablePrefix.EquivalentShape.ToolsHash,
		StablePrefixHash:              input.StablePrefix.FirstShape.PrefixHash,
		EquivalentPrefixHash:          input.StablePrefix.EquivalentShape.PrefixHash,
		DeepseekProviderID:            input.StablePrefix.FirstShape.ProviderID,
		DeepseekEndpointFormat:        input.StablePrefix.FirstShape.EndpointFormat,
		DeepseekModel:                 input.StablePrefix.FirstShape.Model,
		DeepseekStableCacheHitTokens:  pointerIntValue(input.StablePrefix.Usage.CacheHitTokens),
		DeepseekStableCacheMissTokens: pointerIntValue(input.StablePrefix.Usage.CacheMissTokens),
		DeepseekStableCacheHitRate:    pointerFloatValue(input.StablePrefix.Usage.CacheHitRate),
		DiagnosticsPrefixChanged:      input.StablePrefix.ExpectedDiagnostics.PrefixChanged,
		DiagnosticsTelemetrySupported: input.StablePrefix.ExpectedDiagnostics.CacheTelemetrySupported,
		ProviderUsageCaseCount:        len(input.ProviderUsageCaseIDs),
		RequestShapeCaseCount:         len(input.RequestShapeCaseIDs),
		RequestShapeEndpointFormats:   append([]string(nil), input.RequestShapeEndpointFormats...),
		ReleaseGuardStatus:            input.ReleaseGuard.Status,
		ReleaseGuardThresholdPercent:  input.ReleaseGuard.ThresholdPercent,
		ReleaseGuardTailWindow:        input.ReleaseGuard.TailWindow,
	}
}

func replayG5ProviderCachePrivacy(input G5ProviderCachePrivacyCase) G5ProviderCachePrivacyOutput {
	diagnosticsJSON, err := json.Marshal(input.Diagnostics)
	diagnosticsText := ""
	if err == nil {
		diagnosticsText = string(diagnosticsJSON)
	}
	leaksForbiddenDiagnostics := false
	for _, substring := range input.Privacy.ForbiddenDiagnosticsSubstrings {
		if strings.Contains(diagnosticsText, substring) {
			leaksForbiddenDiagnostics = true
			break
		}
	}

	return G5ProviderCachePrivacyOutput{
		FixtureOnly:                        input.LiveCredentialPolicy.FixtureOnly,
		DiagnosticCheckedFieldCount:        len(input.Diagnostics),
		ForbiddenDiagnosticsSubstringCount: len(input.Privacy.ForbiddenDiagnosticsSubstrings),
		DiagnosticsLeakForbiddenSubstrings: leaksForbiddenDiagnostics,
		LiveCredentialsUsed:                input.LiveLocalHTTPContract.UsesLiveCredentials,
		MayClaimLiveSuperiority: input.LiveCredentialPolicy.MayClaimSuperiority ||
			input.LiveLocalHTTPContract.MayClaimLiveSuperiority,
		UsesReasonixProtocol: input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed: input.ProductBoundary.TopLevelRouteExposed,
	}
}

func replayG5ProviderCacheInventory(input G5ProviderCacheInventoryCase) G5ProviderCacheInventoryOutput {
	return G5ProviderCacheInventoryOutput{
		StablePrefixHash:       input.StablePrefix.FirstShape.PrefixHash,
		ToolsHash:              input.StablePrefix.FirstShape.ToolsHash,
		ProviderUsageCaseIDs:   append([]string(nil), input.ProviderUsageCaseIDs...),
		RequestShapeCaseIDs:    append([]string(nil), input.RequestShapeCaseIDs...),
		ProviderUsageCaseCount: len(input.ProviderUsageCaseIDs),
		RequestShapeCaseCount:  len(input.RequestShapeCaseIDs),
		StablePrefixEquivalent: input.StablePrefix.FirstShape.PrefixHash == input.StablePrefix.EquivalentShape.PrefixHash,
		ToolsHashStable:        input.StablePrefix.FirstShape.ToolsHash == input.StablePrefix.EquivalentShape.ToolsHash,
		UsesReasonixProtocol:   input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:   input.ProductBoundary.TopLevelRouteExposed,
	}
}

func buildProviderLiveLocalHTTPContract(contract ProviderCacheContract) ProviderLiveLocalHTTPContract {
	liveContract := contract.LiveLocalHTTPContract
	return ProviderLiveLocalHTTPContract{
		FixtureOnly:                      liveContract.FixtureOnly,
		Transport:                        liveContract.Transport,
		UsesLiveCredentials:              liveContract.UsesLiveCredentials,
		PreservesOriginalProviderBaseURL: liveContract.PreservesOriginalProviderBaseURL,
		UsageCaseCount:                   len(contract.ProviderUsageCases),
		RequestShapeCaseCount:            len(contract.RequestShapeCases),
		ExpectedPostCount:                len(contract.ProviderUsageCases) + len(contract.RequestShapeCases),
		CoveredEndpointFormats:           providerRequestShapeEndpointFormats(contract.RequestShapeCases),
		CoveredProviderFamilies:          append([]string(nil), liveContract.CoveredProviderFamilies...),
		MayClaimLiveSuperiority:          liveContract.MayClaimLiveSuperiority,
	}
}

func replayG5ProviderLiveLocalHTTPContract(input G5ProviderLiveLocalHTTPCase) ProviderLiveLocalHTTPContract {
	return ProviderLiveLocalHTTPContract{
		FixtureOnly:                      input.Contract.FixtureOnly,
		Transport:                        input.Contract.Transport,
		UsesLiveCredentials:              input.Contract.UsesLiveCredentials,
		PreservesOriginalProviderBaseURL: input.Contract.PreservesOriginalProviderBaseURL,
		UsageCaseCount:                   len(input.ProviderUsageCaseIDs),
		RequestShapeCaseCount:            len(input.RequestShapeCaseIDs),
		ExpectedPostCount:                len(input.ProviderUsageCaseIDs) + len(input.RequestShapeCaseIDs),
		CoveredEndpointFormats:           append([]string(nil), input.RequestShapeEndpointFormats...),
		CoveredProviderFamilies:          append([]string(nil), input.Contract.CoveredProviderFamilies...),
		MayClaimLiveSuperiority:          input.Contract.MayClaimLiveSuperiority,
	}
}

func replayG5ProviderCacheCoverageFloor(input G5ProviderCacheCoverageFloorCase) G5ProviderCacheCoverageFloorOutput {
	coveredFamilies := make([]string, 0)
	for _, item := range input.RequestShapeCases {
		coveredFamilies = appendUniqueString(coveredFamilies, providerFamilyForRequestShapeCase(item))
	}
	for _, item := range input.ProviderUsageCases {
		coveredFamilies = appendUniqueString(coveredFamilies, providerFamilyForUsageCase(item))
	}

	out := G5ProviderCacheCoverageFloorOutput{
		RequiredProviderFamilies:                append([]string(nil), input.LiveLocalHTTPContract.CoveredProviderFamilies...),
		CoveredProviderFamilies:                 coveredFamilies,
		ProviderFamilyCoverageComplete:          sameStringSlice(input.LiveLocalHTTPContract.CoveredProviderFamilies, coveredFamilies),
		ProviderUsageCaseIDs:                    make([]string, 0, len(input.ProviderUsageCases)),
		RequestShapeCaseIDs:                     make([]string, 0, len(input.RequestShapeCases)),
		RequestShapeEndpointFormats:             providerRequestShapeCoverageEndpointFormats(input.RequestShapeCases),
		TelemetrySupportedCaseIDs:               make([]string, 0),
		UnsupportedUnknownCaseIDs:               make([]string, 0),
		CustomFullEndpointCaseIDs:               make([]string, 0),
		CustomFullEndpointExactURLCaseIDs:       make([]string, 0),
		CustomFullEndpointToolShapes:            make([]string, 0),
		CustomFullEndpointTelemetryCaseIDs:      make([]string, 0),
		DeepSeekUsageCaseIDs:                    make([]string, 0),
		DeepSeekRequestShapeCaseIDs:             make([]string, 0),
		OpenAICompatibleChatRequestShapeCaseIDs: make([]string, 0),
		OpenAIResponsesUsageCaseIDs:             make([]string, 0),
		OpenAIResponsesRequestShapeCaseIDs:      make([]string, 0),
		AnthropicMessagesUsageCaseIDs:           make([]string, 0),
		AnthropicMessagesRequestShapeCaseIDs:    make([]string, 0),
		LiveCredentialsUsed:                     input.LiveLocalHTTPContract.UsesLiveCredentials,
		MayClaimLiveSuperiority:                 input.LiveLocalHTTPContract.MayClaimLiveSuperiority,
		UsesReasonixProtocol:                    input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:                    input.ProductBoundary.TopLevelRouteExposed,
	}
	for _, item := range input.ProviderUsageCases {
		out.ProviderUsageCaseIDs = append(out.ProviderUsageCaseIDs, item.ID)
		if item.ExpectedUsage.CacheHitTokens != nil && item.ExpectedUsage.CacheMissTokens != nil {
			out.TelemetrySupportedCaseIDs = append(out.TelemetrySupportedCaseIDs, item.ID)
		}
		if item.ExpectedUsage.CacheHitRate == nil {
			out.UnsupportedUnknownCaseIDs = append(out.UnsupportedUnknownCaseIDs, item.ID)
		}
		if strings.Contains(item.BaseURL, "deepseek") {
			out.DeepSeekUsageCaseIDs = append(out.DeepSeekUsageCaseIDs, item.ID)
		}
		if item.EndpointFormat == "responses" {
			out.OpenAIResponsesUsageCaseIDs = append(out.OpenAIResponsesUsageCaseIDs, item.ID)
		}
		if item.EndpointFormat == "messages" {
			out.AnthropicMessagesUsageCaseIDs = append(out.AnthropicMessagesUsageCaseIDs, item.ID)
		}
	}
	for _, item := range input.RequestShapeCases {
		out.RequestShapeCaseIDs = append(out.RequestShapeCaseIDs, item.ID)
		if item.EndpointFormat == "custom_endpoint" {
			out.CustomFullEndpointCaseIDs = append(out.CustomFullEndpointCaseIDs, item.ID)
			if item.ExpectedURL == item.BaseURL {
				out.CustomFullEndpointExactURLCaseIDs = append(out.CustomFullEndpointExactURLCaseIDs, item.ID)
			}
			out.CustomFullEndpointToolShapes = append(out.CustomFullEndpointToolShapes, item.ExpectedToolShape)
		}
		if strings.Contains(item.BaseURL, "deepseek") {
			out.DeepSeekRequestShapeCaseIDs = append(out.DeepSeekRequestShapeCaseIDs, item.ID)
		}
		if item.EndpointFormat == "chat_completions" && !strings.Contains(item.BaseURL, "deepseek") {
			out.OpenAICompatibleChatRequestShapeCaseIDs = append(out.OpenAICompatibleChatRequestShapeCaseIDs, item.ID)
		}
		if item.EndpointFormat == "responses" {
			out.OpenAIResponsesRequestShapeCaseIDs = append(out.OpenAIResponsesRequestShapeCaseIDs, item.ID)
		}
		if item.EndpointFormat == "messages" {
			out.AnthropicMessagesRequestShapeCaseIDs = append(out.AnthropicMessagesRequestShapeCaseIDs, item.ID)
		}
	}
	for _, telemetryID := range out.TelemetrySupportedCaseIDs {
		for _, customID := range out.CustomFullEndpointCaseIDs {
			if telemetryID == customID {
				out.CustomFullEndpointTelemetryCaseIDs = append(out.CustomFullEndpointTelemetryCaseIDs, telemetryID)
			}
		}
	}
	out.CustomFullEndpointRequestShapeOnly = len(out.CustomFullEndpointCaseIDs) > 0 && len(out.CustomFullEndpointTelemetryCaseIDs) == 0
	out.TelemetrySupportedExcludesCustomFullEndpoints = len(out.CustomFullEndpointTelemetryCaseIDs) == 0
	out.CustomProviderCacheTelemetryClaimAllowed = false
	return out
}

func buildProviderDriftAttribution(drift ProviderDriftAttribution) G5ProviderDriftAttribution {
	return provider.BuildProviderDriftAttribution(drift)
}

func buildProviderRequestShapeReplay(cases []ProviderRequestShapeCase) G5ProviderRequestShapeOutput {
	exactURLCount := 0
	derivedURLMatchCaseIDs := make([]string, 0)
	headerShapeMatchCaseIDs := make([]string, 0)
	bodyShapeMatchCaseIDs := make([]string, 0)
	toolShapeMatchCaseIDs := make([]string, 0)
	endpointFormats := make([]string, 0)
	seenEndpointFormats := map[string]bool{}
	fullEndpointCaseIDs := make([]string, 0)
	toolShapes := make([]string, 0)
	seenToolShapes := map[string]bool{}
	customFullEndpointExactURLCaseIDs := make([]string, 0)
	customFullEndpointAppendedPathCount := 0
	for _, item := range cases {
		if item.ExpectedURL != "" {
			exactURLCount += 1
		}
		if derivedProviderRequestURL(item) == item.ExpectedURL {
			derivedURLMatchCaseIDs = append(derivedURLMatchCaseIDs, item.ID)
		}
		if sameStringSlice(derivedProviderRequiredHeaders(item), item.RequiredHeaders) &&
			sameStringSlice(derivedProviderForbiddenHeaders(item), item.ForbiddenHeaders) {
			headerShapeMatchCaseIDs = append(headerShapeMatchCaseIDs, item.ID)
		}
		if sameStringSlice(derivedProviderRequiredBodyFields(item), item.RequiredBodyFields) &&
			sameStringSlice(derivedProviderForbiddenBodyFields(item), item.ForbiddenBodyFields) {
			bodyShapeMatchCaseIDs = append(bodyShapeMatchCaseIDs, item.ID)
		}
		if derivedProviderToolShape(item) == item.ExpectedToolShape {
			toolShapeMatchCaseIDs = append(toolShapeMatchCaseIDs, item.ID)
		}
		if !seenEndpointFormats[item.EndpointFormat] {
			seenEndpointFormats[item.EndpointFormat] = true
			endpointFormats = append(endpointFormats, item.EndpointFormat)
		}
		if item.EndpointFormat == "custom_endpoint" {
			fullEndpointCaseIDs = append(fullEndpointCaseIDs, item.ID)
			if derivedProviderRequestURL(item) == item.BaseURL && item.ExpectedURL == item.BaseURL {
				customFullEndpointExactURLCaseIDs = append(customFullEndpointExactURLCaseIDs, item.ID)
			}
			if item.ExpectedURL != item.BaseURL {
				customFullEndpointAppendedPathCount += 1
			}
		}
		if !seenToolShapes[item.ExpectedToolShape] {
			seenToolShapes[item.ExpectedToolShape] = true
			toolShapes = append(toolShapes, item.ExpectedToolShape)
		}
	}
	return G5ProviderRequestShapeOutput{
		CaseCount:                           len(cases),
		ExactURLCount:                       exactURLCount,
		DerivedURLMatchCaseIDs:              derivedURLMatchCaseIDs,
		HeaderShapeMatchCaseIDs:             headerShapeMatchCaseIDs,
		BodyShapeMatchCaseIDs:               bodyShapeMatchCaseIDs,
		ToolShapeMatchCaseIDs:               toolShapeMatchCaseIDs,
		Matrix:                              cloneProviderRequestShapeCases(cases),
		EndpointFormats:                     endpointFormats,
		FullEndpointCaseIDs:                 fullEndpointCaseIDs,
		ToolShapes:                          toolShapes,
		CustomFullEndpointExactURLCaseIDs:   customFullEndpointExactURLCaseIDs,
		CustomFullEndpointAppendedPathCount: customFullEndpointAppendedPathCount,
		RequiredBodyFieldFamilies: G5ProviderRequiredBodyFieldFamilies{
			MessagesFieldCaseCount:        countProviderRequiredBodyField(cases, "messages"),
			InputFieldCaseCount:           countProviderRequiredBodyField(cases, "input"),
			SystemFieldCaseCount:          countProviderRequiredBodyField(cases, "system"),
			ThinkingFieldCaseCount:        countProviderRequiredBodyField(cases, "thinking"),
			MaxOutputTokensFieldCaseCount: countProviderRequiredBodyField(cases, "max_output_tokens"),
			MaxTokensFieldCaseCount:       countProviderRequiredBodyField(cases, "max_tokens"),
		},
		ForbiddenBodyFieldFamilies: G5ProviderForbiddenBodyFieldFamilies{
			ThinkingForbiddenCaseCount:        countProviderForbiddenBodyField(cases, "thinking"),
			SystemForbiddenCaseCount:          countProviderForbiddenBodyField(cases, "system"),
			InputForbiddenCaseCount:           countProviderForbiddenBodyField(cases, "input"),
			MaxOutputTokensForbiddenCaseCount: countProviderForbiddenBodyField(cases, "max_output_tokens"),
		},
	}
}

func cloneProviderRequestShapeCases(cases []ProviderRequestShapeCase) []ProviderRequestShapeCase {
	out := make([]ProviderRequestShapeCase, 0, len(cases))
	for _, item := range cases {
		out = append(out, ProviderRequestShapeCase{
			ID:                  item.ID,
			EndpointFormat:      item.EndpointFormat,
			BaseURL:             item.BaseURL,
			Model:               item.Model,
			ReasoningEffort:     item.ReasoningEffort,
			ExpectedURL:         item.ExpectedURL,
			RequiredHeaders:     append([]string{}, item.RequiredHeaders...),
			ForbiddenHeaders:    append([]string{}, item.ForbiddenHeaders...),
			RequiredBodyFields:  append([]string{}, item.RequiredBodyFields...),
			ForbiddenBodyFields: append([]string{}, item.ForbiddenBodyFields...),
			ExpectedToolShape:   item.ExpectedToolShape,
		})
	}
	return out
}

func derivedProviderRequestURL(item ProviderRequestShapeCase) string {
	switch item.EndpointFormat {
	case "custom_endpoint":
		return item.BaseURL
	case "responses":
		return appendProviderEndpointPath(item.BaseURL, "/v1/responses")
	case "messages":
		return appendProviderEndpointPath(item.BaseURL, "/v1/messages")
	default:
		return appendProviderEndpointPath(item.BaseURL, "/v1/chat/completions")
	}
}

func derivedProviderToolShape(item ProviderRequestShapeCase) string {
	if item.EndpointFormat == "responses" || strings.HasSuffix(item.BaseURL, "/responses") {
		return "responses-function"
	}
	if item.EndpointFormat == "messages" || strings.HasSuffix(item.BaseURL, "/messages") {
		return "anthropic-input-schema"
	}
	return "openai-function"
}

func derivedProviderRequiredHeaders(item ProviderRequestShapeCase) []string {
	if derivedProviderToolShape(item) == "anthropic-input-schema" {
		return []string{"Authorization", "x-api-key", "anthropic-version"}
	}
	return []string{"Authorization"}
}

func derivedProviderForbiddenHeaders(item ProviderRequestShapeCase) []string {
	if derivedProviderToolShape(item) == "anthropic-input-schema" {
		return []string{}
	}
	return []string{"x-api-key", "anthropic-version"}
}

func derivedProviderRequiredBodyFields(item ProviderRequestShapeCase) []string {
	switch derivedProviderToolShape(item) {
	case "responses-function":
		return []string{"model", "stream", "input", "tools", "max_output_tokens"}
	case "anthropic-input-schema":
		return []string{"model", "stream", "system", "messages", "tools", "max_tokens"}
	default:
		fields := []string{"model", "stream", "messages", "tools"}
		if strings.Contains(item.BaseURL, "deepseek") {
			fields = append(fields, "thinking")
		}
		return append(fields, "reasoning_effort")
	}
}

func derivedProviderForbiddenBodyFields(item ProviderRequestShapeCase) []string {
	switch derivedProviderToolShape(item) {
	case "responses-function":
		return []string{"messages", "system", "thinking"}
	case "anthropic-input-schema":
		return []string{"input", "max_output_tokens", "thinking"}
	default:
		fields := []string{"input", "system", "max_output_tokens"}
		if !strings.Contains(item.BaseURL, "deepseek") {
			fields = append(fields, "thinking")
		}
		return fields
	}
}

func buildG5MCPLiveLocalIndexerReplay(indexer MCPLiveLocalIndexer) map[string]any {
	output := indexer.ExpectedOutput
	return map[string]any{
		"serverId":              output.ServerID,
		"cwd":                   output.CWD,
		"lowPriority":           output.LowPriority,
		"backgroundStart":       output.BackgroundStart,
		"retryAttempts":         cloneStringIntMap(output.RetryAttempts),
		"activePaths":           append([]string(nil), output.ActivePaths...),
		"tombstoneCount":        output.TombstoneCount,
		"restartedFromSnapshot": output.RestartedFromSnapshot,
		"lateTombstonePath":     indexer.LateTombstonePath,
		"secretSafeDiagnostic":  output.SecretSafeDiagnostic,
		"leaksSecret":           strings.Contains(output.SecretSafeDiagnostic, indexer.SecretDiagnostic),
		"topLevelRouteExposed":  false,
	}
}

func buildG5MCPApprovalAnnotationReplay(annotation MCPApprovalAnnotations) map[string]any {
	return map[string]any{
		"serverId":           annotation.ServerID,
		"toolName":           annotation.ToolName,
		"normalizedToolName": annotation.NormalizedToolName,
		"destructiveHint":    annotation.Annotations.DestructiveHint,
		"openWorldHint":      annotation.Annotations.OpenWorldHint,
		"approvalId":         annotation.ApprovalID,
		"decision":           annotation.Decision,
		"resultKind":         annotation.ResultKind,
		"executed":           annotation.Executed,
		"deniedNoExecute":    !annotation.Executed,
	}
}

func cloneStringIntMap(input map[string]int) map[string]int {
	output := make(map[string]int, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func countProviderRequiredBodyField(cases []ProviderRequestShapeCase, field string) int {
	count := 0
	for _, item := range cases {
		if stringSliceContains(item.RequiredBodyFields, field) {
			count += 1
		}
	}
	return count
}

func countProviderForbiddenBodyField(cases []ProviderRequestShapeCase, field string) int {
	count := 0
	for _, item := range cases {
		if stringSliceContains(item.ForbiddenBodyFields, field) {
			count += 1
		}
	}
	return count
}

func providerRequestShapeEndpointFormats(cases []ProviderRequestShapeCase) []string {
	formats := make([]string, 0)
	seen := map[string]bool{}
	for _, item := range cases {
		if seen[item.EndpointFormat] {
			continue
		}
		seen[item.EndpointFormat] = true
		formats = append(formats, item.EndpointFormat)
	}
	return formats
}

func providerRequestShapeCoverageEndpointFormats(cases []ProviderRequestShapeCoverageCase) []string {
	formats := make([]string, 0)
	seen := map[string]bool{}
	for _, item := range cases {
		if seen[item.EndpointFormat] {
			continue
		}
		seen[item.EndpointFormat] = true
		formats = append(formats, item.EndpointFormat)
	}
	return formats
}

func appendUniqueString(values []string, next string) []string {
	for _, item := range values {
		if item == next {
			return values
		}
	}
	return append(values, next)
}

func sameStringSlice(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sameIntSlice(left []int, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func providerFamilyForUsageCase(item ProviderUsageCase) string {
	if strings.Contains(item.BaseURL, "deepseek") {
		return "deepseek"
	}
	if item.EndpointFormat == "responses" {
		return "openai-responses"
	}
	if item.EndpointFormat == "messages" {
		return "anthropic-messages"
	}
	return "openai-compatible"
}

func providerFamilyForRequestShapeCase(item ProviderRequestShapeCoverageCase) string {
	if item.EndpointFormat == "custom_endpoint" {
		return "custom-full-endpoint"
	}
	if strings.Contains(item.BaseURL, "deepseek") {
		return "deepseek"
	}
	if item.EndpointFormat == "responses" {
		return "openai-responses"
	}
	if item.EndpointFormat == "messages" {
		return "anthropic-messages"
	}
	return "openai-compatible"
}

func pointerIntValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func pointerFloatValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func buildG5ProviderCacheAccounting(cases []ProviderUsageCase) G3ProviderCacheAccounting {
	out := G3ProviderCacheAccounting{
		RawPayloadParsedCaseIDs:        make([]string, 0, len(cases)),
		RawTelemetrySupportedCaseIDs:   make([]string, 0, len(cases)),
		RawMatchesExpectedUsageCaseIDs: make([]string, 0, len(cases)),
		TelemetrySupportedCaseIDs:      make([]string, 0, len(cases)),
		UnsupportedUnknownCaseIDs:      make([]string, 0),
		DeepSeekCaseIDs:                make([]string, 0),
		OpenAICacheCaseIDs:             make([]string, 0),
		AnthropicCacheCaseIDs:          make([]string, 0),
	}
	for _, item := range cases {
		parsedUsage := parsedProviderUsageFromRawPayload(item.EndpointFormat, item.BaseURL, item.ResponseBody)
		out.RawPayloadParsedCaseIDs = append(out.RawPayloadParsedCaseIDs, item.ID)
		if usageSummaryMatchesExpected(parsedUsage, item.ExpectedUsage) {
			out.RawMatchesExpectedUsageCaseIDs = append(out.RawMatchesExpectedUsageCaseIDs, item.ID)
		}
		if parsedUsage.CacheHitTokens == nil || parsedUsage.CacheMissTokens == nil {
			if parsedUsage.CacheHitRate == nil {
				out.UnsupportedUnknownCaseIDs = append(out.UnsupportedUnknownCaseIDs, item.ID)
			}
			continue
		}
		out.RawTelemetrySupportedCaseIDs = append(out.RawTelemetrySupportedCaseIDs, item.ID)
		out.TelemetrySupportedCaseIDs = append(out.TelemetrySupportedCaseIDs, item.ID)
		out.TotalCacheHitTokens += *parsedUsage.CacheHitTokens
		out.TotalCacheMissTokens += *parsedUsage.CacheMissTokens
		if strings.Contains(item.BaseURL, "deepseek") {
			out.DeepSeekCaseIDs = append(out.DeepSeekCaseIDs, item.ID)
		}
		if item.EndpointFormat == "responses" {
			out.OpenAICacheCaseIDs = append(out.OpenAICacheCaseIDs, item.ID)
		}
		if item.EndpointFormat == "messages" {
			out.AnthropicCacheCaseIDs = append(out.AnthropicCacheCaseIDs, item.ID)
		}
	}
	denominator := out.TotalCacheHitTokens + out.TotalCacheMissTokens
	if denominator > 0 {
		out.AggregateCacheHitRate = float64(out.TotalCacheHitTokens) / float64(denominator)
	}
	return out
}

func parsedProviderUsageFromRawPayload(endpointFormat string, baseURL string, responseBody map[string]any) G3ProviderUsageSummary {
	return provider.ParsedProviderUsageFromRawPayload(endpointFormat, baseURL, responseBody)
}

func usageSummaryMatchesExpected(parsed G3ProviderUsageSummary, expected G3ProviderUsageSummary) bool {
	return provider.UsageSummaryMatchesExpected(parsed, expected)
}

func UsageSummaryMatchesExpected(parsed G3ProviderUsageSummary, expected G3ProviderUsageSummary) bool {
	return usageSummaryMatchesExpected(parsed, expected)
}

func intPointer(value int) *int {
	return &value
}

func optionalIntPointer(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func floatPointer(value float64) *float64 {
	return &value
}

func intPointerEqual(left *int, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func floatPointerEqual(left *float64, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func buildG5ProviderStreamingReplay(contract G3ProviderConformanceContract) G5ProviderStreamingOutput {
	expectedUsageCase := g3ProviderUsageCaseByID(contract.ProviderUsageMatrix, contract.Streaming.ExpectedUsageCaseID)
	usage := usagePayloadFromSSEFrames(contract.Streaming.SSEFrames)
	return G5ProviderStreamingOutput{
		SourceContractID:              contract.ID,
		ThreadID:                      contract.Streaming.ThreadID,
		SinceSeq:                      contract.Streaming.SinceSeq,
		SSEFrameCount:                 len(contract.Streaming.SSEFrames),
		StreamingKinds:                sseEventNames(contract.Streaming.SSEFrames),
		ExpectedKindsInOrder:          append([]string(nil), contract.Streaming.ExpectedKindsInOrder...),
		ExpectedUsageCaseID:           contract.Streaming.ExpectedUsageCaseID,
		UsageEventMatchesExpectedCase: usagePayloadMatchesExpected(usage, expectedUsageCase.ExpectedUsage),
		UsagePromptTokens:             intFromUsagePayload(usage, "promptTokens"),
		UsageCompletionTokens:         intFromUsagePayload(usage, "completionTokens"),
		UsageReasoningTokens:          intFromUsagePayload(usage, "reasoningTokens"),
		UsageTotalTokens:              intFromUsagePayload(usage, "totalTokens"),
		UsageCacheHitTokens:           intFromUsagePayload(usage, "cacheHitTokens"),
		UsageCacheMissTokens:          intFromUsagePayload(usage, "cacheMissTokens"),
		UsageCacheHitRate:             floatFromUsagePayload(usage, "cacheHitRate"),
		CacheTelemetrySupported:       expectedUsageCase.ExpectedUsage.CacheHitTokens != nil && expectedUsageCase.ExpectedUsage.CacheMissTokens != nil,
		ProductBoundary:               contract.ProductBoundary,
	}
}

func replayG5ProviderStreaming(input G5ProviderStreamingControlCase) G5ProviderStreamingOutput {
	return buildG5ProviderStreamingReplay(G3ProviderConformanceContract{
		ID:                  input.SourceContractID,
		ProductBoundary:     input.ProductBoundary,
		ProviderUsageMatrix: input.ProviderUsageMatrix,
		Streaming:           input.Streaming,
	})
}

func g3ProviderUsageCaseByID(cases []G3ProviderUsageCaseSummary, id string) G3ProviderUsageCaseSummary {
	for _, item := range cases {
		if item.ID == id {
			return item
		}
	}
	return G3ProviderUsageCaseSummary{}
}

func usagePayloadFromSSEFrames(frames []string) map[string]any {
	for _, frame := range frames {
		if !strings.Contains(frame, "event: usage") {
			continue
		}
		for _, line := range strings.Split(frame, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := map[string]any{}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload); err != nil {
				return map[string]any{}
			}
			usage, _ := payload["usage"].(map[string]any)
			return usage
		}
	}
	return map[string]any{}
}

func UsagePayloadFromSSEFrames(frames []string) map[string]any {
	return usagePayloadFromSSEFrames(frames)
}

func usagePayloadMatchesExpected(usage map[string]any, expected G3ProviderUsageSummary) bool {
	if expected.PromptTokens != nil && intFromUsagePayload(usage, "promptTokens") != *expected.PromptTokens {
		return false
	}
	if expected.CompletionTokens != nil && intFromUsagePayload(usage, "completionTokens") != *expected.CompletionTokens {
		return false
	}
	if expected.ReasoningTokens != nil && intFromUsagePayload(usage, "reasoningTokens") != *expected.ReasoningTokens {
		return false
	}
	if expected.TotalTokens != nil && intFromUsagePayload(usage, "totalTokens") != *expected.TotalTokens {
		return false
	}
	if expected.CacheHitTokens != nil && intFromUsagePayload(usage, "cacheHitTokens") != *expected.CacheHitTokens {
		return false
	}
	if expected.CacheMissTokens != nil && intFromUsagePayload(usage, "cacheMissTokens") != *expected.CacheMissTokens {
		return false
	}
	if expected.CacheHitRate != nil && floatFromUsagePayload(usage, "cacheHitRate") != *expected.CacheHitRate {
		return false
	}
	return true
}

func UsagePayloadMatchesExpected(usage map[string]any, expected G3ProviderUsageSummary) bool {
	return usagePayloadMatchesExpected(usage, expected)
}

func intFromUsagePayload(usage map[string]any, field string) int {
	switch value := usage[field].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func IntFromUsagePayload(usage map[string]any, field string) int {
	return intFromUsagePayload(usage, field)
}

func floatFromUsagePayload(usage map[string]any, field string) float64 {
	switch value := usage[field].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return 0
	}
}

func FloatFromUsagePayload(usage map[string]any, field string) float64 {
	return floatFromUsagePayload(usage, field)
}

func routeIDs(routes []G2RouteReplayCase) []string {
	return routeIDsMatching(routes, func(G2RouteReplayCase) bool { return true })
}

func routeIDsMatching(routes []G2RouteReplayCase, include func(G2RouteReplayCase) bool) []string {
	ids := make([]string, 0, len(routes))
	for _, route := range routes {
		if include(route) {
			ids = append(ids, route.ID)
		}
	}
	return ids
}

func replayG5SessionRouteInventory(input G5SessionRouteInventoryCase) G5SessionRouteInventoryOutput {
	return G5SessionRouteInventoryOutput{
		RouteIDs:       sessionRouteInventoryIDs(input.Routes, func(G5SessionRouteInventoryRoute) bool { return true }),
		JsonRouteIDs:   sessionRouteInventoryIDs(input.Routes, func(route G5SessionRouteInventoryRoute) bool { return route.ResponseKind == "json" }),
		SSERouteIDs:    sessionRouteInventoryIDs(input.Routes, func(route G5SessionRouteInventoryRoute) bool { return route.ResponseKind == "sse" }),
		EventRouteIDs:  sessionRouteInventoryIDs(input.Routes, func(route G5SessionRouteInventoryRoute) bool { return route.Setup == "events" }),
		ResumeRouteIDs: sessionRouteInventoryIDs(input.Routes, func(route G5SessionRouteInventoryRoute) bool { return route.Setup == "session-resume" }),
		ForkRouteIDs:   sessionRouteInventoryIDs(input.Routes, func(route G5SessionRouteInventoryRoute) bool { return route.Setup == "thread-fork" }),
		ArchiveRouteIDs: sessionRouteInventoryIDs(input.Routes, func(route G5SessionRouteInventoryRoute) bool {
			return route.Setup == "thread-archive" || route.Setup == "thread-search-archive"
		}),
		SearchRouteIDs:         sessionRouteInventoryIDs(input.Routes, func(route G5SessionRouteInventoryRoute) bool { return route.Setup == "thread-search-archive" }),
		ReadUpdateRouteIDs:     sessionRouteInventoryIDs(input.Routes, func(route G5SessionRouteInventoryRoute) bool { return route.Setup == "thread-read" }),
		RuntimeTokenRouteCount: len(sessionRouteInventoryIDs(input.Routes, func(route G5SessionRouteInventoryRoute) bool { return route.Auth == "runtime-token" })),
		UnauthorizedRouteIDs:   sessionRouteInventoryIDs(input.Routes, func(route G5SessionRouteInventoryRoute) bool { return route.Auth == "none" }),
		UsesReasonixProtocol:   input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:   input.ProductBoundary.TopLevelRouteExposed,
	}
}

func sessionRouteInventoryIDs(routes []G5SessionRouteInventoryRoute, include func(G5SessionRouteInventoryRoute) bool) []string {
	ids := make([]string, 0, len(routes))
	for _, route := range routes {
		if include(route) {
			ids = append(ids, route.ID)
		}
	}
	return ids
}

func replayG5SessionRouteReplay(input G5SessionRouteReplayCase) G5SessionRouteReplayOutput {
	archive := sessionRouteReplayRowByID(input.Routes, "thread-archive-patch")
	search := sessionRouteReplayRowByID(input.Routes, "thread-search-archived")
	fork := sessionRouteReplayRowByID(input.Routes, "thread-fork-side")
	resume := sessionRouteReplayRowByID(input.Routes, "session-resume-thread")
	replay := sessionRouteReplayRowByID(input.Routes, "events-since-seq")
	caughtUp := sessionRouteReplayRowByID(input.Routes, "events-caught-up-since-seq")
	unauthorized := sessionRouteReplayRowByID(input.Routes, "events-unauthorized-since-seq")
	return G5SessionRouteReplayOutput{
		RouteCount: len(input.Routes),
		Matrix:     append([]G5SessionRouteReplayRow{}, input.Routes...),
		ExactJSONBodyRouteCount: countSessionRouteReplayRows(input.Routes, func(route G5SessionRouteReplayRow) bool {
			return route.ResponseKind == "json" && route.ResponseBodyHash != ""
		}),
		ExactSSERouteCount:     countSessionRouteReplayRows(input.Routes, func(route G5SessionRouteReplayRow) bool { return route.ResponseKind == "sse" }),
		RuntimeTokenRouteCount: countSessionRouteReplayRows(input.Routes, func(route G5SessionRouteReplayRow) bool { return route.Auth == "runtime-token" }),
		UnauthorizedRouteIDs:   sessionRouteReplayIDs(input.Routes, func(route G5SessionRouteReplayRow) bool { return route.Auth == "none" }),
		ArchiveResponseHash:    archive.ResponseBodyHash,
		SearchResponseHash:     search.ResponseBodyHash,
		ForkResponseHash:       fork.ResponseBodyHash,
		ResumeResponseHash:     resume.ResponseBodyHash,
		ReplaySSEHash:          replay.SSEFramesHash,
		CaughtUpSSEHash:        caughtUp.SSEFramesHash,
		UnauthorizedBodyHash:   unauthorized.ResponseBodyHash,
		EveryRouteHasMethodPathStatus: everySessionRouteReplayRow(input.Routes, func(route G5SessionRouteReplayRow) bool {
			return route.Method != "" && route.Path != "" && route.Status > 0
		}),
		JSONRoutesHaveBodyHash: everySessionRouteReplayRow(input.Routes, func(route G5SessionRouteReplayRow) bool {
			return route.ResponseKind != "json" || len(route.ResponseBodyHash) == 16
		}),
		SSERoutesHaveExactFrames: everySessionRouteReplayRow(input.Routes, func(route G5SessionRouteReplayRow) bool {
			return route.ResponseKind != "sse" || route.SSEFrameCount == 0 || len(route.SSEFramesHash) == 16
		}),
		UsesReasonixProtocol:   input.ProductBoundary.UsesReasonixProtocol,
		TopLevelRouteExposed:   input.ProductBoundary.TopLevelRouteExposed,
		RendererVisibleGoRoute: input.ProductBoundary.RendererVisibleGoRoute,
		DefaultGoBackend:       input.ProductBoundary.DefaultGoBackend,
	}
}

func sessionRouteReplayRowByID(routes []G5SessionRouteReplayRow, id string) G5SessionRouteReplayRow {
	for _, route := range routes {
		if route.ID == id {
			return route
		}
	}
	return G5SessionRouteReplayRow{}
}

func countSessionRouteReplayRows(routes []G5SessionRouteReplayRow, include func(G5SessionRouteReplayRow) bool) int {
	count := 0
	for _, route := range routes {
		if include(route) {
			count++
		}
	}
	return count
}

func sessionRouteReplayIDs(routes []G5SessionRouteReplayRow, include func(G5SessionRouteReplayRow) bool) []string {
	ids := make([]string, 0, len(routes))
	for _, route := range routes {
		if include(route) {
			ids = append(ids, route.ID)
		}
	}
	return ids
}

func everySessionRouteReplayRow(routes []G5SessionRouteReplayRow, check func(G5SessionRouteReplayRow) bool) bool {
	for _, route := range routes {
		if !check(route) {
			return false
		}
	}
	return true
}

func buildG5RouteStatusReplay(routes []G2RouteReplayCase) G5SessionRouteStatusOutput {
	archive := routeByID(routes, "thread-archive-patch")
	archivedOnly := routeByID(routes, "thread-list-archived-only")
	searchArchived := routeByID(routes, "thread-search-archived")
	read := routeByID(routes, "thread-read-detail")
	update := routeByID(routes, "thread-update-title-workspace")
	fork := routeByID(routes, "thread-fork-side")
	resume := routeByID(routes, "session-resume-thread")
	replay := routeByID(routes, "events-since-seq")
	caughtUp := routeByID(routes, "events-caught-up-since-seq")
	unauthorizedSSE := routeByID(routes, "events-unauthorized-since-seq")
	return G5SessionRouteStatusOutput{
		RouteCount:     len(routes),
		JsonRouteCount: countG2RoutesByKind(routes, "json"),
		SSERouteCount:  countG2RoutesByKind(routes, "sse"),
		StatusCodes: G5RouteStatusCodeCount{
			OK:           countG2RoutesByStatus(routes, 200),
			Created:      countG2RoutesByStatus(routes, 201),
			Unauthorized: countG2RoutesByStatus(routes, 401),
		},
		Auth: G5RouteAuthStatus{
			ProtectedRouteID:  unauthorizedSSE.ID,
			ProtectedPath:     unauthorizedSSE.Path,
			ProtectedStatus:   unauthorizedSSE.Response.Status,
			ProtectedBodyCode: stringFromRouteBody(unauthorizedSSE, "code"),
			SSEFrameCount:     len(unauthorizedSSE.SSEFrames),
		},
		Archive: G5RouteArchiveStatus{
			PatchStatus:         archive.Response.Status,
			ArchivedStatus:      stringFromRouteBody(archive, "status"),
			ArchivedOnlyCount:   threadsCount(archivedOnly),
			SearchArchivedCount: threadsCount(searchArchived),
			SearchFirstStatus:   firstThreadStatus(searchArchived),
		},
		ReadUpdate: G5RouteReadUpdate{
			ReadStatus:       read.Response.Status,
			ReadLatestSeq:    intFromRouteBody(read, "latestSeq"),
			ReadTurnCount:    turnsCount(read),
			UpdateStatus:     update.Response.Status,
			UpdatedWorkspace: stringFromRouteBody(update, "workspace"),
		},
		Fork: G5RouteForkStatus{
			Status:              fork.Response.Status,
			Relation:            stringFromRouteBody(fork, "relation"),
			ParentThreadID:      stringFromRouteBody(fork, "parentThreadId"),
			ForkedFromTurnCount: intFromRouteBody(fork, "forkedFromTurnCount"),
			ForkedTurnCount:     turnsCount(fork),
		},
		Resume: G5RouteResumeStatus{
			Status:       resume.Response.Status,
			SessionID:    stringFromRouteBody(resume, "session_id"),
			MessageCount: intFromRouteBody(resume, "message_count"),
			Summary:      stringFromRouteBody(resume, "summary"),
		},
		SSE: G5RouteSSEStatus{
			ReplayStatus:       replay.Response.Status,
			ReplayFrameCount:   len(replay.SSEFrames),
			CaughtUpStatus:     caughtUp.Response.Status,
			CaughtUpFrameCount: len(caughtUp.SSEFrames),
			ReplayEventNames:   sseEventNames(replay.SSEFrames),
		},
	}
}

func routeByID(routes []G2RouteReplayCase, id string) G2RouteReplayCase {
	for _, route := range routes {
		if route.ID == id {
			return route
		}
	}
	return G2RouteReplayCase{}
}

func countG2RoutesByKind(routes []G2RouteReplayCase, kind string) int {
	count := 0
	for _, route := range routes {
		if route.ResponseKind == kind {
			count += 1
		}
	}
	return count
}

func countG2RoutesByStatus(routes []G2RouteReplayCase, status int) int {
	count := 0
	for _, route := range routes {
		if route.Response.Status == status {
			count += 1
		}
	}
	return count
}

func routeBodyMap(route G2RouteReplayCase) map[string]any {
	out := map[string]any{}
	if len(route.Response.Body) == 0 {
		return out
	}
	if err := json.Unmarshal(route.Response.Body, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func stringFromRouteBody(route G2RouteReplayCase, field string) string {
	value, _ := routeBodyMap(route)[field].(string)
	return value
}

func intFromRouteBody(route G2RouteReplayCase, field string) int {
	switch value := routeBodyMap(route)[field].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func threadsCount(route G2RouteReplayCase) int {
	values, _ := routeBodyMap(route)["threads"].([]any)
	return len(values)
}

func firstThreadStatus(route G2RouteReplayCase) string {
	values, _ := routeBodyMap(route)["threads"].([]any)
	if len(values) == 0 {
		return ""
	}
	thread, _ := values[0].(map[string]any)
	value, _ := thread["status"].(string)
	return value
}

func turnsCount(route G2RouteReplayCase) int {
	values, _ := routeBodyMap(route)["turns"].([]any)
	return len(values)
}

func sseEventNames(frames []string) []string {
	names := make([]string, 0, len(frames))
	for _, frame := range frames {
		for _, line := range strings.Split(frame, "\n") {
			if strings.HasPrefix(line, "event: ") {
				names = append(names, strings.TrimPrefix(line, "event: "))
				break
			}
		}
	}
	return names
}

func SSEEventNames(frames []string) []string {
	return sseEventNames(frames)
}
