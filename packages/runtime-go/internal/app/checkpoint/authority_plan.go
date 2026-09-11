package checkpoint

import (
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type AuthorityMetadataInput struct {
	ThreadID           string
	CheckpointID       string
	AuthorityWorkspace string
	WorkspaceFallback  string
	CreatedAtFallback  string
	Records            []map[string]any
}

func CapturedMetadataFromAuthorityRecords(input AuthorityMetadataInput) (map[string]any, bool) {
	if len(input.Records) == 0 {
		return nil, false
	}
	threadID := strings.TrimSpace(input.ThreadID)
	checkpointID := strings.TrimSpace(input.CheckpointID)
	authorityWorkspace := strings.TrimSpace(input.AuthorityWorkspace)
	turnID := exactString(input.Records[0], "turnId")
	sourceWorkspaceCheckpointID := exactString(input.Records[0], "sourceWorkspaceCheckpointId")
	contextDigest := exactString(input.Records[0], "contextDigest")
	if threadID == "" || checkpointID == "" || authorityWorkspace == "" || turnID == "" ||
		sourceWorkspaceCheckpointID == "" || contextDigest == "" {
		return nil, false
	}
	for _, record := range input.Records {
		if exactString(record, "threadId") != threadID || exactString(record, "checkpointId") != checkpointID ||
			exactString(record, "turnId") != turnID || exactString(record, "contextDigest") != contextDigest ||
			exactString(record, "workspace") != authorityWorkspace {
			return nil, false
		}
	}
	return BuildCapturedCheckpointMetadata(CapturedCheckpointInput{
		ThreadID: threadID, TurnID: turnID, CheckpointID: checkpointID,
		SourceWorkspaceCheckpointID: sourceWorkspaceCheckpointID,
		WorkspaceFallback:           input.WorkspaceFallback, CreatedAtFallback: input.CreatedAtFallback,
		Records: input.Records,
	})
}

func ApplyPlanHandleMismatch(client, authoritative map[string]any) string {
	if client == nil || authoritative == nil {
		return "checkpoint rewind apply requires a current host-issued plan"
	}
	clientVersion, clientVersionOK := nonNegativeInteger(client["schemaVersion"])
	authoritativeVersion, authoritativeVersionOK := nonNegativeInteger(authoritative["schemaVersion"])
	if !clientVersionOK || !authoritativeVersionOK || clientVersion != 1 || authoritativeVersion != 1 {
		return "checkpoint rewind plan schema is invalid"
	}
	clientDigest := exactString(client, "planDigest")
	authoritativeDigest := exactString(authoritative, "planDigest")
	if len(clientDigest) != 64 || clientDigest != authoritativeDigest ||
		PlanDigest(client) != clientDigest || PlanDigest(authoritative) != authoritativeDigest {
		return "checkpoint rewind plan is stale or does not match the current host plan"
	}
	for _, key := range []string{"planId", "checkpointId", "threadId", "scope", "applyMode"} {
		if clientValue, authoritativeValue := exactString(client, key), exactString(authoritative, key); clientValue == "" || authoritativeValue == "" || clientValue != authoritativeValue {
			return "checkpoint rewind plan handle does not match the current host plan"
		}
	}
	clientWorkspace, authoritativeWorkspace := exactString(client, "workspace"), exactString(authoritative, "workspace")
	if clientWorkspace == "" || authoritativeWorkspace == "" || clientWorkspace != authoritativeWorkspace {
		return "checkpoint rewind plan workspace does not match the current host plan"
	}
	clientDestructive, clientDestructiveOK := client["destructive"].(bool)
	authoritativeDestructive, authoritativeDestructiveOK := authoritative["destructive"].(bool)
	if !clientDestructiveOK || !authoritativeDestructiveOK || clientDestructive || authoritativeDestructive {
		return "checkpoint rewind plan destructive state is invalid"
	}
	return ""
}

func ApplyScope(plan map[string]any) string {
	return exactString(plan, "scope")
}

func ValidScope(scope string) bool {
	switch scope {
	case "code", "conversation", "combined":
		return true
	default:
		return false
	}
}

// RewindContextMismatch permits an explicit rewind from a later turn only
// while the immutable execution boundary is unchanged. Turn id, issued time,
// risk-witness nonce, and context digest are deliberately allowed to advance;
// case/workspace/epoch/dataset and publication policy are not.
func RewindContextMismatch(frozen, current domainsecurity.TurnSecurityContext) string {
	if domainsecurity.ValidateTurnSecurityContextForExecution(frozen) != nil ||
		domainsecurity.ValidateTurnSecurityContextForExecution(current) != nil {
		return "checkpoint rewind requires executable frozen and current security contexts"
	}
	if frozen.ThreadID != current.ThreadID {
		return "checkpoint rewind security thread changed"
	}
	if frozen.WorkspaceRealPath != current.WorkspaceRealPath || frozen.TenantID != current.TenantID || frozen.UserID != current.UserID {
		return "checkpoint rewind workspace or principal changed"
	}
	if frozen.ContextEpoch != current.ContextEpoch || frozen.CaseID != current.CaseID ||
		frozen.CaseBindingHash != current.CaseBindingHash {
		return "checkpoint rewind case binding or context epoch changed"
	}
	if frozen.DatasetSnapshotID != current.DatasetSnapshotID || frozen.SourceManifestHash != current.SourceManifestHash {
		return "checkpoint rewind dataset snapshot changed"
	}
	if frozen.PublicationPolicy.ThreadRiskPolicyDigest != current.PublicationPolicy.ThreadRiskPolicyDigest ||
		frozen.PublicationPolicy.RiskClass != current.PublicationPolicy.RiskClass ||
		frozen.PublicationPolicy.Disposition != current.PublicationPolicy.Disposition ||
		frozen.PublicationPolicy.CaseBindingState != current.PublicationPolicy.CaseBindingState ||
		frozen.PublicationPolicy.BlockerCode != current.PublicationPolicy.BlockerCode {
		return "checkpoint rewind publication policy changed"
	}
	return ""
}

func BlockedApplyResults(preflight []ApplyFilePreflight, reason string) []map[string]any {
	results := make([]map[string]any, 0, len(preflight))
	for _, file := range preflight {
		result := ApplyFilePreflightResult(file)
		if file.Status == "apply" {
			result["status"] = "blocked"
			result["reason"] = reason
		}
		results = append(results, result)
	}
	return results
}
