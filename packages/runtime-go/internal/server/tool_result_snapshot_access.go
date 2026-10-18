package server

import (
	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	"context"
	"net/http"
	"reflect"
	"strings"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	snapshotapp "analytix.local/runtime-go/internal/app/toolresultsnapshot"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainprocess "analytix.local/runtime-go/internal/domain/processdisplay"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
)

type protectedToolSnapshotSettlementV1 struct {
	Context    context.Context
	Binding    domaintoolresult.ProtectedSnapshotBindingV1
	CreatedAt  string
	FinishedAt string
}
type runtimeToolSnapshotAccessV1 struct{ handler *runtimeServerHandler }

func (a runtimeToolSnapshotAccessV1) AcquireCurrent(ctx context.Context, selector snapshotport.SelectorV1) (snapshotport.AuthorizedV1, func(), error) {
	h := a.handler
	if h == nil || h.store == nil || h.turnSecurity.Observer == nil || !snapshotapp.ValidateSelectorV1(selector) {
		return snapshotport.AuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil {
		return snapshotport.AuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	first, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, selector.ThreadID)
	if err != nil {
		return snapshotport.AuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	observation, err := h.turnSecurity.Observer.Observe(stringField(first.Thread, "workspace"))
	if err != nil {
		return snapshotport.AuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	release, err := h.runtimeSubagentState().AcquireSecurityScopeRead(ctx, selector.ThreadID, observation.WorkspaceRealPath, principal.TenantID, principal.UserID)
	if err != nil || release == nil {
		return snapshotport.AuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	current, err := a.observeCurrent(ctx, selector, principal)
	if err != nil || current.Workspace != observation.WorkspaceRealPath {
		release()
		return snapshotport.AuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	return current, release, nil
}

func (a runtimeToolSnapshotAccessV1) ValidateCurrent(ctx context.Context, expected snapshotport.AuthorizedV1) error {
	actual, err := a.observeCurrent(ctx, expected.Selector, expected.Principal)
	if err != nil || actual != expected {
		return snapshotport.ErrUnavailable
	}
	return nil
}

// Original immutable result/grant/context are authenticated separately from
// the latest authority. New turns/datasets never replace historical bytes.
func (a runtimeToolSnapshotAccessV1) observeCurrent(ctx context.Context, selector snapshotport.SelectorV1, principal domainidentity.PrincipalV1) (snapshotport.AuthorizedV1, error) {
	h := a.handler
	unavailable := snapshotport.AuthorizedV1{}
	if ctx == nil || ctx.Err() != nil || h == nil || h.store == nil || h.turnSecurity.Observer == nil ||
		turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, principal) != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, selector.ThreadID)
	if err != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	return a.observePrimaryResultCurrentV1(ctx, selector, principal, snapshot.Thread, true)
}

func (a runtimeToolSnapshotAccessV1) observePrimaryScopeCurrentV1(ctx context.Context, thread map[string]any, threadID, turnID string, principal domainidentity.PrincipalV1) (domainsecurity.TurnSecurityContext, error) {
	h := a.handler
	unavailable := domainsecurity.TurnSecurityContext{}
	if ctx == nil || ctx.Err() != nil || h == nil || h.store == nil || h.turnSecurity.Observer == nil ||
		stringField(thread, "id") != threadID || stringField(thread, "status") == "deleted" ||
		turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, principal) != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	frozen, err := turnapp.FrozenSecurityContextForTurn(thread, turnID)
	if err != nil || frozen.ThreadID != threadID || frozen.TurnID != turnID || frozen.TenantID != principal.TenantID || frozen.UserID != principal.UserID {
		return unavailable, snapshotport.ErrUnavailable
	}
	current, err := snapshotapp.CurrentOwnContextV1(thread, frozen)
	if err != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	workspaceObservation, err := h.turnSecurity.Observer.Observe(stringField(thread, "workspace"))
	if err != nil || workspaceObservation.WorkspaceRealPath != frozen.WorkspaceRealPath {
		return unavailable, snapshotport.ErrUnavailable
	}
	workspaceIdentity, workspaceOK := filestore.ResolveMutationIdentityPath(frozen.WorkspaceRealPath, frozen.WorkspaceRealPath)
	observedIdentity, observedOK := filestore.ResolveMutationIdentityPath(workspaceObservation.WorkspaceRealPath, workspaceObservation.WorkspaceRealPath)
	if !workspaceOK || !observedOK || workspaceIdentity != observedIdentity {
		return unavailable, snapshotport.ErrUnavailable
	}
	if turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{OperationContext: ctx, Identity: h.turnSecurity.Identity, Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority, SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: current, Workspace: frozen.WorkspaceRealPath}) != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	relation := stringField(thread, "relation")
	if h.toolSnapshotHistoryValidate != nil {
		if h.toolSnapshotHistoryValidate(ctx, thread, frozen) != nil {
			return unavailable, snapshotport.ErrUnavailable
		}
	} else if relation != "primary" || strings.TrimSpace(stringField(thread, "parentThreadId")) != "" || domainsecurity.TurnSecurityContextRequiresFinalEvidenceGate(frozen) {
		return unavailable, snapshotport.ErrUnavailable
	}
	return frozen, nil
}

func (a runtimeToolSnapshotAccessV1) observePrimaryResultCurrentV1(ctx context.Context, selector snapshotport.SelectorV1, principal domainidentity.PrincipalV1, thread map[string]any, eventTimes bool) (snapshotport.AuthorizedV1, error) {
	h := a.handler
	unavailable := snapshotport.AuthorizedV1{}
	frozen, err := a.observePrimaryScopeCurrentV1(ctx, thread, selector.ThreadID, selector.TurnID, principal)
	if err != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	var result map[string]any
	var call map[string]any
	turns, _ := thread["turns"].([]any)
	matchedTurn := false
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != selector.TurnID {
			continue
		}
		if matchedTurn {
			return unavailable, snapshotport.ErrUnavailable
		}
		matchedTurn = true
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "id") == selector.ResultItemID || (stringField(item, "kind") == "tool_result" && stringField(item, "callId") == selector.CallID) {
				if result != nil || stringField(item, "id") != selector.ResultItemID || stringField(item, "kind") != "tool_result" {
					return unavailable, snapshotport.ErrUnavailable
				}
				result = item
			}
			if stringField(item, "kind") == "tool_call" && stringField(item, "callId") == selector.CallID {
				if call != nil {
					return unavailable, snapshotport.ErrUnavailable
				}
				call = item
			}
		}
	}
	if result == nil || call == nil || stringField(result, "threadId") != selector.ThreadID || stringField(result, "turnId") != selector.TurnID || stringField(result, "callId") != selector.CallID ||
		stringField(result, "toolName") != stringField(call, "toolName") || stringField(call, "threadId") != selector.ThreadID || stringField(call, "turnId") != selector.TurnID {
		return unavailable, snapshotport.ErrUnavailable
	}
	projected, closed := domaintoolresult.PrivateDurableToolResultItemRecordV1(result)
	if !closed || !reflect.DeepEqual(contracts.CloneMap(projected), contracts.CloneMap(result)) {
		return unavailable, snapshotport.ErrUnavailable
	}
	binding, err := domaintoolresult.ParseProtectedSnapshotBindingV1(result[domaintoolresult.ProtectedSnapshotBindingFieldV1])
	if err != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	_, wouldChange, exactErr := turnapp.EnsureOrdinaryToolSettlementExact(turnapp.OrdinaryToolSettlementInput{
		Thread: thread, ThreadID: selector.ThreadID, TurnID: selector.TurnID,
		ToolCallItemID: stringField(call, "id"), CallID: selector.CallID, ToolName: stringField(result, "toolName"),
		Status: stringField(result, "status"), Timestamp: stringField(result, "finishedAt"), ResultItem: result,
	})
	if exactErr != nil || wouldChange {
		return unavailable, snapshotport.ErrUnavailable
	}
	registry, err := executiongrantapp.RegistryFromThread(selector.ThreadID, thread, selector.TurnID)
	if err != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, stringField(result, "executionGrantId"))
	if !found || entry.Status != domainsecurity.GrantRegistrySettled || entry.Grant.ContextDigest != frozen.ContextDigest || entry.Grant.ToolCallID != selector.CallID || entry.Grant.ToolName != stringField(result, "toolName") ||
		executiongrantapp.VerifyThreadGrantMembership(selector.ThreadID, thread, selector.TurnID, entry.Grant, domainsecurity.GrantRegistrySettled) != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	if stringField(result, "contextDigest") != frozen.ContextDigest || projected["contextEpoch"] != frozen.ContextEpoch {
		return unavailable, snapshotport.ErrUnavailable
	}
	if turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, principal) != nil || ctx.Err() != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	var callAt, resultAt string
	if eventTimes {
		callAt, resultAt = h.toolInspectEventTimesV1(ctx, selector, stringField(call, "id"), entry.Grant.ToolName)
	}
	return snapshotport.AuthorizedV1{Selector: selector, Principal: principal, Workspace: frozen.WorkspaceRealPath, ToolName: entry.Grant.ToolName, ContextDigest: frozen.ContextDigest, ContextEpoch: frozen.ContextEpoch, ExecutionGrantID: entry.Grant.GrantID, CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash, Binding: binding, CallEventAt: callAt, ResultEventAt: resultAt}, nil
}

