package server

import (
	"context"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

// The callback is created from the original admitted parent call. It observes
// the already-produced completion body once; it never executes a Job or reads
// another turn to reconstruct output. Storage remains the existing private CAS.
func (h *runtimeServerHandler) runtimeProtectedJobOutputProducerV1(pending runtimePendingToolCall) func(context.Context, domainjob.Record, string, bool) *domainjob.ProtectedOutputSnapshotV1 {
	return func(ctx context.Context, record domainjob.Record, body string, producerTruncated bool) *domainjob.ProtectedOutputSnapshotV1 {
		if h == nil || ctx == nil || ctx.Err() != nil || h.toolSnapshots == nil || record.ProtectedOutputSnapshot != nil ||
			!subagentapp.RecordMatchesSecurityContext(record, pending.SecurityContext) ||
			pending.ExecutionGrant.GrantID != record.SecurityBinding.ParentExecutionGrantID || pending.ExecutionGrant.ToolName != pending.Call.Name ||
			record.ParentToolCallID != pending.Call.ID || record.ParentToolItemID != pending.ToolCallItemID ||
			h.runtimeJobSecurityAuthorizer().Blocker(record) != "" || !h.runtimeProtectedJobProducerIdentityV1(ctx, record) {
			return nil
		}
		binding := record.SecurityBinding
		release, err := h.runtimeSubagentState().AcquireSecurityScopeRead(ctx, record.ParentThreadID, binding.ParentWorkspaceRealPath, binding.TenantID, binding.UserID)
		if err != nil || release == nil {
			return nil
		}
		defer release()
		if !h.runtimeProtectedJobScopeCurrentV1(ctx, record) {
			return nil
		}
		bounded, cut := domaintoolresult.BoundProtectedTextV1(body, domaintoolresult.ProtectedSnapshotBodyLimitV1)
		truncated := cut || producerTruncated
		capture, ok := subagentapp.TaskJobResultCaptureV1("output", subagentapp.TaskJobServiceResult{
			Record: record, OutputCapture: &domaintoolresult.ProtectedJobOutputV1{Availability: "available", Body: &bounded, Truncated: &truncated},
		})
		if !ok {
			return nil
		}
		sealed, ok := h.sealProtectedJobOutputSourceV1(ctx, pending, capture)
		if !ok || !h.runtimeProtectedJobScopeCurrentV1(ctx, record) {
			return nil
		}
		return &domainjob.ProtectedOutputSnapshotV1{Binding: sealed, ChildThreadID: record.ChildThreadID, ChildTurnID: record.ChildTurnID}
	}
}

// The CAS source is not a display result. The query later seals a separate
// bash_output result after checking this original producer envelope. Normal
// settlement and local-display readers keep their original tool/capture gate.
func (h *runtimeServerHandler) sealProtectedJobOutputSourceV1(ctx context.Context, pending runtimePendingToolCall, capture domaintoolresult.ProtectedCaptureV1) (domaintoolresult.ProtectedSnapshotBindingV1, bool) {
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	frozen := pending.SecurityContext
	if err != nil || principal.TenantID != frozen.TenantID || principal.UserID != frozen.UserID ||
		domainsecurity.ValidateTurnSecurityContext(frozen) != nil || executiongrantapp.ArgumentsHash(pending.Call.Arguments) != pending.ExecutionGrant.ArgsHash {
		return domaintoolresult.ProtectedSnapshotBindingV1{}, false
	}
	inspect := &domaintoolresult.ProtectedInspectV1{}
	if len(pending.Call.Arguments) > domaintoolresult.ProtectedWebSnapshotJSONLimitV1 {
		inspect.InputTruncated = true
	} else {
		inspect.InputJSON = string(pending.Call.Arguments)
	}
	if definition := pending.ModelVisibleToolDefinition; definition != nil && domainmodel.ValidateToolDefinitionSnapshotV1(definition, pending.Call.Name) == nil {
		inspect.SchemaJSON, inspect.SchemaTruncated = definition.SchemaJSON, definition.Truncated
	}
	envelope := domaintoolresult.ProtectedSnapshotV1{
		Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1, Principal: principal,
		Workspace: frozen.WorkspaceRealPath, ThreadID: pending.ThreadID, TurnID: pending.TurnID,
		CallID: pending.Call.ID, ResultItemID: domaintoolresult.ToolResultItemIDV1(pending.TurnID, pending.Call.ID), ToolName: pending.Call.Name,
		ContextDigest: frozen.ContextDigest, ContextEpoch: frozen.ContextEpoch, ExecutionGrantID: pending.ExecutionGrant.GrantID,
		CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash, Capture: capture, Inspect: inspect,
	}
	binding, err := h.toolSnapshots.Put(ctx, envelope)
	return binding, err == nil
}

func (h *runtimeServerHandler) runtimeProtectedJobScopeCurrentV1(ctx context.Context, record domainjob.Record) bool {
	if h == nil || h.store == nil || ctx == nil || ctx.Err() != nil || h.runtimeJobSecurityAuthorizer().Blocker(record) != "" {
		return false
	}
	thread, err := h.store.GetThread(record.ParentThreadID)
	if err != nil {
		return false
	}
	frozen, err := turnapp.FrozenSecurityContextForTurn(thread, record.ParentTurnID)
	if err != nil || !subagentapp.RecordMatchesSecurityContext(record, frozen) {
		return false
	}
	return turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{
		OperationContext: ctx, Identity: h.turnSecurity.Identity, Observer: h.turnSecurity.Observer,
		RiskAuthority: h.turnSecurity.RiskAuthority, SnapshotAuthority: h.turnSecurity.SnapshotAuthority,
		SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: frozen, Workspace: frozen.WorkspaceRealPath,
	}) == nil
}

