//go:build !analytix_prod

package mcp

import contracts "analytix.local/runtime-go/internal/contracts"

type DiagnosticsRedaction struct {
	Secret      string `json:"secret"`
	Replacement string `json:"replacement"`
}

type ProviderCancel struct {
	ErrorSubstring string `json:"errorSubstring"`
	Executed       bool   `json:"executed"`
}

type G4ToolsConformanceContract struct {
	Mode              string                       `json:"mode"`
	SourceContractIDs []string                     `json:"sourceContractIds"`
	ProductBoundary   contracts.ProductBoundary    `json:"productBoundary"`
	ToolCatalog       G4ToolCatalogConformance     `json:"toolCatalog"`
	Approval          G4ApprovalConformance        `json:"approval"`
	UserInput         G4UserInputConformance       `json:"userInput"`
	MCP               G4MCPConformance             `json:"mcp"`
	PlannerExecutor   G4PlannerExecutorConformance `json:"plannerExecutor"`
	RemoteEntry       G4RemoteEntryBoundary        `json:"remoteEntryBoundary"`
	ExpectedOutput    map[string]any               `json:"expectedOutput"`
}

type G4ToolCatalogConformance struct {
	AdvertisedToolNames     []string `json:"advertisedToolNames"`
	CanonicalOrderStable    bool     `json:"canonicalOrderStable"`
	ForbiddenTopLevelRoutes []string `json:"forbiddenTopLevelRoutes"`
}

type G4ApprovalConformance struct {
	ID                       string `json:"id"`
	ToolName                 string `json:"toolName"`
	Decision                 string `json:"decision"`
	ExpectedStatus           string `json:"expectedStatus"`
	SecondDecisionStatus     int    `json:"secondDecisionStatus"`
	PendingBefore            int    `json:"pendingBefore"`
	PendingAfter             int    `json:"pendingAfter"`
	MustNotExecuteDeniedTool bool   `json:"mustNotExecuteDeniedTool"`
}

type G4UserInputConformance struct {
	ID                         string                                `json:"id"`
	Resolution                 string                                `json:"resolution"`
	ExpectedStatus             string                                `json:"expectedStatus"`
	SecondResolveStatus        int                                   `json:"secondResolveStatus"`
	PendingBefore              int                                   `json:"pendingBefore"`
	PendingAfter               int                                   `json:"pendingAfter"`
	RemoteDisableUserInput     bool                                  `json:"remoteDisableUserInputPreserved"`
	StructuredChoiceValidation G4UserInputStructuredChoiceValidation `json:"structuredChoiceValidation"`
	SubmittedRoute             G4UserInputSubmittedRoute             `json:"submittedRoute"`
}

type G4UserInputStructuredChoiceValidation struct {
	InvalidResultCode  string   `json:"invalidResultCode"`
	OpensGateOnInvalid bool     `json:"opensGateOnInvalid"`
	InvalidCases       []string `json:"invalidCases"`
}

type G4UserInputSubmittedRoute struct {
	Status                       string `json:"status"`
	AnswersEchoed                bool   `json:"answersEchoed"`
	ResolvedEventIncludesAnswers bool   `json:"resolvedEventIncludesAnswers"`
}

type G4MCPConformance struct {
	ProviderID              string                       `json:"providerId"`
	ConnectToolNames        []string                     `json:"connectToolNames"`
	ReloadToolNames         []string                     `json:"reloadToolNames"`
	DisconnectReason        string                       `json:"disconnectReason"`
	Cancel                  ProviderCancel               `json:"cancel"`
	DiagnosticsRedaction    DiagnosticsRedaction         `json:"diagnosticsRedaction"`
	KnownOverrideDiagnostic G4MCPKnownOverrideDiagnostic `json:"knownOverrideDiagnostic"`
	ApprovalAnnotations     G4MCPApprovalAnnotations     `json:"approvalAnnotations"`
	SearchMetaTools         G4MCPSearchMetaTools         `json:"searchMetaTools"`
}

type G4MCPKnownOverrideDiagnostic struct {
	KnownOverride   string `json:"knownOverride"`
	EffectiveCWD    string `json:"effectiveCwd"`
	LowPriority     bool   `json:"lowPriority"`
	BackgroundStart bool   `json:"backgroundStart"`
}

type G4MCPApprovalAnnotations struct {
	NormalizedToolName string `json:"normalizedToolName"`
	Decision           string `json:"decision"`
	Executed           bool   `json:"executed"`
}

type G4MCPSearchMetaTools struct {
	ToolNames              []string `json:"toolNames"`
	TrustedToolID          string   `json:"trustedToolId"`
	UntrustedSearchedTools int      `json:"untrustedSearchedTools"`
	UnknownToolError       string   `json:"unknownToolError"`
	CallPolicy             string   `json:"callPolicy"`
	DeniedCallExecuted     bool     `json:"deniedCallExecuted"`
}

type G4PlannerExecutorConformance struct {
	PlannerReadOnlyToolset          []string `json:"plannerReadOnlyToolset"`
	BackgroundJobRoutesInternalOnly bool     `json:"backgroundJobRoutesInternalOnly"`
	TopLevelWorkflowRoutesExposed   bool     `json:"topLevelWorkflowRoutesExposed"`
}

type G4RemoteEntryBoundary struct {
	ExpectedPortKeys  []string `json:"expectedPortKeys"`
	ForbiddenPortKeys []string `json:"forbiddenPortKeys"`
}

func BuildG4ToolsConformanceOutput(contract G4ToolsConformanceContract) map[string]any {
	return map[string]any{
		"stage":                              "G4",
		"mode":                               contract.Mode,
		"sourceContractIds":                  contract.SourceContractIDs,
		"toolNames":                          contract.ToolCatalog.AdvertisedToolNames,
		"approvalDeniedNoExecute":            true,
		"userInputCancelled":                 true,
		"userInputValidationCode":            contract.UserInput.StructuredChoiceValidation.InvalidResultCode,
		"userInputValidationCases":           contract.UserInput.StructuredChoiceValidation.InvalidCases,
		"userInputInvalidOpensGate":          contract.UserInput.StructuredChoiceValidation.OpensGateOnInvalid,
		"userInputSubmittedAnswersEchoed":    contract.UserInput.SubmittedRoute.Status == "submitted" && contract.UserInput.SubmittedRoute.AnswersEchoed,
		"userInputResolvedEventOmitsAnswers": contract.UserInput.SubmittedRoute.Status == "submitted" && !contract.UserInput.SubmittedRoute.ResolvedEventIncludesAnswers,
		"mcpApprovalAnnotatedNoExecute":      contract.MCP.ApprovalAnnotations.Decision == "deny" && !contract.MCP.ApprovalAnnotations.Executed,
		"mcpSearchMetaToolsAdvertised":       len(contract.MCP.SearchMetaTools.ToolNames) == 4,
		"mcpSearchUntrustedWorkspaceHidden":  contract.MCP.SearchMetaTools.UntrustedSearchedTools == 0,
		"mcpSearchCallDeniedNoExecute":       contract.MCP.SearchMetaTools.CallPolicy == "on-request" && !contract.MCP.SearchMetaTools.DeniedCallExecuted,
		"mcpKnownOverride":                   contract.MCP.KnownOverrideDiagnostic.KnownOverride,
		"plannerReadOnlyToolset":             contract.PlannerExecutor.PlannerReadOnlyToolset,
		"productBoundary":                    contract.ProductBoundary,
	}
}
