package turnstart

import (
	"context"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type ContextEffectAcquirer func(context.Context, domainsecurity.TurnSecurityContext) (context.Context, func(), error)

// RunResearchStateEffect serializes project-local research state with every
// context transition and validates the exact frozen authority immediately
// before and after the write. The effect receives only the lease-bearing
// context; a stale case, workspace, epoch, snapshot, or risk head cannot be
// repaired or reminted here.
func RunResearchStateEffect(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	acquire ContextEffectAcquirer,
	validate func(context.Context) error,
	effect func(context.Context) error,
) error {
	if ctx == nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		acquire == nil || validate == nil || effect == nil {
		return errors.New("research state effect authority is unavailable")
	}
	effectCtx, release, err := acquire(ctx, securityContext)
	if err != nil {
		return err
	}
	if release == nil {
		return errors.New("research state effect lease is unavailable")
	}
	defer release()
	if err := validate(effectCtx); err != nil {
		return err
	}
	if err := effect(effectCtx); err != nil {
		return err
	}
	return validate(effectCtx)
}
