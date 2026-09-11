package executiongrant

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	"analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

const GrantTransitionItemKind = "execution_grant_transition"

type DurableSettlementAuthority struct {
	ActiveRegistry  domainsecurity.ExecutionGrantRegistry
	SettledRegistry domainsecurity.ExecutionGrantRegistry
	ResultItem      map[string]any
	SettledAt       time.Time
}

type DurableApprovalTransitionAuthority struct {
	Transition    domainsecurity.ApprovalGrantTransitionV1
	PendingGrant  domainsecurity.ExecutionGrant
	ApprovedGrant domainsecurity.ExecutionGrant
	Item          map[string]any
}

// DurableApprovalTransitionFromThread proves that one exact V1 approval
// transition is present in the current durable registry and that its approved
// grant is still active. Public IDs alone are never sufficient authority.
func DurableApprovalTransitionFromThread(
	threadID string,
	thread map[string]any,
	turnID string,
	transitionID string,
) (DurableApprovalTransitionAuthority, error) {
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	transitionID = strings.TrimSpace(transitionID)
	if threadID == "" || turnID == "" || !domainsecurity.IsSHA256Hex(transitionID) {
		return DurableApprovalTransitionAuthority{}, ValidationError{Code: "execution_grant_approval_transition_invalid"}
	}
	turn, ok := appmodel.TurnByID(thread, turnID)
	if !ok || (stringMapField(thread, "id") != "" && stringMapField(thread, "id") != threadID) {
		return DurableApprovalTransitionAuthority{}, ValidationError{Code: "execution_grant_approval_transition_invalid"}
	}
	registry, err := RegistryFromThread(threadID, thread, turnID)
	if err != nil {
		return DurableApprovalTransitionAuthority{}, err
	}
	securityContext, err := registryContextForTurn(threadID, turnID, turn)
	if err != nil || securityContext == nil {
		return DurableApprovalTransitionAuthority{}, ValidationError{Code: "execution_grant_approval_transition_invalid"}
	}
	var authority DurableApprovalTransitionAuthority
	matches := 0
	for _, raw := range registryItems(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringMapField(item, "kind") != GrantTransitionItemKind || stringMapField(item, "approvalTransitionId") != transitionID {
			continue
		}
		transition, parseErr := domainsecurity.ParseApprovalGrantTransitionV1(item["approvalTransition"])
		approved, grantErr := registryGrantFromItem(item)
		parent, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, transition.ParentGrantID)
		if parseErr != nil || grantErr != nil || !found ||
			ValidateExecutionGrantForToolName(*securityContext, parent.Grant, parent.Grant.ToolName) != nil ||
			ValidateExecutionGrantForToolName(*securityContext, approved, approved.ToolName) != nil ||
			domainsecurity.ValidateApprovalGrantTransitionV1(transition, *securityContext, parent.Grant, approved) != nil {
			return DurableApprovalTransitionAuthority{}, ValidationError{Code: "execution_grant_approval_transition_invalid"}
		}
		if domainsecurity.VerifyExecutionGrantMembership(registry, threadID, turnID, approved, domainsecurity.GrantRegistryActive) != nil {
			return DurableApprovalTransitionAuthority{}, ValidationError{Code: "execution_grant_approval_transition_inactive"}
		}
		matches++
		authority = DurableApprovalTransitionAuthority{
			Transition: transition, PendingGrant: parent.Grant, ApprovedGrant: approved, Item: contracts.CloneMap(item),
		}
	}
	if matches != 1 {
		return DurableApprovalTransitionAuthority{}, ValidationError{Code: "execution_grant_approval_transition_missing"}
	}
	return authority, nil
}

