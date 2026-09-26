package server

import (
	"context"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	readapp "analytix.local/runtime-go/internal/app/workspaceread"
	readport "analytix.local/runtime-go/internal/ports/workspaceread"
)

type runtimeWorkspaceReadAuthority struct{ handler *runtimeServerHandler }

func (a runtimeWorkspaceReadAuthority) Current(ctx context.Context, threadID string) (readapp.Scope, error) {
	h := a.handler
	if h == nil || h.store == nil || h.turnSecurity.Identity == nil {
		return readapp.Scope{}, readport.ErrUnavailable
	}
	thread, err := h.store.GetThread(threadID)
	if err != nil || stringField(thread, "id") != threadID || stringField(thread, "relation") != "primary" {
		return readapp.Scope{}, readport.ErrUnavailable
	}
	principal, err := h.turnSecurity.Identity.ResolveCurrent(ctx)
	if err != nil {
		return readapp.Scope{}, readport.ErrUnavailable
	}
	workspace := stringField(thread, "workspace")
	scope := editingapp.ScopeAuthority{Principal: principal, ThreadID: threadID, Workspace: workspace,
		ObjectID: "workspace-retrieval", Path: workspace, Purpose: "discuss"}
	frozen, err := (runtimeObjectProjector{h}).FreezeSelectionAuthority(ctx, scope)
	if err != nil {
		return readapp.Scope{}, readport.ErrUnavailable
	}
	return readapp.Scope{ThreadID: threadID, Workspace: workspace, Binding: frozen}, nil
}
