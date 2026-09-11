package runtimeinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const PublicRuntimeDiagnosticsSchemaVersion = 2

type PublicRuntimeInfoV2 struct {
	SchemaVersion   int                         `json:"schemaVersion"`
	Status          string                      `json:"status"`
	ListenerScope   string                      `json:"listenerScope"`
	Port            int                         `json:"port"`
	StartedAt       string                      `json:"startedAt"`
	Insecure        bool                        `json:"insecure"`
	Storage         PublicStorageStateV2        `json:"storage"`
	ExecutionPolicy PublicExecutionPolicyV2     `json:"executionPolicy"`
	Provider        PublicProviderStateV2       `json:"provider"`
	NetworkProxy    PublicNetworkProxyStateV2   `json:"networkProxy"`
	Capabilities    PublicRuntimeCapabilitiesV2 `json:"capabilities"`
}

type PublicStorageStateV2 struct {
	Configured bool `json:"configured"`
	Available  bool `json:"available"`
}

type PublicExecutionPolicyV2 struct {
	ApprovalPolicy string `json:"approvalPolicy"`
	SandboxMode    string `json:"sandboxMode"`
}

type PublicProviderStateV2 struct {
	ID                      string `json:"id"`
	Model                   string `json:"model"`
	Family                  string `json:"family"`
	EndpointFormat          string `json:"endpointFormat"`
	Available               bool   `json:"available"`
	APIKeyConfigured        bool   `json:"apiKeyConfigured"`
	BaseURLConfigured       bool   `json:"baseUrlConfigured"`
	CacheTelemetrySupported bool   `json:"cacheTelemetrySupported"`
	SupportsImageInput      bool   `json:"supportsImageInput"`
	ContextWindowTokens     int    `json:"contextWindowTokens,omitempty"`
	ReasoningEffort         string `json:"reasoningEffort,omitempty"`
}

type PublicNetworkProxyStateV2 struct {
	Mode              string `json:"mode"`
	Configured        bool   `json:"configured"`
	Source            string `json:"source"`
	Valid             bool   `json:"valid"`
	CredentialsMasked bool   `json:"credentialsMasked"`
}

type PublicCapabilityStateV2 struct {
	Status     string `json:"status"`
	Enabled    bool   `json:"enabled"`
	Available  bool   `json:"available"`
	ReasonCode string `json:"reasonCode"`
}

type PublicModelReasoningV2 struct {
	SupportedEfforts []string `json:"supportedEfforts"`
	DefaultEffort    string   `json:"defaultEffort"`
	RequestProtocol  string   `json:"requestProtocol"`
}

type PublicModelCapabilityV2 struct {
	ID                  string                  `json:"id"`
	ProviderID          string                  `json:"providerId,omitempty"`
	Family              string                  `json:"family,omitempty"`
	InputModalities     []string                `json:"inputModalities"`
	OutputModalities    []string                `json:"outputModalities"`
	SupportsToolCalling bool                    `json:"supportsToolCalling"`
	SupportsImageInput  bool                    `json:"supportsImageInput"`
	ContextWindowTokens int                     `json:"contextWindowTokens,omitempty"`
	MessageParts        []string                `json:"messageParts"`
	Reasoning           *PublicModelReasoningV2 `json:"reasoning,omitempty"`
	EndpointFormat      string                  `json:"endpointFormat,omitempty"`
}

type PublicCLICapabilitiesV2 struct {
	Serve PublicCapabilityStateV2 `json:"serve"`
	Run   PublicCapabilityStateV2 `json:"run"`
	Chat  PublicCapabilityStateV2 `json:"chat"`
	Exec  PublicCapabilityStateV2 `json:"exec"`
}

type PublicMCPCatalogV2 struct {
	Status              string `json:"status"`
	ReasonCode          string `json:"reasonCode"`
	ToolCount           int    `json:"toolCount"`
	AdvertisedToolCount int    `json:"advertisedToolCount"`
	PromptCount         int    `json:"promptCount"`
	ResourceCount       int    `json:"resourceCount"`
	CatalogFingerprint  string `json:"catalogFingerprint,omitempty"`
	CatalogDrift        bool   `json:"catalogDrift"`
}

type PublicMCPSearchV2 struct {
	Enabled                bool    `json:"enabled"`
	Mode                   string  `json:"mode"`
	Active                 bool    `json:"active"`
	Available              bool    `json:"available"`
	ReasonCode             string  `json:"reasonCode"`
	IndexedToolCount       int     `json:"indexedToolCount"`
	AdvertisedToolCount    int     `json:"advertisedToolCount"`
	AutoThresholdToolCount int     `json:"autoThresholdToolCount"`
	TopKDefault            int     `json:"topKDefault"`
	TopKMax                int     `json:"topKMax"`
	MinScore               float64 `json:"minScore"`
	CatalogFingerprint     string  `json:"catalogFingerprint,omitempty"`
	CatalogDrift           bool    `json:"catalogDrift"`
}

type PublicMCPCapabilityV2 struct {
	PublicCapabilityStateV2
	ConfiguredServers int                `json:"configuredServers"`
	ConnectedServers  int                `json:"connectedServers"`
	ToolCount         int                `json:"toolCount"`
	PromptCount       int                `json:"promptCount"`
	ResourceCount     int                `json:"resourceCount"`
	Catalog           PublicMCPCatalogV2 `json:"catalog"`
	Search            PublicMCPSearchV2  `json:"search"`
}

type PublicWebCapabilityV2 struct {
	PublicCapabilityStateV2
	Fetch         PublicCapabilityStateV2 `json:"fetch"`
	Search        PublicCapabilityStateV2 `json:"search"`
	Provider      string                  `json:"provider"`
	FetchEnabled  bool                    `json:"fetchEnabled"`
	SearchEnabled bool                    `json:"searchEnabled"`
}

