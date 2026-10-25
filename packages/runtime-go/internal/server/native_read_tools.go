package server

import (
	"context"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const (
	runtimeReadModeWindow   = filetoolsapp.ReadModeWindow
	runtimeReadModeNumbered = filetoolsapp.ReadModeNumbered
	runtimeReadDefaultLimit = filetoolsapp.DefaultReadLimit
)

func executeReadRuntimeToolWithCapture(ctx context.Context, pending runtimePendingToolCall, args map[string]any, protectedReadDirs []string, readRoots []string, mode string) (map[string]any, string, bool) {
	return filestore.ExecuteReadTextTool(filestore.ReadTextToolInput{
		CaptureError: func(message string) {
			domaintoolresult.CaptureProtectedToolErrorV1(ctx, domaintoolresult.ProjectProtectedOutputErrorV1([]domaintoolresult.ProtectedOutputErrorEntryV1{{Message: message}}))
		},
		Capture: func(candidate domaintoolresult.ProtectedCaptureV1) {
			domaintoolresult.CaptureProtectedToolResultV1(ctx, candidate)
		},
		Workspace:         pending.Workspace,
		Args:              args,
		ProtectedReadDirs: protectedReadDirs,
		ReadRoots:         readRoots,
		Mode:              mode,
		SandboxMode:       pending.SandboxMode,
	})
}

func executeListRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any, protectedReadDirs []string, readRoots []string) (any, bool) {
	return filestore.ExecuteListTool(runtimeSearchToolInput(ctx, pending, args, protectedReadDirs, readRoots))
}

func executeFindRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any, protectedReadDirs []string, readRoots []string) (any, bool) {
	return filestore.ExecuteFindTool(runtimeSearchToolInput(ctx, pending, args, protectedReadDirs, readRoots))
}

func executeGlobRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any, protectedReadDirs []string, readRoots []string) (any, bool) {
	return filestore.ExecuteGlobTool(runtimeSearchToolInput(ctx, pending, args, protectedReadDirs, readRoots))
}

func executeCodeIndexRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any, protectedReadDirs []string, readRoots []string) (any, bool) {
	return filestore.ExecuteCodeIndexTool(runtimeSearchToolInput(ctx, pending, args, protectedReadDirs, readRoots))
}

func executeGrepRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any, protectedReadDirs []string, readRoots []string) (any, bool) {
	return filestore.ExecuteGrepTool(runtimeSearchToolInput(ctx, pending, args, protectedReadDirs, readRoots))
}

func runtimeSearchToolInput(ctx context.Context, pending runtimePendingToolCall, args map[string]any, protectedReadDirs []string, readRoots []string) filestore.WorkspaceSearchToolInput {
	return filestore.WorkspaceSearchToolInput{
		Context:           ctx,
		Workspace:         pending.Workspace,
		Args:              args,
		ProtectedReadDirs: protectedReadDirs,
		ReadRoots:         readRoots,
		SandboxMode:       pending.SandboxMode,
	}
}
