package executiongrant

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	"analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const maxRestartGrantSettlements = 1024

type RestartSettlementStore interface {
	GetThread(string) (map[string]any, error)
	AppendItemToTurn(string, string, map[string]any) error
}

// RestartGrantOutcomesV1 is private host authority derived from exact signed
// pending-work receipts and dispositions. Unknown IDs or dispositions are
// rejected before any grant settlement is appended.
type RestartGrantOutcomeAuthorityV1 struct {
	Receipt     domainpendingwork.PendingWorkReceiptV1
	Disposition domainpendingwork.PendingWorkDispositionV1
}

type RestartGrantOutcomesV1 map[string]RestartGrantOutcomeAuthorityV1

// ReconcileOpenTurnGrantsOnRestart closes durable grants that were registered
// before a process crash but never received a terminal tool_result. The
// settlement is deliberately metadata-only: restart cannot resume the tool,
// infer success, issue evidence, or recover provider-private output.
func ReconcileOpenTurnGrantsOnRestart(
	store RestartSettlementStore,
	threadID, turnID string,
	at time.Time,
	outcomes RestartGrantOutcomesV1,
) (int, error) {
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	if store == nil || threadID == "" || turnID == "" {
		return 0, errors.New("restart execution grant reconciliation authority is unavailable")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	thread, err := store.GetThread(threadID)
	if err != nil {
		return 0, err
	}
	turn, ok := appmodel.TurnByID(thread, turnID)
	if !ok {
		return 0, errors.New("restart execution grant turn is unavailable")
	}
	registry, err := RegistryFromThread(threadID, thread, turnID)
	if err != nil {
		return 0, err
	}
	if err := validateRestartGrantOutcomes(threadID, turnID, thread, registry, outcomes); err != nil {
		return 0, err
	}
	plan, err := restartSettlementPlan(turn, registry)
	if err != nil {
		return 0, err
	}
	for _, planned := range plan {
		if !planned.Grant.ReadOnly && (planned.Grant.ApprovalState == "approved" || planned.Grant.ApprovalState == "not_required") {
			if _, found := outcomes[planned.Grant.GrantID]; !found {
				return 0, errors.New("writable restart grant lacks exact pending-work outcome authority")
			}
		}
	}
	settled := 0
	for _, planned := range plan {
		thread, err := store.GetThread(threadID)
		if err != nil {
			return settled, err
		}
		turn, ok := appmodel.TurnByID(thread, turnID)
		if !ok {
			return settled, errors.New("restart execution grant turn is unavailable")
		}
		registry, err := RegistryFromThread(threadID, thread, turnID)
		if err != nil {
			return settled, err
		}
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, planned.Grant.GrantID)
		if !found || entry != planned || (entry.Status != domainsecurity.GrantRegistryActive && entry.Status != domainsecurity.GrantRegistryPending) {
			return settled, errors.New("restart execution grant plan changed before settlement")
		}
		settledAt, err := restartSettlementTime(at, entry, settled)
		if err != nil {
			return settled, err
		}
		contextEpoch, err := restartTurnContextEpoch(turn)
		if err != nil {
			return settled, err
		}
		call := domainmodel.ToolCall{
			ID:        entry.Grant.ToolCallID,
			Name:      entry.Grant.ToolName,
			Arguments: json.RawMessage(`{}`),
		}
		projection := toolcatalogapp.BuildPublicToolResultProjectionV1(call.Name, map[string]any{"code": "tool_cancelled"}, true)
		if outcome, exists := outcomes[entry.Grant.GrantID]; exists {
			if outcome.Disposition.Status == domainpendingwork.StatusOutcomeUnknown {
				projection = domaintoolresult.OutcomeUnknownAfterRestartProjectionV1()
			} else {
				var valid bool
				projection, valid = ClosedReportRestartProjectionV1(outcome.Receipt, outcome.Disposition)
				if !valid {
					return settled, errors.New("closed report restart projection is unavailable")
				}
			}
		}
		records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
			ThreadID:         threadID,
			TurnID:           turnID,
			CreatedAt:        settledAt.Format(time.RFC3339Nano),
			FinishedAt:       settledAt.Format(time.RFC3339Nano),
			Call:             call,
			Projection:       projection,
			IsError:          true,
			ContextDigest:    entry.ContextDigest,
			ContextEpoch:     contextEpoch,
			ExecutionGrantID: entry.Grant.GrantID,
		})
		if err != nil {
			return settled, errors.New("restart execution grant result construction failed")
		}
		if records.ResultItemID == "" || restartItemIDExists(turn, records.ResultItemID) {
			return settled, errors.New("restart execution grant result identity is ambiguous")
		}
		if err := store.AppendItemToTurn(threadID, turnID, records.ResultItem); err != nil {
			return settled, err
		}
		reloaded, err := store.GetThread(threadID)
		if err != nil {
			return settled, err
		}
		if _, err := DurableSettlementFromThread(threadID, reloaded, turnID, records.ResultItemID, entry.Grant); err != nil {
			return settled, errors.New("restart execution grant settlement was not durably observed")
		}
		settled++
	}
	thread, err = store.GetThread(threadID)
	if err != nil {
		return settled, err
	}
	registry, err = RegistryFromThread(threadID, thread, turnID)
	if err != nil {
		return settled, err
	}
	for _, entry := range registry.Entries {
		if entry.Status == domainsecurity.GrantRegistryActive || entry.Status == domainsecurity.GrantRegistryPending {
			return settled, errors.New("restart execution grant reconciliation left an open member")
		}
	}
	if err := validateRestartGrantOutcomes(threadID, turnID, thread, registry, outcomes); err != nil {
		return settled, err
	}
	return settled, nil
}

