package tool

import "encoding/json"

type ApprovalClass string

const (
	ApprovalRead    ApprovalClass = "read"
	ApprovalWrite   ApprovalClass = "write"
	ApprovalExecute ApprovalClass = "execute"
)

type Spec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Source      string          `json:"source,omitempty"`
	ReadOnly    bool            `json:"readOnly,omitempty"`
}

type Result struct {
	ToolCallID string `json:"toolCallId"`
	Name       string `json:"name"`
	Content    string `json:"content,omitempty"`
	IsError    bool   `json:"isError,omitempty"`
}

type Settlement struct {
	ToolCallID string `json:"toolCallId"`
	Status     string `json:"status"`
	Result     Result `json:"result"`
}
