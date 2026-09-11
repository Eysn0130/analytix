package server

import (
	"context"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
)

const (
	runtimeReadModeWindow   = filetoolsapp.ReadModeWindow
	runtimeReadModeNumbered = filetoolsapp.ReadModeNumbered
	runtimeReadDefaultLimit = filetoolsapp.DefaultReadLimit
)

func executeReadRuntimeTool(pending runtimePendingToolCall, args map[string]any, protectedReadDirs []string, readRoots []string, mode string) (map[string]any, string, bool) {
	return filestore.ExecuteReadTextTool(filestore.ReadTextToolInput{
		Workspace:         pending.Workspace,
		Args:              args,
		ProtectedReadDirs: protectedReadDirs,
		ReadRoots:         readRoots,
		Mode:              mode,
		SandboxMode:       pending.SandboxMode,
	})
}

func executeListRuntimeTool(pending runtimePendingToolCall, args map[string]any, protectedReadDirs []string, readRoots []string) (any, bool) {
	return filestore.ExecuteListTool(runtimeSearchToolInput(nil, pending, args, protectedReadDirs, readRoots))
}

func executeFindRuntimeTool(pending runtimePendingToolCall, args map[string]any, protectedReadDirs []string, readRoots []string) (any, bool) {
	return filestore.ExecuteFindTool(runtimeSearchToolInput(nil, pending, args, protectedReadDirs, readRoots))
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
