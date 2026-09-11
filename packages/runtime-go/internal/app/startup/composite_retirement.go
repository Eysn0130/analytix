package startup

import (
	"context"
	"errors"
)

// OwnerObservationV1 is the application-layer projection of one owner-specific
// read-only inventory. Digest is host-derived from the complete owner view.
type OwnerObservationV1 struct {
	Digest           string
	RecoveryRequired bool
	Retired          bool
}

type FixedPointOwnerV1 interface {
	Observe(context.Context) (OwnerObservationV1, error)
	ValidateObservation(context.Context, OwnerObservationV1) error
}

type PreparedMonotonicRetirementV1 interface {
	Validate(context.Context) error
	Apply(context.Context) error
}

type MonotonicRetirementOwnerV1 interface {
	FixedPointOwnerV1
	Recover(context.Context) error
	Prepare(context.Context) (PreparedMonotonicRetirementV1, error)
}

// RunCompositeSemanticThenRetirementV2 keeps independent owner journals while
// enforcing one externally observable activation boundary. The retirement
// source is fully prepared before semantic migration may mutate the managed
// owners. The prepared retirement is revalidated afterward, and its apply must
// leave the post-semantic managed fixed point unchanged.
func RunCompositeSemanticThenRetirementV2(
	ctx context.Context,
	managedOwners []FixedPointOwnerV1,
	retirement MonotonicRetirementOwnerV1,
	semanticMigration func(context.Context) error,
) error {
	if retirement == nil || semanticMigration == nil {
		return errors.New("composite semantic retirement input is unavailable")
	}
	owners := append(append([]FixedPointOwnerV1(nil), managedOwners...), retirement)
	for _, owner := range owners {
		if owner == nil {
			return errors.New("composite semantic retirement owner is unavailable")
		}
	}
	observations, err := captureCompositeOwnerFixedPointV1(ctx, owners)
	if err != nil {
		return err
	}
	if err := validateCompositeOwnerFixedPointV1(ctx, owners, observations); err != nil {
		return err
	}
	retirementObservation := observations[len(observations)-1]
	if retirementObservation.RecoveryRequired {
		if err := retirement.Recover(ctx); err != nil {
			return err
		}
		if err := validateCompositeOwnerFixedPointV1(ctx, managedOwners, observations[:len(managedOwners)]); err != nil {
			return errors.New("retirement recovery changed a managed owner")
		}
		observations, err = captureCompositeOwnerFixedPointV1(ctx, owners)
		if err != nil {
			return err
		}
		if err := validateCompositeOwnerFixedPointV1(ctx, owners, observations); err != nil {
			return err
		}
		retirementObservation = observations[len(observations)-1]
		if retirementObservation.RecoveryRequired {
			return errors.New("retirement recovery did not settle the transaction")
		}
	}
	prepared, err := retirement.Prepare(ctx)
	if err != nil {
		return err
	}
	if prepared == nil {
		return errors.New("composite semantic retirement plan is unavailable")
	}
	if err := validateCompositeOwnerFixedPointV1(ctx, owners, observations); err != nil {
		return errors.New("retirement preparation changed the composite fixed point")
	}
	if err := prepared.Validate(ctx); err != nil {
		return err
	}
	if err := semanticMigration(ctx); err != nil {
		return err
	}
	if err := retirement.ValidateObservation(ctx, retirementObservation); err != nil {
		return errors.New("semantic migration changed the retirement owner")
	}
	if err := prepared.Validate(ctx); err != nil {
		return err
	}
	managedAfterSemantic, err := captureCompositeOwnerFixedPointV1(ctx, managedOwners)
	if err != nil {
		return err
	}
	if err := validateCompositeOwnerFixedPointV1(ctx, managedOwners, managedAfterSemantic); err != nil {
		return err
	}
	if err := prepared.Apply(ctx); err != nil {
		return err
	}
	if err := validateCompositeOwnerFixedPointV1(ctx, managedOwners, managedAfterSemantic); err != nil {
		return errors.New("retirement apply changed a managed owner")
	}
	settled, err := retirement.Observe(ctx)
	if err != nil {
		return err
	}
	if settled.Digest == "" || settled.RecoveryRequired || !settled.Retired {
		return errors.New("retirement did not reach durable absence")
	}
	return retirement.ValidateObservation(ctx, settled)
}

