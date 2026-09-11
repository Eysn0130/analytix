package security

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const ApprovalGrantTransitionVersionV1 = 1

// ApprovalGrantTransitionV1 is the public, hash-bound audit projection that
// links one pending grant to the only approved grant authorized by a private
// host-signed continuation disposition. Signatures remain in the private
// continuation store; public history retains only their immutable identities.
type ApprovalGrantTransitionV1 struct {
	Version                   int    `json:"version"`
	TransitionID              string `json:"transitionId"`
	ApprovalID                string `json:"approvalId"`
	ApprovalItemID            string `json:"approvalItemId"`
	ContinuationReceiptID     string `json:"continuationReceiptId"`
	ContinuationDispositionID string `json:"continuationDispositionId"`
	DispositionStatus         string `json:"dispositionStatus"`
	DispositionReasonCode     string `json:"dispositionReasonCode"`
	ThreadID                  string `json:"threadId"`
	TurnID                    string `json:"turnId"`
	ContextDigest             string `json:"contextDigest"`
	ContextEpoch              uint64 `json:"contextEpoch"`
	CaseBindingHash           string `json:"caseBindingHash"`
	DatasetSnapshotID         string `json:"datasetSnapshotId"`
	SourceManifestHash        string `json:"sourceManifestHash"`
	ParentGrantID             string `json:"parentGrantId"`
	ApprovedGrantID           string `json:"approvedGrantId"`
	ToolCallID                string `json:"toolCallId"`
	ToolName                  string `json:"toolName"`
	TransitionedAt            string `json:"transitionedAt"`
}

type ApprovalGrantTransitionInputV1 struct {
	Context                   TurnSecurityContext
	ApprovalID                string
	ApprovalItemID            string
	ContinuationReceiptID     string
	ContinuationDispositionID string
	PendingGrant              ExecutionGrant
	ApprovedGrant             ExecutionGrant
	TransitionedAt            time.Time
}

func NewApprovalGrantTransitionV1(input ApprovalGrantTransitionInputV1) (ApprovalGrantTransitionV1, error) {
	transitionedAt := input.TransitionedAt.UTC()
	transition := ApprovalGrantTransitionV1{
		Version:                   ApprovalGrantTransitionVersionV1,
		ApprovalID:                strings.TrimSpace(input.ApprovalID),
		ApprovalItemID:            strings.TrimSpace(input.ApprovalItemID),
		ContinuationReceiptID:     strings.TrimSpace(input.ContinuationReceiptID),
		ContinuationDispositionID: strings.TrimSpace(input.ContinuationDispositionID),
		DispositionStatus:         "allowed", DispositionReasonCode: "approval_allowed",
		ThreadID: strings.TrimSpace(input.Context.ThreadID), TurnID: strings.TrimSpace(input.Context.TurnID),
		ContextDigest: strings.TrimSpace(input.Context.ContextDigest), ContextEpoch: input.Context.ContextEpoch,
		CaseBindingHash: strings.TrimSpace(input.Context.CaseBindingHash), DatasetSnapshotID: strings.TrimSpace(input.Context.DatasetSnapshotID),
		SourceManifestHash: strings.TrimSpace(input.Context.SourceManifestHash),
		ParentGrantID:      strings.TrimSpace(input.PendingGrant.GrantID), ApprovedGrantID: strings.TrimSpace(input.ApprovedGrant.GrantID),
		ToolCallID: input.ApprovedGrant.ToolCallID, ToolName: strings.TrimSpace(input.ApprovedGrant.ToolName),
		TransitionedAt: transitionedAt.Format(time.RFC3339Nano),
	}
	transition.TransitionID = approvalGrantTransitionHashV1(transition)
	if err := ValidateApprovalGrantTransitionV1(transition, input.Context, input.PendingGrant, input.ApprovedGrant); err != nil {
		return ApprovalGrantTransitionV1{}, err
	}
	return transition, nil
}

