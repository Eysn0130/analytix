//go:build !analytix_prod

package agent

type GoMinimalAgentLoopContract struct {
	ID                     string                               `json:"id"`
	RuntimeToken           string                               `json:"runtimeToken"`
	SourceContractIDs      []string                             `json:"sourceContractIds"`
	ProductBoundary        map[string]any                       `json:"productBoundary"`
	ThreadID               string                               `json:"threadId"`
	TurnID                 string                               `json:"turnId"`
	CancelThreadID         string                               `json:"cancelThreadId"`
	ResumeThreadID         string                               `json:"resumeThreadId"`
	UserInput              map[string]any                       `json:"userInput"`
	StablePrefix           map[string]any                       `json:"stablePrefix"`
	ModelRequestShape      map[string]any                       `json:"modelRequestShape"`
	ProviderCacheTelemetry map[string]any                       `json:"providerCacheTelemetry"`
	ApprovalDenied         map[string]any                       `json:"approvalDenied"`
	UserInputGates         map[string]any                       `json:"userInputGates"`
	MCPToolCatalog         map[string]any                       `json:"mcpToolCatalog"`
	LoopScript             map[string]any                       `json:"loopScript"`
	EventDrafts            []map[string]any                     `json:"eventDrafts"`
	Control                GoMinimalAgentLoopControl            `json:"control"`
	Expected               GoMinimalAgentLoopExpectedConformity `json:"expected"`
}

type GoMinimalAgentLoopControl struct {
	StepLimit map[string]any                    `json:"stepLimit"`
	Cancel    GoMinimalAgentLoopControlEventSet `json:"cancel"`
	Resume    GoMinimalAgentLoopControlEventSet `json:"resume"`
}

type GoMinimalAgentLoopControlEventSet struct {
	EventDrafts []map[string]any `json:"eventDrafts"`
	Status      string           `json:"status,omitempty"`
	ResultCode  string           `json:"resultCode,omitempty"`
}

type GoMinimalAgentLoopExpectedConformity struct {
	EventKinds               []string       `json:"eventKinds"`
	ItemKinds                []string       `json:"itemKinds"`
	HighestSeq               int            `json:"highestSeq"`
	ReplayAfterSeq           int            `json:"replayAfterSeq"`
	ReplayAfterSeqCount      int            `json:"replayAfterSeqCount"`
	SSEFrameCount            int            `json:"sseFrameCount"`
	CaughtUpReplayFrameCount int            `json:"caughtUpReplayFrameCount"`
	Providers                []string       `json:"providers"`
	TotalCacheHitTokens      int            `json:"totalCacheHitTokens"`
	TotalCacheMissTokens     int            `json:"totalCacheMissTokens"`
	ApprovalStatuses         []string       `json:"approvalStatuses"`
	UserInputStatuses        []string       `json:"userInputStatuses"`
	MCPToolNames             []string       `json:"mcpToolNames"`
	LatestMCPFingerprint     string         `json:"latestMcpFingerprint"`
	RecoveredStateMatches    bool           `json:"recoveredStateMatches"`
	SideEffects              map[string]int `json:"sideEffects"`
}