func validateRestartGrantOutcomes(
	threadID, turnID string,
	thread map[string]any,
	registry domainsecurity.ExecutionGrantRegistry,
	outcomes RestartGrantOutcomesV1,
) error {
	turn, found := appmodel.TurnByID(thread, turnID)
	if !found {
		return errors.New("restart outcome-unknown turn is unavailable")
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil {
		return errors.New("restart outcome-unknown turn context is invalid")
	}
	expectedContext, err := domainpendingwork.ContextBindingFromSecurityContextV1(securityContext)
	if err != nil {
		return errors.New("restart outcome-unknown context binding is invalid")
	}
	for grantID, authority := range outcomes {
		grantID = strings.TrimSpace(grantID)
		receipt, disposition := authority.Receipt, authority.Disposition
		_, closedReport := ClosedReportRestartProjectionV1(receipt, disposition)
		writeKindValid := len(receipt.GrantMembers) == 1 &&
			((receipt.Kind == domainpendingwork.KindApprovedToolDispatch && receipt.GrantMembers[0].GrantID == grantID) ||
				(receipt.Kind == domainpendingwork.KindSideEffectIntent && receipt.GrantMembers[0].GrantID == grantID) ||
				(receipt.Kind == domainpendingwork.KindReportStage && receipt.GrantMembers[0].GrantID == grantID))
		if !domainsecurity.IsSHA256Hex(grantID) ||
			domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(disposition, receipt) != nil ||
			!writeKindValid || receipt.Context != expectedContext ||
			(disposition.Status != domainpendingwork.StatusOutcomeUnknown && !closedReport) ||
			len(receipt.GrantMembers) != 1 ||
			receipt.GrantMembers[0].GrantID != grantID {
			return errors.New("restart execution grant outcome authority is invalid")
		}
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grantID)
		toolKindValid := ((receipt.Kind == domainpendingwork.KindApprovedToolDispatch || receipt.Kind == domainpendingwork.KindSideEffectIntent) && entry.Grant.ToolName != "stage_case_report") ||
			(receipt.Kind == domainpendingwork.KindReportStage && entry.Grant.ToolName == "stage_case_report")
		approvalValid := entry.Grant.ApprovalState == "approved"
		if receipt.Kind == domainpendingwork.KindSideEffectIntent {
			approvalValid = entry.Grant.ApprovalState == "approved" || entry.Grant.ApprovalState == "not_required"
		}
		if !found || entry.ThreadID != threadID || entry.TurnID != turnID || entry.Grant.TurnID != turnID ||
			entry.Grant.ReadOnly || !approvalValid || !toolKindValid {
			return errors.New("restart outcome-unknown grant lacks exact write authority")
		}
		switch entry.Status {
		case domainsecurity.GrantRegistryActive:
			if !restartOutcomeReceiptMatchesRegistry(threadID, turnID, thread, receipt, entry.Grant) {
				return errors.New("restart outcome-unknown receipt registry authority is invalid")
			}
		case domainsecurity.GrantRegistrySettled:
			if (!closedReport && !restartOutcomeUnknownSettlementMatches(threadID, turnID, thread, entry)) ||
				(closedReport && !restartClosedReportSettlementMatches(threadID, turnID, thread, entry, authority)) {
				return errors.New("restart outcome-unknown grant has an incompatible durable settlement")
			}
			if !restartOutcomeReceiptMatchesRegistry(threadID, turnID, thread, receipt, entry.Grant) {
				return errors.New("restart outcome-unknown settled receipt registry authority is invalid")
			}
		default:
			return errors.New("restart outcome-unknown grant is not active or exactly settled")
		}
	}
	for _, raw := range restartItems(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok || restartString(item, "kind") != "tool_result" {
			continue
		}
		projection, projectionErr := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
		if projectionErr != nil || projection != domaintoolresult.OutcomeUnknownAfterRestartProjectionV1() {
			continue
		}
		grantID := restartString(item, "executionGrantId")
		if authority, found := outcomes[grantID]; !found || authority.Disposition.Status != domainpendingwork.StatusOutcomeUnknown {
			return errors.New("restart outcome-unknown result lost its exact pending-work authority")
		}
	}
	return nil
}