func ValidateApprovalGrantTransitionV1(
	transition ApprovalGrantTransitionV1,
	context TurnSecurityContext,
	pending ExecutionGrant,
	approved ExecutionGrant,
) error {
	if ValidateTurnSecurityContextForExecution(context) != nil ||
		ValidateExecutionGrantForContext(pending, context) != nil || ValidateExecutionGrantForContext(approved, context) != nil ||
		!IsHostToolCallIDV1(pending.ToolCallID) || !IsHostToolCallIDV1(approved.ToolCallID) {
		return errors.New("approval grant transition identity is invalid")
	}
	return ValidateApprovalGrantTransitionForAuditV1(transition, context, pending, approved)
}

// This validates a frozen historical transition without execution authority.
func ValidateApprovalGrantTransitionForAuditV1(transition ApprovalGrantTransitionV1, context TurnSecurityContext, pending, approved ExecutionGrant) error {
	if transition.Version != ApprovalGrantTransitionVersionV1 ||
		!isSHA256Hex(transition.TransitionID) || transition.TransitionID != approvalGrantTransitionHashV1(transition) ||
		!strings.HasPrefix(transition.ApprovalID, "appr_") || len(transition.ApprovalID) > 192 ||
		transition.ApprovalItemID == "" || len(transition.ApprovalItemID) > 256 ||
		!isSHA256Hex(transition.ContinuationReceiptID) || !isSHA256Hex(transition.ContinuationDispositionID) ||
		transition.DispositionStatus != "allowed" || transition.DispositionReasonCode != "approval_allowed" ||
		ValidateTurnSecurityContext(context) != nil ||
		ValidateExecutionGrantForAudit(pending) != nil || ValidateExecutionGrantForAudit(approved) != nil {
		return errors.New("approval grant transition identity is invalid")
	}
	transitionedAt, transitionErr := time.Parse(time.RFC3339Nano, transition.TransitionedAt)
	approvedAt, approvedErr := time.Parse(time.RFC3339Nano, approved.IssuedAt)
	pendingExpiresAt, pendingExpiryErr := time.Parse(time.RFC3339Nano, pending.ExpiresAt)
	if transitionErr != nil || approvedErr != nil || pendingExpiryErr != nil || !transitionedAt.Before(pendingExpiresAt) ||
		transition.TransitionedAt != transitionedAt.UTC().Format(time.RFC3339Nano) ||
		approved.IssuedAt != approvedAt.UTC().Format(time.RFC3339Nano) || !transitionedAt.Equal(approvedAt) ||
		transition.ThreadID != context.ThreadID || transition.TurnID != context.TurnID ||
		transition.ContextDigest != context.ContextDigest || transition.ContextEpoch != context.ContextEpoch ||
		transition.CaseBindingHash != context.CaseBindingHash || transition.DatasetSnapshotID != context.DatasetSnapshotID ||
		transition.SourceManifestHash != context.SourceManifestHash ||
		transition.ParentGrantID != pending.GrantID || transition.ApprovedGrantID != approved.GrantID ||
		transition.ToolCallID != approved.ToolCallID || transition.ToolName != approved.ToolName ||
		pending.ApprovalState != "pending" || approved.ApprovalState != "approved" ||
		!sameExecutionGrantAuthority(pending, approved) {
		return errors.New("approval grant transition authority is invalid")
	}
	expected := NewExecutionGrant(ExecutionGrantInput{
		Context: context, Provider: pending.Provider, ServerIdentity: pending.ServerIdentity,
		ToolName: pending.ToolName, ToolCallID: pending.ToolCallID, ConnectionEpoch: pending.ConnectionEpoch,
		ArgsHash: pending.ArgsHash, SchemaHash: pending.SchemaHash, ScopeHash: pending.ScopeHash,
		ReadOnly: pending.ReadOnly, ApprovalState: "approved", IssuedAt: transitionedAt, ExpiresAt: pendingExpiresAt,
	})
	if approved != expected {
		return errors.New("approved grant is not deterministically derived from the transition")
	}
	return nil
}

func ParseApprovalGrantTransitionV1(value any) (ApprovalGrantTransitionV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return ApprovalGrantTransitionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var transition ApprovalGrantTransitionV1
	if err := decoder.Decode(&transition); err != nil {
		return ApprovalGrantTransitionV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ApprovalGrantTransitionV1{}, errors.New("approval grant transition contains trailing JSON")
	}
	return transition, nil
}

func approvalGrantTransitionHashV1(transition ApprovalGrantTransitionV1) string {
	transition.TransitionID = ""
	return hashContract(transition)
}