type PublicSkillsCapabilityV2 struct {
	PublicCapabilityStateV2
	ConfiguredRoots  int `json:"configuredRoots"`
	DiscoveredSkills int `json:"discoveredSkills"`
}

type PublicSubagentCapabilityV2 struct {
	PublicCapabilityStateV2
	MaxParallel                     int    `json:"maxParallel"`
	MaxChildRuns                    int    `json:"maxChildRuns"`
	DefaultToolPolicy               string `json:"defaultToolPolicy"`
	ProfileCount                    int    `json:"profileCount"`
	InternalLineageAvailable        bool   `json:"internalLineageAvailable"`
	ProfilesAvailable               bool   `json:"profilesAvailable"`
	DurableChildRunStore            bool   `json:"durableChildRunStore"`
	ParallelExecutionAvailable      bool   `json:"parallelExecutionAvailable"`
	TaskToolAvailable               bool   `json:"taskToolAvailable"`
	ParallelTasksToolAvailable      bool   `json:"parallelTasksToolAvailable"`
	BackgroundTaskJobsAvailable     bool   `json:"backgroundTaskJobsAvailable"`
	BackgroundShellAvailable        bool   `json:"backgroundShellAvailable"`
	BackgroundSubagentJobsAvailable bool   `json:"backgroundSubagentJobsAvailable"`
	ModelJobToolsAvailable          bool   `json:"modelJobToolsAvailable"`
	TaskJobThreadScopeSupported     bool   `json:"taskJobThreadScopeSupported"`
}

type PublicAttachmentsCapabilityV2 struct {
	PublicCapabilityStateV2
	MaxImageBytes                 int      `json:"maxImageBytes"`
	MaxImageDimension             int      `json:"maxImageDimension"`
	AllowedMimeTypes              []string `json:"allowedMimeTypes"`
	AllowedDocumentMimeTypes      []string `json:"allowedDocumentMimeTypes"`
	MaxDocumentBytes              int      `json:"maxDocumentBytes"`
	MaxDocumentTextChars          int      `json:"maxDocumentTextChars"`
	TextFallbackMaxBase64Bytes    int      `json:"textFallbackMaxBase64Bytes"`
	TextFallbackMaxImageDimension int      `json:"textFallbackMaxImageDimension"`
	TextFallbackPreferredMimeType string   `json:"textFallbackPreferredMimeType"`
}

type PublicMemoryCapabilityV2 struct {
	PublicCapabilityStateV2
	Mode               string   `json:"mode"`
	StoreOnly          bool     `json:"storeOnly"`
	ModelInjection     bool     `json:"modelInjection"`
	AutomaticCapture   bool     `json:"automaticCapture"`
	Scopes             []string `json:"scopes"`
	MaxInjectedRecords int      `json:"maxInjectedRecords"`
}

type PublicGeneratedCapabilityV2 struct {
	PublicCapabilityStateV2
	Model string `json:"model,omitempty"`
}

type PublicComputerUseCapabilityV2 struct {
	PublicCapabilityStateV2
	Mode               string `json:"mode"`
	BackendID          string `json:"backendId,omitempty"`
	PreferredBackendID string `json:"preferredBackendId,omitempty"`
}

type PublicVisionBridgeCapabilityV2 struct {
	PublicCapabilityStateV2
	Mode                  string `json:"mode"`
	Model                 string `json:"model,omitempty"`
	ProviderID            string `json:"providerId,omitempty"`
	MaxScreenshotsPerTurn int    `json:"maxScreenshotsPerTurn"`
	MaxImagesPerTurn      int    `json:"maxImagesPerTurn"`
	MaxImageBytes         int    `json:"maxImageBytes"`
	MaxImageDimension     int    `json:"maxImageDimension"`
	SemanticProbeStatus   string `json:"semanticProbeStatus"`
}

type PublicRuntimeCapabilitiesV2 struct {
	ContractVersion int                            `json:"contractVersion"`
	Model           PublicModelCapabilityV2        `json:"model"`
	CLI             PublicCLICapabilitiesV2        `json:"cli"`
	MCP             PublicMCPCapabilityV2          `json:"mcp"`
	Web             PublicWebCapabilityV2          `json:"web"`
	Skills          PublicSkillsCapabilityV2       `json:"skills"`
	Subagents       PublicSubagentCapabilityV2     `json:"subagents"`
	Attachments     PublicAttachmentsCapabilityV2  `json:"attachments"`
	Memory          PublicMemoryCapabilityV2       `json:"memory"`
	ImageGen        PublicGeneratedCapabilityV2    `json:"imageGen"`
	SpeechGen       PublicGeneratedCapabilityV2    `json:"speechGen"`
	MusicGen        PublicGeneratedCapabilityV2    `json:"musicGen"`
	VideoGen        PublicGeneratedCapabilityV2    `json:"videoGen"`
	ComputerUse     PublicComputerUseCapabilityV2  `json:"computerUse"`
	VisionBridge    PublicVisionBridgeCapabilityV2 `json:"visionBridge"`
}

type PublicRuntimeToolsV2 struct {
	SchemaVersion    int                           `json:"schemaVersion"`
	ProviderCount    int                           `json:"providerCount"`
	ToolContracts    PublicToolContractCatalogV2   `json:"toolContracts"`
	MCPServers       []PublicMCPServerDiagnosticV2 `json:"mcpServers"`
	MCPSearch        PublicMCPSearchV2             `json:"mcpSearch"`
	MCPPromptCount   int                           `json:"mcpPromptCount"`
	MCPResourceCount int                           `json:"mcpResourceCount"`
	Commands         []PublicCommandDiagnosticV2   `json:"commands"`
	NetworkProxy     PublicNetworkProxyStateV2     `json:"networkProxy"`
	WebProviderCount int                           `json:"webProviderCount"`
	Skills           PublicSkillDiagnosticsV2      `json:"skills"`
	Attachments      PublicAttachmentDiagnosticsV2 `json:"attachments"`
	Memory           PublicMemoryDiagnosticsV2     `json:"memory"`
	Subagents        PublicSubagentDiagnosticsV2   `json:"subagents"`
}

