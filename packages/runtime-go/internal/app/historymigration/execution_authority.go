package historymigration

import (
	"errors"
	"strings"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var executionAuthorityKeys = []string{
	"contextDigest", "contextEpoch", "executionGrantId", "executionGrant", "parentGrantId", "hostEvidenceSettlement",
	"approvalTransition", "approvalTransitionId", "approvalItemId", "continuationDispositionId",
}

var continuationAuthorityKeys = []string{
	"continuationReceiptId", "approvalId", "inputId",
}

// StripUntrustedExecutionAuthority removes authority that cannot be proven to
// belong to the durable thread/turn that contains it. It never rewrites,
// re-signs, or rebinds an old context or grant to a derived thread.
func StripUntrustedExecutionAuthority(thread map[string]any) (map[string]any, bool, error) {
	threadID := strings.TrimSpace(stringField(thread, "id"))
	if threadID == "" {
		return nil, false, errors.New("execution-authority migration thread id is missing")
	}
	projected := contracts.CloneMap(thread)
	derived := derivedThread(projected, threadID)
	changed := false
	turns, _ := projected["turns"].([]any)
	for index, raw := range turns {
		turn, _ := raw.(map[string]any)
		if turn == nil {
			continue
		}
		requiresExecution := turnContainsExecutionAuthority(turn)
		requiresDerivedProof := derived && turnContainsAnyFrozenAuthority(turn)
		if !requiresExecution && !requiresDerivedProof {
			continue
		}
		if turnAuthorityIsCurrent(threadID, projected, turn, requiresExecution) {
			continue
		}
		// A typed V2 context already bound to this exact thread/turn is current
		// host authority, not legacy scalar residue. Corruption in its registry or
		// lifecycle must fail closed; stripping it would make a damaged current
		// record look eligible for the legacy migration path.
		if explicitOwnedTurnSecurityContextV2(turn, threadID) {
			return nil, false, errors.New("current execution authority is invalid")
		}
		turns[index] = stripTurnExecutionAuthority(turn, threadID, stableMigrationTime(projected, turn))
		changed = true
	}
	projected["turns"] = turns
	if derived && !rootContextBelongsToThread(projected, threadID) {
		for _, key := range []string{"securityState", "contextEpochState", "pendingApprovalIds", "pendingUserInputIds"} {
			if _, ok := projected[key]; ok {
				delete(projected, key)
				changed = true
			}
		}
	}
	return projected, changed, nil
}

func explicitOwnedTurnSecurityContextV2(turn map[string]any, threadID string) bool {
	value, present := turn["securityContext"]
	if !present {
		return false
	}
	context, err := domainsecurity.ParseTurnSecurityContext(value)
	if err == nil {
		return context.Version == domainsecurity.TurnSecurityContextVersionV2 &&
			context.ThreadID == strings.TrimSpace(threadID) &&
			context.TurnID == strings.TrimSpace(stringField(turn, "id"))
	}
	boundThreadID := strings.TrimSpace(stringField(turn, "threadId"))
	return boundThreadID == "" || boundThreadID == strings.TrimSpace(threadID)
}

// ProjectAuthorityFreeSidecarItem is the only migration projection for a
// public messages sidecar. Tool payloads are closed, grant transitions are
// removed, and pending gate handles lose continuation authority.
func ProjectAuthorityFreeSidecarItem(threadID string, item map[string]any, at string) (map[string]any, bool) {
	projected := threadapp.CloneItemForThread(item, strings.TrimSpace(threadID), strings.TrimSpace(at))
	if projected == nil {
		return nil, false
	}
	// Legacy provider call ids are intentionally not promoted to current host
	// tool-call identity. The closed tool projection may therefore omit its
	// derived item id. Preserve only the pre-existing canonical durable item id
	// as rendering/audit identity so a migration cannot create an anonymous
	// item or break deterministic turn inventory; it conveys no grant or
	// evidence authority.
	if strings.TrimSpace(stringField(projected, "id")) == "" {
		legacyItemID := strings.TrimSpace(stringField(item, "id"))
		if contracts.SafeRecordID(legacyItemID) == legacyItemID && legacyItemID != "" {
			projected["id"] = legacyItemID
		}
	}
	delete(projected, "acceptedFinal")
	delete(projected, "acceptedFinalView")
	stripAuthorityFields(projected)
	return projected, true
}

// EventAuthorityBelongsToThread validates authority-bearing durable events
// against the exact frozen V2 context and replayable registry of their turn.
// Authority-free events are outside this migration's scope and return true.
func EventAuthorityBelongsToThread(thread map[string]any, event map[string]any) bool {
	if !recordContainsAuthority(event) {
		return true
	}
	threadID := strings.TrimSpace(stringField(thread, "id"))
	turnID := strings.TrimSpace(stringField(event, "turnId"))
	if threadID == "" || turnID == "" || strings.TrimSpace(stringField(event, "threadId")) != threadID {
		return false
	}
	turn, ok := turnByID(thread, turnID)
	if !ok || !turnAuthorityIsCurrent(threadID, thread, turn, true) {
		return false
	}
	context, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil {
		return false
	}
	registry, err := executiongrantapp.RegistryFromThread(threadID, thread, turnID)
	if err != nil || !recordAuthorityMatches(event, context, registry) {
		return false
	}
	if item, _ := event["item"].(map[string]any); item != nil && !recordAuthorityMatches(item, context, registry) {
		return false
	}
	return true
}

func turnAuthorityIsCurrent(threadID string, thread, turn map[string]any, requiresExecution bool) bool {
	turnID := strings.TrimSpace(stringField(turn, "id"))
	context, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || context.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		context.ThreadID != threadID || context.TurnID != turnID {
		return false
	}
	if !requiresExecution {
		return domainsecurity.ValidateTurnSecurityContext(context) == nil
	}
	// Boundary-only contexts can still carry the permanent ordinary Agent
	// capability. RegistryFromThread below replays every concrete grant through
	// its tool-name classifier, so protected calls remain fail-closed while
	// ordinary read/write authority can survive restart.
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
		return false
	}
	_, err = executiongrantapp.RegistryFromThread(threadID, thread, turnID)
	return err == nil
}

