package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// This function returns only a verdict against a trusted signed prepared
// digest. It never returns a registry, capability, or repair candidate.
func validateHistoricalSettlementPrefixV1(prepared domainevidence.PreparedEvidenceSettlement, items []map[string]any) error {
	frozen := prepared.SecurityContext
	registry := domainsecurity.NewExecutionGrantRegistry(frozen.ThreadID)
	found := false
	for _, item := range items {
		id := settlementStringField(item, "executionGrantId")
		_, hasGrant := item["executionGrant"]
		if id == "" && !hasGrant {
			continue
		}
		epoch, ok := settlementUint64Field(item, "contextEpoch")
		if !ok || epoch != frozen.ContextEpoch || settlementStringField(item, "threadId") != frozen.ThreadID ||
			settlementStringField(item, "turnId") != frozen.TurnID || settlementStringField(item, "contextDigest") != frozen.ContextDigest {
			return errors.New("historical settlement grant item context is invalid")
		}
		entryIndex := -1
		for index, entry := range registry.Entries {
			if entry.Grant.GrantID == id {
				entryIndex = index
			}
		}
		if settlementStringField(item, "id") == prepared.ResultItemID {
			if entryIndex < 0 || registry.Sequence != prepared.ActiveGrantRegistrySequence || registry.StateDigest != prepared.ActiveGrantRegistryDigest ||
				registry.Entries[entryIndex].Grant != prepared.ExecutionGrant || registry.Entries[entryIndex].Status != domainsecurity.GrantRegistryActive {
				return errors.New("historical settlement signed grant prefix is missing or changed")
			}
			if found {
				return errors.New("historical settlement result is duplicated")
			}
			found = true
		}
		kind := settlementStringField(item, "kind")
		switch kind {
		case "tool_call", executiongrantapp.GrantTransitionItemKind:
			body, err := json.Marshal(item["executionGrant"])
			if err != nil {
				return err
			}
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.DisallowUnknownFields()
			var grant domainsecurity.ExecutionGrant
			if err := decoder.Decode(&grant); err != nil {
				return err
			}
			if domainsecurity.ValidateExecutionGrantForAudit(grant) != nil || grant.GrantID != id || entryIndex >= 0 ||
				grant.TurnID != frozen.TurnID || grant.ContextDigest != frozen.ContextDigest ||
				settlementStringField(item, "toolName") != grant.ToolName || settlementStringField(item, "callId") != grant.ToolCallID {
				return errors.New("historical settlement grant item is invalid")
			}
			if err := executiongrantapp.ValidateStoredExecutionGrantItemV1(item, grant); err != nil {
				return err
			}
			stamp, err := time.Parse(time.RFC3339Nano, settlementStringField(item, "createdAt"))
			if err != nil {
				return err
			}
			status, parent := domainsecurity.GrantRegistryActive, ""
			if grant.ApprovalState == "pending" {
				status = domainsecurity.GrantRegistryPending
			}
			if kind == executiongrantapp.GrantTransitionItemKind {
				parent = settlementStringField(item, "parentGrantId")
				parentIndex := -1
				for index, entry := range registry.Entries {
					if entry.Grant.GrantID == parent {
						parentIndex = index
					}
				}
				transition, err := domainsecurity.ParseApprovalGrantTransitionV1(item["approvalTransition"])
				if err != nil || parentIndex < 0 || registry.Entries[parentIndex].Status != domainsecurity.GrantRegistryPending {
					return errors.Join(errors.New("historical settlement approval parent is invalid"), err)
				}
				if err := domainsecurity.ValidateApprovalGrantTransitionForAuditV1(transition, frozen, registry.Entries[parentIndex].Grant, grant); err != nil {
					return err
				}
				if settlementStringField(item, "id") != "item_grant_"+transition.TransitionID ||
					settlementStringField(item, "approvalTransitionId") != transition.TransitionID ||
					settlementStringField(item, "approvalId") != transition.ApprovalID ||
					settlementStringField(item, "approvalItemId") != transition.ApprovalItemID ||
					settlementStringField(item, "continuationReceiptId") != transition.ContinuationReceiptID ||
					settlementStringField(item, "continuationDispositionId") != transition.ContinuationDispositionID ||
					stamp.UTC().Format(time.RFC3339Nano) != transition.TransitionedAt {
					return errors.New("historical settlement approval projection is invalid")
				}
				registry.Entries[parentIndex].Status = domainsecurity.GrantRegistryRevoked
				registry.Entries[parentIndex].UpdatedAt = stamp.UTC().Format(time.RFC3339Nano)
			}
			registry.Entries = append(registry.Entries, domainsecurity.ExecutionGrantRegistryEntry{
				Version: domainsecurity.ExecutionGrantRegistryVersion, Sequence: registry.Sequence + 1, ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, ContextDigest: frozen.ContextDigest,
				Grant: grant, Status: status, ParentGrantID: parent, RegisteredAt: stamp.UTC().Format(time.RFC3339Nano), UpdatedAt: stamp.UTC().Format(time.RFC3339Nano),
			})
		case "tool_result":
			if entryIndex < 0 || (registry.Entries[entryIndex].Status != domainsecurity.GrantRegistryActive && registry.Entries[entryIndex].Status != domainsecurity.GrantRegistryPending) ||
				settlementStringField(item, "toolName") != registry.Entries[entryIndex].Grant.ToolName || settlementStringField(item, "callId") != registry.Entries[entryIndex].Grant.ToolCallID {
				return errors.New("historical settlement prior result membership is invalid")
			}
			stampValue := settlementStringField(item, "finishedAt")
			if stampValue == "" {
				stampValue = settlementStringField(item, "createdAt")
			}
			stamp, err := time.Parse(time.RFC3339Nano, stampValue)
			if err != nil {
				return err
			}
			registry.Entries[entryIndex].Status = domainsecurity.GrantRegistrySettled
			registry.Entries[entryIndex].UpdatedAt = stamp.UTC().Format(time.RFC3339Nano)
		default:
			if entryIndex < 0 || (registry.Entries[entryIndex].Status != domainsecurity.GrantRegistryActive && registry.Entries[entryIndex].Status != domainsecurity.GrantRegistryPending) {
				return errors.New("historical settlement grant membership is invalid")
			}
		}
		registry = domainsecurity.SealExecutionGrantRegistry(registry)
		if err := domainsecurity.ValidateExecutionGrantRegistryForAudit(registry); err != nil {
			return err
		}
	}
	if !found {
		return errors.New("historical settlement result is missing from original grant prefix")
	}
	return nil
}
