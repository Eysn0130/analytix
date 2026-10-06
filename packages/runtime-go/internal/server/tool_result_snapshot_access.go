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
	thread := snapshot.Thread
	frozen, err := turnapp.FrozenSecurityContextForTurn(thread, selector.TurnID)
	if err != nil || frozen.ThreadID != selector.ThreadID || frozen.TurnID != selector.TurnID || frozen.TenantID != principal.TenantID || frozen.UserID != principal.UserID {
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
	return snapshotport.AuthorizedV1{Selector: selector, Principal: principal, Workspace: frozen.WorkspaceRealPath, ToolName: entry.Grant.ToolName, ContextDigest: frozen.ContextDigest, ContextEpoch: frozen.ContextEpoch, ExecutionGrantID: entry.Grant.GrantID, CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash, Binding: binding}, nil
}

func (h *runtimeServerHandler) sealProtectedToolSnapshotV1(ctx context.Context, pending runtimePendingToolCall, capture domaintoolresult.ProtectedCaptureV1) (protectedToolSnapshotSettlementV1, bool) {
	if h == nil || h.toolSnapshots == nil {
		return protectedToolSnapshotSettlementV1{}, false
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	frozen := pending.SecurityContext
	if err != nil || principal.TenantID != frozen.TenantID || principal.UserID != frozen.UserID || domainsecurity.ValidateTurnSecurityContext(frozen) != nil {
		return protectedToolSnapshotSettlementV1{}, false
	}
	envelope := domaintoolresult.ProtectedSnapshotV1{Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1, Principal: principal, Workspace: frozen.WorkspaceRealPath, ThreadID: pending.ThreadID, TurnID: pending.TurnID, CallID: pending.Call.ID, ResultItemID: domaintoolresult.ToolResultItemIDV1(pending.TurnID, pending.Call.ID), ToolName: pending.Call.Name, ContextDigest: frozen.ContextDigest, ContextEpoch: frozen.ContextEpoch, ExecutionGrantID: pending.ExecutionGrant.GrantID, CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash, Capture: capture}
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