// DurableSettlementFromThread proves the exact registry prefix immediately
// before one tool_result was active and the prefix including that result is
// settled. A caller-supplied historical active snapshot is insufficient.
func DurableSettlementFromThread(threadID string, thread map[string]any, turnID string, resultItemID string, grant domainsecurity.ExecutionGrant) (DurableSettlementAuthority, error) {
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	resultItemID = strings.TrimSpace(resultItemID)
	if resultItemID == "" || domainsecurity.ValidateExecutionGrant(grant) != nil {
		return DurableSettlementAuthority{}, ValidationError{Code: "execution_grant_settlement_authority_invalid"}
	}
	turn, ok := appmodel.TurnByID(thread, turnID)
	if threadID == "" || turnID == "" || !ok || (stringMapField(thread, "id") != "" && stringMapField(thread, "id") != threadID) {
		return DurableSettlementAuthority{}, ValidationError{Code: "execution_grant_registry_authority_mismatch"}
	}
	securityContext, err := registryContextForTurn(threadID, turnID, turn)
	if err != nil || securityContext == nil {
		return DurableSettlementAuthority{}, ValidationError{Code: "execution_grant_registry_context_invalid"}
	}
	registry := domainsecurity.NewExecutionGrantRegistry(threadID)
	var authority DurableSettlementAuthority
	found := false
	for _, raw := range registryItems(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if stringMapField(item, "id") == resultItemID {
			if found || stringMapField(item, "kind") != "tool_result" ||
				domainsecurity.VerifyExecutionGrantMembership(registry, threadID, turnID, grant, domainsecurity.GrantRegistryActive, domainsecurity.GrantRegistryPending) != nil {
				return DurableSettlementAuthority{}, ValidationError{Code: "execution_grant_settlement_prefix_invalid"}
			}
			activeSnapshot, err := domainsecurity.ParseExecutionGrantRegistry(domainsecurity.ExecutionGrantRegistryRecord(registry))
			if err != nil {
				return DurableSettlementAuthority{}, ValidationError{Code: "execution_grant_settlement_prefix_invalid"}
			}
			authority.ActiveRegistry = activeSnapshot
			settledAt, err := registryItemTime(item, "finishedAt", "createdAt")
			if err != nil {
				return DurableSettlementAuthority{}, ValidationError{Code: "execution_grant_settlement_time_invalid"}
			}
			authority.SettledAt = settledAt
			authority.ResultItem = contracts.CloneMap(item)
			found = true
		}
		next, err := applyRegistryItem(registry, threadID, turnID, item, securityContext)
		if err != nil {
			return DurableSettlementAuthority{}, err
		}
		registry = next
		if found && authority.SettledRegistry.Version == 0 && stringMapField(item, "id") == resultItemID {
			if domainsecurity.VerifyExecutionGrantMembership(registry, threadID, turnID, grant, domainsecurity.GrantRegistrySettled) != nil {
				return DurableSettlementAuthority{}, ValidationError{Code: "execution_grant_settlement_status_invalid"}
			}
			settledSnapshot, err := domainsecurity.ParseExecutionGrantRegistry(domainsecurity.ExecutionGrantRegistryRecord(registry))
			if err != nil {
				return DurableSettlementAuthority{}, ValidationError{Code: "execution_grant_settlement_status_invalid"}
			}
			authority.SettledRegistry = settledSnapshot
		}
	}
	if !found || authority.SettledRegistry.Version == 0 {
		return DurableSettlementAuthority{}, ValidationError{Code: "execution_grant_settlement_result_missing"}
	}
	return authority, nil
}

func RegistryFromThread(threadID string, thread map[string]any, turnID string) (domainsecurity.ExecutionGrantRegistry, error) {
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	if threadID == "" || turnID == "" || thread == nil || (stringMapField(thread, "id") != "" && stringMapField(thread, "id") != threadID) {
		return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_authority_mismatch"}
	}
	turn, ok := appmodel.TurnByID(thread, turnID)
	if !ok {
		return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_turn_inactive"}
	}
	securityContext, err := registryContextForTurn(threadID, turnID, turn)
	if err != nil {
		return domainsecurity.ExecutionGrantRegistry{}, err
	}
	registry := domainsecurity.NewExecutionGrantRegistry(threadID)
	for _, raw := range registryItems(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		registry, err = applyRegistryItem(registry, threadID, turnID, item, securityContext)
		if err != nil {
			if typed, ok := err.(ValidationError); ok {
				return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_replay_" + strings.TrimPrefix(typed.Code, "execution_grant_registry_")}
			}
			return domainsecurity.ExecutionGrantRegistry{}, err
		}
	}
	return registry, nil
}

