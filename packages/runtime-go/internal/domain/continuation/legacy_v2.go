package continuation

import (
	"encoding/json"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// payloadV2 is the immutable wire shape that was signed before provider
// namespace and recovery authority became mandatory. It must never acquire
// new fields: V2 records remain audit/close material but cannot resume work.
type payloadV2 struct {
	Version                int                                   `json:"version"`
	Kind                   string                                `json:"kind"`
	GateID                 string                                `json:"gateId"`
	ThreadID               string                                `json:"threadId"`
	TurnID                 string                                `json:"turnId"`
	ItemID                 string                                `json:"itemId"`
	CallID                 string                                `json:"callId"`
	ToolName               string                                `json:"toolName"`
	ToolCallItemID         string                                `json:"toolCallItemId"`
	Arguments              json.RawMessage                       `json:"arguments"`
	ProviderID             string                                `json:"providerId"`
	Model                  string                                `json:"model"`
	ProviderRouteHash      string                                `json:"providerRouteHash"`
	WorkspaceCheckpointID  string                                `json:"workspaceCheckpointId,omitempty"`
	ApprovalPolicy         string                                `json:"approvalPolicy"`
	SandboxMode            string                                `json:"sandboxMode"`
	SubagentDepth          int                                   `json:"subagentDepth"`
	ReasoningEffort        string                                `json:"reasoningEffort,omitempty"`
	Mode                   string                                `json:"mode,omitempty"`
	GUIPlan                json.RawMessage                       `json:"guiPlan,omitempty"`
	DisableUserInput       bool                                  `json:"disableUserInput"`
	MaxModelSteps          *int                                  `json:"maxModelSteps,omitempty"`
	EffectiveMaxModelSteps *int                                  `json:"effectiveMaxModelSteps,omitempty"`
	ToolScope              []string                              `json:"toolScope"`
	PriorSettledToolRefs   []domainsecurity.SettledToolReference `json:"priorSettledToolRefs,omitempty"`
	SecurityContext        domainsecurity.TurnSecurityContext    `json:"turnSecurityContext"`
	ExecutionGrant         domainsecurity.ExecutionGrant         `json:"executionGrant"`
	IssuedAt               string                                `json:"issuedAt"`
}

type receiptV2 struct {
	Version            int       `json:"version"`
	ReceiptID          string    `json:"receiptId"`
	Payload            payloadV2 `json:"payload"`
	AuthorityAlgorithm string    `json:"authorityAlgorithm"`
	AuthorityKeyID     string    `json:"authorityKeyId"`
	AuthorityPublicKey string    `json:"authorityPublicKey"`
	AuthoritySignature string    `json:"authoritySignature"`
}

func legacyPayloadV2(payload Payload) payloadV2 {
	return payloadV2{
		Version: payload.Version, Kind: payload.Kind, GateID: payload.GateID, ThreadID: payload.ThreadID, TurnID: payload.TurnID,
		ItemID: payload.ItemID, CallID: payload.CallID, ToolName: payload.ToolName, ToolCallItemID: payload.ToolCallItemID,
		Arguments: payload.Arguments, ProviderID: payload.ProviderID, Model: payload.Model, ProviderRouteHash: payload.ProviderRouteHash,
		WorkspaceCheckpointID: payload.WorkspaceCheckpointID, ApprovalPolicy: payload.ApprovalPolicy, SandboxMode: payload.SandboxMode,
		SubagentDepth: payload.SubagentDepth, ReasoningEffort: payload.ReasoningEffort, Mode: payload.Mode, GUIPlan: payload.GUIPlan,
		DisableUserInput: payload.DisableUserInput, MaxModelSteps: payload.MaxModelSteps, EffectiveMaxModelSteps: payload.EffectiveMaxModelSteps,
		ToolScope: payload.ToolScope, PriorSettledToolRefs: payload.PriorSettledToolRefs, SecurityContext: payload.SecurityContext,
		ExecutionGrant: payload.ExecutionGrant, IssuedAt: payload.IssuedAt,
	}
}

func legacyReceiptV2(receipt Receipt) receiptV2 {
	return receiptV2{
		Version: receipt.Version, ReceiptID: receipt.ReceiptID, Payload: legacyPayloadV2(receipt.Payload),
		AuthorityAlgorithm: receipt.AuthorityAlgorithm, AuthorityKeyID: receipt.AuthorityKeyID,
		AuthorityPublicKey: receipt.AuthorityPublicKey, AuthoritySignature: receipt.AuthoritySignature,
	}
}