func (a runtimeToolSnapshotAccessV1) AcquirePresentHistoryCurrentV1(ctx context.Context, selector snapshotport.PresentHistorySelectorV1) (snapshotport.PresentHistoryAuthorizedV1, func(), error) {
	h := a.handler
	if ctx == nil || ctx.Err() != nil || h == nil || h.store == nil || h.turnSecurity.Observer == nil || !snapshotapp.ValidatePresentHistorySelectorV1(selector) {
		return snapshotport.PresentHistoryAuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil {
		return snapshotport.PresentHistoryAuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	first, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, selector.ThreadID)
	if err != nil {
		return snapshotport.PresentHistoryAuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	observation, err := h.turnSecurity.Observer.Observe(stringField(first.Thread, "workspace"))
	if err != nil {
		return snapshotport.PresentHistoryAuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	release, err := h.runtimeSubagentState().AcquireSecurityScopeRead(ctx, selector.ThreadID, observation.WorkspaceRealPath, principal.TenantID, principal.UserID)
	if err != nil || release == nil {
		if release != nil {
			release()
		}
		return snapshotport.PresentHistoryAuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	current, err := a.observePresentHistoryCurrentV1(ctx, selector, principal)
	if err != nil || current.Workspace != observation.WorkspaceRealPath {
		release()
		return snapshotport.PresentHistoryAuthorizedV1{}, nil, snapshotport.ErrUnavailable
	}
	return current, release, nil
}

func (a runtimeToolSnapshotAccessV1) ValidatePresentHistoryCurrentV1(ctx context.Context, expected snapshotport.PresentHistoryAuthorizedV1) error {
	actual, err := a.observePresentHistoryCurrentV1(ctx, expected.Selector, expected.Principal)
	if err != nil || !reflect.DeepEqual(actual, expected) {
		return snapshotport.ErrUnavailable
	}
	return nil
}

func (a runtimeToolSnapshotAccessV1) observePresentHistoryCurrentV1(ctx context.Context, selector snapshotport.PresentHistorySelectorV1, principal domainidentity.PrincipalV1) (snapshotport.PresentHistoryAuthorizedV1, error) {
	unavailable := snapshotport.PresentHistoryAuthorizedV1{}
	h := a.handler
	if ctx == nil || ctx.Err() != nil || h == nil || h.store == nil || h.publicProjector == nil || !snapshotapp.ValidatePresentHistorySelectorV1(selector) {
		return unavailable, snapshotport.ErrUnavailable
	}
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, selector.ThreadID)
	if err != nil || !domainsecurity.IsSHA256Hex(primary.ThreadFileSHA256) {
		return unavailable, snapshotport.ErrUnavailable
	}
	frozen, err := a.observePrimaryScopeCurrentV1(ctx, primary.Thread, selector.ThreadID, selector.TurnID, principal)
	if err != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	public, err := h.publicProjector.ProjectThread(primary.Thread)
	if err != nil || stringField(public, "id") != selector.ThreadID {
		return unavailable, snapshotport.ErrUnavailable
	}
	closing := map[string]bool{}
	publicTurns, ok := public["turns"].([]any)
	if !ok {
		return unavailable, snapshotport.ErrUnavailable
	}
	seenPublicTurn := false
	for _, raw := range publicTurns {
		turn, ok := raw.(map[string]any)
		if !ok {
			return unavailable, snapshotport.ErrUnavailable
		}
		if stringField(turn, "id") != selector.TurnID {
			continue
		}
		if seenPublicTurn {
			return unavailable, snapshotport.ErrUnavailable
		}
		seenPublicTurn = true
		items, ok := turn["items"].([]any)
		if !ok {
			return unavailable, snapshotport.ErrUnavailable
		}
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return unavailable, snapshotport.ErrUnavailable
			}
			if stringField(item, "kind") != "assistant_text" {
				continue
			}
			id := stringField(item, "id")
			if id == "" || closing[id] || stringField(item, "threadId") != selector.ThreadID || stringField(item, "turnId") != selector.TurnID ||
				stringField(item, "role") != "assistant" || stringField(item, "status") != "completed" {
				return unavailable, snapshotport.ErrUnavailable
			}
			// Empty public closings are not DSH reply anchors. The actual turn
			// end remains available from the existing public turn status.
			if strings.TrimSpace(stringField(item, "text")) != "" {
				closing[id] = true
			}
		}
	}
	if !seenPublicTurn {
		return unavailable, snapshotport.ErrUnavailable
	}
	ordered := []snapshotport.PresentHistoryItemV1{}
	turns, ok := primary.Thread["turns"].([]any)
	if !ok {
		return unavailable, snapshotport.ErrUnavailable
	}
	seenTurn := false
	for _, raw := range turns {
		turn, ok := raw.(map[string]any)
		if !ok {
			return unavailable, snapshotport.ErrUnavailable
		}
		if stringField(turn, "id") != selector.TurnID {
			continue
		}
		if seenTurn {
			return unavailable, snapshotport.ErrUnavailable
		}
		seenTurn = true
		items, ok := turn["items"].([]any)
		if !ok {
			return unavailable, snapshotport.ErrUnavailable
		}
		seen := map[string]bool{}
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return unavailable, snapshotport.ErrUnavailable
			}
			id, kind := stringField(item, "id"), stringField(item, "kind")
			if id == "" || seen[id] {
				return unavailable, snapshotport.ErrUnavailable
			}
			seen[id] = true
			if closing[id] {
				if kind != "assistant_text" || stringField(item, "threadId") != selector.ThreadID || stringField(item, "turnId") != selector.TurnID {
					return unavailable, snapshotport.ErrUnavailable
				}
				ordered = append(ordered, snapshotport.PresentHistoryItemV1{Kind: "assistant_text", ItemID: id})
				delete(closing, id)
				continue
			}
			if kind != "tool_result" || stringField(item, "toolName") != "present" {
				continue
			}
			// An unsealed result has no protected receipt to discover; it must
			// not suppress earlier real declarations in a stopped turn.
			if _, bound := item[domaintoolresult.ProtectedSnapshotBindingFieldV1]; !bound {
				continue
			}
			resultSelector := snapshotport.SelectorV1{ThreadID: selector.ThreadID, TurnID: selector.TurnID, CallID: stringField(item, "callId"), ResultItemID: id}
			if !snapshotapp.ValidateSelectorV1(resultSelector) {
				return unavailable, snapshotport.ErrUnavailable
			}
			if _, err := a.observePrimaryResultCurrentV1(ctx, resultSelector, principal, primary.Thread, false); err != nil {
				return unavailable, snapshotport.ErrUnavailable
			}
			ordered = append(ordered, snapshotport.PresentHistoryItemV1{Kind: "tool_result", ItemID: id, CallID: resultSelector.CallID, ToolName: "present"})
		}
	}
	if !seenTurn || len(closing) != 0 || ctx.Err() != nil || turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, principal) != nil {
		return unavailable, snapshotport.ErrUnavailable
	}
	return snapshotport.PresentHistoryAuthorizedV1{Selector: selector, Principal: principal, Workspace: frozen.WorkspaceRealPath, Original: frozen, ThreadFileSHA256: primary.ThreadFileSHA256, OrderedItems: ordered}, nil
}

