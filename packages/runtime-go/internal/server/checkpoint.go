package server

import (
	"context"
	"strings"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
)

func (h *runtimeServerHandler) checkpointPlan(threadID, checkpointID, workspace, scope string) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if workspace == "" {
		workspace = h.dataDir
	}
	if checkpoint, ok := h.runtimeCheckpointMetadataFromPrivateRecords(threadID, checkpointID, workspace, now); ok {
		return h.checkpointPlanFromMetadata(threadID, checkpointID, workspace, scope, checkpoint)
	}
	return checkpointapp.BuildAuditOnlyPlan(checkpointapp.AuditPlanInput{
		ThreadID:     threadID,
		CheckpointID: checkpointID,
		Workspace:    workspace,
		Scope:        scope,
		CreatedAt:    now,
	})
}

type checkpointApplyFilePreflight = checkpointapp.ApplyFilePreflight

func (h *runtimeServerHandler) recordRuntimeCheckpointCaptured(threadID, turnID, checkpointID, sourceWorkspaceCheckpointID string) error {
	event, err := h.checkpoints.CapturedEvent(context.Background(), checkpointapp.CapturedAuthorityInput{
		ThreadID: threadID, TurnID: turnID, CheckpointID: checkpointID,
		SourceWorkspaceCheckpointID: sourceWorkspaceCheckpointID,
		WorkspaceFallback:           h.dataDir, CreatedAtFallback: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	return h.store.ReconcileCheckpointCapturedEvent(event)
}

func (h *runtimeServerHandler) runtimeCheckpointMetadataFromPrivateRecords(threadID, checkpointID, workspace, now string) (map[string]any, bool) {
	workspaceAuthorityPath, err := filestore.WorkspaceRealPath(workspace)
	if err != nil {
		return nil, false
	}
	return h.checkpoints.CapturedMetadata(context.Background(), checkpointapp.CapturedAuthorityInput{
		ThreadID: threadID, CheckpointID: checkpointID, AuthorityWorkspace: workspaceAuthorityPath,
		WorkspaceFallback: workspace, CreatedAtFallback: now,
	})
}

func (h *runtimeServerHandler) checkpointPlanFromMetadata(threadID, checkpointID, workspace, scope string, checkpoint map[string]any) map[string]any {
	createdAt := stringField(checkpoint, "createdAt")
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if metadataWorkspace := strings.TrimSpace(stringField(checkpoint, "workspace")); metadataWorkspace != "" {
		workspace = metadataWorkspace
	}
	if workspace == "" {
		workspace = h.dataDir
	}
	files := []any{}
	for _, raw := range listAny(checkpoint["changedFiles"]) {
		file, _ := raw.(map[string]any)
		if file == nil {
			continue
		}
		files = append(files, filestore.BuildCheckpointRestoreFilePlan(workspace, h.allowWriteRoots, file))
	}
	input := checkpointapp.PlanInput{
		ThreadID:     threadID,
		CheckpointID: checkpointID,
		Workspace:    workspace,
		Scope:        scope,
		CreatedAt:    createdAt,
		Checkpoint:   checkpoint,
		Files:        files,
	}
	if scope != "code" {
		input.Conversation = checkpointConversationPlanFromEvents(h, threadID, checkpointID)
	}
	return checkpointapp.BuildPlan(input)
}

func checkpointConversationPlanFromEvents(h *runtimeServerHandler, threadID, checkpointID string) map[string]any {
	result, err := h.store.LoadEventsSince(threadID, 0)
	if err != nil {
		return checkpointapp.ConversationPlanLoadError(err.Error())
	}
	return checkpointapp.ConversationPlanFromEvents(checkpointID, result.Events)
}

func (h *runtimeServerHandler) checkpointApply(ctx context.Context, threadID, checkpointID, workspace string, body map[string]any, plan map[string]any) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if workspace == "" {
		workspace = h.dataDir
	}
	clientPlan := plan
	planID := checkpointapp.PlanID(checkpointID)
	scope := checkpointapp.ApplyScope(clientPlan)
	base := func(status string, files []map[string]any, rescue map[string]any, auditSeq int) map[string]any {
		return checkpointapp.BuildApplyResponse(checkpointapp.ApplyResponseInput{
			ThreadID:     threadID,
			CheckpointID: checkpointID,
			Workspace:    workspace,
			PlanID:       planID,
			Scope:        scope,
			CreatedAt:    now,
			Status:       status,
			Files:        files,
			Plan:         plan,
			Rescue:       rescue,
			AuditSeq:     auditSeq,
		})
	}
	legacyBlocked := func(reason string) map[string]any {
		return checkpointapp.BuildBlockedApply(threadID, checkpointID, workspace, planID, scope, now, reason)
	}
	if !checkpointapp.ApplyConfirmationValid(body["confirmation"]) {
		return legacyBlocked("Go runtime default keeps checkpoint rewind audit-only unless an explicit checkpoint rewind confirmation is provided.")
	}
	if !checkpointapp.ValidScope(scope) {
		return legacyBlocked("checkpoint rewind scope must be code, conversation, or combined")
	}
	if mismatch := filestore.CheckpointApplyRouteMismatch(threadID, checkpointID, workspace, plan); mismatch != "" {
		return legacyBlocked(mismatch)
	}
	releaseEffect, authorityMismatch := checkpointapp.AcquireApplyAuthority(checkpointapp.ApplyAuthorityInput{
		Context: ctx, ThreadID: threadID, CheckpointID: checkpointID, Workspace: workspace,
		Threads: h.store, Snapshots: h.checkpoints, TurnSecurity: h.turnSecurity, CaseContexts: h.caseThreads,
		AcquireEffect: h.runtimeSubagentState().AcquireContextEffect, WorkspaceRealPath: filestore.WorkspaceRealPath,
	})
	if authorityMismatch != "" {
		return legacyBlocked(authorityMismatch)
	}
	defer releaseEffect()
	releaseMutation, mutationErr := h.workspaceMutations.Acquire(ctx)
	if mutationErr != nil {
		return legacyBlocked("checkpoint mutation coordinator is unavailable")
	}
	defer releaseMutation()
	authoritativePlan := h.checkpointPlan(threadID, checkpointID, workspace, scope)
	if mismatch := filestore.CheckpointApplyRouteMismatch(threadID, checkpointID, workspace, authoritativePlan); mismatch != "" {
		return legacyBlocked("host checkpoint plan is not bound to the current route")
	}
	if mismatch := checkpointapp.ApplyPlanHandleMismatch(clientPlan, authoritativePlan); mismatch != "" {
		return legacyBlocked(mismatch)
	}
	plan = authoritativePlan
	planID = stringField(authoritativePlan, "planId")
	scope = stringField(authoritativePlan, "scope")
	snapshots := h.runtimeCheckpointSnapshotEvidence(threadID, checkpointID)
	preflight := []checkpointApplyFilePreflight{}
	if scope != "conversation" {
		preflight = filestore.PreflightCheckpointApplyFiles(workspace, h.allowWriteRoots, plan, snapshots, h.gitStatusProbe)
	}
	fileResults := make([]map[string]any, 0, len(preflight))
	hasBlocked := false
	for _, file := range preflight {
		result := checkpointapp.ApplyFilePreflightResult(file)
		fileResults = append(fileResults, result)
		if file.Status == "blocked" || file.Status == "manual_review" {
			hasBlocked = true
		}
	}
	if hasBlocked {
		return base("blocked", fileResults, nil, 0)
	}
	mutating := make([]checkpointApplyFilePreflight, 0, len(preflight))
	for _, file := range preflight {
		if file.Status == "apply" {
			mutating = append(mutating, file)
		}
	}
	var rescue map[string]any
	if len(mutating) > 0 {
		privateRescue := filestore.CreateCheckpointRescueRecord(threadID, checkpointID, planID, workspace, now, mutating)
		if err := filestore.WriteCheckpointRescueRecord(h.dataDir, threadID, checkpointID, privateRescue); err != nil {
			return base("blocked", checkpointapp.BlockedApplyResults(preflight, "checkpoint rescue could not be durably recorded"), nil, 0)
		}
		rescueEvent := checkpointapp.BuildRescueCreatedEvent(threadID, privateRescue)
		if rescueEvent == nil {
			return base("blocked", checkpointapp.BlockedApplyResults(preflight, "checkpoint rescue identity could not be verified"), nil, 0)
		}
		recorded, _, err := h.store.RecordEvent(rescueEvent)
		if err != nil {
			return base("blocked", checkpointapp.BlockedApplyResults(preflight, "checkpoint rescue audit could not be durably recorded"), nil, 0)
		}
		rescueSeq, _ := numericSeq(recorded["seq"])
		var ok bool
		rescue, ok = checkpointapp.BuildRescueSummary(privateRescue, rescueSeq)
		if !ok {
			return base("blocked", checkpointapp.BlockedApplyResults(preflight, "checkpoint rescue summary could not be verified"), nil, 0)
		}
	}
	appliedFiles := make([]map[string]any, 0, len(preflight))
	for _, file := range preflight {
		if file.Status != "apply" {
			appliedFiles = append(appliedFiles, checkpointapp.ApplyFilePreflightResult(file))
			continue
		}
		if err := filestore.ApplyCheckpointFileMutationWithAuthority(file, h.mutationAuthority); err != nil {
			failed := checkpointapp.ApplyFilePreflightResult(file)
			failed["status"] = "failed"
			failed["reason"] = err.Error()
			appliedFiles = append(appliedFiles, failed)
			apply := base("failed", appliedFiles, rescue, 0)
			if event := checkpointapp.BuildRewindAppliedEvent(threadID, apply); event != nil {
				_, _, _ = h.store.RecordEvent(event)
			}
			return apply
		}
		result := checkpointapp.ApplyFilePreflightResult(file)
		result["status"] = "applied"
		result["reason"] = checkpointapp.AppliedReason(file.Action)
		appliedFiles = append(appliedFiles, result)
	}
	apply := base("applied", appliedFiles, rescue, 0)
	applyEvent := checkpointapp.BuildRewindAppliedEvent(threadID, apply)
	if applyEvent == nil {
		return base("failed", appliedFiles, rescue, 0)
	}
	event, _, err := h.store.RecordEvent(applyEvent)
	if err != nil {
		return base("failed", appliedFiles, rescue, 0)
	}
	apply = checkpointapp.ApplyResponseWithAuditEventSeq(apply, event)
	return apply
}

func (h *runtimeServerHandler) runtimeCheckpointSnapshotEvidence(threadID, checkpointID string) map[string]map[string]any {
	return h.checkpoints.Evidence(context.Background(), threadID, checkpointID)
}