// RegistrySnapshotFromThread returns the exact durable registry state that
// existed at one content digest. Sequence alone is insufficient because later
// settlements update member status without increasing registry sequence.
// The whole item stream is still replayed so corruption after the requested
// snapshot cannot lend authority to an otherwise valid historical prefix.
func RegistrySnapshotFromThread(
	threadID string,
	thread map[string]any,
	turnID string,
	sequence uint64,
	stateDigest string,
) (domainsecurity.ExecutionGrantRegistry, error) {
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	stateDigest = strings.TrimSpace(stateDigest)
	if threadID == "" || turnID == "" || sequence == 0 || !domainsecurity.IsSHA256Hex(stateDigest) || thread == nil ||
		(stringMapField(thread, "id") != "" && stringMapField(thread, "id") != threadID) {
		return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_snapshot_invalid"}
	}
	turn, ok := appmodel.TurnByID(thread, turnID)
	if !ok {
		return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_snapshot_turn_missing"}
	}
	securityContext, err := registryContextForTurn(threadID, turnID, turn)
	if err != nil {
		return domainsecurity.ExecutionGrantRegistry{}, err
	}
	registry := domainsecurity.NewExecutionGrantRegistry(threadID)
	var snapshot domainsecurity.ExecutionGrantRegistry
	matches := 0
	for _, raw := range registryItems(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		next, err := applyRegistryItem(registry, threadID, turnID, item, securityContext)
		if err != nil {
			return domainsecurity.ExecutionGrantRegistry{}, err
		}
		changed := next.Sequence != registry.Sequence || next.StateDigest != registry.StateDigest
		registry = next
		if changed && registry.Sequence == sequence && registry.StateDigest == stateDigest {
			parsed, err := domainsecurity.ParseExecutionGrantRegistry(domainsecurity.ExecutionGrantRegistryRecord(registry))
			if err != nil {
				return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_snapshot_invalid"}
			}
			snapshot = parsed
			matches++
		}
	}
	if matches != 1 {
		return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_snapshot_missing"}
	}
	return snapshot, nil
}

func ValidateRegistryAppend(threadID string, thread map[string]any, turnID string, item map[string]any) error {
	registry, err := RegistryFromThread(threadID, thread, turnID)
	if err != nil {
		return err
	}
	turn, ok := appmodel.TurnByID(thread, strings.TrimSpace(turnID))
	if !ok {
		return ValidationError{Code: "execution_grant_turn_inactive"}
	}
	securityContext, contextErr := registryContextForTurn(strings.TrimSpace(threadID), strings.TrimSpace(turnID), turn, item)
	if contextErr != nil {
		return contextErr
	}
	_, err = applyRegistryItem(registry, strings.TrimSpace(threadID), strings.TrimSpace(turnID), item, securityContext)
	if typed, ok := err.(ValidationError); ok {
		return ValidationError{Code: "execution_grant_registry_append_" + strings.TrimPrefix(typed.Code, "execution_grant_registry_")}
	}
	return err
}