type PublicToolContractCatalogV2 struct {
	Count       int    `json:"count"`
	CatalogHash string `json:"catalogHash"`
}

type PublicMCPServerDiagnosticV2 struct {
	ID                          string `json:"id"`
	Status                      string `json:"status"`
	FailureCode                 string `json:"failureCode,omitempty"`
	Transport                   string `json:"transport"`
	AuthStatus                  string `json:"authStatus"`
	TrustScope                  string `json:"trustScope"`
	Enabled                     bool   `json:"enabled"`
	Available                   bool   `json:"available"`
	Connected                   bool   `json:"connected"`
	SchemaHintAvailable         bool   `json:"schemaHintAvailable"`
	Connectable                 bool   `json:"connectable"`
	ToolCount                   int    `json:"toolCount"`
	PromptCount                 int    `json:"promptCount"`
	ResourceCount               int    `json:"resourceCount"`
	ToolContractQuarantineCount int    `json:"toolContractQuarantineCount"`
	SourceProbeCount            int    `json:"sourceProbeCount"`
	LowPriority                 bool   `json:"lowPriority"`
	BackgroundStart             bool   `json:"backgroundStart"`
}

type PublicCommandDiagnosticV2 struct {
	Binary string `json:"binary"`
	Found  bool   `json:"found"`
	Status string `json:"status"`
}

type PublicSkillDiagnosticsV2 struct {
	Enabled              bool   `json:"enabled"`
	Available            bool   `json:"available"`
	ReasonCode           string `json:"reasonCode"`
	ConfiguredRootCount  int    `json:"configuredRootCount"`
	SkillCount           int    `json:"skillCount"`
	ValidationErrorCount int    `json:"validationErrorCount"`
}

type PublicAttachmentDiagnosticsV2 struct {
	SchemaVersion            int      `json:"schemaVersion,omitempty"`
	Enabled                  bool     `json:"enabled"`
	Count                    int      `json:"count"`
	TotalBytes               int      `json:"totalBytes"`
	MaxImageBytes            int      `json:"maxImageBytes"`
	MaxImageDimension        int      `json:"maxImageDimension"`
	AllowedMimeTypes         []string `json:"allowedMimeTypes"`
	AllowedDocumentMimeTypes []string `json:"allowedDocumentMimeTypes"`
	MaxDocumentBytes         int      `json:"maxDocumentBytes"`
	MaxDocumentTextChars     int      `json:"maxDocumentTextChars"`
}

type PublicMemoryDiagnosticsV2 struct {
	SchemaVersion  int    `json:"schemaVersion,omitempty"`
	Enabled        bool   `json:"enabled"`
	Status         string `json:"status"`
	ReasonCode     string `json:"reasonCode,omitempty"`
	ActiveCount    *int   `json:"activeCount,omitempty"`
	TombstoneCount *int   `json:"tombstoneCount,omitempty"`
}

type PublicSubagentDiagnosticsV2 struct {
	Status                          string `json:"status"`
	Enabled                         bool   `json:"enabled"`
	Available                       bool   `json:"available"`
	ReasonCode                      string `json:"reasonCode"`
	Active                          int    `json:"active"`
	Queued                          int    `json:"queued"`
	ProfileCount                    int    `json:"profileCount"`
	MaxParallel                     int    `json:"maxParallel"`
	MaxChildRuns                    int    `json:"maxChildRuns"`
	DefaultToolPolicy               string `json:"defaultToolPolicy"`
	InternalLineageAvailable        bool   `json:"internalLineageAvailable"`
	ParallelExecutionAvailable      bool   `json:"parallelExecutionAvailable"`
	TaskToolAvailable               bool   `json:"taskToolAvailable"`
	ParallelTasksToolAvailable      bool   `json:"parallelTasksToolAvailable"`
	BackgroundTaskJobsAvailable     bool   `json:"backgroundTaskJobsAvailable"`
	BackgroundShellAvailable        bool   `json:"backgroundShellAvailable"`
	BackgroundSubagentJobsAvailable bool   `json:"backgroundSubagentJobsAvailable"`
}

func ProjectPublicRuntimeInfo(input RuntimeInfoInput) PublicRuntimeInfoV2 {
	startedAt, timestampOK := publicTimestamp(input.StartedAt)
	configuredStorage := strings.TrimSpace(input.DataDir) != ""
	return PublicRuntimeInfoV2{
		SchemaVersion: PublicRuntimeDiagnosticsSchemaVersion,
		Status:        map[bool]string{true: "ready", false: "degraded"}[timestampOK && configuredStorage],
		ListenerScope: publicListenerScope(input.Host),
		Port:          publicPort(input.Port),
		StartedAt:     startedAt,
		Insecure:      input.Insecure,
		Storage:       PublicStorageStateV2{Configured: configuredStorage, Available: configuredStorage},
		ExecutionPolicy: PublicExecutionPolicyV2{
			ApprovalPolicy: publicEnum(input.ApprovalPolicy, []string{"always", "on-request", "untrusted", "never", "auto", "suggest"}, "unknown"),
			SandboxMode:    publicEnum(input.SandboxMode, []string{"read-only", "workspace-write", "danger-full-access", "external-sandbox"}, "unknown"),
		},
		Provider:     publicProviderState(input.DefaultProvider),
		NetworkProxy: ProjectPublicNetworkProxy(input.NetworkProxy),
		Capabilities: ProjectPublicRuntimeCapabilities(input.Capabilities),
	}
}

