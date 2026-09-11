package executiongrant

import (
	"bytes"
	"encoding/json"
	"time"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

// ClosedReportRestartProjectionV1 preserves the already signed failure. It
// cannot turn a closed report into UNKNOWN or authorize publication effects.
func ClosedReportRestartProjectionV1(receipt domainpending.PendingWorkReceiptV1, disposition domainpending.PendingWorkDispositionV1) (domaintoolresult.PublicToolResultProjectionV1, bool) {
	if receipt.Kind != domainpending.KindReportStage || len(receipt.GrantMembers) != 1 || domainpending.ValidatePendingWorkDispositionForReceiptV1(disposition, receipt) != nil {
		return domaintoolresult.PublicToolResultProjectionV1{}, false
	}
	code := ""
	switch {
	case disposition.Status == domainpending.StatusFailed && disposition.ReasonCode == "report_stage_failed":
		code = "tool_failed"
	case disposition.Status == domainpending.StatusCancelled && disposition.ReasonCode == "report_stage_cancelled":
		code = "tool_cancelled"
	default:
		return domaintoolresult.PublicToolResultProjectionV1{}, false
	}
	return toolcatalogapp.BuildPublicToolResultProjectionV1("stage_case_report", map[string]any{"code": code}, true), true
}

// The root anchors an existing result separately. A newly added restart
// result must use the exact metadata-only projection derived above.
func ClosedReportRestartResultMatchesV1(result map[string]any, receipt domainpending.PendingWorkReceiptV1, disposition domainpending.PendingWorkDispositionV1, addedAtRestart bool) bool {
	expected, valid := ClosedReportRestartProjectionV1(receipt, disposition)
	if !valid || !restartBool(result, "isError") || result["hostEvidenceSettlement"] != nil || result["hostReportAdmission"] != nil {
		return false
	}
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(result["output"])
	if err != nil || projection == domaintoolresult.OutcomeUnknownAfterRestartProjectionV1() {
		return false
	}
	lifecycle, err := domaintoolresult.SettlementLifecycleStatusV1(projection, true)
	if err != nil || lifecycle != restartString(result, "status") {
		return false
	}
	if addedAtRestart {
		created := restartString(result, "createdAt")
		at, err := time.Parse(time.RFC3339Nano, created)
		disposed, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
		if err != nil || disposedErr != nil || at.UTC().Format(time.RFC3339Nano) != created || at.Before(disposed) || created != restartString(result, "finishedAt") || projection != expected {
			return false
		}
		rebuilt, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
			ThreadID: receipt.Context.ThreadID, TurnID: receipt.Context.TurnID, CreatedAt: created, FinishedAt: created,
			Call:       domainmodel.ToolCall{ID: restartString(result, "callId"), Name: "stage_case_report", Arguments: json.RawMessage(`{}`)},
			Projection: expected, IsError: true, ContextDigest: receipt.Context.ContextDigest, ContextEpoch: receipt.Context.ContextEpoch,
			ExecutionGrantID: receipt.GrantMembers[0].GrantID,
		})
		if err != nil {
			return false
		}
		actual, actualErr := json.Marshal(result)
		canonical, canonicalErr := json.Marshal(rebuilt.ResultItem)
		return actualErr == nil && canonicalErr == nil && bytes.Equal(actual, canonical)
	}
	return lifecycle == "failed"
}

func restartClosedReportSettlementMatches(threadID, turnID string, thread map[string]any, entry domainsecurity.ExecutionGrantRegistryEntry, authority RestartGrantOutcomeAuthorityV1) bool {
	resultID := toolcatalogapp.ToolResultItemID(turnID, entry.Grant.ToolCallID)
	durable, err := DurableSettlementFromThread(threadID, thread, turnID, resultID, entry.Grant)
	return err == nil && ClosedReportRestartResultMatchesV1(durable.ResultItem, authority.Receipt, authority.Disposition, false)
}