func VerifyThreadGrantMembership(threadID string, thread map[string]any, turnID string, grant domainsecurity.ExecutionGrant, allowed ...domainsecurity.ExecutionGrantRegistryStatus) error {
	registry, err := RegistryFromThread(threadID, thread, turnID)
	if err != nil {
		return err
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(registry, threadID, turnID, grant, allowed...); err != nil {
		return ValidationError{Code: "execution_grant_registry_membership_invalid"}
	}
	return nil
}

func ApprovalTransitionRecords(
	context domainsecurity.TurnSecurityContext,
	transition domainsecurity.ApprovalGrantTransitionV1,
	pending domainsecurity.ExecutionGrant,
	approved domainsecurity.ExecutionGrant,
) (map[string]any, map[string]any, error) {
	if err := ValidateExecutionGrantForToolName(context, pending, pending.ToolName); err != nil ||
		ValidateExecutionGrantForToolName(context, approved, approved.ToolName) != nil ||
		pending.ContextDigest != context.ContextDigest || approved.ContextDigest != context.ContextDigest ||
		domainsecurity.ValidateApprovalGrantTransitionV1(transition, context, pending, approved) != nil {
		return nil, nil, errors.New("approval transition context is invalid")
	}
	transitionedAt, err := time.Parse(time.RFC3339Nano, transition.TransitionedAt)
	if err != nil {
		return nil, nil, errors.New("approval transition time is invalid")
	}
	registry, err := domainsecurity.RegisterExecutionGrant(domainsecurity.NewExecutionGrantRegistry(context.ThreadID), context.ThreadID, pending, transitionedAt)
	if err != nil {
		return nil, nil, err
	}
	if _, err := domainsecurity.ApproveRegisteredExecutionGrant(registry, pending, approved, transitionedAt); err != nil {
		return nil, nil, err
	}
	itemID := "item_grant_" + transition.TransitionID
	createdAt := transitionedAt.UTC().Format(time.RFC3339Nano)
	transitionRecord := approvalGrantTransitionRecord(transition)
	item := map[string]any{
		"id": itemID, "threadId": context.ThreadID, "turnId": context.TurnID,
		"kind": GrantTransitionItemKind, "role": "tool", "status": "completed", "createdAt": createdAt, "finishedAt": createdAt,
		"toolName": approved.ToolName, "callId": approved.ToolCallID, "contextDigest": approved.ContextDigest,
		"contextEpoch": float64(context.ContextEpoch), "executionGrantId": approved.GrantID,
		"parentGrantId": pending.GrantID, "executionGrant": executionGrantRecord(approved),
		"approvalId": transition.ApprovalID, "approvalItemId": transition.ApprovalItemID, "continuationReceiptId": transition.ContinuationReceiptID,
		"continuationDispositionId": transition.ContinuationDispositionID,
		"approvalTransitionId":      transition.TransitionID, "approvalTransition": transitionRecord,
	}
	event := map[string]any{
		"kind": "execution_grant_approved", "threadId": context.ThreadID, "turnId": context.TurnID,
		"itemId": itemID, "callId": approved.ToolCallID, "toolName": approved.ToolName,
		"contextDigest": approved.ContextDigest, "contextEpoch": float64(context.ContextEpoch),
		"executionGrantId": approved.GrantID, "parentGrantId": pending.GrantID,
		"approvalId": transition.ApprovalID, "approvalItemId": transition.ApprovalItemID, "continuationReceiptId": transition.ContinuationReceiptID,
		"continuationDispositionId": transition.ContinuationDispositionID,
		"approvalTransitionId":      transition.TransitionID, "approvalTransition": transitionRecord,
	}
	return item, event, nil
}

func applyRegistryItem(registry domainsecurity.ExecutionGrantRegistry, threadID string, turnID string, item map[string]any, securityContext *domainsecurity.TurnSecurityContext) (domainsecurity.ExecutionGrantRegistry, error) {
	if !registryItemCarriesAuthority(item) {
		return registry, nil
	}
	if err := validateRegistryItemContext(threadID, turnID, item, securityContext); err != nil {
		return domainsecurity.ExecutionGrantRegistry{}, err
	}
	digest := stringMapField(item, "contextDigest")
	grantID := stringMapField(item, "executionGrantId")
	kind := stringMapField(item, "kind")
	if digest == "" || grantID == "" {
		return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_record_incomplete"}
	}
	switch kind {
	case "tool_call":
		grant, err := registryGrantFromItem(item)
		if err != nil || securityContext == nil ||
			ValidateExecutionGrantForToolName(*securityContext, grant, grant.ToolName) != nil {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_grant_invalid"}
		}
		if mismatch := registryItemGrantMismatch(item, grant); mismatch != "" {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_" + mismatch + "_mismatch"}
		}
		registeredAt, err := registryItemTime(item, "createdAt")
		if err != nil {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_time_invalid"}
		}
		next, err := domainsecurity.RegisterExecutionGrant(registry, threadID, grant, registeredAt)
		if err != nil {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_registration_invalid"}
		}
		return next, nil
	case GrantTransitionItemKind:
		approved, err := registryGrantFromItem(item)
		if err != nil || securityContext == nil ||
			ValidateExecutionGrantForToolName(*securityContext, approved, approved.ToolName) != nil ||
			registryItemGrantMismatch(item, approved) != "" {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_transition_invalid"}
		}
		parentID := stringMapField(item, "parentGrantId")
		parent, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, parentID)
		if !found {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_transition_invalid"}
		}
		transition, err := domainsecurity.ParseApprovalGrantTransitionV1(item["approvalTransition"])
		if err != nil ||
			ValidateExecutionGrantForToolName(*securityContext, parent.Grant, parent.Grant.ToolName) != nil ||
			domainsecurity.ValidateApprovalGrantTransitionV1(transition, *securityContext, parent.Grant, approved) != nil ||
			stringMapField(item, "approvalTransitionId") != transition.TransitionID ||
			stringMapField(item, "approvalId") != transition.ApprovalID ||
			stringMapField(item, "approvalItemId") != transition.ApprovalItemID ||
			stringMapField(item, "continuationReceiptId") != transition.ContinuationReceiptID ||
			stringMapField(item, "continuationDispositionId") != transition.ContinuationDispositionID ||
			stringMapField(item, "id") != "item_grant_"+transition.TransitionID {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_transition_invalid"}
		}
		transitionAt, err := registryItemTime(item, "createdAt")
		if err != nil || transitionAt.Format(time.RFC3339Nano) != transition.TransitionedAt {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_transition_invalid"}
		}
		next, err := domainsecurity.ApproveRegisteredExecutionGrant(registry, parent.Grant, approved, transitionAt)
		if err != nil {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_transition_invalid"}
		}
		return next, nil
	case "tool_result":
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grantID)
		if !found || entry.ContextDigest != digest || entry.TurnID != turnID || stringMapField(item, "toolName") != entry.Grant.ToolName || stringMapField(item, "callId") != entry.Grant.ToolCallID {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_membership_invalid"}
		}
		settledAt, err := registryItemTime(item, "finishedAt", "createdAt")
		if err != nil {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_record_invalid"}
		}
		next, err := domainsecurity.SettleRegisteredExecutionGrant(registry, grantID, settledAt)
		if err != nil {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_status_invalid"}
		}
		return next, nil
	default:
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grantID)
		if !found || entry.ContextDigest != digest || (entry.Status != domainsecurity.GrantRegistryActive && entry.Status != domainsecurity.GrantRegistryPending) {
			return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_membership_invalid"}
		}
		return registry, nil
	}
}

