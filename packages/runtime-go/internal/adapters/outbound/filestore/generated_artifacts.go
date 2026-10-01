package filestore

import (
	"context"
	"errors"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
)

type GeneratedArtifactFiles struct {
	AllowWriteRoots   []string
	ProtectedReadDirs []string
}

func (f GeneratedArtifactFiles) InspectGenerated(ctx context.Context, workspace string, authority checkpointfileport.PathAuthority, kind, expectedHash string) (codecport.ResolvedArtifact, error) {
	invalid := errors.New("generated artifact file unavailable")
	observer := CheckpointOperationObserver{AllowWriteRoots: f.AllowWriteRoots}
	path, err := observer.resolvePathAuthority(workspace, authority)
	if ctx == nil || ctx.Err() != nil || err != nil || MandatoryProtectedPathOutput(workspace, path, f.ProtectedReadDirs) != nil {
		return codecport.ResolvedArtifact{}, invalid
	}
	current, err := inspectAtomicTextTargetWithPolicy(path, true, codecport.MaxDocumentBytes, atomicTextReadPolicy{RequireSingleLink: true})
	if err != nil || !current.Exists || InspectOfficePackage(current.Content, kind) != nil || observer.validatePathAuthority(workspace, authority, path) != nil || ctx.Err() != nil {
		return codecport.ResolvedArtifact{}, invalid
	}
	digest := checkpointapp.HashBytes(current.Content)
	return codecport.ResolvedArtifact{Path: path, Revision: digest, ByteSize: int64(len(current.Content)), Changed: digest != expectedHash}, nil
}
