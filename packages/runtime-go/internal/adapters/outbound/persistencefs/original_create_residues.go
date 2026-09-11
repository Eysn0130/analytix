package persistencefs

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func originalCreateSnapshotProofV1(ctx context.Context, roots RootSet, originals []privatecasport.OriginalCreateResiduesV1) (privatecasport.OriginalCreateResiduesV1, map[string]uint32, error) {
	if len(originals) > 1 {
		return nil, nil, errors.New("original creation snapshot proof is ambiguous")
	}
	if len(originals) == 0 || originals[0] == nil {
		return nil, nil, nil
	}
	proof := originals[0]
	if proof.DataRootV1() != roots.DataDir {
		return nil, nil, errors.New("original creation snapshot root differs")
	}
	ownerRoots := []string{proof.OwnerRootV1()}
	if set, ok := proof.(interface{ OwnerRootsV1() []string }); ok {
		ownerRoots = set.OwnerRootsV1()
	}
	if len(ownerRoots) == 0 || len(ownerRoots) > len(domainprivatecas.RecoverableOwnerDirectoryGroupsV1()) {
		return nil, nil, errors.New("original creation snapshot owner set is invalid")
	}
	owners := []string{}
	seenOwners := map[string]bool{}
	for _, ownerRoot := range ownerRoots {
		owner, err := filepath.Rel(roots.DataDir, ownerRoot)
		if err != nil {
			return nil, nil, err
		}
		owner = filepath.ToSlash(owner)
		known := false
		for _, group := range domainprivatecas.RecoverableOwnerDirectoryGroupsV1() {
			known = known || group.OwnerRelativePath == owner
		}
		if !known || seenOwners[owner] {
			return nil, nil, errors.New("original creation snapshot owner is unknown or duplicated")
		}
		seenOwners[owner] = true
		owners = append(owners, owner)
	}
	if err := proof.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	states := proof.DirectoryStatesV1()
	names := proof.RelativePathsV1()
	if len(states) != len(names) {
		return nil, nil, errors.New("original creation snapshot metadata is incomplete")
	}
	allowed := make(map[string]uint32, len(states))
	for index, state := range states {
		name := state.RelativePath
		label := "data/" + name
		classification := classifyPrivateAuthorityResidueLabel(label)
		inOwner := false
		for _, owner := range owners {
			inOwner = inOwner || strings.HasPrefix(name, owner+"/") || name == path.Join(path.Dir(owner), domainprivatecas.CreateDirectoryResidueNameV1(path.Base(owner)))
		}
		if names[index] != name || path.Clean(name) != name || strings.Contains(name, "\\") || !inOwner || classification.state != privateAuthorityResidueKnown || classification.kind != domainprivatecas.ResidueCreateDirectoryV1 || state.Mode&uint32(os.ModeDir) == 0 || state.Mode & ^(uint32(os.ModeDir)|0o777) != 0 {
			return nil, nil, errors.New("original creation snapshot directory is outside its exact owner")
		}
		if _, repeated := allowed[label]; repeated {
			return nil, nil, errors.New("original creation snapshot directory is duplicated")
		}
		allowed[label] = state.Mode
	}
	return proof, allowed, nil
}

// CaptureStrictWithOriginalCreateResiduesV1 changes only physical observation
// of the exact bound empty directories. Ordinary strict capture remains strict.
func CaptureStrictWithOriginalCreateResiduesV1(ctx context.Context, roots RootSet, proof privatecasport.OriginalCreateResiduesV1) (RawSnapshot, error) {
	return captureStrictWithScope(ctx, roots, nil, nil, true, proof)
}

func bindOriginalCreateStageProofV1(ctx context.Context, original privatecasport.OriginalCreateResiduesV1, stageRoots RootSet) (privatecasport.OriginalCreateResiduesV1, func() error, error) {
	if original == nil {
		return nil, func() error { return nil }, nil
	}
	root, err := FreezeRootAuthority(stageRoots)
	if err != nil {
		return nil, nil, err
	}
	access, err := NewSemanticStagePrivateCASAccessAuthority(stageRoots, root)
	if err != nil {
		return nil, nil, err
	}
	proof, err := original.ObserveCopiedOriginalCreateResiduesV1(ctx, stageRoots.DataDir, access)
	if err != nil {
		return nil, nil, errors.Join(err, access.Close())
	}
	return proof, access.Close, nil
}
