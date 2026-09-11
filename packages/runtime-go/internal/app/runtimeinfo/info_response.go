package runtimeinfo

import domainmodel "analytix.local/runtime-go/internal/domain/model"

type RuntimeInfoInput struct {
	StartedAt       string
	Host            string
	Port            int
	DataDir         string
	Insecure        bool
	ApprovalPolicy  string
	SandboxMode     string
	NetworkProxy    map[string]any
	DefaultProvider domainmodel.TurnConfig
	Capabilities    map[string]any
}

func RuntimeInfo(input RuntimeInfoInput) PublicRuntimeInfoV2 {
	return ProjectPublicRuntimeInfo(input)
}

type RuntimeToolsDiagnosticsInput struct {
	Providers     []any
	ToolContracts []any
	MCPServers    []any
	MCPSearch     map[string]any
	MCPPrompts    []any
	MCPResources  []any
	Commands      []any
	NetworkProxy  map[string]any
	WebProviders  []any
	Skills        map[string]any
	Attachments   map[string]any
	Memory        map[string]any
	Subagents     map[string]any
}

func RuntimeToolsDiagnostics(input RuntimeToolsDiagnosticsInput) PublicRuntimeToolsV2 {
	return ProjectPublicRuntimeTools(input)
}