// RunCompositeMonotonicRetirementV1 coordinates independently owned stores
// under an already-held external lease. Every owner is captured and validated
// before recovery or apply. Only the retirement owner may mutate, and every
// read-only owner must remain byte-stable across both mutation boundaries.
func RunCompositeMonotonicRetirementV1(
	ctx context.Context,
	readOnlyOwners []FixedPointOwnerV1,
	retirement MonotonicRetirementOwnerV1,
) error {
	if retirement == nil {
		return errors.New("composite retirement owner is unavailable")
	}
	owners := append(append([]FixedPointOwnerV1(nil), readOnlyOwners...), retirement)
	for _, owner := range owners {
		if owner == nil {
			return errors.New("composite retirement inventory owner is unavailable")
		}
	}
	observations, err := captureCompositeOwnerFixedPointV1(ctx, owners)
	if err != nil {
		return err
	}
	if err := validateCompositeOwnerFixedPointV1(ctx, owners, observations); err != nil {
		return err
	}
	retirementObservation := observations[len(observations)-1]
	if retirementObservation.RecoveryRequired {
		if err := retirement.Recover(ctx); err != nil {
			return err
		}
		if err := validateCompositeOwnerFixedPointV1(ctx, readOnlyOwners, observations[:len(readOnlyOwners)]); err != nil {
			return errors.New("owner recovery changed a read-only owner")
		}
		observations, err = captureCompositeOwnerFixedPointV1(ctx, owners)
		if err != nil {
			return err
		}
		if err := validateCompositeOwnerFixedPointV1(ctx, owners, observations); err != nil {
			return err
		}
		retirementObservation = observations[len(observations)-1]
		if retirementObservation.RecoveryRequired {
			return errors.New("owner recovery did not settle the transaction")
		}
	}
	prepared, err := retirement.Prepare(ctx)
	if err != nil {
		return err
	}
	if prepared == nil {
		return errors.New("composite retirement plan is unavailable")
	}
	// Preparation itself is read-only and must not invalidate the fixed point.
	if err := validateCompositeOwnerFixedPointV1(ctx, owners, observations); err != nil {
		return errors.New("owner preparation changed the composite fixed point")
	}
	if err := prepared.Validate(ctx); err != nil {
		return err
	}
	if err := prepared.Apply(ctx); err != nil {
		return err
	}
	if err := validateCompositeOwnerFixedPointV1(ctx, readOnlyOwners, observations[:len(readOnlyOwners)]); err != nil {
		return errors.New("owner retirement changed a read-only owner")
	}
	settled, err := retirement.Observe(ctx)
	if err != nil {
		return err
	}
	if settled.Digest == "" || settled.RecoveryRequired || !settled.Retired {
		return errors.New("owner retirement did not reach durable absence")
	}
	if err := retirement.ValidateObservation(ctx, settled); err != nil {
		return err
	}
	return nil
}

func captureCompositeOwnerFixedPointV1(
	ctx context.Context,
	owners []FixedPointOwnerV1,
) ([]OwnerObservationV1, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	observations := make([]OwnerObservationV1, 0, len(owners))
	for _, owner := range owners {
		observation, err := owner.Observe(ctx)
		if err != nil || observation.Digest == "" {
			return nil, errors.New("composite retirement owner observation failed")
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

func validateCompositeOwnerFixedPointV1(
	ctx context.Context,
	owners []FixedPointOwnerV1,
	observations []OwnerObservationV1,
) error {
	if len(owners) != len(observations) {
		return errors.New("composite retirement fixed point is incomplete")
	}
	for index, owner := range owners {
		if err := owner.ValidateObservation(ctx, observations[index]); err != nil {
			return errors.New("composite retirement fixed point changed")
		}
	}
	return nil
}
