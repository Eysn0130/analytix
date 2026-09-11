package finalauthority

import (
	"context"
	"errors"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const (
	maxLargeOpaqueRecoveryCommittedObjects = uint64(1 << 24)
	maxLargeOpaqueRecoveryTemporaryObjects = uint64(1 << 20)
	maxLargeOpaqueRecoveryCreateResidues   = uint32(257)
)

// LargeOpaqueRecoveryLimitsV1 separates durable-object observation from
// uncommitted residue bounds. These are startup boundedness limits, not an
// artifact-level chunk limit and not a substitute for a write-side quota.
type LargeOpaqueRecoveryLimitsV1 struct {
	MaxCommittedObjects uint64
	MaxTemporaryObjects uint64
	MaxCreateResidues   uint32
}

// LargeOpaqueRecoveryReportV1 contains counts only. It deliberately omits
// addresses, paths, content hashes, and object identities and is never factual
// or snapshot authority.
type LargeOpaqueRecoveryReportV1 struct {
	Present                   bool
	ShardCount                uint32
	ObjectCount               uint64
	RemovedTempCount          uint64
	RemovedCreateResidueCount uint32
}

type largeOpaqueRecoveryBoundStateV1 struct {
	rootPath    string
	maxBytes    uint64
	limits      LargeOpaqueRecoveryLimitsV1
	access      privatecasport.RecoveryAccessAuthority
	binding     privatecasport.RootBinding
	authority   privateCASRootAuthority
	present     bool
	observation largeOpaqueRecoveryObservation
}

// PreparedLargeOpaqueRecoveryV1 freezes the private persistence binding and a
// constant-memory topology observation. V1 never deletes a path-named residue:
// the supported Unix platforms cannot bind unlink atomically to the already
// verified object identity, so any residue blocks clean-guard issuance. It
// never materializes blob bytes or an address inventory. A prepared value is
// not an activation guard.
type PreparedLargeOpaqueRecoveryV1 struct {
	state largeOpaqueRecoveryBoundStateV1
}

// LargeOpaqueCleanGuardV1 is issued only after cleanup reached a residue-free
// fixed point. Runtime startup must retain and revalidate it immediately before
// activation because the generic managed snapshot intentionally excludes the
// large-object root.
type LargeOpaqueCleanGuardV1 struct {
	state largeOpaqueRecoveryBoundStateV1
}

func PrepareLargeOpaqueStoreRecoveryV1(
	ctx context.Context,
	root string,
	maxBytes uint64,
	limits LargeOpaqueRecoveryLimitsV1,
	access privatecasport.RecoveryAccessAuthority,
) (*PreparedLargeOpaqueRecoveryV1, error) {
	if !largeOpaqueRecoveryPlatformSupported() || largeOpaqueNilInterface(access) ||
		maxBytes == 0 || maxBytes > maxLargeOpaqueObjectBytes ||
		limits.MaxCommittedObjects == 0 ||
		limits.MaxCommittedObjects > maxLargeOpaqueRecoveryCommittedObjects ||
		limits.MaxTemporaryObjects == 0 ||
		limits.MaxTemporaryObjects > maxLargeOpaqueRecoveryTemporaryObjects ||
		limits.MaxCreateResidues == 0 ||
		limits.MaxCreateResidues > maxLargeOpaqueRecoveryCreateResidues {
		return nil, errors.New("large opaque recovery configuration is invalid")
	}
	ctx = largeOpaqueContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rootPath, err := lexicalPrivateCASRootPath(root)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedLargeOpaqueRecoveryV1{state: largeOpaqueRecoveryBoundStateV1{
		rootPath: rootPath, maxBytes: maxBytes, limits: limits, access: access,
	}}
	err = withExistingPrivateCASAccess(ctx, access, rootPath, func(binding privatecasport.RootBinding) error {
		prepared.state.binding = binding
		authority, present, observation, err := largeOpaqueObserveRecoveryPlatform(
			ctx, binding, maxBytes, limits,
		)
		prepared.state.authority = authority
		prepared.state.present = present
		prepared.state.observation = observation
		return err
	})
	if err != nil {
		return nil, errors.Join(errors.New("prepare large opaque recovery"), err)
	}
	return prepared, nil
}

func (prepared *PreparedLargeOpaqueRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil {
		return errors.New("prepared large opaque recovery is unavailable")
	}
	return revalidateLargeOpaqueRecoveryStateV1(ctx, prepared.state)
}

func (guard *LargeOpaqueCleanGuardV1) Revalidate(ctx context.Context) error {
	if guard == nil {
		return errors.New("large opaque clean guard is unavailable")
	}
	return revalidateLargeOpaqueRecoveryStateV1(ctx, guard.state)
}

func revalidateLargeOpaqueRecoveryStateV1(
	ctx context.Context,
	state largeOpaqueRecoveryBoundStateV1,
) error {
	if largeOpaqueNilInterface(state.access) || state.binding == (privatecasport.RootBinding{}) {
		return errors.New("large opaque recovery state is unavailable")
	}
	ctx = largeOpaqueContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	return withExistingPrivateCASAccess(ctx, state.access, state.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != state.binding {
			return errors.New("large opaque recovery root binding changed")
		}
		authority, present, observation, err := largeOpaqueObserveRecoveryPlatform(
			ctx, binding, state.maxBytes, state.limits,
		)
		if err != nil {
			return err
		}
		if present != state.present || authority != state.authority || observation != state.observation {
			return errors.New("large opaque recovery topology changed after preflight")
		}
		return nil
	})
}

func (prepared *PreparedLargeOpaqueRecoveryV1) Apply(
	ctx context.Context,
) (LargeOpaqueRecoveryReportV1, *LargeOpaqueCleanGuardV1, error) {
	ctx = largeOpaqueContext(ctx)
	if err := privateCASRecoveryExclusion.acquireRecovery(ctx); err != nil {
		return LargeOpaqueRecoveryReportV1{}, nil, err
	}
	defer privateCASRecoveryExclusion.releaseRecovery()
	if err := prepared.Revalidate(ctx); err != nil {
		return LargeOpaqueRecoveryReportV1{}, nil, err
	}
	var report LargeOpaqueRecoveryReportV1
	var clean largeOpaqueRecoveryObservation
	err := withExistingPrivateCASAccess(
		ctx,
		prepared.state.access,
		prepared.state.rootPath,
		func(binding privatecasport.RootBinding) error {
			if binding != prepared.state.binding {
				return errors.New("large opaque recovery root binding changed before apply")
			}
			var err error
			report, clean, err = largeOpaqueApplyRecoveryPlatform(
				ctx,
				binding,
				prepared.state.authority,
				prepared.state.present,
				prepared.state.observation,
				prepared.state.maxBytes,
				prepared.state.limits,
			)
			return err
		},
	)
	if err != nil {
		return LargeOpaqueRecoveryReportV1{}, nil, errors.Join(errors.New("apply large opaque recovery"), err)
	}
	guard := &LargeOpaqueCleanGuardV1{state: prepared.state}
	guard.state.observation = clean
	if err := guard.Revalidate(ctx); err != nil {
		return LargeOpaqueRecoveryReportV1{}, nil, errors.Join(
			errors.New("large opaque clean guard did not survive issuance"),
			err,
		)
	}
	return report, guard, nil
}
