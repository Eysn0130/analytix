package model

import (
	"encoding/json"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

func ContinuationSourceScopeV1(thread map[string]any) (string, bool) {
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	if err != nil || caseSensitive {
		return "", false
	}
	// A compaction changes the context epoch. Source readability is bound to
	// the same thread/principal/workspace; execution still checks the live epoch.
	scope := map[string]any{"version": "continuation-source-scope.v1", "thread": providerHistoryStringField(thread, "id"), "workspace": providerHistoryStringField(thread, "workspace")}
	if raw := thread["securityState"]; raw != nil {
		current, err := domainsecurity.ParseTurnSecurityContext(raw)
		if err != nil || !domainsecurity.TurnSecurityContextIsGeneral(current) {
			return "", false
		}
		scope["workspace"], scope["tenant"], scope["user"], scope["binding"] = current.WorkspaceRealPath, current.TenantID, current.UserID, current.CaseBindingHash
	}
	if scope["thread"] == "" {
		return "", false
	}
	body, _ := json.Marshal(scope)
	return domainsecurity.SHA256Hex(body), true
}

// An explicit fork/resume exports historical data for the same admitted
// principal. Computing its child data scope does not create a child security
// context or reuse any parent execution witness.
func ContinuationDerivedSourceScopeV1(source map[string]any, threadID, workspace string) (string, bool) {
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(source)
	current, contextErr := domainsecurity.ParseTurnSecurityContext(source["securityState"])
	if err != nil || caseSensitive || contextErr != nil || !domainsecurity.TurnSecurityContextIsGeneral(current) ||
		domainsecurity.ValidateTurnSecurityContextForExecution(current) != nil || current.ThreadID != providerHistoryStringField(source, "id") ||
		current.WorkspaceRealPath != providerHistoryStringField(source, "workspace") || !domainthread.IsCanonicalRecordID(threadID) || workspace == "" {
		return "", false
	}
	scope := map[string]any{"version": "continuation-source-scope.v1", "thread": threadID, "workspace": workspace,
		"tenant": current.TenantID, "user": current.UserID, "binding": current.CaseBindingHash}
	body, _ := json.Marshal(scope)
	return domainsecurity.SHA256Hex(body), true
}

// Large sealed sources require the bounded reader even when a new user prompt
// would otherwise select the direct-answer route. This observes data needs;
// normal tool scope and current execution authority still decide access.
func ContinuationSourceReferencesRequiredV1(thread map[string]any) bool {
	scope, ok := ContinuationSourceScopeV1(thread)
	if !ok {
		return false
	}
	turns, _ := thread["turns"].([]any)
	for i := len(turns) - 1; i >= 0; i-- {
		turn, _ := turns[i].(map[string]any)
		items, _ := turn["items"].([]any)
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			version, _ := contracts.NumericSeq(item["schemaVersion"])
			if !domainevent.ValidGeneralCompactionProviderHistoryItemV4(item) {
				if version != 5 && item["summaryKind"] != "conversation" {
					continue
				} // Preserve legacy routing semantics.
				if !domainevent.ValidGeneralCompactionProviderHistoryItemV5(item) {
					return false
				}
			}
			if !GeneralCompactionSourceScopeCurrentV1(thread, item) || item["taskContinuation"] == nil {
				return false
			}
			snapshot, err := domainthread.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
			if err != nil || snapshot.UserHistory == nil || snapshot.UserHistory.ScopeDigest != scope {
				return false
			}
			size := 0
			for _, source := range snapshot.UserHistory.Sources {
				size += len(source.Text)
			}
			return size > domainthread.ContinuationInlineBudgetBytes
		}
	}
	return false
}
