package server

import (
	"context"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	generationapp "analytix.local/runtime-go/internal/app/documentgeneration"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
)

func (h *runtimeServerHandler) executeGenerateDocumentRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	unavailable := map[string]any{"code": "document_generation_unavailable", "error": "Document generation is unavailable."}
	if h.documentCodec == nil || h.turnSecurity.Identity == nil {
		return unavailable, true
	}
	principal, err := h.turnSecurity.Identity.ResolveCurrent(ctx)
	if err != nil {
		return unavailable, true
	}
	validate := func() error {
		return (runtimeObjectProjector{h}).ValidateCurrent(ctx, editingapp.ScopeAuthority{Principal: principal, ThreadID: pending.ThreadID, Workspace: pending.Workspace, Purpose: "discuss"})
	}
	if validate() != nil {
		return unavailable, true
	}
	input := h.mutationToolInput(ctx, pending, args, principal.PrincipalDigest)
	input.ValidateCreationIdentity = validate
	return filestore.ExecuteGenerateDocumentTool(input, h.documentCodec)
}

func (h *runtimeServerHandler) ResolveGeneratedArtifact(ctx context.Context, threadID, artifactID string) (generationapp.Resolved, error) {
	if h == nil {
		return generationapp.Resolved{}, generationapp.ErrArtifactUnavailable
	}
	resolver := generationapp.Resolver{Identity: h.turnSecurity.Identity, Inventory: h.checkpoints,
		Files: filestore.GeneratedArtifactFiles{AllowWriteRoots: h.allowWriteRoots, ProtectedReadDirs: h.protectedReadDirs},
		ValidateAccess: func(ctx context.Context, principal identitydomain.PrincipalV1, thread, workspace string) error {
			return (runtimeObjectProjector{h}).ValidateCurrent(ctx, editingapp.ScopeAuthority{Principal: principal, ThreadID: thread, Workspace: workspace, Purpose: "discuss"})
		}}
	return resolver.Resolve(ctx, threadID, artifactID)
}
