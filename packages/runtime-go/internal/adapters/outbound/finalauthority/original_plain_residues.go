package finalauthority

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// OriginalPlainResiduesV1 is derived only from complete native fixed-owner
// observations. No caller-supplied path or byte map can construct this proof.
// Its revalidation is purely physical and never observes a semantic journal.
type OriginalPlainResiduesV1 struct {
	dataRoot string
	owners   []*OriginalFixedOwnerObservationV1
	states   []privatecasport.OriginalPlainResidueV1
}

func FreezeOriginalPlainResiduesV1(ctx context.Context, owners []*OriginalFixedOwnerObservationV1) (_ *OriginalPlainResiduesV1, resultErr error) {
	if ctx == nil {
		return nil, errors.New("original plain residue context is required")
	}
	if ctx.Value(privateCASSemanticObservationContextKeyV1{}) != nil {
		release, err := privateCASRecoveryExclusion.acquireObservationV1(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
	}
	proof := &OriginalPlainResiduesV1{}
	seen := map[string]bool{}
	for _, owner := range owners {
		if owner == nil || owner.physical == nil || seen[owner.root] {
			return nil, errors.New("original plain residue owner is invalid or repeated")
		}
		seen[owner.root] = true
		dataRoot := filepath.Dir(filepath.Dir(owner.root))
		if filepath.Base(filepath.Dir(owner.root)) != "private" || (proof.dataRoot != "" && proof.dataRoot != dataRoot) {
			return nil, errors.New("original plain residue owner roots differ")
		}
		proof.dataRoot = dataRoot
		// Require the entire canonical leaf set, including absent leaves.
		wanted := map[string]bool{}
		for _, spec := range domainprivatecas.RuntimeRootSpecsV1() {
			if filepath.Dir(spec.RelativeCASRoot) == filepath.Base(owner.root) {
				wanted[filepath.Base(spec.RelativeCASRoot)] = true
			}
		}
		if len(wanted) == 0 || len(wanted) != len(owner.physical.leaves) {
			return nil, errors.New("original plain residue owner denominator is incomplete")
		}
		for _, leaf := range owner.physical.leaves {
			if !wanted[leaf.name] {
				return nil, errors.New("original plain residue leaf is not canonical")
			}
			delete(wanted, leaf.name)
		}
		files, err := owner.SnapshotOriginalFilesV1(ctx)
		if err != nil {
			return nil, err
		}
		ownerStates := []privatecasport.OriginalPlainResidueV1{}
		for name, entry := range files {
			parts := strings.Split(name, "/")
			if entry.Directory || len(parts) != 3 {
				continue
			}
			residue, found := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1])
			if !found || residue.Kind != domainprivatecas.ResidueOrdinaryWriteV1 {
				continue
			}
			ownerStates = append(ownerStates, privatecasport.OriginalPlainResidueV1{
				RelativePath: "private/" + filepath.Base(owner.root) + "/" + name,
				Mode:         entry.Mode, Size: int64(len(entry.Body)), SHA256: domainsecurity.SHA256Hex(entry.Body),
			})
		}
		// Empty owners do not pin unrelated healthy initialization or writes.
		if len(ownerStates) != 0 {
			proof.owners = append(proof.owners, owner)
			proof.states = append(proof.states, ownerStates...)
		}
	}
	sort.Slice(proof.states, func(i, j int) bool { return proof.states[i].RelativePath < proof.states[j].RelativePath })
	if len(proof.states) == 0 {
		return nil, context.Cause(ctx)
	}
	return proof, proof.Revalidate(ctx)
}

func (proof *OriginalPlainResiduesV1) DataRootV1() string { return proof.dataRoot }

func (proof *OriginalPlainResiduesV1) FileStatesV1() []privatecasport.OriginalPlainResidueV1 {
	return append([]privatecasport.OriginalPlainResidueV1(nil), proof.states...)
}

func (proof *OriginalPlainResiduesV1) Revalidate(ctx context.Context) error {
	if ctx == nil || proof == nil || proof.dataRoot == "" || len(proof.owners) == 0 || len(proof.states) == 0 {
		return errors.New("original plain residue observation is unavailable")
	}
	if ctx.Value(privateCASSemanticObservationContextKeyV1{}) != nil {
		release, err := privateCASRecoveryExclusion.acquireObservationV1(ctx)
		if err != nil {
			return err
		}
		defer release()
	}
	for _, owner := range proof.owners {
		if err := owner.RevalidatePhysicalV1(ctx); err != nil {
			return err
		}
	}
	return context.Cause(ctx)
}

// SelectOwnersV1 only narrows the original observation. It never recaptures
// names after cleanup or semantic writes have changed the physical state.
func (proof *OriginalPlainResiduesV1) SelectOwnersV1(ctx context.Context, names []string) (*OriginalPlainResiduesV1, error) {
	if err := proof.Revalidate(ctx); err != nil {
		return nil, err
	}
	selected := &OriginalPlainResiduesV1{dataRoot: proof.dataRoot}
	for _, owner := range proof.owners {
		name := "private/" + filepath.Base(owner.root)
		if !slices.Contains(names, name) {
			continue
		}
		selected.owners = append(selected.owners, owner)
		for _, state := range proof.states {
			if strings.HasPrefix(state.RelativePath, name+"/") {
				selected.states = append(selected.states, state)
			}
		}
	}
	if len(selected.states) == 0 {
		return nil, nil
	}
	sort.Slice(selected.states, func(i, j int) bool { return selected.states[i].RelativePath < selected.states[j].RelativePath })
	return selected, selected.Revalidate(ctx)
}

func (proof *OriginalPlainResiduesV1) ObserveCopiedOriginalPlainResiduesV1(ctx context.Context, dataRoot string, access privatecasport.RecoveryAccessAuthority) (_ privatecasport.OriginalPlainResiduesV1, resultErr error) {
	if err := proof.Revalidate(ctx); err != nil {
		return nil, err
	}
	if dataRoot == proof.dataRoot {
		return nil, errors.New("original plain residue copy requires an independent root")
	}
	defer func() { resultErr = errors.Join(resultErr, proof.Revalidate(ctx)) }()
	owners := []*OriginalFixedOwnerObservationV1{}
	for _, owner := range proof.owners {
		var creates *PreparedSecurePrivateCASOriginalCreateResiduesV1
		if owner.creates != nil {
			copied, err := owner.creates.ObserveCopiedOriginalCreateResiduesV1(ctx, dataRoot, access)
			if err != nil {
				return nil, err
			}
			var ok bool
			creates, ok = copied.(*PreparedSecurePrivateCASOriginalCreateResiduesV1)
			if !ok {
				return nil, errors.New("original plain residue copy creation producer differs")
			}
		}
		leaves := []SecurePrivateCASOwnerLeafV1{}
		for _, leaf := range owner.physical.leaves {
			leaves = append(leaves, SecurePrivateCASOwnerLeafV1{Name: leaf.name, MaxBytes: leaf.maxBytes})
		}
		copied, err := PrepareOriginalFixedOwnerObservationV1(ctx, filepath.Join(dataRoot, "private", filepath.Base(owner.root)), leaves, access, creates)
		if err != nil {
			return nil, err
		}
		owners = append(owners, copied)
	}
	copied, err := FreezeOriginalPlainResiduesV1(ctx, owners)
	if err != nil || copied == nil || !slices.Equal(proof.states, copied.states) {
		return nil, errors.Join(errors.New("original plain residue copy changed the exact files"), err)
	}
	return copied, copied.Revalidate(ctx)
}
