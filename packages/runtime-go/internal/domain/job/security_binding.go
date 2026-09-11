package job

import (
	"encoding/json"
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const SecurityBindingVersion = 2

type SecurityBinding struct {
	Version                 int    `json:"version"`
	ParentContextVersion    int    `json:"parentContextVersion"`
	ParentThreadID          string `json:"parentThreadId"`
	ParentTurnID            string `json:"parentTurnId"`
	ParentContextDigest     string `json:"parentContextDigest"`
	ParentContextEpoch      uint64 `json:"parentContextEpoch"`
	ParentCaseID            string `json:"parentCaseId"`
	ParentCaseBindingHash   string `json:"parentCaseBindingHash"`
	ParentDatasetSnapshot   string `json:"parentDatasetSnapshotId"`
	ParentSourceManifest    string `json:"parentSourceManifestHash"`
	ParentWorkspaceRealPath string `json:"parentWorkspaceRealPath"`
	TenantID                string `json:"tenantId"`
	UserID                  string `json:"userId"`
	ParentExecutionGrantID  string `json:"parentExecutionGrantId"`
	ParentToolCallID        string `json:"parentToolCallId"`
	OutputTrustStatus       string `json:"outputTrustStatus"`
	BindingDigest           string `json:"bindingDigest"`
}

func NewSecurityBinding(context domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, toolCallID string) (*SecurityBinding, error) {
	toolCallID = strings.TrimSpace(toolCallID)
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil ||
		domainsecurity.ValidateExecutionGrantForOrdinaryContext(grant, context) != nil ||
		grant.ContextDigest != context.ContextDigest || grant.TurnID != context.TurnID || grant.ToolCallID != toolCallID || toolCallID == "" {
		return nil, errors.New("job security authority is invalid")
	}
	binding := &SecurityBinding{
		Version: SecurityBindingVersion, ParentContextVersion: context.Version, ParentThreadID: context.ThreadID, ParentTurnID: context.TurnID,
		ParentContextDigest: context.ContextDigest, ParentContextEpoch: context.ContextEpoch,
		ParentCaseID: context.CaseID, ParentCaseBindingHash: context.CaseBindingHash, ParentDatasetSnapshot: context.DatasetSnapshotID,
		ParentSourceManifest: context.SourceManifestHash, ParentWorkspaceRealPath: context.WorkspaceRealPath,
		TenantID: context.TenantID, UserID: context.UserID, ParentExecutionGrantID: grant.GrantID, ParentToolCallID: toolCallID,
		OutputTrustStatus: "untrusted_child_output",
	}
	binding.BindingDigest = securityBindingDigest(*binding)
	if err := ValidateSecurityBinding(binding); err != nil {
		return nil, err
	}
	return binding, nil
}

func ValidateSecurityBinding(binding *SecurityBinding) error {
	if binding == nil || binding.Version != SecurityBindingVersion || binding.ParentContextVersion != domainsecurity.TurnSecurityContextVersionV2 ||
		strings.TrimSpace(binding.ParentThreadID) == "" || strings.TrimSpace(binding.ParentTurnID) == "" ||
		!domainsecurity.IsSHA256Hex(binding.ParentContextDigest) ||
		binding.ParentContextEpoch == 0 || strings.TrimSpace(binding.ParentCaseID) == "" || !domainsecurity.IsSHA256Hex(binding.ParentCaseBindingHash) ||
		strings.TrimSpace(binding.ParentDatasetSnapshot) == "" || !domainsecurity.IsSHA256Hex(binding.ParentSourceManifest) ||
		strings.TrimSpace(binding.ParentWorkspaceRealPath) == "" || strings.TrimSpace(binding.TenantID) == "" || strings.TrimSpace(binding.UserID) == "" ||
		!domainsecurity.IsSHA256Hex(binding.ParentExecutionGrantID) || strings.TrimSpace(binding.ParentToolCallID) == "" ||
		binding.OutputTrustStatus != "untrusted_child_output" || !domainsecurity.IsSHA256Hex(binding.BindingDigest) {
		return errors.New("job security binding is incomplete")
	}
	if securityBindingDigest(*binding) != binding.BindingDigest {
		return errors.New("job security binding integrity is invalid")
	}
	return nil
}

func SecurityBindingMatchesContext(binding *SecurityBinding, context domainsecurity.TurnSecurityContext) bool {
	if ValidateSecurityBinding(binding) != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
		return false
	}
	return binding.ParentThreadID == context.ThreadID && binding.ParentTurnID == context.TurnID &&
		binding.ParentContextDigest == context.ContextDigest && binding.ParentContextEpoch == context.ContextEpoch &&
		binding.ParentCaseID == context.CaseID && binding.ParentCaseBindingHash == context.CaseBindingHash &&
		binding.ParentDatasetSnapshot == context.DatasetSnapshotID && binding.ParentSourceManifest == context.SourceManifestHash &&
		binding.ParentWorkspaceRealPath == context.WorkspaceRealPath && binding.TenantID == context.TenantID && binding.UserID == context.UserID
}