func recordAuthorityMatches(record map[string]any, context domainsecurity.TurnSecurityContext, registry domainsecurity.ExecutionGrantRegistry) bool {
	if record == nil {
		return true
	}
	if value, ok := record["threadId"]; ok && strings.TrimSpace(stringValue(value)) != context.ThreadID {
		return false
	}
	if value, ok := record["turnId"]; ok && strings.TrimSpace(stringValue(value)) != context.TurnID {
		return false
	}
	if _, ok := record["contextDigest"]; ok && strings.TrimSpace(stringField(record, "contextDigest")) != context.ContextDigest {
		return false
	}
	if _, ok := record["contextEpoch"]; ok {
		epoch, valid := uint64Value(record["contextEpoch"])
		if !valid || epoch != context.ContextEpoch {
			return false
		}
	}
	if value, ok := record["executionGrant"]; ok {
		grant, err := domainsecurity.ParseExecutionGrant(value)
		if err != nil || domainsecurity.ValidateExecutionGrantForContext(grant, context) != nil {
			return false
		}
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
		if !found || entry.Grant != grant {
			return false
		}
	}
	if grantID, ok := record["executionGrantId"]; ok {
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, strings.TrimSpace(stringValue(grantID)))
		if !found || entry.ContextDigest != context.ContextDigest || entry.TurnID != context.TurnID {
			return false
		}
	}
	return true
}

func stripTurnExecutionAuthority(turn map[string]any, threadID, at string) map[string]any {
	projected := contracts.CloneMap(turn)
	projected["threadId"] = threadID
	stripAuthorityFields(projected)
	delete(projected, "securityContext")
	delete(projected, "contextEpochSnapshot")
	delete(projected, "acceptedFinal")
	delete(projected, "acceptedFinalView")
	delete(projected, "pendingApprovalIds")
	delete(projected, "pendingUserInputIds")
	items, _ := projected["items"].([]any)
	closed := make([]any, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		projectedItem, ok := ProjectAuthorityFreeSidecarItem(threadID, item, at)
		if ok {
			closed = append(closed, projectedItem)
		}
	}
	projected["items"] = closed
	return projected
}

func turnContainsExecutionAuthority(turn map[string]any) bool {
	items, _ := turn["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if recordContainsAnyKey(item, executionAuthorityKeys) || recordContainsAnyKey(item, continuationAuthorityKeys) ||
			strings.TrimSpace(stringField(item, "kind")) == "execution_grant_transition" {
			return true
		}
	}
	return recordContainsAnyKey(turn, append(append([]string{}, executionAuthorityKeys...), continuationAuthorityKeys...))
}

func turnContainsAnyFrozenAuthority(turn map[string]any) bool {
	if turn["securityContext"] != nil || turn["acceptedFinal"] != nil || turn["acceptedFinalView"] != nil {
		return true
	}
	return turnContainsExecutionAuthority(turn)
}

func recordContainsAuthority(record map[string]any) bool {
	if recordContainsAnyKey(record, append(append([]string{}, executionAuthorityKeys...), continuationAuthorityKeys...)) {
		return true
	}
	item, _ := record["item"].(map[string]any)
	return recordContainsAnyKey(item, append(append([]string{}, executionAuthorityKeys...), continuationAuthorityKeys...))
}

func recordContainsAnyKey(record map[string]any, keys []string) bool {
	if record == nil {
		return false
	}
	for _, key := range keys {
		if _, ok := record[key]; ok {
			return true
		}
	}
	return false
}

func stripAuthorityFields(record map[string]any) {
	for _, key := range append(append([]string{}, executionAuthorityKeys...), continuationAuthorityKeys...) {
		delete(record, key)
	}
}

func rootContextBelongsToThread(thread map[string]any, threadID string) bool {
	context, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	return err == nil && context.Version == domainsecurity.TurnSecurityContextVersionV2 && context.ThreadID == threadID
}

func derivedThread(thread map[string]any, threadID string) bool {
	source := strings.TrimSpace(stringField(thread, "forkedFromThreadId"))
	return source != "" && source != threadID
}

func stableMigrationTime(thread, turn map[string]any) string {
	for _, value := range []string{
		stringField(thread, "forkedAt"), stringField(turn, "finishedAt"), stringField(turn, "createdAt"), stringField(thread, "updatedAt"),
	} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "1970-01-01T00:00:00Z"
}

func turnByID(thread map[string]any, turnID string) (map[string]any, bool) {
	turns, _ := thread["turns"].([]any)
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		if strings.TrimSpace(stringField(turn, "id")) == strings.TrimSpace(turnID) {
			return turn, true
		}
	}
	return nil, false
}

func stringField(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	return stringValue(record[key])
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func uint64Value(value any) (uint64, bool) {
	switch typed := value.(type) {
	case uint64:
		return typed, typed > 0
	case uint:
		return uint64(typed), typed > 0
	case int:
		return uint64(typed), typed > 0
	case int64:
		return uint64(typed), typed > 0
	case float64:
		if typed > 0 && typed <= 9007199254740991 && typed == float64(uint64(typed)) {
			return uint64(typed), true
		}
	}
	return 0, false
}
