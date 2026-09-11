package finalauthority

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
)

// OriginalFixedOwnerObservationV1 observes a physical prefix of a fixed owner
// without exposing a writable owner. Only authenticated journal reconstruction
// can prove that an incomplete physical owner has complete Original/Final
// endpoints. Normal fixed-owner recovery constructors remain strict.
type OriginalFixedOwnerObservationV1 struct {
	physical *PreparedSecurePrivateCASOwnerRecoveryV1
	root     string
	creates  *PreparedSecurePrivateCASOriginalCreateResiduesV1
}

// FrozenOriginalFixedOwnerRecoveryV1 carries every actual present/absent leaf
// and the exact parent topology into recovery. It never authorizes cleanup or
// initialization of an incomplete original owner.
type FrozenOriginalFixedOwnerRecoveryV1 struct {
	original *OriginalFixedOwnerObservationV1
}

func (observation *OriginalFixedOwnerObservationV1) FreezeRecoveryV1(ctx context.Context) (*FrozenOriginalFixedOwnerRecoveryV1, error) {
	if err := observation.RevalidatePhysicalV1(ctx); err != nil {
		return nil, err
	}
	return &FrozenOriginalFixedOwnerRecoveryV1{original: observation}, nil
}

func (frozen *FrozenOriginalFixedOwnerRecoveryV1) Revalidate(ctx context.Context) error {
	if frozen == nil || frozen.original == nil {
		return errors.New("original fixed owner frozen recovery is unavailable")
	}
	return frozen.original.RevalidatePhysicalV1(ctx)
}

func (frozen *FrozenOriginalFixedOwnerRecoveryV1) ValidateSemantics(ctx context.Context) error {
	return frozen.Revalidate(ctx)
}

func (frozen *FrozenOriginalFixedOwnerRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*PreparedSecurePrivateCASRecoveryV1 {
	return frozen.original.physical.SecurePrivateCASRecoveryPlansV2()
}

func (frozen *FrozenOriginalFixedOwnerRecoveryV1) PrivateCASRecoveryTopologiesV3() []SecurePrivateCASRecoveryTopologyAuthorityV3 {
	return frozen.original.physical.PrivateCASRecoveryTopologiesV3()
}

func (frozen *FrozenOriginalFixedOwnerRecoveryV1) FreezeSecurePrivateCASRecoveryTargetsV4() bool {
	return true
}

func PrepareOriginalFixedOwnerObservationV1(ctx context.Context, root string, leaves []SecurePrivateCASOwnerLeafV1, access SecurePrivateCASRecoveryAccessAuthority, originals ...*PreparedSecurePrivateCASOriginalCreateResiduesV1) (*OriginalFixedOwnerObservationV1, error) {
	if ctx == nil || len(leaves) == 0 {
		return nil, errors.New("original fixed owner leaves are unavailable")
	}
	if ctx.Value(privateCASSemanticObservationContextKeyV1{}) != nil {
		release, err := privateCASRecoveryExclusion.acquireObservationV1(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
	}
	if len(originals) > 1 {
		return nil, errors.New("original fixed owner creation proof is ambiguous")
	}
	var creates *PreparedSecurePrivateCASOriginalCreateResiduesV1
	allowedCreates := map[string]bool{}
	if len(originals) == 1 && originals[0] != nil {
		creates = originals[0]
		if creates.OwnerRootV1() != root {
			return nil, errors.New("original fixed owner creation proof root differs")
		}
		if err := creates.Revalidate(ctx); err != nil {
			return nil, err
		}
		for _, relative := range creates.RelativePathsV1() {
			absolute := filepath.Join(creates.DataRootV1(), filepath.FromSlash(relative))
			if filepath.Dir(absolute) == root {
				allowedCreates[filepath.Base(absolute)] = true
			}
		}
	}
	names, known := []string{}, map[string]bool{}
	for _, leaf := range leaves {
		if known[leaf.Name] || leaf.MaxBytes <= 0 {
			return nil, errors.New("original fixed owner leaf is invalid")
		}
		known[leaf.Name] = true
		names = append(names, leaf.Name)
	}
	topology, entries, err := PrepareSecurePrivateCASOwnerDiscoveredMixedTopologyV2(ctx, root, names, len(leaves)+len(allowedCreates), 0, access)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if (!known[entry.Name] && !allowedCreates[entry.Name]) || !entry.Directory {
			return nil, errors.New("original fixed owner has an unknown physical child")
		}
	}
	physical := &PreparedSecurePrivateCASOwnerRecoveryV1{container: topology.container}
	for _, leaf := range leaves {
		var plan *PreparedSecurePrivateCASRecoveryV1
		var err error
		if creates != nil {
			// The complete proof was checked before discovery and is checked
			// after this batch; each constructor retains exact leaf binding.
			plan, err = prepareSecurePrivateCASRecoveryIfPresentV1(ctx, filepath.Join(root, leaf.Name), leaf.MaxBytes, access, creates)
		} else {
			plan, err = PrepareSecurePrivateCASRecoveryIfPresent(ctx, filepath.Join(root, leaf.Name), leaf.MaxBytes, access)
		}
		if err != nil {
			return nil, err
		}
		physical.leaves = append(physical.leaves, preparedSecurePrivateCASOwnerLeafV1{name: leaf.Name, maxBytes: leaf.MaxBytes, plan: plan})
	}
	observation := &OriginalFixedOwnerObservationV1{physical: physical, root: root, creates: creates}
	if err := observation.RevalidatePhysicalV1(ctx); err != nil {
		return nil, err
	}
	return observation, nil
}

func (observation *OriginalFixedOwnerObservationV1) RevalidatePhysicalV1(ctx context.Context) (resultErr error) {
	if ctx == nil || observation == nil || observation.physical == nil || observation.physical.container == nil || len(observation.physical.leaves) == 0 {
		return errors.New("original fixed owner observation is unavailable")
	}
	if ctx.Value(privateCASSemanticObservationContextKeyV1{}) != nil {
		release, err := privateCASRecoveryExclusion.acquireObservationV1(ctx)
		if err != nil {
			return err
		}
		defer release()
	}
	physical := observation.physical
	if observation.creates != nil {
		if err := observation.creates.Revalidate(ctx); err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, observation.creates.Revalidate(ctx)) }()
	}
	if err := physical.container.revalidate(ctx); err != nil {
		return err
	}
	for _, leaf := range physical.leaves {
		if leaf.plan == nil || leaf.plan.originalCreates != observation.creates {
			return errors.New("original fixed owner leaf creation binding changed")
		}
		if err := leaf.plan.revalidateOriginalLeafV1(ctx); err != nil {
			return err
		}
	}
	return errors.Join(physical.container.revalidate(ctx), ctx.Err())
}

func (observation *OriginalFixedOwnerObservationV1) SnapshotOriginalFilesV1(ctx context.Context) (map[string]SecurePrivateCASOriginalEntryV1, error) {
	if observation == nil || observation.physical == nil {
		return nil, errors.New("original fixed owner observation is unavailable")
	}
	entries, err := observation.physical.snapshotOriginalEntriesV1(ctx, observation.RevalidatePhysicalV1, observation.creates)
	if err != nil {
		return nil, err
	}
	if observation.creates != nil {
		for _, state := range observation.creates.DirectoryStatesV1() {
			absolute := filepath.Join(observation.creates.DataRootV1(), filepath.FromSlash(state.RelativePath))
			if !strings.HasPrefix(absolute, observation.root+string(filepath.Separator)) {
				continue
			}
			name := filepath.ToSlash(strings.TrimPrefix(absolute, observation.root+string(filepath.Separator)))
			entry := SecurePrivateCASOriginalEntryV1{Directory: true, Mode: state.Mode & 0o777}
			if old, exists := entries[name]; exists && (!old.Directory || old.Mode != entry.Mode || len(old.Body) != 0) {
				return nil, errors.New("original fixed owner creation metadata differs")
			}
			entries[name] = entry
		}
	}
	return entries, observation.RevalidatePhysicalV1(ctx)
}
