package server

import (
	"context"
	"errors"
	"strings"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	toolsideeffect "analytix.local/runtime-go/internal/adapters/outbound/toolsideeffect"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	checkpointport "analytix.local/runtime-go/internal/ports/checkpointauthority"
)

const checkpointOperationSettlementTimeout = 15 * time.Second

func (h *runtimeServerHandler) executeWriteRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	return filestore.ExecuteWriteTextTool(h.mutationToolInput(ctx, pending, args))
}

func (h *runtimeServerHandler) executeEditRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	return filestore.ExecuteEditTextTool(h.mutationToolInput(ctx, pending, args))
}

func (h *runtimeServerHandler) executeMoveRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	return filestore.ExecuteMoveFileTool(h.mutationToolInput(ctx, pending, args))
}

func (h *runtimeServerHandler) executeNotebookEditRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	prepared, ok := toolsideeffect.Notebook(ctx)
	if !ok {
		return map[string]any{"code": "prepared_mutation_invalid", "error": "notebook_edit prepared owner authority is unavailable"}, true
	}
	return filestore.ExecutePreparedNotebookEditTool(h.mutationToolInput(ctx, pending, args), prepared)
}

func (h *runtimeServerHandler) executeDeleteRangeRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	return filestore.ExecuteDeleteRangeTool(h.mutationToolInput(ctx, pending, args))
}

func (h *runtimeServerHandler) executeDeleteSymbolRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	prepared, ok := toolsideeffect.DeleteSymbol(ctx)
	if !ok {
		return map[string]any{"code": "prepared_mutation_invalid", "error": "delete_symbol prepared owner authority is unavailable"}, true
	}
	return filestore.ExecutePreparedDeleteSymbolTool(h.mutationToolInput(ctx, pending, args), prepared)
}

func (h *runtimeServerHandler) mutationToolInput(ctx context.Context, pending runtimePendingToolCall, args map[string]any) filestore.MutationToolInput {
	operationService := checkpointapp.OperationService{
		Authority: h.checkpoints,
		Observer:  filestore.CheckpointOperationObserver{AllowWriteRoots: h.allowWriteRoots},
	}
	sourceCheckpointID := strings.TrimSpace(pending.WorkspaceCheckpointID)
	if sourceCheckpointID == "" {
		sourceCheckpointID = "host-call-" + checkpointapp.Hash(pending.ThreadID+"\x00"+pending.TurnID+"\x00"+pending.Call.ID)
	}
	checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID(sourceCheckpointID)
	return filestore.MutationToolInput{
		Context:       ctx,
		Workspace:     pending.Workspace,
		Args:          args,
		ArgumentsJSON: append([]byte(nil), pending.Call.Arguments...),
		ToolName:      pending.Call.Name, ProtectedReadDirs: h.protectedReadDirs,
		SandboxMode:         pending.SandboxMode,
		AllowWriteRoots:     h.allowWriteRoots,
		MutationAuthority:   h.mutationAuthority,
		AcquireMutation:     h.workspaceMutations.Acquire,
		CheckManagedTargets: h.checkManagedMutation,
		Checkpoint: filestore.MutationCheckpointHooks{
			BeginOperation: func(paths []filestore.MutationOperationPath) (filestore.MutationOperationDraft, error) {
				return operationService.Begin(ctx, checkpointapp.BeginOperationInput{
					SecurityContext: pending.SecurityContext, ExecutionGrant: pending.ExecutionGrant,
					CheckpointID: checkpointID, SourceWorkspaceCheckpointID: sourceCheckpointID,
					Workspace: pending.Workspace, ToolName: pending.Call.Name,
					ArgumentsJSON: pending.Call.Arguments, Paths: paths, CreatedAt: time.Now().UTC(),
				})
			},
			SettleOperation: func(draft filestore.MutationOperationDraft, mutationSucceeded bool) (string, error) {
				settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), checkpointOperationSettlementTimeout)
				defer cancel()
				terminal, err := operationService.Settle(settlementCtx, draft, mutationSucceeded, time.Now().UTC())
				if err != nil {
					return "", err
				}
				if terminal.Status == "completed" {
					err = h.recordRuntimeCheckpointCaptured(pending.ThreadID, pending.TurnID, checkpointID, sourceCheckpointID)
					if errors.Is(err, checkpointport.ErrNotFound) {
						err = nil
					}
				}
				return terminal.Status, err
			},
		},
	}
}

func runtimeFileOperationErrorOutput(err error) map[string]any {
	return filestore.MutationOperationErrorOutput(err)
}

func workspaceRelativePath(workspace string, path string) string {
	return filestore.WorkspaceRelativePath(workspace, path)
}