func registryContextForTurn(threadID, turnID string, turn map[string]any, additional ...map[string]any) (*domainsecurity.TurnSecurityContext, error) {
	requiresContext := false
	for _, raw := range registryItems(turn["items"]) {
		item, _ := raw.(map[string]any)
		if registryItemCarriesAuthority(item) {
			requiresContext = true
			break
		}
	}
	if !requiresContext {
		for _, item := range additional {
			if registryItemCarriesAuthority(item) {
				requiresContext = true
				break
			}
		}
	}
	if !requiresContext {
		return nil, nil
	}
	context, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil ||
		context.ThreadID != strings.TrimSpace(threadID) || context.TurnID != strings.TrimSpace(turnID) {
		return nil, ValidationError{Code: "execution_grant_registry_context_invalid"}
	}
	return &context, nil
}

func registryItemCarriesAuthority(item map[string]any) bool {
	if item == nil {
		return false
	}
	for _, key := range []string{
		"executionGrantId", "executionGrant", "parentGrantId", "hostEvidenceSettlement",
		"approvalTransition", "approvalTransitionId", "continuationDispositionId",
	} {
		if _, ok := item[key]; ok {
			return true
		}
	}
	return false
}

func validateRegistryItemContext(threadID, turnID string, item map[string]any, context *domainsecurity.TurnSecurityContext) error {
	if !registryItemCarriesAuthority(item) {
		return nil
	}
	if context == nil || context.ThreadID != strings.TrimSpace(threadID) || context.TurnID != strings.TrimSpace(turnID) ||
		stringMapField(item, "threadId") != context.ThreadID || stringMapField(item, "turnId") != context.TurnID ||
		stringMapField(item, "contextDigest") != context.ContextDigest {
		return ValidationError{Code: "execution_grant_registry_context_invalid"}
	}
	epoch, ok := registryItemUint64(item["contextEpoch"])
	if !ok || epoch != context.ContextEpoch {
		return ValidationError{Code: "execution_grant_registry_context_invalid"}
	}
	return nil
}

