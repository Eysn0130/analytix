//go:build !analytix_prod

package mcp

type MCPToolLifecycleContract struct {
	ID                   string                 `json:"id"`
	ProviderID           string                 `json:"providerId"`
	Connect              MCPProviderConnect     `json:"connect"`
	Disconnect           MCPProviderDisconnect  `json:"disconnect"`
	Reload               MCPProviderReload      `json:"reload"`
	Cancel               ProviderCancel         `json:"cancel"`
	Error                MCPProviderError       `json:"error"`
	CallReconnect        MCPCallReconnect       `json:"callReconnect"`
	ApprovalAnnotations  MCPApprovalAnnotations `json:"approvalAnnotations"`
	SearchMetaTools      MCPSearchMetaTools     `json:"searchMetaTools"`
	DiagnosticsRedaction DiagnosticsRedaction   `json:"diagnosticsRedaction"`
	BackgroundReconnect  MCPBackgroundReconnect `json:"backgroundReconnect"`
	KnownOverride        MCPKnownOverride       `json:"knownOverride"`
	KnownOverrideVars    []MCPKnownOverride     `json:"knownOverrideVariants"`
	LiveLocalIndexer     MCPLiveLocalIndexer    `json:"liveLocalIndexer"`
}

type MCPProviderConnect struct {
	ToolNames  []string              `json:"toolNames"`
	Diagnostic MCPProviderDiagnostic `json:"diagnostic"`
}

type MCPProviderDisconnect struct {
	Reason     string                `json:"reason"`
	ToolNames  []string              `json:"toolNames"`
	Diagnostic MCPProviderDiagnostic `json:"diagnostic"`
}

type MCPProviderDiagnostic struct {
	Available bool `json:"available"`
	ToolCount int  `json:"toolCount"`
}

type MCPProviderReload struct {
	ToolNames         []string `json:"toolNames"`
	SchemaOrderStable bool     `json:"schemaOrderStable"`
}

type MCPProviderError struct {
	Code     string `json:"code"`
	Approved bool   `json:"approved"`
}

type MCPCallReconnect struct {
	ServerID              string                   `json:"serverId"`
	ToolName              string                   `json:"toolName"`
	NormalizedToolName    string                   `json:"normalizedToolName"`
	TransportError        string                   `json:"transportError"`
	ProtocolError         string                   `json:"protocolError"`
	RetryOnTransportError bool                     `json:"retryOnTransportError"`
	RetryOnProtocolError  bool                     `json:"retryOnProtocolError"`
	MaxAttempts           int                      `json:"maxAttempts"`
	StaleConnection       MCPStaleConnectionResult `json:"staleConnection"`
	ProtocolFailure       MCPProtocolFailureResult `json:"protocolFailure"`
}

type MCPStaleConnectionResult struct {
	FactoryAttempts int  `json:"factoryAttempts"`
	CloseCount      int  `json:"closeCount"`
	ResultInstance  int  `json:"resultInstance"`
	IsError         bool `json:"isError"`
}

type MCPProtocolFailureResult struct {
	FactoryAttempts int    `json:"factoryAttempts"`
	CloseCount      int    `json:"closeCount"`
	Code            string `json:"code"`
	IsError         bool   `json:"isError"`
}

type MCPApprovalAnnotations struct {
	ServerID           string                `json:"serverId"`
	ToolName           string                `json:"toolName"`
	NormalizedToolName string                `json:"normalizedToolName"`
	Annotations        MCPApprovalAnnotation `json:"annotations"`
	ApprovalID         string                `json:"approvalId"`
	Decision           string                `json:"decision"`
	ResultKind         string                `json:"resultKind"`
	Executed           bool                  `json:"executed"`
}

type MCPApprovalAnnotation struct {
	DestructiveHint bool `json:"destructiveHint"`
	OpenWorldHint   bool `json:"openWorldHint"`
}