func SecurityBindingMatchesCaseEpoch(binding *SecurityBinding, context domainsecurity.TurnSecurityContext) bool {
	if ValidateSecurityBinding(binding) != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
		return false
	}
	return binding.ParentThreadID == context.ThreadID && securityBindingMatchesCaseEpochScope(binding, context)
}

// SecurityBindingSharesThread identifies the exact principal-owned thread
// namespace before callers compare its mutable case/epoch authority. Thread
// ids alone are not a tenant boundary.
func SecurityBindingSharesThread(binding *SecurityBinding, context domainsecurity.TurnSecurityContext) bool {
	if ValidateSecurityBinding(binding) != nil || domainsecurity.ValidateTurnSecurityContext(context) != nil ||
		context.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return false
	}
	return binding.ParentThreadID == context.ThreadID && binding.TenantID == context.TenantID && binding.UserID == context.UserID
}

// SecurityBindingSharesWorkspace identifies the exact principal-owned
// workspace namespace before callers compare its mutable case authority.
func SecurityBindingSharesWorkspace(binding *SecurityBinding, context domainsecurity.TurnSecurityContext) bool {
	if ValidateSecurityBinding(binding) != nil || domainsecurity.ValidateTurnSecurityContext(context) != nil ||
		context.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return false
	}
	return binding.ParentWorkspaceRealPath == context.WorkspaceRealPath && binding.TenantID == context.TenantID && binding.UserID == context.UserID
}

// SecurityBindingMatchesCaseEpochScope permits an explicitly validated
// descendant thread to continue or fork an ancestor child transcript without
// carrying the ancestor's evidence authority into the new turn. Callers must
// prove the thread ancestry separately before using this relaxed comparison.
func SecurityBindingMatchesCaseEpochScope(binding *SecurityBinding, context domainsecurity.TurnSecurityContext) bool {
	if ValidateSecurityBinding(binding) != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
		return false
	}
	return securityBindingMatchesCaseEpochScope(binding, context)
}

// SecurityBindingMatchesWorkspaceScope compares the parts of a frozen job
// binding that are authoritative for every thread sharing one workspace. A
// context epoch is thread-local, so it is deliberately not compared here;
// case binding, dataset, source manifest, tenant, and user changes are global
// invalidations for work that can still mutate the shared workspace.
func SecurityBindingMatchesWorkspaceScope(binding *SecurityBinding, context domainsecurity.TurnSecurityContext) bool {
	if ValidateSecurityBinding(binding) != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
		return false
	}
	return binding.ParentCaseID == context.CaseID && binding.ParentCaseBindingHash == context.CaseBindingHash &&
		binding.ParentDatasetSnapshot == context.DatasetSnapshotID && binding.ParentSourceManifest == context.SourceManifestHash &&
		binding.ParentWorkspaceRealPath == context.WorkspaceRealPath && binding.TenantID == context.TenantID && binding.UserID == context.UserID
}

func securityBindingMatchesCaseEpochScope(binding *SecurityBinding, context domainsecurity.TurnSecurityContext) bool {
	return binding.ParentContextEpoch == context.ContextEpoch &&
		binding.ParentCaseID == context.CaseID && binding.ParentCaseBindingHash == context.CaseBindingHash &&
		binding.ParentDatasetSnapshot == context.DatasetSnapshotID && binding.ParentSourceManifest == context.SourceManifestHash &&
		binding.ParentWorkspaceRealPath == context.WorkspaceRealPath && binding.TenantID == context.TenantID && binding.UserID == context.UserID
}

func CloneSecurityBinding(binding *SecurityBinding) *SecurityBinding {
	if binding == nil {
		return nil
	}
	clone := *binding
	return &clone
}

func securityBindingDigest(binding SecurityBinding) string {
	binding.BindingDigest = ""
	body, _ := json.Marshal(binding)
	return domainsecurity.SHA256Hex(body)
}