func registryItemUint64(value any) (uint64, bool) {
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
	case json.Number:
		// Strict primary readers retain JSON numbers. Do not round an epoch
		// through float64 or accept fractional/exponent/overflow spellings.
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			return uint64(parsed), true
		}
	}
	return 0, false
}

func registryGrantFromItem(item map[string]any) (domainsecurity.ExecutionGrant, error) {
	value := item["executionGrant"]
	if value == nil {
		return domainsecurity.ExecutionGrant{}, errors.New("execution grant registry record is missing the full grant")
	}
	return domainsecurity.ParseExecutionGrant(value)
}

func registryItemGrantMismatch(item map[string]any, grant domainsecurity.ExecutionGrant) string {
	for _, check := range []struct {
		field string
		want  string
		code  string
	}{
		{"executionGrantId", grant.GrantID, "id"}, {"contextDigest", grant.ContextDigest, "context"},
		{"turnId", grant.TurnID, "turn"}, {"toolName", grant.ToolName, "tool"}, {"callId", grant.ToolCallID, "call"},
	} {
		if stringMapField(item, check.field) != check.want {
			return check.code
		}
	}
	if arguments, found := item["arguments"]; found {
		if _, err := domaintoolcall.ParsePublicToolCallArgumentsProjectionV1(arguments); err == nil {
			return ""
		}
		// Legacy records may still contain exact arguments until the journaled
		// migration rewrites them. They are read only when the host-issued grant
		// cryptographically binds the same canonical value; no new write path
		// emits this shape and every public projection withholds it.
		body, err := json.Marshal(arguments)
		if err != nil || domainsecurity.CanonicalJSONHash(body) != grant.ArgsHash {
			return "arguments"
		}
		return ""
	}
	if stringMapField(item, "kind") != GrantTransitionItemKind {
		return "arguments"
	}
	return ""
}

// ValidateStoredExecutionGrantItemV1 checks the immutable item bindings and
// raw/projection argument contract. It grants no current execution authority.
func ValidateStoredExecutionGrantItemV1(item map[string]any, grant domainsecurity.ExecutionGrant) error {
	if mismatch := registryItemGrantMismatch(item, grant); mismatch != "" {
		return ValidationError{Code: "execution_grant_registry_" + mismatch + "_mismatch"}
	}
	return nil
}

func registryItemTime(item map[string]any, keys ...string) (time.Time, error) {
	for _, key := range keys {
		if value := stringMapField(item, key); value != "" {
			return time.Parse(time.RFC3339Nano, value)
		}
	}
	return time.Time{}, errors.New("execution grant registry record time is missing")
}

func registryItems(value any) []any {
	items, _ := value.([]any)
	return items
}

func executionGrantRecord(grant domainsecurity.ExecutionGrant) map[string]any {
	body, _ := json.Marshal(grant)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func approvalGrantTransitionRecord(transition domainsecurity.ApprovalGrantTransitionV1) map[string]any {
	body, _ := json.Marshal(transition)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
