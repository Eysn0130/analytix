package executiongrant

import (
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// ValidatePriorSettledToolReferences resolves private gate-continuation
// references exclusively from the current durable turn and grant registry.
// Synthetic provider pairing messages and caller-supplied grant objects cannot
// satisfy this authority boundary.
func ValidatePriorSettledToolReferences(threadID string, thread map[string]any, context domainsecurity.TurnSecurityContext, currentGrant domainsecurity.ExecutionGrant, references []domainsecurity.SettledToolReference) error {
	if strings.TrimSpace(threadID) != context.ThreadID ||
		ValidateExecutionGrantForToolName(context, currentGrant, currentGrant.ToolName) != nil ||
		currentGrant.TurnID != context.TurnID || currentGrant.ContextDigest != context.ContextDigest ||
		domainsecurity.ValidateSettledToolReferences(references) != nil {
		return ValidationError{Code: "execution_grant_prior_settlement_invalid"}
	}
	registry, err := RegistryFromThread(threadID, thread, context.TurnID)
	if err != nil {
		return ValidationError{Code: "execution_grant_prior_settlement_registry_invalid"}
	}
	currentEntry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, currentGrant.GrantID)
	currentIssuedAt, issuedErr := time.Parse(time.RFC3339Nano, currentGrant.IssuedAt)
	if !found || currentEntry.Grant != currentGrant || issuedErr != nil {
		return ValidationError{Code: "execution_grant_prior_settlement_current_invalid"}
	}
	for _, reference := range references {
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, reference.GrantID)
		if !found || ValidateExecutionGrantForToolName(context, entry.Grant, entry.Grant.ToolName) != nil ||
			entry.Status != domainsecurity.GrantRegistrySettled || entry.TurnID != context.TurnID || entry.ContextDigest != context.ContextDigest {
			return ValidationError{Code: "execution_grant_prior_settlement_membership_invalid"}
		}
		durable, err := DurableSettlementFromThread(threadID, thread, context.TurnID, reference.ResultItemID, entry.Grant)
		if err != nil || entry.Sequence >= currentEntry.Sequence || durable.SettledAt.After(currentIssuedAt) ||
			!priorResultMatchesContext(durable.ResultItem, context, reference) {
			return ValidationError{Code: "execution_grant_prior_settlement_result_invalid"}
		}
	}
	return nil
}

// ValidatePriorSettledToolReferencesBeforeAdmission validates continuation
// ancestry for a grant that must not yet be present in the durable registry.
// Admission cannot use the ordinary validator because registry membership is
// the effect being authorized, not a precondition supplied by the caller.
func ValidatePriorSettledToolReferencesBeforeAdmission(threadID string, thread map[string]any, context domainsecurity.TurnSecurityContext, currentGrant domainsecurity.ExecutionGrant, references []domainsecurity.SettledToolReference) error {
	if strings.TrimSpace(threadID) != context.ThreadID ||
		ValidateExecutionGrantForToolName(context, currentGrant, currentGrant.ToolName) != nil ||
		currentGrant.TurnID != context.TurnID || currentGrant.ContextDigest != context.ContextDigest ||
		domainsecurity.ValidateSettledToolReferences(references) != nil {
		return ValidationError{Code: "execution_grant_prior_settlement_invalid"}
	}
	registry, err := RegistryFromThread(threadID, thread, context.TurnID)
	if err != nil {
		return ValidationError{Code: "execution_grant_prior_settlement_registry_invalid"}
	}
	if _, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, currentGrant.GrantID); found {
		return ValidationError{Code: "execution_grant_admission_already_registered"}
	}
	currentIssuedAt, issuedErr := time.Parse(time.RFC3339Nano, currentGrant.IssuedAt)
	if issuedErr != nil {
		return ValidationError{Code: "execution_grant_prior_settlement_current_invalid"}
	}
	for _, reference := range references {
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, reference.GrantID)
		if !found || ValidateExecutionGrantForToolName(context, entry.Grant, entry.Grant.ToolName) != nil ||
			entry.Status != domainsecurity.GrantRegistrySettled || entry.TurnID != context.TurnID || entry.ContextDigest != context.ContextDigest {
			return ValidationError{Code: "execution_grant_prior_settlement_membership_invalid"}
		}
		durable, err := DurableSettlementFromThread(threadID, thread, context.TurnID, reference.ResultItemID, entry.Grant)
		if err != nil || durable.SettledAt.After(currentIssuedAt) || !priorResultMatchesContext(durable.ResultItem, context, reference) {
			return ValidationError{Code: "execution_grant_prior_settlement_result_invalid"}
		}
	}
	return nil
}

func priorResultMatchesContext(item map[string]any, context domainsecurity.TurnSecurityContext, reference domainsecurity.SettledToolReference) bool {
	epoch, ok := priorResultEpoch(item["contextEpoch"])
	return ok && epoch == context.ContextEpoch && stringMapField(item, "id") == reference.ResultItemID &&
		stringMapField(item, "kind") == "tool_result" && stringMapField(item, "role") == "tool" &&
		stringMapField(item, "threadId") == context.ThreadID && stringMapField(item, "turnId") == context.TurnID &&
		stringMapField(item, "contextDigest") == context.ContextDigest && stringMapField(item, "executionGrantId") == reference.GrantID
}

func priorResultEpoch(value any) (uint64, bool) {
	switch number := value.(type) {
	case uint64:
		return number, true
	case int:
		return uint64(number), number >= 0
	case float64:
		return uint64(number), number >= 0 && number == float64(uint64(number))
	default:
		return 0, false
	}
}