type MCPSearchMetaTools struct {
	ToolNames              []string        `json:"toolNames"`
	TrustedWorkspace       string          `json:"trustedWorkspace"`
	UntrustedWorkspace     string          `json:"untrustedWorkspace"`
	TrustedToolID          string          `json:"trustedToolId"`
	Query                  string          `json:"query"`
	UnknownToolError       string          `json:"unknownToolError"`
	UntrustedSearchedTools int             `json:"untrustedSearchedTools"`
	CallPolicy             string          `json:"callPolicy"`
	DeniedCallExecuted     bool            `json:"deniedCallExecuted"`
	RefreshDrift           MCPRefreshDrift `json:"refreshDrift"`
}

type MCPRefreshDrift struct {
	ServerID             string   `json:"serverId"`
	InitialToolNames     []string `json:"initialToolNames"`
	ExpandedToolNames    []string `json:"expandedToolNames"`
	ExpectedTotalIndexed int      `json:"expectedTotalIndexed"`
	ExpectedCatalogDrift bool     `json:"expectedCatalogDrift"`
}

type MCPBackgroundReconnect struct {
	FailedServerIDs            []string `json:"failedServerIds"`
	SuspendedProviderID        string   `json:"suspendedProviderId"`
	SuspendedReason            string   `json:"suspendedReason"`
	ExpectedConnectedServerIDs []string `json:"expectedConnectedServerIds"`
	ExpectedErrorServerIDs     []string `json:"expectedErrorServerIds"`
	AttemptsPerFailedServer    int      `json:"attemptsPerFailedServer"`
	RequiresRuntimeRestart     bool     `json:"requiresRuntimeRestart"`
}

type MCPKnownOverride struct {
	ServerID            string                     `json:"serverId"`
	ExplicitCWD         string                     `json:"explicitCwd,omitempty"`
	WorkspaceRoot       string                     `json:"workspaceRoot"`
	DaemonIdleTimeoutMs string                     `json:"daemonIdleTimeoutMs,omitempty"`
	Diagnostic          MCPKnownOverrideDiagnostic `json:"diagnostic"`
}

type MCPKnownOverrideDiagnostic struct {
	KnownOverride   string `json:"knownOverride"`
	EffectiveCWD    string `json:"effectiveCwd"`
	LowPriority     bool   `json:"lowPriority"`
	BackgroundStart bool   `json:"backgroundStart"`
}

type MCPLiveLocalIndexer struct {
	ServerID          string                     `json:"serverId"`
	CWD               string                     `json:"cwd"`
	LowPriority       bool                       `json:"lowPriority"`
	BackgroundStart   bool                       `json:"backgroundStart"`
	FailedServerIDs   []string                   `json:"failedServerIds"`
	AttemptsPerServer int                        `json:"attemptsPerFailedServer"`
	InitialFiles      []MCPLiveLocalFile         `json:"initialFiles"`
	LateTombstonePath string                     `json:"lateTombstonePath"`
	ResumeFiles       []MCPLiveLocalFile         `json:"resumeFiles"`
	SecretDiagnostic  string                     `json:"secretDiagnostic"`
	ExpectedOutput    MCPLiveLocalExpectedOutput `json:"expectedOutput"`
	ExecutionError    MCPLiveLocalExecutionError `json:"executionError"`
}

type MCPLiveLocalFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type MCPLiveLocalExpectedOutput struct {
	ServerID              string         `json:"serverId"`
	CWD                   string         `json:"cwd"`
	LowPriority           bool           `json:"lowPriority"`
	BackgroundStart       bool           `json:"backgroundStart"`
	RetryAttempts         map[string]int `json:"retryAttempts"`
	ActivePaths           []string       `json:"activePaths"`
	TombstoneCount        int            `json:"tombstoneCount"`
	RestartedFromSnapshot bool           `json:"restartedFromSnapshot"`
	SecretSafeDiagnostic  string         `json:"secretSafeDiagnostic"`
}

type MCPLiveLocalExecutionError struct {
	ToolName        string `json:"toolName"`
	IsError         bool   `json:"isError"`
	SecretSafeError string `json:"secretSafeError"`
	LeaksSecret     bool   `json:"leaksSecret"`
}
