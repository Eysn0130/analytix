package subagent

import "analytix.local/runtime-go/internal/domain/model"

type Profile struct {
	Name           string         `json:"name"`
	ModelRef       model.ModelRef `json:"modelRef"`
	EndpointFormat string         `json:"endpointFormat,omitempty"`
	Effort         string         `json:"effort,omitempty"`
	ToolPolicy     string         `json:"toolPolicy,omitempty"`
	ToolScope      []string       `json:"toolScope,omitempty"`
}

type RunIdentity struct {
	ParentThreadID string         `json:"parentThreadId"`
	ParentTurnID   string         `json:"parentTurnId,omitempty"`
	ChildThreadID  string         `json:"childThreadId,omitempty"`
	ChildTurnID    string         `json:"childTurnId,omitempty"`
	ModelRef       model.ModelRef `json:"modelRef"`
	ProfileName    string         `json:"profileName,omitempty"`
	ToolSchemaHash string         `json:"toolSchemaHash,omitempty"`
}

type SourceGuard struct {
	ContinueFrom string `json:"continueFrom,omitempty"`
	ForkFrom     string `json:"forkFrom,omitempty"`
	LineageKey   string `json:"lineageKey,omitempty"`
}