func (h *runtimeServerHandler) toolInspectEventTimesV1(ctx context.Context, selector snapshotport.SelectorV1, callItemID, toolName string) (string, string) {
	loaded, err := h.store.LoadEventsSinceContext(ctx, selector.ThreadID, 0)
	if err != nil || len(loaded.Diagnostics) != 0 {
		return "", ""
	}
	var callAt, resultAt string
	callSeen, resultSeen := false, false
	for _, event := range loaded.Events {
		if stringField(event, "threadId") != selector.ThreadID || stringField(event, "turnId") != selector.TurnID {
			continue
		}
		at := stringField(event, "timestamp")
		switch stringField(event, "kind") {
		case "tool_call_ready":
			if stringField(event, "itemId") != callItemID || stringField(event, "callId") != selector.CallID || stringField(event, "toolName") != toolName {
				continue
			}
			if callSeen {
				return "", ""
			}
			callSeen, callAt = true, at
		case "tool_call_finished":
			item, _ := event["item"].(map[string]any)
			if stringField(event, "itemId") != selector.ResultItemID || stringField(item, "callId") != selector.CallID || stringField(item, "toolName") != toolName {
				continue
			}
			if resultSeen {
				return "", ""
			}
			resultSeen, resultAt = true, at
		}
	}
	return callAt, resultAt
}

