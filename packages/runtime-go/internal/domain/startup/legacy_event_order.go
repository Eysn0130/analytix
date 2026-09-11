package startup

import (
	"reflect"
	"sort"
	"strings"

	domainevent "analytix.local/runtime-go/internal/domain/event"
)

var currentEventOrderAuthorityKeysV1 = map[string]struct{}{
	"securityState": {}, "contextEpochState": {}, "securityContext": {}, "contextEpochSnapshot": {},
	"contextDigest": {}, "contextEpoch": {}, "workspaceRealPath": {}, "tenantId": {}, "userId": {},
	"caseId": {}, "caseBindingHash": {}, "datasetSnapshotId": {}, "sourceManifestHash": {},
	"pendingApprovalIds": {}, "pendingUserInputIds": {},
	"executionGrant": {}, "executionGrantId": {}, "grantId": {}, "parentGrantId": {},
	"hostEvidenceSettlement": {}, "approvalTransition": {}, "approvalTransitionId": {}, "approvalItemId": {},
	"continuationDispositionId": {}, "continuationReceiptId": {}, "approvalId": {}, "inputId": {},
}

var currentEventOrderAuthorityKindsV1 = map[string]struct{}{
	"accepted_final_batch":   {},
	"general_terminal_batch": {},
}

var currentEventOrderAuthorityPurposesV1 = map[string]struct{}{
	"analytix.accepted-final-delivery-batch/v1":   {},
	"analytix.accepted-final-delivery-batch/v2":   {},
	"analytix.general-terminal-delivery-batch/v1": {},
}

// ContainsCurrentEventOrderAuthorityV1 reports whether a persisted value
// carries current context, accepted-final, or general-terminal publication
// authority. Execution keys are authoritative only at bounded host structural
// positions; same-name keys inside arbitrary user or tool payloads remain
// ordinary content. At a host position, presence is authoritative even when
// the value is null: a migration must never reinterpret a malformed current
// record as legacy and then repair its physical event order.
func ContainsCurrentEventOrderAuthorityV1(value any) bool {
	if domainevent.ContainsTerminalPublicationAuthority(value) {
		return true
	}
	return containsCurrentEventOrderExecutionAuthorityV1(value)
}

// IsCurrentEventOrderAuthorityKeyV1 is the canonical key-level predicate used
// by startup migrations that must preserve frozen authority values byte for
// byte. Callers must first establish a bounded host structural position. At
// such a position, it deliberately treats presence as authority even when the
// value is null or malformed.
func IsCurrentEventOrderAuthorityKeyV1(key string) bool {
	_, protected := currentEventOrderAuthorityKeysV1[key]
	return protected
}

// A gate handle identifies a replay entry but does not itself prove current
// execution authority. Current approval/input records become authoritative
// only when a host-issued context, transition, disposition, or continuation
// receipt is also present. The handles remain frozen after such an anchor is
// established; they are merely excluded from authority discovery here.
func isCurrentEventOrderAuthorityAnchorKeyV1(key string) bool {
	if key == "approvalId" || key == "inputId" {
		return false
	}
	return IsCurrentEventOrderAuthorityKeyV1(key)
}

