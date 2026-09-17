package filestore

import (
	"context"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
)

var _ checkpointfileport.GeneratedFileObserver = CheckpointOperationObserver{}

func (observer CheckpointOperationObserver) ObserveGenerated(ctx context.Context, workspace string, authority checkpointfileport.PathAuthority, resolvedPath string) domaincheckpoint.ObservedOperationPathV2 {
	base := observedCheckpointPath(authority)
	base.ObservationStatus, base.BlockerCode = "unavailable", "path_unsafe"
	if ctx == nil || ctx.Err() != nil || resolvedPath == "" || observer.validatePathAuthority(workspace, authority, resolvedPath) != nil ||
		CheckpointPathSafeForMutation(authority.Root, authority.RelativePath, resolvedPath) != nil {
		return base
	}
	current, err := inspectAtomicTextTargetWithPolicy(resolvedPath, true, codecport.MaxDocumentBytes, atomicTextReadPolicy{RequireSingleLink: true})
	if err != nil {
		base.BlockerCode = checkpointObservationBlocker(err)
		return base
	}
	// Recheck the frozen authority after reading; a replaced root cannot provide
	// an exact saved receipt for bytes read through its former pathname.
	if ctx.Err() != nil || observer.validatePathAuthority(workspace, authority, resolvedPath) != nil {
		return base
	}
	base.ObservationStatus, base.BlockerCode, base.Existed = "exact", "", current.Exists
	if current.Exists {
		base.Hash = checkpointapp.HashBytes(current.Content)
	}
	return base
}

func (observer CheckpointOperationObserver) ObserveGeneratedRelative(ctx context.Context, workspace string, authority checkpointfileport.PathAuthority) domaincheckpoint.ObservedOperationPathV2 {
	resolvedPath, err := observer.resolvePathAuthority(workspace, authority)
	if err != nil {
		base := observedCheckpointPath(authority)
		base.ObservationStatus, base.BlockerCode = "unavailable", "path_unsafe"
		return base
	}
	return observer.ObserveGenerated(ctx, workspace, authority, resolvedPath)
}