func ProjectPublicRuntimeCapabilities(raw map[string]any) PublicRuntimeCapabilitiesV2 {
	if raw == nil {
		raw = RuntimeCapabilitiesResponse()
	}
	model := publicMap(raw["model"])
	cli := publicMap(raw["cli"])
	mcp := publicMap(raw["mcp"])
	web := publicMap(raw["web"])
	skills := publicMap(raw["skills"])
	subagents := publicMap(raw["subagents"])
	attachments := publicMap(raw["attachments"])
	memory := publicMap(raw["memory"])
	return PublicRuntimeCapabilitiesV2{
		ContractVersion: publicCount(raw["contractVersion"]),
		Model:           publicModelCapability(model),
		CLI: PublicCLICapabilitiesV2{
			Serve: publicCapabilityState(publicMap(cli["serve"])),
			Run:   publicCapabilityState(publicMap(cli["run"])),
			Chat:  publicCapabilityState(publicMap(cli["chat"])),
			Exec:  publicCapabilityState(publicMap(cli["exec"])),
		},
		MCP: PublicMCPCapabilityV2{
			PublicCapabilityStateV2: publicCapabilityState(mcp),
			ConfiguredServers:       publicCount(mcp["configuredServers"]),
			ConnectedServers:        publicCount(mcp["connectedServers"]),
			ToolCount:               publicCount(mcp["toolCount"]),
			PromptCount:             publicCount(mcp["promptCount"]),
			ResourceCount:           publicCount(mcp["resourceCount"]),
			Catalog:                 publicMCPCatalog(publicMap(mcp["catalog"])),
			Search:                  ProjectPublicMCPSearch(publicMap(mcp["search"])),
		},
		Web: PublicWebCapabilityV2{
			PublicCapabilityStateV2: publicCapabilityState(web),
			Fetch:                   publicCapabilityState(publicMap(web["fetch"])),
			Search:                  publicCapabilityState(publicMap(web["search"])),
			Provider:                publicIdentifier(publicString(web["provider"]), "unknown"),
			FetchEnabled:            publicBool(web["fetchEnabled"]),
			SearchEnabled:           publicBool(web["searchEnabled"]),
		},
		Skills: PublicSkillsCapabilityV2{
			PublicCapabilityStateV2: publicCapabilityState(skills),
			ConfiguredRoots:         publicCount(skills["configuredRoots"]),
			DiscoveredSkills:        publicCount(skills["discoveredSkills"]),
		},
		Subagents:    publicSubagentCapability(subagents),
		Attachments:  publicAttachmentsCapability(attachments),
		Memory:       publicMemoryCapability(memory),
		ImageGen:     publicGeneratedCapability(publicMap(raw["imageGen"])),
		SpeechGen:    publicGeneratedCapability(publicMap(raw["speechGen"])),
		MusicGen:     publicGeneratedCapability(publicMap(raw["musicGen"])),
		VideoGen:     publicGeneratedCapability(publicMap(raw["videoGen"])),
		ComputerUse:  publicComputerUseCapability(publicMap(raw["computerUse"])),
		VisionBridge: publicVisionBridgeCapability(publicMap(raw["visionBridge"])),
	}
}

func ProjectPublicRuntimeTools(input RuntimeToolsDiagnosticsInput) PublicRuntimeToolsV2 {
	return PublicRuntimeToolsV2{
		SchemaVersion:    PublicRuntimeDiagnosticsSchemaVersion,
		ProviderCount:    len(input.Providers),
		ToolContracts:    publicToolContractCatalog(input.ToolContracts),
		MCPServers:       ProjectPublicMCPServers(input.MCPServers),
		MCPSearch:        ProjectPublicMCPSearch(input.MCPSearch),
		MCPPromptCount:   len(input.MCPPrompts),
		MCPResourceCount: len(input.MCPResources),
		Commands:         ProjectPublicCommands(input.Commands),
		NetworkProxy:     ProjectPublicNetworkProxy(input.NetworkProxy),
		WebProviderCount: len(input.WebProviders),
		Skills:           ProjectPublicSkills(input.Skills),
		Attachments:      ProjectPublicAttachments(input.Attachments, false),
		Memory:           ProjectPublicMemory(input.Memory, false),
		Subagents:        ProjectPublicSubagents(input.Subagents),
	}
}

func ProjectPublicNetworkProxy(raw map[string]any) PublicNetworkProxyStateV2 {
	return PublicNetworkProxyStateV2{
		Mode:              publicEnum(publicString(raw["mode"]), []string{"auto", "env", "off", "custom"}, "unknown"),
		Configured:        publicBool(raw["configured"]),
		Source:            publicEnum(publicString(raw["source"]), []string{"environment", "settings.provider.proxy"}, "unknown"),
		Valid:             publicBool(raw["valid"]),
		CredentialsMasked: publicBool(raw["credentialsMasked"]),
	}
}

func ProjectPublicMCPSearch(raw map[string]any) PublicMCPSearchV2 {
	mode := publicEnum(publicString(raw["mode"]), []string{"direct", "search", "auto"}, "auto")
	enabled := publicBool(raw["enabled"])
	available := enabled && publicBool(raw["available"])
	return PublicMCPSearchV2{
		Enabled:                enabled,
		Mode:                   mode,
		Active:                 available && publicBool(raw["active"]),
		Available:              available,
		ReasonCode:             publicCapabilityReasonCode(enabled, available),
		IndexedToolCount:       publicCount(raw["indexedToolCount"]),
		AdvertisedToolCount:    publicCount(raw["advertisedToolCount"]),
		AutoThresholdToolCount: publicCount(raw["autoThresholdToolCount"]),
		TopKDefault:            publicCount(raw["topKDefault"]),
		TopKMax:                publicCount(raw["topKMax"]),
		MinScore:               publicBoundedFloat(raw["minScore"], 0, 1),
		CatalogFingerprint:     publicSHA256(publicString(raw["catalogFingerprint"])),
		CatalogDrift:           publicBool(raw["catalogDrift"]),
	}
}