func (h *runtimeServerHandler) sealProtectedToolSnapshotV1(ctx context.Context, pending runtimePendingToolCall, capture domaintoolresult.ProtectedCaptureV1, outputFacts ...domaintoolresult.ProtectedInspectV1) (protectedToolSnapshotSettlementV1, bool) {
	if ctx == nil || ctx.Err() != nil || h == nil || h.toolSnapshots == nil {
		return protectedToolSnapshotSettlementV1{}, false
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	frozen := pending.SecurityContext
	if err != nil || principal.TenantID != frozen.TenantID || principal.UserID != frozen.UserID || domainsecurity.ValidateTurnSecurityContext(frozen) != nil {
		return protectedToolSnapshotSettlementV1{}, false
	}
	if executiongrantapp.ArgumentsHash(pending.Call.Arguments) != pending.ExecutionGrant.ArgsHash {
		return protectedToolSnapshotSettlementV1{}, false
	}
	inspect := &domaintoolresult.ProtectedInspectV1{}
	if len(outputFacts) > 1 {
		return protectedToolSnapshotSettlementV1{}, false
	}
	if len(outputFacts) == 1 && outputFacts[0].OutputText != nil {
		if capture.Kind != "inspect" || capture.Status != "completed" || outputFacts[0].OutputError != nil || domaintoolresult.ValidateProtectedOutputTextV1(outputFacts[0].OutputText) != nil {
			return protectedToolSnapshotSettlementV1{}, false
		}
		text := *outputFacts[0].OutputText
		inspect.OutputText = &text
	}
	if len(outputFacts) == 1 && outputFacts[0].OutputError != nil {
		observed := outputFacts[0].OutputError
		if domaintoolresult.ValidateProtectedOutputErrorV1(observed) != nil {
			return protectedToolSnapshotSettlementV1{}, false
		}
		if observed.Availability == "available" {
			inspect.OutputError = domaintoolresult.ProjectProtectedOutputErrorV1(observed.Entries, domainprocess.KnownSecretsV1(ctx)...)
		} else {
			copy_ := *observed
			inspect.OutputError = &copy_
		}
	}
	// Reuse the existing protected JSON display budget; do not retain partial
	// input or add a spill/export authority when the original call exceeds it.
	if len(pending.Call.Arguments) > domaintoolresult.ProtectedWebSnapshotJSONLimitV1 {
		inspect.InputTruncated = true
	} else {
		inspect.InputJSON = string(pending.Call.Arguments)
	}
	if definition := pending.ModelVisibleToolDefinition; definition != nil && domainmodel.ValidateToolDefinitionSnapshotV1(definition, pending.Call.Name) == nil {
		inspect.SchemaJSON, inspect.SchemaTruncated = definition.SchemaJSON, definition.Truncated
	}
	if pending.Call.Name == "present" && capture.Kind == "present" {
		if !protectedPresentMatchesArgumentsV1(capture.Present, pending.Call.Arguments) {
			return protectedToolSnapshotSettlementV1{}, false
		}
	}
	if pending.Call.Name == "web_fetch" {
		if capture.Kind != "web_fetch" || capture.Web == nil || executiongrantapp.ArgumentsHash(pending.Call.Arguments) != pending.ExecutionGrant.ArgsHash {
			return protectedToolSnapshotSettlementV1{}, false
		}
		web := *capture.Web
		// Historical Web snapshots remain readable. New request facts have one
		// owner: the common Inspect observation, rather than a Web-only copy.
		web.InputJSON, web.SchemaJSON = "", ""
		web.InputTruncated, web.SchemaTruncated = false, false
		capture.Web = &web
	}
	if capture.Search != nil {
		search := *capture.Search
		search.InputJSON, search.InputTruncated = "", false
		capture.Search = &search
	}
	if capture.Kind == "file_result" {
		projected, ok := domaintoolresult.ProjectProtectedFileResultV1(capture.FileResult, domainprocess.KnownSecretsV1(ctx)...)
		if !ok {
			// Keep original request/schema facts when the result cannot safely be
			// retained; never turn redaction failure into an empty success result.
			capture = domaintoolresult.ProtectedCaptureV1{Kind: "inspect", Status: capture.Status}
		} else {
			capture.FileResult = projected
		}
	}
	envelope := domaintoolresult.ProtectedSnapshotV1{Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1, Principal: principal, Workspace: frozen.WorkspaceRealPath, ThreadID: pending.ThreadID, TurnID: pending.TurnID, CallID: pending.Call.ID, ResultItemID: domaintoolresult.ToolResultItemIDV1(pending.TurnID, pending.Call.ID), ToolName: pending.Call.Name, ContextDigest: frozen.ContextDigest, ContextEpoch: frozen.ContextEpoch, ExecutionGrantID: pending.ExecutionGrant.GrantID, CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash, Capture: capture, Inspect: inspect}
	if domaintoolresult.ValidateProtectedDisplaySnapshotV1(envelope) != nil {
		return protectedToolSnapshotSettlementV1{}, false
	}
	binding, err := h.toolSnapshots.Put(ctx, envelope)
	if err != nil {
		return protectedToolSnapshotSettlementV1{}, false
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	return protectedToolSnapshotSettlementV1{Context: ctx, Binding: binding, CreatedAt: at, FinishedAt: at}, true
}

// Composition-owned private handler; bearer and typed header remain at the
// existing local-display mux. It is not a generic public runtime request.
func (h *runtimeServerHandler) ToolResultLocalDisplayV1() http.Handler {
	return httpapi.ToolResultLocalDisplayHandlerV1{Service: &snapshotapp.Service{Store: h.toolSnapshots, Authority: runtimeToolSnapshotAccessV1{h}}}
}