// CurrentEventOrderAuthorityKeysV1 returns a deterministic copy for strict
// malformed-record preflight. Callers cannot mutate the canonical set.
func CurrentEventOrderAuthorityKeysV1() []string {
	keys := make([]string, 0, len(currentEventOrderAuthorityKeysV1))
	for key := range currentEventOrderAuthorityKeysV1 {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// FrozenEventOrderAuthorityStableV1 verifies that a public projection changed
// no frozen execution-authority value and no terminal-publication container.
// Ordinary content outside those fields may still be redacted. Terminal
// containers are compared as a whole because their payload digests bind
// sibling fields as well as the marker values themselves.
func FrozenEventOrderAuthorityStableV1(before, after any) bool {
	if !ContainsCurrentEventOrderAuthorityV1(before) && !ContainsCurrentEventOrderAuthorityV1(after) {
		return true
	}
	return frozenStructuredAuthorityStableV1(before, after)
}

func frozenStructuredAuthorityStableV1(before, after any) bool {
	previous, previousMap := before.(map[string]any)
	current, currentMap := after.(map[string]any)
	if !previousMap || !currentMap {
		return !previousMap && !currentMap
	}
	if directTerminalPublicationAuthorityV1(previous) || directTerminalPublicationAuthorityV1(current) {
		return reflect.DeepEqual(previous, current)
	}
	for key := range currentEventOrderAuthorityKeysV1 {
		previousValue, previousPresent := previous[key]
		currentValue, currentPresent := current[key]
		if previousPresent != currentPresent || previousPresent && !reflect.DeepEqual(previousValue, currentValue) {
			return false
		}
	}
	for _, key := range []string{"turns", "items"} {
		previousValues, previousPresent := previous[key].([]any)
		currentValues, currentPresent := current[key].([]any)
		if previousPresent != currentPresent {
			return false
		}
		if !previousPresent {
			continue
		}
		if len(previousValues) != len(currentValues) {
			return false
		}
		for index := range previousValues {
			if !frozenStructuredAuthorityStableV1(previousValues[index], currentValues[index]) {
				return false
			}
		}
	}
	previousItem, previousItemPresent := previous["item"].(map[string]any)
	currentItem, currentItemPresent := current["item"].(map[string]any)
	if previousItemPresent != currentItemPresent || previousItemPresent && !frozenStructuredAuthorityStableV1(previousItem, currentItem) {
		return false
	}
	return true
}

func directTerminalPublicationAuthorityV1(record map[string]any) bool {
	for key := range record {
		if domainevent.ContainsTerminalPublicationAuthority(map[string]any{key: nil}) {
			return true
		}
	}
	kind, _ := record["kind"].(string)
	if _, protected := currentEventOrderAuthorityKindsV1[strings.TrimSpace(kind)]; protected {
		return true
	}
	purpose, _ := record["purpose"].(string)
	_, protected := currentEventOrderAuthorityPurposesV1[strings.TrimSpace(purpose)]
	return protected
}

func containsCurrentEventOrderExecutionAuthorityV1(value any) bool {
	record, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for key, child := range record {
		if isCurrentEventOrderAuthorityAnchorKeyV1(key) {
			return true
		}
		switch key {
		case "kind":
			kind, _ := child.(string)
			if _, protected := currentEventOrderAuthorityKindsV1[strings.TrimSpace(kind)]; protected {
				return true
			}
		case "purpose":
			purpose, _ := child.(string)
			if _, protected := currentEventOrderAuthorityPurposesV1[strings.TrimSpace(purpose)]; protected {
				return true
			}
		}
	}
	for _, key := range []string{"turns", "items"} {
		values, _ := record[key].([]any)
		for _, child := range values {
			if containsCurrentEventOrderExecutionAuthorityV1(child) {
				return true
			}
		}
	}
	if item, _ := record["item"].(map[string]any); item != nil && containsCurrentEventOrderExecutionAuthorityV1(item) {
		return true
	}
	return false
}

// LegacyPrimaryThreadAllowsEventOrderMigrationV1 is the shared classification
// used by both the startup snapshot and the staged event migration. It accepts
// only the bounded legacy primary shape needed to repair historical
// operational-event interleaving. Unknown or malformed turn/item containers
// fail closed.
func LegacyPrimaryThreadAllowsEventOrderMigrationV1(record map[string]any, threadID string) bool {
	threadID = strings.TrimSpace(threadID)
	recordID, _ := record["id"].(string)
	if threadID == "" || recordID != threadID || ContainsCurrentEventOrderAuthorityV1(record) {
		return false
	}
	turnsValue, present := record["turns"]
	if !present {
		return false
	}
	turns, ok := turnsValue.([]any)
	if !ok {
		return false
	}
	for _, value := range turns {
		turn, ok := value.(map[string]any)
		if !ok {
			return false
		}
		itemsValue, present := turn["items"]
		if !present {
			continue
		}
		items, ok := itemsValue.([]any)
		if !ok {
			return false
		}
		for _, item := range items {
			if _, ok := item.(map[string]any); !ok {
				return false
			}
		}
	}
	return true
}

// IsEventOrderTransactionResidueV1 identifies every known or future file in
// the atomic event-bundle transaction namespace. A legacy order migration may
// run only after that namespace is completely absent.
func IsEventOrderTransactionResidueV1(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), ".events-bundle-")
}
