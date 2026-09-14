package server

import (
	"context"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	generationapp "analytix.local/runtime-go/internal/app/documentgeneration"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
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
	loaded, ok := ctx.Value(preparedHostedSkillKey{}).(hostapp.HostedSkill)
	if !ok || h.officePackageHost == nil {
		return unavailable, true
	}
	var output any
	var failed bool
	// Preserve the established Host -> workspace lock order. The callback's
	// identity checks do not re-enter the Host while the workspace is locked.
	err = h.officePackageHost.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), func(domainskill.PackageSnapshot) error {
		output, failed = filestore.ExecuteGenerateDocumentTool(input, h.documentCodec)
		return nil
	})
	if err != nil {
		return unavailable, true
	}
	return output, failed
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