func (h *runtimeServerHandler) runtimeProtectedJobProducerIdentityV1(ctx context.Context, record domainjob.Record) bool {
	if !domainjob.TerminalStatusV1(record.Status) {
		return false
	}
	if record.Kind == "background-shell" {
		return record.ChildThreadID == "" && record.ChildTurnID == ""
	}
	if record.Kind != "subagent" || record.Status != string(domainjob.StatusCompleted) || h.pendingWork == nil {
		return false
	}
	inventory, err := h.pendingWork.TrustedInventoryV1(ctx)
	if err != nil {
		return false
	}
	if _, err := pendingworkapp.DeriveChildIdentityFloorsV1(inventory, []domainjob.Record{record}, nil); err != nil {
		return false
	}
	if record.ChildCompletionReceipt != nil {
		if h.childCompletions == nil {
			return false
		}
		if _, err := h.childCompletions.Rehydrate(ctx, record); err != nil {
			return false
		}
	}
	return true
}

// Called only inside the original parent's current scope read/effect lease.
// A private reference is untrusted metadata, not a read permission. Check both
// the exact producer identity and the complete existing snapshot envelope.
func (h *runtimeServerHandler) resolveRuntimeProtectedJobOutputV1(ctx context.Context, record domainjob.Record) (domaintoolresult.ProtectedJobOutputV1, bool) {
	unavailable := domaintoolresult.ProtectedJobOutputV1{}
	if h == nil || h.toolSnapshots == nil || record.ProtectedOutputSnapshot == nil ||
		domainjob.ValidateProtectedOutputSnapshotV1(record) != nil || !h.runtimeProtectedJobScopeCurrentV1(ctx, record) ||
		!h.runtimeProtectedJobProducerIdentityV1(ctx, record) {
		return unavailable, false
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil {
		return unavailable, false
	}
	snapshot, err := h.toolSnapshots.Read(ctx, record.ProtectedOutputSnapshot.Binding)
	binding := record.SecurityBinding
	thread, threadErr := h.store.GetThread(record.ParentThreadID)
	registry, registryErr := executiongrantapp.RegistryFromThread(record.ParentThreadID, thread, record.ParentTurnID)
	entry, member := domainsecurity.ExecutionGrantRegistryEntryByID(registry, binding.ParentExecutionGrantID)
	if err != nil || threadErr != nil || registryErr != nil || !member || snapshot.ToolName != entry.Grant.ToolName || !domainidentity.SamePrincipalV1(snapshot.Principal, principal) ||
		principal.TenantID != binding.TenantID || principal.UserID != binding.UserID ||
		snapshot.ThreadID != record.ParentThreadID || snapshot.TurnID != record.ParentTurnID || snapshot.CallID != record.ParentToolCallID ||
		snapshot.ResultItemID != domaintoolresult.ToolResultItemIDV1(record.ParentTurnID, record.ParentToolCallID) ||
		snapshot.Workspace != binding.ParentWorkspaceRealPath || snapshot.ContextDigest != binding.ParentContextDigest || snapshot.ContextEpoch != binding.ParentContextEpoch ||
		snapshot.ExecutionGrantID != binding.ParentExecutionGrantID || snapshot.CaseID != binding.ParentCaseID || snapshot.CaseBindingHash != binding.ParentCaseBindingHash ||
		snapshot.Capture.Kind != "job" || snapshot.Capture.Job == nil || snapshot.Capture.Job.Operation != "output" || len(snapshot.Capture.Job.Jobs) != 1 {
		return unavailable, false
	}
	observed := snapshot.Capture.Job.Jobs[0]
	if observed.JobID != record.ID || observed.Kind != record.Kind || observed.Status != record.Status || observed.Output.Availability != "available" ||
		!h.runtimeProtectedJobScopeCurrentV1(ctx, record) || turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, principal) != nil || ctx.Err() != nil {
		return unavailable, false
	}
	// The CAS's strict decoder owns these pointers; the capture collector makes
	// the final defensive copy before the querying result is settled.
	output := observed.Output
	if output.Body == nil || output.Truncated == nil {
		return unavailable, false
	}
	return output, true
}