func restartOutcomeReceiptMatchesRegistry(
	threadID, turnID string,
	thread map[string]any,
	receipt domainpendingwork.PendingWorkReceiptV1,
	grant domainsecurity.ExecutionGrant,
) bool {
	if len(receipt.GrantMembers) != 1 {
		return false
	}
	prefix, err := RegistrySnapshotFromThread(
		threadID, thread, turnID, receipt.GrantRegistrySequence, receipt.GrantRegistryDigest,
	)
	if err != nil {
		return false
	}
	member := receipt.GrantMembers[0]
	if member.RegistrySequence == 0 || member.RegistrySequence > uint64(len(prefix.Entries)) {
		return false
	}
	entry := prefix.Entries[member.RegistrySequence-1]
	return entry.Grant == grant && entry.Grant.GrantID == member.GrantID && entry.EntryDigest == member.RegistryEntryDigest &&
		entry.Status == domainsecurity.GrantRegistryActive
}

func restartOutcomeUnknownSettlementMatches(
	threadID, turnID string,
	thread map[string]any,
	entry domainsecurity.ExecutionGrantRegistryEntry,
) bool {
	resultIDs := []string{
		toolcatalogapp.ToolResultItemID(turnID, entry.Grant.ToolCallID),
		legacyToolResultItemIDV0(turnID, entry.Grant.ToolCallID),
	}
	for index, resultID := range resultIDs {
		if resultID == "" || (index > 0 && resultID == resultIDs[0]) {
			continue
		}
		durable, err := DurableSettlementFromThread(threadID, thread, turnID, resultID, entry.Grant)
		if err != nil {
			continue
		}
		projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(durable.ResultItem["output"])
		if err == nil && projection == domaintoolresult.OutcomeUnknownAfterRestartProjectionV1() &&
			restartString(durable.ResultItem, "status") == "failed" && restartBool(durable.ResultItem, "isError") &&
			durable.ResultItem["hostEvidenceSettlement"] == nil {
			return true
		}
	}
	return false
}

