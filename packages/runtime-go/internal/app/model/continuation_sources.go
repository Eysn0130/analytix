package model

import (
	"encoding/json"

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
			if !domainevent.ValidGeneralCompactionProviderHistoryItemV4(item) {
				continue
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
