package persistencefs

import (
	"context"
	"errors"
	"path"
	"strings"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

type originalPlainResidueContextKeyV1 struct{}
type originalPlainResidueContextV1 struct {
	proof privatecasport.OriginalPlainResiduesV1
}

// WithOriginalPlainResiduesV1 binds a native physical proof to this observation
// scope, including nested authenticated-journal captures. Nil explicitly clears
// an earlier proof after healthy owners have completed normal cleanup.
func WithOriginalPlainResiduesV1(ctx context.Context, proof privatecasport.OriginalPlainResiduesV1) context.Context {
	return context.WithValue(ctx, originalPlainResidueContextKeyV1{}, originalPlainResidueContextV1{proof: proof})
}

func originalPlainSnapshotProofV1(ctx context.Context, roots RootSet) (privatecasport.OriginalPlainResiduesV1, map[string]privatecasport.OriginalPlainResidueV1, error) {
	bound, _ := ctx.Value(originalPlainResidueContextKeyV1{}).(originalPlainResidueContextV1)
	proof := bound.proof
	if proof == nil {
		return nil, nil, nil
	}
	if proof.DataRootV1() != roots.DataDir {
		return nil, nil, errors.New("original plain residue snapshot root differs")
	}
	if err := proof.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	states := map[string]privatecasport.OriginalPlainResidueV1{}
	for _, state := range proof.FileStatesV1() {
		label := "data/" + state.RelativePath
		classified := classifyPrivateAuthorityResidueLabel(label)
		if path.Clean(state.RelativePath) != state.RelativePath || strings.Contains(state.RelativePath, "\\") ||
			classified.state != privateAuthorityResidueKnown || classified.kind != domainprivatecas.ResidueOrdinaryWriteV1 ||
			state.Mode & ^uint32(0o777) != 0 || state.Size < 0 || !domainsecurity.IsSHA256Hex(state.SHA256) {
			return nil, nil, errors.New("original plain residue snapshot file is invalid")
		}
		if _, duplicate := states[label]; duplicate {
			return nil, nil, errors.New("original plain residue snapshot file is repeated")
		}
		states[label] = state
	}
	if len(states) == 0 {
		return nil, nil, errors.New("original plain residue snapshot proof is empty")
	}
	return proof, states, nil
}

func validateOriginalPlainSnapshotV1(snapshot RawSnapshot, states map[string]privatecasport.OriginalPlainResidueV1) error {
	remaining := len(states)
	for _, entry := range snapshot.Entries {
		if expected, found := states[entry.Path]; found {
			if entry.Type != "file" || entry.Mode != expected.Mode || entry.Size != expected.Size || entry.SHA256 != expected.SHA256 {
				return errors.New("original plain residue snapshot bytes or mode changed")
			}
			remaining--
		}
	}
	if remaining != 0 {
		return errors.New("original plain residue disappeared from snapshot")
	}
	return nil
}

func bindOriginalPlainStageContextV1(ctx context.Context, roots RootSet) (context.Context, func() error, error) {
	bound, _ := ctx.Value(originalPlainResidueContextKeyV1{}).(originalPlainResidueContextV1)
	if bound.proof == nil {
		return ctx, func() error { return nil }, nil
	}
	root, err := FreezeRootAuthority(roots)
	if err != nil {
		return nil, nil, err
	}
	access, err := NewSemanticStagePrivateCASAccessAuthority(roots, root)
	if err != nil {
		return nil, nil, err
	}
	proof, err := bound.proof.ObserveCopiedOriginalPlainResiduesV1(ctx, roots.DataDir, access)
	if err != nil {
		return nil, nil, errors.Join(err, access.Close())
	}
	return WithOriginalPlainResiduesV1(ctx, proof), access.Close, nil
}