// legacyToolResultItemIDV0 is read-only migration compatibility. New writes
// must use ToolResultItemIDV1 so provider call IDs and turn IDs are never
// copied into public record identities.
func legacyToolResultItemIDV0(turnID, callID string) string {
	return "item_result_" + turnID + "_" + contracts.SafeRecordID(callID)
}

func restartSettlementPlan(turn map[string]any, registry domainsecurity.ExecutionGrantRegistry) ([]domainsecurity.ExecutionGrantRegistryEntry, error) {
	plan := make([]domainsecurity.ExecutionGrantRegistryEntry, 0)
	for _, entry := range registry.Entries {
		if entry.Status != domainsecurity.GrantRegistryActive && entry.Status != domainsecurity.GrantRegistryPending {
			continue
		}
		plan = append(plan, entry)
	}
	if len(plan) > maxRestartGrantSettlements {
		return nil, errors.New("restart execution grant settlement limit exceeded")
	}
	items, ok := turn["items"].([]any)
	if !ok {
		return nil, errors.New("restart execution grant item inventory is invalid")
	}
	callIDs := map[string]bool{}
	resultIDs := map[string]bool{}
	for _, entry := range plan {
		callID := entry.Grant.ToolCallID
		resultID := toolcatalogapp.ToolResultItemID(entry.Grant.TurnID, callID)
		if !domainmodel.IsHostToolCallIDV1(callID) || resultID == "" {
			return nil, errors.New("restart execution grant has an unsafe legacy call identity")
		}
		if callIDs[callID] || resultIDs[resultID] {
			return nil, errors.New("restart execution grant call identity is duplicated")
		}
		callIDs[callID] = true
		resultIDs[resultID] = true
		callCount := 0
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("restart execution grant item inventory is invalid")
			}
			kind := restartString(item, "kind")
			if restartString(item, "id") == resultID ||
				(kind == "tool_result" && (restartString(item, "executionGrantId") == entry.Grant.GrantID || restartString(item, "callId") == callID)) {
				return nil, errors.New("restart execution grant has an unrecognized terminal result")
			}
			if kind != "tool_call" || restartString(item, "callId") != callID {
				continue
			}
			grant, err := domainsecurity.ParseExecutionGrant(item["executionGrant"])
			if err != nil || grant != entry.Grant || restartString(item, "executionGrantId") != entry.Grant.GrantID ||
				restartString(item, "toolName") != entry.Grant.ToolName {
				return nil, errors.New("restart execution grant call authority is invalid")
			}
			callCount++
		}
		if callCount != 1 {
			return nil, errors.New("restart execution grant call record is ambiguous")
		}
	}
	return plan, nil
}

func restartSettlementTime(at time.Time, entry domainsecurity.ExecutionGrantRegistryEntry, offset int) (time.Time, error) {
	registeredAt, err := time.Parse(time.RFC3339Nano, entry.RegisteredAt)
	if err != nil {
		return time.Time{}, errors.New("restart execution grant registration time is invalid")
	}
	at = at.UTC().Add(time.Duration(offset) * time.Nanosecond)
	if at.Before(registeredAt) {
		at = registeredAt
	}
	return at, nil
}

func restartTurnContextEpoch(turn map[string]any) (uint64, error) {
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil {
		return 0, errors.New("restart execution grant turn context is invalid")
	}
	return securityContext.ContextEpoch, nil
}

func restartItemIDExists(turn map[string]any, itemID string) bool {
	for _, raw := range restartItems(turn["items"]) {
		item, _ := raw.(map[string]any)
		if restartString(item, "id") == itemID {
			return true
		}
	}
	return false
}

func restartItems(value any) []any {
	items, _ := value.([]any)
	return items
}

func restartString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func restartBool(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}