func ProjectPublicMCPServers(items []any) []PublicMCPServerDiagnosticV2 {
	out := make([]PublicMCPServerDiagnosticV2, 0, len(items))
	for _, item := range items {
		record := publicMap(item)
		if record == nil {
			continue
		}
		id := publicIdentifier(publicString(record["id"]), "")
		if id == "" {
			id = "server-redacted-" + shortPublicHash(publicString(record["id"]))
		}
		connected := publicBool(record["connected"])
		failure := publicString(record["failure"])
		status := "unavailable"
		failureCode := ""
		switch {
		case failure != "":
			status, failureCode = "error", "connection_failed"
		case connected:
			status = "connected"
		case publicBool(record["schemaHintAvailable"]) || publicBool(record["lazyPlaceholder"]):
			status = "configured"
		}
		out = append(out, PublicMCPServerDiagnosticV2{
			ID:                          id,
			Status:                      status,
			FailureCode:                 failureCode,
			Transport:                   publicMCPTransport(publicString(record["transport"])),
			AuthStatus:                  publicEnum(publicString(record["authStatus"]), []string{"none", "possible", "required"}, "unknown"),
			TrustScope:                  publicEnum(publicString(record["trustScope"]), []string{"user", "workspace"}, "unknown"),
			Enabled:                     publicBool(record["enabled"]),
			Available:                   connected && publicBool(record["available"]),
			Connected:                   connected,
			SchemaHintAvailable:         publicBool(record["schemaHintAvailable"]),
			Connectable:                 publicBool(record["connectable"]),
			ToolCount:                   publicCount(record["toolCount"]),
			PromptCount:                 publicCount(record["promptCount"]),
			ResourceCount:               publicCount(record["resourceCount"]),
			ToolContractQuarantineCount: publicCount(record["toolContractQuarantineCount"]),
			SourceProbeCount:            publicCount(record["sourceProbeCount"]),
			LowPriority:                 publicBool(record["lowPriority"]),
			BackgroundStart:             publicBool(record["backgroundStart"]),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func ProjectPublicCommands(items []any) []PublicCommandDiagnosticV2 {
	allowed := map[string]struct{}{"go": {}, "node": {}, "npm": {}, "git": {}, "rg": {}, "rustc": {}, "cargo": {}, "make": {}, "docker": {}, "python3": {}, "python": {}}
	out := make([]PublicCommandDiagnosticV2, 0, len(items))
	for _, item := range items {
		record := publicMap(item)
		binary := publicString(record["binary"])
		if _, ok := allowed[binary]; !ok {
			continue
		}
		found := publicBool(record["found"])
		out = append(out, PublicCommandDiagnosticV2{Binary: binary, Found: found, Status: map[bool]string{true: "available", false: "unavailable"}[found]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Binary < out[j].Binary })
	return out
}

func ProjectPublicSkills(raw map[string]any) PublicSkillDiagnosticsV2 {
	enabled := publicBool(raw["enabled"])
	available := publicBool(raw["available"])
	return PublicSkillDiagnosticsV2{
		Enabled:              enabled,
		Available:            available,
		ReasonCode:           publicCapabilityReasonCode(enabled, available),
		ConfiguredRootCount:  len(publicList(raw["roots"])),
		SkillCount:           len(publicList(raw["skills"])),
		ValidationErrorCount: len(publicList(raw["validationErrors"])),
	}
}

func ProjectPublicAttachments(raw map[string]any, standalone bool) PublicAttachmentDiagnosticsV2 {
	version := 0
	if standalone {
		version = PublicRuntimeDiagnosticsSchemaVersion
	}
	return PublicAttachmentDiagnosticsV2{
		SchemaVersion:            version,
		Enabled:                  publicBool(raw["enabled"]),
		Count:                    publicCount(raw["count"]),
		TotalBytes:               publicCount(raw["totalBytes"]),
		MaxImageBytes:            publicCount(raw["maxImageBytes"]),
		MaxImageDimension:        publicCount(raw["maxImageDimension"]),
		AllowedMimeTypes:         publicMIMETypes(raw["allowedMimeTypes"]),
		AllowedDocumentMimeTypes: publicMIMETypes(raw["allowedDocumentMimeTypes"]),
		MaxDocumentBytes:         publicCount(raw["maxDocumentBytes"]),
		MaxDocumentTextChars:     publicCount(raw["maxDocumentTextChars"]),
	}
}

func ProjectPublicMemory(raw map[string]any, standalone bool) PublicMemoryDiagnosticsV2 {
	version := 0
	if standalone {
		version = PublicRuntimeDiagnosticsSchemaVersion
	}
	active, activeKnown := publicOptionalCount(raw["activeCount"])
	tombstone, tombstoneKnown := publicOptionalCount(raw["tombstoneCount"])
	status := publicString(raw["status"])
	if status == "" && activeKnown && tombstoneKnown {
		status = "ok"
	}
	if status != "ok" || !activeKnown || !tombstoneKnown {
		reasonCode := publicEnum(publicString(raw["reasonCode"]), []string{"memory_store_read_failed", "memory_store_unavailable"}, "memory_store_unavailable")
		return PublicMemoryDiagnosticsV2{
			SchemaVersion: version,
			Enabled:       publicBool(raw["enabled"]),
			Status:        "unavailable",
			ReasonCode:    reasonCode,
		}
	}
	return PublicMemoryDiagnosticsV2{
		SchemaVersion:  version,
		Enabled:        publicBool(raw["enabled"]),
		Status:         "ok",
		ActiveCount:    active,
		TombstoneCount: tombstone,
	}
}

func ProjectPublicSubagents(raw map[string]any) PublicSubagentDiagnosticsV2 {
	enabled := publicBool(raw["enabled"])
	available := publicBool(raw["available"])
	return PublicSubagentDiagnosticsV2{
		Status:                          publicCapabilityStatus(enabled, available),
		Enabled:                         enabled,
		Available:                       available,
		ReasonCode:                      publicCapabilityReasonCode(enabled, available),
		Active:                          publicCount(raw["active"]),
		Queued:                          publicCount(raw["queued"]),
		ProfileCount:                    len(publicList(raw["profiles"])),
		MaxParallel:                     publicCount(raw["maxParallel"]),
		MaxChildRuns:                    publicCount(raw["maxChildRuns"]),
		DefaultToolPolicy:               publicEnum(publicString(raw["defaultToolPolicy"]), []string{"readOnly", "inherit"}, "readOnly"),
		InternalLineageAvailable:        publicBool(raw["internalLineageAvailable"]),
		ParallelExecutionAvailable:      publicBool(raw["parallelExecutionAvailable"]),
		TaskToolAvailable:               publicBool(raw["taskToolAvailable"]),
		ParallelTasksToolAvailable:      publicBool(raw["parallelTasksToolAvailable"]),
		BackgroundTaskJobsAvailable:     publicBool(raw["backgroundTaskJobsAvailable"]),
		BackgroundShellAvailable:        publicBool(raw["backgroundShellAvailable"]),
		BackgroundSubagentJobsAvailable: publicBool(raw["backgroundSubagentJobsAvailable"]),
	}
}

func publicProviderState(config domainmodel.TurnConfig) PublicProviderStateV2 {
	apiKeyConfigured := strings.TrimSpace(config.APIKey) != ""
	baseURLConfigured := strings.TrimSpace(config.BaseURL) != ""
	model := publicIdentifier(config.Model, "unknown")
	return PublicProviderStateV2{
		ID:                      publicIdentifier(config.ProviderID, "unknown"),
		Model:                   model,
		Family:                  publicIdentifier(config.Family, "unknown"),
		EndpointFormat:          publicEnum(config.EndpointFormat, []string{"chat_completions", "responses", "messages", "custom_endpoint"}, "unknown"),
		Available:               apiKeyConfigured && baseURLConfigured && model != "unknown",
		APIKeyConfigured:        apiKeyConfigured,
		BaseURLConfigured:       baseURLConfigured,
		CacheTelemetrySupported: config.CacheTelemetrySupported,
		SupportsImageInput:      config.SupportsImageInput,
		ContextWindowTokens:     publicCount(config.ContextWindowTokens),
		ReasoningEffort:         publicReasoningEffort(config.ReasoningEffort),
	}
}

func publicModelCapability(raw map[string]any) PublicModelCapabilityV2 {
	result := PublicModelCapabilityV2{
		ID:                  publicIdentifier(publicString(raw["id"]), "unknown"),
		ProviderID:          publicIdentifier(publicString(raw["providerId"]), ""),
		Family:              publicIdentifier(publicString(raw["family"]), ""),
		InputModalities:     publicEnumList(raw["inputModalities"], []string{"text", "image"}, []string{"text"}),
		OutputModalities:    publicEnumList(raw["outputModalities"], []string{"text", "image"}, []string{"text"}),
		SupportsToolCalling: publicBool(raw["supportsToolCalling"]),
		SupportsImageInput:  publicBool(raw["supportsImageInput"]),
		ContextWindowTokens: publicCount(raw["contextWindowTokens"]),
		MessageParts:        publicEnumList(raw["messageParts"], []string{"text", "image_url", "input_image"}, []string{"text"}),
		EndpointFormat:      publicEnum(publicString(raw["endpointFormat"]), []string{"chat_completions", "responses", "messages", "custom_endpoint"}, ""),
	}
	if reasoning := publicMap(raw["reasoning"]); reasoning != nil {
		supportedEfforts := publicEnumList(reasoning["supportedEfforts"], []string{"auto", "off", "low", "medium", "high", "max"}, nil)
		defaultEffort := publicEnum(publicString(reasoning["defaultEffort"]), []string{"auto", "off", "low", "medium", "high", "max"}, "")
		requestProtocol := publicEnum(publicString(reasoning["requestProtocol"]), []string{"deepseek-chat-completions", "glm-chat-completions", "mimo-chat-completions", "openai-responses", "anthropic-thinking"}, "")
		if len(supportedEfforts) == 0 || requestProtocol == "" || !containsString(supportedEfforts, defaultEffort) {
			return result
		}
		result.Reasoning = &PublicModelReasoningV2{
			SupportedEfforts: supportedEfforts,
			DefaultEffort:    defaultEffort,
			RequestProtocol:  requestProtocol,
		}
	}
	return result
}

func publicCapabilityState(raw map[string]any) PublicCapabilityStateV2 {
	enabled := publicBool(raw["enabled"])
	available := publicBool(raw["available"])
	return PublicCapabilityStateV2{Status: publicCapabilityStatus(enabled, available), Enabled: enabled, Available: available, ReasonCode: publicCapabilityReasonCode(enabled, available)}
}

func publicMCPCatalog(raw map[string]any) PublicMCPCatalogV2 {
	status := publicEnum(publicString(raw["status"]), []string{"available", "unavailable", "disabled", "cached", "lazy"}, "unavailable")
	return PublicMCPCatalogV2{
		Status:              status,
		ReasonCode:          map[string]string{"available": "available", "disabled": "disabled_by_config", "cached": "schema_hint_only", "lazy": "not_live", "unavailable": "unavailable"}[status],
		ToolCount:           publicCount(raw["toolCount"]),
		AdvertisedToolCount: publicCount(raw["advertisedToolCount"]),
		PromptCount:         publicCount(raw["promptCount"]),
		ResourceCount:       publicCount(raw["resourceCount"]),
		CatalogFingerprint:  publicSHA256(publicString(raw["catalogFingerprint"])),
		CatalogDrift:        publicBool(raw["catalogDrift"]),
	}
}

func publicSubagentCapability(raw map[string]any) PublicSubagentCapabilityV2 {
	state := publicCapabilityState(raw)
	return PublicSubagentCapabilityV2{
		PublicCapabilityStateV2:         state,
		MaxParallel:                     publicCount(raw["maxParallel"]),
		MaxChildRuns:                    publicCount(raw["maxChildRuns"]),
		DefaultToolPolicy:               publicEnum(publicString(raw["defaultToolPolicy"]), []string{"readOnly", "inherit"}, "readOnly"),
		ProfileCount:                    len(publicList(raw["profiles"])),
		InternalLineageAvailable:        publicBool(raw["internalLineageAvailable"]),
		ProfilesAvailable:               publicBool(raw["profilesAvailable"]),
		DurableChildRunStore:            publicBool(raw["durableChildRunStore"]),
		ParallelExecutionAvailable:      publicBool(raw["parallelExecutionAvailable"]),
		TaskToolAvailable:               publicBool(raw["taskToolAvailable"]),
		ParallelTasksToolAvailable:      publicBool(raw["parallelTasksToolAvailable"]),
		BackgroundTaskJobsAvailable:     publicBool(raw["backgroundTaskJobsAvailable"]),
		BackgroundShellAvailable:        publicBool(raw["backgroundShellAvailable"]),
		BackgroundSubagentJobsAvailable: publicBool(raw["backgroundSubagentJobsAvailable"]),
		ModelJobToolsAvailable:          publicBool(raw["modelJobToolsAvailable"]),
		TaskJobThreadScopeSupported:     publicBool(raw["taskJobThreadScopeSupported"]),
	}
}

func publicAttachmentsCapability(raw map[string]any) PublicAttachmentsCapabilityV2 {
	return PublicAttachmentsCapabilityV2{
		PublicCapabilityStateV2:       publicCapabilityState(raw),
		MaxImageBytes:                 publicCount(raw["maxImageBytes"]),
		MaxImageDimension:             publicCount(raw["maxImageDimension"]),
		AllowedMimeTypes:              publicMIMETypes(raw["allowedMimeTypes"]),
		AllowedDocumentMimeTypes:      publicMIMETypes(raw["allowedDocumentMimeTypes"]),
		MaxDocumentBytes:              publicCount(raw["maxDocumentBytes"]),
		MaxDocumentTextChars:          publicCount(raw["maxDocumentTextChars"]),
		TextFallbackMaxBase64Bytes:    publicCount(raw["textFallbackMaxBase64Bytes"]),
		TextFallbackMaxImageDimension: publicCount(raw["textFallbackMaxImageDimension"]),
		TextFallbackPreferredMimeType: publicMIME(publicString(raw["textFallbackPreferredMimeType"])),
	}
}

func publicMemoryCapability(raw map[string]any) PublicMemoryCapabilityV2 {
	return PublicMemoryCapabilityV2{
		PublicCapabilityStateV2: publicCapabilityState(raw),
		Mode:                    publicEnum(publicString(raw["mode"]), []string{"manual"}, "manual"),
		StoreOnly:               publicBool(raw["storeOnly"]),
		ModelInjection:          publicBool(raw["modelInjection"]),
		AutomaticCapture:        publicBool(raw["automaticCapture"]),
		Scopes:                  publicEnumList(raw["scopes"], []string{"user", "workspace", "project"}, []string{}),
		MaxInjectedRecords:      publicCount(raw["maxInjectedRecords"]),
	}
}

func publicGeneratedCapability(raw map[string]any) PublicGeneratedCapabilityV2 {
	return PublicGeneratedCapabilityV2{PublicCapabilityStateV2: publicCapabilityState(raw), Model: publicIdentifier(publicString(raw["model"]), "")}
}

func publicComputerUseCapability(raw map[string]any) PublicComputerUseCapabilityV2 {
	return PublicComputerUseCapabilityV2{
		PublicCapabilityStateV2: publicCapabilityState(raw),
		Mode:                    publicEnum(publicString(raw["mode"]), []string{"auto", "always", "off"}, "auto"),
		BackendID:               publicIdentifier(publicString(raw["backendId"]), ""),
		PreferredBackendID:      publicIdentifier(publicString(raw["preferredBackendId"]), ""),
	}
}

func publicVisionBridgeCapability(raw map[string]any) PublicVisionBridgeCapabilityV2 {
	return PublicVisionBridgeCapabilityV2{
		PublicCapabilityStateV2: publicCapabilityState(raw),
		Mode:                    publicEnum(publicString(raw["mode"]), []string{"auto", "always", "off"}, "auto"),
		Model:                   publicIdentifier(publicString(raw["model"]), ""),
		ProviderID:              publicIdentifier(publicString(raw["providerId"]), ""),
		MaxScreenshotsPerTurn:   publicCount(raw["maxScreenshotsPerTurn"]),
		MaxImagesPerTurn:        publicCount(raw["maxImagesPerTurn"]),
		MaxImageBytes:           publicCount(raw["maxImageBytes"]),
		MaxImageDimension:       publicCount(raw["maxImageDimension"]),
		SemanticProbeStatus:     publicEnum(publicString(raw["semanticProbeStatus"]), []string{"unknown", "supported", "unsupported", "semantic_failed", "auth_failed", "http_failed", "timeout", "failed", "stale"}, "unknown"),
	}
}

func publicToolContractCatalog(items []any) PublicToolContractCatalogV2 {
	type tuple struct {
		Name         string `json:"name"`
		ToolKind     string `json:"toolKind"`
		ProviderKind string `json:"providerKind"`
		ToolPolicy   string `json:"toolPolicy"`
		InputHash    string `json:"inputHash"`
		OutputHash   string `json:"outputHash,omitempty"`
	}
	tuples := make([]tuple, 0, len(items))
	for _, item := range items {
		record := publicMap(item)
		name := publicIdentifier(publicString(record["name"]), "")
		if name == "" {
			continue
		}
		tuples = append(tuples, tuple{
			Name:         name,
			ToolKind:     publicEnum(publicString(record["toolKind"]), []string{"tool_call", "command_execution", "file_change", "subagent"}, "tool_call"),
			ProviderKind: publicEnum(publicString(record["providerKind"]), []string{"built-in", "mcp", "gui", "skill", "delegation", "web"}, "built-in"),
			ToolPolicy:   publicEnum(publicString(record["toolPolicy"]), []string{"auto", "on-request", "never"}, "on-request"),
			InputHash:    publicJSONHash(record["inputSchema"]),
			OutputHash:   publicJSONHash(record["outputSchema"]),
		})
	}
	sort.SliceStable(tuples, func(i, j int) bool { return tuples[i].Name < tuples[j].Name })
	return PublicToolContractCatalogV2{Count: len(tuples), CatalogHash: publicJSONHash(tuples)}
}

func publicCapabilityStatus(enabled bool, available bool) string {
	if available {
		return "available"
	}
	if !enabled {
		return "disabled"
	}
	return "unavailable"
}

func publicCapabilityReasonCode(enabled bool, available bool) string {
	if available {
		return "available"
	}
	if !enabled {
		return "disabled_by_config"
	}
	return "unavailable"
}

func publicPort(value int) int {
	if value < 0 || value > 65535 {
		return 0
	}
	return value
}

func publicMCPTransport(value string) string {
	switch strings.TrimSpace(value) {
	case "stdio":
		return "stdio"
	case "http", "streamable-http":
		return "streamable-http"
	case "sse":
		return "sse"
	default:
		return "unknown"
	}
}

func publicListenerScope(host string) string {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "127.0.0.1", "localhost", "::1":
		return "loopback"
	default:
		return "non_loopback"
	}
}

func publicTimestamp(value string) (string, bool) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Unix(0, 0).UTC().Format(time.RFC3339), false
	}
	return parsed.UTC().Format(time.RFC3339Nano), true
}

func publicIdentifier(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || publicIdentifierLooksPrivate(value) {
		return fallback
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._:/@+-", r) {
			continue
		}
		return fallback
	}
	return value
}

func publicIdentifierLooksPrivate(value string) bool {
	lower := strings.ToLower(value)
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~/") || strings.Contains(value, "\\") ||
		strings.Contains(lower, "://") || strings.Contains(value, "../") || strings.Contains(value, "/..") ||
		strings.Contains(value, "@") {
		return true
	}
	if len(value) >= 3 && value[1] == ':' && value[2] == '/' {
		return true
	}
	digitRun := 0
	for _, r := range value {
		if unicode.IsDigit(r) {
			digitRun++
			if digitRun >= 12 {
				return true
			}
			continue
		}
		digitRun = 0
	}
	return false
}

func publicEnum(value string, allowed []string, fallback string) string {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return candidate
		}
	}
	return fallback
}

func publicReasoningEffort(value string) string {
	projected, valid := domainmodel.ProjectReasoningEffortV1(value)
	if !valid {
		return ""
	}
	return projected
}

func publicEnumList(value any, allowed []string, fallback []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, item := range publicList(value) {
		candidate := publicEnum(publicString(item), allowed, "")
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

func publicMIMETypes(value any) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range publicList(value) {
		mime := publicMIME(publicString(item))
		if mime == "" {
			continue
		}
		if _, ok := seen[mime]; ok {
			continue
		}
		seen[mime] = struct{}{}
		out = append(out, mime)
	}
	sort.Strings(out)
	return out
}

func publicMIME(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || !strings.Contains(value, "/") {
		return ""
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-+./", r) {
			continue
		}
		return ""
	}
	return value
}

func publicCount(value any) int {
	number, ok := publicNumber(value)
	if !ok || number <= 0 {
		return 0
	}
	if number > 1_000_000_000 {
		return 1_000_000_000
	}
	return int(number)
}

func publicOptionalCount(value any) (*int, bool) {
	number, ok := publicNumber(value)
	if !ok || number < 0 || math.Trunc(number) != number {
		return nil, false
	}
	if number > 1_000_000_000 {
		number = 1_000_000_000
	}
	count := int(number)
	return &count, true
}

func publicBoundedFloat(value any, min float64, max float64) float64 {
	number, ok := publicNumber(value)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
		return min
	}
	if number < min {
		return min
	}
	if number > max {
		return max
	}
	return number
}

func publicNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case json.Number:
		number, err := strconv.ParseFloat(string(typed), 64)
		return number, err == nil
	default:
		return 0, false
	}
}

func publicMap(value any) map[string]any {
	record, _ := value.(map[string]any)
	return record
}

func publicList(value any) []any {
	items, _ := value.([]any)
	return items
}

func publicString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func publicBool(value any) bool {
	result, _ := value.(bool)
	return result
}

func publicSHA256(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != sha256.Size*2 {
		return ""
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return ""
	}
	return value
}

func publicJSONHash(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		encoded = []byte("null")
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func shortPublicHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:6])
}
